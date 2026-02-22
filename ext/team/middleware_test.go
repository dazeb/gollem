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

	mw := TeamAwarenessMiddleware(tm)
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

	mw := TeamAwarenessMiddleware(tm)
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

	// Should have original + injected message.
	if len(injectedMessages) != 2 {
		t.Fatalf("expected 2 messages (original + injected), got %d", len(injectedMessages))
	}

	// Second message should be the injected one with both teammate messages.
	req, ok := injectedMessages[1].(core.ModelRequest)
	if !ok {
		t.Fatal("expected ModelRequest for injected message")
	}
	if len(req.Parts) != 1 {
		t.Fatalf("expected 1 part, got %d", len(req.Parts))
	}
	userPart, ok := req.Parts[0].(core.UserPromptPart)
	if !ok {
		t.Fatal("expected UserPromptPart")
	}
	if !strings.Contains(userPart.Content, "leader") {
		t.Error("injected content should mention 'leader'")
	}
	if !strings.Contains(userPart.Content, "alice") {
		t.Error("injected content should mention 'alice'")
	}
	if !strings.Contains(userPart.Content, "do X") {
		t.Error("injected content should contain message content 'do X'")
	}
}

func TestTeamAwarenessMiddleware_ShutdownMessage(t *testing.T) {
	mb := NewMailbox(10)
	tm := &Teammate{name: "worker", mailbox: mb}

	mb.Send(Message{From: "leader", Content: "wrapping up", Type: MessageShutdownRequest, Timestamp: time.Now()})

	mw := TeamAwarenessMiddleware(tm)
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

	// The injected message should contain shutdown language.
	req := injectedMessages[1].(core.ModelRequest)
	userPart := req.Parts[0].(core.UserPromptPart)
	if !strings.Contains(userPart.Content, "SHUTDOWN REQUEST") {
		t.Error("expected SHUTDOWN REQUEST in injected content")
	}
	if !strings.Contains(userPart.Content, "IMPORTANT") {
		t.Error("expected IMPORTANT warning for shutdown")
	}
}

func TestTeamAwarenessMiddleware_DrainsOnce(t *testing.T) {
	mb := NewMailbox(10)
	tm := &Teammate{name: "worker", mailbox: mb}

	mb.Send(Message{From: "leader", Content: "task", Type: MessageText})

	mw := TeamAwarenessMiddleware(tm)
	callCount := 0
	next := func(_ context.Context, msgs []core.ModelMessage, _ *core.ModelSettings, _ *core.ModelRequestParameters) (*core.ModelResponse, error) {
		callCount++
		return core.TextResponse("ok"), nil
	}

	original := []core.ModelMessage{
		core.ModelRequest{Parts: []core.ModelRequestPart{core.UserPromptPart{Content: "hello"}}},
	}

	// First call should inject.
	mw(context.Background(), original, nil, nil, next)

	// Second call should pass through without injection (mailbox drained).
	var secondMsgs []core.ModelMessage
	next2 := func(_ context.Context, msgs []core.ModelMessage, _ *core.ModelSettings, _ *core.ModelRequestParameters) (*core.ModelResponse, error) {
		secondMsgs = msgs
		return core.TextResponse("ok"), nil
	}
	mw(context.Background(), original, nil, nil, next2)

	if len(secondMsgs) != 1 {
		t.Errorf("second call should not inject messages, got %d", len(secondMsgs))
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
