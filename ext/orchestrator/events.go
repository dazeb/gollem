package orchestrator

import "time"

// TaskCreatedEvent is published when a task is created in an orchestrator store.
type TaskCreatedEvent struct {
	TaskID      string
	Kind        string
	Subject     string
	Description string
	CreatedAt   time.Time
}

// TaskUpdatedEvent is published when a task's non-terminal fields are updated.
type TaskUpdatedEvent struct {
	TaskID    string
	Subject   string
	Blocks    []string
	BlockedBy []string
	UpdatedAt time.Time
}

// TaskDeletedEvent is published when a task is removed.
type TaskDeletedEvent struct {
	TaskID    string
	DeletedAt time.Time
}

// TaskClaimedEvent is published when a worker acquires a task lease.
type TaskClaimedEvent struct {
	TaskID     string
	RunID      string
	LeaseID    string
	WorkerID   string
	Attempt    int
	AcquiredAt time.Time
	ExpiresAt  time.Time
}

// LeaseRenewedEvent is published when an active task lease is renewed.
type LeaseRenewedEvent struct {
	TaskID    string
	LeaseID   string
	WorkerID  string
	ExpiresAt time.Time
}

// LeaseReleasedEvent is published when a task lease is released manually.
type LeaseReleasedEvent struct {
	TaskID     string
	LeaseID    string
	WorkerID   string
	ReleasedAt time.Time
	Requeued   bool
}

// TaskRequeuedEvent is published when a running task returns to pending.
type TaskRequeuedEvent struct {
	TaskID      string
	LastRunID   string
	LastAttempt int
	Reason      string
	RequeuedAt  time.Time
}

// TaskCompletedEvent is published when a task reaches completed.
type TaskCompletedEvent struct {
	TaskID      string
	RunID       string
	Attempt     int
	CompletedAt time.Time
}

// TaskFailedEvent is published when a task reaches failed.
type TaskFailedEvent struct {
	TaskID   string
	RunID    string
	Attempt  int
	Error    string
	FailedAt time.Time
}

// TaskCanceledEvent is published when a task is canceled.
type TaskCanceledEvent struct {
	TaskID     string
	RunID      string
	Attempt    int
	Reason     string
	CanceledAt time.Time
}

// ArtifactCreatedEvent is published when an artifact is persisted.
type ArtifactCreatedEvent struct {
	ArtifactID  string
	TaskID      string
	RunID       string
	Kind        string
	Name        string
	ContentType string
	SizeBytes   int
	CreatedAt   time.Time
}

// CommandCreatedEvent is published when a durable command is created.
type CommandCreatedEvent struct {
	CommandID      string
	Kind           CommandKind
	TaskID         string
	RunID          string
	TargetWorkerID string
	CreatedAt      time.Time
}

// CommandClaimedEvent is published when a durable command is claimed by a worker.
type CommandClaimedEvent struct {
	CommandID string
	Kind      CommandKind
	TaskID    string
	RunID     string
	ClaimedBy string
	ClaimedAt time.Time
}

// CommandReleasedEvent is published when a claimed command is returned to pending.
type CommandReleasedEvent struct {
	CommandID  string
	Kind       CommandKind
	TaskID     string
	RunID      string
	ReleasedBy string
	ReleasedAt time.Time
}

// CommandHandledEvent is published when a durable command is handled.
type CommandHandledEvent struct {
	CommandID string
	Kind      CommandKind
	TaskID    string
	RunID     string
	HandledBy string
	HandledAt time.Time
}
