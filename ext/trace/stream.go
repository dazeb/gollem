package trace

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/fugue-labs/gollem/core"
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
	case "run.started", "run.completed", "run.failed", "turn.started", "turn.completed", "turn.failed", "wait.started", "wait.resolved":
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

// ReadJSONLStreamFile reconstructs a minimal canonical artifact from a live
// stream file. This is used for local running-run export before the final
// trace artifact has been written.
func ReadJSONLStreamFile(path string) (*Artifact, error) {
	path = strings.TrimSpace(path)
	if path == "" || path == "-" {
		return nil, errors.New("trace stream path is required")
	}
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	return ReadJSONLStream(f)
}

// ReadJSONLStream reconstructs a minimal canonical artifact from JSONL runtime
// events. It intentionally does not invent snapshots or embedded RunTrace data.
func ReadJSONLStream(r io.Reader) (*Artifact, error) {
	if r == nil {
		return nil, errors.New("nil trace stream")
	}
	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 0, 64*1024), 16*1024*1024)
	var events []Event
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		var event Event
		if err := json.Unmarshal([]byte(line), &event); err != nil {
			return nil, fmt.Errorf("decode stream event: %w", err)
		}
		events = append(events, event)
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	if len(events) == 0 {
		return nil, errors.New("trace stream is empty")
	}
	events = core.NormalizeTraceEvents(events)
	runID := streamRunID(events)
	if runID == "" {
		return nil, errors.New("trace stream is missing run id")
	}
	started, ended := streamTimeBounds(events)
	status, success, errText := streamStatus(events)
	metadata := map[string]any{
		"source":             "trace_stream",
		"partial":            status == "running",
		"partial_reason":     "live_stream",
		"stream_event_count": len(events),
	}
	if parentRunID := streamParentRunID(events, runID); parentRunID != "" {
		metadata["stream_parent_run_id"] = parentRunID
	}
	artifact := &Artifact{
		SchemaVersion: SchemaVersion,
		Run: RunMetadata{
			ID:             runID,
			Prompt:         streamPrompt(events),
			StartedAt:      started,
			EndedAt:        ended,
			DurationMillis: streamDurationMillis(started, ended),
			RuntimeVersion: "stream",
			Mode:           "stream",
		},
		Metadata: metadata,
		Events:   events,
		Summary: Summary{
			Status:         status,
			Success:        success,
			Error:          errText,
			Steps:          streamMaxStep(events),
			Requests:       streamCount(events, "model.requested"),
			ToolCalls:      streamCount(events, "tool.called"),
			DurationMillis: streamDurationMillis(started, ended),
		},
	}
	return artifact, nil
}

func streamRunID(events []Event) string {
	for _, event := range events {
		if event.Kind == "run.started" && strings.TrimSpace(event.AgentID) != "" {
			return strings.TrimSpace(event.AgentID)
		}
	}
	for _, event := range events {
		if strings.TrimSpace(event.AgentID) != "" {
			return strings.TrimSpace(event.AgentID)
		}
	}
	return ""
}

func streamPrompt(events []Event) string {
	for _, event := range events {
		if event.Kind == "run.started" {
			return firstPayloadString(event, "prompt")
		}
	}
	return ""
}

func streamParentRunID(events []Event, runID string) string {
	for _, event := range events {
		if event.Kind == "run.started" && event.AgentID == runID && strings.TrimSpace(event.CausalParentID) != "" {
			return strings.TrimSpace(event.CausalParentID)
		}
	}
	return ""
}

func streamTimeBounds(events []Event) (time.Time, time.Time) {
	var started, ended time.Time
	for _, event := range events {
		if event.Timestamp.IsZero() {
			continue
		}
		if started.IsZero() || event.Timestamp.Before(started) {
			started = event.Timestamp
		}
		if ended.IsZero() || event.Timestamp.After(ended) {
			ended = event.Timestamp
		}
	}
	return started, ended
}

func streamStatus(events []Event) (string, bool, string) {
	status := "running"
	var errText string
	for _, event := range events {
		switch event.Kind {
		case "run.completed":
			status = "succeeded"
			errText = ""
		case "run.failed":
			status = "failed"
			errText = firstPayloadString(event, "error")
		}
	}
	return status, status == "succeeded", errText
}

func streamMaxStep(events []Event) int {
	maxStep := 0
	for _, event := range events {
		if event.Step > maxStep {
			maxStep = event.Step
		}
	}
	return maxStep
}

func streamCount(events []Event, kind string) int {
	count := 0
	for _, event := range events {
		if event.Kind == kind {
			count++
		}
	}
	return count
}

func streamDurationMillis(started, ended time.Time) int64 {
	if started.IsZero() || ended.IsZero() || ended.Before(started) {
		return 0
	}
	return ended.Sub(started).Milliseconds()
}
