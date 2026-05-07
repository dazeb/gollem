package trace

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

// JSONLStreamWriter writes live canonical runtime-boundary events as JSONL.
// It is intentionally append-only and segment-local: final trace artifacts are
// still written separately, while this stream lets operators watch long runs.
type JSONLStreamWriter struct {
	mu     sync.Mutex
	w      io.Writer
	file   *os.File
	seq    int
	closed bool
}

// NewJSONLStreamWriter creates a live event stream writer. A path of "-"
// writes to stdout.
func NewJSONLStreamWriter(path string) (*JSONLStreamWriter, error) {
	path = strings.TrimSpace(path)
	if path == "" {
		return nil, errors.New("trace stream path is required")
	}
	if path == "-" {
		return &JSONLStreamWriter{w: os.Stdout}, nil
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, err
	}
	f, err := os.Create(path)
	if err != nil {
		return nil, err
	}
	return &JSONLStreamWriter{w: f, file: f}, nil
}

// WriteEvent writes one event as a JSON line. It assigns stream-local sequence
// numbers and event IDs without mutating the recorder's stored event.
func (s *JSONLStreamWriter) WriteEvent(event Event) {
	if s == nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed || s.w == nil {
		return
	}
	s.seq++
	event.Seq = s.seq
	if event.ID == "" {
		event.ID = fmt.Sprintf("live_evt_%06d", s.seq)
	}
	if event.ReplayPolicy == "" {
		event.ReplayPolicy = streamReplayPolicy(event.Kind)
	}
	data, err := json.Marshal(event)
	if err != nil {
		return
	}
	_, _ = s.w.Write(append(data, '\n'))
}

func streamReplayPolicy(kind string) string {
	switch kind {
	case "checkpoint.created":
		return "checkpoint"
	case "snapshot.created":
		return "snapshot"
	case "run.started", "run.completed", "run.failed", "turn.started", "turn.completed":
		return "inspect"
	default:
		return "recorded"
	}
}

// Close closes the underlying file when the stream owns one.
func (s *JSONLStreamWriter) Close() error {
	if s == nil {
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return nil
	}
	s.closed = true
	if s.file != nil {
		return s.file.Close()
	}
	return nil
}
