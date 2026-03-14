package core

import "time"

const (
	// RuntimeEventTypeRunStarted marks a run-start lifecycle event.
	RuntimeEventTypeRunStarted = "run_started"
	// RuntimeEventTypeRunCompleted marks a run-complete lifecycle event.
	RuntimeEventTypeRunCompleted = "run_completed"
	// RuntimeEventTypeToolCalled marks a tool-start lifecycle event.
	RuntimeEventTypeToolCalled = "tool_called"
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
		StartedAt:   startedAt,
		CompletedAt: completedAt,
	}
	if runErr != nil {
		evt.Error = runErr.Error()
	}
	return evt
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
