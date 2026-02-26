package core

import (
	"context"
	"strings"
	"testing"
)

// TestToolPanic_SingleTool verifies that a panicking tool handler is recovered
// and the panic is reported back to the model as an error, rather than crashing
// the entire process.
func TestToolPanic_SingleTool(t *testing.T) {
	type Params struct {
		N int `json:"n"`
	}
	panicTool := FuncTool[Params]("explode", "explodes", func(ctx context.Context, p Params) (string, error) {
		panic("tool went boom")
	})

	model := NewTestModel(
		ToolCallResponse("explode", `{"n":1}`),
		TextResponse("recovered"),
	)
	agent := NewAgent[string](model, WithTools[string](panicTool))

	result, err := agent.Run(context.Background(), "test")
	if err != nil {
		t.Fatalf("expected agent to recover from panic, got error: %v", err)
	}
	if result.Output != "recovered" {
		t.Errorf("expected 'recovered', got %q", result.Output)
	}

	// Verify the model received the panic as a tool error.
	calls := model.Calls()
	if len(calls) < 2 {
		t.Fatalf("expected at least 2 model calls, got %d", len(calls))
	}
	// The second call should contain the panic error in a tool return.
	secondCall := calls[1]
	found := false
	for _, msg := range secondCall.Messages {
		if req, ok := msg.(ModelRequest); ok {
			for _, part := range req.Parts {
				if tr, ok := part.(ToolReturnPart); ok {
					if s, ok := tr.Content.(string); ok && strings.Contains(s, "panicked") && strings.Contains(s, "tool went boom") {
						found = true
					}
				}
			}
		}
	}
	if !found {
		t.Error("expected panic error to be reported back to the model as a tool result")
	}
}

// TestToolPanic_ConcurrentTools verifies that a panic in one concurrent tool
// does not crash the process or prevent other tools from executing.
func TestToolPanic_ConcurrentTools(t *testing.T) {
	type Params struct {
		N int `json:"n"`
	}
	panicTool := FuncTool[Params]("explode", "explodes", func(ctx context.Context, p Params) (string, error) {
		panic("concurrent boom")
	})
	safeTool := FuncTool[Params]("safe", "safe tool", func(ctx context.Context, p Params) (string, error) {
		return "safe result", nil
	})

	model := NewTestModel(
		MultiToolCallResponse(
			ToolCallPart{ToolName: "explode", ArgsJSON: `{"n":1}`, ToolCallID: "call_explode"},
			ToolCallPart{ToolName: "safe", ArgsJSON: `{"n":2}`, ToolCallID: "call_safe"},
		),
		TextResponse("all done"),
	)
	agent := NewAgent[string](model, WithTools[string](panicTool, safeTool))

	result, err := agent.Run(context.Background(), "test")
	if err != nil {
		t.Fatalf("expected agent to recover from concurrent panic, got error: %v", err)
	}
	if result.Output != "all done" {
		t.Errorf("expected 'all done', got %q", result.Output)
	}

	// Verify the model received both a panic error and a successful result.
	calls := model.Calls()
	if len(calls) < 2 {
		t.Fatalf("expected at least 2 model calls, got %d", len(calls))
	}
	secondCall := calls[1]
	foundPanic := false
	foundSafe := false
	for _, msg := range secondCall.Messages {
		if req, ok := msg.(ModelRequest); ok {
			for _, part := range req.Parts {
				if tr, ok := part.(ToolReturnPart); ok {
					if s, ok := tr.Content.(string); ok {
						if strings.Contains(s, "panicked") {
							foundPanic = true
						}
						if strings.Contains(s, "safe result") {
							foundSafe = true
						}
					}
				}
			}
		}
	}
	if !foundPanic {
		t.Error("expected panic error to be reported for the exploding tool")
	}
	if !foundSafe {
		t.Error("expected safe tool result to be preserved")
	}
}

// TestToolPanic_NilPanic verifies that a nil panic is also recovered.
func TestToolPanic_NilPanic(t *testing.T) {
	type Params struct {
		N int `json:"n"`
	}
	// Trigger a nil pointer dereference (common production panic).
	panicTool := FuncTool[Params]("deref", "nil deref", func(ctx context.Context, p Params) (string, error) {
		var s *string
		return *s, nil
	})

	model := NewTestModel(
		ToolCallResponse("deref", `{"n":1}`),
		TextResponse("recovered from nil"),
	)
	agent := NewAgent[string](model, WithTools[string](panicTool))

	result, err := agent.Run(context.Background(), "test")
	if err != nil {
		t.Fatalf("expected agent to recover from nil panic, got error: %v", err)
	}
	if result.Output != "recovered from nil" {
		t.Errorf("expected 'recovered from nil', got %q", result.Output)
	}
}

// TestToolPanic_SequentialTool verifies that panic recovery works for
// sequential (non-concurrent) tools.
func TestToolPanic_SequentialTool(t *testing.T) {
	type Params struct {
		N int `json:"n"`
	}
	panicTool := FuncTool[Params]("seq_explode", "sequential explosion", func(ctx context.Context, p Params) (string, error) {
		panic("sequential boom")
	}, WithToolSequential(true))

	model := NewTestModel(
		ToolCallResponse("seq_explode", `{"n":1}`),
		TextResponse("seq recovered"),
	)
	agent := NewAgent[string](model, WithTools[string](panicTool))

	result, err := agent.Run(context.Background(), "test")
	if err != nil {
		t.Fatalf("expected agent to recover from sequential panic, got error: %v", err)
	}
	if result.Output != "seq recovered" {
		t.Errorf("expected 'seq recovered', got %q", result.Output)
	}
}
