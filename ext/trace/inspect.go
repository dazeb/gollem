package trace

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/fugue-labs/gollem/core"
)

// InspectOptions configures human-readable trace inspection output.
type InspectOptions struct {
	EventsLimit int
}

// ReplayOptions configures recorded-boundary replay behavior.
type ReplayOptions struct {
	Mode       string
	LiveReexec func(*ReplayState) error `json:"-"`
}

// ReplayState is the deterministic runtime-boundary state reconstructed from a
// trace artifact. It intentionally records boundaries, snapshots, and replayed
// conversation state without re-executing external model or tool calls.
type ReplayState struct {
	RunID              string           `json:"run_id"`
	Mode               string           `json:"mode"`
	Status             string           `json:"status"`
	SourceTraceRunID   string           `json:"source_trace_run_id,omitempty"`
	SourceSnapshotID   string           `json:"source_snapshot_id,omitempty"`
	Boundaries         []ReplayBoundary `json:"boundaries"`
	Messages           []ReplayMessage  `json:"messages,omitempty"`
	RestoredSnapshotID string           `json:"restored_snapshot_id,omitempty"`
	SnapshotCount      int              `json:"snapshot_count"`
}

// ReplayBoundary is one recorded runtime boundary applied during replay.
type ReplayBoundary struct {
	Seq          int            `json:"seq"`
	EventID      string         `json:"event_id"`
	Kind         string         `json:"kind"`
	Step         int            `json:"step,omitempty"`
	RequestID    string         `json:"request_id,omitempty"`
	ReplayPolicy string         `json:"replay_policy,omitempty"`
	Model        string         `json:"model,omitempty"`
	ToolName     string         `json:"tool_name,omitempty"`
	ToolCallID   string         `json:"tool_call_id,omitempty"`
	Text         string         `json:"text,omitempty"`
	Args         string         `json:"args,omitempty"`
	Result       string         `json:"result,omitempty"`
	Error        string         `json:"error,omitempty"`
	CheckpointID string         `json:"checkpoint_id,omitempty"`
	SnapshotID   string         `json:"snapshot_id,omitempty"`
	From         string         `json:"from,omitempty"`
	To           string         `json:"to,omitempty"`
	Score        *float64       `json:"score,omitempty"`
	Passed       *bool          `json:"passed,omitempty"`
	Usage        *core.Usage    `json:"usage,omitempty"`
	Payload      map[string]any `json:"payload,omitempty"`
}

// ReplayMessage is a replayed conversation-state message synthesized from
// recorded model/tool boundaries or restored directly from a snapshot.
type ReplayMessage struct {
	Role       string `json:"role"`
	Kind       string `json:"kind"`
	Step       int    `json:"step,omitempty"`
	Seq        int    `json:"seq,omitempty"`
	Content    string `json:"content,omitempty"`
	ToolName   string `json:"tool_name,omitempty"`
	ToolCallID string `json:"tool_call_id,omitempty"`
}

// SupportedReplayMode reports whether mode is a known replay mode.
func SupportedReplayMode(mode string) bool {
	switch normalizeReplayMode(mode) {
	case "inspect", "strict", "simulated", "fork", "live-reexec":
		return true
	default:
		return false
	}
}

// Inspect writes a compact human-readable trace summary.
func Inspect(w io.Writer, artifact *Artifact, opts InspectOptions) error {
	if artifact == nil {
		return errors.New("nil trace artifact")
	}

	fmt.Fprintf(w, "Trace %s\n", displayRunID(artifact))
	fmt.Fprintf(w, "schema: %s\n", artifact.SchemaVersion)
	fmt.Fprintf(w, "status: %s\n", artifact.Summary.Status)
	if artifact.Summary.Error != "" {
		fmt.Fprintf(w, "error: %s\n", artifact.Summary.Error)
	}
	if !artifact.Run.StartedAt.IsZero() {
		fmt.Fprintf(w, "started: %s\n", artifact.Run.StartedAt.Format(time.RFC3339))
	}
	if artifact.Summary.DurationMillis > 0 {
		fmt.Fprintf(w, "duration: %s\n", time.Duration(artifact.Summary.DurationMillis)*time.Millisecond)
	}
	fmt.Fprintf(w, "steps: %d\n", artifact.Summary.Steps)
	fmt.Fprintf(w, "events: %d\n", len(artifact.Events))
	fmt.Fprintf(w, "snapshots: %d\n", len(artifact.Snapshots))
	fmt.Fprintf(w, "requests: %d\n", artifact.Summary.Requests)
	fmt.Fprintf(w, "tools: %d\n", artifact.Summary.ToolCalls)
	fmt.Fprintf(
		w,
		"tokens: in=%d out=%d cache_read=%d cache_write=%d\n",
		artifact.Summary.Usage.InputTokens,
		artifact.Summary.Usage.OutputTokens,
		artifact.Summary.Usage.CacheReadTokens,
		artifact.Summary.Usage.CacheWriteTokens,
	)
	if artifact.Summary.Cost != nil {
		fmt.Fprintf(w, "cost: %.6f %s\n", artifact.Summary.Cost.TotalCost, nonEmpty(artifact.Summary.Cost.Currency, "USD"))
	}
	if artifact.Run.Prompt != "" {
		fmt.Fprintf(w, "prompt: %s\n", truncateLine(artifact.Run.Prompt, 180))
	}

	limit := opts.EventsLimit
	if limit == 0 || limit > len(artifact.Events) {
		limit = len(artifact.Events)
	}
	if limit > 0 {
		fmt.Fprintln(w)
		fmt.Fprintln(w, "Events:")
		for _, event := range artifact.Events[:limit] {
			fmt.Fprintf(w, "  %03d %-18s %s\n", event.Seq, event.Kind, eventSummary(event))
		}
		if limit < len(artifact.Events) {
			fmt.Fprintf(w, "  ... %d more events\n", len(artifact.Events)-limit)
		}
	}

	return nil
}

// Replay writes a deterministic-boundary replay of recorded events. It does not
// re-execute model or tool calls.
func Replay(w io.Writer, artifact *Artifact) error {
	return ReplayWithOptions(w, artifact, ReplayOptions{Mode: "strict"})
}

// ReplayWithOptions writes a deterministic-boundary replay of recorded events.
// Strict mode validates that recorded external boundaries are paired before
// rendering the timeline.
func ReplayWithOptions(w io.Writer, artifact *Artifact, opts ReplayOptions) error {
	if artifact == nil {
		return errors.New("nil trace artifact")
	}
	mode := normalizeReplayMode(opts.Mode)
	if !SupportedReplayMode(mode) {
		return fmt.Errorf("unsupported replay mode %q", mode)
	}
	if mode != "inspect" {
		if err := ValidateReplay(artifact); err != nil {
			return err
		}
	}
	state, err := BuildReplayState(artifact, ReplayOptions{Mode: mode})
	if err != nil {
		return err
	}
	fmt.Fprintf(w, "Replay %s (%s runtime-boundary replay)\n", displayRunID(artifact), mode)
	switch mode {
	case "inspect":
		fmt.Fprintln(w, "inspect mode: render only; no boundary validation")
	case "strict":
		fmt.Fprintln(w, "strict replay validation: ok")
	case "simulated":
		fmt.Fprintln(w, "simulated mode: recorded external boundaries applied to reconstructed state")
	case "fork":
		fmt.Fprintln(w, "fork mode: source state reconstructed; use `gollem trace fork --continue` to branch and continue live")
	case "live-reexec":
		fmt.Fprintln(w, "live-reexec mode: replay state is ready for a configured live runner")
	}
	fmt.Fprintf(
		w,
		"reconstructed: boundaries=%d messages=%d snapshots=%d restored_snapshot=%s\n",
		len(state.Boundaries),
		len(state.Messages),
		state.SnapshotCount,
		nonEmpty(state.RestoredSnapshotID, "<none>"),
	)
	if mode == "live-reexec" {
		if opts.LiveReexec == nil {
			return errors.New("live-reexec mode requires a configured live runner")
		}
		if err := opts.LiveReexec(state); err != nil {
			return err
		}
		fmt.Fprintln(w, "live re-execution: completed")
	}
	for _, boundary := range state.Boundaries {
		fmt.Fprintf(w, "%03d %-18s %s\n", boundary.Seq, boundary.Kind, replayBoundarySummary(boundary))
	}
	return nil
}

// BuildReplayState reconstructs deterministic runtime-boundary state from a
// trace artifact. It restores the newest snapshot when available, then applies
// recorded model/tool/topology/evaluator/checkpoint events as replay
// boundaries. It never calls models, tools, filesystems, or networks.
func BuildReplayState(artifact *Artifact, opts ReplayOptions) (*ReplayState, error) {
	if artifact == nil {
		return nil, errors.New("nil trace artifact")
	}
	mode := normalizeReplayMode(opts.Mode)
	if !SupportedReplayMode(mode) {
		return nil, fmt.Errorf("unsupported replay mode %q", mode)
	}
	state := &ReplayState{
		RunID:         displayRunID(artifact),
		Mode:          mode,
		Status:        artifact.Summary.Status,
		SnapshotCount: len(artifact.Snapshots),
	}
	if len(artifact.Snapshots) > 0 {
		record := artifact.Snapshots[len(artifact.Snapshots)-1]
		state.RestoredSnapshotID = record.ID
		snap, err := DecodeSnapshotRecord(record)
		if err != nil {
			return nil, fmt.Errorf("restore snapshot %s: %w", record.ID, err)
		}
		state.SourceTraceRunID = snap.SourceTraceRunID
		state.SourceSnapshotID = snap.SourceSnapshotID
		state.Messages = replayMessagesFromSnapshot(snap)
	}
	for _, event := range artifact.Events {
		boundary := replayBoundaryFromEvent(artifact, event)
		if boundary.Kind == "" {
			continue
		}
		state.Boundaries = append(state.Boundaries, boundary)
		state.Messages = applyReplayBoundaryMessage(state.Messages, boundary)
	}
	return state, nil
}

func normalizeReplayMode(mode string) string {
	mode = strings.TrimSpace(mode)
	if mode == "" {
		return "strict"
	}
	return mode
}

// ValidateReplay checks whether a trace has enough recorded boundary structure
// for strict replay/inspection without re-executing model or tool calls.
func ValidateReplay(artifact *Artifact) error {
	if artifact == nil {
		return errors.New("nil trace artifact")
	}
	pendingModels := make(map[string]Event)
	pendingTools := make(map[string]Event)
	pendingApprovals := make(map[string]Event)
	pendingDeferred := make(map[string]Event)

	for _, event := range artifact.Events {
		switch event.Kind {
		case "model.requested":
			key := nonEmpty(event.RequestID, fmt.Sprintf("event-%d", event.Seq))
			pendingModels[key] = event
		case "model.responded", "model.failed":
			key := nonEmpty(event.RequestID, onlyPendingKey(pendingModels))
			delete(pendingModels, key)
		case "tool.called":
			key := boundaryKey(event)
			if key == "" {
				key = fmt.Sprintf("tool-event-%d", event.Seq)
			}
			pendingTools[key] = event
		case "tool.completed", "tool.failed":
			key := boundaryKey(event)
			if key == "" {
				key = onlyPendingKey(pendingTools)
			}
			delete(pendingTools, key)
		case "approval.requested":
			key := boundaryKey(event)
			if key != "" {
				pendingApprovals[key] = event
			}
		case "approval.resolved":
			key := boundaryKey(event)
			if key == "" {
				key = onlyPendingKey(pendingApprovals)
			}
			delete(pendingApprovals, key)
		case "deferred.requested":
			key := boundaryKey(event)
			if key != "" {
				pendingDeferred[key] = event
			}
		case "deferred.resolved":
			key := boundaryKey(event)
			if key == "" {
				key = onlyPendingKey(pendingDeferred)
			}
			delete(pendingDeferred, key)
		}
	}

	switch {
	case len(pendingModels) > 0:
		event := firstPendingEvent(pendingModels)
		return fmt.Errorf("strict replay: model request %s has no recorded response", eventIdentity(event))
	case len(pendingTools) > 0:
		event := firstPendingEvent(pendingTools)
		return fmt.Errorf("strict replay: tool call %s has no recorded result", eventIdentity(event))
	case len(pendingApprovals) > 0 && artifact.Summary.Status != "waiting":
		event := firstPendingEvent(pendingApprovals)
		return fmt.Errorf("strict replay: approval request %s has no recorded resolution", eventIdentity(event))
	case len(pendingDeferred) > 0 && artifact.Summary.Status != "waiting":
		event := firstPendingEvent(pendingDeferred)
		return fmt.Errorf("strict replay: deferred request %s has no recorded resolution", eventIdentity(event))
	default:
		return nil
	}
}

func replayBoundaryFromEvent(artifact *Artifact, event Event) ReplayBoundary {
	boundary := ReplayBoundary{
		Seq:          event.Seq,
		EventID:      event.ID,
		Kind:         event.Kind,
		Step:         event.Step,
		RequestID:    event.RequestID,
		ReplayPolicy: event.ReplayPolicy,
		Payload:      event.Payload,
		Model:        firstPayloadString(event, "model"),
		ToolName:     firstPayloadString(event, "tool_name"),
		ToolCallID:   firstPayloadString(event, "tool_call_id"),
		Args:         firstPayloadString(event, "args"),
		Result:       firstPayloadString(event, "result"),
		Error:        firstPayloadString(event, "error"),
		CheckpointID: firstPayloadString(event, "checkpoint_id"),
		SnapshotID:   firstPayloadString(event, "snapshot_id"),
		From:         firstPayloadString(event, "from"),
		To:           firstPayloadString(event, "to"),
		Score:        payloadFloatPointer(event.Payload, "score"),
		Passed:       payloadBoolPointer(event.Payload, "passed"),
	}
	if boundary.ToolCallID == "" {
		boundary.ToolCallID = boundaryKey(event)
	}
	if boundary.ToolName == "" {
		if value, ok := nestedPayloadValue(event.Payload, "data", "tool_name"); ok {
			boundary.ToolName = fmt.Sprint(value)
		}
	}
	if boundary.Args == "" {
		if value, ok := nestedPayloadValue(event.Payload, "data", "args"); ok {
			boundary.Args = fmt.Sprint(value)
		}
	}
	if boundary.Result == "" {
		if value, ok := nestedPayloadValue(event.Payload, "data", "result"); ok {
			boundary.Result = fmt.Sprint(value)
		}
	}
	if boundary.Text == "" {
		boundary.Text = traceTextForEvent(artifact, event)
	}
	if usage, ok := payloadUsage(event.Payload, "usage"); ok {
		boundary.Usage = &usage
	}
	return boundary
}

func applyReplayBoundaryMessage(messages []ReplayMessage, boundary ReplayBoundary) []ReplayMessage {
	switch boundary.Kind {
	case "model.requested":
		content := ""
		if boundary.Step == 1 {
			content = "recorded model request"
		}
		return append(messages, ReplayMessage{Role: "user", Kind: boundary.Kind, Step: boundary.Step, Seq: boundary.Seq, Content: content})
	case "model.responded":
		return append(messages, ReplayMessage{Role: "assistant", Kind: boundary.Kind, Step: boundary.Step, Seq: boundary.Seq, Content: boundary.Text})
	case "tool.called":
		return append(messages, ReplayMessage{Role: "assistant", Kind: boundary.Kind, Step: boundary.Step, Seq: boundary.Seq, Content: boundary.Args, ToolName: boundary.ToolName, ToolCallID: boundary.ToolCallID})
	case "tool.completed", "tool.failed":
		return append(messages, ReplayMessage{Role: "tool", Kind: boundary.Kind, Step: boundary.Step, Seq: boundary.Seq, Content: nonEmpty(boundary.Result, boundary.Error), ToolName: boundary.ToolName, ToolCallID: boundary.ToolCallID})
	default:
		return messages
	}
}

func replayMessagesFromSnapshot(snap *core.RunSnapshot) []ReplayMessage {
	if snap == nil {
		return nil
	}
	messages := make([]ReplayMessage, 0, len(snap.Messages))
	for idx, msg := range snap.Messages {
		switch m := msg.(type) {
		case core.ModelRequest:
			messages = append(messages, ReplayMessage{Role: "user", Kind: "snapshot.request", Seq: idx + 1, Content: replayRequestContent(m)})
		case core.ModelResponse:
			messages = append(messages, ReplayMessage{Role: "assistant", Kind: "snapshot.response", Seq: idx + 1, Content: m.TextContent()})
		}
	}
	return messages
}

func replayRequestContent(req core.ModelRequest) string {
	var parts []string
	for _, part := range req.Parts {
		switch p := part.(type) {
		case core.SystemPromptPart:
			parts = append(parts, "system: "+p.Content)
		case core.UserPromptPart:
			parts = append(parts, p.Content)
		case core.ToolReturnPart:
			parts = append(parts, fmt.Sprintf("tool %s: %v", p.ToolName, p.Content))
		case core.RetryPromptPart:
			parts = append(parts, "retry: "+p.Content)
		}
	}
	return strings.Join(parts, "\n")
}

func traceTextForEvent(artifact *Artifact, event Event) string {
	if value, ok := event.Payload["text"]; ok {
		return fmt.Sprint(value)
	}
	if value, ok := nestedPayloadValue(event.Payload, "data", "text"); ok {
		return fmt.Sprint(value)
	}
	if artifact == nil || artifact.Trace == nil {
		return ""
	}
	for _, step := range artifact.Trace.Steps {
		if step.Kind != core.TraceModelResponse {
			continue
		}
		if event.Step > 0 && replayTraceStepDataInt(step.Data, "turn_number") > 0 && replayTraceStepDataInt(step.Data, "turn_number") != event.Step {
			continue
		}
		if text := traceStepDataValue(step.Data, "text"); text != "" {
			return text
		}
	}
	return ""
}

func replayBoundarySummary(boundary ReplayBoundary) string {
	parts := make([]string, 0, 8)
	if boundary.Step > 0 {
		parts = append(parts, fmt.Sprintf("step=%d", boundary.Step))
	}
	if boundary.RequestID != "" {
		parts = append(parts, "request="+shortID(boundary.RequestID))
	}
	for _, item := range []struct {
		key   string
		value string
	}{
		{"model", boundary.Model},
		{"tool", boundary.ToolName},
		{"call", shortID(boundary.ToolCallID)},
		{"args", boundary.Args},
		{"result", boundary.Result},
		{"error", boundary.Error},
		{"checkpoint", boundary.CheckpointID},
		{"snapshot", boundary.SnapshotID},
		{"from", boundary.From},
		{"to", boundary.To},
		{"text", boundary.Text},
	} {
		if strings.TrimSpace(item.value) != "" {
			parts = append(parts, fmt.Sprintf("%s=%s", item.key, truncateLine(item.value, 100)))
		}
	}
	if boundary.Score != nil {
		parts = append(parts, fmt.Sprintf("score=%.4f", *boundary.Score))
	}
	if boundary.Passed != nil {
		parts = append(parts, fmt.Sprintf("passed=%v", *boundary.Passed))
	}
	return strings.Join(parts, " ")
}

func replayTraceStepDataInt(data any, key string) int {
	text := traceStepDataValue(data, key)
	if text == "" {
		return 0
	}
	var n int
	if _, err := fmt.Sscanf(text, "%d", &n); err != nil {
		return 0
	}
	return n
}

func payloadFloatPointer(payload map[string]any, key string) *float64 {
	if len(payload) == 0 {
		return nil
	}
	value, ok := payload[key]
	if !ok || value == nil {
		return nil
	}
	switch v := value.(type) {
	case *float64:
		return v
	case float64:
		return &v
	case float32:
		out := float64(v)
		return &out
	case int:
		out := float64(v)
		return &out
	case int64:
		out := float64(v)
		return &out
	case json.Number:
		out, err := v.Float64()
		if err != nil {
			return nil
		}
		return &out
	case string:
		var out float64
		if _, err := fmt.Sscanf(strings.TrimSpace(v), "%f", &out); err != nil {
			return nil
		}
		return &out
	default:
		return nil
	}
}

func payloadBoolPointer(payload map[string]any, key string) *bool {
	if len(payload) == 0 {
		return nil
	}
	value, ok := payload[key]
	if !ok || value == nil {
		return nil
	}
	switch v := value.(type) {
	case *bool:
		return v
	case bool:
		return &v
	case string:
		normalized := strings.ToLower(strings.TrimSpace(v))
		if normalized == "true" || normalized == "1" || normalized == "yes" {
			out := true
			return &out
		}
		if normalized == "false" || normalized == "0" || normalized == "no" {
			out := false
			return &out
		}
	}
	return nil
}

func payloadUsage(payload map[string]any, key string) (core.Usage, bool) {
	if len(payload) == 0 {
		return core.Usage{}, false
	}
	value, ok := payload[key]
	if !ok || value == nil {
		usage := core.Usage{
			InputTokens:      payloadInt(payload, "input_tokens"),
			OutputTokens:     payloadInt(payload, "output_tokens"),
			CacheReadTokens:  payloadInt(payload, "cache_read_tokens"),
			CacheWriteTokens: payloadInt(payload, "cache_write_tokens"),
		}
		return usage, usage.TotalTokens() > 0 || usage.CacheReadTokens > 0 || usage.CacheWriteTokens > 0
	}
	switch v := value.(type) {
	case core.Usage:
		return v, true
	case map[string]any:
		return core.Usage{
			InputTokens:      payloadInt(v, "input_tokens"),
			OutputTokens:     payloadInt(v, "output_tokens"),
			CacheReadTokens:  payloadInt(v, "cache_read_tokens"),
			CacheWriteTokens: payloadInt(v, "cache_write_tokens"),
		}, true
	default:
		data, err := json.Marshal(v)
		if err != nil {
			return core.Usage{}, false
		}
		var usage core.Usage
		if err := json.Unmarshal(data, &usage); err != nil {
			return core.Usage{}, false
		}
		return usage, true
	}
}

func payloadInt(payload map[string]any, key string) int {
	value, ok := payload[key]
	if !ok || value == nil {
		return 0
	}
	switch v := value.(type) {
	case int:
		return v
	case int64:
		return int(v)
	case float64:
		return int(v)
	case json.Number:
		n, _ := v.Int64()
		return int(n)
	case string:
		var n int
		_, _ = fmt.Sscanf(strings.TrimSpace(v), "%d", &n)
		return n
	default:
		return 0
	}
}

func eventSummary(event Event) string {
	parts := make([]string, 0, 6)
	if event.Step > 0 {
		parts = append(parts, fmt.Sprintf("step=%d", event.Step))
	}
	if event.RequestID != "" {
		parts = append(parts, "request="+shortID(event.RequestID))
	}
	if event.DurationMillis > 0 {
		parts = append(parts, fmt.Sprintf("duration=%s", time.Duration(event.DurationMillis)*time.Millisecond))
	}
	for _, key := range []string{"prompt", "model", "finish_reason", "message_count", "function_tool_count", "output_tool_count", "compactions", "success", "error"} {
		if value, ok := event.Payload[key]; ok {
			parts = append(parts, fmt.Sprintf("%s=%v", key, truncateLine(fmt.Sprint(value), 120)))
		}
	}
	for _, key := range []string{"tool_name", "args", "result", "approved", "reason"} {
		if value, ok := event.Payload[key]; ok {
			parts = append(parts, fmt.Sprintf("%s=%v", key, truncateLine(fmt.Sprint(value), 120)))
		}
	}
	if value, ok := nestedPayloadValue(event.Payload, "data", "tool_name"); ok {
		parts = append(parts, fmt.Sprintf("tool=%v", value))
	}
	if value, ok := nestedPayloadValue(event.Payload, "data", "args"); ok {
		parts = append(parts, "args="+truncateLine(fmt.Sprint(value), 80))
	}
	if value, ok := nestedPayloadValue(event.Payload, "data", "error"); ok && fmt.Sprint(value) != "" {
		parts = append(parts, fmt.Sprintf("error=%v", value))
	}
	if len(parts) == 0 && len(event.Payload) > 0 {
		parts = append(parts, truncateLine(stableJSON(event.Payload), 120))
	}
	return strings.Join(parts, " ")
}

func boundaryKey(event Event) string {
	if event.RequestID != "" {
		return event.RequestID
	}
	for _, key := range []string{"tool_call_id", "request_id"} {
		if value, ok := event.Payload[key]; ok && strings.TrimSpace(fmt.Sprint(value)) != "" {
			return strings.TrimSpace(fmt.Sprint(value))
		}
	}
	if value, ok := nestedPayloadValue(event.Payload, "data", "tool_call_id"); ok && strings.TrimSpace(fmt.Sprint(value)) != "" {
		return strings.TrimSpace(fmt.Sprint(value))
	}
	return ""
}

func onlyPendingKey(events map[string]Event) string {
	if len(events) != 1 {
		return ""
	}
	for key := range events {
		return key
	}
	return ""
}

func firstPendingEvent(events map[string]Event) Event {
	var first Event
	for _, event := range events {
		if first.Seq == 0 || event.Seq < first.Seq {
			first = event
		}
	}
	return first
}

func eventIdentity(event Event) string {
	if key := boundaryKey(event); key != "" {
		return key
	}
	if event.RequestID != "" {
		return event.RequestID
	}
	if event.ID != "" {
		return event.ID
	}
	return fmt.Sprintf("event %d", event.Seq)
}

func nestedPayloadValue(payload map[string]any, outer, inner string) (any, bool) {
	value, ok := payload[outer]
	if !ok {
		return nil, false
	}
	switch data := value.(type) {
	case map[string]any:
		got, ok := data[inner]
		return got, ok
	case map[string]string:
		got, ok := data[inner]
		return got, ok
	default:
		return nil, false
	}
}

func stableJSON(v any) string {
	data, err := json.Marshal(v)
	if err != nil {
		return fmt.Sprint(v)
	}
	return string(data)
}

func displayRunID(artifact *Artifact) string {
	if artifact.Run.ID != "" {
		return artifact.Run.ID
	}
	if artifact.Trace != nil && artifact.Trace.RunID != "" {
		return artifact.Trace.RunID
	}
	return "(unknown)"
}

func shortID(id string) string {
	if len(id) <= 24 {
		return id
	}
	return id[:12] + "..." + id[len(id)-8:]
}

func truncateLine(s string, max int) string {
	s = strings.ReplaceAll(s, "\n", "\\n")
	if len(s) <= max {
		return s
	}
	if max <= 3 {
		return s[:max]
	}
	return s[:max-3] + "..."
}
