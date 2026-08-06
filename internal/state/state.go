package state

import (
	"encoding/json"
	"fmt"
	"os"
	"sync"
	"time"
)

type HealerState struct {
	PrimaryStatus      string    `json:"primaryStatus"` // "healthy", "unhealthy"
	ActiveFailover     bool      `json:"activeFailover"`
	VolumeAttachmentID string    `json:"volumeAttachmentID,omitempty"`
	LastTransition     time.Time `json:"lastTransition"`
}

type Store struct {
	path string
	mu   sync.RWMutex
}

// NewStore inicializa o persistidor do estado em arquivo
func NewStore(path string) *Store {
	return &Store{path: path}
}

// Load le o arquivo de estado em disco
func (s *Store) Load() (*HealerState, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	file, err := os.Open(s.path)
	if err != nil {
		if os.IsNotExist(err) {
			// Retorna estado limpo inicial
			return &HealerState{
				PrimaryStatus:  "healthy",
				LastTransition: time.Now(),
			}, nil
		}
		return nil, fmt.Errorf("failed to open state file: %w", err)
	}
	defer file.Close()

	var state HealerState
	if err := json.NewDecoder(file).Decode(&state); err != nil {
		return nil, fmt.Errorf("failed to decode state json: %w", err)
	}

	return &state, nil
}

// Save grava o estado de forma atomica em disco
func (s *Store) Save(state *HealerState) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	tmpPath := s.path + ".tmp"
	file, err := os.OpenFile(tmpPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0644)
	if err != nil {
		return fmt.Errorf("failed to create temp state file: %w", err)
	}
	defer file.Close()

	if err := json.NewEncoder(file).Encode(state); err != nil {
		return fmt.Errorf("failed to encode state json: %w", err)
	}
	file.Sync()

	// Substituicao atomica
	if err := os.Rename(tmpPath, s.path); err != nil {
		return fmt.Errorf("failed to rename temp file to state path: %w", err)
	}

	return nil
}
