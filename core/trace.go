package core

import "time"

// RunTrace captures the full execution trace of an agent run.
type RunTrace struct {
	RunID     string         `json:"run_id"`
	Prompt    string         `json:"prompt"`
	StartTime time.Time      `json:"start_time"`
	EndTime   time.Time      `json:"end_time"`
	Duration  time.Duration  `json:"duration"`
	Steps     []TraceStep    `json:"steps"`
	Requests  []RequestTrace `json:"requests,omitempty"`
	Usage     RunUsage       `json:"usage"`
	Success   bool           `json:"success"`
	Error     string         `json:"error,omitempty"`
}

// TraceStep captures a single step in the agent execution.
type TraceStep struct {
	Kind      TraceStepKind `json:"kind"`
	Timestamp time.Time     `json:"timestamp"`
	Duration  time.Duration `json:"duration"`
	Data      any           `json:"data"`
}

// TraceStepKind identifies the type of trace step.
type TraceStepKind string

const (
	TraceModelRequest         TraceStepKind = "model_request"
	TraceModelResponse        TraceStepKind = "model_response"
	TraceModelDelta           TraceStepKind = "model_delta"
	TraceToolCall             TraceStepKind = "tool_call"
	TraceToolResult           TraceStepKind = "tool_result"
	TraceGuardrail            TraceStepKind = "guardrail"
	TraceCheckpointCreated    TraceStepKind = "checkpoint_created"
	TraceApprovalRequested    TraceStepKind = "approval_requested"
	TraceApprovalResolved     TraceStepKind = "approval_resolved"
	TraceDeferredRequested    TraceStepKind = "deferred_requested"
	TraceDeferredResolved     TraceStepKind = "deferred_resolved"
	TraceRunWaiting           TraceStepKind = "run_waiting"
	TraceRunResumed           TraceStepKind = "run_resumed"
	TraceRetryScheduled       TraceStepKind = "retry_scheduled"
	TraceTopologyTransitioned TraceStepKind = "topology_transitioned"
	TraceEvaluatorCompleted   TraceStepKind = "evaluator_completed"
	TraceArtifactChanged      TraceStepKind = "artifact_changed"
	TraceErrorRaised          TraceStepKind = "error_raised"
)

// WithTracing enables execution tracing. The trace is available
// on the RunResult after the run completes.
func WithTracing[T any]() AgentOption[T] {
	return func(a *Agent[T]) {
		a.tracingEnabled = true
	}
}
