package deep

import (
	"context"
	"testing"
	"time"

	"github.com/trevorprater/gollem"
)

func TestCheckpoint_SaveLoad(t *testing.T) {
	dir := t.TempDir()
	store, err := NewFileCheckpointStore(dir)
	if err != nil {
		t.Fatalf("NewFileCheckpointStore: %v", err)
	}

	cp := &Checkpoint{
		Messages: []gollem.ModelMessage{
			gollem.ModelRequest{
				Parts: []gollem.ModelRequestPart{
					gollem.UserPromptPart{Content: "Hello"},
				},
				Timestamp: time.Now(),
			},
		},
		Usage: gollem.RunUsage{
			Requests: 1,
		},
		RunID:     "run-123",
		Timestamp: time.Now(),
		Metadata:  map[string]any{"key": "value"},
	}

	ctx := context.Background()
	if err := store.Save(ctx, cp); err != nil {
		t.Fatalf("Save: %v", err)
	}

	loaded, err := store.Load(ctx, "run-123")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	if loaded.RunID != "run-123" {
		t.Errorf("RunID = %q, want %q", loaded.RunID, "run-123")
	}
	if loaded.Usage.Requests != 1 {
		t.Errorf("Requests = %d, want 1", loaded.Usage.Requests)
	}
}

func TestCheckpoint_LoadNotFound(t *testing.T) {
	dir := t.TempDir()
	store, err := NewFileCheckpointStore(dir)
	if err != nil {
		t.Fatalf("NewFileCheckpointStore: %v", err)
	}

	_, err = store.Load(context.Background(), "nonexistent")
	if err == nil {
		t.Fatal("expected error for nonexistent checkpoint")
	}
}

func TestCheckpoint_List(t *testing.T) {
	dir := t.TempDir()
	store, err := NewFileCheckpointStore(dir)
	if err != nil {
		t.Fatalf("NewFileCheckpointStore: %v", err)
	}

	ctx := context.Background()

	// Save multiple checkpoints.
	for _, id := range []string{"run-1", "run-2", "run-3"} {
		cp := &Checkpoint{
			RunID:     id,
			Timestamp: time.Now(),
		}
		if err := store.Save(ctx, cp); err != nil {
			t.Fatalf("Save %s: %v", id, err)
		}
	}

	list, err := store.List(ctx)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(list) != 3 {
		t.Errorf("expected 3 checkpoints, got %d", len(list))
	}
}

func TestCheckpoint_Delete(t *testing.T) {
	dir := t.TempDir()
	store, err := NewFileCheckpointStore(dir)
	if err != nil {
		t.Fatalf("NewFileCheckpointStore: %v", err)
	}

	ctx := context.Background()
	cp := &Checkpoint{RunID: "run-del", Timestamp: time.Now()}
	if err := store.Save(ctx, cp); err != nil {
		t.Fatalf("Save: %v", err)
	}

	if err := store.Delete(ctx, "run-del"); err != nil {
		t.Fatalf("Delete: %v", err)
	}

	_, err = store.Load(ctx, "run-del")
	if err == nil {
		t.Fatal("expected error after delete")
	}
}

func TestCheckpoint_DeleteNonexistent(t *testing.T) {
	dir := t.TempDir()
	store, err := NewFileCheckpointStore(dir)
	if err != nil {
		t.Fatalf("NewFileCheckpointStore: %v", err)
	}

	// Should not error.
	if err := store.Delete(context.Background(), "nonexistent"); err != nil {
		t.Fatalf("Delete nonexistent: %v", err)
	}
}

func TestCheckpoint_DefaultDir(t *testing.T) {
	store, err := NewFileCheckpointStore("")
	if err != nil {
		t.Fatalf("NewFileCheckpointStore with empty dir: %v", err)
	}

	ctx := context.Background()
	cp := &Checkpoint{RunID: "run-default", Timestamp: time.Now()}
	if err := store.Save(ctx, cp); err != nil {
		t.Fatalf("Save: %v", err)
	}

	loaded, err := store.Load(ctx, "run-default")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if loaded.RunID != "run-default" {
		t.Errorf("RunID = %q, want %q", loaded.RunID, "run-default")
	}

	_ = store.Delete(ctx, "run-default")
}

func TestResumeFromCheckpoint(t *testing.T) {
	dir := t.TempDir()
	store, err := NewFileCheckpointStore(dir)
	if err != nil {
		t.Fatalf("NewFileCheckpointStore: %v", err)
	}

	ctx := context.Background()
	cp := &Checkpoint{
		Messages: []gollem.ModelMessage{
			gollem.ModelRequest{
				Parts: []gollem.ModelRequestPart{
					gollem.UserPromptPart{Content: "Previous conversation"},
				},
				Timestamp: time.Now(),
			},
		},
		RunID:     "resume-run",
		Timestamp: time.Now(),
	}
	if err := store.Save(ctx, cp); err != nil {
		t.Fatalf("Save: %v", err)
	}

	opt, err := ResumeFromCheckpoint(store, "resume-run")
	if err != nil {
		t.Fatalf("ResumeFromCheckpoint: %v", err)
	}
	if opt == nil {
		t.Fatal("expected non-nil RunOption")
	}
}

func TestResumeFromCheckpoint_NotFound(t *testing.T) {
	dir := t.TempDir()
	store, err := NewFileCheckpointStore(dir)
	if err != nil {
		t.Fatalf("NewFileCheckpointStore: %v", err)
	}

	_, err = ResumeFromCheckpoint(store, "nonexistent")
	if err == nil {
		t.Fatal("expected error for nonexistent checkpoint")
	}
}
