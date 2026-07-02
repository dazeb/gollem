package store

import (
	"context"
	"encoding/json"
	"errors"
	"path/filepath"
	"testing"
)

func TestSQLiteStoreThreadLifecyclePersists(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "appserver.db")

	s := newTestSQLiteStore(t, path)
	thread, err := s.CreateThread(ctx, CreateThreadRequest{
		Title:     "Build protocol",
		Workspace: "/work",
		Settings:  map[string]any{"model": "claude"},
		Metadata:  map[string]any{"source": "test"},
	})
	if err != nil {
		t.Fatalf("CreateThread: %v", err)
	}
	if thread.ID == "" || thread.Status != ThreadActive {
		t.Fatalf("thread = %+v", thread)
	}

	archived, err := s.ArchiveThread(ctx, thread.ID)
	if err != nil {
		t.Fatalf("ArchiveThread: %v", err)
	}
	if archived.Status != ThreadArchived || archived.ArchivedAt.IsZero() {
		t.Fatalf("archived = %+v", archived)
	}

	if err := s.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}
	s = newTestSQLiteStore(t, path)

	loaded, err := s.GetThread(ctx, thread.ID)
	if err != nil {
		t.Fatalf("GetThread after reopen: %v", err)
	}
	if loaded.Status != ThreadArchived || loaded.Settings["model"] != "claude" {
		t.Fatalf("loaded = %+v", loaded)
	}

	activeOnly, err := s.ListThreads(ctx, ThreadFilter{Statuses: []ThreadStatus{ThreadActive}})
	if err != nil {
		t.Fatalf("ListThreads active: %v", err)
	}
	if len(activeOnly) != 0 {
		t.Fatalf("activeOnly = %+v", activeOnly)
	}

	unarchived, err := s.UnarchiveThread(ctx, thread.ID)
	if err != nil {
		t.Fatalf("UnarchiveThread: %v", err)
	}
	if unarchived.Status != ThreadActive || !unarchived.ArchivedAt.IsZero() {
		t.Fatalf("unarchived = %+v", unarchived)
	}

	deleted, err := s.DeleteThread(ctx, thread.ID)
	if err != nil {
		t.Fatalf("DeleteThread: %v", err)
	}
	if deleted.Status != ThreadDeleted || deleted.DeletedAt.IsZero() {
		t.Fatalf("deleted = %+v", deleted)
	}

	visible, err := s.ListThreads(ctx, ThreadFilter{})
	if err != nil {
		t.Fatalf("ListThreads visible: %v", err)
	}
	if len(visible) != 0 {
		t.Fatalf("deleted thread should be hidden by default: %+v", visible)
	}

	all, err := s.ListThreads(ctx, ThreadFilter{IncludeDeleted: true})
	if err != nil {
		t.Fatalf("ListThreads all: %v", err)
	}
	if len(all) != 1 || all[0].Status != ThreadDeleted {
		t.Fatalf("all = %+v", all)
	}
}

func TestSQLiteStoreTurnsAndItemsPersistAndPaginate(t *testing.T) {
	ctx := context.Background()
	s := newTestSQLiteStore(t, filepath.Join(t.TempDir(), "appserver.db"))

	thread, err := s.CreateThread(ctx, CreateThreadRequest{Title: "Timeline"})
	if err != nil {
		t.Fatalf("CreateThread: %v", err)
	}
	turn, err := s.CreateTurn(ctx, CreateTurnRequest{
		ThreadID: thread.ID,
		Input:    json.RawMessage(`{"prompt":"hello"}`),
		Metadata: map[string]any{"kind": "user"},
	})
	if err != nil {
		t.Fatalf("CreateTurn: %v", err)
	}
	if turn.Status != TurnQueued {
		t.Fatalf("turn status = %s", turn.Status)
	}
	started, err := s.StartTurn(ctx, turn.ID)
	if err != nil {
		t.Fatalf("StartTurn: %v", err)
	}
	if started.Status != TurnRunning || started.StartedAt.IsZero() {
		t.Fatalf("started = %+v", started)
	}
	completed, err := s.CompleteTurn(ctx, CompleteTurnRequest{
		ID:     turn.ID,
		Result: json.RawMessage(`{"answer":"ok"}`),
		Usage:  map[string]any{"tokens": float64(12)},
	})
	if err != nil {
		t.Fatalf("CompleteTurn: %v", err)
	}
	if completed.Status != TurnCompleted || completed.CompletedAt.IsZero() {
		t.Fatalf("completed = %+v", completed)
	}

	first, err := s.AppendItem(ctx, AppendItemRequest{
		ThreadID: thread.ID,
		TurnID:   turn.ID,
		Kind:     "message",
		Status:   "completed",
		Payload:  json.RawMessage(`{"role":"user","text":"hello"}`),
	})
	if err != nil {
		t.Fatalf("AppendItem first: %v", err)
	}
	second, err := s.AppendItem(ctx, AppendItemRequest{
		ThreadID: thread.ID,
		TurnID:   turn.ID,
		Kind:     "message",
		Status:   "completed",
		Payload:  json.RawMessage(`{"role":"assistant","text":"ok"}`),
	})
	if err != nil {
		t.Fatalf("AppendItem second: %v", err)
	}
	if first.Seq == 0 || second.Seq <= first.Seq {
		t.Fatalf("item seqs not increasing: first=%d second=%d", first.Seq, second.Seq)
	}

	items, err := s.ListItems(ctx, ItemFilter{ThreadID: thread.ID, Limit: 1})
	if err != nil {
		t.Fatalf("ListItems first page: %v", err)
	}
	if len(items) != 1 || items[0].ID != first.ID {
		t.Fatalf("first page = %+v", items)
	}
	next, err := s.ListItems(ctx, ItemFilter{ThreadID: thread.ID, AfterSeq: items[0].Seq})
	if err != nil {
		t.Fatalf("ListItems next page: %v", err)
	}
	if len(next) != 1 || next[0].ID != second.ID {
		t.Fatalf("next page = %+v", next)
	}

	turns, err := s.ListTurns(ctx, TurnFilter{ThreadID: thread.ID, Statuses: []TurnStatus{TurnCompleted}})
	if err != nil {
		t.Fatalf("ListTurns: %v", err)
	}
	if len(turns) != 1 || turns[0].ID != turn.ID {
		t.Fatalf("turns = %+v", turns)
	}
}

func TestSQLiteStoreForkCopiesThreadHistory(t *testing.T) {
	ctx := context.Background()
	s := newTestSQLiteStore(t, filepath.Join(t.TempDir(), "appserver.db"))

	source, err := s.CreateThread(ctx, CreateThreadRequest{
		Title:    "Source",
		Settings: map[string]any{"model": "sonnet"},
		Metadata: map[string]any{"root": "yes"},
	})
	if err != nil {
		t.Fatalf("CreateThread: %v", err)
	}
	turn, err := s.CreateTurn(ctx, CreateTurnRequest{ThreadID: source.ID, Input: json.RawMessage(`{"prompt":"fork me"}`)})
	if err != nil {
		t.Fatalf("CreateTurn: %v", err)
	}
	if _, err := s.AppendItem(ctx, AppendItemRequest{ThreadID: source.ID, TurnID: turn.ID, Kind: "message", Payload: json.RawMessage(`{"text":"fork me"}`)}); err != nil {
		t.Fatalf("AppendItem: %v", err)
	}

	fork, err := s.ForkThread(ctx, ForkThreadRequest{
		SourceThreadID: source.ID,
		Title:          "Fork",
		Metadata:       map[string]any{"fork": "yes"},
		IncludeItems:   true,
	})
	if err != nil {
		t.Fatalf("ForkThread: %v", err)
	}
	if fork.ID == source.ID || fork.ForkedFromThreadID != source.ID || fork.Title != "Fork" {
		t.Fatalf("fork = %+v source = %+v", fork, source)
	}
	if fork.Settings["model"] != "sonnet" || fork.Metadata["root"] != "yes" || fork.Metadata["fork"] != "yes" {
		t.Fatalf("fork metadata/settings not copied: %+v", fork)
	}

	forkTurns, err := s.ListTurns(ctx, TurnFilter{ThreadID: fork.ID})
	if err != nil {
		t.Fatalf("ListTurns fork: %v", err)
	}
	if len(forkTurns) != 1 || forkTurns[0].ID == turn.ID || forkTurns[0].ThreadID != fork.ID {
		t.Fatalf("fork turns = %+v", forkTurns)
	}
	forkItems, err := s.ListItems(ctx, ItemFilter{ThreadID: fork.ID})
	if err != nil {
		t.Fatalf("ListItems fork: %v", err)
	}
	if len(forkItems) != 1 || forkItems[0].ThreadID != fork.ID || forkItems[0].TurnID != forkTurns[0].ID {
		t.Fatalf("fork items = %+v turns = %+v", forkItems, forkTurns)
	}
}

func TestSQLiteStoreReturnsCopies(t *testing.T) {
	ctx := context.Background()
	s := newTestSQLiteStore(t, filepath.Join(t.TempDir(), "appserver.db"))

	thread, err := s.CreateThread(ctx, CreateThreadRequest{Settings: map[string]any{"model": "a"}})
	if err != nil {
		t.Fatalf("CreateThread: %v", err)
	}
	thread.Settings["model"] = "mutated"
	loaded, err := s.GetThread(ctx, thread.ID)
	if err != nil {
		t.Fatalf("GetThread: %v", err)
	}
	if loaded.Settings["model"] != "a" {
		t.Fatalf("store leaked thread settings map: %+v", loaded.Settings)
	}

	turn, err := s.CreateTurn(ctx, CreateTurnRequest{ThreadID: loaded.ID, Input: json.RawMessage(`{"prompt":"original"}`)})
	if err != nil {
		t.Fatalf("CreateTurn: %v", err)
	}
	turn.Input[11] = 'X'
	loadedTurn, err := s.GetTurn(ctx, turn.ID)
	if err != nil {
		t.Fatalf("GetTurn: %v", err)
	}
	if string(loadedTurn.Input) != `{"prompt":"original"}` {
		t.Fatalf("store leaked turn raw message: %s", loadedTurn.Input)
	}
}

func TestSQLiteStoreRejectsDeletedThreadMutations(t *testing.T) {
	ctx := context.Background()
	s := newTestSQLiteStore(t, filepath.Join(t.TempDir(), "appserver.db"))

	thread, err := s.CreateThread(ctx, CreateThreadRequest{})
	if err != nil {
		t.Fatalf("CreateThread: %v", err)
	}
	if _, err := s.DeleteThread(ctx, thread.ID); err != nil {
		t.Fatalf("DeleteThread: %v", err)
	}
	if _, err := s.ArchiveThread(ctx, thread.ID); !errors.Is(err, ErrThreadDeleted) {
		t.Fatalf("ArchiveThread on deleted thread error = %v, want ErrThreadDeleted", err)
	}
	if _, err := s.UnarchiveThread(ctx, thread.ID); !errors.Is(err, ErrThreadDeleted) {
		t.Fatalf("UnarchiveThread on deleted thread error = %v, want ErrThreadDeleted", err)
	}
	if _, err := s.CreateTurn(ctx, CreateTurnRequest{ThreadID: thread.ID}); !errors.Is(err, ErrThreadDeleted) {
		t.Fatalf("CreateTurn on deleted thread error = %v, want ErrThreadDeleted", err)
	}
	if _, err := s.ForkThread(ctx, ForkThreadRequest{SourceThreadID: thread.ID}); !errors.Is(err, ErrThreadDeleted) {
		t.Fatalf("ForkThread on deleted thread error = %v, want ErrThreadDeleted", err)
	}
}

func TestSQLiteStoreRejectsCrossThreadItemTurn(t *testing.T) {
	ctx := context.Background()
	s := newTestSQLiteStore(t, filepath.Join(t.TempDir(), "appserver.db"))

	threadA, err := s.CreateThread(ctx, CreateThreadRequest{})
	if err != nil {
		t.Fatalf("CreateThread A: %v", err)
	}
	threadB, err := s.CreateThread(ctx, CreateThreadRequest{})
	if err != nil {
		t.Fatalf("CreateThread B: %v", err)
	}
	turnA, err := s.CreateTurn(ctx, CreateTurnRequest{ThreadID: threadA.ID})
	if err != nil {
		t.Fatalf("CreateTurn: %v", err)
	}

	_, err = s.AppendItem(ctx, AppendItemRequest{ThreadID: threadB.ID, TurnID: turnA.ID, Kind: "message"})
	if !errors.Is(err, ErrTurnNotFound) {
		t.Fatalf("AppendItem cross-thread error = %v, want ErrTurnNotFound", err)
	}
}

func TestSQLiteStoreRejectsStartingExistingTurnAfterThreadDeleted(t *testing.T) {
	ctx := context.Background()
	s := newTestSQLiteStore(t, filepath.Join(t.TempDir(), "appserver.db"))

	thread, err := s.CreateThread(ctx, CreateThreadRequest{})
	if err != nil {
		t.Fatalf("CreateThread: %v", err)
	}
	turn, err := s.CreateTurn(ctx, CreateTurnRequest{ThreadID: thread.ID})
	if err != nil {
		t.Fatalf("CreateTurn: %v", err)
	}
	if _, err := s.DeleteThread(ctx, thread.ID); err != nil {
		t.Fatalf("DeleteThread: %v", err)
	}
	if _, err := s.StartTurn(ctx, turn.ID); !errors.Is(err, ErrThreadDeleted) {
		t.Fatalf("StartTurn after delete error = %v, want ErrThreadDeleted", err)
	}
	if _, err := s.CompleteTurn(ctx, CompleteTurnRequest{ID: turn.ID}); !errors.Is(err, ErrThreadDeleted) {
		t.Fatalf("CompleteTurn after delete error = %v, want ErrThreadDeleted", err)
	}
}

func newTestSQLiteStore(t *testing.T, path string) *SQLiteStore {
	t.Helper()
	s, err := NewSQLiteStore(path)
	if err != nil {
		t.Fatalf("NewSQLiteStore: %v", err)
	}
	t.Cleanup(func() {
		if err := s.Close(); err != nil {
			t.Fatalf("Close: %v", err)
		}
	})
	return s
}
