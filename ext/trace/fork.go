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
		step, anchor, err := resolveForkAnchor(artifact, opts)
		if err != nil {
			return nil, SnapshotRecord{}, err
		}
		var selectErr error
		record, selectErr = selectSnapshot(artifact.Snapshots, step)
		if selectErr != nil {
			var synthErr error
			var snap *core.RunSnapshot
			snap, record, synthErr = synthesizeForkSnapshot(artifact, step, anchor)
			if synthErr != nil {
				return nil, SnapshotRecord{}, fmt.Errorf("%w; synthetic trace snapshot unavailable: %w", selectErr, synthErr)
			}
			return finishForkSnapshot(artifact, snap, record, opts), record, nil
		}
	}
	snap, err := DecodeSnapshotRecord(record)
	if err != nil {
		return nil, SnapshotRecord{}, err
	}

	return finishForkSnapshot(artifact, snap, record, opts), record, nil
}

func finishForkSnapshot(artifact *Artifact, snap *core.RunSnapshot, record SnapshotRecord, opts ForkOptions) *core.RunSnapshot {
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
	return snap
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
	step, _, err := resolveForkAnchor(artifact, opts)
	return step, err
}

func resolveForkAnchor(artifact *Artifact, opts ForkOptions) (int, *Event, error) {
	if strings.TrimSpace(opts.FromEventID) != "" {
		for _, event := range artifact.Events {
			if event.ID == opts.FromEventID {
				eventCopy := event
				return event.Step, &eventCopy, nil
			}
		}
		return 0, nil, fmt.Errorf("event %q not found", opts.FromEventID)
	}
	if strings.TrimSpace(opts.FromKind) != "" {
		for i := len(artifact.Events) - 1; i >= 0; i-- {
			event := artifact.Events[i]
			if event.Kind == opts.FromKind {
				eventCopy := event
				return event.Step, &eventCopy, nil
			}
		}
		return 0, nil, fmt.Errorf("event kind %q not found", opts.FromKind)
	}
	if opts.FromStep > 0 {
		for i := len(artifact.Events) - 1; i >= 0; i-- {
			event := artifact.Events[i]
			if event.Step <= opts.FromStep {
				eventCopy := event
				return opts.FromStep, &eventCopy, nil
			}
		}
	}
	return opts.FromStep, nil, nil
}

func synthesizeForkSnapshot(artifact *Artifact, step int, anchor *Event) (*core.RunSnapshot, SnapshotRecord, error) {
	if artifact == nil || artifact.Trace == nil || len(artifact.Trace.Requests) == 0 {
		return nil, SnapshotRecord{}, errors.New("embedded request trace is missing")
	}
	messages, timestamp, err := forkMessagesForBoundary(artifact.Trace.Requests, step, anchor)
	if err != nil {
		return nil, SnapshotRecord{}, err
	}
	if step <= 0 {
		step = traceLastRequestStep(artifact.Trace.Requests)
	}
	if timestamp.IsZero() {
		timestamp = artifact.Run.EndedAt
	}
	if timestamp.IsZero() {
		timestamp = time.Now()
	}
	donor, donorRecord, _ := syntheticStateDonor(artifact.Snapshots, step)
	snap := &core.RunSnapshot{
		Messages:         messages,
		Usage:            artifact.Summary.Usage,
		RunID:            displayRunID(artifact),
		ParentRunID:      "",
		RunStep:          step,
		RunStartTime:     artifact.Run.StartedAt,
		Prompt:           artifact.Run.Prompt,
		ToolState:        map[string]any{},
		Timestamp:        timestamp,
		SourceTraceRunID: displayRunID(artifact),
	}
	if donor != nil {
		snap.Usage = donor.Usage
		snap.LastInputTokens = donor.LastInputTokens
		snap.Retries = donor.Retries
		snap.ToolRetries = cloneSyntheticIntMap(donor.ToolRetries)
		snap.ParentRunID = donor.ParentRunID
		if snap.RunStartTime.IsZero() {
			snap.RunStartTime = donor.RunStartTime
		}
		if snap.Prompt == "" {
			snap.Prompt = donor.Prompt
		}
		snap.ToolState = cloneSyntheticAnyMap(donor.ToolState)
		if snap.ToolState == nil {
			snap.ToolState = map[string]any{}
		}
		snap.ToolState["_gollem_synthetic_state_source"] = map[string]any{
			"snapshot_id":    donorRecord.ID,
			"snapshot_step":  donorRecord.Step,
			"requested_step": step,
		}
	}
	recordID := fmt.Sprintf("synthetic_step_%06d", step)
	encoded, err := core.EncodeRunSnapshot(snap)
	if err != nil {
		return nil, SnapshotRecord{}, err
	}
	return snap, SnapshotRecord{
		ID:        recordID,
		Step:      step,
		RunID:     snap.RunID,
		Timestamp: timestamp,
		Snapshot:  encoded,
	}, nil
}

func syntheticStateDonor(records []SnapshotRecord, step int) (*core.RunSnapshot, SnapshotRecord, error) {
	if len(records) == 0 {
		return nil, SnapshotRecord{}, errors.New("no stored snapshots")
	}
	var selected *SnapshotRecord
	for _, record := range records {
		if step > 0 && record.Step < step {
			continue
		}
		if selected == nil || record.Step < selected.Step {
			recordCopy := record
			selected = &recordCopy
		}
	}
	if selected == nil {
		record := records[len(records)-1]
		selected = &record
	}
	snap, err := DecodeSnapshotRecord(*selected)
	if err != nil {
		return nil, SnapshotRecord{}, err
	}
	return snap, *selected, nil
}

func forkMessagesForBoundary(requests []core.RequestTrace, step int, anchor *Event) ([]core.ModelMessage, time.Time, error) {
	if len(requests) == 0 {
		return nil, time.Time{}, errors.New("no request traces")
	}
	reqIdx := requestTraceIndexForBoundary(requests, step, anchor)
	if reqIdx < 0 {
		return nil, time.Time{}, fmt.Errorf("no request trace for step %d", step)
	}
	req := requests[reqIdx]
	if len(req.Messages) == 0 {
		return nil, time.Time{}, fmt.Errorf("request trace %s has no serialized messages", req.RequestID)
	}
	if anchor == nil && step <= 0 {
		return messagesAfterRequest(requests, reqIdx)
	}
	if anchor != nil {
		switch anchor.Kind {
		case "model.requested":
			messages, err := core.DecodeMessages(req.Messages)
			return messages, nonZeroTime(req.StartedAt, anchor.Timestamp), err
		case "model.responded", "model.failed":
			return messagesWithResponse(req)
		case "tool.called", "tool.completed", "tool.failed", "approval.requested", "approval.resolved", "deferred.requested", "deferred.resolved", "wait.started", "wait.resolved":
			if reqIdx+1 < len(requests) {
				next := requests[reqIdx+1]
				messages, err := core.DecodeMessages(next.Messages)
				return messages, nonZeroTime(next.StartedAt, anchor.Timestamp), err
			}
			return messagesWithResponse(req)
		}
	}
	return messagesAfterRequest(requests, reqIdx)
}

func messagesAfterRequest(requests []core.RequestTrace, idx int) ([]core.ModelMessage, time.Time, error) {
	if idx+1 < len(requests) {
		next := requests[idx+1]
		messages, err := core.DecodeMessages(next.Messages)
		return messages, next.StartedAt, err
	}
	return messagesWithResponse(requests[idx])
}

func messagesWithResponse(req core.RequestTrace) ([]core.ModelMessage, time.Time, error) {
	messages, err := core.DecodeMessages(req.Messages)
	if err != nil {
		return nil, time.Time{}, err
	}
	if req.Response != nil && req.Response.Message != nil {
		resp, err := core.DecodeModelResponse(req.Response.Message)
		if err != nil {
			return nil, time.Time{}, err
		}
		messages = append(messages, *resp)
	}
	return messages, nonZeroTime(req.EndedAt, req.StartedAt), nil
}

func requestTraceIndexForBoundary(requests []core.RequestTrace, step int, anchor *Event) int {
	if anchor != nil && anchor.RequestID != "" {
		for i, req := range requests {
			if req.RequestID == anchor.RequestID {
				return i
			}
		}
	}
	if step <= 0 {
		return len(requests) - 1
	}
	selected := -1
	for i, req := range requests {
		if req.TurnNumber == step {
			return i
		}
		if req.TurnNumber < step {
			selected = i
		}
	}
	return selected
}

func traceLastRequestStep(requests []core.RequestTrace) int {
	for i := len(requests) - 1; i >= 0; i-- {
		if requests[i].TurnNumber > 0 {
			return requests[i].TurnNumber
		}
	}
	return len(requests)
}

func cloneSyntheticAnyMap(src map[string]any) map[string]any {
	if len(src) == 0 {
		return nil
	}
	dst := make(map[string]any, len(src))
	for key, value := range src {
		dst[key] = value
	}
	return dst
}

func cloneSyntheticIntMap(src map[string]int) map[string]int {
	if len(src) == 0 {
		return nil
	}
	dst := make(map[string]int, len(src))
	for key, value := range src {
		dst[key] = value
	}
	return dst
}

func nonZeroTime(values ...time.Time) time.Time {
	for _, value := range values {
		if !value.IsZero() {
			return value
		}
	}
	return time.Time{}
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
