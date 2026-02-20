package gollem

import (
	"context"
	"testing"
)

func TestHandoff_TwoAgents(t *testing.T) {
	agentA := NewAgent[string](NewTestModel(TextResponse("Step A output")))
	agentB := NewAgent[string](NewTestModel(TextResponse("Step B final")))

	pipeline := NewHandoff[string]()
	pipeline.AddStep("agent-a", agentA, nil)
	pipeline.AddStep("agent-b", agentB, func(prevOutput string) string {
		return "Continue from: " + prevOutput
	})

	result, err := pipeline.Run(context.Background(), "Start")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Output != "Step B final" {
		t.Errorf("expected 'Step B final', got %q", result.Output)
	}
}

func TestHandoff_UsageAggregation(t *testing.T) {
	agentA := NewAgent[string](NewTestModel(TextResponse("A")))
	agentB := NewAgent[string](NewTestModel(TextResponse("B")))

	pipeline := NewHandoff[string]()
	pipeline.AddStep("a", agentA, nil)
	pipeline.AddStep("b", agentB, func(_ string) string { return "B prompt" })

	result, err := pipeline.Run(context.Background(), "Start")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Both agents made one request each.
	if result.Usage.Requests != 2 {
		t.Errorf("expected 2 total requests, got %d", result.Usage.Requests)
	}
}

func TestHandoff_EmptyPipeline(t *testing.T) {
	pipeline := NewHandoff[string]()
	_, err := pipeline.Run(context.Background(), "Start")
	if err == nil {
		t.Fatal("expected error for empty pipeline")
	}
}

func TestHandoff_ThreeAgents(t *testing.T) {
	agentA := NewAgent[string](NewTestModel(TextResponse("A result")))
	agentB := NewAgent[string](NewTestModel(TextResponse("B result")))
	agentC := NewAgent[string](NewTestModel(TextResponse("C final")))

	pipeline := NewHandoff[string]()
	pipeline.AddStep("a", agentA, nil)
	pipeline.AddStep("b", agentB, func(prev string) string { return "From A: " + prev })
	pipeline.AddStep("c", agentC, func(prev string) string { return "From B: " + prev })

	result, err := pipeline.Run(context.Background(), "Initial")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Output != "C final" {
		t.Errorf("expected 'C final', got %q", result.Output)
	}
	if result.Usage.Requests != 3 {
		t.Errorf("expected 3 requests, got %d", result.Usage.Requests)
	}
}
