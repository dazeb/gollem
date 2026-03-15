package orchestrator

import (
	"context"
	"testing"
	"time"
)

func TestSchedulerDispatchExcludesLocallyActiveTasks(t *testing.T) {
	store := &excludeRecordingStore{}
	scheduler := NewScheduler(store, store, RunnerFunc(func(context.Context, *ClaimedTask) (*TaskOutcome, error) {
		t.Fatal("runner should not execute")
		return nil, nil
	}))

	scheduler.active["task-a"] = activeTaskRun{
		claim: &ClaimedTask{
			Task: &Task{ID: "task-a"},
			Run:  &RunRef{ID: "run-a"},
		},
		cancel: func(error) {},
	}

	if err := scheduler.dispatch(context.Background()); err != nil {
		t.Fatalf("dispatch failed: %v", err)
	}
	if len(store.claims) != 1 {
		t.Fatalf("expected 1 claim attempt, got %d", len(store.claims))
	}
	if !containsTaskID(store.claims[0].ExcludeTaskIDs, "task-a") {
		t.Fatalf("expected dispatch to exclude locally active task, got %+v", store.claims[0].ExcludeTaskIDs)
	}
}

type excludeRecordingStore struct {
	claims []ClaimTaskRequest
}

func (s *excludeRecordingStore) ClaimReadyTask(_ context.Context, req ClaimTaskRequest) (*ClaimedTask, error) {
	s.claims = append(s.claims, req)
	return nil, ErrNoReadyTask
}

func (s *excludeRecordingStore) CreateTask(context.Context, CreateTaskRequest) (*Task, error) {
	panic("unexpected call")
}

func (s *excludeRecordingStore) GetTask(context.Context, string) (*Task, error) {
	panic("unexpected call")
}

func (s *excludeRecordingStore) ListTasks(context.Context, TaskFilter) ([]*Task, error) {
	panic("unexpected call")
}

func (s *excludeRecordingStore) ClaimTask(context.Context, string, ClaimTaskRequest) (*ClaimedTask, error) {
	panic("unexpected call")
}

func (s *excludeRecordingStore) UpdateTask(context.Context, UpdateTaskRequest) (*Task, error) {
	panic("unexpected call")
}

func (s *excludeRecordingStore) DeleteTask(context.Context, string) error {
	panic("unexpected call")
}

func (s *excludeRecordingStore) CompleteTask(context.Context, string, string, *TaskOutcome, time.Time) (*Task, error) {
	panic("unexpected call")
}

func (s *excludeRecordingStore) FailTask(context.Context, string, string, error, time.Time) (*Task, error) {
	panic("unexpected call")
}

func (s *excludeRecordingStore) CancelTask(context.Context, string, string, string, time.Time) (*Task, error) {
	panic("unexpected call")
}

func (s *excludeRecordingStore) RetryTask(context.Context, string, string, time.Time) (*Task, error) {
	panic("unexpected call")
}

func (s *excludeRecordingStore) GetLease(context.Context, string) (*Lease, error) {
	panic("unexpected call")
}

func (s *excludeRecordingStore) RenewLease(context.Context, string, string, time.Duration, time.Time) (*Lease, error) {
	panic("unexpected call")
}

func (s *excludeRecordingStore) ReleaseLease(context.Context, string, string) error {
	panic("unexpected call")
}

func containsTaskID(taskIDs []string, target string) bool {
	for _, taskID := range taskIDs {
		if taskID == target {
			return true
		}
	}
	return false
}

var _ LeaseStore = (*excludeRecordingStore)(nil)
var _ TaskStore = (*excludeRecordingStore)(nil)
