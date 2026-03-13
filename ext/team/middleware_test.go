package team

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/fugue-labs/gollem/core"
)

func TestTeamAwarenessMiddleware_NoMessages(t *testing.T) {
	mb := NewMailbox(10)
	tm := &Teammate{name: "worker", mailbox: mb}

	mw := requireRequestMiddleware(t, TeamAwarenessMiddleware(tm))
	called := false
	next := func(_ context.Context, msgs []core.ModelMessage, _ *core.ModelSettings, _ *core.ModelRequestParameters) (*core.ModelResponse, error) {
		called = true
		// No extra messages should be injected.
		if len(msgs) != 1 {
			t.Errorf("expected 1 original message, got %d", len(msgs))
		}
		return core.TextResponse("ok"), nil
	}

	original := []core.ModelMessage{
		core.ModelRequest{Parts: []core.ModelRequestPart{core.UserPromptPart{Content: "hello"}}},
	}
	_, err := mw(context.Background(), original, nil, nil, next)
	if err != nil {
		t.Fatal(err)
	}
	if !called {
		t.Error("next was not called")
	}
}

func TestTeamAwarenessMiddleware_InjectsMessages(t *testing.T) {
	mb := NewMailbox(10)
	tm := &Teammate{name: "worker", mailbox: mb}

	// Queue up messages.
	mb.Send(Message{From: "leader", Content: "do X", Type: MessageText, Timestamp: time.Now()})
	mb.Send(Message{From: "alice", Content: "found bug in Y", Type: MessageText, Timestamp: time.Now()})

	mw := requireRequestMiddleware(t, TeamAwarenessMiddleware(tm))
	var injectedMessages []core.ModelMessage
	next := func(_ context.Context, msgs []core.ModelMessage, _ *core.ModelSettings, _ *core.ModelRequestParameters) (*core.ModelResponse, error) {
		injectedMessages = msgs
		return core.TextResponse("ok"), nil
	}

	original := []core.ModelMessage{
		core.ModelRequest{Parts: []core.ModelRequestPart{core.UserPromptPart{Content: "hello"}}},
	}
	_, err := mw(context.Background(), original, nil, nil, next)
	if err != nil {
		t.Fatal(err)
	}

	// Should still have 1 message — content merged into existing ModelRequest
	// to avoid consecutive user-role messages (Anthropic 400 error).
	if len(injectedMessages) != 1 {
		t.Fatalf("expected 1 message (merged), got %d", len(injectedMessages))
	}

	// The single ModelRequest should have the original part + injected part.
	req, ok := injectedMessages[0].(core.ModelRequest)
	if !ok {
		t.Fatal("expected ModelRequest")
	}
	if len(req.Parts) != 2 {
		t.Fatalf("expected 2 parts (original + injected), got %d", len(req.Parts))
	}
	// First part: original prompt.
	origPart, ok := req.Parts[0].(core.UserPromptPart)
	if !ok {
		t.Fatal("expected UserPromptPart for original")
	}
	if origPart.Content != "hello" {
		t.Errorf("original content should be 'hello', got %q", origPart.Content)
	}
	// Second part: injected teammate messages.
	injectedPart, ok := req.Parts[1].(core.UserPromptPart)
	if !ok {
		t.Fatal("expected UserPromptPart for injected content")
	}
	if !strings.Contains(injectedPart.Content, "leader") {
		t.Error("injected content should mention 'leader'")
	}
	if !strings.Contains(injectedPart.Content, "alice") {
		t.Error("injected content should mention 'alice'")
	}
	if !strings.Contains(injectedPart.Content, "do X") {
		t.Error("injected content should contain message content 'do X'")
	}
}

func TestTeamAwarenessMiddleware_ShutdownMessage(t *testing.T) {
	mb := NewMailbox(10)
	tm := &Teammate{name: "worker", mailbox: mb}

	mb.Send(Message{From: "leader", Content: "wrapping up", Type: MessageShutdownRequest, Timestamp: time.Now()})

	mw := requireRequestMiddleware(t, TeamAwarenessMiddleware(tm))
	var injectedMessages []core.ModelMessage
	next := func(_ context.Context, msgs []core.ModelMessage, _ *core.ModelSettings, _ *core.ModelRequestParameters) (*core.ModelResponse, error) {
		injectedMessages = msgs
		return core.TextResponse("ok"), nil
	}

	original := []core.ModelMessage{
		core.ModelRequest{Parts: []core.ModelRequestPart{core.UserPromptPart{Content: "hello"}}},
	}
	_, err := mw(context.Background(), original, nil, nil, next)
	if err != nil {
		t.Fatal(err)
	}

	// Content merged into existing ModelRequest (last part).
	req := injectedMessages[0].(core.ModelRequest)
	lastPart := req.Parts[len(req.Parts)-1].(core.UserPromptPart)
	if !strings.Contains(lastPart.Content, "SHUTDOWN REQUEST") {
		t.Error("expected SHUTDOWN REQUEST in injected content")
	}
	if !strings.Contains(lastPart.Content, "IMPORTANT") {
		t.Error("expected IMPORTANT warning for shutdown")
	}
}

func TestTeamAwarenessMiddleware_DrainsOnce(t *testing.T) {
	mb := NewMailbox(10)
	tm := &Teammate{name: "worker", mailbox: mb}

	mb.Send(Message{From: "leader", Content: "task", Type: MessageText})

	mw := requireRequestMiddleware(t, TeamAwarenessMiddleware(tm))
	callCount := 0
	next := func(_ context.Context, msgs []core.ModelMessage, _ *core.ModelSettings, _ *core.ModelRequestParameters) (*core.ModelResponse, error) {
		callCount++
		return core.TextResponse("ok"), nil
	}

	original := []core.ModelMessage{
		core.ModelRequest{Parts: []core.ModelRequestPart{core.UserPromptPart{Content: "hello"}}},
	}

	// First call should inject.
	_, _ = mw(context.Background(), original, nil, nil, next)

	// Second call should pass through without injection (mailbox drained).
	var secondMsgs []core.ModelMessage
	next2 := func(_ context.Context, msgs []core.ModelMessage, _ *core.ModelSettings, _ *core.ModelRequestParameters) (*core.ModelResponse, error) {
		secondMsgs = msgs
		return core.TextResponse("ok"), nil
	}
	_, _ = mw(context.Background(), original, nil, nil, next2)

	if len(secondMsgs) != 1 {
		t.Errorf("second call should not inject messages, got %d", len(secondMsgs))
	}
}

func TestTeamAwarenessMiddleware_NoConsecutiveUserMessages(t *testing.T) {
	mb := NewMailbox(10)
	tm := &Teammate{name: "worker", mailbox: mb}

	mb.Send(Message{From: "leader", Content: "new task", Type: MessageText, Timestamp: time.Now()})

	mw := requireRequestMiddleware(t, TeamAwarenessMiddleware(tm))
	next := func(_ context.Context, msgs []core.ModelMessage, _ *core.ModelSettings, _ *core.ModelRequestParameters) (*core.ModelResponse, error) {
		// Verify no consecutive ModelRequest messages (would cause Anthropic 400).
		for i := 1; i < len(msgs); i++ {
			_, prevIsReq := msgs[i-1].(core.ModelRequest)
			_, currIsReq := msgs[i].(core.ModelRequest)
			if prevIsReq && currIsReq {
				t.Errorf("consecutive ModelRequest messages at indices %d and %d — would cause Anthropic 400 error", i-1, i)
			}
		}
		return core.TextResponse("ok"), nil
	}

	// Simulate a typical agent loop state: messages ending with a ModelRequest.
	messages := []core.ModelMessage{
		core.ModelRequest{Parts: []core.ModelRequestPart{
			core.SystemPromptPart{Content: "You are a helpful agent."},
			core.UserPromptPart{Content: "Fix the bug"},
		}},
		core.ModelResponse{Parts: []core.ModelResponsePart{
			core.ToolCallPart{ToolName: "bash", ArgsJSON: `{"cmd":"ls"}`, ToolCallID: "tc1"},
		}},
		core.ModelRequest{Parts: []core.ModelRequestPart{
			core.ToolReturnPart{ToolName: "bash", ToolCallID: "tc1", Content: "file.go"},
		}},
	}
	_, err := mw(context.Background(), messages, nil, nil, next)
	if err != nil {
		t.Fatal(err)
	}
}

func TestFormatMessagesAsPrompt_Single(t *testing.T) {
	result := formatMessagesAsPrompt([]Message{
		{From: "alice", Content: "hello world"},
	})
	expected := "[Message from alice]: hello world"
	if result != expected {
		t.Errorf("expected %q, got %q", expected, result)
	}
}

func TestFormatMessagesAsPrompt_Multiple(t *testing.T) {
	result := formatMessagesAsPrompt([]Message{
		{From: "alice", Content: "hello"},
		{From: "bob", Content: "world"},
	})
	if !strings.Contains(result, "[Message from alice]: hello") {
		t.Error("missing alice's message")
	}
	if !strings.Contains(result, "[Message from bob]: world") {
		t.Error("missing bob's message")
	}
	// Should be separated by double newline.
	if !strings.Contains(result, "\n\n") {
		t.Error("messages should be separated by double newline")
	}
}

func TestTeammateState_String(t *testing.T) {
	tests := []struct {
		state TeammateState
		want  string
	}{
		{TeammateStarting, "starting"},
		{TeammateRunning, "running"},
		{TeammateIdle, "idle"},
		{TeammateShuttingDown, "shutting_down"},
		{TeammateStopped, "stopped"},
		{TeammateState(99), "unknown"},
	}
	for _, tc := range tests {
		if got := tc.state.String(); got != tc.want {
			t.Errorf("TeammateState(%d).String() = %q, want %q", tc.state, got, tc.want)
		}
	}
}
