package mcp

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"
)

type mockHTTPServer struct {
	mu                sync.Mutex
	tools             []Tool
	toolResults       map[string]*ToolResult
	resources         []Resource
	resourceTemplates []ResourceTemplate
	resourceResults   map[string]*ReadResourceResult
	prompts           []Prompt
	promptResults     map[string]*PromptResult
}

func newMockHTTPServer() *mockHTTPServer {
	return &mockHTTPServer{
		toolResults:     make(map[string]*ToolResult),
		resourceResults: make(map[string]*ReadResourceResult),
		promptResults:   make(map[string]*PromptResult),
	}
}

func (m *mockHTTPServer) handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/mcp", m.handle)
	return mux
}

func (m *mockHTTPServer) handle(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodPost:
		m.handlePost(w, r)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (m *mockHTTPServer) handlePost(w http.ResponseWriter, r *http.Request) {
	var req jsonRPCRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid JSON", http.StatusBadRequest)
		return
	}

	var result any
	var rpcErr *jsonRPCError

	switch req.Method {
	case "server/discover":
		result = map[string]any{
			"protocolVersions": []string{ProtocolVersion},
			"capabilities": map[string]any{
				"tools":     map[string]any{"listChanged": true},
				"resources": map[string]any{"listChanged": true},
				"prompts":   map[string]any{"listChanged": true},
			},
			"serverInfo": map[string]any{
				"name":    "mock-http-server",
				"version": "1.0.0",
			},
		}
	case "tools/list":
		m.mu.Lock()
		tools := m.tools
		m.mu.Unlock()
		result = map[string]any{"tools": tools}
	case "tools/call":
		params, _ := json.Marshal(req.Params)
		var callParams struct {
			Name string `json:"name"`
		}
		json.Unmarshal(params, &callParams)

		m.mu.Lock()
		res, ok := m.toolResults[callParams.Name]
		m.mu.Unlock()
		if !ok {
			rpcErr = &jsonRPCError{Code: -32601, Message: "tool not found"}
		} else {
			result = res
		}
	case "resources/list":
		m.mu.Lock()
		resources := m.resources
		m.mu.Unlock()
		result = map[string]any{"resources": resources}
	case "resources/read":
		params, _ := json.Marshal(req.Params)
		var readParams struct {
			URI string `json:"uri"`
		}
		json.Unmarshal(params, &readParams)
		m.mu.Lock()
		res, ok := m.resourceResults[readParams.URI]
		m.mu.Unlock()
		if !ok {
			rpcErr = &jsonRPCError{Code: -32602, Message: "resource not found"}
		} else {
			result = res
		}
	case "resources/templates/list":
		m.mu.Lock()
		templates := m.resourceTemplates
		m.mu.Unlock()
		result = map[string]any{"resourceTemplates": templates}
	case "prompts/list":
		m.mu.Lock()
		prompts := m.prompts
		m.mu.Unlock()
		result = map[string]any{"prompts": prompts}
	case "prompts/get":
		params, _ := json.Marshal(req.Params)
		var getParams struct {
			Name string `json:"name"`
		}
		json.Unmarshal(params, &getParams)
		m.mu.Lock()
		res, ok := m.promptResults[getParams.Name]
		m.mu.Unlock()
		if !ok {
			rpcErr = &jsonRPCError{Code: -32602, Message: "prompt not found"}
		} else {
			result = res
		}
	default:
		rpcErr = &jsonRPCError{Code: -32601, Message: "method not found"}
	}

	resp := jsonRPCMessage{
		JSONRPC: "2.0",
		ID:      rawJSONID(req.ID),
	}
	if rpcErr != nil {
		resp.Error = rpcErr
	} else {
		data, _ := json.Marshal(result)
		resp.Result = data
	}

	w.Header().Set("Content-Type", "application/json")
	respData, _ := json.Marshal(resp)
	_, _ = w.Write(respData)
}

func tMarshal(v any) []byte {
	data, _ := json.Marshal(v)
	return data
}

func TestHTTPClientResourcesPromptsAndToolCall(t *testing.T) {
	mock := newMockHTTPServer()
	mock.tools = []Tool{
		{
			Name:        "echo_tool",
			Description: "Echoes back",
			InputSchema: json.RawMessage(`{"type":"object"}`),
		},
	}
	mock.toolResults["echo_tool"] = &ToolResult{
		Content: []Content{{Type: "text", Text: "echo done"}},
	}
	mock.resources = []Resource{{
		URI:         "file:///workspace/README.md",
		Name:        "README",
		Description: "Project readme",
		MIMEType:    "text/markdown",
	}}
	mock.resourceTemplates = []ResourceTemplate{{
		URITemplate: "file:///workspace/{path}",
		Name:        "workspace_file",
	}}
	mock.resourceResults["file:///workspace/README.md"] = &ReadResourceResult{
		Contents: []ResourceContents{{URI: "file:///workspace/README.md", Text: "# Gollem\n"}},
	}
	mock.prompts = []Prompt{{Name: "summarize_repo", Description: "Summarize the repo"}}
	mock.promptResults["summarize_repo"] = &PromptResult{
		Messages: []PromptMessage{{
			Role:    "user",
			Content: Content{Type: "text", Text: "Summarize the repository."},
		}},
	}

	server := httptest.NewServer(mock.handler())
	defer server.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	client, err := NewHTTPClient(ctx, server.URL+"/mcp")
	if err != nil {
		t.Fatalf("failed to create HTTP client: %v", err)
	}
	defer client.Close()

	if _, err := client.Discover(ctx); err != nil {
		t.Fatalf("Discover failed: %v", err)
	}

	resources, err := client.ListResources(ctx)
	if err != nil {
		t.Fatalf("ListResources failed: %v", err)
	}
	if len(resources) != 1 || resources[0].URI != "file:///workspace/README.md" {
		t.Fatalf("unexpected resources: %+v", resources)
	}

	readResult, err := client.ReadResource(ctx, "file:///workspace/README.md")
	if err != nil {
		t.Fatalf("ReadResource failed: %v", err)
	}
	if readResult.TextContent() != "# Gollem\n" {
		t.Fatalf("unexpected resource content: %q", readResult.TextContent())
	}

	templates, err := client.ListResourceTemplates(ctx)
	if err != nil {
		t.Fatalf("ListResourceTemplates failed: %v", err)
	}
	if len(templates) != 1 || templates[0].Name != "workspace_file" {
		t.Fatalf("unexpected templates: %+v", templates)
	}

	prompts, err := client.ListPrompts(ctx)
	if err != nil {
		t.Fatalf("ListPrompts failed: %v", err)
	}
	if len(prompts) != 1 || prompts[0].Name != "summarize_repo" {
		t.Fatalf("unexpected prompts: %+v", prompts)
	}

	promptResult, err := client.GetPrompt(ctx, "summarize_repo", nil)
	if err != nil {
		t.Fatalf("GetPrompt failed: %v", err)
	}
	if promptResult.TextContent() != "user: Summarize the repository." {
		t.Fatalf("unexpected prompt content: %q", promptResult.TextContent())
	}

	toolResult, err := client.CallTool(ctx, "echo_tool", nil)
	if err != nil {
		t.Fatalf("CallTool failed: %v", err)
	}
	if toolResult.TextContent() != "echo done" {
		t.Fatalf("unexpected tool result: %q", toolResult.TextContent())
	}

	if client.ServerInfo() == nil || client.ServerInfo().Name != "mock-http-server" {
		t.Fatalf("unexpected server info: %+v", client.ServerInfo())
	}
	if client.Capabilities().Resources == nil || client.Capabilities().Prompts == nil {
		t.Fatalf("expected prompt and resource capabilities, got %+v", client.Capabilities())
	}
}

func TestHTTPClientCallToolError(t *testing.T) {
	mock := newMockHTTPServer()
	server := httptest.NewServer(mock.handler())
	defer server.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	client, err := NewHTTPClient(ctx, server.URL+"/mcp")
	if err != nil {
		t.Fatalf("failed to create HTTP client: %v", err)
	}
	defer client.Close()

	_, err = client.CallTool(ctx, "nonexistent", nil)
	if err == nil {
		t.Fatal("expected error for nonexistent tool")
	}
	if !strings.Contains(err.Error(), "tool not found") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestHTTPClientSendsStatelessMetaAndHeaders(t *testing.T) {
	var seenHeaders http.Header
	var seenParams map[string]any

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seenHeaders = r.Header.Clone()
		var req jsonRPCRequest
		_ = json.NewDecoder(r.Body).Decode(&req)
		_ = json.Unmarshal(req.Params, &seenParams)

		w.Header().Set("Content-Type", "application/json")
		resp, _ := json.Marshal(jsonRPCMessage{
			JSONRPC: "2.0",
			ID:      rawJSONID(req.ID),
			Result:  mustRawJSON(tMarshal(map[string]any{"tools": []Tool{}})),
		})
		_, _ = w.Write(resp)
	}))
	defer srv.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	client, err := NewHTTPClient(ctx, srv.URL)
	if err != nil {
		t.Fatalf("failed to create HTTP client: %v", err)
	}
	defer client.Close()

	if _, err := client.ListTools(ctx); err != nil {
		t.Fatalf("ListTools failed: %v", err)
	}

	if got := seenHeaders.Get("Mcp-Method"); got != "tools/list" {
		t.Fatalf("Mcp-Method header = %q, want tools/list", got)
	}
	if got := seenHeaders.Get("MCP-Protocol-Version"); got != ProtocolVersion {
		t.Fatalf("MCP-Protocol-Version header = %q, want %s", got, ProtocolVersion)
	}

	meta, ok := seenParams["_meta"].(map[string]any)
	if !ok {
		t.Fatalf("request params missing _meta: %+v", seenParams)
	}
	if pv, _ := meta[MetaProtocolVersion].(string); pv != ProtocolVersion {
		t.Fatalf("_meta.protocolVersion = %v, want %s", meta[MetaProtocolVersion], ProtocolVersion)
	}
	if _, ok := meta[MetaClientCapabilities].(map[string]any); !ok {
		t.Fatalf("_meta.clientCapabilities missing or wrong type: %+v", meta[MetaClientCapabilities])
	}
}

func TestHTTPClientCallToolSendsMcpNameHeader(t *testing.T) {
	var seenHeaders http.Header
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seenHeaders = r.Header.Clone()
		var req jsonRPCRequest
		_ = json.NewDecoder(r.Body).Decode(&req)

		w.Header().Set("Content-Type", "application/json")
		resp, _ := json.Marshal(jsonRPCMessage{
			JSONRPC: "2.0",
			ID:      rawJSONID(req.ID),
			Result:  mustRawJSON(tMarshal(&ToolResult{Content: []Content{{Type: "text", Text: "ok"}}})),
		})
		_, _ = w.Write(resp)
	}))
	defer srv.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	client, err := NewHTTPClient(ctx, srv.URL)
	if err != nil {
		t.Fatalf("failed to create HTTP client: %v", err)
	}
	defer client.Close()

	if _, err := client.CallTool(ctx, "greet", map[string]any{"name": "world"}); err != nil {
		t.Fatalf("CallTool failed: %v", err)
	}

	if got := seenHeaders.Get("Mcp-Method"); got != "tools/call" {
		t.Fatalf("Mcp-Method header = %q, want tools/call", got)
	}
	if got := seenHeaders.Get("Mcp-Name"); got != "greet" {
		t.Fatalf("Mcp-Name header = %q, want greet", got)
	}
}
