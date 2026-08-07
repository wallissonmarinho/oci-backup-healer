package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/wallissonmarinho/oci-backup-healer/internal/config"
	"github.com/wallissonmarinho/oci-backup-healer/internal/controller"
	"github.com/wallissonmarinho/oci-backup-healer/internal/notifier"
	"github.com/wallissonmarinho/oci-backup-healer/internal/provider"
	"github.com/wallissonmarinho/oci-backup-healer/internal/provider/fake"
	"github.com/wallissonmarinho/oci-backup-healer/internal/provider/oci"
	"github.com/wallissonmarinho/oci-backup-healer/internal/state"
)

func main() {
	// 1. Configurar Logger Estruturado slog (JSON)
	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	}))
	slog.SetDefault(logger)

	logger.Info("=== oci-backup-healer Iniciando ===")

	// 2. Carregar configuracoes via CLI/Env
	cfg := config.LoadConfig()

	// 3. Escolher o Provedor
	var prov provider.Provider
	var err error

	if cfg.UseFakeProvider {
		logger.Info("Modo FAKE ativo! Utilizando OCI Provider simulado.")
		prov = fake.NewFakeProvider()
	} else {
		logger.Info("Inicializando provedor real OCI SDK...", "profile", cfg.OCIProfile)
		prov, err = oci.NewOCIProvider(cfg)
		if err != nil {
			logger.Error("Erro fatal ao inicializar OCI Provider", "error", err)
			os.Exit(1)
		}
	}

	// 4. Instanciar persistidor de estado local
	statePath := "/tmp/healer-state.json"
	if cfg.UseFakeProvider {
		statePath = "./healer-state.json"
	}
	store := state.NewStore(statePath)

	// 5. Instanciar notificador
	var notif notifier.Notifier
	if cfg.NtfyTopicURL != "" {
		logger.Info("Notificador ntfy ativo", "url", cfg.NtfyTopicURL)
		notif = notifier.NewNtfyNotifier(cfg.NtfyTopicURL)
	} else {
		logger.Warn("Nenhum canal de notificacao configurado.")
		notif = notifier.NewNtfyNotifier("")
	}

	// 6. Criar Reconciler Controller
	reconciler := controller.NewReconciler(cfg, prov, store, notif, logger)

	// 7. Configurar canal de contexto e sinais de encerramento do SO
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		sig := <-sigChan
		logger.Info("Sinal de parada recebido. Encerrando daemon...", "signal", sig.String())
		cancel()
	}()

	// 8. Iniciar Loop de Reconciliacao
	logger.Info("Loop de Reconciliacao iniciado com sucesso!",
		"interval", cfg.Interval.String(),
		"tolerance", cfg.UnhealthyBefore.String(),
		"dry-run", cfg.DryRun)

	ticker := time.NewTicker(cfg.Interval)
	defer ticker.Stop()

	// Rodar primeira reconciliacao imediatamente
	if err := reconciler.Reconcile(ctx); err != nil {
		logger.Error("Erro na reconciliacao inicial", "error", err)
	}

	for {
		select {
		case <-ctx.Done():
			logger.Info("oci-backup-healer finalizado com sucesso.")
			return
		case <-ticker.C:
			if err := reconciler.Reconcile(ctx); err != nil {
				logger.Error("Erro no ciclo de reconciliacao", "error", err)
			}
		}
	}
}
