package appserver

import (
	"context"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/fugue-labs/gollem/appserver/protocol"
	"github.com/fugue-labs/gollem/appserver/store"
	"github.com/fugue-labs/gollem/core"
)

func TestServerRuntimeThreadStartCompletesTurn(t *testing.T) {
	ctx := context.Background()
	st := newRuntimeTestStore(t)
	model := core.NewTestModel(core.TextResponse("runtime answer"))
	server := readyServer(
		WithStore(st),
		WithRuntimeService(NewRuntimeService(WithRuntimeModel(model, RuntimeModelInfo{ProviderID: "test", Model: "test-model"}))),
	)

	resp := server.HandleRequest(ctx, request("thread/start", map[string]any{
		"title":    "Runtime",
		"prompt":   "hello runtime",
		"provider": "test",
		"model":    "test-model",
	}))
	if resp.Error != nil {
		t.Fatalf("thread/start error: %v", resp.Error)
	}
	var started struct {
		Thread *store.Thread `json:"thread"`
		Turn   *store.Turn   `json:"turn"`
	}
	decodeResult(t, resp, &started)
	if started.Thread.ID == "" || started.Turn.ID == "" || started.Turn.Status != store.TurnRunning {
		t.Fatalf("thread/start result = %#v", started)
	}

	waitForNotificationSet(t, server, "thread/started", "turn/started", "item/agentMessage/delta", "item/completed", "turn/completed")
	readResp := server.HandleRequest(ctx, request("thread/read", map[string]any{"threadId": started.Thread.ID}))
	if readResp.Error != nil {
		t.Fatalf("thread/read error: %v", readResp.Error)
	}
	var read threadReadResult
	decodeResult(t, readResp, &read)
	if len(read.Turns) != 1 || read.Turns[0].Status != store.TurnCompleted {
		t.Fatalf("turns = %#v", read.Turns)
	}
	if len(read.Items) != 2 {
		t.Fatalf("items = %#v", read.Items)
	}
	if got := runtimePromptFromInput(read.Turns[0].Input); got != "hello runtime" {
		t.Fatalf("turn input prompt = %q", got)
	}
	if len(model.Calls()) != 1 {
		t.Fatalf("model calls = %d, want 1", len(model.Calls()))
	}
}

func TestServerRuntimeThreadResumeUsesPersistedHistory(t *testing.T) {
	ctx := context.Background()
	st := newRuntimeTestStore(t)
	model := core.NewTestModel(core.TextResponse("first answer"), core.TextResponse("second answer"))
	server := readyServer(
		WithStore(st),
		WithRuntimeService(NewRuntimeService(WithRuntimeModel(model, RuntimeModelInfo{ProviderID: "test", Model: "test-model"}))),
	)

	startResp := server.HandleRequest(ctx, request("thread/start", map[string]any{"prompt": "first prompt"}))
	if startResp.Error != nil {
		t.Fatalf("thread/start error: %v", startResp.Error)
	}
	var started struct {
		Thread *store.Thread `json:"thread"`
		Turn   *store.Turn   `json:"turn"`
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
	waitForNotificationSet(t, server, "turn/completed")

	calls := model.Calls()
	if len(calls) != 2 {
		t.Fatalf("model calls = %d, want 2", len(calls))
	}
	secondMessages := calls[1].Messages
	if len(secondMessages) != 3 {
		t.Fatalf("second call messages = %#v", secondMessages)
	}
	assertRuntimeUserPrompt(t, secondMessages[0], "first prompt")
	assertRuntimeAssistantText(t, secondMessages[1], "first answer")
	assertRuntimeUserPrompt(t, secondMessages[2], "second prompt")
}

func TestServerRuntimeThreadResumeUsesInjectedResponseItems(t *testing.T) {
	ctx := context.Background()
	st := newRuntimeTestStore(t)
	model := core.NewTestModel(core.TextResponse("after injection"))
	server := readyServer(
		WithStore(st),
		WithRuntimeService(NewRuntimeService(WithRuntimeModel(model, RuntimeModelInfo{ProviderID: "test", Model: "test-model"}))),
	)
	thread, err := st.CreateThread(ctx, store.CreateThreadRequest{Title: "Injected history"})
	if err != nil {
		t.Fatalf("CreateThread: %v", err)
	}

	injectResp := server.HandleRequest(ctx, request("thread/inject_items", map[string]any{
		"threadId": thread.ID,
		"items": []any{
			map[string]any{
				"type": "message",
				"role": "user",
				"content": []any{
					map[string]any{"type": "input_text", "text": "injected user"},
				},
			},
			map[string]any{
				"type":  "message",
				"role":  "assistant",
				"model": "prior-model",
				"content": []any{
					map[string]any{"type": "output_text", "text": "injected assistant"},
				},
			},
		},
	}))
	if injectResp.Error != nil {
		t.Fatalf("thread/inject_items error: %v", injectResp.Error)
	}
	server.DrainNotifications()

	resumeResp := server.HandleRequest(ctx, request("thread/resume", map[string]any{
		"threadId": thread.ID,
		"prompt":   "next prompt",
	}))
	if resumeResp.Error != nil {
		t.Fatalf("thread/resume error: %v", resumeResp.Error)
	}
	waitForNotificationSet(t, server, "turn/completed")

	calls := model.Calls()
	if len(calls) != 1 {
		t.Fatalf("model calls = %d, want 1", len(calls))
	}
	messages := calls[0].Messages
	if len(messages) != 3 {
		t.Fatalf("messages = %#v", messages)
	}
	assertRuntimeUserPrompt(t, messages[0], "injected user")
	assertRuntimeAssistantText(t, messages[1], "injected assistant")
	assertRuntimeUserPrompt(t, messages[2], "next prompt")
}

func TestServerRuntimeThreadCompactBoundsResumeHistory(t *testing.T) {
	ctx := context.Background()
	st := newRuntimeTestStore(t)
	model := core.NewTestModel(
		core.TextResponse("first answer"),
		core.TextResponse("second answer"),
		core.TextResponse("third answer"),
	)
	server := readyServer(
		WithStore(st),
		WithRuntimeService(NewRuntimeService(WithRuntimeModel(model, RuntimeModelInfo{ProviderID: "test", Model: "test-model"}))),
	)

	startResp := server.HandleRequest(ctx, request("thread/start", map[string]any{"prompt": "first prompt"}))
	if startResp.Error != nil {
		t.Fatalf("thread/start error: %v", startResp.Error)
	}
	var started struct {
		Thread *store.Thread `json:"thread"`
		Turn   *store.Turn   `json:"turn"`
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
	waitForNotificationSet(t, server, "turn/completed")

	compactResp := server.HandleRequest(ctx, request("thread/compact/start", map[string]any{"threadId": started.Thread.ID}))
	if compactResp.Error != nil {
		t.Fatalf("thread/compact/start error: %v", compactResp.Error)
	}
	waitForNotificationSet(t, server, "thread/compacted")

	resumeResp = server.HandleRequest(ctx, request("thread/resume", map[string]any{
		"threadId": started.Thread.ID,
		"prompt":   "third prompt",
	}))
	if resumeResp.Error != nil {
		t.Fatalf("thread/resume third error: %v", resumeResp.Error)
	}
	waitForNotificationSet(t, server, "turn/completed")

	calls := model.Calls()
	if len(calls) != 3 {
		t.Fatalf("model calls = %d, want 3", len(calls))
	}
	messages := calls[2].Messages
	if len(messages) != 2 {
		t.Fatalf("third call messages = %#v", messages)
	}
	assertRuntimeSystemPromptContains(t, messages[0], "first prompt", "second answer")
	assertRuntimeUserPrompt(t, messages[1], "third prompt")
}

func TestServerRuntimeTurnRetryBranchesBeforeSourceTurn(t *testing.T) {
	ctx := context.Background()
	st := newRuntimeTestStore(t)
	model := core.NewTestModel(core.TextResponse("first answer"), core.TextResponse("retry answer"))
	server := readyServer(
		WithStore(st),
		WithRuntimeService(NewRuntimeService(WithRuntimeModel(model, RuntimeModelInfo{ProviderID: "test", Model: "test-model"}))),
	)

	startResp := server.HandleRequest(ctx, request("thread/start", map[string]any{"prompt": "original prompt"}))
	if startResp.Error != nil {
		t.Fatalf("thread/start error: %v", startResp.Error)
	}
	var started struct {
		Turn *store.Turn `json:"turn"`
	}
	decodeResult(t, startResp, &started)
	waitForNotificationSet(t, server, "turn/completed")

	retryResp := server.HandleRequest(ctx, request("turn/retry", map[string]any{"turnId": started.Turn.ID}))
	if retryResp.Error != nil {
		t.Fatalf("turn/retry error: %v", retryResp.Error)
	}
	waitForNotificationSet(t, server, "turn/completed")

	calls := model.Calls()
	if len(calls) != 2 {
		t.Fatalf("model calls = %d, want 2", len(calls))
	}
	if len(calls[1].Messages) != 1 {
		t.Fatalf("retry messages = %#v", calls[1].Messages)
	}
	assertRuntimeUserPrompt(t, calls[1].Messages[0], "original prompt")
}

func TestServerRuntimeTurnInterruptCancelsActiveRun(t *testing.T) {
	ctx := context.Background()
	st := newRuntimeTestStore(t)
	model := &blockingRuntimeModel{started: make(chan struct{})}
	server := readyServer(
		WithStore(st),
		WithRuntimeService(NewRuntimeService(WithRuntimeModel(model, RuntimeModelInfo{ProviderID: "test", Model: "blocking"}))),
	)

	startResp := server.HandleRequest(ctx, request("thread/start", map[string]any{"prompt": "block"}))
	if startResp.Error != nil {
		t.Fatalf("thread/start error: %v", startResp.Error)
	}
	var started struct {
		Thread *store.Thread `json:"thread"`
		Turn   *store.Turn   `json:"turn"`
	}
	decodeResult(t, startResp, &started)
	waitForBlockingModel(t, model)

	interruptResp := server.HandleRequest(ctx, request("turn/interrupt", map[string]any{"turnId": started.Turn.ID}))
	if interruptResp.Error != nil {
		t.Fatalf("turn/interrupt error: %v", interruptResp.Error)
	}
	waitForNotificationSet(t, server, "turn/completed")
	turn, err := st.GetTurn(ctx, started.Turn.ID)
	if err != nil {
		t.Fatalf("GetTurn: %v", err)
	}
	if turn.Status != store.TurnInterrupted {
		t.Fatalf("turn status = %s, want interrupted; error=%q", turn.Status, turn.Error)
	}
}

func TestServerRuntimeTurnSteerRecordsActiveMessage(t *testing.T) {
	ctx := context.Background()
	st := newRuntimeTestStore(t)
	model := &blockingRuntimeModel{started: make(chan struct{})}
	server := readyServer(
		WithStore(st),
		WithRuntimeService(NewRuntimeService(WithRuntimeModel(model, RuntimeModelInfo{ProviderID: "test", Model: "blocking"}))),
	)

	startResp := server.HandleRequest(ctx, request("thread/start", map[string]any{"prompt": "block"}))
	if startResp.Error != nil {
		t.Fatalf("thread/start error: %v", startResp.Error)
	}
	var started struct {
		Turn *store.Turn `json:"turn"`
	}
	decodeResult(t, startResp, &started)
	waitForBlockingModel(t, model)

	steerResp := server.HandleRequest(ctx, request("turn/steer", map[string]any{
		"turnId":  started.Turn.ID,
		"message": "adjust course",
	}))
	if steerResp.Error != nil {
		t.Fatalf("turn/steer error: %v", steerResp.Error)
	}
	var steer struct {
		Accepted bool        `json:"accepted"`
		Item     *store.Item `json:"item"`
	}
	decodeResult(t, steerResp, &steer)
	if !steer.Accepted || steer.Item == nil || steer.Item.Kind != "steer" || steer.Item.Status != "queued" {
		t.Fatalf("turn/steer result = %#v", steer)
	}

	interruptResp := server.HandleRequest(ctx, request("turn/interrupt", map[string]any{"turnId": started.Turn.ID}))
	if interruptResp.Error != nil {
		t.Fatalf("turn/interrupt error: %v", interruptResp.Error)
	}
	waitForNotificationSet(t, server, "turn/completed")
}

func newRuntimeTestStore(t *testing.T) *store.SQLiteStore {
	t.Helper()
	st, err := store.NewSQLiteStore(filepath.Join(t.TempDir(), "runtime.db"))
	if err != nil {
		t.Fatalf("NewSQLiteStore: %v", err)
	}
	t.Cleanup(func() { _ = st.Close() })
	return st
}

func waitForNotificationSet(t *testing.T, server *Server, want ...string) []protocol.Notification {
	t.Helper()
	remaining := make(map[string]int, len(want))
	for _, method := range want {
		remaining[method]++
	}
	var seen []protocol.Notification
	timeout := time.After(3 * time.Second)
	for len(remaining) > 0 {
		select {
		case <-server.NotificationSignal():
			for _, notification := range server.DrainNotifications() {
				seen = append(seen, notification)
				if remaining[notification.Method] > 1 {
					remaining[notification.Method]--
				} else if remaining[notification.Method] == 1 {
					delete(remaining, notification.Method)
				}
			}
		case <-timeout:
			t.Fatalf("timed out waiting for %v; seen=%v", remaining, notificationMethods(seen))
		}
	}
	return seen
}

func notificationMethods(notifications []protocol.Notification) []string {
	methods := make([]string, len(notifications))
	for i, notification := range notifications {
		methods[i] = notification.Method
	}
	return methods
}

func assertRuntimeUserPrompt(t *testing.T, message core.ModelMessage, want string) {
	t.Helper()
	req, ok := message.(core.ModelRequest)
	if !ok || len(req.Parts) != 1 {
		t.Fatalf("message = %#v, want one-part user request", message)
	}
	part, ok := req.Parts[0].(core.UserPromptPart)
	if !ok || part.Content != want {
		t.Fatalf("request part = %#v, want user prompt %q", req.Parts[0], want)
	}
}

func assertRuntimeSystemPromptContains(t *testing.T, message core.ModelMessage, want ...string) {
	t.Helper()
	req, ok := message.(core.ModelRequest)
	if !ok || len(req.Parts) != 1 {
		t.Fatalf("message = %#v, want one-part system request", message)
	}
	part, ok := req.Parts[0].(core.SystemPromptPart)
	if !ok {
		t.Fatalf("request part = %#v, want system prompt", req.Parts[0])
	}
	for _, text := range want {
		if !strings.Contains(part.Content, text) {
			t.Fatalf("system prompt = %q, want substring %q", part.Content, text)
		}
	}
}

func assertRuntimeAssistantText(t *testing.T, message core.ModelMessage, want string) {
	t.Helper()
	resp, ok := message.(core.ModelResponse)
	if !ok || resp.TextContent() != want {
		t.Fatalf("message = %#v, want assistant text %q", message, want)
	}
}

type blockingRuntimeModel struct {
	started chan struct{}
}

func (m *blockingRuntimeModel) Request(ctx context.Context, _ []core.ModelMessage, _ *core.ModelSettings, _ *core.ModelRequestParameters) (*core.ModelResponse, error) {
	<-ctx.Done()
	return nil, ctx.Err()
}

func (m *blockingRuntimeModel) RequestStream(ctx context.Context, _ []core.ModelMessage, _ *core.ModelSettings, _ *core.ModelRequestParameters) (core.StreamedResponse, error) {
	close(m.started)
	<-ctx.Done()
	return nil, ctx.Err()
}

func (m *blockingRuntimeModel) ModelName() string {
	return "blocking"
}

func waitForBlockingModel(t *testing.T, model *blockingRuntimeModel) {
	t.Helper()
	select {
	case <-model.started:
	case <-time.After(2 * time.Second):
		t.Fatal("blocking model did not start")
	}
}
