package controller

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"net"
	"os"
	"time"

	"golang.org/x/crypto/ssh"

	"github.com/wallissonmarinho/oci-backup-healer/internal/config"
	"github.com/wallissonmarinho/oci-backup-healer/internal/notifier"
	"github.com/wallissonmarinho/oci-backup-healer/internal/provider"
	"github.com/wallissonmarinho/oci-backup-healer/internal/state"
)

type Reconciler struct {
	cfg      *config.Config
	prov     provider.Provider
	store    *state.Store
	notifier notifier.Notifier
	logger   *slog.Logger
}

func NewReconciler(
	cfg *config.Config,
	prov provider.Provider,
	store *state.Store,
	notifier notifier.Notifier,
	logger *slog.Logger,
) *Reconciler {
	return &Reconciler{
		cfg:      cfg,
		prov:     prov,
		store:    store,
		notifier: notifier,
		logger:   logger,
	}
}

// Reconcile executa um ciclo de checagem e remediação do ecossistema de backups
func (r *Reconciler) Reconcile(ctx context.Context) error {
	r.logger.Debug("Iniciando ciclo de reconciliacao")

	// 1. Carregar estado persistido
	st, err := r.store.Load()
	if err != nil {
		return fmt.Errorf("failed to load state store: %w", err)
	}

	// 2. Checar saude da VM primaria na API da OCI
	running, err := r.prov.IsInstanceRunning(ctx, r.cfg.PrimaryVMID)
	if err != nil {
		r.logger.Error("Erro ao consultar saude da VM primaria no provedor", "error", err)
		return err
	}

	if running {
		// Primaria saudavel
		if st.PrimaryStatus == "unhealthy" {
			r.logger.Info("VM Primaria recuperou a saude na OCI. Registrando normalidade.")
			st.PrimaryStatus = "healthy"
			st.LastTransition = time.Now()
			_ = r.store.Save(st)

			// Notificar recuperacao
			_ = r.notifier.Send(notifier.Event{
				Type:       notifier.EventPrimaryRecovered,
				Timestamp:  time.Now(),
				Message:    "A VM principal voltou ao status RUNNING na API da OCI.",
				InstanceID: r.cfg.PrimaryVMID,
				VolumeID:   r.cfg.VolumeID,
			})
		}

		if st.ActiveFailover {
			r.logger.Warn("ATENCAO: A VM primaria esta ativa, mas o failover de backup continua ativo na VM secundária. O failback deve ser executado manualmente por seguranca.")
		}

		return nil
	}

	// 3. VM Primaria inativa (running == false)
	if st.PrimaryStatus == "healthy" {
		r.logger.Warn("VM Primaria inativa detectada pela primeira vez. Iniciando contagem de tolerancia.")
		st.PrimaryStatus = "unhealthy"
		st.LastTransition = time.Now()
		_ = r.store.Save(st)

		// Notificar primeira queda
		_ = r.notifier.Send(notifier.Event{
			Type:       notifier.EventPrimaryDown,
			Timestamp:  time.Now(),
			Message:    "A VM principal esta offline na API da OCI. Tolerancia iniciada.",
			InstanceID: r.cfg.PrimaryVMID,
			VolumeID:   r.cfg.VolumeID,
		})

		return nil
	}

	// Se ja esta unhealthy, checar tolerancia
	unhealthyDuration := time.Since(st.LastTransition)
	if unhealthyDuration < r.cfg.UnhealthyBefore {
		r.logger.Info("VM Primaria inativa, aguardando expiracao da tolerancia",
			"unhealthy-duration", unhealthyDuration.String(),
			"tolerance-limit", r.cfg.UnhealthyBefore.String())
		return nil
	}

	// Tolerancia expirou! Iniciar Failover se ainda nao feito
	if !st.ActiveFailover {
		r.logger.Error("VM Primaria inativa excedeu a tolerancia! Iniciando processo de FAILOVER DO VOLUME DE BACKUPS!",
			"unhealthy-duration", unhealthyDuration.String())

		if r.cfg.DryRun {
			r.logger.Info("[DRY RUN] Failover simulado com sucesso. Nenhuma acao fisica executada.")
			st.ActiveFailover = true
			_ = r.store.Save(st)
			return nil
		}

		// Disparar Notificacao de inicio de Failover
		_ = r.notifier.Send(notifier.Event{
			Type:       notifier.EventFailoverStarted,
			Timestamp:  time.Now(),
			Message:    "Tolerancia estourada. Iniciando remocao e transferencia do Block Volume de backup.",
			InstanceID: r.cfg.BackupVMID,
			VolumeID:   r.cfg.VolumeID,
		})

		err := r.executeFailoverTransaction(ctx, st)
		if err != nil {
			r.logger.Error("ERRO CRITICO durante a execucao da transacao de failover", "error", err)
			_ = r.notifier.Send(notifier.Event{
				Type:       notifier.EventFailoverFailed,
				Timestamp:  time.Now(),
				Message:    fmt.Sprintf("Falha critica na transacao de failover do volume: %v", err),
				InstanceID: r.cfg.BackupVMID,
				VolumeID:   r.cfg.VolumeID,
			})
			return err
		}

		r.logger.Info("PROCESSO DE FAILOVER CONCLUIDO COM ABSOLUTO SUCESSO!")
	}

	return nil
}

// executeFailoverTransaction realiza a transacao idempotente de desconexao, conexao e montagem do volume
func (r *Reconciler) executeFailoverTransaction(ctx context.Context, st *state.HealerState) error {
	// Passo A: Localizar attachment ativo atual do volume (se houver)
	r.logger.Info("Localizando associacao ativa atual do volume na OCI...")
	attachID, currentVM, err := r.prov.GetActiveAttachmentForVolume(ctx, r.cfg.VolumeID)
	if err != nil {
		r.logger.Error("Erro ao consultar associacoes de volume na OCI", "error", err)
		return err
	}

	// Passo B: Desassociar da VM atual (se estiver anexado)
	if attachID != "" && currentVM != r.cfg.BackupVMID {
		r.logger.Warn("Volume encontrado associado a outra VM. Solicitando desassociacao...",
			"attachment-id", attachID, "instance-id", currentVM)

		err = r.prov.DetachVolume(ctx, attachID)
		if err != nil {
			return fmt.Errorf("failed to detach volume from %s: %w", currentVM, err)
		}

		// Aguardar desassociacao completa (DETACHED)
		r.logger.Info("Aguardando conclusao de desassociacao do volume na OCI...")
		for {
			info, err := r.prov.GetVolumeAttachment(ctx, attachID)
			if err != nil {
				// 404 indica que foi deletado (sucesso)
				break
			}
			if info.State == provider.StateDetached {
				break
			}
			r.logger.Debug("Aguardando volume...", "current-state", info.State)
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(5 * time.Second):
			}
		}
		r.logger.Info("Volume liberado com sucesso na OCI!")
	}

	// Passo C: Associar o volume na VM secundária (nossa VM de backup de contingência)
	r.logger.Info("Solicitando associacao do volume na VM secundaria...", "backup-vm-id", r.cfg.BackupVMID)
	newAttachID, err := r.prov.AttachVolume(ctx, r.cfg.BackupVMID, r.cfg.VolumeID)
	if err != nil {
		return fmt.Errorf("failed to attach volume to backup VM: %w", err)
	}

	// Passo D: Aguardar conclusao de associacao (ATTACHED)
	r.logger.Info("Aguardando conclusao de associacao na OCI e carga de dados iSCSI...")
	var attachInfo *provider.VolumeAttachmentInfo
	for {
		info, err := r.prov.GetVolumeAttachment(ctx, newAttachID)
		if err != nil {
			return fmt.Errorf("failed to check new volume attachment: %w", err)
		}
		if info.State == provider.StateAttached && info.IPv4 != "" && info.IQN != "" {
			attachInfo = info
			break
		}
		r.logger.Debug("Aguardando conexao volume...", "current-state", info.State)
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(5 * time.Second):
		}
	}
	r.logger.Info("Volume associado na OCI e dados de conexao iSCSI obtidos!", "ip", attachInfo.IPv4, "iqn", attachInfo.IQN)

	// Passo E: Efetuar conexao fisica iSCSI e montagem via SSH Remoto
	r.logger.Info("Iniciando comandos de conexao fisica iSCSI e montagem via SSH Remoto...")
	err = r.mountVolumeRemote(attachInfo)
	if err != nil {
		return fmt.Errorf("failed to mount volume remotely via SSH: %w", err)
	}

	// Passo F: Salvar estado de conclusao
	st.ActiveFailover = true
	st.VolumeAttachmentID = newAttachID
	st.LastTransition = time.Now()
	if err := r.store.Save(st); err != nil {
		return fmt.Errorf("failed to persist success failover state: %w", err)
	}

	// Notificar sucesso absoluto
	_ = r.notifier.Send(notifier.Event{
		Type:       notifier.EventFailoverSuccess,
		Timestamp:  time.Now(),
		Message:    "O Block Volume foi montado fisicamente e os cronjobs locais estao ativos na VM secundaria via failover automatico.",
		InstanceID: r.cfg.BackupVMID,
		VolumeID:   r.cfg.VolumeID,
	})

	return nil
}

// mountVolumeRemote executa os comandos do shell de forma remota na VM 2 via SSH
func (r *Reconciler) mountVolumeRemote(info *provider.VolumeAttachmentInfo) error {
	if r.cfg.BackupVMIP == "" {
		return fmt.Errorf("backup VM IP is empty in config, cannot establish SSH connection")
	}
	targetIP := r.cfg.BackupVMIP
	user := "ubuntu"
	keyPath := "/etc/healer/ssh_private_key"

	r.logger.Info("Lendo chave privada SSH...", "path", keyPath)
	key, err := os.ReadFile(keyPath)
	if err != nil {
		return fmt.Errorf("failed to read ssh private key: %w", err)
	}

	signer, err := ssh.ParsePrivateKey(key)
	if err != nil {
		return fmt.Errorf("failed to parse private key: %w", err)
	}

	config := &ssh.ClientConfig{
		User: user,
		Auth: []ssh.AuthMethod{
			ssh.PublicKeys(signer),
		},
		HostKeyCallback: ssh.InsecureIgnoreHostKey(), // Seguro pois roda inteiramente dentro da VPN Tailscale
		Timeout:         15 * time.Second,
	}

	addr := net.JoinHostPort(targetIP, "22")
	r.logger.Info("Conectando via SSH na VM secundaria...", "address", addr)
	client, err := ssh.Dial("tcp", addr, config)
	if err != nil {
		return fmt.Errorf("failed to dial ssh connection: %w", err)
	}
	defer client.Close()

	portal := fmt.Sprintf("%s:%d", info.IPv4, info.Port)
	r.logger.Info("Enviando comandos iSCSI e montagem via sessao SSH...")

	session, err := client.NewSession()
	if err != nil {
		return fmt.Errorf("failed to create ssh session: %w", err)
	}
	defer session.Close()

	// Pipeline de comandos que serão executados na VM 2
	cmd := fmt.Sprintf(`
		sudo iscsiadm -m discoverydb -t sendtargets -p %[1]s --discover && \
		sudo iscsiadm -m node -T %[2]s -p %[1]s -l && \
		sudo iscsiadm -m node -T %[2]s -p %[1]s --op update -n node.startup -v automatic && \
		sleep 3 && \
		sudo mkdir -p /backup && \
		sudo mount /dev/sdb /backup && \
		sudo systemctl start cron
	`, portal, info.IQN)

	r.logger.Info("Executando comandos remotos iSCSI...")
	stdout, err := session.StdoutPipe()
	if err != nil {
		return fmt.Errorf("failed to setup stdout pipe: %w", err)
	}
	stderr, err := session.StderrPipe()
	if err != nil {
		return fmt.Errorf("failed to setup stderr pipe: %w", err)
	}

	if err := session.Start(cmd); err != nil {
		return fmt.Errorf("failed to start remote command execution: %w", err)
	}

	outBytes, _ := io.ReadAll(stdout)
	errBytes, _ := io.ReadAll(stderr)

	if err := session.Wait(); err != nil {
		r.logger.Error("Comando remoto retornou erro", "stdout", string(outBytes), "stderr", string(errBytes))
		return fmt.Errorf("remote ssh execution failed: %w", err)
	}

	r.logger.Info("Comandos iSCSI e montagem remota executados com sucesso!", "stdout", string(outBytes))
	return nil
}
