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

// sseEvent is a message to be sent over the SSE connection.
type sseEvent struct {
	eventType string
	data      string
}

// mockSSEServer simulates an MCP server using the SSE transport.
// It serves SSE events on /sse and accepts JSON-RPC requests on /messages.
type mockSSEServer struct {
	mu                sync.Mutex
	tools             []Tool
	toolResults       map[string]*ToolResult
	resources         []Resource
	resourceTemplates []ResourceTemplate
	resourceResults   map[string]*ReadResourceResult
	prompts           []Prompt
	promptResults     map[string]*PromptResult

	// eventCh delivers events to the SSE handler for writing.
	eventCh chan sseEvent
	// ready signals that the SSE handler is ready to receive events.
	ready chan struct{}
}

func newMockSSEServer() *mockSSEServer {
	return &mockSSEServer{
		toolResults:     make(map[string]*ToolResult),
		resourceResults: make(map[string]*ReadResourceResult),
		promptResults:   make(map[string]*PromptResult),
		eventCh:         make(chan sseEvent, 100),
		ready:           make(chan struct{}),
	}
}

func (m *mockSSEServer) handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/sse", m.handleSSE)
	mux.HandleFunc("/messages", m.handleMessages)
	return mux
}

func (m *mockSSEServer) handleSSE(w http.ResponseWriter, r *http.Request) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming not supported", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")

	// Send the endpoint event.
	fmt.Fprintf(w, "event: endpoint\ndata: /messages\n\n")
	flusher.Flush()

	// Signal that we're ready.
	select {
	case <-m.ready:
	default:
		close(m.ready)
	}

	// Read events from the channel and write them to the SSE stream.
	// This ensures all writes to w happen on this goroutine (the HTTP handler),
	// avoiding race conditions with the HTTP server closing the connection.
	for {
		select {
		case ev := <-m.eventCh:
			fmt.Fprintf(w, "event: %s\ndata: %s\n\n", ev.eventType, ev.data)
			flusher.Flush()
		case <-r.Context().Done():
			return
		}
	}
}

func (m *mockSSEServer) handleMessages(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req jsonRPCRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid JSON", http.StatusBadRequest)
		return
	}

	// Build the response.
	var result any
	var rpcErr *jsonRPCError

	switch req.Method {
	case "initialize":
		result = map[string]any{
			"protocolVersion": ProtocolVersion,
			"capabilities": map[string]any{
				"tools":     map[string]any{"listChanged": true},
				"resources": map[string]any{"listChanged": true},
				"prompts":   map[string]any{"listChanged": true},
			},
			"serverInfo": map[string]any{
				"name":    "mock-server",
				"version": "1.0.0",
			},
		}
	case "notifications/initialized":
		// Notification, no response needed.
		w.WriteHeader(http.StatusAccepted)
		return
	case "tools/list":
		m.mu.Lock()
		tools := m.tools
		m.mu.Unlock()
		result = map[string]any{"tools": tools}
	case "tools/call":
		params, _ := json.Marshal(req.Params)
		var callParams struct {
			Name      string         `json:"name"`
			Arguments map[string]any `json:"arguments"`
		}
		json.Unmarshal(params, &callParams)

		m.mu.Lock()
		tr, ok := m.toolResults[callParams.Name]
		m.mu.Unlock()

		if ok {
			result = tr
		} else {
			rpcErr = &jsonRPCError{Code: -32601, Message: "tool not found"}
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
		resourceResult, ok := m.resourceResults[readParams.URI]
		m.mu.Unlock()
		if ok {
			result = resourceResult
		} else {
			rpcErr = &jsonRPCError{Code: -32602, Message: "resource not found"}
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
		promptResult, ok := m.promptResults[getParams.Name]
		m.mu.Unlock()
		if ok {
			result = promptResult
		} else {
			rpcErr = &jsonRPCError{Code: -32602, Message: "prompt not found"}
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

	respData, _ := json.Marshal(resp)

	// Send response via SSE through the channel (thread-safe).
	m.eventCh <- sseEvent{
		eventType: "message",
		data:      string(respData),
	}

	w.WriteHeader(http.StatusAccepted)
}

func TestSSEClientConnect(t *testing.T) {
	mock := newMockSSEServer()
	server := httptest.NewServer(mock.handler())
	defer server.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	client, err := NewSSEClient(ctx, server.URL+"/sse")
	if err != nil {
		t.Fatalf("failed to create SSE client: %v", err)
	}
	defer client.Close()

	if client.messagesURL == "" {
		t.Error("expected messagesURL to be set")
	}
}

func TestSSEClientListTools(t *testing.T) {
	mock := newMockSSEServer()
	mock.tools = []Tool{
		{
			Name:        "get_weather",
			Description: "Get weather for a city",
			InputSchema: json.RawMessage(`{"type":"object","properties":{"city":{"type":"string"}}}`),
		},
		{
			Name:        "get_time",
			Description: "Get current time",
			InputSchema: json.RawMessage(`{"type":"object"}`),
		},
	}

	server := httptest.NewServer(mock.handler())
	defer server.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	client, err := NewSSEClient(ctx, server.URL+"/sse")
	if err != nil {
		t.Fatalf("failed to create SSE client: %v", err)
	}
	defer client.Close()

	tools, err := client.ListTools(ctx)
	if err != nil {
		t.Fatalf("ListTools failed: %v", err)
	}

	if len(tools) != 2 {
		t.Fatalf("expected 2 tools, got %d", len(tools))
	}
	if tools[0].Name != "get_weather" {
		t.Errorf("expected get_weather, got %s", tools[0].Name)
	}
	if tools[1].Name != "get_time" {
		t.Errorf("expected get_time, got %s", tools[1].Name)
	}
}

func TestSSEClientCallTool(t *testing.T) {
	mock := newMockSSEServer()
	mock.toolResults["get_weather"] = &ToolResult{
		Content: []Content{
			{Type: "text", Text: "Sunny, 72F"},
		},
	}

	server := httptest.NewServer(mock.handler())
	defer server.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	client, err := NewSSEClient(ctx, server.URL+"/sse")
	if err != nil {
		t.Fatalf("failed to create SSE client: %v", err)
	}
	defer client.Close()

	result, err := client.CallTool(ctx, "get_weather", map[string]any{"city": "NYC"})
	if err != nil {
		t.Fatalf("CallTool failed: %v", err)
	}

	if result.TextContent() != "Sunny, 72F" {
		t.Errorf("expected 'Sunny, 72F', got '%s'", result.TextContent())
	}
}

func TestSSEClientCallToolError(t *testing.T) {
	mock := newMockSSEServer()

	server := httptest.NewServer(mock.handler())
	defer server.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	client, err := NewSSEClient(ctx, server.URL+"/sse")
	if err != nil {
		t.Fatalf("failed to create SSE client: %v", err)
	}
	defer client.Close()

	_, err = client.CallTool(ctx, "nonexistent", nil)
	if err == nil {
		t.Fatal("expected error for nonexistent tool")
	}
}

func TestSSEClientResourcesAndPrompts(t *testing.T) {
	mock := newMockSSEServer()
	mock.resources = []Resource{
		{
			URI:         "file:///workspace/README.md",
			Name:        "README",
			Description: "Project readme",
			MIMEType:    "text/markdown",
		},
	}
	mock.resourceTemplates = []ResourceTemplate{
		{
			URITemplate: "file:///workspace/{path}",
			Name:        "workspace_file",
			Description: "Template for project files",
			MIMEType:    "text/plain",
		},
	}
	mock.resourceResults["file:///workspace/README.md"] = &ReadResourceResult{
		Contents: []ResourceContents{{
			URI:      "file:///workspace/README.md",
			MIMEType: "text/markdown",
			Text:     "# Gollem\n",
		}},
	}
	mock.prompts = []Prompt{
		{
			Name:        "summarize_repo",
			Description: "Summarize the repository",
		},
	}
	mock.promptResults["summarize_repo"] = &PromptResult{
		Messages: []PromptMessage{{
			Role:    "assistant",
			Content: Content{Type: "text", Text: "Repository summary"},
		}},
	}

	server := httptest.NewServer(mock.handler())
	defer server.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	client, err := NewSSEClient(ctx, server.URL+"/sse")
	if err != nil {
		t.Fatalf("failed to create SSE client: %v", err)
	}
	defer client.Close()

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
		t.Fatalf("unexpected read result: %q", readResult.TextContent())
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
	if promptResult.TextContent() != "assistant: Repository summary" {
		t.Fatalf("unexpected prompt content: %q", promptResult.TextContent())
	}
}

func TestSSEClientNotificationHandler(t *testing.T) {
	mock := newMockSSEServer()
	server := httptest.NewServer(mock.handler())
	defer server.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	client, err := NewSSEClient(ctx, server.URL+"/sse")
	if err != nil {
		t.Fatalf("failed to create SSE client: %v", err)
	}
	defer client.Close()

	received := make(chan string, 1)
	unregister := client.OnNotification("notifications/resources/list_changed", func(note Notification) {
		received <- note.Method
	})
	defer unregister()

	notification, _ := json.Marshal(jsonRPCMessage{
		JSONRPC: "2.0",
		Method:  "notifications/resources/list_changed",
		Params:  json.RawMessage(`{"reason":"refresh"}`),
	})
	mock.eventCh <- sseEvent{
		eventType: "message",
		data:      string(notification),
	}

	select {
	case method := <-received:
		if method != "notifications/resources/list_changed" {
			t.Fatalf("unexpected notification method: %s", method)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for notification")
	}
}

func TestSSEClientTools(t *testing.T) {
	mock := newMockSSEServer()
	mock.tools = []Tool{
		{
			Name:        "echo",
			Description: "Echo input",
			InputSchema: json.RawMessage(`{"type":"object","properties":{"text":{"type":"string"}}}`),
		},
	}
	mock.toolResults["echo"] = &ToolResult{
		Content: []Content{{Type: "text", Text: "echoed"}},
	}

	server := httptest.NewServer(mock.handler())
	defer server.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	client, err := NewSSEClient(ctx, server.URL+"/sse")
	if err != nil {
		t.Fatalf("failed to create SSE client: %v", err)
	}
	defer client.Close()

	tools, err := client.Tools(ctx)
	if err != nil {
		t.Fatalf("Tools failed: %v", err)
	}

	if len(tools) != 1 {
		t.Fatalf("expected 1 tool, got %d", len(tools))
	}
	if tools[0].Definition.Name != "echo" {
		t.Errorf("expected echo, got %s", tools[0].Definition.Name)
	}
	if tools[0].Handler == nil {
		t.Error("expected handler to be set")
	}
}

func TestSSEClientClose(t *testing.T) {
	mock := newMockSSEServer()
	server := httptest.NewServer(mock.handler())
	defer server.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	client, err := NewSSEClient(ctx, server.URL+"/sse")
	if err != nil {
		t.Fatalf("failed to create SSE client: %v", err)
	}

	err = client.Close()
	if err != nil {
		t.Errorf("Close returned error: %v", err)
	}

	// Double close should not panic.
	err = client.Close()
	if err != nil {
		t.Errorf("second Close returned error: %v", err)
	}
}

func TestSSEClientWithCustomHTTPClient(t *testing.T) {
	mock := newMockSSEServer()
	server := httptest.NewServer(mock.handler())
	defer server.Close()

	customClient := &http.Client{Timeout: 10 * time.Second}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	client, err := NewSSEClient(ctx, server.URL+"/sse", WithHTTPClient(customClient))
	if err != nil {
		t.Fatalf("failed to create SSE client: %v", err)
	}
	defer client.Close()

	if client.httpClient != customClient {
		t.Error("expected custom HTTP client to be used")
	}
}

func TestSSEClientNoSpaceAfterColon(t *testing.T) {
	// Per the SSE spec, "event:message" (no space after colon) is valid.
	// Some MCP server implementations omit the optional space. This test
	// verifies the client handles both forms correctly.
	mock := newMockSSEServer()
	mock.tools = []Tool{
		{
			Name:        "get_data",
			Description: "Get data",
			InputSchema: json.RawMessage(`{"type":"object"}`),
		},
	}

	// Override the SSE handler to send events without spaces after colons.
	mux := http.NewServeMux()
	mux.HandleFunc("/sse", func(w http.ResponseWriter, r *http.Request) {
		flusher, ok := w.(http.Flusher)
		if !ok {
			http.Error(w, "streaming not supported", http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache")

		// Send endpoint event WITHOUT space after colon.
		fmt.Fprintf(w, "event:endpoint\ndata:/messages\n\n")
		flusher.Flush()

		select {
		case <-mock.ready:
		default:
			close(mock.ready)
		}

		for {
			select {
			case ev := <-mock.eventCh:
				// Send response events WITHOUT space after colons.
				fmt.Fprintf(w, "event:%s\ndata:%s\n\n", ev.eventType, ev.data)
				flusher.Flush()
			case <-r.Context().Done():
				return
			}
		}
	})
	mux.HandleFunc("/messages", mock.handleMessages)

	server := httptest.NewServer(mux)
	defer server.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	client, err := NewSSEClient(ctx, server.URL+"/sse")
	if err != nil {
		t.Fatalf("failed to create SSE client with no-space events: %v", err)
	}
	defer client.Close()

	// Verify the endpoint was correctly parsed.
	if client.messagesURL == "" {
		t.Fatal("messagesURL was not set — event:endpoint without space was not parsed")
	}

	// Verify tools can be listed (requires message events to be parsed).
	tools, err := client.ListTools(ctx)
	if err != nil {
		t.Fatalf("ListTools failed with no-space events: %v", err)
	}
	if len(tools) != 1 {
		t.Fatalf("expected 1 tool, got %d", len(tools))
	}
	if tools[0].Name != "get_data" {
		t.Errorf("expected get_data, got %s", tools[0].Name)
	}
}

func TestSSEClientLargeToolResult(t *testing.T) {
	// MCP tool results can exceed the default bufio.Scanner max token size
	// of 64KB (e.g., file contents, command output). Verify that large
	// responses are handled correctly.
	mock := newMockSSEServer()

	// Create a tool result larger than bufio.MaxScanTokenSize (64KB).
	largeText := strings.Repeat("x", 100*1024) // 100KB
	mock.toolResults["read_file"] = &ToolResult{
		Content: []Content{{Type: "text", Text: largeText}},
	}

	server := httptest.NewServer(mock.handler())
	defer server.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	client, err := NewSSEClient(ctx, server.URL+"/sse")
	if err != nil {
		t.Fatalf("failed to create SSE client: %v", err)
	}
	defer client.Close()

	result, err := client.CallTool(ctx, "read_file", map[string]any{"path": "/big/file"})
	if err != nil {
		t.Fatalf("CallTool failed for large result: %v", err)
	}

	got := result.TextContent()
	if len(got) != 100*1024 {
		t.Errorf("expected 100KB result, got %d bytes", len(got))
	}
}

func TestSSEResolveURL(t *testing.T) {
	c := &SSEClient{baseURL: "http://localhost:8080/sse"}

	tests := []struct {
		input    string
		expected string
	}{
		{"/messages", "http://localhost:8080/messages"},
		{"http://other.host/messages", "http://other.host/messages"},
		{"https://other.host/messages", "https://other.host/messages"},
	}

	for _, tc := range tests {
		result := c.resolveURL(tc.input)
		if result != tc.expected {
			t.Errorf("resolveURL(%q) = %q, expected %q", tc.input, result, tc.expected)
		}
	}
}
