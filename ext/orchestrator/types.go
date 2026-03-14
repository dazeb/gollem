package orchestrator

import (
	"errors"
	"time"

	"github.com/fugue-labs/gollem/core"
)

// TaskStatus tracks a task's orchestration lifecycle.
type TaskStatus string

const (
	TaskPending   TaskStatus = "pending"
	TaskRunning   TaskStatus = "running"
	TaskCompleted TaskStatus = "completed"
	TaskFailed    TaskStatus = "failed"
	TaskCanceled  TaskStatus = "canceled"
)

var (
	ErrTaskNotFound           = errors.New("orchestrator: task not found")
	ErrTaskDependencyNotFound = errors.New("orchestrator: task dependency not found")
	ErrTaskBlocked            = errors.New("orchestrator: task blocked")
	ErrNoReadyTask            = errors.New("orchestrator: no ready task")
	ErrTaskNotCancelable      = errors.New("orchestrator: task not cancelable")
	ErrTaskNotRetryable       = errors.New("orchestrator: task not retryable")
	ErrArtifactNotFound       = errors.New("orchestrator: artifact not found")
	ErrArtifactTaskRequired   = errors.New("orchestrator: artifact task id required")
	ErrLeaseNotFound          = errors.New("orchestrator: lease not found")
	ErrLeaseExpired           = errors.New("orchestrator: lease expired")
	ErrLeaseMismatch          = errors.New("orchestrator: lease token mismatch")
	ErrCommandNotFound        = errors.New("orchestrator: command not found")
	ErrNoPendingCommand       = errors.New("orchestrator: no pending command")
	ErrCommandClaimMismatch   = errors.New("orchestrator: command claim mismatch")
	ErrInvalidCommand         = errors.New("orchestrator: invalid command")
)

// RetryableError marks a task failure that should be requeued while attempts remain.
type RetryableError struct {
	Err error
}

// Error implements error.
func (e *RetryableError) Error() string {
	if e == nil || e.Err == nil {
		return "retryable task failure"
	}
	return e.Err.Error()
}

// Unwrap implements errors.Wrapper.
func (e *RetryableError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Err
}

// Retryable wraps err so the scheduler treats the failure as requeueable.
func Retryable(err error) error {
	if err == nil {
		return nil
	}
	return &RetryableError{Err: err}
}

// Task is the durable unit of work managed by the orchestrator.
type Task struct {
	ID          string
	Kind        string
	Subject     string
	Description string
	Input       string
	Status      TaskStatus
	Attempt     int
	MaxAttempts int
	Blocks      []string
	BlockedBy   []string
	Metadata    map[string]any
	Run         *RunRef
	Result      *TaskResult
	LastError   string
	CreatedAt   time.Time
	UpdatedAt   time.Time
	StartedAt   time.Time
	CompletedAt time.Time
}

// RunRef identifies a single scheduler-assigned task run attempt.
type RunRef struct {
	ID        string
	TaskID    string
	WorkerID  string
	Attempt   int
	StartedAt time.Time
}

// Lease grants temporary ownership of a task run to a worker.
type Lease struct {
	ID         string
	TaskID     string
	WorkerID   string
	Token      string
	AcquiredAt time.Time
	ExpiresAt  time.Time
}

// TaskResult captures runner output stored on a completed task.
type TaskResult struct {
	RunnerRunID string
	Output      any
	Usage       core.RunUsage
	ToolState   map[string]any
	Metadata    map[string]any
	CompletedAt time.Time
}

// CreateTaskRequest describes a new task to persist.
type CreateTaskRequest struct {
	Kind        string
	Subject     string
	Description string
	Input       string
	Blocks      []string
	BlockedBy   []string
	MaxAttempts int
	Metadata    map[string]any
}

// UpdateTaskRequest describes non-terminal task mutations owned by the store.
type UpdateTaskRequest struct {
	ID           string
	Subject      *string
	Description  *string
	AddBlocks    []string
	AddBlockedBy []string
	Metadata     map[string]any
}

// TaskFilter narrows ListTasks results.
type TaskFilter struct {
	Kinds    []string
	Statuses []TaskStatus
}

// ClaimTaskRequest describes a scheduler claim attempt.
type ClaimTaskRequest struct {
	WorkerID string
	LeaseTTL time.Duration
	Now      time.Time
	Kinds    []string
}

// ClaimedTask is the atomic result of selecting a ready task and acquiring its lease.
type ClaimedTask struct {
	Task  *Task
	Lease *Lease
	Run   *RunRef
}

func cloneAnyMap(src map[string]any) map[string]any {
	if len(src) == 0 {
		return nil
	}
	cloned := make(map[string]any, len(src))
	for k, v := range src {
		cloned[k] = v
	}
	return cloned
}
