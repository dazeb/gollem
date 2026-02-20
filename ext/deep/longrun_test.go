package deep

import (
	"context"
	"testing"

	"github.com/fugue-labs/gollem"
)

func TestLongRunAgent_Basic(t *testing.T) {
	model := gollem.NewTestModel(gollem.TextResponse("Hello from long run!"))
	agent := NewLongRunAgent[string](model)

	result, err := agent.Run(context.Background(), "Hello")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Output != "Hello from long run!" {
		t.Errorf("unexpected output: %s", result.Output)
	}
}

func TestLongRunAgent_WithPlanning(t *testing.T) {
	model := gollem.NewTestModel(
		gollem.ToolCallResponseWithID("planning", `{
			"command": "create",
			"tasks": [{"id": "1", "description": "Step 1", "status": "pending"}]
		}`, "tc1"),
		gollem.TextResponse("Plan created and executed."),
	)

	agent := NewLongRunAgent[string](model,
		WithPlanningEnabled[string](),
	)

	result, err := agent.Run(context.Background(), "Plan and execute")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Output != "Plan created and executed." {
		t.Errorf("unexpected output: %s", result.Output)
	}
}

func TestLongRunAgent_WithContextManagement(t *testing.T) {
	model := gollem.NewTestModel(gollem.TextResponse("Response after context management"))

	agent := NewLongRunAgent[string](model,
		WithContextWindow[string](100000),
		WithLongRunContextOptions[string](
			WithOffloadThreshold(20000),
			WithCompressionThreshold(0.85),
		),
	)

	result, err := agent.Run(context.Background(), "Test context management")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Output != "Response after context management" {
		t.Errorf("unexpected output: %s", result.Output)
	}
}

func TestLongRunAgent_WithAgentOptions(t *testing.T) {
	model := gollem.NewTestModel(gollem.TextResponse("With options"))

	type Params struct {
		Q string `json:"q"`
	}
	tool := gollem.FuncTool[Params]("test_tool", "A test tool",
		func(_ context.Context, p Params) (string, error) {
			return "result", nil
		},
	)

	agent := NewLongRunAgent[string](model,
		WithLongRunAgentOptions[string](
			gollem.WithTools[string](tool),
			gollem.WithSystemPrompt[string]("You are a test agent."),
		),
	)

	result, err := agent.Run(context.Background(), "Use the tool")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Output != "With options" {
		t.Errorf("unexpected output: %s", result.Output)
	}
}
