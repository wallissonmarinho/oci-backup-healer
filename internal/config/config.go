package config

import (
	"flag"
	"os"
	"strconv"
	"time"
)

// Config armazena os parametros de execucao do oci-backup-healer
type Config struct {
	OCIConfigFile   string
	OCIProfile      string
	PrimaryVMID     string
	BackupVMID      string
	VolumeID        string
	Interval        time.Duration
	UnhealthyBefore time.Duration
	NtfyTopicURL    string
	DryRun          bool
	UseFakeProvider bool
}

// LoadConfig parseia flags e variaveis de ambiente retornando a configuracao de runtime
func LoadConfig() *Config {
	cfg := &Config{}

	flag.StringVar(&cfg.OCIConfigFile, "oci-config-file", getEnv("OCI_CONFIG_FILE", ""), "Caminho do arquivo de credenciais da OCI")
	flag.StringVar(&cfg.OCIProfile, "oci-profile", getEnv("OCI_PROFILE", "DEFAULT"), "Perfil de conexao da OCI")
	flag.StringVar(&cfg.PrimaryVMID, "primary-vm-id", getEnv("PRIMARY_VM_ID", ""), "OCID da VM primaria (oci-sp-c-micro-1)")
	flag.StringVar(&cfg.BackupVMID, "backup-vm-id", getEnv("BACKUP_VM_ID", ""), "OCID da VM de contingencia (oci-sp-c-micro-2)")
	flag.StringVar(&cfg.VolumeID, "volume-id", getEnv("VOLUME_ID", ""), "OCID do Block Volume de backup")
	flag.StringVar(&cfg.NtfyTopicURL, "ntfy-topic-url", getEnv("NTFY_TOPIC_URL", ""), "URL do canal ntfy para notificacoes")
	flag.BoolVar(&cfg.DryRun, "dry-run", getEnvBool("DRY_RUN", false), "Executar apenas simulacao (sem modificar volumes na nuvem)")
	flag.BoolVar(&cfg.UseFakeProvider, "fake", getEnvBool("FAKE_PROVIDER", false), "Utilizar provedor simulado para testes locais")

	// Intervalos de tempo
	var intervalSecs, unhealthySecs int
	flag.IntVar(&intervalSecs, "interval", getEnvInt("RECONCILE_INTERVAL_SECS", 30), "Intervalo de reconciliacao em segundos")
	flag.IntVar(&unhealthySecs, "unhealthy-timeout", getEnvInt("UNHEALTHY_TIMEOUT_SECS", 300), "Tempo minimo inativo para considerar a VM 1 em falha")

	flag.Parse()

	cfg.Interval = time.Duration(intervalSecs) * time.Second
	cfg.UnhealthyBefore = time.Duration(unhealthySecs) * time.Second

	return cfg
}

func getEnv(key, defaultVal string) string {
	if val, ok := os.LookupEnv(key); ok {
		return val
	}
	return defaultVal
}

func getEnvInt(key string, defaultVal int) int {
	if val, ok := os.LookupEnv(key); ok {
		if i, err := strconv.Atoi(val); err == nil {
			return i
		}
	}
	return defaultVal
}

func getEnvBool(key string, defaultVal bool) bool {
	if val, ok := os.LookupEnv(key); ok {
		if b, err := strconv.ParseBool(val); err == nil {
			return b
		}
	}
	return defaultVal
}
