package deep

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/trevorprater/gollem"
)

// Checkpoint captures the state of an agent run at a point in time.
type Checkpoint struct {
	Messages  []gollem.ModelMessage `json:"-"`
	Usage     gollem.RunUsage      `json:"usage"`
	RunID     string               `json:"run_id"`
	Timestamp time.Time            `json:"timestamp"`
	Metadata  map[string]any       `json:"metadata,omitempty"`
}

// checkpointJSON is the serializable form of Checkpoint.
type checkpointJSON struct {
	Messages  []messageEnvelope `json:"messages"`
	Usage     gollem.RunUsage   `json:"usage"`
	RunID     string            `json:"run_id"`
	Timestamp time.Time         `json:"timestamp"`
	Metadata  map[string]any    `json:"metadata,omitempty"`
}

// messageEnvelope wraps a ModelMessage for JSON serialization.
type messageEnvelope struct {
	Kind    string          `json:"kind"` // "request" or "response"
	RawData json.RawMessage `json:"data"`
}

// MarshalJSON implements custom JSON marshaling for Checkpoint.
func (cp Checkpoint) MarshalJSON() ([]byte, error) {
	cj := checkpointJSON{
		Messages: encodeMessages(cp.Messages),
		Usage:    cp.Usage,
		RunID:    cp.RunID,
		Timestamp: cp.Timestamp,
		Metadata:  cp.Metadata,
	}
	return json.Marshal(cj)
}

// UnmarshalJSON implements custom JSON unmarshaling for Checkpoint.
func (cp *Checkpoint) UnmarshalJSON(data []byte) error {
	var cj checkpointJSON
	if err := json.Unmarshal(data, &cj); err != nil {
		return err
	}
	messages, err := decodeMessages(cj.Messages)
	if err != nil {
		return fmt.Errorf("decoding messages: %w", err)
	}
	cp.Usage = cj.Usage
	cp.RunID = cj.RunID
	cp.Timestamp = cj.Timestamp
	cp.Metadata = cj.Metadata
	cp.Messages = messages
	return nil
}

// CheckpointStore persists and retrieves checkpoints.
type CheckpointStore interface {
	Save(ctx context.Context, checkpoint *Checkpoint) error
	Load(ctx context.Context, runID string) (*Checkpoint, error)
	List(ctx context.Context) ([]*Checkpoint, error)
	Delete(ctx context.Context, runID string) error
}

// FileCheckpointStore stores checkpoints as JSON files.
type FileCheckpointStore struct {
	dir string
	mu  sync.Mutex
}

// NewFileCheckpointStore creates a file-based checkpoint store.
func NewFileCheckpointStore(dir string) (*FileCheckpointStore, error) {
	if dir == "" {
		dir = filepath.Join(os.TempDir(), "gollem-checkpoints")
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, fmt.Errorf("creating checkpoint directory: %w", err)
	}
	return &FileCheckpointStore{dir: dir}, nil
}

// Save persists a checkpoint to disk.
func (s *FileCheckpointStore) Save(_ context.Context, checkpoint *Checkpoint) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	data, err := json.Marshal(checkpoint)
	if err != nil {
		return fmt.Errorf("marshaling checkpoint: %w", err)
	}

	path := filepath.Join(s.dir, checkpoint.RunID+".json")
	if err := os.WriteFile(path, data, 0o600); err != nil {
		return fmt.Errorf("writing checkpoint: %w", err)
	}
	return nil
}

// Load retrieves a checkpoint by run ID.
func (s *FileCheckpointStore) Load(_ context.Context, runID string) (*Checkpoint, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	path := filepath.Join(s.dir, runID+".json")
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, fmt.Errorf("checkpoint %q not found", runID)
		}
		return nil, fmt.Errorf("reading checkpoint: %w", err)
	}

	var cp Checkpoint
	if err := json.Unmarshal(data, &cp); err != nil {
		return nil, fmt.Errorf("unmarshaling checkpoint: %w", err)
	}
	return &cp, nil
}

// List returns all stored checkpoints.
func (s *FileCheckpointStore) List(_ context.Context) ([]*Checkpoint, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	entries, err := os.ReadDir(s.dir)
	if err != nil {
		return nil, fmt.Errorf("listing checkpoints: %w", err)
	}

	var checkpoints []*Checkpoint
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".json" {
			continue
		}
		data, err := os.ReadFile(filepath.Join(s.dir, entry.Name()))
		if err != nil {
			continue
		}
		var cp Checkpoint
		if err := json.Unmarshal(data, &cp); err != nil {
			continue
		}
		checkpoints = append(checkpoints, &cp)
	}
	return checkpoints, nil
}

// Delete removes a checkpoint by run ID.
func (s *FileCheckpointStore) Delete(_ context.Context, runID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	path := filepath.Join(s.dir, runID+".json")
	if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("deleting checkpoint: %w", err)
	}
	return nil
}

// ResumeFromCheckpoint loads a checkpoint and returns a RunOption that
// initializes the agent run with the checkpoint's message history.
func ResumeFromCheckpoint(store CheckpointStore, runID string) (gollem.RunOption, error) {
	cp, err := store.Load(context.Background(), runID)
	if err != nil {
		return nil, fmt.Errorf("loading checkpoint: %w", err)
	}
	return gollem.WithMessages(cp.Messages...), nil
}
