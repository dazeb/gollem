package codetool

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/fugue-labs/gollem/core"
)

func TestTimeBudgetMiddleware_GreedyScalingCapsSettings(t *testing.T) {
	base := time.Unix(1_700_000_000, 0)
	now := base
	oldNow := timeBudgetNow
	timeBudgetNow = func() time.Time { return now }
	defer func() { timeBudgetNow = oldNow }()

	mw := requireRequestMiddleware(t, TimeBudgetMiddleware(100*time.Second))
	now = base.Add(92 * time.Second)

	maxTokens := 50000
	thinking := 32000
	effort := "xhigh"
	settings := &core.ModelSettings{
		MaxTokens:       &maxTokens,
		ThinkingBudget:  &thinking,
		ReasoningEffort: &effort,
	}

	var captured *core.ModelSettings
	next := func(_ context.Context, _ []core.ModelMessage, s *core.ModelSettings, _ *core.ModelRequestParameters) (*core.ModelResponse, error) {
		captured = s
		return &core.ModelResponse{}, nil
	}

	msgs := []core.ModelMessage{
		core.ModelRequest{
			Parts: []core.ModelRequestPart{
				core.UserPromptPart{Content: "solve the task"},
			},
		},
	}
	_, err := mw(context.Background(), msgs, settings, &core.ModelRequestParameters{}, next)
	if err != nil {
		t.Fatalf("middleware returned error: %v", err)
	}
	if captured == nil {
		t.Fatalf("expected settings to be passed through")
	}
	if captured.MaxTokens == nil || *captured.MaxTokens != 10000 {
		t.Fatalf("expected max_tokens cap 10000, got %+v", captured.MaxTokens)
	}
	if captured.ThinkingBudget == nil || *captured.ThinkingBudget != 4000 {
		t.Fatalf("expected thinking budget cap 4000, got %+v", captured.ThinkingBudget)
	}
	if captured.ReasoningEffort == nil || *captured.ReasoningEffort != "low" {
		t.Fatalf("expected reasoning effort low, got %+v", captured.ReasoningEffort)
	}
	if maxTokens != 50000 {
		t.Fatalf("expected original maxTokens unchanged, got %d", maxTokens)
	}
	if thinking != 32000 {
		t.Fatalf("expected original thinking budget unchanged, got %d", thinking)
	}
	if effort != "xhigh" {
		t.Fatalf("expected original reasoning effort unchanged, got %q", effort)
	}
	if captured.MaxTokens == settings.MaxTokens || captured.ThinkingBudget == settings.ThinkingBudget || captured.ReasoningEffort == settings.ReasoningEffort {
		t.Fatal("expected greedy scaling to use cloned setting pointers")
	}
}

func TestTimeBudgetMiddleware_DoesNotForceToolChoiceAtSeventyPercent(t *testing.T) {
	base := time.Unix(1_700_000_000, 0)
	now := base
	oldNow := timeBudgetNow
	timeBudgetNow = func() time.Time { return now }
	defer func() { timeBudgetNow = oldNow }()

	mw := requireRequestMiddleware(t, TimeBudgetMiddleware(200*time.Second))
	now = base.Add(140 * time.Second)

	maxTokens := 50000
	effort := "xhigh"
	settings := &core.ModelSettings{
		MaxTokens:       &maxTokens,
		ReasoningEffort: &effort,
	}
	params := &core.ModelRequestParameters{
		FunctionTools: []core.ToolDefinition{
			{Name: "delegate"},
			{Name: "bash"},
		},
	}

	var capturedMsgs []core.ModelMessage
	var capturedSettings *core.ModelSettings
	next := func(_ context.Context, msgs []core.ModelMessage, s *core.ModelSettings, _ *core.ModelRequestParameters) (*core.ModelResponse, error) {
		capturedMsgs = msgs
		capturedSettings = s
		return &core.ModelResponse{}, nil
	}

	msgs := []core.ModelMessage{
		core.ModelRequest{
			Parts: []core.ModelRequestPart{
				core.UserPromptPart{Content: "complex task"},
			},
		},
	}
	_, err := mw(context.Background(), msgs, settings, params, next)
	if err != nil {
		t.Fatalf("middleware returned error: %v", err)
	}
	if capturedSettings == nil {
		t.Fatalf("expected settings to be passed")
	}
	if capturedSettings.ToolChoice != nil {
		t.Fatalf("did not expect forced tool choice, got %+v", capturedSettings.ToolChoice)
	}
	if capturedSettings.MaxTokens == nil || *capturedSettings.MaxTokens != 28000 {
		t.Fatalf("expected max_tokens cap 28000 at 70%%, got %+v", capturedSettings.MaxTokens)
	}
	if capturedSettings.ReasoningEffort == nil || *capturedSettings.ReasoningEffort != "high" {
		t.Fatalf("expected effort capped to high at 70%%, got %+v", capturedSettings.ReasoningEffort)
	}
	if !containsInjectedPrompt(capturedMsgs, "TIME HALFWAY") {
		t.Fatalf("expected telemetry-only halfway time injection")
	}
}

func TestTimeBudgetMiddleware_DoesNotForceParallelAfterPriorDelegation(t *testing.T) {
	base := time.Unix(1_700_000_000, 0)
	now := base
	oldNow := timeBudgetNow
	timeBudgetNow = func() time.Time { return now }
	defer func() { timeBudgetNow = oldNow }()

	mw := requireRequestMiddleware(t, TimeBudgetMiddleware(200*time.Second))
	now = base.Add(140 * time.Second)

	maxTokens := 50000
	effort := "xhigh"
	settings := &core.ModelSettings{
		MaxTokens:       &maxTokens,
		ReasoningEffort: &effort,
	}
	params := &core.ModelRequestParameters{
		FunctionTools: []core.ToolDefinition{
			{Name: "delegate"},
		},
	}

	var capturedSettings *core.ModelSettings
	next := func(_ context.Context, _ []core.ModelMessage, s *core.ModelSettings, _ *core.ModelRequestParameters) (*core.ModelResponse, error) {
		capturedSettings = s
		return &core.ModelResponse{}, nil
	}

	msgs := []core.ModelMessage{
		core.ModelRequest{Parts: []core.ModelRequestPart{core.UserPromptPart{Content: "continue"}}},
		core.ModelResponse{Parts: []core.ModelResponsePart{
			core.ToolCallPart{ToolName: "delegate", ArgsJSON: `{"task":"x"}`, ToolCallID: "call_1"},
		}},
	}
	_, err := mw(context.Background(), msgs, settings, params, next)
	if err != nil {
		t.Fatalf("middleware returned error: %v", err)
	}
	if capturedSettings == nil {
		t.Fatalf("expected settings to be passed")
	}
	if capturedSettings.ToolChoice != nil {
		t.Fatalf("did not expect forced tool choice after prior delegation, got %+v", capturedSettings.ToolChoice)
	}
}

func TestTimeBudgetMiddleware_InjectsGuidanceAtHalfway(t *testing.T) {
	base := time.Unix(1_700_000_000, 0)
	now := base
	oldNow := timeBudgetNow
	timeBudgetNow = func() time.Time { return now }
	defer func() { timeBudgetNow = oldNow }()

	mw := requireRequestMiddleware(t, TimeBudgetMiddleware(200*time.Second))
	now = base.Add(120 * time.Second)

	var capturedMsgs []core.ModelMessage
	next := func(_ context.Context, msgs []core.ModelMessage, _ *core.ModelSettings, _ *core.ModelRequestParameters) (*core.ModelResponse, error) {
		capturedMsgs = msgs
		return &core.ModelResponse{}, nil
	}

	msgs := []core.ModelMessage{
		core.ModelRequest{
			Parts: []core.ModelRequestPart{
				core.UserPromptPart{Content: "keep going"},
			},
		},
	}
	_, err := mw(context.Background(), msgs, &core.ModelSettings{}, &core.ModelRequestParameters{}, next)
	if err != nil {
		t.Fatalf("middleware returned error: %v", err)
	}
	if !containsInjectedPrompt(capturedMsgs, "TIME HALFWAY") {
		t.Fatalf("expected halfway warning to be injected")
	}
	if !containsInjectedPrompt(capturedMsgs, "Half-time checkpoint") {
		t.Fatalf("expected halfway execution guidance to be injected")
	}
}

func TestTimeBudgetGuidanceThresholds(t *testing.T) {
	tests := []struct {
		name     string
		pct      float64
		contains string
	}{
		{name: "none", pct: 0.10, contains: ""},
		{name: "quarter", pct: 0.30, contains: "Quarter checkpoint"},
		{name: "halfway", pct: 0.50, contains: "Half-time checkpoint"},
		{name: "warning", pct: 0.80, contains: "Pivot now"},
		{name: "critical", pct: 0.92, contains: "Execution mode"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := timeBudgetGuidance(tt.pct)
			if tt.contains == "" {
				if got != "" {
					t.Fatalf("expected empty guidance, got %q", got)
				}
				return
			}
			if !strings.Contains(got, tt.contains) {
				t.Fatalf("expected guidance %q to contain %q", got, tt.contains)
			}
		})
	}
}

func containsInjectedPrompt(messages []core.ModelMessage, needle string) bool {
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
			if strings.Contains(up.Content, needle) {
				return true
			}
		}
	}
	return false
}
