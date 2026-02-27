package codetool

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/fugue-labs/gollem/core"
)

func TestInvariantsTool_ExtractAndUpdate(t *testing.T) {
	model := core.NewTestModel(core.TextResponse(`{"items":[{"id":"I1","description":"Write /app/result.txt","kind":"hard"},{"id":"I2","description":"Keep runtime under 30s","kind":"hard"}]}`))
	tool := InvariantsTool(model)

	rc := &core.RunContext{
		Prompt: "Requirements: write /app/result.txt and keep runtime under 30s.",
	}

	extractArgs := `{"command":"extract"}`
	extracted, err := tool.Handler(context.Background(), rc, extractArgs)
	if err != nil {
		t.Fatalf("extract failed: %v", err)
	}
	extractMap, ok := extracted.(map[string]any)
	if !ok {
		t.Fatalf("extract result type = %T, want map[string]any", extracted)
	}
	if extractMap["hard_total"] != 2 {
		t.Fatalf("hard_total = %v, want 2", extractMap["hard_total"])
	}

	updateArgs := `{"command":"update","id":"I1","status":"pass","evidence":"created /app/result.txt and verified with ls"}`
	updated, err := tool.Handler(context.Background(), rc, updateArgs)
	if err != nil {
		t.Fatalf("update failed: %v", err)
	}
	updateMap := updated.(map[string]any)
	if updateMap["hard_pass"] != 1 {
		t.Fatalf("hard_pass = %v, want 1", updateMap["hard_pass"])
	}

	summaryArgs := `{"command":"summary"}`
	summary, err := tool.Handler(context.Background(), rc, summaryArgs)
	if err != nil {
		t.Fatalf("summary failed: %v", err)
	}
	summaryMap := summary.(map[string]any)
	if summaryMap["hard_unresolved"] != 1 {
		t.Fatalf("hard_unresolved = %v, want 1", summaryMap["hard_unresolved"])
	}
}

func TestInvariantsTool_Extract_NoFallback(t *testing.T) {
	model := core.NewTestModel(core.TextResponse("not-json"))
	tool := InvariantsTool(model)
	rc := &core.RunContext{Prompt: "Requirements: write output.txt"}

	_, err := tool.Handler(context.Background(), rc, `{"command":"extract"}`)
	if err == nil {
		t.Fatal("expected extract error on non-JSON model response")
	}
	if !strings.Contains(strings.ToLower(err.Error()), "valid json") {
		t.Fatalf("expected strict JSON parse error, got: %v", err)
	}
}

func TestInvariantsTool_StatefulExportRestore(t *testing.T) {
	model := core.NewTestModel(core.TextResponse(`{"items":[{"id":"I1","description":"Produce /app/out.txt","kind":"hard"}]}`))
	tool := InvariantsTool(model)
	rc := &core.RunContext{Prompt: "Requirements: Produce /app/out.txt"}

	if _, err := tool.Handler(context.Background(), rc, `{"command":"extract"}`); err != nil {
		t.Fatalf("extract failed: %v", err)
	}
	if _, err := tool.Handler(context.Background(), rc, `{"command":"update","id":"I1","status":"pass","evidence":"created file"}`); err != nil {
		t.Fatalf("update failed: %v", err)
	}

	state, err := tool.Stateful.ExportState()
	if err != nil {
		t.Fatalf("export state failed: %v", err)
	}
	stateJSON, err := json.Marshal(state)
	if err != nil {
		t.Fatalf("marshal state failed: %v", err)
	}

	model2 := core.NewTestModel(core.TextResponse(`{"items":[]}`))
	tool2 := InvariantsTool(model2)
	var decoded any
	if err := json.Unmarshal(stateJSON, &decoded); err != nil {
		t.Fatalf("unmarshal state failed: %v", err)
	}
	if err := tool2.Stateful.RestoreState(decoded); err != nil {
		t.Fatalf("restore state failed: %v", err)
	}

	summary, err := tool2.Handler(context.Background(), rc, `{"command":"summary"}`)
	if err != nil {
		t.Fatalf("summary failed: %v", err)
	}
	summaryMap := summary.(map[string]any)
	if summaryMap["hard_pass"] != 1 {
		t.Fatalf("restored hard_pass = %v, want 1", summaryMap["hard_pass"])
	}
}
