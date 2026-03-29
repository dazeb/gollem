// Package agui provides a normalized event stream and session model for
// building agent UIs on top of gollem. It translates gollem's internal
// lifecycle signals (hooks, runtime events, stream events) into a single
// replayable event contract with stable session identity.
//
// See DESIGN.md in this package for the full integration specification.
package agui

import (
	"encoding/json"
	"sync/atomic"
	"time"
)

// Event type constants for the AGUI normalized event stream.
const (
	// Session lifecycle
	EventSessionOpened        = "session.opened"
	EventSessionInputAccepted = "session.input.accepted"
	EventSessionCompleted     = "session.completed"
	EventSessionFailed        = "session.failed"
	EventSessionCancelled     = "session.cancelled"
	EventSessionAborted       = "session.aborted"
	EventSessionWaiting       = "session.waiting"
	EventSessionResumed       = "session.resumed"
	EventSessionSnapshot      = "session.snapshot"

	// Run lifecycle
	EventRunStarted   = "run.started"
	EventRunCompleted = "run.completed"

	// Turn lifecycle
	EventTurnStarted   = "turn.started"
	EventTurnCompleted = "turn.completed"

	// Model request/response
	EventModelRequestStarted    = "model.request.started"
	EventModelResponseCompleted = "model.response.completed"

	// Model output streaming
	EventModelOutputTextDelta       = "model.output.text.delta"
	EventModelOutputTextStarted     = "model.output.text.started"
	EventModelOutputTextCompleted   = "model.output.text.completed"
	EventModelOutputThinkingDelta   = "model.output.thinking.delta"
	EventModelOutputToolCallDelta   = "model.output.tool_call.delta"
	EventModelOutputToolCallStarted = "model.output.tool_call.started"

	// Tool lifecycle
	EventToolCallRequested      = "tool.call.requested"
	EventToolExecutionStarted   = "tool.execution.started"
	EventToolExecutionCompleted = "tool.execution.completed"
	EventToolExecutionFailed    = "tool.execution.failed"
	EventToolDeferred           = "tool.deferred"

	// Approval
	EventApprovalRequested = "approval.requested"
	EventApprovalApproved  = "approval.approved"
	EventApprovalDenied    = "approval.denied"

	// External input (deferred tools)
	EventExternalInputRequested = "external_input.requested"
	EventExternalInputProvided  = "external_input.provided"

	// Graph topology (P1)
	EventGraphNodeStarted   = "graph.node.started"
	EventGraphNodeCompleted = "graph.node.completed"
	EventGraphFanoutStarted = "graph.fanout.started"
	EventGraphFanoutJoined  = "graph.fanout.joined"

	// Team topology (P1)
	EventTeamTeammateSpawned     = "team.teammate.spawned"
	EventTeamTeammateIdle        = "team.teammate.idle"
	EventTeamTeammateTerminated  = "team.teammate.terminated"
	EventTeamTeammateOutputDelta = "team.teammate.output.delta"
)

// Event is the normalized AGUI event envelope. Every event emitted through
// the AGUI adapter uses this structure, regardless of the underlying gollem
// backend (core Run, RunStream, Iter, Temporal, graph, or team).
type Event struct {
	// ID is a unique event identifier for deduplication.
	ID string `json:"id"`

	// Sequence is a monotonically increasing counter within a session.
	// Used for reconnect replay: clients send last_seq to resume.
	Sequence uint64 `json:"sequence"`

	// Type is one of the Event* constants.
	Type string `json:"type"`

	// SessionID is the stable AGUI session identifier.
	SessionID string `json:"session_id"`

	// RunID is the current gollem run ID.
	RunID string `json:"run_id,omitempty"`

	// ParentRunID is the parent run ID for nested/child runs.
	ParentRunID string `json:"parent_run_id,omitempty"`

	// TurnNumber is the current turn within the run (1-based).
	TurnNumber int `json:"turn_number,omitempty"`

	// Timestamp is when the event occurred.
	Timestamp time.Time `json:"timestamp"`

	// Data contains event-type-specific payload.
	Data json.RawMessage `json:"data,omitempty"`
}

// Sequencer assigns monotonically increasing sequence numbers to events.
type Sequencer struct {
	counter atomic.Uint64
}

// Next returns the next sequence number.
func (s *Sequencer) Next() uint64 {
	return s.counter.Add(1)
}

// ── Event data payloads ─────────────────────────────────────────────

// SessionOpenedData is the payload for EventSessionOpened.
type SessionOpenedData struct {
	Mode string `json:"mode"` // "core-run", "core-stream", "core-iter", "temporal", "graph", "team"
}

// SessionWaitingData is the payload for EventSessionWaiting.
type SessionWaitingData struct {
	Reason string `json:"reason"` // "approval", "deferred", "approval_and_deferred"
}

// RunStartedData is the payload for EventRunStarted.
type RunStartedData struct {
	Prompt string `json:"prompt,omitempty"`
}

// RunCompletedData is the payload for EventRunCompleted.
type RunCompletedData struct {
	Success bool   `json:"success"`
	Error   string `json:"error,omitempty"`
}

// TurnData is the payload for EventTurnStarted / EventTurnCompleted.
type TurnData struct {
	TurnNumber int `json:"turn_number"`
}

// ModelRequestData is the payload for EventModelRequestStarted.
type ModelRequestData struct {
	MessageCount int `json:"message_count"`
}

// ModelResponseData is the payload for EventModelResponseCompleted.
type ModelResponseData struct {
	FinishReason string `json:"finish_reason,omitempty"`
	InputTokens  int    `json:"input_tokens,omitempty"`
	OutputTokens int    `json:"output_tokens,omitempty"`
	HasToolCalls bool   `json:"has_tool_calls,omitempty"`
	HasText      bool   `json:"has_text,omitempty"`
}

// TextDeltaData is the payload for EventModelOutputTextDelta.
type TextDeltaData struct {
	Delta string `json:"delta"`
}

// ThinkingDeltaData is the payload for EventModelOutputThinkingDelta.
type ThinkingDeltaData struct {
	Delta string `json:"delta"`
}

// ToolCallDeltaData is the payload for EventModelOutputToolCallDelta.
type ToolCallDeltaData struct {
	ToolCallID string `json:"tool_call_id"`
	ToolName   string `json:"tool_name,omitempty"`
	ArgsDelta  string `json:"args_delta,omitempty"`
}

// ToolExecutionData is the payload for tool execution events.
type ToolExecutionData struct {
	ToolCallID string `json:"tool_call_id"`
	ToolName   string `json:"tool_name"`
	ArgsJSON   string `json:"args_json,omitempty"`
	Result     string `json:"result,omitempty"`
	Error      string `json:"error,omitempty"`
	DurationMs int64  `json:"duration_ms,omitempty"`
}

// ApprovalData is the payload for approval events.
type ApprovalData struct {
	ToolCallID string `json:"tool_call_id"`
	ToolName   string `json:"tool_name"`
	ArgsJSON   string `json:"args_json,omitempty"`
	Approved   *bool  `json:"approved,omitempty"` // nil for requested, set for resolved
	Message    string `json:"message,omitempty"`
}

// ExternalInputData is the payload for external input (deferred tool) events.
type ExternalInputData struct {
	ToolCallID string `json:"tool_call_id"`
	ToolName   string `json:"tool_name"`
	ArgsJSON   string `json:"args_json,omitempty"`
	Content    string `json:"content,omitempty"`
	IsError    bool   `json:"is_error,omitempty"`
}

// ErrorData is a general error payload.
type ErrorData struct {
	Message string `json:"message"`
	Code    string `json:"code,omitempty"`
}

// MarshalData serializes a payload into json.RawMessage for Event.Data.
func MarshalData(v any) json.RawMessage {
	if v == nil {
		return nil
	}
	b, err := json.Marshal(v)
	if err != nil {
		return json.RawMessage(`{"error":"failed to marshal event data"}`)
	}
	return b
}
