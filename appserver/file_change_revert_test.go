package appserver

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/fugue-labs/gollem/appserver/protocol"
	"github.com/fugue-labs/gollem/appserver/store"
	toolfs "github.com/fugue-labs/gollem/appserver/tools/fs"
	"github.com/fugue-labs/gollem/core"
)

func TestFileChangeRevertSurvivesRestartUsesApprovalAndIsIdempotent(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	path := filepath.Join(root, "notes.txt")
	if err := os.WriteFile(path, []byte("before\n"), 0o600); err != nil {
		t.Fatalf("WriteFile before: %v", err)
	}
	dirtyPath := filepath.Join(root, "unrelated.txt")
	if err := os.WriteFile(dirtyPath, []byte("keep me dirty\n"), 0o644); err != nil {
		t.Fatalf("WriteFile unrelated: %v", err)
	}
	storePath := filepath.Join(t.TempDir(), "appserver.db")
	st := newTestFileChangeRevertStore(t, storePath)
	approvals := NewApprovalService()
	fsService, err := toolfs.NewService(root, toolfs.WithApproval(approvals.FilesystemApproval))
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}
	model := core.NewTestModel(
		core.ToolCallResponseWithID("workspace_write_file", `{"path":"notes.txt","content":"after\n"}`, "call-update"),
		core.TextResponse("updated"),
	)
	server := readyServer(
		WithStore(st),
		WithFilesystem(fsService),
		WithApprovalService(approvals),
		WithRuntimeService(NewRuntimeService(
			WithRuntimeModel(model, RuntimeModelInfo{ProviderID: "test", Model: "test-model"}),
			WithRuntimeTools(FilesystemRuntimeTools(fsService)...),
		)),
	)
	start := server.HandleRequest(ctx, request("thread/start", map[string]any{
		"workspace": root,
		"prompt":    "update notes",
	}))
	if start.Error != nil {
		t.Fatalf("thread/start error: %v", start.Error)
	}
	var started protocol.ThreadRunStartResult
	decodeResult(t, start, &started)
	approveServerRequest(t, server, "item/fileChange/requestApproval", func(params protocol.FileChangeApprovalRequestParams) {
		if params.ThreadID != started.Thread.ID || params.TurnID != started.Turn.ID || params.Operation != string(toolfs.OperationWriteFile) {
			t.Fatalf("write approval params = %+v", params)
		}
	})
	waitForNotificationSet(t, server, "turn/completed")
	items, err := st.ListItems(ctx, store.ItemFilter{ThreadID: started.Thread.ID, TurnID: started.Turn.ID})
	if err != nil {
		t.Fatalf("ListItems: %v", err)
	}
	fileItem := findRuntimeFileChangeItem(t, items, "notes.txt")
	if !fileItem.Payload.Evidence[0].RevertSnapshotAvailable {
		t.Fatalf("file item evidence = %+v", fileItem.Payload.Evidence)
	}
	if err := fsService.Close(); err != nil {
		t.Fatalf("close filesystem: %v", err)
	}
	if err := st.Close(); err != nil {
		t.Fatalf("close store: %v", err)
	}

	st = newTestFileChangeRevertStore(t, storePath)
	approvals = NewApprovalService()
	fsService, err = toolfs.NewService(root, toolfs.WithApproval(approvals.FilesystemApproval))
	if err != nil {
		t.Fatalf("NewService after restart: %v", err)
	}
	defer fsService.Close()
	server = readyServer(WithStore(st), WithFilesystem(fsService), WithApprovalService(approvals))
	deniedCh := make(chan protocol.Response, 1)
	go func() {
		deniedCh <- server.HandleRequest(ctx, request(fileChangeRevertMethod, map[string]any{
			"threadId":       started.Thread.ID,
			"itemId":         fileItem.Item.ID,
			"idempotencyKey": "revert-denied",
		}))
	}()
	denyServerRequest(t, server, "item/fileChange/requestApproval", func(params protocol.FileChangeApprovalRequestParams) {
		if params.Operation != string(toolfs.OperationRevertFileChange) || params.ItemID != fileItem.Item.ID {
			t.Fatalf("denied revert approval params = %+v", params)
		}
	})
	select {
	case denied := <-deniedCh:
		if denied.Error == nil || denied.Error.Code != protocol.CodeInvalidRequest {
			t.Fatalf("denied revert response = %+v", denied)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("timed out waiting for denied file-change revert response")
	}
	recoveryAfterDenial, err := st.GetFileChangeRecovery(ctx, fileItem.Item.ID)
	if err != nil {
		t.Fatalf("GetFileChangeRecovery after denial: %v", err)
	}
	if recoveryAfterDenial.Status != store.FileChangeRecoveryAvailable || recoveryAfterDenial.IdempotencyKey != "" {
		t.Fatalf("recovery after denial = %+v", recoveryAfterDenial)
	}

	responseCh := make(chan protocol.Response, 1)
	go func() {
		responseCh <- server.HandleRequest(ctx, request(fileChangeRevertMethod, map[string]any{
			"threadId":       started.Thread.ID,
			"itemId":         fileItem.Item.ID,
			"idempotencyKey": "revert-restart-1",
		}))
	}()
	approveServerRequest(t, server, "item/fileChange/requestApproval", func(params protocol.FileChangeApprovalRequestParams) {
		if params.ThreadID != started.Thread.ID || params.TurnID != started.Turn.ID ||
			params.ItemID != fileItem.Item.ID || params.Operation != string(toolfs.OperationRevertFileChange) ||
			params.Path != "notes.txt" || !params.Destructive {
			t.Fatalf("revert approval params = %+v", params)
		}
	})
	var response protocol.Response
	select {
	case response = <-responseCh:
	case <-time.After(3 * time.Second):
		t.Fatal("timed out waiting for file-change revert response")
	}
	if response.Error != nil {
		t.Fatalf("file-change revert error: %v", response.Error)
	}
	var reverted protocol.FileChangeRevertResult
	decodeResult(t, response, &reverted)
	if reverted.ThreadID != started.Thread.ID || reverted.TurnID != started.Turn.ID ||
		reverted.ItemID != fileItem.Item.ID || reverted.IdempotencyKey != "revert-restart-1" ||
		reverted.Path != "notes.txt" || reverted.Reused || reverted.Marker.Kind != "fileChangeRevert" {
		t.Fatalf("revert result = %+v", reverted)
	}
	var sawChanged, sawReceipt bool
	for _, notification := range server.DrainNotifications() {
		sawChanged = sawChanged || notification.Method == "fs/changed"
		sawReceipt = sawReceipt || notification.Method == "item/completed"
	}
	if !sawChanged || !sawReceipt {
		t.Fatalf("revert notifications changed=%v receipt=%v", sawChanged, sawReceipt)
	}
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile reverted: %v", err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("Stat reverted: %v", err)
	}
	dirty, err := os.ReadFile(dirtyPath)
	if err != nil {
		t.Fatalf("ReadFile unrelated: %v", err)
	}
	if string(content) != "before\n" || info.Mode().Perm() != 0o600 || string(dirty) != "keep me dirty\n" {
		t.Fatalf("reverted/dirty state = %q mode=%o dirty=%q", content, info.Mode().Perm(), dirty)
	}

	duplicate := server.HandleRequest(ctx, request(fileChangeRevertMethod, map[string]any{
		"threadId":       started.Thread.ID,
		"itemId":         fileItem.Item.ID,
		"idempotencyKey": "revert-restart-1",
	}))
	if duplicate.Error != nil {
		t.Fatalf("duplicate revert error: %v", duplicate.Error)
	}
	var reused protocol.FileChangeRevertResult
	decodeResult(t, duplicate, &reused)
	if !reused.Reused || reused.Marker.ID != reverted.Marker.ID {
		t.Fatalf("duplicate result = %+v", reused)
	}
	conflict := server.HandleRequest(ctx, request(fileChangeRevertMethod, map[string]any{
		"threadId":       started.Thread.ID,
		"itemId":         fileItem.Item.ID,
		"idempotencyKey": "revert-restart-2",
	}))
	if conflict.Error == nil || conflict.Error.Code != protocol.CodeInvalidParams {
		t.Fatalf("conflicting revert response = %+v", conflict)
	}
	malformed := server.HandleRequest(ctx, request(fileChangeRevertMethod, map[string]any{
		"threadId":       started.Thread.ID,
		"itemId":         fileItem.Item.ID,
		"idempotencyKey": "revert-restart-1",
		"path":           "caller-supplied.txt",
	}))
	if malformed.Error == nil || malformed.Error.Code != protocol.CodeInvalidParams {
		t.Fatalf("malformed revert response = %+v", malformed)
	}
}

func TestFileChangeRevertRejectsStaleSymlinkAndTamperedEvidence(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*testing.T, string, *store.SQLiteStore, storedRuntimeFileChangeItem)
	}{
		{
			name: "concurrent edit",
			mutate: func(t *testing.T, root string, _ *store.SQLiteStore, _ storedRuntimeFileChangeItem) {
				t.Helper()
				if err := os.WriteFile(filepath.Join(root, "notes.txt"), []byte("user edit\n"), 0o644); err != nil {
					t.Fatalf("WriteFile concurrent edit: %v", err)
				}
			},
		},
		{
			name: "symlink replacement",
			mutate: func(t *testing.T, root string, _ *store.SQLiteStore, _ storedRuntimeFileChangeItem) {
				t.Helper()
				if err := os.WriteFile(filepath.Join(root, "target.txt"), []byte("after\n"), 0o644); err != nil {
					t.Fatalf("WriteFile target: %v", err)
				}
				if err := os.Remove(filepath.Join(root, "notes.txt")); err != nil {
					t.Fatalf("Remove notes: %v", err)
				}
				if err := os.Symlink("target.txt", filepath.Join(root, "notes.txt")); err != nil {
					t.Fatalf("Symlink notes: %v", err)
				}
			},
		},
		{
			name: "mode edit",
			mutate: func(t *testing.T, root string, _ *store.SQLiteStore, _ storedRuntimeFileChangeItem) {
				t.Helper()
				if err := os.Chmod(filepath.Join(root, "notes.txt"), 0o600); err != nil {
					t.Fatalf("Chmod concurrent edit: %v", err)
				}
			},
		},
		{
			name: "tampered patch evidence",
			mutate: func(t *testing.T, _ string, st *store.SQLiteStore, item storedRuntimeFileChangeItem) {
				t.Helper()
				item.Payload.Evidence[0].AfterSHA256 = "tampered"
				if _, err := st.UpdateItem(context.Background(), store.UpdateItemRequest{
					ID:      item.Item.ID,
					Status:  item.Item.Status,
					Payload: mustRuntimeJSON(item.Payload),
				}); err != nil {
					t.Fatalf("UpdateItem tampered: %v", err)
				}
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			ctx := context.Background()
			root := t.TempDir()
			if err := os.WriteFile(filepath.Join(root, "notes.txt"), []byte("before\n"), 0o644); err != nil {
				t.Fatalf("WriteFile before: %v", err)
			}
			st := newRuntimeTestStore(t)
			fsService, err := toolfs.NewService(root)
			if err != nil {
				t.Fatalf("NewService: %v", err)
			}
			defer fsService.Close()
			model := core.NewTestModel(
				core.ToolCallResponseWithID("workspace_write_file", `{"path":"notes.txt","content":"after\n"}`, "call-update"),
				core.TextResponse("updated"),
			)
			server := readyServer(
				WithStore(st),
				WithFilesystem(fsService),
				WithRuntimeService(NewRuntimeService(
					WithRuntimeModel(model, RuntimeModelInfo{ProviderID: "test", Model: "test-model"}),
					WithRuntimeTools(FilesystemRuntimeTools(fsService)...),
				)),
			)
			start := server.HandleRequest(ctx, request("thread/start", map[string]any{"workspace": root, "prompt": "update"}))
			if start.Error != nil {
				t.Fatalf("thread/start error: %v", start.Error)
			}
			var started protocol.ThreadRunStartResult
			decodeResult(t, start, &started)
			waitForNotificationSet(t, server, "turn/completed")
			items, err := st.ListItems(ctx, store.ItemFilter{ThreadID: started.Thread.ID, TurnID: started.Turn.ID})
			if err != nil {
				t.Fatalf("ListItems: %v", err)
			}
			fileItem := findRuntimeFileChangeItem(t, items, "notes.txt")
			test.mutate(t, root, st, fileItem)

			response := server.HandleRequest(ctx, request(fileChangeRevertMethod, map[string]any{
				"threadId":       started.Thread.ID,
				"itemId":         fileItem.Item.ID,
				"idempotencyKey": "stale-1",
			}))
			if response.Error == nil || response.Error.Code != protocol.CodeInvalidParams {
				t.Fatalf("stale revert response = %+v", response)
			}
			if test.name == "symlink replacement" {
				target, err := os.ReadFile(filepath.Join(root, "target.txt"))
				if err != nil || string(target) != "after\n" {
					t.Fatalf("symlink target = %q, error %v", target, err)
				}
			}
		})
	}
}

func TestFileChangeRevertCompletesPendingReceiptAfterResponseLoss(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "notes.txt"), []byte("before\n"), 0o644); err != nil {
		t.Fatalf("WriteFile before: %v", err)
	}
	st := newRuntimeTestStore(t)
	fsService, err := toolfs.NewService(root)
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}
	defer fsService.Close()
	model := core.NewTestModel(
		core.ToolCallResponseWithID("workspace_write_file", `{"path":"notes.txt","content":"after\n"}`, "call-update"),
		core.TextResponse("updated"),
	)
	server := readyServer(
		WithStore(st),
		WithFilesystem(fsService),
		WithRuntimeService(NewRuntimeService(
			WithRuntimeModel(model, RuntimeModelInfo{ProviderID: "test", Model: "test-model"}),
			WithRuntimeTools(FilesystemRuntimeTools(fsService)...),
		)),
	)
	start := server.HandleRequest(ctx, request("thread/start", map[string]any{"workspace": root, "prompt": "update"}))
	if start.Error != nil {
		t.Fatalf("thread/start error: %v", start.Error)
	}
	var started protocol.ThreadRunStartResult
	decodeResult(t, start, &started)
	waitForNotificationSet(t, server, "turn/completed")
	items, err := st.ListItems(ctx, store.ItemFilter{ThreadID: started.Thread.ID, TurnID: started.Turn.ID})
	if err != nil {
		t.Fatalf("ListItems: %v", err)
	}
	fileItem := findRuntimeFileChangeItem(t, items, "notes.txt")
	recovery, err := st.GetFileChangeRecovery(ctx, fileItem.Item.ID)
	if err != nil {
		t.Fatalf("GetFileChangeRecovery: %v", err)
	}
	active, err := st.CreateTurn(ctx, store.CreateTurnRequest{ThreadID: started.Thread.ID})
	if err != nil {
		t.Fatalf("CreateTurn active: %v", err)
	}
	blocked := server.HandleRequest(ctx, request(fileChangeRevertMethod, map[string]any{
		"threadId":       started.Thread.ID,
		"itemId":         fileItem.Item.ID,
		"idempotencyKey": "lost-response-1",
	}))
	if blocked.Error == nil || blocked.Error.Code != protocol.CodeInvalidRequest {
		t.Fatalf("active-turn revert response = %+v", blocked)
	}
	if _, err := st.CompleteTurn(ctx, store.CompleteTurnRequest{ID: active.ID, Status: store.TurnInterrupted}); err != nil {
		t.Fatalf("CompleteTurn active: %v", err)
	}
	if _, err := st.PrepareFileChangeRevert(ctx, store.PrepareFileChangeRevertRequest{
		ItemID:         fileItem.Item.ID,
		IdempotencyKey: "lost-response-1",
	}); err != nil {
		t.Fatalf("PrepareFileChangeRevert: %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, "notes.txt"), recovery.BeforeContent, os.FileMode(recovery.BeforeMode)); err != nil {
		t.Fatalf("simulate completed filesystem mutation: %v", err)
	}

	response := server.HandleRequest(ctx, request(fileChangeRevertMethod, map[string]any{
		"threadId":       started.Thread.ID,
		"itemId":         fileItem.Item.ID,
		"idempotencyKey": "lost-response-1",
	}))
	if response.Error != nil {
		t.Fatalf("recovered revert error: %v", response.Error)
	}
	var result protocol.FileChangeRevertResult
	decodeResult(t, response, &result)
	if !result.Reused || result.Marker.Kind != "fileChangeRevert" {
		t.Fatalf("recovered result = %+v", result)
	}
}

func TestFileChangeRevertHandlesCreateAndDeleteExactly(t *testing.T) {
	tests := []struct {
		name         string
		setup        func(*testing.T, string)
		toolResponse *core.ModelResponse
		assert       func(*testing.T, string)
	}{
		{
			name:  "undo create",
			setup: func(*testing.T, string) {},
			toolResponse: core.ToolCallResponseWithID(
				"workspace_write_file",
				`{"path":"notes.txt","content":"created\n"}`,
				"call-create",
			),
			assert: func(t *testing.T, root string) {
				t.Helper()
				if _, err := os.Stat(filepath.Join(root, "notes.txt")); !errors.Is(err, os.ErrNotExist) {
					t.Fatalf("created file stat after revert = %v, want not-exist", err)
				}
			},
		},
		{
			name: "undo delete",
			setup: func(t *testing.T, root string) {
				t.Helper()
				if err := os.WriteFile(filepath.Join(root, "notes.txt"), []byte("deleted\n"), 0o600); err != nil {
					t.Fatalf("WriteFile delete fixture: %v", err)
				}
			},
			toolResponse: core.ToolCallResponseWithID(
				"workspace_remove_path",
				`{"path":"notes.txt"}`,
				"call-delete",
			),
			assert: func(t *testing.T, root string) {
				t.Helper()
				content, err := os.ReadFile(filepath.Join(root, "notes.txt"))
				if err != nil {
					t.Fatalf("ReadFile restored delete: %v", err)
				}
				info, err := os.Stat(filepath.Join(root, "notes.txt"))
				if err != nil || string(content) != "deleted\n" || info.Mode().Perm() != 0o600 {
					t.Fatalf("restored delete = %q mode=%o error=%v", content, info.Mode().Perm(), err)
				}
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			ctx := context.Background()
			root := t.TempDir()
			test.setup(t, root)
			st := newRuntimeTestStore(t)
			fsService, err := toolfs.NewService(root)
			if err != nil {
				t.Fatalf("NewService: %v", err)
			}
			defer fsService.Close()
			model := core.NewTestModel(test.toolResponse, core.TextResponse("done"))
			server := readyServer(
				WithStore(st),
				WithFilesystem(fsService),
				WithRuntimeService(NewRuntimeService(
					WithRuntimeModel(model, RuntimeModelInfo{ProviderID: "test", Model: "test-model"}),
					WithRuntimeTools(FilesystemRuntimeTools(fsService)...),
				)),
			)
			start := server.HandleRequest(ctx, request("thread/start", map[string]any{"workspace": root, "prompt": "mutate"}))
			if start.Error != nil {
				t.Fatalf("thread/start error: %v", start.Error)
			}
			var started protocol.ThreadRunStartResult
			decodeResult(t, start, &started)
			waitForNotificationSet(t, server, "turn/completed")
			items, err := st.ListItems(ctx, store.ItemFilter{ThreadID: started.Thread.ID, TurnID: started.Turn.ID})
			if err != nil {
				t.Fatalf("ListItems: %v", err)
			}
			fileItem := findRuntimeFileChangeItem(t, items, "notes.txt")
			response := server.HandleRequest(ctx, request(fileChangeRevertMethod, map[string]any{
				"threadId":       started.Thread.ID,
				"itemId":         fileItem.Item.ID,
				"idempotencyKey": "revert-" + test.name,
			}))
			if response.Error != nil {
				t.Fatalf("file-change revert error: %v", response.Error)
			}
			test.assert(t, root)
		})
	}
}

func TestFileChangeRevertRejectsUnavailableAndMalformedRequests(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	fsService, err := toolfs.NewService(root)
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}
	defer fsService.Close()
	st := newRuntimeTestStore(t)
	legacy := struct{ store.Store }{Store: st}

	unavailable := []struct {
		name   string
		server *Server
	}{
		{"store", readyServer(WithFilesystem(fsService))},
		{"filesystem", readyServer(WithStore(st))},
		{"recovery capability", readyServer(WithStore(legacy), WithFilesystem(fsService))},
	}
	for _, test := range unavailable {
		t.Run(test.name, func(t *testing.T) {
			response := test.server.HandleRequest(ctx, request(fileChangeRevertMethod, map[string]any{
				"threadId":       "thread",
				"itemId":         "item",
				"idempotencyKey": "key",
			}))
			if response.Error == nil || response.Error.Code != protocol.CodeMethodUnavailable {
				t.Fatalf("unavailable response = %+v", response)
			}
		})
	}

	server := readyServer(WithStore(st), WithFilesystem(fsService))
	invalid := []struct {
		name   string
		params any
	}{
		{"empty payload", nil},
		{"required fields", map[string]any{"threadId": " ", "itemId": "item", "idempotencyKey": "key"}},
		{"long key", map[string]any{
			"threadId": "thread", "itemId": "item", "idempotencyKey": strings.Repeat("x", fileChangeRevertIdempotencyMaxLen+1),
		}},
		{"missing thread", map[string]any{"threadId": "missing", "itemId": "item", "idempotencyKey": "key"}},
	}
	for _, test := range invalid {
		t.Run(test.name, func(t *testing.T) {
			response := server.HandleRequest(ctx, request(fileChangeRevertMethod, test.params))
			if response.Error == nil || response.Error.Code != protocol.CodeInvalidParams {
				t.Fatalf("invalid response = %+v", response)
			}
		})
	}
}

func TestFileChangeRevertEvidenceAndHelperGuards(t *testing.T) {
	newEvidence := func() (*store.Item, *store.FileChangeRecovery, protocol.FileChangeItem) {
		payload := protocol.FileChangeItem{
			Type:   runtimeFileChangeItemKind,
			ID:     "item-1",
			Status: protocol.PatchApplyStatusCompleted,
			Changes: []protocol.FileUpdateChange{{
				Path: "notes.txt",
				Kind: protocol.PatchChangeKind{Type: runtimePatchChangeUpdate},
			}},
			Evidence: []protocol.FileChangeArtifactEvidence{{
				Path:                    "notes.txt",
				Operation:               "update",
				BeforeSHA256:            "before",
				AfterSHA256:             "after",
				RevertSnapshotAvailable: true,
			}},
		}
		item := &store.Item{
			ID:       "item-1",
			ThreadID: "thread-1",
			TurnID:   "turn-1",
			Kind:     runtimeFileChangeItemKind,
			Status:   runtimeFileChangeStatusCompleted,
			Payload:  mustRuntimeJSON(payload),
		}
		recovery := &store.FileChangeRecovery{
			ItemID:       item.ID,
			ThreadID:     item.ThreadID,
			TurnID:       item.TurnID,
			Path:         "notes.txt",
			BeforeExists: true,
			AfterExists:  true,
			BeforeSHA256: "before",
			AfterSHA256:  "after",
			Status:       store.FileChangeRecoveryAvailable,
		}
		return item, recovery, payload
	}
	item, recovery, _ := newEvidence()
	if err := validateFileChangeRevertEvidence(item, recovery); err != nil {
		t.Fatalf("valid evidence: %v", err)
	}

	tests := []struct {
		name   string
		mutate func(*store.Item, *store.FileChangeRecovery, *protocol.FileChangeItem)
	}{
		{"malformed payload", func(item *store.Item, _ *store.FileChangeRecovery, _ *protocol.FileChangeItem) {
			item.Payload = json.RawMessage(`{"type":`)
		}},
		{"identity mismatch", func(_ *store.Item, recovery *store.FileChangeRecovery, _ *protocol.FileChangeItem) {
			recovery.ThreadID = "other-thread"
		}},
		{"inexact shape", func(item *store.Item, _ *store.FileChangeRecovery, payload *protocol.FileChangeItem) {
			payload.Changes = nil
			item.Payload = mustRuntimeJSON(*payload)
		}},
		{"empty existence", func(item *store.Item, recovery *store.FileChangeRecovery, payload *protocol.FileChangeItem) {
			recovery.BeforeExists = false
			recovery.AfterExists = false
			item.Payload = mustRuntimeJSON(*payload)
		}},
		{"unsupported operation", func(item *store.Item, _ *store.FileChangeRecovery, payload *protocol.FileChangeItem) {
			payload.Evidence[0].Operation = "move"
			item.Payload = mustRuntimeJSON(*payload)
		}},
		{"public mismatch", func(item *store.Item, _ *store.FileChangeRecovery, payload *protocol.FileChangeItem) {
			payload.Evidence[0].RevertSnapshotAvailable = false
			item.Payload = mustRuntimeJSON(*payload)
		}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			item, recovery, payload := newEvidence()
			test.mutate(item, recovery, &payload)
			if err := validateFileChangeRevertEvidence(item, recovery); err == nil {
				t.Fatal("inconsistent evidence was accepted")
			}
		})
	}
	item, _, _ = newEvidence()
	if err := validateFileChangeRevertEvidence(item, nil); err == nil {
		t.Fatal("missing recovery evidence was accepted")
	}

	for _, status := range []store.TurnStatus{store.TurnCompleted, store.TurnFailed, store.TurnInterrupted} {
		if !fileChangeRevertTerminalTurnStatus(status) {
			t.Fatalf("terminal status %q rejected", status)
		}
	}
	for _, status := range []store.TurnStatus{store.TurnQueued, store.TurnRunning, "unknown"} {
		if fileChangeRevertTerminalTurnStatus(status) {
			t.Fatalf("non-terminal status %q accepted", status)
		}
	}

	if _, rpcErr := fileChangeRevertProtocolResult(nil, nil, false); rpcErr == nil || rpcErr.Code != protocol.CodeInternalError {
		t.Fatalf("incomplete receipt error = %+v", rpcErr)
	}
	var decoded struct {
		Value bool `json:"value"`
	}
	if err := decodeStrictJSON(json.RawMessage(`{"value":true} {}`), &decoded); err == nil {
		t.Fatal("multiple JSON values were accepted")
	}
	if err := decodeStrictJSON(json.RawMessage(`{"value":true} trailing`), &decoded); err == nil {
		t.Fatal("trailing invalid JSON was accepted")
	}

	root := t.TempDir()
	fsService, err := toolfs.NewService(root)
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}
	defer fsService.Close()
	st := newRuntimeTestStore(t)
	abortFileChangeRevertIfUnchanged(context.Background(), nil, nil, nil, "")
	abortFileChangeRevertIfUnchanged(context.Background(), st, fsService, nil, "")
	if err := os.WriteFile(filepath.Join(root, "notes.txt"), []byte("user edit"), 0o644); err != nil {
		t.Fatalf("WriteFile stale fixture: %v", err)
	}
	stale := &store.FileChangeRecovery{
		ItemID:      "item",
		Path:        "notes.txt",
		AfterExists: true,
		AfterSHA256: runtimeSHA256([]byte("expected")),
		AfterMode:   0o644,
	}
	abortFileChangeRevertIfUnchanged(context.Background(), st, fsService, stale, "key")
	canceled, cancel := context.WithCancel(context.Background())
	cancel()
	abortFileChangeRevertIfUnchanged(canceled, st, fsService, stale, "key")
}

func TestRuntimeFileChangeRecoveryRejectsUnsupportedSnapshots(t *testing.T) {
	turn := &store.Turn{ID: "turn-1", ThreadID: "thread-1"}
	valid := core.ArtifactChangedEvent{
		Path:               "notes.txt",
		Operation:          "update",
		BeforeExists:       true,
		AfterExists:        true,
		BeforeMode:         0o644,
		BeforeSize:         int64(len("before")),
		AfterSize:          int64(len("after")),
		BeforeSHA256:       runtimeSHA256([]byte("before")),
		AfterSHA256:        runtimeSHA256([]byte("after")),
		BeforeContentBytes: []byte("before"),
		AfterContentBytes:  []byte("after"),
	}
	if recovery, reason := runtimeFileChangeRecovery(valid, turn); reason != "" || recovery.Path != "notes.txt" {
		t.Fatalf("valid recovery = %+v, reason %q", recovery, reason)
	}
	tests := []struct {
		name   string
		mutate func(*core.ArtifactChangedEvent)
	}{
		{"empty path", func(event *core.ArtifactChangedEvent) { event.Path = "" }},
		{"absolute path", func(event *core.ArtifactChangedEvent) { event.Path = filepath.Join(t.TempDir(), "outside.txt") }},
		{"directory", func(event *core.ArtifactChangedEvent) { event.AfterIsDir = true }},
		{"symlink", func(event *core.ArtifactChangedEvent) { event.AfterIsSymlink = true }},
		{"no existence evidence", func(event *core.ArtifactChangedEvent) {
			event.BeforeExists = false
			event.AfterExists = false
		}},
		{"create mismatch", func(event *core.ArtifactChangedEvent) { event.Operation = "create" }},
		{"delete mismatch", func(event *core.ArtifactChangedEvent) { event.Operation = "delete" }},
		{"update mismatch", func(event *core.ArtifactChangedEvent) { event.BeforeExists = false }},
		{"oversized", func(event *core.ArtifactChangedEvent) {
			event.AfterSize = runtimeFileChangeRecoveryMaxBytes + 1
		}},
		{"before size mismatch", func(event *core.ArtifactChangedEvent) { event.BeforeSize++ }},
		{"after size mismatch", func(event *core.ArtifactChangedEvent) { event.AfterSize++ }},
		{"before digest mismatch", func(event *core.ArtifactChangedEvent) { event.BeforeSHA256 = "wrong" }},
		{"after digest mismatch", func(event *core.ArtifactChangedEvent) { event.AfterSHA256 = "wrong" }},
		{"path escape", func(event *core.ArtifactChangedEvent) { event.Path = "../outside.txt" }},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			event := valid
			test.mutate(&event)
			if _, reason := runtimeFileChangeRecovery(event, turn); reason == "" {
				t.Fatal("unsupported snapshot unexpectedly became recoverable")
			}
		})
	}

	content := []byte("recovery")
	bounded := runtimeRecoveryContentBytes(content)
	content[0] = 'X'
	if string(bounded) != "recovery" {
		t.Fatalf("bounded recovery content aliased input: %q", bounded)
	}
	if oversized := runtimeRecoveryContentBytes(make([]byte, runtimeFileChangeRecoveryMaxBytes+1)); oversized != nil {
		t.Fatalf("oversized recovery content length = %d, want nil", len(oversized))
	}
}

func TestRuntimeFileChangeTrackerRecoveryCapabilityAndErrors(t *testing.T) {
	event := core.ArtifactChangedEvent{
		Path:               "notes.txt",
		Operation:          "update",
		BeforeExists:       true,
		AfterExists:        true,
		BeforeMode:         0o644,
		AfterMode:          0o644,
		BeforeSize:         int64(len("before")),
		AfterSize:          int64(len("after")),
		BeforeSHA256:       runtimeSHA256([]byte("before")),
		AfterSHA256:        runtimeSHA256([]byte("after")),
		BeforeContentBytes: []byte("before"),
		AfterContentBytes:  []byte("after"),
		ChangedAt:          time.Now().UTC(),
	}
	newTurn := func(t *testing.T) (*store.SQLiteStore, *store.Turn) {
		t.Helper()
		st := newRuntimeTestStore(t)
		thread, err := st.CreateThread(context.Background(), store.CreateThreadRequest{Title: "File changes"})
		if err != nil {
			t.Fatalf("CreateThread: %v", err)
		}
		turn, err := st.CreateTurn(context.Background(), store.CreateTurnRequest{ThreadID: thread.ID})
		if err != nil {
			t.Fatalf("CreateTurn: %v", err)
		}
		return st, turn
	}

	t.Run("legacy store exposes public unavailability", func(t *testing.T) {
		st, turn := newTurn(t)
		legacy := struct{ store.Store }{Store: st}
		notifier := &runtimeErrorCaptureNotifier{}
		tracker := newRuntimeFileChangeTracker(legacy, notifier, turn, nil)
		tracker.artifactChanged(event)
		if err := tracker.Err(); err != nil {
			t.Fatalf("artifactChanged: %v", err)
		}
		items, err := st.ListItems(context.Background(), store.ItemFilter{ThreadID: turn.ThreadID, TurnID: turn.ID})
		if err != nil {
			t.Fatalf("ListItems: %v", err)
		}
		fileItem := findRuntimeFileChangeItem(t, items, event.Path)
		evidence := fileItem.Payload.Evidence[0]
		if evidence.RevertSnapshotAvailable || evidence.RevertUnavailableReason != "durable recovery storage is unavailable" {
			t.Fatalf("legacy recovery evidence = %+v", evidence)
		}
		if notifier.method != "item/completed" {
			t.Fatalf("last notification = %q, want item/completed", notifier.method)
		}
	})

	t.Run("snapshot persistence failure is sticky", func(t *testing.T) {
		st, turn := newTurn(t)
		notifier := &runtimeErrorCaptureNotifier{}
		persistErr := errors.New("snapshot unavailable")
		failing := failingFileChangeRecoveryStore{Store: st, err: persistErr}
		tracker := newRuntimeFileChangeTracker(failing, notifier, turn, nil)
		tracker.artifactChanged(event)
		first := tracker.Err()
		if first == nil || !errors.Is(first, persistErr) || notifier.method != "error" {
			t.Fatalf("tracker error = %v, notification = %q", first, notifier.method)
		}
		tracker.recordErrorLocked("ignored", errors.New("second"))
		tracker.recordErrorLocked("ignored nil", nil)
		if tracker.Err() != first {
			t.Fatalf("sticky tracker error changed from %v to %v", first, tracker.Err())
		}
	})

	var nilTracker *runtimeFileChangeTracker
	nilTracker.artifactChanged(event)
	if err := nilTracker.Err(); err != nil {
		t.Fatalf("nil tracker error = %v", err)
	}
}

func TestFileChangeRevertRequiresExactCanonicalThreadWorkspace(t *testing.T) {
	root := t.TempDir()
	alias := filepath.Join(t.TempDir(), "workspace-link")
	if err := os.Symlink(root, alias); err != nil {
		t.Fatalf("Symlink workspace alias: %v", err)
	}
	if err := requireExactThreadWorkspace(&store.Thread{Workspace: alias}, root); err != nil {
		t.Fatalf("canonical workspace alias rejected: %v", err)
	}
	if err := requireExactThreadWorkspace(&store.Thread{Workspace: t.TempDir()}, root); err == nil {
		t.Fatal("mismatched workspace was accepted")
	}
	if err := requireExactThreadWorkspace(&store.Thread{}, root); err == nil {
		t.Fatal("empty workspace was accepted")
	}
	if err := requireExactThreadWorkspace(nil, root); err == nil {
		t.Fatal("nil thread was accepted")
	}
	if err := requireExactThreadWorkspace(&store.Thread{Workspace: filepath.Join(root, "missing")}, root); err == nil {
		t.Fatal("missing thread workspace was accepted")
	}
	if err := requireExactThreadWorkspace(&store.Thread{Workspace: root}, filepath.Join(root, "missing")); err == nil {
		t.Fatal("missing filesystem root was accepted")
	}
}

type failingFileChangeRecoveryStore struct {
	store.Store
	err error
}

func (s failingFileChangeRecoveryStore) SaveFileChangeRecovery(context.Context, store.SaveFileChangeRecoveryRequest) (*store.FileChangeRecovery, error) {
	return nil, s.err
}

func (s failingFileChangeRecoveryStore) GetFileChangeRecovery(context.Context, string) (*store.FileChangeRecovery, error) {
	return nil, s.err
}

func (s failingFileChangeRecoveryStore) PrepareFileChangeRevert(context.Context, store.PrepareFileChangeRevertRequest) (*store.PrepareFileChangeRevertResult, error) {
	return nil, s.err
}

func (s failingFileChangeRecoveryStore) AbortFileChangeRevert(context.Context, store.AbortFileChangeRevertRequest) (*store.FileChangeRecovery, error) {
	return nil, s.err
}

func (s failingFileChangeRecoveryStore) CompleteFileChangeRevert(context.Context, store.CompleteFileChangeRevertRequest) (*store.CompleteFileChangeRevertResult, error) {
	return nil, s.err
}

func approveServerRequest(
	t *testing.T,
	server *Server,
	method string,
	check func(protocol.FileChangeApprovalRequestParams),
) {
	t.Helper()
	pending := waitForServerRequest(t, server)
	if pending.Method != method {
		t.Fatalf("server request method = %q, want %q", pending.Method, method)
	}
	var params protocol.FileChangeApprovalRequestParams
	if err := json.Unmarshal(pending.Params, &params); err != nil {
		t.Fatalf("decode file-change approval: %v", err)
	}
	check(params)
	requestID, _ := pending.ID.Value().(string)
	response := server.HandleRequest(context.Background(), request("approval/respond", map[string]any{
		"requestId": requestID,
		"approved":  true,
	}))
	if response.Error != nil {
		t.Fatalf("approval/respond error: %v", response.Error)
	}
}

func denyServerRequest(
	t *testing.T,
	server *Server,
	method string,
	check func(protocol.FileChangeApprovalRequestParams),
) {
	t.Helper()
	pending := waitForServerRequest(t, server)
	if pending.Method != method {
		t.Fatalf("server request method = %q, want %q", pending.Method, method)
	}
	var params protocol.FileChangeApprovalRequestParams
	if err := json.Unmarshal(pending.Params, &params); err != nil {
		t.Fatalf("decode file-change approval: %v", err)
	}
	check(params)
	requestID, _ := pending.ID.Value().(string)
	response := server.HandleRequest(context.Background(), request("approval/respond", map[string]any{
		"requestId": requestID,
		"approved":  false,
		"message":   "denied by test",
	}))
	if response.Error != nil {
		t.Fatalf("approval/respond denial error: %v", response.Error)
	}
}

func newTestFileChangeRevertStore(t *testing.T, path string) *store.SQLiteStore {
	t.Helper()
	st, err := store.NewSQLiteStore(path)
	if err != nil {
		t.Fatalf("NewSQLiteStore: %v", err)
	}
	t.Cleanup(func() {
		if err := st.Close(); err != nil && !errors.Is(err, store.ErrStoreClosed) {
			t.Errorf("Close store: %v", err)
		}
	})
	return st
}
