package core

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime/debug"
	"sort"
	"strconv"
	"strings"
	"time"
)

const TraceArtifactSchemaVersion = "gollem.trace.v1"

// TraceArtifact is the canonical, portable trace artifact for a Gollem run.
type TraceArtifact struct {
	SchemaVersion string                `json:"schema_version"`
	Run           TraceRunMetadata      `json:"run"`
	Metadata      map[string]any        `json:"metadata,omitempty"`
	Events        []TraceEvent          `json:"events"`
	Snapshots     []TraceSnapshotRecord `json:"snapshots,omitempty"`
	Trace         *RunTrace             `json:"trace,omitempty"`
	Summary       TraceSummary          `json:"summary"`
}

// TraceRunMetadata contains stable run-level metadata used by trace tools
// without requiring consumers to understand the full RunTrace payload.
type TraceRunMetadata struct {
	ID             string    `json:"id"`
	Prompt         string    `json:"prompt,omitempty"`
	StartedAt      time.Time `json:"started_at,omitempty"`
	EndedAt        time.Time `json:"ended_at,omitempty"`
	DurationMillis int64     `json:"duration_ms,omitempty"`
	RuntimeVersion string    `json:"runtime_version,omitempty"`
	Mode           string    `json:"mode,omitempty"`
}

// TraceEvent is a canonical runtime-boundary event.
type TraceEvent struct {
	ID             string                  `json:"id"`
	Seq            int                     `json:"seq"`
	Kind           string                  `json:"kind"`
	Timestamp      time.Time               `json:"timestamp,omitempty"`
	DurationMillis int64                   `json:"duration_ms,omitempty"`
	Step           int                     `json:"step,omitempty"`
	RequestID      string                  `json:"request_id,omitempty"`
	CausalParentID string                  `json:"causal_parent_id,omitempty"`
	AgentID        string                  `json:"agent_id,omitempty"`
	NodeID         string                  `json:"node_id,omitempty"`
	TopologyID     string                  `json:"topology_id,omitempty"`
	ReplayPolicy   string                  `json:"replay_policy,omitempty"`
	Redacted       bool                    `json:"redacted,omitempty"`
	Redaction      *TraceRedactionMetadata `json:"redaction,omitempty"`
	Payload        map[string]any          `json:"payload,omitempty"`
}

// TraceRedactionMetadata describes redaction applied to a trace event.
type TraceRedactionMetadata struct {
	Applied     bool     `json:"applied"`
	Keys        []string `json:"keys,omitempty"`
	Patterns    int      `json:"patterns,omitempty"`
	Replacement string   `json:"replacement,omitempty"`
}

// TraceSnapshotRecord stores one replay/fork anchor captured during a run.
type TraceSnapshotRecord struct {
	ID          string                 `json:"id"`
	Step        int                    `json:"step"`
	RunID       string                 `json:"run_id,omitempty"`
	ParentRunID string                 `json:"parent_run_id,omitempty"`
	Timestamp   time.Time              `json:"timestamp,omitempty"`
	Snapshot    *SerializedRunSnapshot `json:"snapshot"`
}

// TraceSummary contains compact aggregate data for inspect/diff surfaces.
type TraceSummary struct {
	Status         string                 `json:"status"`
	Success        bool                   `json:"success"`
	Error          string                 `json:"error,omitempty"`
	Steps          int                    `json:"steps"`
	Requests       int                    `json:"requests"`
	ToolCalls      int                    `json:"tool_calls"`
	Usage          RunUsage               `json:"usage"`
	Cost           *RunCost               `json:"cost,omitempty"`
	Evaluator      *TraceEvaluatorSummary `json:"evaluator,omitempty"`
	DurationMillis int64                  `json:"duration_ms,omitempty"`
}

// TraceEvaluatorSummary stores aggregate evaluator output when a runtime or
// diff/regression harness attaches evaluation results to a trace.
type TraceEvaluatorSummary struct {
	Name    string         `json:"name,omitempty"`
	Score   *float64       `json:"score,omitempty"`
	Passed  *bool          `json:"passed,omitempty"`
	Results map[string]any `json:"results,omitempty"`
}

// WithTraceArtifactCost attaches a run cost snapshot to an artifact summary.
func WithTraceArtifactCost(artifact *TraceArtifact, cost *RunCost) *TraceArtifact {
	if artifact != nil && cost != nil {
		artifact.Summary.Cost = cost
	}
	return artifact
}

// NewTraceArtifact converts a core trace into the canonical artifact shape
// while preserving the original trace payload.
func NewTraceArtifact(runTrace *RunTrace, metadata map[string]any) (*TraceArtifact, error) {
	return NewTraceArtifactWithSnapshots(runTrace, nil, metadata)
}

// NewTraceArtifactWithSnapshots converts a core trace plus optional run
// snapshots into the canonical artifact shape.
func NewTraceArtifactWithSnapshots(runTrace *RunTrace, snapshots []*RunSnapshot, metadata map[string]any) (*TraceArtifact, error) {
	return NewTraceArtifactWithSnapshotsAndEvents(runTrace, snapshots, nil, metadata)
}

// NewTraceArtifactWithSnapshotsAndEvents converts a core trace plus optional
// snapshots and already-canonical runtime events into the artifact shape.
func NewTraceArtifactWithSnapshotsAndEvents(runTrace *RunTrace, snapshots []*RunSnapshot, runtimeEvents []TraceEvent, metadata map[string]any) (*TraceArtifact, error) {
	if runTrace == nil {
		return nil, errors.New("nil run trace")
	}

	records, err := EncodeTraceSnapshotRecords(snapshots)
	if err != nil {
		return nil, err
	}

	artifact := &TraceArtifact{
		SchemaVersion: TraceArtifactSchemaVersion,
		Run: TraceRunMetadata{
			ID:             runTrace.RunID,
			Prompt:         runTrace.Prompt,
			StartedAt:      runTrace.StartTime,
			EndedAt:        runTrace.EndTime,
			DurationMillis: runTrace.Duration.Milliseconds(),
			RuntimeVersion: traceRuntimeVersion(),
		},
		Metadata:  cloneTraceMetadata(metadata),
		Snapshots: records,
		Trace:     runTrace,
		Summary:   buildTraceSummary(runTrace),
	}
	artifact.Summary.Evaluator = traceEvaluatorSummaryFromMetadata(artifact.Metadata)
	artifact.Events = projectTraceEvents(runTrace)
	artifact.Events = append(artifact.Events, traceSnapshotEvents(records)...)
	artifact.Events = append(artifact.Events, runtimeEvents...)
	artifact.Events = append(artifact.Events, traceMetadataEvents(artifact.Run.ID, artifact.Run.StartedAt, artifact.Run.EndedAt, artifact.Metadata, artifact.Summary.Evaluator)...)
	artifact.Events = NormalizeTraceEvents(artifact.Events)
	if artifact.Summary.Evaluator == nil {
		artifact.Summary.Evaluator = traceEvaluatorSummaryFromEvents(artifact.Run.ID, artifact.Events)
	}
	return artifact, nil
}

// EncodeTraceSnapshotRecords converts run snapshots into JSON-safe artifact records.
func EncodeTraceSnapshotRecords(snapshots []*RunSnapshot) ([]TraceSnapshotRecord, error) {
	if len(snapshots) == 0 {
		return nil, nil
	}
	records := make([]TraceSnapshotRecord, 0, len(snapshots))
	for _, snap := range snapshots {
		if snap == nil {
			continue
		}
		encoded, err := EncodeRunSnapshot(snap)
		if err != nil {
			return nil, fmt.Errorf("encode snapshot step %d: %w", snap.RunStep, err)
		}
		records = append(records, TraceSnapshotRecord{
			ID:          fmt.Sprintf("snap_%06d", len(records)+1),
			Step:        snap.RunStep,
			RunID:       snap.RunID,
			ParentRunID: snap.ParentRunID,
			Timestamp:   snap.Timestamp,
			Snapshot:    encoded,
		})
	}
	return records, nil
}

// DecodeTraceSnapshotRecord decodes a stored snapshot record into a run snapshot.
func DecodeTraceSnapshotRecord(record TraceSnapshotRecord) (*RunSnapshot, error) {
	if record.Snapshot == nil {
		return nil, errors.New("snapshot record has nil snapshot")
	}
	return DecodeRunSnapshot(record.Snapshot)
}

// ReadTraceArtifact decodes a canonical artifact. It also accepts older raw
// RunTrace JSON files as an import compatibility path and immediately converts
// them into the canonical artifact shape.
func ReadTraceArtifact(r io.Reader) (*TraceArtifact, error) {
	data, err := io.ReadAll(r)
	if err != nil {
		return nil, err
	}
	data = bytes.TrimSpace(data)
	if len(data) == 0 {
		return nil, errors.New("empty trace input")
	}

	var probe struct {
		SchemaVersion string `json:"schema_version"`
	}
	if err := json.Unmarshal(data, &probe); err != nil {
		return nil, fmt.Errorf("decode trace JSON: %w", err)
	}
	if probe.SchemaVersion == TraceArtifactSchemaVersion {
		var artifact TraceArtifact
		if err := json.Unmarshal(data, &artifact); err != nil {
			return nil, fmt.Errorf("decode trace artifact: %w", err)
		}
		if artifact.SchemaVersion == "" {
			artifact.SchemaVersion = TraceArtifactSchemaVersion
		}
		if artifact.Trace != nil && len(artifact.Events) == 0 {
			artifact.Events = projectTraceEvents(artifact.Trace)
		}
		if artifact.Trace != nil && artifact.Summary.Status == "" {
			artifact.Summary = buildTraceSummary(artifact.Trace)
		}
		return &artifact, nil
	}

	var keys map[string]json.RawMessage
	if err := json.Unmarshal(data, &keys); err == nil {
		if _, hasMessages := keys["messages"]; hasMessages {
			if _, hasRunStep := keys["run_step"]; hasRunStep {
				return nil, errors.New("input looks like a run snapshot, not a trace artifact")
			}
		}
	}

	var runTrace RunTrace
	if err := json.Unmarshal(data, &runTrace); err != nil {
		return nil, fmt.Errorf("decode legacy run trace: %w", err)
	}
	if runTrace.Prompt == "" && runTrace.StartTime.IsZero() && runTrace.EndTime.IsZero() && len(runTrace.Steps) == 0 && len(runTrace.Requests) == 0 {
		return nil, errors.New("trace JSON is neither a gollem.trace.v1 artifact nor a core.RunTrace")
	}
	return NewTraceArtifact(&runTrace, nil)
}

// ReadTraceArtifactFile reads a trace artifact from path. A path of "-"
// reads stdin.
func ReadTraceArtifactFile(path string) (*TraceArtifact, error) {
	if strings.TrimSpace(path) == "" {
		return nil, errors.New("trace path is required")
	}
	if path == "-" {
		return ReadTraceArtifact(os.Stdin)
	}
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	return ReadTraceArtifact(f)
}

// WriteTraceArtifact writes an artifact as stable, indented JSON.
func WriteTraceArtifact(w io.Writer, artifact *TraceArtifact) error {
	if artifact == nil {
		return errors.New("nil trace artifact")
	}
	data, err := json.MarshalIndent(artifact, "", "  ")
	if err != nil {
		return err
	}
	_, err = w.Write(append(data, '\n'))
	return err
}

// WriteTraceArtifactFile writes an artifact to path. A path of "-" writes stdout.
func WriteTraceArtifactFile(path string, artifact *TraceArtifact) error {
	if strings.TrimSpace(path) == "" {
		return errors.New("trace output path is required")
	}
	if path == "-" {
		return WriteTraceArtifact(os.Stdout, artifact)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	var buf bytes.Buffer
	if err := WriteTraceArtifact(&buf, artifact); err != nil {
		return err
	}
	return os.WriteFile(path, buf.Bytes(), 0o600)
}

func buildTraceSummary(runTrace *RunTrace) TraceSummary {
	status := "failed"
	if runTrace.Success {
		status = "succeeded"
	}
	return TraceSummary{
		Status:         status,
		Success:        runTrace.Success,
		Error:          runTrace.Error,
		Steps:          len(runTrace.Steps),
		Requests:       traceRequestCount(runTrace),
		ToolCalls:      traceToolCallCount(runTrace),
		Usage:          runTrace.Usage,
		Cost:           runTrace.Cost,
		DurationMillis: runTrace.Duration.Milliseconds(),
	}
}

func traceEvaluatorSummaryFromMetadata(metadata map[string]any) *TraceEvaluatorSummary {
	if len(metadata) == 0 {
		return nil
	}
	raw, ok := metadata["evaluator"]
	if !ok || raw == nil {
		return nil
	}
	switch value := raw.(type) {
	case TraceEvaluatorSummary:
		return &value
	case *TraceEvaluatorSummary:
		return value
	case map[string]any:
		summary := &TraceEvaluatorSummary{Results: cloneTraceMetadata(value)}
		if name, ok := value["name"].(string); ok {
			summary.Name = name
		}
		if score, ok := numericPointer(value["score"]); ok {
			summary.Score = score
		}
		if passed, ok := value["passed"].(bool); ok {
			summary.Passed = &passed
		}
		return summary
	default:
		return &TraceEvaluatorSummary{Results: map[string]any{"value": value}}
	}
}

func numericPointer(value any) (*float64, bool) {
	switch v := value.(type) {
	case float64:
		return &v, true
	case float32:
		out := float64(v)
		return &out, true
	case int:
		out := float64(v)
		return &out, true
	case int64:
		out := float64(v)
		return &out, true
	case json.Number:
		out, err := v.Float64()
		if err != nil {
			return nil, false
		}
		return &out, true
	default:
		return nil, false
	}
}

func traceRequestCount(runTrace *RunTrace) int {
	if len(runTrace.Requests) > 0 {
		return len(runTrace.Requests)
	}
	if runTrace.Usage.Requests > 0 {
		return runTrace.Usage.Requests
	}
	count := 0
	for _, step := range runTrace.Steps {
		if step.Kind == TraceModelRequest {
			count++
		}
	}
	return count
}

func traceToolCallCount(runTrace *RunTrace) int {
	if runTrace.Usage.ToolCalls > 0 {
		return runTrace.Usage.ToolCalls
	}
	count := 0
	for _, step := range runTrace.Steps {
		if step.Kind == TraceToolCall {
			count++
		}
	}
	return count
}

func projectTraceEvents(runTrace *RunTrace) []TraceEvent {
	var events []TraceEvent
	if !runTrace.StartTime.IsZero() {
		events = append(events, TraceEvent{
			Kind:      "run.started",
			Timestamp: runTrace.StartTime,
			AgentID:   runTrace.RunID,
			Payload: map[string]any{
				"prompt": runTrace.Prompt,
			},
		})
	}

	if len(runTrace.Requests) > 0 {
		for _, req := range runTrace.Requests {
			events = append(events, traceRequestStartedEvent(runTrace.RunID, req))
			if req.Response != nil {
				events = append(events, traceRequestCompletedEvent(runTrace.RunID, req))
			} else if req.Error != "" {
				events = append(events, traceRequestFailedEvent(runTrace.RunID, req))
			}
		}
	}

	includeModelSteps := len(runTrace.Requests) == 0
	for i, step := range runTrace.Steps {
		if !includeModelSteps && (step.Kind == TraceModelRequest || step.Kind == TraceModelResponse) {
			continue
		}
		events = append(events, traceStepEvent(runTrace.RunID, traceStepNumber(runTrace, step, i+1), step))
	}

	if !runTrace.EndTime.IsZero() {
		kind := "run.failed"
		if runTrace.Success {
			kind = "run.completed"
		}
		payload := map[string]any{
			"success": runTrace.Success,
			"usage":   runTrace.Usage,
		}
		if runTrace.Error != "" {
			payload["error"] = runTrace.Error
		}
		events = append(events, TraceEvent{
			Kind:           kind,
			Timestamp:      runTrace.EndTime,
			DurationMillis: runTrace.Duration.Milliseconds(),
			AgentID:        runTrace.RunID,
			Payload:        payload,
		})
	}

	return NormalizeTraceEvents(events)
}

func traceRequestStartedEvent(runID string, req RequestTrace) TraceEvent {
	return TraceEvent{
		Kind:         "model.requested",
		Timestamp:    req.StartedAt,
		Step:         req.TurnNumber,
		RequestID:    req.RequestID,
		AgentID:      runID,
		ReplayPolicy: "recorded",
		Payload: compactTracePayload(map[string]any{
			"sequence":            req.Sequence,
			"model":               req.ModelName,
			"message_count":       req.MessageCount,
			"function_tool_count": req.FunctionToolCount,
			"output_tool_count":   req.OutputToolCount,
			"compactions":         len(req.Compactions),
		}),
	}
}

func traceRequestCompletedEvent(runID string, req RequestTrace) TraceEvent {
	payload := map[string]any{
		"sequence": req.Sequence,
	}
	if req.Response != nil {
		payload["model"] = req.Response.ModelName
		payload["finish_reason"] = req.Response.FinishReason
		payload["usage"] = req.Response.Usage
	}
	return TraceEvent{
		Kind:           "model.responded",
		Timestamp:      nonZeroTraceTime(req.EndedAt, req.StartedAt),
		DurationMillis: req.Duration.Milliseconds(),
		Step:           req.TurnNumber,
		RequestID:      req.RequestID,
		AgentID:        runID,
		ReplayPolicy:   "recorded",
		Payload:        compactTracePayload(payload),
	}
}

func traceRequestFailedEvent(runID string, req RequestTrace) TraceEvent {
	return TraceEvent{
		Kind:           "model.failed",
		Timestamp:      nonZeroTraceTime(req.EndedAt, req.StartedAt),
		DurationMillis: req.Duration.Milliseconds(),
		Step:           req.TurnNumber,
		RequestID:      req.RequestID,
		AgentID:        runID,
		ReplayPolicy:   "recorded",
		Payload: compactTracePayload(map[string]any{
			"sequence": req.Sequence,
			"model":    req.ModelName,
			"error":    req.Error,
		}),
	}
}

func traceStepEvent(runID string, stepNumber int, step TraceStep) TraceEvent {
	kind := traceStepEventKind(step.Kind)
	if step.Kind == TraceToolResult && traceStepDataString(step.Data, "error") != "" {
		kind = "tool.failed"
	}
	return TraceEvent{
		Kind:           kind,
		Timestamp:      step.Timestamp,
		DurationMillis: step.Duration.Milliseconds(),
		Step:           stepNumber,
		RequestID:      traceStepDataString(step.Data, "tool_call_id"),
		AgentID:        runID,
		ReplayPolicy:   traceReplayPolicyForKind(kind),
		Payload: compactTracePayload(map[string]any{
			"trace_kind": string(step.Kind),
			"data":       step.Data,
		}),
	}
}

func traceStepNumber(runTrace *RunTrace, step TraceStep, fallback int) int {
	if n := traceStepDataInt(step.Data, "turn_number"); n > 0 {
		return n
	}
	if n := traceStepDataInt(step.Data, "run_step"); n > 0 {
		return n
	}
	if runTrace != nil {
		if n := requestTurnForTraceStep(runTrace.Requests, step.Timestamp); n > 0 {
			return n
		}
	}
	return fallback
}

func requestTurnForTraceStep(requests []RequestTrace, at time.Time) int {
	if len(requests) == 0 || at.IsZero() {
		return 0
	}
	var lastTurn int
	for i, req := range requests {
		if req.TurnNumber <= 0 {
			continue
		}
		start := nonZeroTraceTime(req.StartedAt, req.EndedAt)
		if start.IsZero() {
			continue
		}
		if at.Before(start) {
			continue
		}
		nextStart := nextTraceRequestStart(requests, i+1)
		if !nextStart.IsZero() && !at.Before(nextStart) {
			lastTurn = req.TurnNumber
			continue
		}
		return req.TurnNumber
	}
	return lastTurn
}

func nextTraceRequestStart(requests []RequestTrace, from int) time.Time {
	for i := from; i < len(requests); i++ {
		if !requests[i].StartedAt.IsZero() {
			return requests[i].StartedAt
		}
	}
	return time.Time{}
}

func traceStepEventKind(kind TraceStepKind) string {
	switch kind {
	case TraceModelRequest:
		return "model.requested"
	case TraceModelResponse:
		return "model.responded"
	case TraceModelDelta:
		return "model.delta"
	case TraceToolCall:
		return "tool.called"
	case TraceToolResult:
		return "tool.completed"
	case TraceGuardrail:
		return "guardrail.evaluated"
	case TraceCheckpointCreated:
		return "checkpoint.created"
	case TraceApprovalRequested:
		return "approval.requested"
	case TraceApprovalResolved:
		return "approval.resolved"
	case TraceDeferredRequested:
		return "deferred.requested"
	case TraceDeferredResolved:
		return "deferred.resolved"
	case TraceRunWaiting:
		return "wait.started"
	case TraceRunResumed:
		return "wait.resolved"
	case TraceRetryScheduled:
		return "retry.scheduled"
	case TraceTopologyTransitioned:
		return "topology.transitioned"
	case TraceEvaluatorCompleted:
		return "evaluator.completed"
	case TraceArtifactChanged:
		return "artifact.changed"
	case TraceErrorRaised:
		return "error.raised"
	default:
		if kind == "" {
			return "trace.step"
		}
		return "trace." + strings.ReplaceAll(string(kind), "_", ".")
	}
}

func traceSnapshotEvents(records []TraceSnapshotRecord) []TraceEvent {
	if len(records) == 0 {
		return nil
	}
	events := make([]TraceEvent, 0, len(records)*2)
	for _, record := range records {
		events = append(events, TraceEvent{
			Kind:         "checkpoint.created",
			Timestamp:    record.Timestamp,
			Step:         record.Step,
			AgentID:      record.RunID,
			ReplayPolicy: "checkpoint",
			Payload: compactTracePayload(map[string]any{
				"checkpoint_id": record.ID,
				"snapshot_id":   record.ID,
				"run_id":        record.RunID,
				"parent_run_id": record.ParentRunID,
			}),
		})
		events = append(events, TraceEvent{
			Kind:         "snapshot.created",
			Timestamp:    record.Timestamp,
			Step:         record.Step,
			AgentID:      record.RunID,
			ReplayPolicy: "snapshot",
			Payload: compactTracePayload(map[string]any{
				"snapshot_id":   record.ID,
				"run_id":        record.RunID,
				"parent_run_id": record.ParentRunID,
				"message_count": traceSnapshotMessageCount(record),
				"tool_state":    traceSnapshotToolStateKeys(record),
			}),
		})
	}
	return events
}

func traceMetadataEvents(runID string, startedAt, endedAt time.Time, metadata map[string]any, evaluator *TraceEvaluatorSummary) []TraceEvent {
	var events []TraceEvent
	if topology := traceMetadataString(metadata, "topology"); topology != "" {
		events = append(events, TraceEvent{
			Kind:         "topology.transitioned",
			Timestamp:    startedAt,
			AgentID:      runID,
			ReplayPolicy: "recorded",
			Payload: compactTracePayload(map[string]any{
				"from":   traceMetadataString(metadata, "source_topology"),
				"to":     topology,
				"reason": "trace metadata",
			}),
		})
	}
	if evaluator != nil {
		evaluatorAt := endedAt
		if evaluatorAt.IsZero() {
			evaluatorAt = startedAt
		}
		events = append(events, TraceEvent{
			Kind:         "evaluator.completed",
			Timestamp:    evaluatorAt,
			AgentID:      runID,
			ReplayPolicy: "recorded",
			Payload: compactTracePayload(map[string]any{
				"name":    evaluator.Name,
				"score":   evaluator.Score,
				"passed":  evaluator.Passed,
				"results": evaluator.Results,
			}),
		})
	}
	return events
}

func traceMetadataString(metadata map[string]any, key string) string {
	if len(metadata) == 0 {
		return ""
	}
	value, ok := metadata[key]
	if !ok || value == nil {
		return ""
	}
	return strings.TrimSpace(fmt.Sprint(value))
}

func traceEvaluatorSummaryFromEvents(runID string, events []TraceEvent) *TraceEvaluatorSummary {
	for i := len(events) - 1; i >= 0; i-- {
		event := events[i]
		if event.Kind != "evaluator.completed" {
			continue
		}
		if runID != "" && event.AgentID != "" && event.AgentID != runID {
			continue
		}
		return &TraceEvaluatorSummary{
			Name:    tracePayloadString(event.Payload, "name"),
			Score:   tracePayloadFloatPointer(event.Payload, "score"),
			Passed:  tracePayloadBoolPointer(event.Payload, "passed"),
			Results: tracePayloadMap(event.Payload, "results"),
		}
	}
	return nil
}

func tracePayloadString(payload map[string]any, key string) string {
	if len(payload) == 0 {
		return ""
	}
	value, ok := payload[key]
	if !ok || value == nil {
		return ""
	}
	return strings.TrimSpace(fmt.Sprint(value))
}

func tracePayloadFloatPointer(payload map[string]any, key string) *float64 {
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
		if out, err := v.Float64(); err == nil {
			return &out
		}
	case string:
		if out, err := strconv.ParseFloat(strings.TrimSpace(v), 64); err == nil {
			return &out
		}
	}
	return nil
}

func tracePayloadBoolPointer(payload map[string]any, key string) *bool {
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
		if out, err := strconv.ParseBool(strings.TrimSpace(v)); err == nil {
			return &out
		}
	}
	return nil
}

func tracePayloadMap(payload map[string]any, key string) map[string]any {
	if len(payload) == 0 {
		return nil
	}
	switch value := payload[key].(type) {
	case map[string]any:
		return cloneTraceMetadata(value)
	case map[string]string:
		out := make(map[string]any, len(value))
		for k, v := range value {
			out[k] = v
		}
		return out
	default:
		if value == nil {
			return nil
		}
		return map[string]any{"value": value}
	}
}

func traceSnapshotMessageCount(record TraceSnapshotRecord) int {
	if record.Snapshot == nil {
		return 0
	}
	return len(record.Snapshot.Messages)
}

func traceSnapshotToolStateKeys(record TraceSnapshotRecord) []string {
	if record.Snapshot == nil || len(record.Snapshot.ToolState) == 0 {
		return nil
	}
	keys := make([]string, 0, len(record.Snapshot.ToolState))
	for key := range record.Snapshot.ToolState {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

// NormalizeTraceEvents sorts events into a deterministic timeline and assigns
// stable sequence numbers and event IDs.
func NormalizeTraceEvents(events []TraceEvent) []TraceEvent {
	sort.SliceStable(events, func(i, j int) bool {
		left, right := events[i].Timestamp, events[j].Timestamp
		if left.IsZero() && !right.IsZero() {
			return false
		}
		if !left.IsZero() && right.IsZero() {
			return true
		}
		if left.Equal(right) {
			return traceEventKindSortRank(events[i].Kind) < traceEventKindSortRank(events[j].Kind)
		}
		return left.Before(right)
	})
	for i := range events {
		events[i].Seq = i + 1
		events[i].ID = fmt.Sprintf("evt_%06d", i+1)
		if events[i].ReplayPolicy == "" {
			events[i].ReplayPolicy = traceReplayPolicyForKind(events[i].Kind)
		}
	}
	return events
}

func traceEventKindSortRank(kind string) int {
	switch kind {
	case "run.started":
		return 0
	case "checkpoint.created":
		return 5
	case "turn.started":
		return 10
	case "model.requested":
		return 20
	case "model.delta":
		return 25
	case "model.responded", "model.failed":
		return 30
	case "tool.called":
		return 40
	case "approval.requested", "deferred.requested", "wait.started":
		return 50
	case "approval.resolved", "deferred.resolved", "wait.resolved":
		return 60
	case "retry.scheduled":
		return 65
	case "tool.completed", "tool.failed":
		return 70
	case "turn.completed":
		return 80
	case "snapshot.created":
		return 90
	case "topology.transitioned":
		return 92
	case "artifact.changed":
		return 94
	case "evaluator.completed":
		return 96
	case "error.raised":
		return 98
	case "run.completed", "run.failed":
		return 100
	default:
		return 500
	}
}

func traceReplayPolicyForKind(kind string) string {
	switch kind {
	case "model.requested", "model.delta", "model.responded", "model.failed", "tool.called", "tool.completed", "tool.failed", "approval.requested", "approval.resolved", "deferred.requested", "deferred.resolved", "retry.scheduled", "topology.transitioned", "evaluator.completed", "artifact.changed", "error.raised":
		return "recorded"
	case "checkpoint.created":
		return "checkpoint"
	case "snapshot.created":
		return "snapshot"
	default:
		return "inspect"
	}
}

func traceRuntimeVersion() string {
	info, ok := debug.ReadBuildInfo()
	if !ok {
		return "unknown"
	}
	if info.Main.Version != "" && info.Main.Version != "(devel)" {
		return info.Main.Version
	}
	var revision, modified string
	for _, setting := range info.Settings {
		switch setting.Key {
		case "vcs.revision":
			revision = setting.Value
		case "vcs.modified":
			modified = setting.Value
		}
	}
	if revision != "" {
		if len(revision) > 12 {
			revision = revision[:12]
		}
		if modified == "true" {
			return "devel+" + revision + "-modified"
		}
		return "devel+" + revision
	}
	return "devel"
}

func traceStepDataString(data any, key string) string {
	switch value := data.(type) {
	case map[string]any:
		got, ok := value[key]
		if !ok || got == nil {
			return ""
		}
		return strings.TrimSpace(fmt.Sprint(got))
	case map[string]string:
		return strings.TrimSpace(value[key])
	default:
		return ""
	}
}

func traceStepDataInt(data any, key string) int {
	text := traceStepDataString(data, key)
	if text == "" {
		return 0
	}
	n, err := strconv.Atoi(text)
	if err != nil {
		return 0
	}
	return n
}

func nonZeroTraceTime(primary, fallback time.Time) time.Time {
	if !primary.IsZero() {
		return primary
	}
	return fallback
}

func compactTracePayload(src map[string]any) map[string]any {
	out := make(map[string]any, len(src))
	for k, v := range src {
		if isZeroTracePayloadValue(v) {
			continue
		}
		out[k] = v
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func isZeroTracePayloadValue(v any) bool {
	switch t := v.(type) {
	case nil:
		return true
	case string:
		return t == ""
	case int:
		return t == 0
	case int64:
		return t == 0
	default:
		return false
	}
}

func cloneTraceMetadata(src map[string]any) map[string]any {
	if len(src) == 0 {
		return nil
	}
	dst := make(map[string]any, len(src))
	for k, v := range src {
		dst[k] = v
	}
	return dst
}
