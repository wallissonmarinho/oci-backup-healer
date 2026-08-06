package provider

import (
	"context"
)

// VolumeAttachmentState representa os estados de associação na nuvem
type VolumeAttachmentState string

const (
	StateAttaching VolumeAttachmentState = "ATTACHING"
	StateAttached  VolumeAttachmentState = "ATTACHED"
	StateDetaching VolumeAttachmentState = "DETACHING"
	StateDetached  VolumeAttachmentState = "DETACHED"
	StateUnknown   VolumeAttachmentState = "UNKNOWN"
)

// VolumeAttachmentInfo contem os dados necessarios para conexao iSCSI no Linux
type VolumeAttachmentInfo struct {
	ID         string
	InstanceID string
	VolumeID   string
	State      VolumeAttachmentState
	IPv4       string
	Port       int
	IQN        string
}

// Provider define a interface abstrata de comunicacao com a nuvem (OCI ou Fake)
type Provider interface {
	// IsInstanceRunning checa se a VM especificada esta com status RUNNING na API
	IsInstanceRunning(ctx context.Context, instanceID string) (bool, error)

	// GetActiveAttachmentForVolume localiza a associacao ativa de um volume de bloco
	GetActiveAttachmentForVolume(ctx context.Context, volumeID string) (attachmentID string, instanceID string, err error)

	// DetachVolume solicita a desconexao do Block Volume
	DetachVolume(ctx context.Context, volumeAttachmentID string) error

	// AttachVolume associa o Block Volume a instancia e retorna o ID do novo attachment
	AttachVolume(ctx context.Context, instanceID, volumeID string) (string, error)

	// GetVolumeAttachment consulta o status e dados de conexao iSCSI do attachment
	GetVolumeAttachment(ctx context.Context, volumeAttachmentID string) (*VolumeAttachmentInfo, error)
}
