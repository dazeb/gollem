package core

import (
	"context"
	"testing"
)

func TestRunWithReflection_AcceptedFirst(t *testing.T) {
	model := NewTestModel(TextResponse("correct answer"))
	agent := NewAgent[string](model)

	result, err := RunWithReflection(context.Background(), agent, "test prompt",
		func(_ context.Context, output string) (string, error) {
			return "", nil // Accept immediately
		},
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.Accepted {
		t.Error("expected output to be accepted")
	}
	if result.Iterations != 1 {
		t.Errorf("expected 1 iteration, got %d", result.Iterations)
	}
	if result.Output != "correct answer" {
		t.Errorf("expected 'correct answer', got %q", result.Output)
	}
}

func TestRunWithReflection_CorrectedOnSecond(t *testing.T) {
	model := NewTestModel(
		TextResponse("wrong answer"),
		TextResponse("correct answer"),
	)
	agent := NewAgent[string](model)

	callCount := 0
	result, err := RunWithReflection(context.Background(), agent, "test prompt",
		func(_ context.Context, output string) (string, error) {
			callCount++
			if output == "wrong answer" {
				return "The answer should be 'correct answer'", nil
			}
			return "", nil // Accept
		},
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.Accepted {
		t.Error("expected output to be accepted")
	}
	if result.Iterations != 2 {
		t.Errorf("expected 2 iterations, got %d", result.Iterations)
	}
	if result.Output != "correct answer" {
		t.Errorf("expected 'correct answer', got %q", result.Output)
	}
}

func TestRunWithReflection_MaxReflections(t *testing.T) {
	// Model always gives wrong answer
	model := NewTestModel(
		TextResponse("wrong 1"),
		TextResponse("wrong 2"),
		TextResponse("wrong 3"),
		TextResponse("wrong 4"),
	)
	agent := NewAgent[string](model)

	result, err := RunWithReflection(context.Background(), agent, "test prompt",
		func(_ context.Context, _ string) (string, error) {
			return "still wrong", nil // Never accept
		},
		WithMaxReflections(2),
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Accepted {
		t.Error("expected output to NOT be accepted after max reflections")
	}
	if result.Iterations != 3 { // initial + 2 reflections
		t.Errorf("expected 3 iterations, got %d", result.Iterations)
	}
}

func TestRunWithReflection_UsageAggregation(t *testing.T) {
	model := NewTestModel(
		TextResponse("first"),
		TextResponse("second"),
	)
	agent := NewAgent[string](model)

	callCount := 0
	result, err := RunWithReflection(context.Background(), agent, "test prompt",
		func(_ context.Context, _ string) (string, error) {
			callCount++
			if callCount < 2 {
				return "try again", nil
			}
			return "", nil
		},
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Usage.Requests != 2 {
		t.Errorf("expected 2 requests in usage, got %d", result.Usage.Requests)
	}
}

func TestRunWithReflection_CustomPrompt(t *testing.T) {
	model := NewTestModel(
		TextResponse("first"),
		TextResponse("second"),
	)
	agent := NewAgent[string](model)

	callCount := 0
	_, err := RunWithReflection(context.Background(), agent, "test prompt",
		func(_ context.Context, _ string) (string, error) {
			callCount++
			if callCount < 2 {
				return "feedback", nil
			}
			return "", nil
		},
		WithReflectPrompt("CUSTOM: %s"),
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// The test just verifies it doesn't panic with custom prompt
}

// TestRunWithReflection_PreservesConversationHistory verifies that reflection
// iterations pass prior conversation history to the agent. Without this, the model
// on iteration 2+ only sees the correction prompt with zero context about the
// original task — it doesn't know what it was supposed to produce.
func TestRunWithReflection_PreservesConversationHistory(t *testing.T) {
	model := NewTestModel(
		TextResponse("wrong answer"),
		TextResponse("correct answer"),
	)
	agent := NewAgent[string](model)

	callCount := 0
	_, err := RunWithReflection(context.Background(), agent, "Write a fibonacci function",
		func(_ context.Context, output string) (string, error) {
			callCount++
			if callCount < 2 {
				return "Missing memoization", nil
			}
			return "", nil
		},
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	calls := model.Calls()
	if len(calls) < 2 {
		t.Fatalf("expected at least 2 model calls, got %d", len(calls))
	}

	// The second model call should have more messages than the first,
	// because it should include the conversation history from iteration 1.
	// Without the fix, both calls would have exactly 1 message (just the prompt).
	firstCallMsgs := len(calls[0].Messages)
	secondCallMsgs := len(calls[1].Messages)

	if secondCallMsgs <= firstCallMsgs {
		t.Errorf("second call should have more messages than first (conversation history):\n"+
			"  first call: %d messages\n  second call: %d messages",
			firstCallMsgs, secondCallMsgs)
	}

	// Verify the original prompt appears in the second call's history.
	found := false
	for _, msg := range calls[1].Messages {
		if req, ok := msg.(ModelRequest); ok {
			for _, part := range req.Parts {
				if up, ok := part.(UserPromptPart); ok {
					if up.Content == "Write a fibonacci function" {
						found = true
					}
				}
			}
		}
	}
	if !found {
		t.Error("second call should contain the original prompt in conversation history")
	}
}

func TestRunWithReflection_StructuredOutput(t *testing.T) {
	model := NewTestModel(
		TextResponse("first attempt"),
		TextResponse("second attempt"),
	)
	agent := NewAgent[string](model)

	callCount := 0
	result, err := RunWithReflection(context.Background(), agent, "compute 2+2",
		func(_ context.Context, output string) (string, error) {
			callCount++
			if callCount < 2 {
				return "please correct", nil
			}
			return "", nil
		},
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Output != "second attempt" {
		t.Errorf("expected 'second attempt', got %q", result.Output)
	}
}
