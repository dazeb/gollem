package gollem

import (
	"context"
	"testing"
)

func TestAgentTool_Delegation(t *testing.T) {
	// Inner agent: given a prompt, returns a response.
	innerModel := NewTestModel(TextResponse("Inner agent completed the task."))
	innerAgent := NewAgent[string](innerModel)

	// Outer agent: calls inner agent as a tool.
	outerModel := NewTestModel(
		ToolCallResponseWithID("delegate", `{"prompt":"Do the inner task"}`, "tc1"),
		TextResponse("Outer done."),
	)
	outerAgent := NewAgent[string](outerModel,
		WithTools[string](AgentTool("delegate", "Delegate to inner agent", innerAgent)),
	)

	result, err := outerAgent.Run(context.Background(), "Do the task")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Output != "Outer done." {
		t.Errorf("expected 'Outer done.', got %q", result.Output)
	}
}

func TestAgentTool_ChainedDelegation(t *testing.T) {
	// Level 3: innermost agent.
	level3Model := NewTestModel(TextResponse("Level 3 result"))
	level3Agent := NewAgent[string](level3Model)

	// Level 2: calls level 3.
	level2Model := NewTestModel(
		ToolCallResponseWithID("level3", `{"prompt":"Go deeper"}`, "tc1"),
		TextResponse("Level 2 done."),
	)
	level2Agent := NewAgent[string](level2Model,
		WithTools[string](AgentTool("level3", "Call level 3", level3Agent)),
	)

	// Level 1: calls level 2.
	level1Model := NewTestModel(
		ToolCallResponseWithID("level2", `{"prompt":"Go to level 2"}`, "tc1"),
		TextResponse("Level 1 done."),
	)
	level1Agent := NewAgent[string](level1Model,
		WithTools[string](AgentTool("level2", "Call level 2", level2Agent)),
	)

	result, err := level1Agent.Run(context.Background(), "Start chain")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Output != "Level 1 done." {
		t.Errorf("expected 'Level 1 done.', got %q", result.Output)
	}
}
