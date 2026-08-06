package oci

import (
	"context"
	"fmt"

	"github.com/oracle/oci-go-sdk/v65/common"
	"github.com/oracle/oci-go-sdk/v65/common/auth"
	"github.com/oracle/oci-go-sdk/v65/core"
	"github.com/wallissonmarinho/oci-backup-healer/internal/config"
	"github.com/wallissonmarinho/oci-backup-healer/internal/provider"
)

type OCIProvider struct {
	computeClient core.ComputeClient
}

// NewOCIProvider inicializa os clients de conexao com a OCI usando o profile do arquivo de config
func NewOCIProvider(cfg *config.Config) (*OCIProvider, error) {
	var authProvider common.ConfigurationProvider
	var err error

	if cfg.OCIConfigFile != "" {
		authProvider, err = common.ConfigurationProviderFromFileWithProfile(cfg.OCIConfigFile, cfg.OCIProfile, "")
		if err != nil {
			return nil, fmt.Errorf("failed to load configuration provider from file: %w", err)
		}
	} else {
		// Fallback para Instance Principal se arquivo nao fornecido
		authProvider, err = auth.InstancePrincipalConfigurationProvider()
		if err != nil {
			return nil, fmt.Errorf("failed to initialize instance principal authentication: %w", err)
		}
	}

	computeClient, err := core.NewComputeClientWithConfigurationProvider(authProvider)
	if err != nil {
		return nil, fmt.Errorf("failed to create compute client: %w", err)
	}

	return &OCIProvider{
		computeClient: computeClient,
	}, nil
}

// IsInstanceRunning consulta o estado de vida da VM primaria
func (o *OCIProvider) IsInstanceRunning(ctx context.Context, instanceID string) (bool, error) {
	resp, err := o.computeClient.GetInstance(ctx, core.GetInstanceRequest{
		InstanceId: common.String(instanceID),
	})
	if err != nil {
		return false, fmt.Errorf("failed to get instance: %w", err)
	}

	return resp.Instance.LifecycleState == core.InstanceLifecycleStateRunning, nil
}

// GetActiveAttachmentForVolume localiza a associacao ativa de um volume de bloco
func (o *OCIProvider) GetActiveAttachmentForVolume(ctx context.Context, volumeID string) (string, string, error) {
	// Listar associacoes no compartimento root
	// Nota: Como em Always Free o compartment-id padrão é o tenancy root:
	prov := o.computeClient.ConfigurationProvider()
	tenancyID, err := (*prov).TenancyOCID()
	if err != nil {
		return "", "", fmt.Errorf("failed to obtain tenancy id from auth provider: %w", err)
	}

	resp, err := o.computeClient.ListVolumeAttachments(ctx, core.ListVolumeAttachmentsRequest{
		CompartmentId: common.String(tenancyID),
		VolumeId:      common.String(volumeID),
	})
	if err != nil {
		return "", "", fmt.Errorf("failed to list volume attachments: %w", err)
	}

	for _, att := range resp.Items {
		state := VolumeAttachmentStateMapping(att.GetLifecycleState())
		if state != provider.StateDetached && state != provider.StateUnknown {
			return *att.GetId(), *att.GetInstanceId(), nil
		}
	}

	return "", "", nil
}

// DetachVolume solicita a desconexao do Block Volume
func (o *OCIProvider) DetachVolume(ctx context.Context, volumeAttachmentID string) error {
	_, err := o.computeClient.DetachVolume(ctx, core.DetachVolumeRequest{
		VolumeAttachmentId: common.String(volumeAttachmentID),
	})
	if err != nil {
		return fmt.Errorf("failed to execute OCI detach command: %w", err)
	}
	return nil
}

// AttachVolume associa o Block Volume a instancia e retorna o ID do novo attachment
func (o *OCIProvider) AttachVolume(ctx context.Context, instanceID, volumeID string) (string, error) {
	resp, err := o.computeClient.AttachVolume(ctx, core.AttachVolumeRequest{
		AttachVolumeDetails: core.AttachIScsiVolumeDetails{
			InstanceId: common.String(instanceID),
			VolumeId:      common.String(volumeID),
		},
	})
	if err != nil {
		return "", fmt.Errorf("failed to execute OCI attach command: %w", err)
	}
	return *resp.VolumeAttachment.GetId(), nil
}

// GetVolumeAttachment consulta o status e dados de conexao iSCSI do attachment
func (o *OCIProvider) GetVolumeAttachment(ctx context.Context, volumeAttachmentID string) (*provider.VolumeAttachmentInfo, error) {
	resp, err := o.computeClient.GetVolumeAttachment(ctx, core.GetVolumeAttachmentRequest{
		VolumeAttachmentId: common.String(volumeAttachmentID),
	})
	if err != nil {
		return nil, fmt.Errorf("failed to get volume attachment: %w", err)
	}

	att := resp.VolumeAttachment
	info := &provider.VolumeAttachmentInfo{
		ID:         *att.GetId(),
		InstanceID: *att.GetInstanceId(),
		VolumeID:   *att.GetVolumeId(),
		State:      VolumeAttachmentStateMapping(att.GetLifecycleState()),
	}

	// Tentar obter dados iSCSI se for desse tipo
	if iscsi, ok := att.(core.IScsiVolumeAttachment); ok {
		if iscsi.Ipv4 != nil {
			info.IPv4 = *iscsi.Ipv4
		}
		if iscsi.Port != nil {
			info.Port = *iscsi.Port
		}
		if iscsi.Iqn != nil {
			info.IQN = *iscsi.Iqn
		}
	}

	return info, nil
}

// VolumeAttachmentStateMapping mapeia o enum do SDK para a nossa interface
func VolumeAttachmentStateMapping(state core.VolumeAttachmentLifecycleStateEnum) provider.VolumeAttachmentState {
	switch state {
	case core.VolumeAttachmentLifecycleStateAttaching:
		return provider.StateAttaching
	case core.VolumeAttachmentLifecycleStateAttached:
		return provider.StateAttached
	case core.VolumeAttachmentLifecycleStateDetaching:
		return provider.StateDetaching
	case core.VolumeAttachmentLifecycleStateDetached:
		return provider.StateDetached
	default:
		return provider.StateUnknown
	}
}
