package fake

import (
	"context"
	"fmt"
	"sync"

	"github.com/wallissonmarinho/oci-backup-healer/internal/provider"
)

type FakeProvider struct {
	mu           sync.Mutex
	instances    map[string]bool // true = RUNNING, false = STOPPED
	attachments  map[string]*provider.VolumeAttachmentInfo
	attachmentID int
}

func NewFakeProvider() *FakeProvider {
	return &FakeProvider{
		instances: map[string]bool{
			"oci-sp-c-micro-1": true,
			"oci-sp-c-micro-2": true,
		},
		attachments: make(map[string]*provider.VolumeAttachmentInfo),
	}
}

// SetInstanceStatus altera artificialmente o status de saude de uma VM para testes
func (f *FakeProvider) SetInstanceStatus(instanceID string, running bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.instances[instanceID] = running
}

func (f *FakeProvider) IsInstanceRunning(ctx context.Context, instanceID string) (bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	running, ok := f.instances[instanceID]
	if !ok {
		return false, fmt.Errorf("instance not found: %s", instanceID)
	}
	return running, nil
}

func (f *FakeProvider) GetActiveAttachmentForVolume(ctx context.Context, volumeID string) (string, string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	for _, att := range f.attachments {
		if att.VolumeID == volumeID && att.State != provider.StateDetached {
			return att.ID, att.InstanceID, nil
		}
	}
	return "", "", nil
}

func (f *FakeProvider) DetachVolume(ctx context.Context, volumeAttachmentID string) error {
	f.mu.Lock()
	defer f.mu.Unlock()

	att, ok := f.attachments[volumeAttachmentID]
	if !ok {
		return fmt.Errorf("attachment not found: %s", volumeAttachmentID)
	}

	att.State = provider.StateDetached
	return nil
}

func (f *FakeProvider) AttachVolume(ctx context.Context, instanceID, volumeID string) (string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	f.attachmentID++
	id := fmt.Sprintf("fake-attach-%d", f.attachmentID)

	att := &provider.VolumeAttachmentInfo{
		ID:         id,
		InstanceID: instanceID,
		VolumeID:   volumeID,
		State:      provider.StateAttached,
		IPv4:       "169.254.2.2",
		Port:       3260,
		IQN:        "iqn.2015-12.com.oracleiaas:fake-iqn-volume-backup",
	}

	f.attachments[id] = att
	return id, nil
}

func (f *FakeProvider) GetVolumeAttachment(ctx context.Context, volumeAttachmentID string) (*provider.VolumeAttachmentInfo, error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	att, ok := f.attachments[volumeAttachmentID]
	if !ok {
		return nil, fmt.Errorf("attachment not found: %s", volumeAttachmentID)
	}
	return att, nil
}
