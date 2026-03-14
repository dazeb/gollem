package memory

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/fugue-labs/gollem/core"
	"github.com/fugue-labs/gollem/ext/orchestrator"
)

func TestStore_CreateArtifact(t *testing.T) {
	store := NewStore()
	task, err := store.CreateTask(context.Background(), orchestrator.CreateTaskRequest{
		Kind:    "prompt",
		Subject: "summarize logs",
		Input:   "Summarize the attached logs.",
	})
	if err != nil {
		t.Fatalf("CreateTask failed: %v", err)
	}

	artifact, err := store.CreateArtifact(context.Background(), orchestrator.CreateArtifactRequest{
		TaskID:      task.ID,
		RunID:       "run-1",
		Kind:        "report",
		Name:        "summary.md",
		ContentType: "text/markdown",
		Body:        []byte("# Summary\n\nAll checks passed."),
		Metadata:    map[string]any{"section_count": 1},
	})
	if err != nil {
		t.Fatalf("CreateArtifact failed: %v", err)
	}
	if artifact.ID == "" {
		t.Fatal("expected artifact ID")
	}
	if artifact.TaskID != task.ID {
		t.Fatalf("expected task ID %q, got %q", task.ID, artifact.TaskID)
	}
	if artifact.RunID != "run-1" {
		t.Fatalf("expected run-1, got %q", artifact.RunID)
	}
	if string(artifact.Body) != "# Summary\n\nAll checks passed." {
		t.Fatalf("unexpected artifact body: %q", string(artifact.Body))
	}

	got, err := store.GetArtifact(context.Background(), artifact.ID)
	if err != nil {
		t.Fatalf("GetArtifact failed: %v", err)
	}
	if got.Metadata["section_count"] != 1 {
		t.Fatalf("expected metadata to round-trip, got %+v", got.Metadata)
	}
	got.Body[0] = '!'

	again, err := store.GetArtifact(context.Background(), artifact.ID)
	if err != nil {
		t.Fatalf("GetArtifact second read failed: %v", err)
	}
	if string(again.Body) != "# Summary\n\nAll checks passed." {
		t.Fatalf("expected stored body copy to stay unchanged, got %q", string(again.Body))
	}
}

func TestStore_CreateArtifactRequiresTask(t *testing.T) {
	store := NewStore()

	if _, err := store.CreateArtifact(context.Background(), orchestrator.CreateArtifactRequest{
		Name: "missing-task",
	}); !errors.Is(err, orchestrator.ErrArtifactTaskRequired) {
		t.Fatalf("expected ErrArtifactTaskRequired, got %v", err)
	}

	if _, err := store.CreateArtifact(context.Background(), orchestrator.CreateArtifactRequest{
		TaskID: "task-does-not-exist",
		Name:   "missing-task",
	}); !errors.Is(err, orchestrator.ErrTaskNotFound) {
		t.Fatalf("expected ErrTaskNotFound, got %v", err)
	}
}

func TestStore_ListArtifactsFilters(t *testing.T) {
	store := NewStore()
	taskA, err := store.CreateTask(context.Background(), orchestrator.CreateTaskRequest{Kind: "prompt", Input: "a"})
	if err != nil {
		t.Fatalf("CreateTask A failed: %v", err)
	}
	taskB, err := store.CreateTask(context.Background(), orchestrator.CreateTaskRequest{Kind: "prompt", Input: "b"})
	if err != nil {
		t.Fatalf("CreateTask B failed: %v", err)
	}

	if _, err := store.CreateArtifact(context.Background(), orchestrator.CreateArtifactRequest{
		TaskID: taskA.ID,
		RunID:  "run-a",
		Kind:   "report",
		Name:   "summary.md",
	}); err != nil {
		t.Fatalf("CreateArtifact A failed: %v", err)
	}
	if _, err := store.CreateArtifact(context.Background(), orchestrator.CreateArtifactRequest{
		TaskID: taskB.ID,
		RunID:  "run-b",
		Kind:   "transcript",
		Name:   "trace.json",
	}); err != nil {
		t.Fatalf("CreateArtifact B failed: %v", err)
	}

	filtered, err := store.ListArtifacts(context.Background(), orchestrator.ArtifactFilter{
		TaskID: taskA.ID,
		Kind:   "report",
	})
	if err != nil {
		t.Fatalf("ListArtifacts failed: %v", err)
	}
	if len(filtered) != 1 {
		t.Fatalf("expected 1 filtered artifact, got %d", len(filtered))
	}
	if filtered[0].Name != "summary.md" {
		t.Fatalf("expected summary.md, got %q", filtered[0].Name)
	}
}

func TestStore_CreateArtifactPublishesEvent(t *testing.T) {
	bus := core.NewEventBus()
	store := NewStore(WithEventBus(bus))
	task, err := store.CreateTask(context.Background(), orchestrator.CreateTaskRequest{
		Kind:  "prompt",
		Input: "emit artifact",
	})
	if err != nil {
		t.Fatalf("CreateTask failed: %v", err)
	}

	var (
		mu     sync.Mutex
		events []orchestrator.ArtifactCreatedEvent
	)
	core.Subscribe(bus, func(event orchestrator.ArtifactCreatedEvent) {
		mu.Lock()
		events = append(events, event)
		mu.Unlock()
	})

	artifact, err := store.CreateArtifact(context.Background(), orchestrator.CreateArtifactRequest{
		TaskID:      task.ID,
		RunID:       "run-1",
		Kind:        "report",
		Name:        "summary.md",
		ContentType: "text/markdown",
		Body:        []byte("hello"),
	})
	if err != nil {
		t.Fatalf("CreateArtifact failed: %v", err)
	}

	waitForAsync(t, func() bool {
		mu.Lock()
		defer mu.Unlock()
		return len(events) == 1
	})

	mu.Lock()
	defer mu.Unlock()
	if events[0].ArtifactID != artifact.ID {
		t.Fatalf("expected artifact ID %q, got %q", artifact.ID, events[0].ArtifactID)
	}
	if events[0].TaskID != task.ID {
		t.Fatalf("expected task ID %q, got %q", task.ID, events[0].TaskID)
	}
	if events[0].SizeBytes != 5 {
		t.Fatalf("expected size 5, got %d", events[0].SizeBytes)
	}
}

func TestStore_DeleteTaskRemovesArtifacts(t *testing.T) {
	store := NewStore()
	taskA, err := store.CreateTask(context.Background(), orchestrator.CreateTaskRequest{
		Kind:  "prompt",
		Input: "task a",
	})
	if err != nil {
		t.Fatalf("CreateTask A failed: %v", err)
	}
	taskB, err := store.CreateTask(context.Background(), orchestrator.CreateTaskRequest{
		Kind:  "prompt",
		Input: "task b",
	})
	if err != nil {
		t.Fatalf("CreateTask B failed: %v", err)
	}

	deletedArtifact, err := store.CreateArtifact(context.Background(), orchestrator.CreateArtifactRequest{
		TaskID: taskA.ID,
		Name:   "a.txt",
		Body:   []byte("artifact-a"),
	})
	if err != nil {
		t.Fatalf("CreateArtifact A failed: %v", err)
	}
	keptArtifact, err := store.CreateArtifact(context.Background(), orchestrator.CreateArtifactRequest{
		TaskID: taskB.ID,
		Name:   "b.txt",
		Body:   []byte("artifact-b"),
	})
	if err != nil {
		t.Fatalf("CreateArtifact B failed: %v", err)
	}

	if err := store.DeleteTask(context.Background(), taskA.ID); err != nil {
		t.Fatalf("DeleteTask failed: %v", err)
	}

	if _, err := store.GetArtifact(context.Background(), deletedArtifact.ID); !errors.Is(err, orchestrator.ErrArtifactNotFound) {
		t.Fatalf("expected deleted task artifact to be removed, got %v", err)
	}

	artifacts, err := store.ListArtifacts(context.Background(), orchestrator.ArtifactFilter{})
	if err != nil {
		t.Fatalf("ListArtifacts failed: %v", err)
	}
	if len(artifacts) != 1 {
		t.Fatalf("expected 1 remaining artifact, got %d", len(artifacts))
	}
	if artifacts[0].ID != keptArtifact.ID {
		t.Fatalf("expected kept artifact %q, got %q", keptArtifact.ID, artifacts[0].ID)
	}
}

func TestStore_CompleteTaskPersistsOutcomeArtifactsAtomically(t *testing.T) {
	store := NewStore()
	task, err := store.CreateTask(context.Background(), orchestrator.CreateTaskRequest{
		Kind:  "prompt",
		Input: "complete with artifact",
	})
	if err != nil {
		t.Fatalf("CreateTask failed: %v", err)
	}

	claim, err := store.ClaimTask(context.Background(), task.ID, orchestrator.ClaimTaskRequest{
		WorkerID: "worker-a",
		LeaseTTL: time.Minute,
		Now:      time.Unix(1, 0).UTC(),
	})
	if err != nil {
		t.Fatalf("ClaimTask failed: %v", err)
	}

	completedAt := time.Unix(2, 0).UTC()
	completed, err := store.CompleteTask(context.Background(), task.ID, claim.Lease.Token, &orchestrator.TaskOutcome{
		Result: &orchestrator.TaskResult{Output: "done"},
		Artifacts: []orchestrator.ArtifactSpec{{
			Kind:        "report",
			Name:        "handoff.md",
			ContentType: "text/markdown",
			Body:        []byte("# done"),
		}},
	}, completedAt)
	if err != nil {
		t.Fatalf("CompleteTask failed: %v", err)
	}
	if completed.Status != orchestrator.TaskCompleted {
		t.Fatalf("expected completed status, got %s", completed.Status)
	}

	artifacts, err := store.ListArtifacts(context.Background(), orchestrator.ArtifactFilter{TaskID: task.ID})
	if err != nil {
		t.Fatalf("ListArtifacts failed: %v", err)
	}
	if len(artifacts) != 1 {
		t.Fatalf("expected 1 artifact, got %d", len(artifacts))
	}
	if artifacts[0].RunID != claim.Run.ID {
		t.Fatalf("expected artifact run ID %q, got %q", claim.Run.ID, artifacts[0].RunID)
	}
	if artifacts[0].CreatedAt != completedAt {
		t.Fatalf("expected artifact CreatedAt %v, got %v", completedAt, artifacts[0].CreatedAt)
	}
}
