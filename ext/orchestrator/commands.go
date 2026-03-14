package orchestrator

import (
	"context"
	"time"
)

// CommandKind identifies a durable orchestration command.
type CommandKind string

const (
	CommandCancelTask CommandKind = "cancel_task"
	CommandRetryTask  CommandKind = "retry_task"
)

// CommandStatus tracks a command's lifecycle.
type CommandStatus string

const (
	CommandPending CommandStatus = "pending"
	CommandClaimed CommandStatus = "claimed"
	CommandHandled CommandStatus = "handled"
)

// Command is a durable addressed control instruction for orchestration state.
type Command struct {
	ID             string
	Kind           CommandKind
	TaskID         string
	RunID          string
	TargetWorkerID string
	Reason         string
	Metadata       map[string]any
	Status         CommandStatus
	ClaimToken     string
	ClaimedBy      string
	HandledBy      string
	CreatedAt      time.Time
	ClaimedAt      time.Time
	HandledAt      time.Time
}

// CreateCommandRequest describes a new orchestration command.
type CreateCommandRequest struct {
	Kind     CommandKind
	TaskID   string
	RunID    string
	Reason   string
	Metadata map[string]any
}

// CommandFilter narrows ListCommands results.
type CommandFilter struct {
	Kinds          []CommandKind
	Statuses       []CommandStatus
	TaskID         string
	RunID          string
	TargetWorkerID string
}

// ClaimCommandRequest describes a scheduler claim attempt for pending commands.
type ClaimCommandRequest struct {
	WorkerID string
	Now      time.Time
}

// CommandStore persists durable orchestration commands.
type CommandStore interface {
	CreateCommand(ctx context.Context, req CreateCommandRequest) (*Command, error)
	GetCommand(ctx context.Context, id string) (*Command, error)
	ListCommands(ctx context.Context, filter CommandFilter) ([]*Command, error)
	ClaimPendingCommand(ctx context.Context, req ClaimCommandRequest) (*Command, error)
	HandleCommand(ctx context.Context, id, claimToken, handledBy string, now time.Time) (*Command, error)
	ReleaseCommand(ctx context.Context, id, claimToken string) error
}
