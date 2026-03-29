package core

import (
	"errors"
	"time"
)

const (
	// RuntimeEventTypeRunStarted marks a run-start lifecycle event.
	RuntimeEventTypeRunStarted = "run_started"
	// RuntimeEventTypeRunCompleted marks a run-complete lifecycle event.
	RuntimeEventTypeRunCompleted = "run_completed"
	// RuntimeEventTypeToolCalled marks a tool-start lifecycle event.
	RuntimeEventTypeToolCalled = "tool_called"
	// RuntimeEventTypeToolCompleted marks a tool-end lifecycle event (success).
	RuntimeEventTypeToolCompleted = "tool_completed"
	// RuntimeEventTypeToolFailed marks a tool-end lifecycle event (error).
	RuntimeEventTypeToolFailed = "tool_failed"
	// RuntimeEventTypeTurnStarted marks the start of an agent turn.
	RuntimeEventTypeTurnStarted = "turn_started"
	// RuntimeEventTypeTurnCompleted marks the end of an agent turn.
	RuntimeEventTypeTurnCompleted = "turn_completed"
	// RuntimeEventTypeModelRequestStarted marks the start of a model request.
	RuntimeEventTypeModelRequestStarted = "model_request_started"
	// RuntimeEventTypeModelResponseCompleted marks the end of a model response.
	RuntimeEventTypeModelResponseCompleted = "model_response_completed"
	// RuntimeEventTypeApprovalRequested marks a tool approval request.
	RuntimeEventTypeApprovalRequested = "approval_requested"
	// RuntimeEventTypeApprovalResolved marks a tool approval resolution.
	RuntimeEventTypeApprovalResolved = "approval_resolved"
	// RuntimeEventTypeDeferredRequested marks a deferred tool request.
	RuntimeEventTypeDeferredRequested = "deferred_requested"
	// RuntimeEventTypeDeferredResolved marks a deferred tool resolution.
	RuntimeEventTypeDeferredResolved = "deferred_resolved"
	// RuntimeEventTypeRunWaiting marks a run entering a waiting state.
	RuntimeEventTypeRunWaiting = "run_waiting"
	// RuntimeEventTypeRunResumed marks a run resuming from a waiting state.
	RuntimeEventTypeRunResumed = "run_resumed"
)

// RuntimeEvent is implemented by built-in runtime lifecycle events.
type RuntimeEvent interface {
	RuntimeEventType() string
	RuntimeRunID() string
	RuntimeParentRunID() string
	RuntimeOccurredAt() time.Time
}

// RunStartedEvent is published when an agent run starts.
type RunStartedEvent struct {
	RunID       string
	ParentRunID string
	Prompt      string
	StartedAt   time.Time
}

func (e RunStartedEvent) RuntimeEventType() string     { return RuntimeEventTypeRunStarted }
func (e RunStartedEvent) RuntimeRunID() string         { return e.RunID }
func (e RunStartedEvent) RuntimeParentRunID() string   { return e.ParentRunID }
func (e RunStartedEvent) RuntimeOccurredAt() time.Time { return e.StartedAt }

// RunCompletedEvent is published when an agent run completes.
type RunCompletedEvent struct {
	RunID       string
	ParentRunID string
	Success     bool
	Deferred    bool
	Error       string
	StartedAt   time.Time
	CompletedAt time.Time
}

func (e RunCompletedEvent) RuntimeEventType() string     { return RuntimeEventTypeRunCompleted }
func (e RunCompletedEvent) RuntimeRunID() string         { return e.RunID }
func (e RunCompletedEvent) RuntimeParentRunID() string   { return e.ParentRunID }
func (e RunCompletedEvent) RuntimeOccurredAt() time.Time { return e.CompletedAt }

// ToolCalledEvent is published when a tool call starts.
type ToolCalledEvent struct {
	RunID       string
	ParentRunID string
	ToolCallID  string
	ToolName    string
	ArgsJSON    string
	CalledAt    time.Time
}

func (e ToolCalledEvent) RuntimeEventType() string     { return RuntimeEventTypeToolCalled }
func (e ToolCalledEvent) RuntimeRunID() string         { return e.RunID }
func (e ToolCalledEvent) RuntimeParentRunID() string   { return e.ParentRunID }
func (e ToolCalledEvent) RuntimeOccurredAt() time.Time { return e.CalledAt }

// NewRunStartedEvent constructs a standardized run-start event.
func NewRunStartedEvent(runID, parentRunID, prompt string, startedAt time.Time) RunStartedEvent {
	return RunStartedEvent{
		RunID:       runID,
		ParentRunID: parentRunID,
		Prompt:      prompt,
		StartedAt:   startedAt,
	}
}

// NewRunCompletedEvent constructs a standardized run-complete event.
func NewRunCompletedEvent(runID, parentRunID string, startedAt, completedAt time.Time, runErr error) RunCompletedEvent {
	evt := RunCompletedEvent{
		RunID:       runID,
		ParentRunID: parentRunID,
		Success:     runErr == nil,
		Deferred:    isDeferredRunError(runErr),
		StartedAt:   startedAt,
		CompletedAt: completedAt,
	}
	if runErr != nil {
		evt.Error = runErr.Error()
	}
	return evt
}

type deferredRunError interface {
	error
	deferredRunError()
}

func isDeferredRunError(err error) bool {
	if err == nil {
		return false
	}
	var deferred deferredRunError
	return errors.As(err, &deferred)
}

// NewToolCalledEvent constructs a standardized tool-start event.
func NewToolCalledEvent(runID, parentRunID, toolCallID, toolName, argsJSON string, calledAt time.Time) ToolCalledEvent {
	return ToolCalledEvent{
		RunID:       runID,
		ParentRunID: parentRunID,
		ToolCallID:  toolCallID,
		ToolName:    toolName,
		ArgsJSON:    argsJSON,
		CalledAt:    calledAt,
	}
}

// ToolCompletedEvent is published when a tool call completes successfully.
type ToolCompletedEvent struct {
	RunID       string
	ParentRunID string
	ToolCallID  string
	ToolName    string
	Result      string
	DurationMs  int64
	CompletedAt time.Time
}

func (e ToolCompletedEvent) RuntimeEventType() string     { return RuntimeEventTypeToolCompleted }
func (e ToolCompletedEvent) RuntimeRunID() string         { return e.RunID }
func (e ToolCompletedEvent) RuntimeParentRunID() string   { return e.ParentRunID }
func (e ToolCompletedEvent) RuntimeOccurredAt() time.Time { return e.CompletedAt }

// ToolFailedEvent is published when a tool call fails.
type ToolFailedEvent struct {
	RunID       string
	ParentRunID string
	ToolCallID  string
	ToolName    string
	Error       string
	DurationMs  int64
	FailedAt    time.Time
}

func (e ToolFailedEvent) RuntimeEventType() string     { return RuntimeEventTypeToolFailed }
func (e ToolFailedEvent) RuntimeRunID() string         { return e.RunID }
func (e ToolFailedEvent) RuntimeParentRunID() string   { return e.ParentRunID }
func (e ToolFailedEvent) RuntimeOccurredAt() time.Time { return e.FailedAt }

// TurnStartedEvent is published when an agent turn begins.
type TurnStartedEvent struct {
	RunID       string
	ParentRunID string
	TurnNumber  int
	StartedAt   time.Time
}

func (e TurnStartedEvent) RuntimeEventType() string     { return RuntimeEventTypeTurnStarted }
func (e TurnStartedEvent) RuntimeRunID() string         { return e.RunID }
func (e TurnStartedEvent) RuntimeParentRunID() string   { return e.ParentRunID }
func (e TurnStartedEvent) RuntimeOccurredAt() time.Time { return e.StartedAt }

// TurnCompletedEvent is published when an agent turn ends.
// If the turn ended due to an error, Error is non-empty.
type TurnCompletedEvent struct {
	RunID        string
	ParentRunID  string
	TurnNumber   int
	HasToolCalls bool
	HasText      bool
	Error        string
	CompletedAt  time.Time
}

func (e TurnCompletedEvent) RuntimeEventType() string     { return RuntimeEventTypeTurnCompleted }
func (e TurnCompletedEvent) RuntimeRunID() string         { return e.RunID }
func (e TurnCompletedEvent) RuntimeParentRunID() string   { return e.ParentRunID }
func (e TurnCompletedEvent) RuntimeOccurredAt() time.Time { return e.CompletedAt }

// ModelRequestStartedEvent is published before a model request is sent.
type ModelRequestStartedEvent struct {
	RunID        string
	ParentRunID  string
	TurnNumber   int
	MessageCount int
	StartedAt    time.Time
}

func (e ModelRequestStartedEvent) RuntimeEventType() string {
	return RuntimeEventTypeModelRequestStarted
}
func (e ModelRequestStartedEvent) RuntimeRunID() string         { return e.RunID }
func (e ModelRequestStartedEvent) RuntimeParentRunID() string   { return e.ParentRunID }
func (e ModelRequestStartedEvent) RuntimeOccurredAt() time.Time { return e.StartedAt }

// ModelResponseCompletedEvent is published after a model response is received.
type ModelResponseCompletedEvent struct {
	RunID        string
	ParentRunID  string
	TurnNumber   int
	FinishReason string
	InputTokens  int
	OutputTokens int
	HasToolCalls bool
	HasText      bool
	DurationMs   int64
	CompletedAt  time.Time
}

func (e ModelResponseCompletedEvent) RuntimeEventType() string {
	return RuntimeEventTypeModelResponseCompleted
}
func (e ModelResponseCompletedEvent) RuntimeRunID() string         { return e.RunID }
func (e ModelResponseCompletedEvent) RuntimeParentRunID() string   { return e.ParentRunID }
func (e ModelResponseCompletedEvent) RuntimeOccurredAt() time.Time { return e.CompletedAt }

// ApprovalRequestedEvent is published when a tool requires approval.
type ApprovalRequestedEvent struct {
	RunID       string
	ParentRunID string
	ToolCallID  string
	ToolName    string
	ArgsJSON    string
	RequestedAt time.Time
}

func (e ApprovalRequestedEvent) RuntimeEventType() string     { return RuntimeEventTypeApprovalRequested }
func (e ApprovalRequestedEvent) RuntimeRunID() string         { return e.RunID }
func (e ApprovalRequestedEvent) RuntimeParentRunID() string   { return e.ParentRunID }
func (e ApprovalRequestedEvent) RuntimeOccurredAt() time.Time { return e.RequestedAt }

// ApprovalResolvedEvent is published when a tool approval is resolved.
type ApprovalResolvedEvent struct {
	RunID       string
	ParentRunID string
	ToolCallID  string
	ToolName    string
	Approved    bool
	ResolvedAt  time.Time
}

func (e ApprovalResolvedEvent) RuntimeEventType() string     { return RuntimeEventTypeApprovalResolved }
func (e ApprovalResolvedEvent) RuntimeRunID() string         { return e.RunID }
func (e ApprovalResolvedEvent) RuntimeParentRunID() string   { return e.ParentRunID }
func (e ApprovalResolvedEvent) RuntimeOccurredAt() time.Time { return e.ResolvedAt }

// DeferredRequestedEvent is published when a tool call is deferred.
type DeferredRequestedEvent struct {
	RunID       string
	ParentRunID string
	ToolCallID  string
	ToolName    string
	ArgsJSON    string
	RequestedAt time.Time
}

func (e DeferredRequestedEvent) RuntimeEventType() string     { return RuntimeEventTypeDeferredRequested }
func (e DeferredRequestedEvent) RuntimeRunID() string         { return e.RunID }
func (e DeferredRequestedEvent) RuntimeParentRunID() string   { return e.ParentRunID }
func (e DeferredRequestedEvent) RuntimeOccurredAt() time.Time { return e.RequestedAt }

// DeferredResolvedEvent is published when a deferred tool result is provided.
type DeferredResolvedEvent struct {
	RunID       string
	ParentRunID string
	ToolCallID  string
	ToolName    string
	Content     string
	IsError     bool
	ResolvedAt  time.Time
}

func (e DeferredResolvedEvent) RuntimeEventType() string     { return RuntimeEventTypeDeferredResolved }
func (e DeferredResolvedEvent) RuntimeRunID() string         { return e.RunID }
func (e DeferredResolvedEvent) RuntimeParentRunID() string   { return e.ParentRunID }
func (e DeferredResolvedEvent) RuntimeOccurredAt() time.Time { return e.ResolvedAt }

// RunWaitingEvent is published when a run enters a waiting state.
type RunWaitingEvent struct {
	RunID       string
	ParentRunID string
	Reason      string // "approval", "deferred", "approval_and_deferred"
	WaitingAt   time.Time
}

func (e RunWaitingEvent) RuntimeEventType() string     { return RuntimeEventTypeRunWaiting }
func (e RunWaitingEvent) RuntimeRunID() string         { return e.RunID }
func (e RunWaitingEvent) RuntimeParentRunID() string   { return e.ParentRunID }
func (e RunWaitingEvent) RuntimeOccurredAt() time.Time { return e.WaitingAt }

// RunResumedEvent is published when a run resumes from a waiting state.
type RunResumedEvent struct {
	RunID       string
	ParentRunID string
	ResumedAt   time.Time
}

func (e RunResumedEvent) RuntimeEventType() string     { return RuntimeEventTypeRunResumed }
func (e RunResumedEvent) RuntimeRunID() string         { return e.RunID }
func (e RunResumedEvent) RuntimeParentRunID() string   { return e.ParentRunID }
func (e RunResumedEvent) RuntimeOccurredAt() time.Time { return e.ResumedAt }
