package appserver

import (
	"context"
	"encoding/json"
	"path/filepath"
	"testing"

	"github.com/fugue-labs/gollem/appserver/protocol"
	"github.com/fugue-labs/gollem/appserver/store"
)

func TestServerThreadLifecycleNotificationsUseExportedContracts(t *testing.T) {
	ctx := context.Background()
	st, err := store.NewSQLiteStore(filepath.Join(t.TempDir(), "threads.db"))
	if err != nil {
		t.Fatalf("NewSQLiteStore: %v", err)
	}
	t.Cleanup(func() { _ = st.Close() })
	thread, err := st.CreateThread(ctx, store.CreateThreadRequest{Title: "Lifecycle notifications"})
	if err != nil {
		t.Fatalf("CreateThread: %v", err)
	}
	server := readyServer(WithStore(st))

	archive := server.HandleRequest(ctx, request("thread/archive", map[string]any{"threadId": thread.ID}))
	if archive.Error != nil {
		t.Fatalf("thread/archive error: %v", archive.Error)
	}
	events := server.DrainNotifications()
	assertNotificationMethods(t, events, "thread/status/changed", "thread/archived")
	var archived protocol.ThreadArchivedNotification
	if err := json.Unmarshal(events[1].Params, &archived); err != nil {
		t.Fatalf("decode archived notification: %v", err)
	}
	if archived.ThreadID != thread.ID || archived.Status != protocol.ThreadLifecycleArchived ||
		archived.Thread == nil || archived.Thread.Status != protocol.ThreadLifecycleArchived || archived.At == nil {
		t.Fatalf("archived notification = %#v", archived)
	}

	unarchive := server.HandleRequest(ctx, request("thread/unarchive", map[string]any{"threadId": thread.ID}))
	if unarchive.Error != nil {
		t.Fatalf("thread/unarchive error: %v", unarchive.Error)
	}
	events = server.DrainNotifications()
	assertNotificationMethods(t, events, "thread/status/changed", "thread/unarchived")
	var unarchived protocol.ThreadUnarchivedNotification
	if err := json.Unmarshal(events[1].Params, &unarchived); err != nil {
		t.Fatalf("decode unarchived notification: %v", err)
	}
	if unarchived.ThreadID != thread.ID || unarchived.Status != protocol.ThreadLifecycleActive ||
		unarchived.Thread == nil || unarchived.Thread.Status != protocol.ThreadLifecycleActive || unarchived.At == nil {
		t.Fatalf("unarchived notification = %#v", unarchived)
	}

	deletedResponse := server.HandleRequest(ctx, request("thread/delete", map[string]any{"threadId": thread.ID}))
	if deletedResponse.Error != nil {
		t.Fatalf("thread/delete error: %v", deletedResponse.Error)
	}
	events = server.DrainNotifications()
	assertNotificationMethods(t, events, "thread/status/changed", "thread/deleted")
	var deleted protocol.ThreadDeletedNotification
	if err := json.Unmarshal(events[1].Params, &deleted); err != nil {
		t.Fatalf("decode deleted notification: %v", err)
	}
	if deleted.ThreadID != thread.ID || deleted.Status != protocol.ThreadLifecycleDeleted ||
		deleted.Thread == nil || deleted.Thread.Status != protocol.ThreadLifecycleDeleted || deleted.At == nil {
		t.Fatalf("deleted notification = %#v", deleted)
	}
}
