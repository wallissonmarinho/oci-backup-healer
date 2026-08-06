package notifier

import (
	"bytes"
	"fmt"
	"net/http"
	"strings"
	"time"
)

type NtfyNotifier struct {
	topicURL string
}

// NewNtfyNotifier cria um novo canal de notificacoes via ntfy
func NewNtfyNotifier(topicURL string) *NtfyNotifier {
	return &NtfyNotifier{topicURL: topicURL}
}

// Send formata e dispara o evento estruturado para o webhook do ntfy
func (n *NtfyNotifier) Send(event Event) error {
	if n.topicURL == "" {
		return nil // Notificador desabilitado
	}

	title := "⚠️ OCI Backup Healer Alert"
	priority := "3" // Default
	tags := "backup"

	switch event.Type {
	case EventPrimaryDown:
		title = "🚨 Primary Backup VM Down"
		priority = "4" // High
		tags = "skull,warning"
	case EventFailoverStarted:
		title = "🔄 Failover Process Started"
		priority = "3"
		tags = "recycle"
	case EventFailoverSuccess:
		title = "✅ Failover Completed Successfully"
		priority = "5" // Urgent
		tags = "white_check_mark,floppy_disk"
	case EventFailoverFailed:
		title = "❌ Failover CRITICAL FAILURE"
		priority = "5" // Urgent
		tags = "x,fire,emergency"
	case EventPrimaryRecovered:
		title = "💚 Primary VM Recovered"
		priority = "3"
		tags = "healing,rocket"
	}

	body := fmt.Sprintf("[%s] %s\nInstance: %s\nVolume: %s",
		event.Timestamp.Format(time.RFC822),
		event.Message,
		event.InstanceID,
		event.VolumeID,
	)

	req, err := http.NewRequest("POST", n.topicURL, bytes.NewBufferString(body))
	if err != nil {
		return fmt.Errorf("failed to create http request: %w", err)
	}

	req.Header.Set("Title", title)
	req.Header.Set("Priority", priority)
	req.Header.Set("Tags", tags)

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("failed to dispatch ntfy request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("ntfy returned unexpected status code: %d", resp.StatusCode)
	}

	// Logging local silencioso do sucesso do envio
	return nil
}
