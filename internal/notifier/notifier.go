package notifier

import (
	"time"
)

// EventType categoriza as acoes do Healer de backup
type EventType string

const (
	EventPrimaryDown      EventType = "PrimaryVMDown"
	EventFailoverStarted  EventType = "FailoverStarted"
	EventFailoverSuccess  EventType = "FailoverSuccess"
	EventFailoverFailed   EventType = "FailoverFailed"
	EventPrimaryRecovered EventType = "PrimaryVMRecovered"
)

// Event armazena dados estruturados para notificacao
type Event struct {
	Type       EventType
	Timestamp  time.Time
	Message    string
	InstanceID string
	VolumeID   string
}

// Notifier define o contrato para disparar alertas do Healer
type Notifier interface {
	Send(event Event) error
}
