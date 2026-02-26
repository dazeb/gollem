package codetool

import (
	"context"
	"testing"

	"github.com/fugue-labs/gollem/core"
)

func TestEarlyTeamKickoffMiddleware_NudgeThenForce(t *testing.T) {
	mw := EarlyTeamKickoffMiddleware()
	ctx := context.Background()

	params := &core.ModelRequestParameters{
		FunctionTools: []core.ToolDefinition{
			{Name: "spawn_teammate"},
			{Name: "bash"},
		},
	}

	call := 0
	next := func(_ context.Context, msgs []core.ModelMessage, settings *core.ModelSettings, _ *core.ModelRequestParameters) (*core.ModelResponse, error) {
		call++
		switch call {
		case 1:
			if settings != nil && settings.ToolChoice != nil {
				t.Fatalf("turn 1 should not force tool choice, got %+v", settings.ToolChoice)
			}
			if !containsPrompt(msgs, "TEAM KICKOFF") {
				t.Fatalf("turn 1 should inject TEAM KICKOFF nudge")
			}
			return &core.ModelResponse{Parts: []core.ModelResponsePart{core.TextPart{Content: "ok"}}}, nil
		case 2:
			if settings == nil || settings.ToolChoice == nil || settings.ToolChoice.ToolName != "spawn_teammate" {
				t.Fatalf("turn 2 should force spawn_teammate, got %+v", settings)
			}
			if !containsPrompt(msgs, "TEAM KICKOFF ENFORCEMENT") {
				t.Fatalf("turn 2 should inject TEAM KICKOFF ENFORCEMENT")
			}
			return &core.ModelResponse{Parts: []core.ModelResponsePart{core.TextPart{Content: "ok"}}}, nil
		default:
			return &core.ModelResponse{Parts: []core.ModelResponsePart{core.TextPart{Content: "ok"}}}, nil
		}
	}

	messagesTurn1 := []core.ModelMessage{
		core.ModelRequest{
			Parts: []core.ModelRequestPart{
				core.UserPromptPart{Content: "solve task"},
			},
		},
	}
	if _, err := mw(ctx, messagesTurn1, nil, params, next); err != nil {
		t.Fatalf("turn 1 error: %v", err)
	}

	// Still no spawn in history, so turn 2 should force the spawn tool.
	messagesTurn2 := []core.ModelMessage{
		core.ModelRequest{
			Parts: []core.ModelRequestPart{
				core.UserPromptPart{Content: "continue"},
			},
		},
		core.ModelResponse{
			Parts: []core.ModelResponsePart{
				core.ToolCallPart{ToolName: "bash", ArgsJSON: `{"command":"ls"}`, ToolCallID: "call_1"},
			},
		},
	}
	if _, err := mw(ctx, messagesTurn2, nil, params, next); err != nil {
		t.Fatalf("turn 2 error: %v", err)
	}
}

func TestEarlyTeamKickoffMiddleware_DisablesAfterSpawn(t *testing.T) {
	mw := EarlyTeamKickoffMiddleware()
	ctx := context.Background()

	params := &core.ModelRequestParameters{
		FunctionTools: []core.ToolDefinition{
			{Name: "spawn_teammate"},
		},
	}

	messages := []core.ModelMessage{
		core.ModelRequest{
			Parts: []core.ModelRequestPart{
				core.UserPromptPart{Content: "continue"},
			},
		},
		core.ModelResponse{
			Parts: []core.ModelResponsePart{
				core.ToolCallPart{ToolName: "spawn_teammate", ArgsJSON: `{"name":"worker","task":"x"}`, ToolCallID: "call_1"},
			},
		},
	}

	next := func(_ context.Context, msgs []core.ModelMessage, settings *core.ModelSettings, _ *core.ModelRequestParameters) (*core.ModelResponse, error) {
		if settings != nil && settings.ToolChoice != nil {
			t.Fatalf("should not force tool choice after spawn, got %+v", settings.ToolChoice)
		}
		if containsPrompt(msgs, "TEAM KICKOFF") {
			t.Fatalf("should not inject kickoff prompt after spawn")
		}
		return &core.ModelResponse{Parts: []core.ModelResponsePart{core.TextPart{Content: "ok"}}}, nil
	}

	if _, err := mw(ctx, messages, nil, params, next); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestEarlyDelegateKickoffMiddleware_NudgeThenForceForComplexTask(t *testing.T) {
	mw := EarlyDelegateKickoffMiddleware()
	ctx := context.Background()

	params := &core.ModelRequestParameters{
		FunctionTools: []core.ToolDefinition{
			{Name: "delegate"},
			{Name: "bash"},
		},
	}

	complexPrompt := "Write a dependency-free C implementation with strict size and performance constraints " +
		"that parses checkpoint weights and BPE merges across multiple components."

	call := 0
	next := func(_ context.Context, msgs []core.ModelMessage, settings *core.ModelSettings, _ *core.ModelRequestParameters) (*core.ModelResponse, error) {
		call++
		switch call {
		case 1:
			if settings != nil && settings.ToolChoice != nil {
				t.Fatalf("turn 1 should not force tool choice, got %+v", settings.ToolChoice)
			}
			if !containsPrompt(msgs, "DELEGATION KICKOFF") {
				t.Fatalf("turn 1 should inject DELEGATION KICKOFF nudge")
			}
		case 2:
			if settings == nil || settings.ToolChoice == nil || settings.ToolChoice.ToolName != "delegate" {
				t.Fatalf("turn 2 should force delegate, got %+v", settings)
			}
			if !containsPrompt(msgs, "DELEGATION KICKOFF ENFORCEMENT") {
				t.Fatalf("turn 2 should inject DELEGATION KICKOFF ENFORCEMENT")
			}
		}
		return &core.ModelResponse{Parts: []core.ModelResponsePart{core.TextPart{Content: "ok"}}}, nil
	}

	messagesTurn1 := []core.ModelMessage{
		core.ModelRequest{
			Parts: []core.ModelRequestPart{
				core.UserPromptPart{Content: complexPrompt},
			},
		},
	}
	if _, err := mw(ctx, messagesTurn1, nil, params, next); err != nil {
		t.Fatalf("turn 1 error: %v", err)
	}

	messagesTurn2 := []core.ModelMessage{
		core.ModelRequest{
			Parts: []core.ModelRequestPart{
				core.UserPromptPart{Content: complexPrompt},
			},
		},
		core.ModelResponse{
			Parts: []core.ModelResponsePart{
				core.ToolCallPart{ToolName: "bash", ArgsJSON: `{"command":"ls"}`, ToolCallID: "call_1"},
			},
		},
	}
	if _, err := mw(ctx, messagesTurn2, nil, params, next); err != nil {
		t.Fatalf("turn 2 error: %v", err)
	}
}

func TestEarlyDelegateKickoffMiddleware_SkipsSimpleTask(t *testing.T) {
	mw := EarlyDelegateKickoffMiddleware()
	ctx := context.Background()

	params := &core.ModelRequestParameters{
		FunctionTools: []core.ToolDefinition{
			{Name: "delegate"},
		},
	}

	messages := []core.ModelMessage{
		core.ModelRequest{
			Parts: []core.ModelRequestPart{
				core.UserPromptPart{Content: "Fix a typo in README.md"},
			},
		},
	}

	next := func(_ context.Context, msgs []core.ModelMessage, settings *core.ModelSettings, _ *core.ModelRequestParameters) (*core.ModelResponse, error) {
		if settings != nil && settings.ToolChoice != nil {
			t.Fatalf("simple task should not force delegate, got %+v", settings.ToolChoice)
		}
		if containsPrompt(msgs, "DELEGATION KICKOFF") {
			t.Fatalf("simple task should not inject delegation kickoff prompts")
		}
		return &core.ModelResponse{Parts: []core.ModelResponsePart{core.TextPart{Content: "ok"}}}, nil
	}

	// Exercise two turns; still should not force delegation.
	if _, err := mw(ctx, messages, nil, params, next); err != nil {
		t.Fatalf("turn 1 error: %v", err)
	}
	if _, err := mw(ctx, messages, nil, params, next); err != nil {
		t.Fatalf("turn 2 error: %v", err)
	}
}

func TestEarlyDelegateKickoffMiddleware_DisablesAfterDelegate(t *testing.T) {
	mw := EarlyDelegateKickoffMiddleware()
	ctx := context.Background()

	params := &core.ModelRequestParameters{
		FunctionTools: []core.ToolDefinition{
			{Name: "delegate"},
		},
	}

	messages := []core.ModelMessage{
		core.ModelRequest{
			Parts: []core.ModelRequestPart{
				core.UserPromptPart{Content: "Complex task with strict constraints"},
			},
		},
		core.ModelResponse{
			Parts: []core.ModelResponsePart{
				core.ToolCallPart{ToolName: "delegate", ArgsJSON: `{"task":"do x"}`, ToolCallID: "call_1"},
			},
		},
	}

	next := func(_ context.Context, msgs []core.ModelMessage, settings *core.ModelSettings, _ *core.ModelRequestParameters) (*core.ModelResponse, error) {
		if settings != nil && settings.ToolChoice != nil {
			t.Fatalf("should not force delegate after it was already used, got %+v", settings.ToolChoice)
		}
		if containsPrompt(msgs, "DELEGATION KICKOFF") {
			t.Fatalf("should not inject delegation kickoff after delegate usage")
		}
		return &core.ModelResponse{Parts: []core.ModelResponsePart{core.TextPart{Content: "ok"}}}, nil
	}

	if _, err := mw(ctx, messages, nil, params, next); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func containsPrompt(messages []core.ModelMessage, needle string) bool {
	for _, msg := range messages {
		req, ok := msg.(core.ModelRequest)
		if !ok {
			continue
		}
		for _, part := range req.Parts {
			up, ok := part.(core.UserPromptPart)
			if !ok {
				continue
			}
			if up.Content != "" && contains(up.Content, needle) {
				return true
			}
		}
	}
	return false
}

func contains(s, needle string) bool {
	return len(needle) == 0 || (len(s) >= len(needle) && indexOf(s, needle) >= 0)
}

func indexOf(s, needle string) int {
	// small test helper to avoid extra imports
	if len(needle) == 0 {
		return 0
	}
	for i := 0; i+len(needle) <= len(s); i++ {
		if s[i:i+len(needle)] == needle {
			return i
		}
	}
	return -1
}
