package trace

import (
	"strconv"
	"sync"
	"time"

	"github.com/fugue-labs/gollem/core"
)

// RuntimeRecorder captures runtime lifecycle events that are not fully
// represented in core.RunTrace steps, such as approvals, waits, and resumes.
type RuntimeRecorder struct {
	mu     sync.Mutex
	events []Event
	unsubs []func()
	sinks  []func(Event)
}

// NewRuntimeRecorder subscribes to a run event bus and records canonical trace
// events until Close is called or the bus is discarded.
func NewRuntimeRecorder(bus *core.EventBus) *RuntimeRecorder {
	rec := &RuntimeRecorder{}
	if bus == nil {
		return rec
	}
	rec.unsubs = append(rec.unsubs,
		core.Subscribe(bus, rec.recordRunStarted),
		core.Subscribe(bus, rec.recordRunCompleted),
		core.Subscribe(bus, rec.recordTurnStarted),
		core.Subscribe(bus, rec.recordTurnCompleted),
		core.Subscribe(bus, rec.recordModelRequestStarted),
		core.Subscribe(bus, rec.recordModelResponseCompleted),
		core.Subscribe(bus, rec.recordModelDelta),
		core.Subscribe(bus, rec.recordToolCalled),
		core.Subscribe(bus, rec.recordToolCompleted),
		core.Subscribe(bus, rec.recordToolFailed),
		core.Subscribe(bus, rec.recordApprovalRequested),
		core.Subscribe(bus, rec.recordApprovalResolved),
		core.Subscribe(bus, rec.recordDeferredRequested),
		core.Subscribe(bus, rec.recordDeferredResolved),
		core.Subscribe(bus, rec.recordRunWaiting),
		core.Subscribe(bus, rec.recordRunResumed),
		core.Subscribe(bus, rec.recordArtifactChanged),
		core.Subscribe(bus, rec.recordRetryScheduled),
		core.Subscribe(bus, rec.recordCheckpointCreated),
		core.Subscribe(bus, rec.recordTopologyTransitioned),
		core.Subscribe(bus, rec.recordEvaluatorCompleted),
		core.Subscribe(bus, rec.recordErrorRaised),
	)
	return rec
}

// Close unsubscribes the recorder from its event bus.
func (r *RuntimeRecorder) Close() {
	if r == nil {
		return
	}
	r.mu.Lock()
	unsubs := append([]func(){}, r.unsubs...)
	r.unsubs = nil
	r.sinks = nil
	r.mu.Unlock()
	for _, unsub := range unsubs {
		if unsub != nil {
			unsub()
		}
	}
}

// Events returns a stable copy of all recorded events.
func (r *RuntimeRecorder) Events() []Event {
	if r == nil {
		return nil
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]Event(nil), r.events...)
}

// OnEvent registers a live event sink. The sink is called after the event has
// been appended to the recorder, and receives canonical runtime events before
// final artifact normalization.
func (r *RuntimeRecorder) OnEvent(fn func(Event)) func() {
	if r == nil || fn == nil {
		return func() {}
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.sinks = append(r.sinks, fn)
	idx := len(r.sinks) - 1
	return func() {
		r.mu.Lock()
		defer r.mu.Unlock()
		if idx >= 0 && idx < len(r.sinks) {
			r.sinks[idx] = nil
		}
	}
}

// EventsForTrace returns recorded runtime events with events already projected
// from the root RunTrace removed for the root run. Nested child-agent events are
// retained, which makes delegate/team/orchestrator work replayable in one
// canonical timeline without duplicating the top-level model/tool boundaries.
func (r *RuntimeRecorder) EventsForTrace(rootRunID string) []Event {
	events := r.Events()
	if rootRunID == "" {
		return events
	}
	lineage := traceLineage(rootRunID, events)
	filtered := events[:0]
	for _, event := range events {
		if !eventInLineage(event, lineage) {
			continue
		}
		if event.AgentID == rootRunID && isProjectedRunTraceKind(event.Kind) {
			continue
		}
		filtered = append(filtered, event)
	}
	return append([]Event(nil), filtered...)
}

func traceLineage(rootRunID string, events []Event) map[string]bool {
	lineage := map[string]bool{rootRunID: true}
	changed := true
	for changed {
		changed = false
		for _, event := range events {
			if event.AgentID == "" || lineage[event.AgentID] {
				continue
			}
			parent := event.CausalParentID
			if parent == "" {
				parent = firstPayloadString(event, "parent_run_id")
			}
			if lineage[parent] {
				lineage[event.AgentID] = true
				changed = true
			}
		}
	}
	return lineage
}

func eventInLineage(event Event, lineage map[string]bool) bool {
	if event.AgentID != "" {
		return lineage[event.AgentID]
	}
	if event.CausalParentID != "" && lineage[event.CausalParentID] {
		return true
	}
	if parent := firstPayloadString(event, "parent_run_id"); parent != "" {
		return lineage[parent]
	}
	return false
}

func (r *RuntimeRecorder) append(event Event) {
	r.mu.Lock()
	r.events = append(r.events, event)
	sinks := append([]func(Event){}, r.sinks...)
	r.mu.Unlock()
	for _, sink := range sinks {
		if sink != nil {
			sink(event)
		}
	}
}

func (r *RuntimeRecorder) recordRunStarted(ev core.RunStartedEvent) {
	r.append(Event{
		Kind:           "run.started",
		Timestamp:      ev.StartedAt,
		AgentID:        ev.RunID,
		CausalParentID: ev.ParentRunID,
		Payload: compactMap(map[string]any{
			"prompt":        ev.Prompt,
			"parent_run_id": ev.ParentRunID,
		}),
	})
}

func (r *RuntimeRecorder) recordRunCompleted(ev core.RunCompletedEvent) {
	kind := "run.completed"
	if !ev.Success || ev.Error != "" {
		kind = "run.failed"
	}
	r.append(Event{
		Kind:           kind,
		Timestamp:      ev.CompletedAt,
		DurationMillis: durationMillis(ev.StartedAt, ev.CompletedAt),
		AgentID:        ev.RunID,
		CausalParentID: ev.ParentRunID,
		Payload: compactMap(map[string]any{
			"parent_run_id": ev.ParentRunID,
			"success":       ev.Success,
			"error":         ev.Error,
			"deferred":      ev.Deferred,
		}),
	})
}

func (r *RuntimeRecorder) recordTurnStarted(ev core.TurnStartedEvent) {
	r.append(Event{
		Kind:           "turn.started",
		Timestamp:      ev.StartedAt,
		Step:           ev.TurnNumber,
		AgentID:        ev.RunID,
		CausalParentID: ev.ParentRunID,
		Payload:        compactMap(map[string]any{"parent_run_id": ev.ParentRunID}),
	})
}

func (r *RuntimeRecorder) recordTurnCompleted(ev core.TurnCompletedEvent) {
	kind := "turn.completed"
	if ev.Error != "" {
		kind = "turn.failed"
	}
	r.append(Event{
		Kind:           kind,
		Timestamp:      ev.CompletedAt,
		Step:           ev.TurnNumber,
		AgentID:        ev.RunID,
		CausalParentID: ev.ParentRunID,
		Payload: compactMap(map[string]any{
			"parent_run_id":  ev.ParentRunID,
			"has_tool_calls": ev.HasToolCalls,
			"has_text":       ev.HasText,
			"error":          ev.Error,
		}),
	})
}

func (r *RuntimeRecorder) recordModelRequestStarted(ev core.ModelRequestStartedEvent) {
	requestID := modelBoundaryRequestID(ev.RunID, ev.TurnNumber)
	r.append(Event{
		Kind:           "model.requested",
		Timestamp:      ev.StartedAt,
		Step:           ev.TurnNumber,
		RequestID:      requestID,
		AgentID:        ev.RunID,
		CausalParentID: ev.ParentRunID,
		Payload: compactMap(map[string]any{
			"parent_run_id": ev.ParentRunID,
			"message_count": ev.MessageCount,
		}),
	})
}

func (r *RuntimeRecorder) recordModelResponseCompleted(ev core.ModelResponseCompletedEvent) {
	requestID := modelBoundaryRequestID(ev.RunID, ev.TurnNumber)
	r.append(Event{
		Kind:           "model.responded",
		Timestamp:      ev.CompletedAt,
		DurationMillis: ev.DurationMs,
		Step:           ev.TurnNumber,
		RequestID:      requestID,
		AgentID:        ev.RunID,
		CausalParentID: ev.ParentRunID,
		Payload: compactMap(map[string]any{
			"parent_run_id":  ev.ParentRunID,
			"finish_reason":  ev.FinishReason,
			"input_tokens":   ev.InputTokens,
			"output_tokens":  ev.OutputTokens,
			"has_tool_calls": ev.HasToolCalls,
			"has_text":       ev.HasText,
		}),
	})
}

func (r *RuntimeRecorder) recordModelDelta(ev core.ModelDeltaEvent) {
	requestID := modelBoundaryRequestID(ev.RunID, ev.TurnNumber)
	r.append(Event{
		Kind:           "model.delta",
		Timestamp:      ev.DeltaAt,
		Step:           ev.TurnNumber,
		RequestID:      requestID,
		AgentID:        ev.RunID,
		CausalParentID: ev.ParentRunID,
		Payload: compactMap(map[string]any{
			"parent_run_id": ev.ParentRunID,
			"part_index":    ev.PartIndex,
			"delta_kind":    ev.DeltaKind,
			"content_delta": ev.ContentDelta,
		}),
	})
}

func (r *RuntimeRecorder) recordToolCalled(ev core.ToolCalledEvent) {
	r.append(Event{
		Kind:           "tool.called",
		Timestamp:      ev.CalledAt,
		AgentID:        ev.RunID,
		CausalParentID: ev.ParentRunID,
		RequestID:      ev.ToolCallID,
		Payload:        toolBoundaryPayload(ev.ParentRunID, ev.ToolCallID, ev.ToolName, ev.ArgsJSON, "", false),
	})
}

func (r *RuntimeRecorder) recordToolCompleted(ev core.ToolCompletedEvent) {
	r.append(Event{
		Kind:           "tool.completed",
		Timestamp:      ev.CompletedAt,
		DurationMillis: ev.DurationMs,
		AgentID:        ev.RunID,
		CausalParentID: ev.ParentRunID,
		RequestID:      ev.ToolCallID,
		Payload:        toolBoundaryPayload(ev.ParentRunID, ev.ToolCallID, ev.ToolName, "", ev.Result, false),
	})
}

func (r *RuntimeRecorder) recordToolFailed(ev core.ToolFailedEvent) {
	r.append(Event{
		Kind:           "tool.failed",
		Timestamp:      ev.FailedAt,
		DurationMillis: ev.DurationMs,
		AgentID:        ev.RunID,
		CausalParentID: ev.ParentRunID,
		RequestID:      ev.ToolCallID,
		Payload: compactMap(map[string]any{
			"parent_run_id": ev.ParentRunID,
			"tool_call_id":  ev.ToolCallID,
			"tool_name":     ev.ToolName,
			"error":         ev.Error,
		}),
	})
}

func (r *RuntimeRecorder) recordApprovalRequested(ev core.ApprovalRequestedEvent) {
	r.append(Event{
		Kind:           "approval.requested",
		Timestamp:      ev.RequestedAt,
		AgentID:        ev.RunID,
		CausalParentID: ev.ParentRunID,
		RequestID:      ev.ToolCallID,
		Payload:        toolBoundaryPayload(ev.ParentRunID, ev.ToolCallID, ev.ToolName, ev.ArgsJSON, "", false),
	})
}

func (r *RuntimeRecorder) recordApprovalResolved(ev core.ApprovalResolvedEvent) {
	r.append(Event{
		Kind:           "approval.resolved",
		Timestamp:      ev.ResolvedAt,
		AgentID:        ev.RunID,
		CausalParentID: ev.ParentRunID,
		RequestID:      ev.ToolCallID,
		Payload: compactMap(map[string]any{
			"parent_run_id": ev.ParentRunID,
			"tool_call_id":  ev.ToolCallID,
			"tool_name":     ev.ToolName,
			"approved":      ev.Approved,
		}),
	})
}

func (r *RuntimeRecorder) recordDeferredRequested(ev core.DeferredRequestedEvent) {
	r.append(Event{
		Kind:           "deferred.requested",
		Timestamp:      ev.RequestedAt,
		AgentID:        ev.RunID,
		CausalParentID: ev.ParentRunID,
		RequestID:      ev.ToolCallID,
		Payload:        toolBoundaryPayload(ev.ParentRunID, ev.ToolCallID, ev.ToolName, ev.ArgsJSON, "", false),
	})
}

func (r *RuntimeRecorder) recordDeferredResolved(ev core.DeferredResolvedEvent) {
	r.append(Event{
		Kind:           "deferred.resolved",
		Timestamp:      ev.ResolvedAt,
		AgentID:        ev.RunID,
		CausalParentID: ev.ParentRunID,
		RequestID:      ev.ToolCallID,
		Payload: toolBoundaryPayload(
			ev.ParentRunID,
			ev.ToolCallID,
			ev.ToolName,
			"",
			ev.Content,
			ev.IsError,
		),
	})
}

func (r *RuntimeRecorder) recordRunWaiting(ev core.RunWaitingEvent) {
	r.append(Event{
		Kind:           "wait.started",
		Timestamp:      ev.WaitingAt,
		AgentID:        ev.RunID,
		CausalParentID: ev.ParentRunID,
		Payload: compactMap(map[string]any{
			"parent_run_id": ev.ParentRunID,
			"reason":        ev.Reason,
		}),
	})
}

func (r *RuntimeRecorder) recordRunResumed(ev core.RunResumedEvent) {
	r.append(Event{
		Kind:           "wait.resolved",
		Timestamp:      ev.ResumedAt,
		AgentID:        ev.RunID,
		CausalParentID: ev.ParentRunID,
		Payload: compactMap(map[string]any{
			"parent_run_id": ev.ParentRunID,
		}),
	})
}

func (r *RuntimeRecorder) recordArtifactChanged(ev core.ArtifactChangedEvent) {
	r.append(Event{
		Kind:           "artifact.changed",
		Timestamp:      ev.ChangedAt,
		AgentID:        ev.RunID,
		CausalParentID: ev.ParentRunID,
		RequestID:      ev.ToolCallID,
		Payload: compactMap(map[string]any{
			"parent_run_id": ev.ParentRunID,
			"tool_call_id":  ev.ToolCallID,
			"tool_name":     ev.ToolName,
			"path":          ev.Path,
			"operation":     ev.Operation,
			"bytes":         ev.Bytes,
		}),
	})
}

func (r *RuntimeRecorder) recordRetryScheduled(ev core.RetryScheduledEvent) {
	r.append(Event{
		Kind:           "retry.scheduled",
		Timestamp:      ev.ScheduledAt,
		Step:           ev.TurnNumber,
		AgentID:        ev.RunID,
		CausalParentID: ev.ParentRunID,
		RequestID:      ev.ToolCallID,
		Payload: compactMap(map[string]any{
			"parent_run_id": ev.ParentRunID,
			"tool_name":     ev.ToolName,
			"tool_call_id":  ev.ToolCallID,
			"reason":        ev.Reason,
			"retry":         ev.Retry,
			"max_retries":   ev.MaxRetries,
		}),
	})
}

func (r *RuntimeRecorder) recordCheckpointCreated(ev core.CheckpointCreatedEvent) {
	r.append(Event{
		Kind:           "checkpoint.created",
		Timestamp:      ev.CreatedAt,
		Step:           ev.Step,
		AgentID:        ev.RunID,
		CausalParentID: ev.ParentRunID,
		Payload: compactMap(map[string]any{
			"parent_run_id": ev.ParentRunID,
			"checkpoint_id": ev.CheckpointID,
			"snapshot_id":   ev.SnapshotID,
		}),
	})
}

func (r *RuntimeRecorder) recordTopologyTransitioned(ev core.TopologyTransitionedEvent) {
	r.append(Event{
		Kind:           "topology.transitioned",
		Timestamp:      ev.TransitionedAt,
		AgentID:        ev.RunID,
		CausalParentID: ev.ParentRunID,
		Payload: compactMap(map[string]any{
			"parent_run_id": ev.ParentRunID,
			"from":          ev.From,
			"to":            ev.To,
			"reason":        ev.Reason,
		}),
	})
}

func (r *RuntimeRecorder) recordEvaluatorCompleted(ev core.EvaluatorCompletedEvent) {
	r.append(Event{
		Kind:           "evaluator.completed",
		Timestamp:      ev.CompletedAt,
		AgentID:        ev.RunID,
		CausalParentID: ev.ParentRunID,
		Payload: compactMap(map[string]any{
			"parent_run_id": ev.ParentRunID,
			"name":          ev.Name,
			"score":         ev.Score,
			"passed":        ev.Passed,
			"results":       ev.Results,
		}),
	})
}

func (r *RuntimeRecorder) recordErrorRaised(ev core.ErrorRaisedEvent) {
	r.append(Event{
		Kind:           "error.raised",
		Timestamp:      ev.RaisedAt,
		Step:           ev.TurnNumber,
		AgentID:        ev.RunID,
		CausalParentID: ev.ParentRunID,
		RequestID:      ev.ToolCallID,
		Payload: compactMap(map[string]any{
			"parent_run_id": ev.ParentRunID,
			"tool_name":     ev.ToolName,
			"tool_call_id":  ev.ToolCallID,
			"error":         ev.Error,
		}),
	})
}

func modelBoundaryRequestID(runID string, turn int) string {
	if runID == "" || turn <= 0 {
		return ""
	}
	return runID + "/turn-" + strconv.Itoa(turn)
}

func isProjectedRunTraceKind(kind string) bool {
	switch kind {
	case "run.started", "run.completed", "run.failed", "model.requested", "model.delta", "model.responded", "model.failed", "tool.called", "tool.completed", "tool.failed", "checkpoint.created", "retry.scheduled", "error.raised":
		return true
	default:
		return false
	}
}

func durationMillis(start, end time.Time) int64 {
	if start.IsZero() || end.IsZero() || end.Before(start) {
		return 0
	}
	return end.Sub(start).Milliseconds()
}

func toolBoundaryPayload(parentRunID, toolCallID, toolName, argsJSON, result string, isError bool) map[string]any {
	return compactMap(map[string]any{
		"parent_run_id": parentRunID,
		"tool_call_id":  toolCallID,
		"tool_name":     toolName,
		"args":          argsJSON,
		"result":        result,
		"is_error":      isError,
	})
}
