package mcp

import (
	"context"
	"encoding/json"
	"fmt"
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
	eventCh           chan string
	ready             chan struct{}
}

func newMockHTTPServer() *mockHTTPServer {
	return &mockHTTPServer{
		toolResults:     make(map[string]*ToolResult),
		resourceResults: make(map[string]*ReadResourceResult),
		promptResults:   make(map[string]*PromptResult),
		eventCh:         make(chan string, 100),
		ready:           make(chan struct{}),
	}
}

func (m *mockHTTPServer) handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/mcp", m.handle)
	return mux
}

func (m *mockHTTPServer) handle(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		m.handleStream(w, r)
	case http.MethodPost:
		m.handlePost(w, r)
	case http.MethodDelete:
		w.WriteHeader(http.StatusNoContent)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (m *mockHTTPServer) handleStream(w http.ResponseWriter, r *http.Request) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming not supported", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("Mcp-Session-Id", "session-123")

	select {
	case <-m.ready:
	default:
		close(m.ready)
	}

	for {
		select {
		case payload := <-m.eventCh:
			fmt.Fprintf(w, "event: message\ndata: %s\n\n", payload)
			flusher.Flush()
		case <-r.Context().Done():
			return
		}
	}
}

func (m *mockHTTPServer) handlePost(w http.ResponseWriter, r *http.Request) {
	var req jsonRPCRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid JSON", http.StatusBadRequest)
		return
	}

	w.Header().Set("Mcp-Session-Id", "session-123")

	switch req.Method {
	case "notifications/initialized":
		w.WriteHeader(http.StatusAccepted)
		return
	case "initialize":
		m.writeJSONResponse(w, req.ID, map[string]any{
			"protocolVersion": ProtocolVersion,
			"capabilities": map[string]any{
				"tools":     map[string]any{"listChanged": true},
				"resources": map[string]any{"listChanged": true},
				"prompts":   map[string]any{"listChanged": true},
			},
			"serverInfo": map[string]any{
				"name":    "mock-http-server",
				"version": "1.0.0",
			},
		})
		return
	case "tools/list":
		m.mu.Lock()
		tools := m.tools
		m.mu.Unlock()
		m.writeJSONResponse(w, req.ID, map[string]any{"tools": tools})
		return
	case "tools/call":
		params, _ := json.Marshal(req.Params)
		var callParams struct {
			Name string `json:"name"`
		}
		json.Unmarshal(params, &callParams)

		m.mu.Lock()
		result, ok := m.toolResults[callParams.Name]
		m.mu.Unlock()
		if !ok {
			m.writeJSONError(w, req.ID, &jsonRPCError{Code: -32601, Message: "tool not found"})
			return
		}
		if callParams.Name == "delayed_tool" {
			payload, _ := json.Marshal(jsonRPCMessage{
				JSONRPC: "2.0",
				ID:      rawJSONID(req.ID),
				Result:  mustRawJSON(tMarshal(result)),
			})
			m.eventCh <- string(payload)
			w.WriteHeader(http.StatusAccepted)
			return
		}
		m.writeJSONResponse(w, req.ID, result)
		return
	case "resources/list":
		m.mu.Lock()
		resources := m.resources
		m.mu.Unlock()
		m.writeJSONResponse(w, req.ID, map[string]any{"resources": resources})
		return
	case "resources/read":
		params, _ := json.Marshal(req.Params)
		var readParams struct {
			URI string `json:"uri"`
		}
		json.Unmarshal(params, &readParams)
		m.mu.Lock()
		result, ok := m.resourceResults[readParams.URI]
		m.mu.Unlock()
		if !ok {
			m.writeJSONError(w, req.ID, &jsonRPCError{Code: -32602, Message: "resource not found"})
			return
		}
		m.writeJSONResponse(w, req.ID, result)
		return
	case "resources/templates/list":
		m.mu.Lock()
		templates := m.resourceTemplates
		m.mu.Unlock()
		m.writeJSONResponse(w, req.ID, map[string]any{"resourceTemplates": templates})
		return
	case "prompts/list":
		m.mu.Lock()
		prompts := m.prompts
		m.mu.Unlock()
		m.writeJSONResponse(w, req.ID, map[string]any{"prompts": prompts})
		return
	case "prompts/get":
		params, _ := json.Marshal(req.Params)
		var getParams struct {
			Name string `json:"name"`
		}
		json.Unmarshal(params, &getParams)
		m.mu.Lock()
		result, ok := m.promptResults[getParams.Name]
		m.mu.Unlock()
		if !ok {
			m.writeJSONError(w, req.ID, &jsonRPCError{Code: -32602, Message: "prompt not found"})
			return
		}
		m.writeJSONResponse(w, req.ID, result)
		return
	default:
		m.writeJSONError(w, req.ID, &jsonRPCError{Code: -32601, Message: "method not found"})
	}
}

func (m *mockHTTPServer) writeJSONResponse(w http.ResponseWriter, id int64, result any) {
	w.Header().Set("Content-Type", "application/json")
	payload, _ := json.Marshal(jsonRPCMessage{
		JSONRPC: "2.0",
		ID:      rawJSONID(id),
		Result:  mustRawJSON(tMarshal(result)),
	})
	_, _ = w.Write(payload)
}

func (m *mockHTTPServer) writeJSONError(w http.ResponseWriter, id int64, rpcErr *jsonRPCError) {
	w.Header().Set("Content-Type", "application/json")
	payload, _ := json.Marshal(jsonRPCMessage{
		JSONRPC: "2.0",
		ID:      rawJSONID(id),
		Error:   rpcErr,
	})
	_, _ = w.Write(payload)
}

func tMarshal(v any) []byte {
	data, _ := json.Marshal(v)
	return data
}

func mustRawJSON(data []byte) json.RawMessage {
	return json.RawMessage(data)
}

func TestHTTPClientResourcesPromptsAndAsyncToolCall(t *testing.T) {
	mock := newMockHTTPServer()
	mock.tools = []Tool{
		{
			Name:        "delayed_tool",
			Description: "Resolves asynchronously",
			InputSchema: json.RawMessage(`{"type":"object"}`),
		},
	}
	mock.toolResults["delayed_tool"] = &ToolResult{
		Content: []Content{{Type: "text", Text: "async done"}},
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

	select {
	case <-mock.ready:
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for HTTP notification stream")
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

	toolResult, err := client.CallTool(ctx, "delayed_tool", nil)
	if err != nil {
		t.Fatalf("CallTool failed: %v", err)
	}
	if toolResult.TextContent() != "async done" {
		t.Fatalf("unexpected tool result: %q", toolResult.TextContent())
	}

	if client.ServerInfo() == nil || client.ServerInfo().Name != "mock-http-server" {
		t.Fatalf("unexpected server info: %+v", client.ServerInfo())
	}
	if client.Capabilities().Resources == nil || client.Capabilities().Prompts == nil {
		t.Fatalf("expected prompt and resource capabilities, got %+v", client.Capabilities())
	}
}

func TestHTTPClientNotificationHandler(t *testing.T) {
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

	select {
	case <-mock.ready:
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for HTTP notification stream")
	}

	received := make(chan string, 1)
	unregister := client.OnNotification("notifications/prompts/list_changed", func(note Notification) {
		received <- note.Method
	})
	defer unregister()

	payload, _ := json.Marshal(jsonRPCMessage{
		JSONRPC: "2.0",
		Method:  "notifications/prompts/list_changed",
		Params:  json.RawMessage(`{"reason":"refresh"}`),
	})
	mock.eventCh <- string(payload)

	select {
	case method := <-received:
		if method != "notifications/prompts/list_changed" {
			t.Fatalf("unexpected method: %s", method)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for notification")
	}
}

func TestHTTPClientCallFailsWhenPOSTStreamClosesWithoutResponse(t *testing.T) {
	streamReady := make(chan struct{})
	holdStream := make(chan struct{})

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			flusher, ok := w.(http.Flusher)
			if !ok {
				http.Error(w, "streaming not supported", http.StatusInternalServerError)
				return
			}

			w.Header().Set("Content-Type", "text/event-stream")
			w.Header().Set("Cache-Control", "no-cache")
			w.Header().Set("Connection", "keep-alive")
			w.Header().Set("Mcp-Session-Id", "session-123")
			flusher.Flush()

			select {
			case <-streamReady:
			default:
				close(streamReady)
			}

			select {
			case <-holdStream:
			case <-r.Context().Done():
			}
		case http.MethodPost:
			var req jsonRPCRequest
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				http.Error(w, "invalid JSON", http.StatusBadRequest)
				return
			}

			w.Header().Set("Mcp-Session-Id", "session-123")

			switch req.Method {
			case "initialize":
				w.Header().Set("Content-Type", "application/json")
				payload, _ := json.Marshal(jsonRPCMessage{
					JSONRPC: "2.0",
					ID:      rawJSONID(req.ID),
					Result: mustRawJSON(tMarshal(map[string]any{
						"protocolVersion": ProtocolVersion,
						"capabilities": map[string]any{
							"tools": map[string]any{},
						},
						"serverInfo": map[string]any{
							"name":    "broken-stream-server",
							"version": "1.0.0",
						},
					})),
				})
				_, _ = w.Write(payload)
			case "notifications/initialized":
				w.WriteHeader(http.StatusAccepted)
			case "tools/call":
				w.Header().Set("Content-Type", "text/event-stream")
				if flusher, ok := w.(http.Flusher); ok {
					flusher.Flush()
				}
			default:
				http.Error(w, "method not found", http.StatusNotFound)
			}
		case http.MethodDelete:
			w.WriteHeader(http.StatusNoContent)
		default:
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		}
	}))
	defer func() {
		close(holdStream)
		server.Close()
	}()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	client, err := NewHTTPClient(ctx, server.URL)
	if err != nil {
		t.Fatalf("failed to create HTTP client: %v", err)
	}
	defer client.Close()

	select {
	case <-streamReady:
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for HTTP notification stream")
	}

	callCtx, cancelCall := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancelCall()

	_, err = client.CallTool(callCtx, "broken_stream_tool", nil)
	if err == nil {
		t.Fatal("expected CallTool to fail when POST SSE stream closes without a response")
	}
	if !strings.Contains(err.Error(), "connection closed while waiting for response") {
		t.Fatalf("unexpected error: %v", err)
	}
}
