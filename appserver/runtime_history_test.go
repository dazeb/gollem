package appserver

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/fugue-labs/gollem/appserver/store"
	"github.com/fugue-labs/gollem/core"
)

func TestBoundedRuntimeReplayItemsKeepsLatestCompactionAndTail(t *testing.T) {
	now := time.Now().UTC()
	items := []*store.Item{
		runtimeHistoryMessageItem("older", "user", "excluded before compaction", now),
		{
			ID:      "compaction",
			Kind:    threadCompactionItemKind,
			Payload: json.RawMessage(`{"type":"contextCompaction","summary":"bounded summary"}`),
		},
	}
	for i := range runtimeReplayMaxItems + 20 {
		items = append(items, runtimeHistoryMessageItem(
			fmt.Sprintf("message-%03d", i),
			"user",
			"bounded replay",
			now.Add(time.Duration(i+1)*time.Second),
		))
	}

	got := boundedRuntimeReplayItems(items)
	if len(got) != runtimeReplayMaxItems {
		t.Fatalf("bounded replay item count = %d, want %d", len(got), runtimeReplayMaxItems)
	}
	if got[0].ID != "compaction" {
		t.Fatalf("bounded replay first item = %#v, want latest compaction", got[0])
	}
	if got[len(got)-1].ID != fmt.Sprintf("message-%03d", runtimeReplayMaxItems+19) {
		t.Fatalf("bounded replay last item = %#v", got[len(got)-1])
	}
	for _, item := range got {
		if item.ID == "older" {
			t.Fatal("bounded replay retained an item before the latest compaction")
		}
	}
	payloadBytes := 0
	for _, item := range got {
		if item != nil {
			payloadBytes += len(item.Payload)
		}
	}
	if payloadBytes > runtimeReplayMaxPayloadBytes {
		t.Fatalf("bounded replay payload bytes = %d, max %d", payloadBytes, runtimeReplayMaxPayloadBytes)
	}
}

func TestBoundedRuntimeReplayItemsDropsOversizedCompaction(t *testing.T) {
	now := time.Now().UTC()
	compaction := &store.Item{
		ID:      "oversized-compaction",
		Kind:    threadCompactionItemKind,
		Payload: json.RawMessage(strings.Repeat("x", runtimeReplayMaxPayloadBytes+1)),
	}
	tail := runtimeHistoryMessageItem("tail", "user", "bounded tail", now)

	got := boundedRuntimeReplayItems([]*store.Item{compaction, tail})
	if len(got) != 1 || got[0].ID != tail.ID {
		t.Fatalf("bounded replay = %#v, want only tail", got)
	}
}

func TestRuntimeMessagesFromItemsReconstructsMixedToolHistory(t *testing.T) {
	now := time.Now().UTC()
	success := true
	failure := false
	items := []*store.Item{
		runtimeHistoryMessageItem("message-user", "user", "inspect the workspace", now),
		runtimeHistoryDynamicToolItem("tool-success", runtimeDynamicToolCallPayload{
			Type:      runtimeDynamicToolCallItemKind,
			Tool:      "workspace_read_file",
			Arguments: map[string]any{"path": "README.md"},
			Status:    runtimeToolStatusCompleted,
			ContentItems: []runtimeDynamicToolCallContentItem{{
				Type: "inputText",
				Text: `{"text":"contents"}`,
			}},
			Success: &success,
		}, now.Add(time.Second)),
		{
			ID:           "command-child",
			ParentItemID: "tool-success",
			Kind:         threadShellCommandItemKind,
			Status:       commandExecutionStatusCompleted,
			Payload: mustRuntimeJSON(newCommandExecutionPayload(
				"cat README.md", ".", "process-1", commandExecutionSourceAgent,
				commandExecutionStatusCompleted, "contents", runtimeIntPointer(0), now, runtimeTimePointer(now.Add(time.Second)),
			)),
			CreatedAt: now.Add(2 * time.Second),
			UpdatedAt: now.Add(2 * time.Second),
		},
		runtimeHistoryDynamicToolItem("tool-denied", runtimeDynamicToolCallPayload{
			Type:      runtimeDynamicToolCallItemKind,
			Tool:      "git_commit",
			Arguments: map[string]any{"message": "do not commit"},
			Status:    "declined",
			ContentItems: []runtimeDynamicToolCallContentItem{{
				Type: "inputText",
				Text: "approval denied",
			}},
			Success: &failure,
		}, now.Add(3*time.Second)),
		{
			ID:           "mcp-child",
			ParentItemID: "tool-denied",
			Kind:         runtimeMCPToolCallItemKind,
			Status:       runtimeMCPStatusFailed,
			Payload: mustRuntimeJSON(runtimeMCPToolCallPayload{
				Type:      runtimeMCPToolCallItemKind,
				Server:    "repo",
				Tool:      "commit",
				Status:    runtimeMCPStatusFailed,
				Arguments: map[string]any{"message": "do not commit"},
				Error:     &runtimeMCPToolCallErrorPayload{Message: "approval denied"},
			}),
			CreatedAt: now.Add(4 * time.Second),
			UpdatedAt: now.Add(4 * time.Second),
		},
		runtimeHistoryDynamicToolItem("tool-interrupted", runtimeDynamicToolCallPayload{
			Type:      runtimeDynamicToolCallItemKind,
			Tool:      "workspace_run_command",
			Arguments: map[string]any{"command": "go test ./..."},
			Status:    runtimeToolStatusInProgress,
		}, now.Add(5*time.Second)),
		runtimeHistoryDynamicToolItem("tool-interaction", runtimeDynamicToolCallPayload{
			Type:      runtimeDynamicToolCallItemKind,
			Tool:      "request_user_input",
			Arguments: map[string]any{"prompt": "Continue?"},
			Status:    runtimeToolStatusCompleted,
			ContentItems: []runtimeDynamicToolCallContentItem{{
				Type: "inputText",
				Text: `{"answer":"yes"}`,
			}},
			Success: &success,
		}, now.Add(6*time.Second)),
		runtimeHistoryMessageItem("message-assistant", "assistant", "workspace inspected", now.Add(7*time.Second)),
	}

	messages := runtimeMessagesFromItems(items)
	if len(messages) != 10 {
		t.Fatalf("messages = %#v, want user + four tool pairs + assistant", messages)
	}
	assertRuntimeUserPrompt(t, messages[0], "inspect the workspace")
	assertRuntimeReplayPair(t, messages[1], messages[2], "workspace_read_file", "tool-success", false, "contents")
	assertRuntimeReplayPair(t, messages[3], messages[4], "git_commit", "tool-denied", true, "approval denied")
	assertRuntimeReplayPair(t, messages[5], messages[6], "workspace_run_command", "tool-interrupted", true, "inProgress")
	assertRuntimeReplayPair(t, messages[7], messages[8], "request_user_input", "tool-interaction", false, "answer")
	assertRuntimeAssistantText(t, messages[9], "workspace inspected")

	normalized, err := core.NormalizeHistory()(context.Background(), messages)
	if err != nil {
		t.Fatalf("NormalizeHistory: %v", err)
	}
	if len(normalized) != len(messages) {
		t.Fatalf("normalized messages = %#v, want every reconstructed pair retained", normalized)
	}
	encoded, err := core.MarshalMessages(messages)
	if err != nil {
		t.Fatalf("MarshalMessages: %v", err)
	}
	roundTripped, err := core.UnmarshalMessages(encoded)
	if err != nil {
		t.Fatalf("UnmarshalMessages: %v", err)
	}
	if len(roundTripped) != len(messages) {
		t.Fatalf("round-tripped messages = %#v, want %d", roundTripped, len(messages))
	}
}

func TestRuntimeMessagesFromItemsReplaysOnlyConsumedSteers(t *testing.T) {
	now := time.Now().UTC()
	consumedAt := now.Add(2 * time.Second)
	failedAt := now.Add(3 * time.Second)
	items := []*store.Item{
		{
			ID:     "completed-steer",
			Seq:    1,
			Kind:   runtimeSteerItemKind,
			Status: runtimeSteerStatusComplete,
			Payload: mustRuntimeJSON(runtimeSteerPayload{
				ClientUserMessageID: "client-completed",
				Message:             "use the focused test result",
				Status:              runtimeSteerStatusComplete,
				QueuedAt:            now,
				ConsumedAfterSeq:    1,
				ConsumedAt:          &consumedAt,
			}),
		},
		{
			ID:     "queued-steer",
			Seq:    2,
			Kind:   runtimeSteerItemKind,
			Status: runtimeSteerStatusQueued,
			Payload: mustRuntimeJSON(runtimeSteerPayload{
				ClientUserMessageID: "client-queued",
				Message:             "do not replay queued",
				Status:              runtimeSteerStatusQueued,
				QueuedAt:            now,
			}),
		},
		{
			ID:     "failed-steer",
			Seq:    3,
			Kind:   runtimeSteerItemKind,
			Status: runtimeSteerStatusFailed,
			Payload: mustRuntimeJSON(runtimeSteerPayload{
				ClientUserMessageID: "client-failed",
				Message:             "do not replay failed",
				Status:              runtimeSteerStatusFailed,
				QueuedAt:            now,
				FailedAt:            &failedAt,
				Error:               "turn interrupted",
			}),
		},
		{
			ID:     "partial-steer",
			Seq:    4,
			Kind:   runtimeSteerItemKind,
			Status: runtimeSteerStatusComplete,
			Payload: mustRuntimeJSON(runtimeSteerPayload{
				ClientUserMessageID: "client-partial",
				Message:             "do not replay partial persistence",
				Status:              runtimeSteerStatusQueued,
				QueuedAt:            now,
			}),
		},
		{
			ID:      "malformed-steer",
			Seq:     5,
			Kind:    runtimeSteerItemKind,
			Status:  runtimeSteerStatusComplete,
			Payload: json.RawMessage(`{"message":`),
		},
		{
			ID:     "backward-boundary-steer",
			Seq:    6,
			Kind:   runtimeSteerItemKind,
			Status: runtimeSteerStatusComplete,
			Payload: mustRuntimeJSON(runtimeSteerPayload{
				ClientUserMessageID: "client-backward",
				Message:             "do not replay corrupt boundary",
				Status:              runtimeSteerStatusComplete,
				QueuedAt:            now,
				ConsumedAfterSeq:    5,
				ConsumedAt:          &consumedAt,
			}),
		},
	}

	messages := runtimeMessagesFromItems(items)
	if len(messages) != 1 {
		t.Fatalf("messages = %#v, want only the consumed steer", messages)
	}
	assertRuntimeUserPrompt(t, messages[0], "use the focused test result")
	request, ok := messages[0].(core.ModelRequest)
	if !ok || !request.Timestamp.Equal(consumedAt) {
		t.Fatalf("consumed steer timestamp = %#v, want %s", messages[0], consumedAt)
	}
}

func TestRuntimeMessagesFromItemsOrdersConsumedSteerAfterToolBoundary(t *testing.T) {
	now := time.Now().UTC()
	consumedAt := now.Add(4 * time.Second)
	success := true
	items := []*store.Item{
		runtimeHistoryMessageItem("message-user", "user", "inspect first", now),
		{
			ID:     "steer-before-tool-item",
			Seq:    2,
			Kind:   runtimeSteerItemKind,
			Status: runtimeSteerStatusComplete,
			Payload: mustRuntimeJSON(runtimeSteerPayload{
				ClientUserMessageID: "client-boundary",
				Message:             "adjust after tool",
				Status:              runtimeSteerStatusComplete,
				QueuedAt:            now.Add(time.Second),
				ConsumedAfterSeq:    3,
				ConsumedAt:          &consumedAt,
			}),
			CreatedAt: now.Add(time.Second),
			UpdatedAt: consumedAt,
		},
		runtimeHistoryDynamicToolItem("tool-after-queue", runtimeDynamicToolCallPayload{
			Tool:         "workspace_read_file",
			Arguments:    map[string]any{"path": "README.md"},
			Status:       runtimeToolStatusCompleted,
			ContentItems: []runtimeDynamicToolCallContentItem{{Type: "inputText", Text: "contents"}},
			Success:      &success,
		}, now.Add(2*time.Second)),
		runtimeHistoryMessageItem("message-assistant", "assistant", "adjusted answer", now.Add(5*time.Second)),
	}
	for i, item := range items {
		if item.Seq == 0 {
			item.Seq = int64(i + 1)
		}
	}

	messages := runtimeMessagesFromItems(items)
	if len(messages) != 5 {
		t.Fatalf("messages = %#v, want user + tool pair + steer + assistant", messages)
	}
	assertRuntimeUserPrompt(t, messages[0], "inspect first")
	assertRuntimeReplayPair(t, messages[1], messages[2], "workspace_read_file", "tool-after-queue", false, "contents")
	assertRuntimeUserPrompt(t, messages[3], "adjust after tool")
	assertRuntimeAssistantText(t, messages[4], "adjusted answer")
}

func TestRuntimeMessagesFromItemsReconstructsStandaloneOperationalItems(t *testing.T) {
	now := time.Now().UTC()
	output := "tests passed\n... tool output truncated ...\nfinal line"
	items := []*store.Item{
		runtimeHistoryMessageItem("message-user", "user", "recover operations", now),
		{
			ID:     "standalone-command",
			Kind:   threadShellCommandItemKind,
			Status: commandExecutionStatusCompleted,
			Payload: mustRuntimeJSON(newCommandExecutionPayload(
				"go test ./...", "/workspace", "process-1", commandExecutionSourceUserShell,
				commandExecutionStatusCompleted, output, runtimeIntPointer(0), now, runtimeTimePointer(now.Add(time.Second)),
			)),
			CreatedAt: now.Add(time.Second),
			UpdatedAt: now.Add(time.Second),
		},
		{
			ID:     "standalone-file",
			Kind:   runtimeFileChangeItemKind,
			Status: runtimeFileChangeStatusCompleted,
			Payload: mustRuntimeJSON(runtimeFileChangePayload{
				Type:   runtimeFileChangeItemKind,
				Status: runtimeFileChangeStatusCompleted,
				Changes: []runtimeFileUpdateChange{{
					Path: "notes.txt",
					Kind: runtimePatchChangeKind{Type: runtimePatchChangeAdd},
					Diff: "+recovered\n",
				}},
			}),
			CreatedAt: now.Add(2 * time.Second),
			UpdatedAt: now.Add(2 * time.Second),
		},
		{
			ID:     "standalone-mcp",
			Kind:   runtimeMCPToolCallItemKind,
			Status: runtimeMCPStatusFailed,
			Payload: mustRuntimeJSON(runtimeMCPToolCallPayload{
				Type:      runtimeMCPToolCallItemKind,
				Server:    "repo",
				Tool:      "search",
				Status:    runtimeMCPStatusFailed,
				Arguments: map[string]any{"query": "history"},
				Error:     &runtimeMCPToolCallErrorPayload{Message: "connection interrupted"},
			}),
			CreatedAt: now.Add(3 * time.Second),
			UpdatedAt: now.Add(3 * time.Second),
		},
		{
			ID:        "malformed-dynamic",
			Kind:      runtimeDynamicToolCallItemKind,
			Status:    runtimeToolStatusCompleted,
			Payload:   json.RawMessage(`{"tool":`),
			CreatedAt: now.Add(4 * time.Second),
			UpdatedAt: now.Add(4 * time.Second),
		},
		runtimeHistoryMessageItem("message-assistant", "assistant", "operations recovered", now.Add(5*time.Second)),
	}

	messages := runtimeMessagesFromItems(items)
	if len(messages) != 8 {
		t.Fatalf("messages = %#v, want user + three standalone pairs + assistant", messages)
	}
	assertRuntimeReplayPair(t, messages[1], messages[2], "thread_shell_command", "standalone-command", false, "tool output truncated")
	assertRuntimeReplayPair(t, messages[3], messages[4], "file_change", "standalone-file", false, "notes.txt")
	assertRuntimeReplayPair(t, messages[5], messages[6], "mcp_call_tool", "standalone-mcp", true, "connection interrupted")
}

func TestRuntimeMessagesFromItemsMakesDuplicateCallIDsAndMalformedArgumentsSafe(t *testing.T) {
	now := time.Now().UTC()
	success := true
	items := []*store.Item{
		runtimeHistoryMessageItem("message-user", "user", "recover duplicates", now),
		runtimeHistoryDynamicToolItem("duplicate", runtimeDynamicToolCallPayload{
			Tool:         "first_tool",
			Arguments:    "{not-json",
			Status:       runtimeToolStatusCompleted,
			ContentItems: []runtimeDynamicToolCallContentItem{{Type: "inputText", Text: "first"}},
			Success:      &success,
		}, now.Add(time.Second)),
		runtimeHistoryDynamicToolItem("duplicate", runtimeDynamicToolCallPayload{
			Tool:         "second_tool",
			Arguments:    map[string]any{"value": "second"},
			Status:       runtimeToolStatusCompleted,
			ContentItems: []runtimeDynamicToolCallContentItem{{Type: "inputText", Text: "second"}},
			Success:      &success,
		}, now.Add(2*time.Second)),
		runtimeHistoryMessageItem("message-assistant", "assistant", "duplicates recovered", now.Add(3*time.Second)),
	}

	messages := runtimeMessagesFromItems(items)
	firstCall, _ := runtimeReplayParts(t, messages[1], messages[2])
	secondCall, _ := runtimeReplayParts(t, messages[3], messages[4])
	if firstCall.ToolCallID != "duplicate" || secondCall.ToolCallID != "duplicate_2" {
		t.Fatalf("call IDs = %q/%q, want duplicate/duplicate_2", firstCall.ToolCallID, secondCall.ToolCallID)
	}
	if !json.Valid([]byte(firstCall.ArgsJSON)) {
		t.Fatalf("first args = %q, want valid JSON", firstCall.ArgsJSON)
	}
	var args map[string]any
	if err := json.Unmarshal([]byte(firstCall.ArgsJSON), &args); err != nil || args["persistedArguments"] != "{not-json" {
		t.Fatalf("first args = %q (%v), want wrapped persisted arguments", firstCall.ArgsJSON, err)
	}
}

func TestRuntimeMessagesFromItemsUsesStoreStatusAfterPartialPersistence(t *testing.T) {
	now := time.Now().UTC()
	success := true
	item := runtimeHistoryDynamicToolItem("partial-tool", runtimeDynamicToolCallPayload{
		Tool:         "partially_persisted",
		Arguments:    map[string]any{"value": "stale"},
		Status:       runtimeToolStatusCompleted,
		ContentItems: []runtimeDynamicToolCallContentItem{{Type: "inputText", Text: "stale success"}},
		Success:      &success,
	}, now)
	item.Status = runtimeToolStatusFailed

	messages := runtimeMessagesFromItems([]*store.Item{item})
	if len(messages) != 2 {
		t.Fatalf("messages = %#v, want one recovered pair", messages)
	}
	assertRuntimeReplayPair(t, messages[0], messages[1], "partially_persisted", "partial-tool", true, "failed")
}

func TestServerRuntimeThreadResumeReconstructsToolTranscriptWithoutReexecution(t *testing.T) {
	ctx := context.Background()
	st := newRuntimeTestStore(t)
	var executions atomic.Int32
	type echoParams struct {
		Text string `json:"text"`
	}
	echo := core.FuncTool[echoParams]("echo", "Echo text.", func(_ context.Context, params echoParams) (string, error) {
		executions.Add(1)
		return `{"echo":` + runtimeJSONString(params.Text) + `}`, nil
	})
	model := core.NewTestModel(
		core.ToolCallResponseWithID("echo", `{"text":"hello"}`, "provider-call-id"),
		core.TextResponse("first answer"),
		core.TextResponse("second answer"),
	)
	server := readyServer(
		WithStore(st),
		WithRuntimeService(NewRuntimeService(
			WithRuntimeModel(model, RuntimeModelInfo{ProviderID: "test", Model: "test-model"}),
			WithRuntimeTools(echo),
		)),
	)

	startResp := server.HandleRequest(ctx, request("thread/start", map[string]any{"prompt": "first prompt"}))
	if startResp.Error != nil {
		t.Fatalf("thread/start error: %v", startResp.Error)
	}
	var started struct {
		Thread *store.Thread `json:"thread"`
	}
	decodeResult(t, startResp, &started)
	waitForNotificationSet(t, server, "turn/completed")
	server = readyServer(
		WithStore(st),
		WithRuntimeService(NewRuntimeService(
			WithRuntimeModel(model, RuntimeModelInfo{ProviderID: "test", Model: "test-model"}),
			WithRuntimeTools(echo),
		)),
	)

	resumeResp := server.HandleRequest(ctx, request("thread/resume", map[string]any{
		"threadId": started.Thread.ID,
		"prompt":   "second prompt",
	}))
	if resumeResp.Error != nil {
		t.Fatalf("thread/resume error: %v", resumeResp.Error)
	}
	waitForNotificationSet(t, server, "turn/completed")

	if got := executions.Load(); got != 1 {
		t.Fatalf("tool executions = %d, want 1", got)
	}
	calls := model.Calls()
	if len(calls) != 3 {
		t.Fatalf("model calls = %d, want 3", len(calls))
	}
	messages := calls[2].Messages
	if len(messages) != 5 {
		t.Fatalf("resume messages = %#v, want prior user/tool pair/assistant plus prompt", messages)
	}
	assertRuntimeUserPrompt(t, messages[0], "first prompt")
	call, result := runtimeReplayParts(t, messages[1], messages[2])
	if call.ToolName != "echo" || call.ToolCallID == "provider-call-id" || result.ToolCallID != call.ToolCallID {
		t.Fatalf("replayed pair = %#v/%#v", call, result)
	}
	assertRuntimeAssistantText(t, messages[3], "first answer")
	assertRuntimeUserPrompt(t, messages[4], "second prompt")
}

func TestServerRuntimeTurnRetryReconstructsPriorToolTranscriptWithoutReexecution(t *testing.T) {
	ctx := context.Background()
	st := newRuntimeTestStore(t)
	var executions atomic.Int32
	type echoParams struct {
		Text string `json:"text"`
	}
	echo := core.FuncTool[echoParams]("echo", "Echo text.", func(_ context.Context, params echoParams) (string, error) {
		executions.Add(1)
		return params.Text, nil
	})
	model := core.NewTestModel(
		core.ToolCallResponseWithID("echo", `{"text":"hello"}`, "provider-call-id"),
		core.TextResponse("first answer"),
		core.TextResponse("second answer"),
		core.TextResponse("retry answer"),
	)
	server := readyServer(
		WithStore(st),
		WithRuntimeService(NewRuntimeService(
			WithRuntimeModel(model, RuntimeModelInfo{ProviderID: "test", Model: "test-model"}),
			WithRuntimeTools(echo),
		)),
	)

	startResp := server.HandleRequest(ctx, request("thread/start", map[string]any{"prompt": "first prompt"}))
	if startResp.Error != nil {
		t.Fatalf("thread/start error: %v", startResp.Error)
	}
	var started struct {
		Thread *store.Thread `json:"thread"`
	}
	decodeResult(t, startResp, &started)
	waitForNotificationSet(t, server, "turn/completed")

	resumeResp := server.HandleRequest(ctx, request("thread/resume", map[string]any{
		"threadId": started.Thread.ID,
		"prompt":   "second prompt",
	}))
	if resumeResp.Error != nil {
		t.Fatalf("thread/resume error: %v", resumeResp.Error)
	}
	var resumed struct {
		Turn *store.Turn `json:"turn"`
	}
	decodeResult(t, resumeResp, &resumed)
	waitForNotificationSet(t, server, "turn/completed")

	retryResp := server.HandleRequest(ctx, request("turn/retry", map[string]any{"turnId": resumed.Turn.ID}))
	if retryResp.Error != nil {
		t.Fatalf("turn/retry error: %v", retryResp.Error)
	}
	waitForNotificationSet(t, server, "turn/completed")

	if got := executions.Load(); got != 1 {
		t.Fatalf("tool executions = %d, want 1", got)
	}
	calls := model.Calls()
	if len(calls) != 4 {
		t.Fatalf("model calls = %d, want 4", len(calls))
	}
	messages := calls[3].Messages
	if len(messages) != 5 {
		t.Fatalf("retry messages = %#v, want prior user/tool pair/assistant plus retried prompt", messages)
	}
	assertRuntimeReplayPair(t, messages[1], messages[2], "echo", "", false, "hello")
	assertRuntimeUserPrompt(t, messages[4], "second prompt")
}

func TestServerRuntimeForkResumeRemapsChildItemsWithoutDuplicateReplay(t *testing.T) {
	ctx := context.Background()
	st := newRuntimeTestStore(t)
	var executions atomic.Int32
	type echoParams struct {
		Text string `json:"text"`
	}
	echo := core.FuncTool[echoParams]("echo", "Echo text.", func(_ context.Context, params echoParams) (string, error) {
		executions.Add(1)
		return params.Text, nil
	})
	model := core.NewTestModel(
		core.ToolCallResponseWithID("echo", `{"text":"hello"}`, "provider-call-id"),
		core.TextResponse("first answer"),
		core.TextResponse("fork answer"),
	)
	server := readyServer(
		WithStore(st),
		WithRuntimeService(NewRuntimeService(
			WithRuntimeModel(model, RuntimeModelInfo{ProviderID: "test", Model: "test-model"}),
			WithRuntimeTools(echo),
		)),
	)

	startResp := server.HandleRequest(ctx, request("thread/start", map[string]any{"prompt": "first prompt"}))
	if startResp.Error != nil {
		t.Fatalf("thread/start error: %v", startResp.Error)
	}
	var started struct {
		Thread *store.Thread `json:"thread"`
	}
	decodeResult(t, startResp, &started)
	waitForNotificationSet(t, server, "turn/completed")
	items, err := st.ListItems(ctx, store.ItemFilter{ThreadID: started.Thread.ID})
	if err != nil {
		t.Fatalf("ListItems: %v", err)
	}
	parent := findRuntimeToolItem(t, items, "echo", "text", "hello")
	if _, err := st.AppendItem(ctx, store.AppendItemRequest{
		ThreadID:     started.Thread.ID,
		TurnID:       parent.Item.TurnID,
		ParentItemID: parent.Item.ID,
		Kind:         runtimeFileChangeItemKind,
		Status:       runtimeFileChangeStatusCompleted,
		Payload: mustRuntimeJSON(runtimeFileChangePayload{
			Type:   runtimeFileChangeItemKind,
			Status: runtimeFileChangeStatusCompleted,
			Changes: []runtimeFileUpdateChange{{
				Path: "echo.txt",
				Kind: runtimePatchChangeKind{Type: runtimePatchChangeAdd},
				Diff: "+hello\n",
			}},
		}),
	}); err != nil {
		t.Fatalf("AppendItem child: %v", err)
	}

	forkResp := server.HandleRequest(ctx, request("thread/fork", map[string]any{
		"threadId":     started.Thread.ID,
		"includeItems": true,
	}))
	if forkResp.Error != nil {
		t.Fatalf("thread/fork error: %v", forkResp.Error)
	}
	var forked struct {
		Thread *store.Thread `json:"thread"`
	}
	decodeResult(t, forkResp, &forked)
	server.DrainNotifications()

	resumeResp := server.HandleRequest(ctx, request("thread/resume", map[string]any{
		"threadId": forked.Thread.ID,
		"prompt":   "continue fork",
	}))
	if resumeResp.Error != nil {
		t.Fatalf("thread/resume fork error: %v", resumeResp.Error)
	}
	waitForNotificationSet(t, server, "turn/completed")

	if got := executions.Load(); got != 1 {
		t.Fatalf("tool executions = %d, want 1", got)
	}
	calls := model.Calls()
	messages := calls[2].Messages
	if len(messages) != 5 {
		t.Fatalf("fork resume messages = %#v, want one reconstructed tool pair", messages)
	}
	assertRuntimeReplayPair(t, messages[1], messages[2], "echo", "", false, "hello")
}

func TestServerRuntimeCompactSummaryPreservesToolActivity(t *testing.T) {
	ctx := context.Background()
	st := newRuntimeTestStore(t)
	var executions atomic.Int32
	type echoParams struct {
		Text string `json:"text"`
	}
	echo := core.FuncTool[echoParams]("echo", "Echo text.", func(_ context.Context, params echoParams) (string, error) {
		executions.Add(1)
		return `{"echo":` + runtimeJSONString(params.Text) + `}`, nil
	})
	model := core.NewTestModel(
		core.ToolCallResponseWithID("echo", `{"text":"hello"}`, "provider-call-id"),
		core.TextResponse("first answer"),
		core.TextResponse("after compact"),
	)
	server := readyServer(
		WithStore(st),
		WithRuntimeService(NewRuntimeService(
			WithRuntimeModel(model, RuntimeModelInfo{ProviderID: "test", Model: "test-model"}),
			WithRuntimeTools(echo),
		)),
	)

	startResp := server.HandleRequest(ctx, request("thread/start", map[string]any{"prompt": "first prompt"}))
	if startResp.Error != nil {
		t.Fatalf("thread/start error: %v", startResp.Error)
	}
	var started struct {
		Thread *store.Thread `json:"thread"`
	}
	decodeResult(t, startResp, &started)
	waitForNotificationSet(t, server, "turn/completed")

	compactResp := server.HandleRequest(ctx, request("thread/compact/start", map[string]any{"threadId": started.Thread.ID}))
	if compactResp.Error != nil {
		t.Fatalf("thread/compact/start error: %v", compactResp.Error)
	}
	waitForNotificationSet(t, server, "thread/compacted")

	resumeResp := server.HandleRequest(ctx, request("thread/resume", map[string]any{
		"threadId": started.Thread.ID,
		"prompt":   "after compact prompt",
	}))
	if resumeResp.Error != nil {
		t.Fatalf("thread/resume error: %v", resumeResp.Error)
	}
	waitForNotificationSet(t, server, "turn/completed")

	if got := executions.Load(); got != 1 {
		t.Fatalf("tool executions = %d, want 1", got)
	}
	calls := model.Calls()
	if len(calls) != 3 || len(calls[2].Messages) != 2 {
		t.Fatalf("post-compact calls = %#v", calls)
	}
	assertRuntimeSystemPromptContains(t, calls[2].Messages[0], "tool call: echo", "tool result: echo", "hello")
	assertRuntimeUserPrompt(t, calls[2].Messages[1], "after compact prompt")
}

func runtimeHistoryMessageItem(id, role, text string, at time.Time) *store.Item {
	return &store.Item{
		ID:        id,
		Kind:      "message",
		Status:    "completed",
		Payload:   mustRuntimeJSON(runtimeMessagePayload{Role: role, Text: text, CreatedAt: at}),
		CreatedAt: at,
		UpdatedAt: at,
	}
}

func runtimeHistoryDynamicToolItem(id string, payload runtimeDynamicToolCallPayload, at time.Time) *store.Item {
	payload.ID = id
	return &store.Item{
		ID:        id,
		Kind:      runtimeDynamicToolCallItemKind,
		Status:    payload.Status,
		Payload:   mustRuntimeJSON(payload),
		CreatedAt: at,
		UpdatedAt: at,
	}
}

func runtimeReplayParts(t *testing.T, callMessage, resultMessage core.ModelMessage) (core.ToolCallPart, core.ToolReturnPart) {
	t.Helper()
	response, ok := callMessage.(core.ModelResponse)
	if !ok || len(response.Parts) != 1 {
		t.Fatalf("call message = %#v, want one-part model response", callMessage)
	}
	call, ok := response.Parts[0].(core.ToolCallPart)
	if !ok {
		t.Fatalf("call part = %#v, want ToolCallPart", response.Parts[0])
	}
	request, ok := resultMessage.(core.ModelRequest)
	if !ok || len(request.Parts) != 1 {
		t.Fatalf("result message = %#v, want one-part model request", resultMessage)
	}
	result, ok := request.Parts[0].(core.ToolReturnPart)
	if !ok {
		t.Fatalf("result part = %#v, want ToolReturnPart", request.Parts[0])
	}
	return call, result
}

func assertRuntimeReplayPair(t *testing.T, callMessage, resultMessage core.ModelMessage, tool, callID string, wantError bool, wantContent string) {
	t.Helper()
	response, ok := callMessage.(core.ModelResponse)
	if !ok || len(response.Parts) != 1 {
		t.Fatalf("call message = %#v, want one-part model response", callMessage)
	}
	call, ok := response.Parts[0].(core.ToolCallPart)
	if !ok || call.ToolName != tool || (callID != "" && call.ToolCallID != callID) || !json.Valid([]byte(call.ArgsJSON)) {
		t.Fatalf("call part = %#v, want tool %q id %q with valid arguments", response.Parts[0], tool, callID)
	}
	request, ok := resultMessage.(core.ModelRequest)
	if !ok || len(request.Parts) != 1 {
		t.Fatalf("result message = %#v, want one-part model request", resultMessage)
	}
	var content string
	switch part := request.Parts[0].(type) {
	case core.ToolReturnPart:
		if wantError {
			t.Fatalf("result part = %#v, want RetryPromptPart", part)
		}
		if part.ToolName != tool || part.ToolCallID != call.ToolCallID {
			t.Fatalf("result part = %#v, want paired %q/%q", part, tool, call.ToolCallID)
		}
		encoded, err := json.Marshal(part.Content)
		if err != nil {
			t.Fatalf("marshal result content: %v", err)
		}
		content = string(encoded)
	case core.RetryPromptPart:
		if !wantError {
			t.Fatalf("result part = %#v, want ToolReturnPart", part)
		}
		if part.ToolName != tool || part.ToolCallID != call.ToolCallID {
			t.Fatalf("retry part = %#v, want paired %q/%q", part, tool, call.ToolCallID)
		}
		content = part.Content
	default:
		t.Fatalf("result part = %#v, want tool return or retry", request.Parts[0])
	}
	if !strings.Contains(content, wantContent) {
		t.Fatalf("result content = %q, want substring %q", content, wantContent)
	}
}

func runtimeIntPointer(value int) *int {
	return &value
}

func runtimeTimePointer(value time.Time) *time.Time {
	return &value
}

func runtimeJSONString(value string) string {
	encoded, _ := json.Marshal(value)
	return string(encoded)
}
