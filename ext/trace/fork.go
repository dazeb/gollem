package trace

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/fugue-labs/gollem/core"
)

// ForkOptions configures snapshot extraction for a branch run.
type ForkOptions struct {
	FromStep       int
	FromEventID    string
	FromCheckpoint string
	FromKind       string
	NewRunID       string
	Prompt         string
	SystemPrompt   string
	PlannerPrompt  string
	AppendUser     string
	Model          string
	Topology       string
	Middleware     string
	ToolPolicy     string
	Evaluator      string
	MemoryEdits    []string
}

// ForkSnapshot extracts a snapshot anchor from an artifact and returns a
// branchable RunSnapshot. The caller can pass the result to core.WithSnapshot.
func ForkSnapshot(artifact *Artifact, opts ForkOptions) (*core.RunSnapshot, SnapshotRecord, error) {
	if artifact == nil {
		return nil, SnapshotRecord{}, errors.New("nil trace artifact")
	}
	var record SnapshotRecord
	if strings.TrimSpace(opts.FromCheckpoint) != "" {
		var err error
		record, err = selectSnapshotByCheckpoint(artifact, opts.FromCheckpoint)
		if err != nil {
			return nil, SnapshotRecord{}, err
		}
	} else {
		step, err := resolveForkStep(artifact, opts)
		if err != nil {
			return nil, SnapshotRecord{}, err
		}
		var selectErr error
		record, selectErr = selectSnapshot(artifact.Snapshots, step)
		if selectErr != nil {
			return nil, SnapshotRecord{}, selectErr
		}
	}
	snap, err := DecodeSnapshotRecord(record)
	if err != nil {
		return nil, SnapshotRecord{}, err
	}

	if strings.TrimSpace(opts.AppendUser) != "" {
		snap = snap.Branch(func(messages []core.ModelMessage) []core.ModelMessage {
			return append(messages, core.ModelRequest{
				Parts: []core.ModelRequestPart{
					core.UserPromptPart{
						Content:   opts.AppendUser,
						Timestamp: time.Now(),
					},
				},
				Timestamp: time.Now(),
			})
		})
	}
	if strings.TrimSpace(opts.SystemPrompt) != "" {
		snap = snap.Branch(func(messages []core.ModelMessage) []core.ModelMessage {
			return replaceSystemPrompt(messages, opts.SystemPrompt)
		})
	}
	if strings.TrimSpace(opts.PlannerPrompt) != "" {
		snap = snap.Branch(func(messages []core.ModelMessage) []core.ModelMessage {
			return appendPlannerPrompt(messages, opts.PlannerPrompt)
		})
	}
	applyForkOverrides(snap, opts)

	parentRunID := snap.RunID
	if parentRunID == "" {
		parentRunID = artifact.Run.ID
	}
	snap.ParentRunID = parentRunID
	snap.RunID = strings.TrimSpace(opts.NewRunID)
	if snap.RunID == "" {
		snap.RunID = fmt.Sprintf("%s.fork.%d", nonEmpty(parentRunID, "run"), time.Now().UnixNano())
	}
	if strings.TrimSpace(opts.Prompt) != "" {
		snap.Prompt = opts.Prompt
	}
	snap.SourceTraceRunID = displayRunID(artifact)
	snap.SourceSnapshotID = record.ID
	snap.Timestamp = time.Now()
	return snap, record, nil
}

func applyForkOverrides(snap *core.RunSnapshot, opts ForkOptions) {
	if snap == nil {
		return
	}
	if snap.ToolState == nil {
		snap.ToolState = make(map[string]any)
	}
	for _, edit := range opts.MemoryEdits {
		key, value, ok := strings.Cut(edit, "=")
		key = strings.TrimSpace(key)
		if !ok || key == "" {
			continue
		}
		snap.ToolState[key] = strings.TrimSpace(value)
	}

	overrides := make(map[string]any)
	if opts.Model != "" {
		overrides["model"] = strings.TrimSpace(opts.Model)
	}
	if opts.Topology != "" {
		overrides["topology"] = strings.TrimSpace(opts.Topology)
	}
	if opts.Middleware != "" {
		overrides["middleware"] = strings.TrimSpace(opts.Middleware)
	}
	if opts.ToolPolicy != "" {
		overrides["tool_policy"] = strings.TrimSpace(opts.ToolPolicy)
	}
	if opts.Evaluator != "" {
		overrides["evaluator"] = strings.TrimSpace(opts.Evaluator)
	}
	if opts.PlannerPrompt != "" {
		overrides["planner_prompt"] = strings.TrimSpace(opts.PlannerPrompt)
	}
	if len(overrides) > 0 {
		snap.ToolState["_gollem_fork_overrides"] = overrides
	}
}

// WriteSnapshotFile writes a RunSnapshot using core's stable snapshot JSON
// format. A path of "-" writes stdout.
func WriteSnapshotFile(path string, snap *core.RunSnapshot) error {
	if strings.TrimSpace(path) == "" {
		return errors.New("snapshot output path is required")
	}
	data, err := core.MarshalSnapshot(snap)
	if err != nil {
		return err
	}
	if path == "-" {
		_, err = os.Stdout.Write(append(data, '\n'))
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	var buf bytes.Buffer
	buf.Write(data)
	buf.WriteByte('\n')
	return os.WriteFile(path, buf.Bytes(), 0o600)
}

func selectSnapshot(records []SnapshotRecord, step int) (SnapshotRecord, error) {
	if len(records) == 0 {
		return SnapshotRecord{}, errors.New("trace artifact has no snapshots; create it with `gollem run --trace-out` after snapshot capture support")
	}
	if step <= 0 {
		return records[len(records)-1], nil
	}
	var selected *SnapshotRecord
	for _, record := range records {
		if record.Step == step {
			return record, nil
		}
		if record.Step <= step && (selected == nil || record.Step > selected.Step) {
			recordCopy := record
			selected = &recordCopy
		}
	}
	if selected != nil {
		return *selected, nil
	}
	steps := make([]string, 0, len(records))
	for _, record := range records {
		steps = append(steps, strconv.Itoa(record.Step))
	}
	return SnapshotRecord{}, fmt.Errorf("no snapshot available at or before step %d (available: %s)", step, strings.Join(steps, ", "))
}

func selectSnapshotByCheckpoint(artifact *Artifact, checkpoint string) (SnapshotRecord, error) {
	checkpoint = strings.TrimSpace(checkpoint)
	if checkpoint == "" {
		return SnapshotRecord{}, errors.New("checkpoint id is required")
	}
	for _, record := range artifact.Snapshots {
		if record.ID == checkpoint {
			return record, nil
		}
	}
	for _, event := range artifact.Events {
		if event.Kind != "checkpoint.created" && event.Kind != "snapshot.created" {
			continue
		}
		if payloadString(event.Payload, "checkpoint_id") != checkpoint && payloadString(event.Payload, "snapshot_id") != checkpoint {
			continue
		}
		return selectSnapshot(artifact.Snapshots, event.Step)
	}
	return SnapshotRecord{}, fmt.Errorf("checkpoint %q not found", checkpoint)
}

func resolveForkStep(artifact *Artifact, opts ForkOptions) (int, error) {
	if strings.TrimSpace(opts.FromEventID) != "" {
		for _, event := range artifact.Events {
			if event.ID == opts.FromEventID {
				return event.Step, nil
			}
		}
		return 0, fmt.Errorf("event %q not found", opts.FromEventID)
	}
	if strings.TrimSpace(opts.FromKind) != "" {
		for i := len(artifact.Events) - 1; i >= 0; i-- {
			event := artifact.Events[i]
			if event.Kind == opts.FromKind {
				return event.Step, nil
			}
		}
		return 0, fmt.Errorf("event kind %q not found", opts.FromKind)
	}
	return opts.FromStep, nil
}

func payloadString(payload map[string]any, key string) string {
	if len(payload) == 0 {
		return ""
	}
	value, ok := payload[key]
	if !ok || value == nil {
		return ""
	}
	return strings.TrimSpace(fmt.Sprint(value))
}

func replaceSystemPrompt(messages []core.ModelMessage, prompt string) []core.ModelMessage {
	prompt = strings.TrimSpace(prompt)
	if prompt == "" {
		return messages
	}
	for msgIdx, msg := range messages {
		req, ok := msg.(core.ModelRequest)
		if !ok {
			continue
		}
		for partIdx, part := range req.Parts {
			if _, ok := part.(core.SystemPromptPart); !ok {
				continue
			}
			req.Parts = append([]core.ModelRequestPart(nil), req.Parts...)
			req.Parts[partIdx] = core.SystemPromptPart{Content: prompt, Timestamp: time.Now()}
			messages[msgIdx] = req
			return messages
		}
	}
	req := core.ModelRequest{
		Parts: []core.ModelRequestPart{
			core.SystemPromptPart{Content: prompt, Timestamp: time.Now()},
		},
		Timestamp: time.Now(),
	}
	return append([]core.ModelMessage{req}, messages...)
}

func appendPlannerPrompt(messages []core.ModelMessage, prompt string) []core.ModelMessage {
	prompt = strings.TrimSpace(prompt)
	if prompt == "" {
		return messages
	}
	req := core.ModelRequest{
		Parts: []core.ModelRequestPart{
			core.SystemPromptPart{Content: "[Planner Prompt]\n" + prompt, Timestamp: time.Now()},
		},
		Timestamp: time.Now(),
	}
	return append([]core.ModelMessage{req}, messages...)
}

func nonEmpty(value, fallback string) string {
	if strings.TrimSpace(value) != "" {
		return value
	}
	return fallback
}
