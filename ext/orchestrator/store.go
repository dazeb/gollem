package orchestrator

import (
	"context"
	"time"
)

// TaskStore persists orchestrated tasks and owns the atomic claim transition.
type TaskStore interface {
	CreateTask(ctx context.Context, req CreateTaskRequest) (*Task, error)
	GetTask(ctx context.Context, id string) (*Task, error)
	ListTasks(ctx context.Context, filter TaskFilter) ([]*Task, error)
	ClaimReadyTask(ctx context.Context, req ClaimTaskRequest) (*ClaimedTask, error)
	ClaimTask(ctx context.Context, taskID string, req ClaimTaskRequest) (*ClaimedTask, error)
	UpdateTask(ctx context.Context, req UpdateTaskRequest) (*Task, error)
	DeleteTask(ctx context.Context, id string) error
	CompleteTask(ctx context.Context, taskID, leaseToken string, outcome *TaskOutcome, now time.Time) (*Task, error)
	FailTask(ctx context.Context, taskID, leaseToken string, runErr error, now time.Time) (*Task, error)
}

// LeaseStore manages task lease inspection and renewal.
type LeaseStore interface {
	GetLease(ctx context.Context, taskID string) (*Lease, error)
	RenewLease(ctx context.Context, taskID, leaseToken string, ttl time.Duration, now time.Time) (*Lease, error)
	ReleaseLease(ctx context.Context, taskID, leaseToken string) error
}
