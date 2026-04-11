package openai

import (
	"encoding/json"
	"testing"

	"github.com/fugue-labs/gollem/core"
)

func TestResponsesSupportsToolSearch(t *testing.T) {
	cases := []struct {
		model string
		want  bool
	}{
		{"gpt-5.4", true},
		{"gpt-5.4-turbo", true},
		{"gpt-5.5", true},
		{"gpt-5.10", true},
		{"gpt-5.3", false},
		{"gpt-5", false},
		{"gpt-5.4-codex", false},
		{"gpt-5.2-codex", false},
		{"gpt-4o", false},
		{"chatgpt-5.4-preview", false},
		{"", false},
	}
	for _, tc := range cases {
		got := responsesSupportsToolSearch(tc.model)
		if got != tc.want {
			t.Errorf("responsesSupportsToolSearch(%q) = %v, want %v", tc.model, got, tc.want)
		}
	}
}

func TestBuildResponsesRequestEmitsDeferLoading(t *testing.T) {
	params := &core.ModelRequestParameters{
		FunctionTools: []core.ToolDefinition{
			{
				Name:             "search",
				Description:      "Search the web",
				ParametersSchema: core.Schema{"type": "object"},
				DeferLoading:     true,
			},
		},
	}

	req, err := buildResponsesRequest(nil, nil, params, "gpt-5.4", 4096, false)
	if err != nil {
		t.Fatalf("buildResponsesRequest: %v", err)
	}

	// 2 items: tool_search built-in + user tool.
	if len(req.Tools) != 2 {
		t.Fatalf("expected 2 tools, got %d", len(req.Tools))
	}

	// First: built-in tool_search.
	builtin, ok := req.Tools[0].(responsesToolDef)
	if !ok {
		t.Fatalf("tools[0] is %T, want responsesToolDef", req.Tools[0])
	}
	if builtin.Type != "tool_search" {
		t.Errorf("builtin.Type = %q, want 'tool_search'", builtin.Type)
	}

	// Second: user tool with defer_loading.
	userTool, ok := req.Tools[1].(responsesToolDef)
	if !ok {
		t.Fatalf("tools[1] is %T, want responsesToolDef", req.Tools[1])
	}
	if !userTool.DeferLoading {
		t.Error("user tool should have DeferLoading=true")
	}
	if userTool.Name != "search" {
		t.Errorf("user tool name = %q, want 'search'", userTool.Name)
	}
}

func TestBuildResponsesRequestNamespaceGrouping(t *testing.T) {
	params := &core.ModelRequestParameters{
		FunctionTools: []core.ToolDefinition{
			{Name: "list_orders", Description: "List orders", ParametersSchema: core.Schema{"type": "object"}, DeferLoading: true, Namespace: "crm"},
			{Name: "get_customer", Description: "Get customer", ParametersSchema: core.Schema{"type": "object"}, DeferLoading: true, Namespace: "crm"},
			{Name: "standalone", Description: "Standalone tool", ParametersSchema: core.Schema{"type": "object"}, DeferLoading: true},
		},
	}

	req, err := buildResponsesRequest(nil, nil, params, "gpt-5.4", 4096, false)
	if err != nil {
		t.Fatalf("buildResponsesRequest: %v", err)
	}

	// Expected: tool_search + namespace("crm") + standalone.
	if len(req.Tools) != 3 {
		t.Fatalf("expected 3 tools (built-in + namespace + standalone), got %d", len(req.Tools))
	}

	// [0]: tool_search built-in.
	builtin, ok := req.Tools[0].(responsesToolDef)
	if !ok || builtin.Type != "tool_search" {
		t.Errorf("tools[0] should be tool_search, got %T %+v", req.Tools[0], req.Tools[0])
	}

	// [1]: namespace wrapping the 2 crm tools.
	ns, ok := req.Tools[1].(responsesNamespace)
	if !ok {
		t.Fatalf("tools[1] should be responsesNamespace, got %T", req.Tools[1])
	}
	if ns.Name != "crm" {
		t.Errorf("namespace name = %q, want 'crm'", ns.Name)
	}
	if len(ns.Tools) != 2 {
		t.Fatalf("namespace should have 2 tools, got %d", len(ns.Tools))
	}
	if ns.Tools[0].Name != "list_orders" || ns.Tools[1].Name != "get_customer" {
		t.Errorf("namespace tools = [%q, %q], want [list_orders, get_customer]", ns.Tools[0].Name, ns.Tools[1].Name)
	}

	// [2]: standalone tool.
	sa, ok := req.Tools[2].(responsesToolDef)
	if !ok {
		t.Fatalf("tools[2] should be responsesToolDef, got %T", req.Tools[2])
	}
	if sa.Name != "standalone" {
		t.Errorf("standalone name = %q, want 'standalone'", sa.Name)
	}

	// Verify wire JSON contains namespace.
	data, _ := json.Marshal(req.Tools)
	s := string(data)
	if !jsonContains(s, `"type":"namespace"`) {
		t.Errorf("wire JSON should contain namespace type, got %s", s)
	}
}

func TestBuildResponsesRequestSilentDegradeOnGPT53(t *testing.T) {
	params := &core.ModelRequestParameters{
		FunctionTools: []core.ToolDefinition{
			{Name: "search", ParametersSchema: core.Schema{"type": "object"}, DeferLoading: true},
		},
	}

	req, err := buildResponsesRequest(nil, nil, params, "gpt-5.3", 4096, false)
	if err != nil {
		t.Fatalf("buildResponsesRequest: %v", err)
	}

	// No built-in injected.
	if len(req.Tools) != 1 {
		t.Fatalf("expected 1 tool on unsupported model, got %d", len(req.Tools))
	}
	tool := req.Tools[0].(responsesToolDef)
	if tool.DeferLoading {
		t.Error("DeferLoading should not be emitted on unsupported model")
	}
}

func TestBuildResponsesRequestBuiltinSkippedWhenDisabled(t *testing.T) {
	params := &core.ModelRequestParameters{
		FunctionTools: []core.ToolDefinition{
			{Name: "search", ParametersSchema: core.Schema{"type": "object"}, DeferLoading: true},
		},
	}

	req, err := buildResponsesRequest(nil, nil, params, "gpt-5.4", 4096, true) // disableToolSearch=true
	if err != nil {
		t.Fatalf("buildResponsesRequest: %v", err)
	}

	// No built-in, but defer_loading still emitted.
	if len(req.Tools) != 1 {
		t.Fatalf("expected 1 tool, got %d", len(req.Tools))
	}
	tool := req.Tools[0].(responsesToolDef)
	if !tool.DeferLoading {
		t.Error("defer_loading should still be emitted when built-in is disabled")
	}
}

func TestParseResponsesFunctionCallWithNamespace(t *testing.T) {
	resp := &responsesAPIResponse{
		Output: []responsesOutputItem{
			{
				Type:      "function_call",
				Name:      "list_orders",
				Namespace: "crm",
				CallID:    "call_123",
				Arguments: `{"customer_id":"C1"}`,
			},
		},
	}

	result := parseResponsesResponse(resp, "gpt-5.4")
	if len(result.Parts) != 1 {
		t.Fatalf("expected 1 part, got %d", len(result.Parts))
	}
	tc, ok := result.Parts[0].(core.ToolCallPart)
	if !ok {
		t.Fatalf("expected ToolCallPart, got %T", result.Parts[0])
	}
	if tc.ToolName != "list_orders" {
		t.Errorf("ToolName = %q, want 'list_orders'", tc.ToolName)
	}
	if tc.Metadata == nil || tc.Metadata["namespace"] != "crm" {
		t.Errorf("Metadata[namespace] = %q, want 'crm'", tc.Metadata["namespace"])
	}
}

func jsonContains(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
