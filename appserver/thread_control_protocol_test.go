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
	var unarchived protocol.ThreadUnarchiveResponse
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
