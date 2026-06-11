package codetool

import (
	"context"
	"testing"

	"github.com/fugue-labs/gollem/core"
)

// driveSandwich invokes the middleware once and returns the settings the
// wrapped request actually saw.
func driveSandwich(t *testing.T, mw core.AgentMiddleware, settings *core.ModelSettings) *core.ModelSettings {
	t.Helper()
	var seen *core.ModelSettings
	next := func(_ context.Context, _ []core.ModelMessage, s *core.ModelSettings, _ *core.ModelRequestParameters) (*core.ModelResponse, error) {
		seen = s
		return &core.ModelResponse{}, nil
	}
	if _, err := mw.Request(context.Background(), nil, settings, nil, next); err != nil {
		t.Fatalf("middleware request: %v", err)
	}
	return seen
}

// TestReasoningSandwich_AdaptiveThinkingVariesEffort verifies the sandwich
// drives per-phase effort when Anthropic adaptive thinking is on — the
// model has no budget to vary, so effort IS the depth control.
func TestReasoningSandwich_AdaptiveThinkingVariesEffort(t *testing.T) {
	cfg := ReasoningSandwichConfig{
		Planning:       ReasoningLevel{ReasoningEffort: "high"},
		Implementation: ReasoningLevel{ReasoningEffort: "medium"},
		Verification:   ReasoningLevel{ReasoningEffort: "high"},
		PlanningTurns:  1,
	}
	mw := ReasoningSandwichMiddleware(cfg)
	on := true

	// Turn 1: planning.
	seen := driveSandwich(t, mw, &core.ModelSettings{AdaptiveThinking: &on})
	if seen.ReasoningEffort == nil || *seen.ReasoningEffort != "high" {
		t.Fatalf("planning turn: effort = %v, want high", seen.ReasoningEffort)
	}
	if seen.ThinkingBudget != nil {
		t.Fatalf("adaptive path must not invent a thinking budget, got %v", *seen.ThinkingBudget)
	}
	if seen.AdaptiveThinking == nil || !*seen.AdaptiveThinking {
		t.Fatal("adaptive thinking flag must survive the sandwich")
	}

	// Turn 2: implementation.
	seen = driveSandwich(t, mw, &core.ModelSettings{AdaptiveThinking: &on})
	if seen.ReasoningEffort == nil || *seen.ReasoningEffort != "medium" {
		t.Fatalf("implementation turn: effort = %v, want medium", seen.ReasoningEffort)
	}
}

// TestReasoningSandwich_PassthroughWithoutReasoning verifies settings with
// no budget, no effort, and no adaptive thinking flow through untouched.
func TestReasoningSandwich_PassthroughWithoutReasoning(t *testing.T) {
	mw := ReasoningSandwichMiddleware(DefaultReasoningSandwichConfig())
	settings := &core.ModelSettings{}
	seen := driveSandwich(t, mw, settings)
	if seen != settings {
		t.Fatal("expected identical settings pointer for passthrough")
	}
	if seen.ReasoningEffort != nil {
		t.Fatalf("passthrough must not inject effort, got %v", *seen.ReasoningEffort)
	}
}

// TestReasoningSandwich_BudgetPathStillVaries pins the pre-existing manual
// thinking-budget behavior: a configured budget is rewritten per phase and
// max_tokens keeps headroom above it.
func TestReasoningSandwich_BudgetPathStillVaries(t *testing.T) {
	cfg := ReasoningSandwichConfig{
		Planning:       ReasoningLevel{ThinkingBudget: 32000, ReasoningEffort: "high"},
		Implementation: ReasoningLevel{ThinkingBudget: 12000, ReasoningEffort: "medium"},
		Verification:   ReasoningLevel{ThinkingBudget: 32000, ReasoningEffort: "high"},
		PlanningTurns:  1,
	}
	mw := ReasoningSandwichMiddleware(cfg)
	budget := 16000
	maxTok := 20000

	seen := driveSandwich(t, mw, &core.ModelSettings{ThinkingBudget: &budget, MaxTokens: &maxTok})
	if seen.ThinkingBudget == nil || *seen.ThinkingBudget != 32000 {
		t.Fatalf("planning turn: budget = %v, want 32000", seen.ThinkingBudget)
	}
	if seen.MaxTokens == nil || *seen.MaxTokens <= 32000 {
		t.Fatalf("max_tokens must exceed the rewritten budget, got %v", seen.MaxTokens)
	}
	// The original caller settings must not be mutated.
	if budget != 16000 || maxTok != 20000 {
		t.Fatalf("caller settings mutated: budget=%d maxTokens=%d", budget, maxTok)
	}
}
