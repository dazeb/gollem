package trace

import (
	"errors"
	"fmt"
	"time"

	"github.com/fugue-labs/gollem/core"
	temporalext "github.com/fugue-labs/gollem/ext/temporal"
)

// FromTemporalWorkflowStatus converts a queried Temporal workflow status into
// a canonical trace artifact.
func FromTemporalWorkflowStatus(status *temporalext.WorkflowStatus, metadata map[string]any) (*Artifact, error) {
	if status == nil {
		return nil, errors.New("nil workflow status")
	}

	snapshot, err := temporalext.DecodeWorkflowStatusSnapshot(status)
	if err != nil {
		return nil, fmt.Errorf("decode workflow status snapshot: %w", err)
	}
	runTrace, err := temporalext.DecodeWorkflowStatusTrace(status)
	if err != nil {
		return nil, fmt.Errorf("decode workflow status trace: %w", err)
	}
	if runTrace == nil {
		runTrace = synthesizeTraceFromStatus(status, snapshot)
	}

	var snapshots []*core.RunSnapshot
	if snapshot != nil {
		snapshots = append(snapshots, snapshot)
	}

	merged := cloneMetadata(metadata)
	if merged == nil {
		merged = make(map[string]any)
	}
	merged["temporal_workflow_name"] = status.WorkflowName
	merged["temporal_registration_name"] = status.RegistrationName
	merged["temporal_version"] = status.Version
	if status.TemporalWorkflowID != "" {
		merged["temporal_workflow_id"] = status.TemporalWorkflowID
	}
	if status.TemporalRunID != "" {
		merged["temporal_run_id"] = status.TemporalRunID
	}
	if len(status.TemporalRunChain) > 0 {
		merged["temporal_run_chain"] = status.TemporalRunChain
	}
	merged["temporal_waiting"] = status.Waiting
	merged["temporal_waiting_reason"] = status.WaitingReason
	merged["temporal_completed"] = status.Completed
	merged["temporal_aborted"] = status.Aborted
	merged["temporal_continue_as_new_count"] = status.ContinueAsNewCount
	merged["temporal_current_history_length"] = status.CurrentHistoryLength
	merged["temporal_current_history_size"] = status.CurrentHistorySize
	merged["temporal_continue_as_new_suggested"] = status.ContinueAsNewSuggested
	merged["temporal_last_continue_as_new_reason"] = status.LastContinueAsNewReason
	merged["temporal_pending_approvals"] = len(status.PendingApprovals)
	merged["temporal_deferred_requests"] = len(status.DeferredRequests)
	if status.TraceExport != nil {
		merged["temporal_trace_export_attempted"] = status.TraceExport.Attempted
		merged["temporal_trace_export_total"] = status.TraceExport.Total
		merged["temporal_trace_export_succeeded"] = status.TraceExport.Succeeded
		merged["temporal_trace_export_failed"] = status.TraceExport.Failed
	}

	artifact, err := FromRunTraceWithSnapshotsAndEvents(runTrace, snapshots, temporalStatusEvents(status, snapshot, runTrace), merged)
	if err != nil {
		return nil, err
	}
	artifact.Run.Mode = "temporal"
	artifact.Summary.Status = temporalWorkflowSummaryStatus(status)
	artifact.Summary.Success = artifact.Summary.Status == "succeeded"
	if status.LastError != "" {
		artifact.Summary.Error = status.LastError
	}
	WithCost(artifact, status.Cost)
	return artifact, nil
}

func temporalWorkflowSummaryStatus(status *temporalext.WorkflowStatus) string {
	if status == nil {
		return ""
	}
	switch {
	case status.Waiting:
		return "waiting"
	case status.Completed && status.Aborted:
		return "aborted"
	case status.Completed && status.LastError != "":
		return "failed"
	case status.Completed:
		return "succeeded"
	case status.Aborted:
		return "aborted"
	case status.LastError != "":
		return "failed"
	default:
		return "running"
	}
}

func synthesizeTraceFromStatus(status *temporalext.WorkflowStatus, snapshot *core.RunSnapshot) *core.RunTrace {
	prompt := ""
	started := time.Time{}
	ended := time.Now()
	if snapshot != nil {
		prompt = snapshot.Prompt
		started = snapshot.RunStartTime
		if !snapshot.Timestamp.IsZero() {
			ended = snapshot.Timestamp
		}
	}
	if started.IsZero() {
		started = ended
	}
	success := status.Completed && !status.Aborted && status.LastError == ""
	trace := &core.RunTrace{
		RunID:     status.RunID,
		Prompt:    prompt,
		StartTime: started,
		EndTime:   ended,
		Duration:  ended.Sub(started),
		Usage:     status.Usage,
		Success:   success,
		Error:     status.LastError,
	}
	if trace.Duration < 0 {
		trace.Duration = 0
	}
	return trace
}

func temporalStatusEvents(status *temporalext.WorkflowStatus, snapshot *core.RunSnapshot, runTrace *core.RunTrace) []Event {
	if status == nil {
		return nil
	}
	when := time.Now()
	step := status.RunStep
	if snapshot != nil {
		if !snapshot.Timestamp.IsZero() {
			when = snapshot.Timestamp
		}
		if step == 0 {
			step = snapshot.RunStep
		}
	}

	var events []Event
	for _, approval := range status.PendingApprovals {
		if traceHasRuntimeBoundary(runTrace, core.TraceApprovalRequested, approval.ToolCallID) {
			continue
		}
		events = append(events, Event{
			Kind:      "approval.requested",
			Timestamp: when,
			Step:      step,
			AgentID:   status.RunID,
			RequestID: approval.ToolCallID,
			Payload:   toolBoundaryPayload(status.ParentRunID, approval.ToolCallID, approval.ToolName, approval.ArgsJSON, "", false),
		})
	}
	for _, deferred := range status.DeferredRequests {
		if traceHasRuntimeBoundary(runTrace, core.TraceDeferredRequested, deferred.ToolCallID) {
			continue
		}
		events = append(events, Event{
			Kind:      "deferred.requested",
			Timestamp: when,
			Step:      step,
			AgentID:   status.RunID,
			RequestID: deferred.ToolCallID,
			Payload:   toolBoundaryPayload(status.ParentRunID, deferred.ToolCallID, deferred.ToolName, deferred.ArgsJSON, "", false),
		})
	}
	if status.Waiting {
		if traceHasRuntimeBoundary(runTrace, core.TraceRunWaiting, "") {
			return events
		}
		events = append(events, Event{
			Kind:      "wait.started",
			Timestamp: when,
			Step:      step,
			AgentID:   status.RunID,
			Payload: compactMap(map[string]any{
				"parent_run_id": status.ParentRunID,
				"reason":        status.WaitingReason,
			}),
		})
	}
	return events
}

func traceHasRuntimeBoundary(runTrace *core.RunTrace, kind core.TraceStepKind, toolCallID string) bool {
	if runTrace == nil {
		return false
	}
	for _, step := range runTrace.Steps {
		if step.Kind != kind {
			continue
		}
		if toolCallID == "" || traceStepDataValue(step.Data, "tool_call_id") == toolCallID {
			return true
		}
	}
	return false
}
