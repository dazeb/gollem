package appserver

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/fugue-labs/gollem/appserver/protocol"
	"github.com/fugue-labs/gollem/appserver/store"
)

func TestThreadLifecycleControlsUseExportedContracts(t *testing.T) {
	ctx := context.Background()
	st, err := store.NewSQLiteStore(filepath.Join(t.TempDir(), "threads.db"))
	if err != nil {
		t.Fatalf("NewSQLiteStore: %v", err)
	}
	t.Cleanup(func() { _ = st.Close() })
	thread, err := st.CreateThread(ctx, store.CreateThreadRequest{Title: "Lifecycle"})
	if err != nil {
		t.Fatalf("CreateThread: %v", err)
	}
	server := readyServer(WithStore(st))

	archive := server.HandleRequest(ctx, request("thread/archive", map[string]any{"threadId": thread.ID}))
	if archive.Error != nil {
		t.Fatalf("thread/archive error: %v", archive.Error)
	}
	var archived protocol.ThreadArchiveResponse
	decodeResult(t, archive, &archived)
	if archived.Thread == nil || archived.Thread.Status != protocol.ThreadLifecycleArchived {
		t.Fatalf("thread/archive = %#v", archived)
	}

	unarchive := server.HandleRequest(ctx, request("thread/unarchive", map[string]any{"threadId": thread.ID}))
	if unarchive.Error != nil {
		t.Fatalf("thread/unarchive error: %v", unarchive.Error)
	}
	var unarchived protocol.ThreadUnarchiveResult
	decodeResult(t, unarchive, &unarchived)
	if unarchived.Thread.ID != thread.ID || unarchived.Thread.Status != protocol.ThreadLifecycleActive {
		t.Fatalf("thread/unarchive = %#v", unarchived)
	}

	deletedResponse := server.HandleRequest(ctx, request("thread/delete", map[string]any{"threadId": thread.ID}))
	if deletedResponse.Error != nil {
		t.Fatalf("thread/delete error: %v", deletedResponse.Error)
	}
	var deleted protocol.ThreadDeleteResponse
	decodeResult(t, deletedResponse, &deleted)
	if deleted.Thread == nil || deleted.Thread.Status != protocol.ThreadLifecycleDeleted {
		t.Fatalf("thread/delete = %#v", deleted)
	}

	for _, method := range []string{"thread/archive", "thread/unarchive", "thread/delete"} {
		response := server.HandleRequest(ctx, request(method, nil))
		if response.Error == nil || response.Error.Code != protocol.CodeInvalidParams {
			t.Errorf("%s missing threadId error = %#v", method, response.Error)
		}
	}
}

func TestThreadLifecycleControlsRejectArchiveAndDeleteDuringActiveTurn(t *testing.T) {
	ctx := context.Background()
	st := newRuntimeTestStore(t)
	model := &blockingRuntimeModel{started: make(chan struct{})}
	server := readyServer(
		WithStore(st),
		WithRuntimeService(NewRuntimeService(WithRuntimeModel(
			model,
			RuntimeModelInfo{ProviderID: "test", Model: "blocking"},
		))),
	)

	start := server.HandleRequest(ctx, request("thread/start", map[string]any{
		"prompt": "keep running",
		"title":  "Lifecycle guard",
	}))
	if start.Error != nil {
		t.Fatalf("thread/start error: %v", start.Error)
	}
	var started protocol.ThreadRunStartResult
	decodeResult(t, start, &started)
	waitForBlockingModel(t, model)

	for method, wantMessage := range map[string]string{
		"thread/archive": "cannot archive a thread while a turn is active",
		"thread/delete":  "cannot delete a thread while a turn is active",
	} {
		response := server.HandleRequest(ctx, request(method, map[string]any{
			"threadId": started.Thread.ID,
		}))
		if response.Error == nil ||
			response.Error.Code != protocol.CodeInvalidRequest ||
			response.Error.Message != wantMessage {
			t.Fatalf("%s active-turn error = %#v", method, response.Error)
		}
	}
	unchanged, err := st.GetThread(ctx, started.Thread.ID)
	if err != nil {
		t.Fatalf("GetThread after rejected lifecycle controls: %v", err)
	}
	if unchanged.Status != store.ThreadActive {
		t.Fatalf("thread status after rejected lifecycle controls = %q", unchanged.Status)
	}

	rename := server.HandleRequest(ctx, request("thread/name/set", map[string]any{
		"threadId": started.Thread.ID,
		"name":     "Still running",
	}))
	if rename.Error != nil {
		t.Fatalf("thread/name/set during active turn error: %v", rename.Error)
	}

	interrupt := server.HandleRequest(ctx, request("turn/interrupt", map[string]any{
		"threadId": started.Thread.ID,
		"turnId":   started.Turn.ID,
	}))
	if interrupt.Error != nil {
		t.Fatalf("turn/interrupt error: %v", interrupt.Error)
	}
	waitForNotificationSet(t, server, "turn/completed")

	archive := server.HandleRequest(ctx, request("thread/archive", map[string]any{
		"threadId": started.Thread.ID,
	}))
	if archive.Error != nil {
		t.Fatalf("thread/archive after terminal turn error: %v", archive.Error)
	}
}
