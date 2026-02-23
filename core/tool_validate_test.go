package core

import (
	"context"
	"errors"
	"strings"
	"testing"
)

func TestToolResultValidator_Rejects(t *testing.T) {
	type Params struct {
		Query string `json:"query"`
	}
	searchTool := FuncTool[Params]("search", "search", func(ctx context.Context, p Params) (string, error) {
		return "bad result with SENSITIVE data", nil
	}, WithToolResultValidator(func(ctx context.Context, rc *RunContext, toolName string, result string) error {
		if strings.Contains(result, "SENSITIVE") {
			return errors.New("result contains sensitive data")
		}
		return nil
	}))

	model := NewTestModel(
		ToolCallResponse("search", `{"query":"test"}`),
		TextResponse("done"),
	)
	agent := NewAgent[string](model, WithTools[string](searchTool))

	result, err := agent.Run(context.Background(), "search for something")
	if err != nil {
		t.Fatal(err)
	}

	// The model should have received a retry prompt about validation.
	calls := model.Calls()
	if len(calls) < 2 {
		t.Fatal("expected at least 2 model calls (initial + retry)")
	}

	// Check that the retry message mentions validation.
	lastCall := calls[1]
	found := false
	for _, msg := range lastCall.Messages {
		if req, ok := msg.(ModelRequest); ok {
			for _, part := range req.Parts {
				if rp, ok := part.(RetryPromptPart); ok {
					if strings.Contains(rp.Content, "validation failed") {
						found = true
					}
				}
			}
		}
	}
	if !found {
		t.Error("expected retry prompt about validation failure")
	}
	_ = result
}

func TestToolResultValidator_Passes(t *testing.T) {
	type Params struct {
		Query string `json:"query"`
	}
	searchTool := FuncTool[Params]("search", "search", func(ctx context.Context, p Params) (string, error) {
		return "clean result", nil
	}, WithToolResultValidator(func(ctx context.Context, rc *RunContext, toolName string, result string) error {
		// Always passes.
		return nil
	}))

	model := NewTestModel(
		ToolCallResponse("search", `{"query":"test"}`),
		TextResponse("done"),
	)
	agent := NewAgent[string](model, WithTools[string](searchTool))

	result, err := agent.Run(context.Background(), "test")
	if err != nil {
		t.Fatal(err)
	}
	if result.Output != "done" {
		t.Errorf("expected 'done', got %q", result.Output)
	}
}

func TestToolResultValidator_PerTool(t *testing.T) {
	type Params struct {
		N int `json:"n"`
	}
	// Tool with validator.
	validatedTool := FuncTool[Params]("validated", "validated tool", func(ctx context.Context, p Params) (string, error) {
		return "validated result", nil
	}, WithToolResultValidator(func(ctx context.Context, rc *RunContext, toolName string, result string) error {
		if toolName != "validated" {
			return errors.New("unexpected tool name")
		}
		return nil
	}))

	// Tool without validator.
	plainTool := FuncTool[Params]("plain", "plain tool", func(ctx context.Context, p Params) (string, error) {
		return "plain result", nil
	})

	model := NewTestModel(
		ToolCallResponse("plain", `{"n":1}`),
		ToolCallResponse("validated", `{"n":2}`),
		TextResponse("done"),
	)
	agent := NewAgent[string](model, WithTools[string](validatedTool, plainTool))

	result, err := agent.Run(context.Background(), "test")
	if err != nil {
		t.Fatal(err)
	}
	if result.Output != "done" {
		t.Errorf("expected 'done', got %q", result.Output)
	}
}

func TestToolResultValidator_Global(t *testing.T) {
	type Params struct {
		N int `json:"n"`
	}
	tool1 := FuncTool[Params]("tool1", "tool1", func(ctx context.Context, p Params) (string, error) {
		return "result1", nil
	})
	tool2 := FuncTool[Params]("tool2", "tool2", func(ctx context.Context, p Params) (string, error) {
		return "result2", nil
	})

	var validatedTools []string
	model := NewTestModel(
		ToolCallResponse("tool1", `{"n":1}`),
		ToolCallResponse("tool2", `{"n":2}`),
		TextResponse("done"),
	)
	agent := NewAgent[string](model,
		WithTools[string](tool1, tool2),
		WithGlobalToolResultValidator[string](func(ctx context.Context, rc *RunContext, toolName string, result string) error {
			validatedTools = append(validatedTools, toolName)
			return nil
		}),
	)

	_, err := agent.Run(context.Background(), "test")
	if err != nil {
		t.Fatal(err)
	}

	// Global validator should have been called for both tools.
	if len(validatedTools) != 2 {
		t.Fatalf("expected 2 validated tools, got %d: %v", len(validatedTools), validatedTools)
	}
}

func TestToolResultValidator_Combined(t *testing.T) {
	type Params struct {
		N int `json:"n"`
	}

	perToolCalled := false
	globalCalled := false

	tool := FuncTool[Params]("mytool", "my tool", func(ctx context.Context, p Params) (string, error) {
		return "result", nil
	}, WithToolResultValidator(func(ctx context.Context, rc *RunContext, toolName string, result string) error {
		perToolCalled = true
		return nil
	}))

	model := NewTestModel(
		ToolCallResponse("mytool", `{"n":1}`),
		TextResponse("done"),
	)
	agent := NewAgent[string](model,
		WithTools[string](tool),
		WithGlobalToolResultValidator[string](func(ctx context.Context, rc *RunContext, toolName string, result string) error {
			globalCalled = true
			return nil
		}),
	)

	_, err := agent.Run(context.Background(), "test")
	if err != nil {
		t.Fatal(err)
	}

	if !perToolCalled {
		t.Error("per-tool validator not called")
	}
	if !globalCalled {
		t.Error("global validator not called")
	}
}

// TestToolResultValidator_IncrementsRetryCounter verifies that result validator
// failures increment the retry counter and eventually produce an "exceeded
// maximum retries" message, matching ModelRetryError behavior.
func TestToolResultValidator_IncrementsRetryCounter(t *testing.T) {
	type Params struct {
		Query string `json:"query"`
	}
	var retryValues []int
	tool := FuncTool[Params]("search", "search", func(ctx context.Context, p Params) (string, error) {
		return "always bad", nil
	}, WithToolResultValidator(func(ctx context.Context, rc *RunContext, toolName string, result string) error {
		retryValues = append(retryValues, rc.Retry)
		return errors.New("result is invalid")
	}))

	model := NewTestModel(
		ToolCallResponse("search", `{"query":"a"}`),
		ToolCallResponse("search", `{"query":"b"}`),
		ToolCallResponse("search", `{"query":"c"}`),
		ToolCallResponse("search", `{"query":"d"}`),
		TextResponse("gave up"),
	)
	agent := NewAgent[string](model,
		WithTools[string](tool),
		WithMaxRetries[string](2),
	)

	_, err := agent.Run(context.Background(), "search for something")
	if err != nil {
		t.Fatal(err)
	}

	// Verify the retry counter was incremented across validator failures.
	// Without the fix, rc.Retry would always be 0.
	if len(retryValues) < 3 {
		t.Fatalf("expected at least 3 validator calls, got %d", len(retryValues))
	}
	// First call: retry=0, second: retry=1, third: retry=2.
	for i, v := range retryValues {
		if v != i {
			t.Errorf("validator call %d: rc.Retry = %d, want %d", i, v, i)
		}
	}

	// Verify the "exceeded maximum retries" message was sent to the model.
	// Without the fix, this message was never produced for validator failures.
	calls := model.Calls()
	found := false
	for _, call := range calls {
		for _, msg := range call.Messages {
			if req, ok := msg.(ModelRequest); ok {
				for _, part := range req.Parts {
					if rp, ok := part.(RetryPromptPart); ok {
						if strings.Contains(rp.Content, "exceeded maximum retries") {
							found = true
						}
					}
				}
			}
		}
	}
	if !found {
		t.Error("expected 'exceeded maximum retries' message after persistent validator failures")
	}
}
