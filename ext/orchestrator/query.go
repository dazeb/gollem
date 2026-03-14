package orchestrator

import (
	"context"
	"errors"
	"time"
)

// RunQueryStore exposes optimized current-state run queries.
type RunQueryStore interface {
	ListActiveRuns(ctx context.Context, filter ActiveRunFilter) ([]*ActiveRunSummary, error)
	GetActiveRun(ctx context.Context, runID string) (*ActiveRunSummary, error)
}

// CommandQueryStore exposes optimized current-state command queries.
type CommandQueryStore interface {
	ListPendingCommandsForWorker(ctx context.Context, workerID string) ([]*Command, error)
}

// RecoveryQueryStore exposes optimized recovery inspection queries.
type RecoveryQueryStore interface {
	ListExpiredLeases(ctx context.Context, now time.Time) ([]*ExpiredLeaseSummary, error)
	ListStaleClaimedCommands(ctx context.Context, claimedBefore time.Time) ([]*StaleClaimedCommandSummary, error)
}

// ActiveRunSummary is the current-state view of a running task attempt.
type ActiveRunSummary struct {
	RunID       string
	TaskID      string
	TaskKind    string
	TaskSubject string
	WorkerID    string
	Attempt     int
	StartedAt   time.Time
	UpdatedAt   time.Time
}

// ExpiredLeaseSummary is the current-state view of a lease eligible for recovery.
type ExpiredLeaseSummary struct {
	LeaseID   string
	TaskID    string
	RunID     string
	WorkerID  string
	Attempt   int
	ExpiresAt time.Time
}

// StaleClaimedCommandSummary is the current-state view of a claimed command eligible for recovery.
type StaleClaimedCommandSummary struct {
	CommandID      string
	Kind           CommandKind
	TaskID         string
	RunID          string
	TargetWorkerID string
	ClaimedBy      string
	ClaimedAt      time.Time
	Reason         string
}

// ActiveRunFilter narrows current active-run queries.
type ActiveRunFilter struct {
	WorkerID string
	Kinds    []string
}

// ListActiveRuns projects currently running task attempts from the task store.
func ListActiveRuns(ctx context.Context, tasks TaskStore, filter ActiveRunFilter) ([]*ActiveRunSummary, error) {
	if tasks == nil {
		return nil, errors.New("orchestrator: task store must not be nil")
	}
	if queryStore, ok := tasks.(RunQueryStore); ok {
		return queryStore.ListActiveRuns(ctx, filter)
	}
	kinds := make([]string, len(filter.Kinds))
	copy(kinds, filter.Kinds)
	list, err := tasks.ListTasks(ctx, TaskFilter{
		Kinds:    kinds,
		Statuses: []TaskStatus{TaskRunning},
	})
	if err != nil {
		return nil, err
	}

	out := make([]*ActiveRunSummary, 0, len(list))
	for _, task := range list {
		if task == nil || task.Run == nil || task.Run.ID == "" {
			continue
		}
		if filter.WorkerID != "" && task.Run.WorkerID != filter.WorkerID {
			continue
		}
		out = append(out, &ActiveRunSummary{
			RunID:       task.Run.ID,
			TaskID:      task.ID,
			TaskKind:    task.Kind,
			TaskSubject: task.Subject,
			WorkerID:    task.Run.WorkerID,
			Attempt:     task.Run.Attempt,
			StartedAt:   task.Run.StartedAt,
			UpdatedAt:   task.UpdatedAt,
		})
	}
	return out, nil
}

// GetActiveRun returns the current-state view for a single active run.
func GetActiveRun(ctx context.Context, tasks TaskStore, runID string) (*ActiveRunSummary, error) {
	if runID == "" {
		return nil, ErrRunNotFound
	}
	if queryStore, ok := tasks.(RunQueryStore); ok {
		return queryStore.GetActiveRun(ctx, runID)
	}
	runs, err := ListActiveRuns(ctx, tasks, ActiveRunFilter{})
	if err != nil {
		return nil, err
	}
	for _, run := range runs {
		if run != nil && run.RunID == runID {
			return run, nil
		}
	}
	return nil, ErrRunNotFound
}

// ListPendingCommandsForWorker returns pending commands that workerID can claim.
// Commands explicitly targeted at another worker are excluded.
func ListPendingCommandsForWorker(ctx context.Context, commands CommandStore, workerID string) ([]*Command, error) {
	if commands == nil {
		return nil, errors.New("orchestrator: command store must not be nil")
	}
	if queryStore, ok := commands.(CommandQueryStore); ok {
		return queryStore.ListPendingCommandsForWorker(ctx, workerID)
	}
	all, err := commands.ListCommands(ctx, CommandFilter{
		Statuses: []CommandStatus{CommandPending},
	})
	if err != nil {
		return nil, err
	}
	out := make([]*Command, 0, len(all))
	for _, command := range all {
		if command == nil {
			continue
		}
		if command.TargetWorkerID != "" && command.TargetWorkerID != workerID {
			continue
		}
		out = append(out, cloneCommandView(command))
	}
	return out, nil
}

// ListExpiredLeases returns expired task leases that are eligible for recovery.
func ListExpiredLeases(ctx context.Context, store any, now time.Time) ([]*ExpiredLeaseSummary, error) {
	queryStore, ok := store.(RecoveryQueryStore)
	if !ok {
		return nil, errors.New("orchestrator: recovery query store not available")
	}
	return queryStore.ListExpiredLeases(ctx, now)
}

// ListStaleClaimedCommands returns claimed commands old enough to be released back to pending.
func ListStaleClaimedCommands(ctx context.Context, store any, claimedBefore time.Time) ([]*StaleClaimedCommandSummary, error) {
	queryStore, ok := store.(RecoveryQueryStore)
	if !ok {
		return nil, errors.New("orchestrator: recovery query store not available")
	}
	return queryStore.ListStaleClaimedCommands(ctx, claimedBefore)
}

func cloneCommandView(src *Command) *Command {
	if src == nil {
		return nil
	}
	cp := *src
	cp.Metadata = cloneAnyMap(src.Metadata)
	return &cp
}
