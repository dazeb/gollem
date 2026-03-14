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
)

var (
	ErrTaskNotFound  = errors.New("orchestrator: task not found")
	ErrNoReadyTask   = errors.New("orchestrator: no ready task")
	ErrLeaseNotFound = errors.New("orchestrator: lease not found")
	ErrLeaseExpired  = errors.New("orchestrator: lease expired")
	ErrLeaseMismatch = errors.New("orchestrator: lease token mismatch")
)

// Task is the durable unit of work managed by the orchestrator.
type Task struct {
	ID          string
	Kind        string
	Input       string
	Status      TaskStatus
	Attempt     int
	MaxAttempts int
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
	Input       string
	MaxAttempts int
	Metadata    map[string]any
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

func cloneRunRef(ref *RunRef) *RunRef {
	if ref == nil {
		return nil
	}
	cp := *ref
	return &cp
}

func cloneLease(lease *Lease) *Lease {
	if lease == nil {
		return nil
	}
	cp := *lease
	return &cp
}

func cloneTaskResult(result *TaskResult) *TaskResult {
	if result == nil {
		return nil
	}
	cp := *result
	cp.ToolState = cloneAnyMap(result.ToolState)
	cp.Metadata = cloneAnyMap(result.Metadata)
	return &cp
}

func cloneTask(task *Task) *Task {
	if task == nil {
		return nil
	}
	cp := *task
	cp.Metadata = cloneAnyMap(task.Metadata)
	cp.Run = cloneRunRef(task.Run)
	cp.Result = cloneTaskResult(task.Result)
	return &cp
}
