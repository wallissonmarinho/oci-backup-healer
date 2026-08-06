package state

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestStateSaveAndLoad(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "healer-test")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	statePath := filepath.Join(tmpDir, "state.json")
	store := NewStore(statePath)

	// 1. Validar carga inicial (arquivo nao existente deve retornar estado limpo)
	initialState, err := store.Load()
	if err != nil {
		t.Fatalf("failed to load initial state: %v", err)
	}
	if initialState.PrimaryStatus != "healthy" {
		t.Errorf("expected initial status to be healthy, got %s", initialState.PrimaryStatus)
	}
	if initialState.ActiveFailover {
		t.Errorf("expected initial failover status to be false, got true")
	}

	// 2. Modificar e Gravar Estado
	now := time.Now().Round(time.Second) // Evitar discrepancia de precisao de milissegundos
	testState := &HealerState{
		PrimaryStatus:      "unhealthy",
		ActiveFailover:     true,
		VolumeAttachmentID: "att-12345",
		LastTransition:     now,
	}

	err = store.Save(testState)
	if err != nil {
		t.Fatalf("failed to save state: %v", err)
	}

	// 3. Carregar e Validar se bate com o gravado
	loadedState, err := store.Load()
	if err != nil {
		t.Fatalf("failed to load saved state: %v", err)
	}

	if loadedState.PrimaryStatus != testState.PrimaryStatus {
		t.Errorf("status mismatch: got %s, want %s", loadedState.PrimaryStatus, testState.PrimaryStatus)
	}
	if loadedState.ActiveFailover != testState.ActiveFailover {
		t.Errorf("activeFailover mismatch: got %t, want %t", loadedState.ActiveFailover, testState.ActiveFailover)
	}
	if loadedState.VolumeAttachmentID != testState.VolumeAttachmentID {
		t.Errorf("attachmentID mismatch: got %s, want %s", loadedState.VolumeAttachmentID, testState.VolumeAttachmentID)
	}
	if !loadedState.LastTransition.Equal(testState.LastTransition) {
		t.Errorf("timestamp mismatch: got %s, want %s", loadedState.LastTransition, testState.LastTransition)
	}
}
