package modelutil

import (
	"context"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/fugue-labs/gollem/core"
)

func TestGeneratePersonality_Basic(t *testing.T) {
	model := core.NewTestModel(core.TextResponse("You are an expert Go developer specializing in API design."))
	gen := GeneratePersonality(model)

	result, err := gen(context.Background(), PersonalityRequest{
		Task: "Implement a REST API handler for user authentication",
		Role: "backend developer",
	})
	if err != nil {
		t.Fatal(err)
	}
	if result == "" {
		t.Error("expected non-empty generated personality")
	}
	if result != "You are an expert Go developer specializing in API design." {
		t.Errorf("unexpected result: %q", result)
	}

	// Verify the model received the right prompt structure.
	calls := model.Calls()
	if len(calls) != 1 {
		t.Fatalf("expected 1 model call, got %d", len(calls))
	}
}

func TestGeneratePersonality_EmptyTask(t *testing.T) {
	model := core.NewTestModel(core.TextResponse("should not be called"))
	gen := GeneratePersonality(model)

	_, err := gen(context.Background(), PersonalityRequest{})
	if err == nil {
		t.Error("expected error for empty task")
	}
}

func TestGeneratePersonality_WithBasePrompt(t *testing.T) {
	model := core.NewTestModel(core.TextResponse("Extended prompt with base."))
	gen := GeneratePersonality(model)

	result, err := gen(context.Background(), PersonalityRequest{
		Task:       "Write unit tests",
		BasePrompt: "You are a focused coding assistant.",
	})
	if err != nil {
		t.Fatal(err)
	}
	if result != "Extended prompt with base." {
		t.Errorf("unexpected result: %q", result)
	}

	// Verify the meta-prompt includes the base prompt.
	calls := model.Calls()
	req := calls[0].Messages[0].(core.ModelRequest)
	userPart := req.Parts[1].(core.UserPromptPart)
	if !strings.Contains(userPart.Content, "You are a focused coding assistant.") {
		t.Error("meta-prompt should include the base prompt")
	}
}

func TestGeneratePersonality_WithConstraints(t *testing.T) {
	model := core.NewTestModel(core.TextResponse("Constrained prompt."))
	gen := GeneratePersonality(model)

	_, err := gen(context.Background(), PersonalityRequest{
		Task:        "Refactor auth module",
		Constraints: []string{"never modify tests", "Go only"},
	})
	if err != nil {
		t.Fatal(err)
	}

	calls := model.Calls()
	req := calls[0].Messages[0].(core.ModelRequest)
	userPart := req.Parts[1].(core.UserPromptPart)
	if !strings.Contains(userPart.Content, "never modify tests") {
		t.Error("meta-prompt should include constraints")
	}
	if !strings.Contains(userPart.Content, "Go only") {
		t.Error("meta-prompt should include all constraints")
	}
}

func TestGeneratePersonality_WithContext(t *testing.T) {
	model := core.NewTestModel(core.TextResponse("Context-aware prompt."))
	gen := GeneratePersonality(model)

	_, err := gen(context.Background(), PersonalityRequest{
		Task:    "Fix bug in handler",
		Context: map[string]string{"language": "Go", "framework": "net/http"},
	})
	if err != nil {
		t.Fatal(err)
	}

	calls := model.Calls()
	req := calls[0].Messages[0].(core.ModelRequest)
	userPart := req.Parts[1].(core.UserPromptPart)
	if !strings.Contains(userPart.Content, "language") {
		t.Error("meta-prompt should include context keys")
	}
}

func TestGeneratePersonality_TrimsWhitespace(t *testing.T) {
	model := core.NewTestModel(core.TextResponse("  \n  Generated prompt.  \n  "))
	gen := GeneratePersonality(model)

	result, err := gen(context.Background(), PersonalityRequest{Task: "some task"})
	if err != nil {
		t.Fatal(err)
	}
	if result != "Generated prompt." {
		t.Errorf("expected trimmed result, got %q", result)
	}
}

func TestCachedPersonalityGenerator_CachesIdenticalRequests(t *testing.T) {
	var callCount atomic.Int32
	inner := func(ctx context.Context, req PersonalityRequest) (string, error) {
		callCount.Add(1)
		return "generated prompt for " + req.Task, nil
	}

	gen := CachedPersonalityGenerator(inner)

	req := PersonalityRequest{Task: "implement feature X"}

	r1, err := gen(context.Background(), req)
	if err != nil {
		t.Fatal(err)
	}
	r2, err := gen(context.Background(), req)
	if err != nil {
		t.Fatal(err)
	}

	if r1 != r2 {
		t.Errorf("cached results should be identical: %q vs %q", r1, r2)
	}
	if callCount.Load() != 1 {
		t.Errorf("expected 1 call to inner generator, got %d", callCount.Load())
	}
}

func TestCachedPersonalityGenerator_DifferentRequests(t *testing.T) {
	var callCount atomic.Int32
	inner := func(ctx context.Context, req PersonalityRequest) (string, error) {
		callCount.Add(1)
		return "prompt for " + req.Task, nil
	}

	gen := CachedPersonalityGenerator(inner)

	_, err := gen(context.Background(), PersonalityRequest{Task: "task A"})
	if err != nil {
		t.Fatal(err)
	}
	_, err = gen(context.Background(), PersonalityRequest{Task: "task B"})
	if err != nil {
		t.Fatal(err)
	}

	if callCount.Load() != 2 {
		t.Errorf("expected 2 calls for different requests, got %d", callCount.Load())
	}
}

func TestCachedPersonalityGenerator_DoesNotCacheErrors(t *testing.T) {
	var callCount atomic.Int32
	shouldFail := true
	inner := func(ctx context.Context, req PersonalityRequest) (string, error) {
		callCount.Add(1)
		if shouldFail {
			shouldFail = false
			return "", context.DeadlineExceeded
		}
		return "success", nil
	}

	gen := CachedPersonalityGenerator(inner)

	_, err := gen(context.Background(), PersonalityRequest{Task: "flaky task"})
	if err == nil {
		t.Fatal("expected error on first call")
	}

	result, err := gen(context.Background(), PersonalityRequest{Task: "flaky task"})
	if err != nil {
		t.Fatal(err)
	}
	if result != "success" {
		t.Errorf("expected 'success', got %q", result)
	}
	if callCount.Load() != 2 {
		t.Errorf("expected 2 calls (error not cached), got %d", callCount.Load())
	}
}

func TestCachedPersonalityGenerator_ConcurrentAccess(t *testing.T) {
	var callCount atomic.Int32
	inner := func(ctx context.Context, req PersonalityRequest) (string, error) {
		callCount.Add(1)
		return "prompt", nil
	}

	gen := CachedPersonalityGenerator(inner)
	req := PersonalityRequest{Task: "concurrent task"}

	var wg sync.WaitGroup
	for range 10 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			gen(context.Background(), req)
		}()
	}
	wg.Wait()

	// With sync.Map, some concurrent calls may slip through before
	// the first Store, but it should be far fewer than 10.
	if callCount.Load() > 5 {
		t.Errorf("expected mostly cached results, got %d calls", callCount.Load())
	}
}

func TestBuildMetaPrompt_AllFields(t *testing.T) {
	prompt := buildMetaPrompt(PersonalityRequest{
		Task:        "Build a REST API",
		Role:        "backend engineer",
		BasePrompt:  "You are a coding assistant.",
		Constraints: []string{"use net/http", "no frameworks"},
		Context:     map[string]string{"language": "Go"},
	})

	if !strings.Contains(prompt, "Build a REST API") {
		t.Error("should contain task")
	}
	if !strings.Contains(prompt, "backend engineer") {
		t.Error("should contain role")
	}
	if !strings.Contains(prompt, "You are a coding assistant.") {
		t.Error("should contain base prompt")
	}
	if !strings.Contains(prompt, "use net/http") {
		t.Error("should contain constraints")
	}
	if !strings.Contains(prompt, "language") {
		t.Error("should contain context keys")
	}
}

func TestBuildMetaPrompt_MinimalFields(t *testing.T) {
	prompt := buildMetaPrompt(PersonalityRequest{Task: "fix bug"})

	if !strings.Contains(prompt, "fix bug") {
		t.Error("should contain task")
	}
	if strings.Contains(prompt, "ROLE:") {
		t.Error("should not contain ROLE section when empty")
	}
	if strings.Contains(prompt, "BASE INSTRUCTIONS") {
		t.Error("should not contain BASE INSTRUCTIONS when empty")
	}
	if strings.Contains(prompt, "CONSTRAINTS") {
		t.Error("should not contain CONSTRAINTS when empty")
	}
}

func TestCacheKey_Deterministic(t *testing.T) {
	req := PersonalityRequest{
		Task: "test",
		Role: "dev",
	}
	k1 := personalityCacheKey(req)
	k2 := personalityCacheKey(req)
	if k1 != k2 {
		t.Errorf("cache keys should be deterministic: %q vs %q", k1, k2)
	}
}

func TestCacheKey_DifferentForDifferentRequests(t *testing.T) {
	k1 := personalityCacheKey(PersonalityRequest{Task: "task A"})
	k2 := personalityCacheKey(PersonalityRequest{Task: "task B"})
	if k1 == k2 {
		t.Error("different requests should produce different cache keys")
	}
}
