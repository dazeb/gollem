package appserver

import (
	"context"
	"encoding/json"
	"path/filepath"
	"testing"

	"github.com/fugue-labs/gollem/appserver/protocol"
	"github.com/fugue-labs/gollem/appserver/store"
)

func TestServerThreadMetadataPublicContracts(t *testing.T) {
	ctx := context.Background()
	st, err := store.NewSQLiteStore(filepath.Join(t.TempDir(), "threads.db"))
	if err != nil {
		t.Fatalf("NewSQLiteStore: %v", err)
	}
	t.Cleanup(func() { _ = st.Close() })
	thread, err := st.CreateThread(ctx, store.CreateThreadRequest{
		Title:    "Metadata controls",
		Settings: map[string]any{threadMemoryModeSettingKey: "enabled"},
		Metadata: map[string]any{
			"source": "initial",
			"gitInfo": map[string]any{
				"sha":       "old-sha",
				"branch":    "main",
				"originUrl": "https://example.test/repo.git",
				"provider":  "preserved-extension",
			},
		},
	})
	if err != nil {
		t.Fatalf("CreateThread: %v", err)
	}
	server := readyServer(WithStore(st))

	updateResp := server.HandleRequest(ctx, request("thread/metadata/update", map[string]any{
		"threadId": thread.ID,
		"gitInfo": map[string]any{
			"sha":    " new-sha ",
			"branch": nil,
		},
	}))
	if updateResp.Error != nil {
		t.Fatalf("thread/metadata/update error: %v", updateResp.Error)
	}
	var updated protocol.ThreadMetadataUpdateResult
	decodeResult(t, updateResp, &updated)
	gitInfo := nestedMetadataMap(t, updated.Metadata, "gitInfo")
	if gitInfo["sha"] != "new-sha" || gitInfo["branch"] != nil ||
		gitInfo["originUrl"] != "https://example.test/repo.git" ||
		gitInfo["provider"] != "preserved-extension" || updated.Metadata["source"] != "initial" {
		t.Fatalf("patched metadata = %#v", updated.Metadata)
	}
	if updated.Thread.Settings[threadMemoryModeSettingKey] != "enabled" {
		t.Fatalf("settings after metadata patch = %#v", updated.Thread.Settings)
	}
	assertNotificationMethods(t, server.DrainNotifications(), "thread/settings/updated")

	clearResp := server.HandleRequest(ctx, request("thread/metadata/update", map[string]any{
		"id": thread.ID,
		"gitInfo": map[string]any{
			"originUrl": nil,
		},
	}))
	if clearResp.Error != nil {
		t.Fatalf("thread/metadata/update clear error: %v", clearResp.Error)
	}
	updated = protocol.ThreadMetadataUpdateResult{}
	decodeResult(t, clearResp, &updated)
	gitInfo = nestedMetadataMap(t, updated.Metadata, "gitInfo")
	if gitInfo["sha"] != "new-sha" || gitInfo["originUrl"] != nil || gitInfo["provider"] != "preserved-extension" {
		t.Fatalf("metadata after clear = %#v", updated.Metadata)
	}
	_ = server.DrainNotifications()

	legacyResp := server.HandleRequest(ctx, request("thread/metadata/update", map[string]any{
		"id":       thread.ID,
		"metadata": map[string]any{"reviewed": true},
	}))
	if legacyResp.Error != nil {
		t.Fatalf("legacy thread/metadata/update error: %v", legacyResp.Error)
	}
	updated = protocol.ThreadMetadataUpdateResult{}
	decodeResult(t, legacyResp, &updated)
	if updated.Metadata["reviewed"] != true || updated.Metadata["source"] != "initial" {
		t.Fatalf("legacy merged metadata = %#v", updated.Metadata)
	}
	_ = server.DrainNotifications()

	for _, test := range []struct {
		name   string
		params map[string]any
	}{
		{name: "missing patch", params: map[string]any{"threadId": thread.ID}},
		{name: "null git info", params: map[string]any{"threadId": thread.ID, "gitInfo": nil}},
		{name: "empty git info", params: map[string]any{"threadId": thread.ID, "gitInfo": map[string]any{}}},
		{name: "blank sha", params: map[string]any{"threadId": thread.ID, "gitInfo": map[string]any{"sha": "  "}}},
	} {
		t.Run(test.name, func(t *testing.T) {
			resp := server.HandleRequest(ctx, request("thread/metadata/update", test.params))
			if resp.Error == nil || resp.Error.Code != protocol.CodeInvalidParams {
				t.Fatalf("response error = %#v", resp.Error)
			}
		})
	}

	memoryResp := server.HandleRequest(ctx, request("thread/memoryMode/set", map[string]any{
		"threadId": thread.ID,
		"mode":     "disabled",
	}))
	if memoryResp.Error != nil {
		t.Fatalf("thread/memoryMode/set error: %v", memoryResp.Error)
	}
	var memory protocol.ThreadMemoryModeSetResponse
	decodeResult(t, memoryResp, &memory)
	if memory.ThreadID != thread.ID || memory.MemoryMode != protocol.ThreadMemoryModeDisabled ||
		memory.Thread == nil || memory.Thread.Settings[threadMemoryModeSettingKey] != "disabled" {
		t.Fatalf("memory response = %#v", memory)
	}
	_ = server.DrainNotifications()

	legacyMemoryResp := server.HandleRequest(ctx, request("thread/memoryMode/set", map[string]any{
		"id":         thread.ID,
		"memoryMode": "enabled",
	}))
	if legacyMemoryResp.Error != nil {
		t.Fatalf("legacy thread/memoryMode/set error: %v", legacyMemoryResp.Error)
	}
	memory = protocol.ThreadMemoryModeSetResponse{}
	decodeResult(t, legacyMemoryResp, &memory)
	if memory.MemoryMode != protocol.ThreadMemoryModeEnabled {
		t.Fatalf("legacy memory response = %#v", memory)
	}
	_ = server.DrainNotifications()

	nameResp := server.HandleRequest(ctx, request("thread/name/set", map[string]any{
		"threadId": thread.ID,
		"name":     "Renamed metadata controls",
	}))
	if nameResp.Error != nil {
		t.Fatalf("thread/name/set error: %v", nameResp.Error)
	}
	events := server.DrainNotifications()
	assertNotificationMethods(t, events, "thread/name/updated")
	var nameNotice protocol.ThreadNameUpdatedNotification
	if err := json.Unmarshal(events[0].Params, &nameNotice); err != nil {
		t.Fatalf("decode name notification: %v", err)
	}
	if nameNotice.ThreadID != thread.ID || nameNotice.ThreadName == nil ||
		*nameNotice.ThreadName != "Renamed metadata controls" ||
		nameNotice.Name != "Renamed metadata controls" || nameNotice.Thread == nil {
		t.Fatalf("name notification = %#v", nameNotice)
	}
}

func TestServerThreadMetadataGitPatchSurvivesRestart(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "threads.db")
	st, err := store.NewSQLiteStore(path)
	if err != nil {
		t.Fatalf("NewSQLiteStore: %v", err)
	}
	thread, err := st.CreateThread(ctx, store.CreateThreadRequest{
		Title:    "Restart metadata",
		Metadata: map[string]any{"source": "restart"},
	})
	if err != nil {
		t.Fatalf("CreateThread: %v", err)
	}
	server := readyServer(WithStore(st))
	response := server.HandleRequest(ctx, request("thread/metadata/update", map[string]any{
		"threadId": thread.ID,
		"gitInfo": map[string]any{
			"sha":       "restart-sha",
			"branch":    "restart-branch",
			"originUrl": nil,
		},
	}))
	if response.Error != nil {
		t.Fatalf("thread/metadata/update error: %v", response.Error)
	}
	if err := st.Close(); err != nil {
		t.Fatalf("close first store: %v", err)
	}

	reopened, err := store.NewSQLiteStore(path)
	if err != nil {
		t.Fatalf("reopen SQLite store: %v", err)
	}
	t.Cleanup(func() { _ = reopened.Close() })
	loaded, err := reopened.GetThread(ctx, thread.ID)
	if err != nil {
		t.Fatalf("GetThread after restart: %v", err)
	}
	gitInfo := nestedMetadataMap(t, loaded.Metadata, threadGitInfoMetadataKey)
	if gitInfo["sha"] != "restart-sha" || gitInfo["branch"] != "restart-branch" || gitInfo["originUrl"] != nil || loaded.Metadata["source"] != "restart" {
		t.Fatalf("metadata after restart = %#v", loaded.Metadata)
	}
}

func nestedMetadataMap(t *testing.T, metadata map[string]any, key string) map[string]any {
	t.Helper()
	value, ok := metadata[key]
	if !ok {
		t.Fatalf("metadata missing %q: %#v", key, metadata)
	}
	result, ok := value.(map[string]any)
	if !ok {
		t.Fatalf("metadata[%q] = %#v, want object", key, value)
	}
	return result
}
