package codetool

import (
	"context"
	"strings"
	"testing"

	"github.com/fugue-labs/gollem/core"
)

func TestVerificationCheckpoint_RequiresInvariantsToolWhenEnabled(t *testing.T) {
	t.Setenv("GOLLEM_REQUIRE_INVARIANT_CHECKLIST", "1")
	middleware, validator := VerificationCheckpoint("")
	mw := requireRequestMiddleware(t, middleware)

	msgs := []core.ModelMessage{
		core.ModelRequest{Parts: []core.ModelRequestPart{core.UserPromptPart{Content: "task"}}},
		core.ModelResponse{Parts: []core.ModelResponsePart{
			core.ToolCallPart{ToolName: "bash", ArgsJSON: `{"command":"pytest -q"}`, ToolCallID: "v1"},
		}},
		core.ModelRequest{Parts: []core.ModelRequestPart{
			core.ToolReturnPart{ToolName: "bash", ToolCallID: "v1", Content: "=== 10 passed in 0.10s ==="},
		}},
	}

	next := func(_ context.Context, _ []core.ModelMessage, _ *core.ModelSettings, _ *core.ModelRequestParameters) (*core.ModelResponse, error) {
		return &core.ModelResponse{}, nil
	}
	params := &core.ModelRequestParameters{
		FunctionTools: []core.ToolDefinition{{Name: "bash"}, {Name: "invariants"}},
	}
	if _, err := mw(context.Background(), msgs, &core.ModelSettings{}, params, next); err != nil {
		t.Fatalf("middleware failed: %v", err)
	}

	_, err := validator(context.Background(), &core.RunContext{}, "done")
	if err == nil {
		t.Fatal("expected invariant checklist gate rejection")
	}
	retryErr, ok := err.(*core.ModelRetryError)
	if !ok {
		t.Fatalf("expected ModelRetryError, got %T", err)
	}
	if !strings.Contains(retryErr.Message, "must run the `invariants` tool") {
		t.Fatalf("unexpected retry message: %s", retryErr.Message)
	}
}

func TestVerificationCheckpoint_RejectsUnresolvedHardInvariants(t *testing.T) {
	t.Setenv("GOLLEM_REQUIRE_INVARIANT_CHECKLIST", "1")
	middleware, validator := VerificationCheckpoint("")
	mw := requireRequestMiddleware(t, middleware)

	msgs := []core.ModelMessage{
		core.ModelRequest{Parts: []core.ModelRequestPart{core.UserPromptPart{Content: "task"}}},
		core.ModelResponse{Parts: []core.ModelResponsePart{
			core.ToolCallPart{ToolName: "bash", ArgsJSON: `{"command":"pytest -q"}`, ToolCallID: "v1"},
		}},
		core.ModelRequest{Parts: []core.ModelRequestPart{
			core.ToolReturnPart{ToolName: "bash", ToolCallID: "v1", Content: "=== 10 passed in 0.10s ==="},
		}},
		core.ModelResponse{Parts: []core.ModelResponsePart{
			core.ToolCallPart{ToolName: "invariants", ArgsJSON: `{"command":"summary"}`, ToolCallID: "i1"},
		}},
		core.ModelRequest{Parts: []core.ModelRequestPart{
			core.ToolReturnPart{ToolName: "invariants", ToolCallID: "i1", Content: `{"status":"ok","hard_total":3,"hard_pass":2,"hard_fail":0,"hard_unresolved":1}`},
		}},
	}

	next := func(_ context.Context, _ []core.ModelMessage, _ *core.ModelSettings, _ *core.ModelRequestParameters) (*core.ModelResponse, error) {
		return &core.ModelResponse{}, nil
	}
	params := &core.ModelRequestParameters{
		FunctionTools: []core.ToolDefinition{{Name: "bash"}, {Name: "invariants"}},
	}
	if _, err := mw(context.Background(), msgs, &core.ModelSettings{}, params, next); err != nil {
		t.Fatalf("middleware failed: %v", err)
	}

	_, err := validator(context.Background(), &core.RunContext{}, "done")
	if err == nil {
		t.Fatal("expected unresolved hard invariants rejection")
	}
	retryErr := err.(*core.ModelRetryError)
	if !strings.Contains(retryErr.Message, "hard_unresolved=1") {
		t.Fatalf("unexpected retry message: %s", retryErr.Message)
	}
}

func TestVerificationCheckpoint_AcceptsWhenHardInvariantsPass(t *testing.T) {
	t.Setenv("GOLLEM_REQUIRE_INVARIANT_CHECKLIST", "1")
	middleware, validator := VerificationCheckpoint("")
	mw := requireRequestMiddleware(t, middleware)

	msgs := []core.ModelMessage{
		core.ModelRequest{Parts: []core.ModelRequestPart{core.UserPromptPart{Content: "task"}}},
		core.ModelResponse{Parts: []core.ModelResponsePart{
			core.ToolCallPart{ToolName: "bash", ArgsJSON: `{"command":"pytest -q"}`, ToolCallID: "v1"},
		}},
		core.ModelRequest{Parts: []core.ModelRequestPart{
			core.ToolReturnPart{ToolName: "bash", ToolCallID: "v1", Content: "=== 10 passed in 0.10s ==="},
		}},
		core.ModelResponse{Parts: []core.ModelResponsePart{
			core.ToolCallPart{ToolName: "invariants", ArgsJSON: `{"command":"summary"}`, ToolCallID: "i1"},
		}},
		core.ModelRequest{Parts: []core.ModelRequestPart{
			core.ToolReturnPart{ToolName: "invariants", ToolCallID: "i1", Content: `{"status":"ok","hard_total":3,"hard_pass":3,"hard_fail":0,"hard_unresolved":0}`},
		}},
	}

	next := func(_ context.Context, _ []core.ModelMessage, _ *core.ModelSettings, _ *core.ModelRequestParameters) (*core.ModelResponse, error) {
		return &core.ModelResponse{}, nil
	}
	params := &core.ModelRequestParameters{
		FunctionTools: []core.ToolDefinition{{Name: "bash"}, {Name: "invariants"}},
	}
	if _, err := mw(context.Background(), msgs, &core.ModelSettings{}, params, next); err != nil {
		t.Fatalf("middleware failed: %v", err)
	}

	if _, err := validator(context.Background(), &core.RunContext{}, "done"); err != nil {
		t.Fatalf("expected acceptance with passing hard invariants, got: %v", err)
	}
}
