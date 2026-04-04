package mcp

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/fugue-labs/gollem/core"
)

// pipeWriteCloser wraps an io.Writer as an io.WriteCloser.
type pipeWriteCloser struct {
	w *io.PipeWriter
}

func (p *pipeWriteCloser) Write(b []byte) (int, error) { return p.w.Write(b) }
func (p *pipeWriteCloser) Close() error                { return p.w.Close() }

type discardWriteCloser struct {
	io.Writer
}

func (discardWriteCloser) Close() error { return nil }

// mockStdioServer simulates an MCP server process connected via pipes.
// It reads JSON-RPC requests from the "stdin" pipe and writes responses
// to the "stdout" pipe, mimicking what a real MCP server subprocess does.
type mockStdioServer struct {
	mu                sync.Mutex
	tools             []Tool
	toolResults       map[string]*ToolResult
	resources         []Resource
	resourceTemplates []ResourceTemplate
	resourceResults   map[string]*ReadResourceResult
	prompts           []Prompt
	promptResults     map[string]*PromptResult
	reader            *bufio.Reader
	writer            io.Writer
}

func (m *mockStdioServer) serve() {
	for {
		line, err := m.reader.ReadBytes('\n')
		if err != nil {
			return
		}

		var req jsonRPCRequest
		if err := json.Unmarshal(line, &req); err != nil {
			continue
		}

		// Notifications have ID 0 (our implementation uses int64, starting at 1).
		// The initialized notification uses no ID in the spec, but our struct
		// will deserialize it as 0.
		if req.Method == "notifications/initialized" {
			continue // notification, no response
		}

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
					"name":    "mock-stdio-server",
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
		fmt.Fprintf(m.writer, "%s\n", respData)
	}
}

// newMockStdioClient creates a Client connected to a mock server via pipes.
// It wires up stdin/stdout pipes with a mock server goroutine, starts
// readLoop, and performs the initialization handshake — just like
// NewStdioClient does, but without spawning a subprocess.
//
// The returned Client has no exec.Cmd, so don't call Close() on it directly;
// the cleanup function handles shutting down pipes instead.
func newMockStdioClient(t *testing.T, tools []Tool, results map[string]*ToolResult) *Client {
	t.Helper()

	// Client writes to clientWriter -> serverReader reads requests.
	serverReader, clientWriter := io.Pipe()
	// Server writes to serverWriter -> clientReader reads responses.
	clientReader, serverWriter := io.Pipe()

	mock := &mockStdioServer{
		tools:           tools,
		toolResults:     results,
		resourceResults: make(map[string]*ReadResourceResult),
		promptResults:   make(map[string]*PromptResult),
		reader:          bufio.NewReader(serverReader),
		writer:          serverWriter,
	}
	go mock.serve()

	c := &Client{
		clientState: newClientState(),
		stdin:       &pipeWriteCloser{w: clientWriter},
		stdout:      bufio.NewReader(clientReader),
	}

	// Start the read loop (same as NewStdioClient).
	go c.readLoop()

	// Perform initialization handshake.
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := c.initialize(ctx); err != nil {
		serverWriter.Close()
		serverReader.Close()
		t.Fatalf("initialization failed: %v", err)
	}

	// Clean up: close stdin pipe (kills readLoop), then close server pipes.
	// We don't call c.Close() because it dereferences c.cmd which is nil.
	t.Cleanup(func() {
		c.mu.Lock()
		c.closed = true
		c.mu.Unlock()
		clientWriter.Close()
		serverWriter.Close()
		serverReader.Close()
	})

	return c
}

func TestStdioClientInitialize(t *testing.T) {
	c := newMockStdioClient(t, nil, nil)
	if c.closed {
		t.Error("client should not be closed after initialization")
	}
}

func TestStdioClientListTools(t *testing.T) {
	tools := []Tool{
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

	c := newMockStdioClient(t, tools, nil)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	result, err := c.ListTools(ctx)
	if err != nil {
		t.Fatalf("ListTools failed: %v", err)
	}
	if len(result) != 2 {
		t.Fatalf("expected 2 tools, got %d", len(result))
	}
	if result[0].Name != "get_weather" {
		t.Errorf("expected get_weather, got %s", result[0].Name)
	}
	if result[1].Name != "get_time" {
		t.Errorf("expected get_time, got %s", result[1].Name)
	}
}

func TestStdioClientCallTool(t *testing.T) {
	results := map[string]*ToolResult{
		"get_weather": {
			Content: []Content{{Type: "text", Text: "Sunny, 72F"}},
		},
	}
	c := newMockStdioClient(t, nil, results)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	result, err := c.CallTool(ctx, "get_weather", map[string]any{"city": "NYC"})
	if err != nil {
		t.Fatalf("CallTool failed: %v", err)
	}
	if result.TextContent() != "Sunny, 72F" {
		t.Errorf("expected 'Sunny, 72F', got '%s'", result.TextContent())
	}
}

func TestStdioClientCallToolNotFound(t *testing.T) {
	c := newMockStdioClient(t, nil, nil)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	_, err := c.CallTool(ctx, "nonexistent", nil)
	if err == nil {
		t.Fatal("expected error for nonexistent tool")
	}
	if !strings.Contains(err.Error(), "tool not found") {
		t.Errorf("expected 'tool not found' error, got: %v", err)
	}
}

func TestStdioClientCallClosed(t *testing.T) {
	c := newMockStdioClient(t, nil, nil)

	// Manually mark closed (can't call c.Close() since cmd is nil in mock).
	c.mu.Lock()
	c.closed = true
	c.mu.Unlock()

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	_, err := c.call(ctx, "tools/list", nil)
	if err == nil {
		t.Fatal("expected error on closed client")
	}
	if !strings.Contains(err.Error(), "closed") {
		t.Errorf("expected 'closed' error, got: %v", err)
	}
}

func TestStdioClientCloseIdempotent(t *testing.T) {
	// Verify that marking a client closed multiple times doesn't panic.
	c := newMockStdioClient(t, nil, nil)

	c.mu.Lock()
	c.closed = true
	c.mu.Unlock()

	// Marking closed again should be fine.
	c.mu.Lock()
	c.closed = true
	c.mu.Unlock()

	// Calling call() on closed client returns error.
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	_, err := c.call(ctx, "test", nil)
	if err == nil {
		t.Fatal("expected error on closed client")
	}
}

func TestStdioClientConcurrentCalls(t *testing.T) {
	results := map[string]*ToolResult{
		"echo": {
			Content: []Content{{Type: "text", Text: "echoed"}},
		},
	}
	c := newMockStdioClient(t, nil, results)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Fire 10 concurrent calls.
	const n = 10
	errs := make(chan error, n)
	for i := range n {
		go func() {
			result, err := c.CallTool(ctx, "echo", map[string]any{"i": i})
			if err != nil {
				errs <- err
				return
			}
			if result.TextContent() != "echoed" {
				errs <- fmt.Errorf("unexpected result: %s", result.TextContent())
				return
			}
			errs <- nil
		}()
	}

	for i := range n {
		if err := <-errs; err != nil {
			t.Errorf("concurrent call %d failed: %v", i, err)
		}
	}
}

func TestStdioClientContextCancellation(t *testing.T) {
	// Create a client with a server that never responds.
	serverReader, clientWriter := io.Pipe()
	clientReader, serverWriter := io.Pipe()

	// Drain server reader but never write responses.
	go func() {
		buf := make([]byte, 4096)
		for {
			_, err := serverReader.Read(buf)
			if err != nil {
				return
			}
		}
	}()

	c := &Client{
		clientState: newClientState(),
		stdin:       &pipeWriteCloser{w: clientWriter},
		stdout:      bufio.NewReader(clientReader),
	}
	go c.readLoop()

	t.Cleanup(func() {
		c.mu.Lock()
		c.closed = true
		c.mu.Unlock()
		clientWriter.Close()
		serverWriter.Close()
		serverReader.Close()
	})

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	_, err := c.call(ctx, "tools/list", nil)
	if err == nil {
		t.Fatal("expected error from context cancellation")
	}
	if !strings.Contains(err.Error(), "context deadline exceeded") {
		t.Errorf("expected context deadline error, got: %v", err)
	}

	// Verify pending entry was cleaned up.
	c.mu.Lock()
	pendingCount := len(c.pending)
	c.mu.Unlock()
	if pendingCount != 0 {
		t.Errorf("expected 0 pending entries after cancellation, got %d", pendingCount)
	}
}

func TestStdioClientTools(t *testing.T) {
	tools := []Tool{
		{
			Name:        "echo",
			Description: "Echo input",
			InputSchema: json.RawMessage(`{"type":"object","properties":{"text":{"type":"string"}}}`),
		},
	}
	results := map[string]*ToolResult{
		"echo": {Content: []Content{{Type: "text", Text: "hello"}}},
	}

	c := newMockStdioClient(t, tools, results)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	coreTools, err := c.Tools(ctx)
	if err != nil {
		t.Fatalf("Tools failed: %v", err)
	}
	if len(coreTools) != 1 {
		t.Fatalf("expected 1 tool, got %d", len(coreTools))
	}
	if coreTools[0].Definition.Name != "echo" {
		t.Errorf("expected echo, got %s", coreTools[0].Definition.Name)
	}

	// Actually invoke the handler — this exercises convertTool's handler path.
	result, err := coreTools[0].Handler(ctx, nil, `{"text":"hello"}`)
	if err != nil {
		t.Fatalf("handler failed: %v", err)
	}
	if result != "hello" {
		t.Errorf("expected 'hello', got '%v'", result)
	}
}

func TestStdioClientToolsHandlerEmptyArgs(t *testing.T) {
	tools := []Tool{
		{
			Name:        "noop",
			Description: "No-op tool",
			InputSchema: json.RawMessage(`{"type":"object"}`),
		},
	}
	results := map[string]*ToolResult{
		"noop": {Content: []Content{{Type: "text", Text: "done"}}},
	}

	c := newMockStdioClient(t, tools, results)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	coreTools, err := c.Tools(ctx)
	if err != nil {
		t.Fatalf("Tools failed: %v", err)
	}

	// Empty args string.
	result, err := coreTools[0].Handler(ctx, nil, "")
	if err != nil {
		t.Fatalf("handler failed with empty args: %v", err)
	}
	if result != "done" {
		t.Errorf("expected 'done', got '%v'", result)
	}

	// Empty object args.
	result, err = coreTools[0].Handler(ctx, nil, "{}")
	if err != nil {
		t.Fatalf("handler failed with empty object args: %v", err)
	}
	if result != "done" {
		t.Errorf("expected 'done', got '%v'", result)
	}
}

func TestStdioClientToolsHandlerIsError(t *testing.T) {
	tools := []Tool{
		{
			Name:        "failing",
			Description: "Tool that returns isError",
			InputSchema: json.RawMessage(`{"type":"object"}`),
		},
	}
	results := map[string]*ToolResult{
		"failing": {
			Content: []Content{{Type: "text", Text: "something went wrong"}},
			IsError: true,
		},
	}

	c := newMockStdioClient(t, tools, results)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	coreTools, err := c.Tools(ctx)
	if err != nil {
		t.Fatalf("Tools failed: %v", err)
	}

	// Handler should return a ModelRetryError when IsError is true.
	_, err = coreTools[0].Handler(ctx, nil, "{}")
	if err == nil {
		t.Fatal("expected error for isError tool result")
	}
	retryErr, ok := err.(*core.ModelRetryError)
	if !ok {
		t.Fatalf("expected *ModelRetryError, got %T: %v", err, err)
	}
	if retryErr.Message != "something went wrong" {
		t.Errorf("expected 'something went wrong', got '%s'", retryErr.Message)
	}
}

func TestStdioClientToolsHandlerInvalidJSON(t *testing.T) {
	tools := []Tool{
		{
			Name:        "echo",
			Description: "Echo",
			InputSchema: json.RawMessage(`{"type":"object"}`),
		},
	}
	results := map[string]*ToolResult{
		"echo": {Content: []Content{{Type: "text", Text: "ok"}}},
	}

	c := newMockStdioClient(t, tools, results)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	coreTools, err := c.Tools(ctx)
	if err != nil {
		t.Fatalf("Tools failed: %v", err)
	}

	// Invalid JSON args should return an error.
	_, err = coreTools[0].Handler(ctx, nil, "not-valid-json")
	if err == nil {
		t.Fatal("expected error for invalid JSON args")
	}
}

func TestStdioClientReadLoopClosePending(t *testing.T) {
	// When the server pipe closes, readLoop should wake all pending callers.
	serverReader, clientWriter := io.Pipe()
	clientReader, serverWriter := io.Pipe()

	c := &Client{
		clientState: newClientState(),
		stdin:       &pipeWriteCloser{w: clientWriter},
		stdout:      bufio.NewReader(clientReader),
	}
	go c.readLoop()

	// Drain requests from the server side.
	go func() {
		buf := make([]byte, 4096)
		for {
			_, err := serverReader.Read(buf)
			if err != nil {
				return
			}
		}
	}()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	errCh := make(chan error, 1)
	go func() {
		_, err := c.call(ctx, "tools/list", nil)
		errCh <- err
	}()

	// Wait for the pending entry to appear, then close the server writer
	// to simulate the server process dying.
	deadline := time.After(2 * time.Second)
	for {
		c.mu.Lock()
		n := len(c.pending)
		c.mu.Unlock()
		if n > 0 {
			break
		}
		select {
		case <-deadline:
			t.Fatal("timed out waiting for pending entry")
		default:
			time.Sleep(5 * time.Millisecond)
		}
	}

	serverWriter.Close()

	err := <-errCh
	if err == nil {
		t.Fatal("expected error when server pipe closes")
	}

	t.Cleanup(func() {
		c.mu.Lock()
		c.closed = true
		c.mu.Unlock()
		clientWriter.Close()
		serverReader.Close()
	})
}

func TestStdioServerTransportEOFClosesPendingRequests(t *testing.T) {
	server := NewServer()
	transport := NewStdioServerTransport(server, strings.NewReader(""), discardWriteCloser{Writer: io.Discard})

	id, ch, err := server.prepareCall()
	if err != nil {
		t.Fatalf("prepareCall failed: %v", err)
	}

	errCh := make(chan error, 1)
	go func() {
		_, err := server.awaitResponse(context.Background(), id, ch)
		errCh <- err
	}()

	if err := transport.Run(context.Background()); err != nil {
		t.Fatalf("Run returned error: %v", err)
	}

	select {
	case err := <-errCh:
		if err == nil {
			t.Fatal("expected pending request to fail on EOF")
		}
		if !strings.Contains(err.Error(), "connection closed") {
			t.Fatalf("unexpected pending-request error: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for pending request to unblock")
	}
}

func TestStdioServerTransportEOFRejectsNewNestedRequests(t *testing.T) {
	server := NewServer()
	transport := NewStdioServerTransport(server, strings.NewReader(""), discardWriteCloser{Writer: io.Discard})

	if err := transport.Run(context.Background()); err != nil {
		t.Fatalf("Run returned error: %v", err)
	}

	if _, _, err := server.prepareCall(); err == nil {
		t.Fatal("expected prepareCall to fail after EOF")
	} else if !strings.Contains(err.Error(), "connection closed") {
		t.Fatalf("unexpected prepareCall error: %v", err)
	}
}

func TestStdioClientLargeResponse(t *testing.T) {
	// Verify stdio client can handle large tool results (>64KB).
	largeText := strings.Repeat("x", 100*1024) // 100KB
	results := map[string]*ToolResult{
		"read_file": {
			Content: []Content{{Type: "text", Text: largeText}},
		},
	}
	c := newMockStdioClient(t, nil, results)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	result, err := c.CallTool(ctx, "read_file", map[string]any{"path": "/big"})
	if err != nil {
		t.Fatalf("CallTool failed for large response: %v", err)
	}
	if len(result.TextContent()) != 100*1024 {
		t.Errorf("expected 100KB result, got %d bytes", len(result.TextContent()))
	}
}

func TestStdioClientResourcesAndPrompts(t *testing.T) {
	// Client writes to clientWriter -> serverReader reads requests.
	serverReader, clientWriter := io.Pipe()
	// Server writes to serverWriter -> clientReader reads responses.
	clientReader, serverWriter := io.Pipe()

	mock := &mockStdioServer{
		resources: []Resource{
			{
				URI:         "file:///workspace/README.md",
				Name:        "README",
				Description: "Project readme",
				MIMEType:    "text/markdown",
			},
		},
		resourceTemplates: []ResourceTemplate{
			{
				URITemplate: "file:///workspace/{path}",
				Name:        "workspace_file",
				Description: "Template for project files",
				MIMEType:    "text/plain",
			},
		},
		resourceResults: map[string]*ReadResourceResult{
			"file:///workspace/README.md": {
				Contents: []ResourceContents{{
					URI:      "file:///workspace/README.md",
					MIMEType: "text/markdown",
					Text:     "# Gollem\n",
				}},
			},
		},
		prompts: []Prompt{
			{
				Name:        "summarize_repo",
				Description: "Summarize the repository",
				Arguments: []PromptArgument{{
					Name:        "focus",
					Description: "Optional focus area",
				}},
			},
		},
		promptResults: map[string]*PromptResult{
			"summarize_repo": {
				Messages: []PromptMessage{{
					Role:    "user",
					Content: Content{Type: "text", Text: "Summarize the repository."},
				}},
			},
		},
		reader:      bufio.NewReader(serverReader),
		writer:      serverWriter,
		toolResults: make(map[string]*ToolResult),
	}
	go mock.serve()

	c := &Client{
		clientState: newClientState(),
		stdin:       &pipeWriteCloser{w: clientWriter},
		stdout:      bufio.NewReader(clientReader),
	}
	go c.readLoop()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := c.initialize(ctx); err != nil {
		t.Fatalf("initialization failed: %v", err)
	}

	t.Cleanup(func() {
		c.shutdown()
		clientWriter.Close()
		serverWriter.Close()
		serverReader.Close()
	})

	resources, err := c.ListResources(ctx)
	if err != nil {
		t.Fatalf("ListResources failed: %v", err)
	}
	if len(resources) != 1 || resources[0].URI != "file:///workspace/README.md" {
		t.Fatalf("unexpected resources: %+v", resources)
	}

	resourceResult, err := c.ReadResource(ctx, "file:///workspace/README.md")
	if err != nil {
		t.Fatalf("ReadResource failed: %v", err)
	}
	if resourceResult.TextContent() != "# Gollem\n" {
		t.Fatalf("unexpected resource content: %q", resourceResult.TextContent())
	}

	templates, err := c.ListResourceTemplates(ctx)
	if err != nil {
		t.Fatalf("ListResourceTemplates failed: %v", err)
	}
	if len(templates) != 1 || templates[0].URITemplate != "file:///workspace/{path}" {
		t.Fatalf("unexpected templates: %+v", templates)
	}

	prompts, err := c.ListPrompts(ctx)
	if err != nil {
		t.Fatalf("ListPrompts failed: %v", err)
	}
	if len(prompts) != 1 || prompts[0].Name != "summarize_repo" {
		t.Fatalf("unexpected prompts: %+v", prompts)
	}

	promptResult, err := c.GetPrompt(ctx, "summarize_repo", map[string]string{"focus": "architecture"})
	if err != nil {
		t.Fatalf("GetPrompt failed: %v", err)
	}
	if promptResult.TextContent() != "user: Summarize the repository." {
		t.Fatalf("unexpected prompt content: %q", promptResult.TextContent())
	}

	if c.ProtocolVersion() != ProtocolVersion {
		t.Fatalf("unexpected protocol version: %s", c.ProtocolVersion())
	}
	if c.ServerInfo() == nil || c.ServerInfo().Name != "mock-stdio-server" {
		t.Fatalf("unexpected server info: %+v", c.ServerInfo())
	}
	if c.Capabilities().Resources == nil || c.Capabilities().Prompts == nil {
		t.Fatalf("expected resource and prompt capabilities, got %+v", c.Capabilities())
	}
}

func TestStdioClientNotificationHandler(t *testing.T) {
	serverReader, clientWriter := io.Pipe()
	clientReader, serverWriter := io.Pipe()

	c := &Client{
		clientState: newClientState(),
		stdin:       &pipeWriteCloser{w: clientWriter},
		stdout:      bufio.NewReader(clientReader),
	}
	go c.readLoop()

	t.Cleanup(func() {
		c.shutdown()
		clientWriter.Close()
		serverWriter.Close()
		serverReader.Close()
	})

	// Drain writes so notify() can succeed if used by future setup changes.
	go func() {
		_, _ = io.Copy(io.Discard, serverReader)
	}()

	received := make(chan string, 1)
	unregister := c.OnNotification("notifications/tools/list_changed", func(note Notification) {
		received <- note.Method
	})
	defer unregister()

	data, _ := json.Marshal(jsonRPCMessage{
		JSONRPC: "2.0",
		Method:  "notifications/tools/list_changed",
		Params:  json.RawMessage(`{"reason":"refresh"}`),
	})
	fmt.Fprintf(serverWriter, "%s\n", data)

	select {
	case method := <-received:
		if method != "notifications/tools/list_changed" {
			t.Fatalf("unexpected notification method: %s", method)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for notification")
	}
}

// TestConvertToolInvalidSchema verifies that convertTool handles unparseable
// InputSchema gracefully by falling back to a default {"type":"object"} schema.
func TestConvertToolInvalidSchema(t *testing.T) {
	mcpTool := Tool{
		Name:        "bad_schema",
		Description: "Tool with invalid schema",
		InputSchema: json.RawMessage(`not valid json`),
	}

	tool := convertTool(nil, mcpTool)
	schema := tool.Definition.ParametersSchema
	if schema["type"] != "object" {
		t.Errorf("expected fallback type=object, got %v", schema["type"])
	}
}

// TestSSEConvertToolHandlerIsError verifies that the SSE convertSSETool handler
// returns ModelRetryError when the tool result has IsError set.
func TestSSEConvertToolHandlerIsError(t *testing.T) {
	mock := newMockSSEServer()
	mock.tools = []Tool{
		{
			Name:        "failing",
			Description: "Failing tool",
			InputSchema: json.RawMessage(`{"type":"object"}`),
		},
	}
	mock.toolResults["failing"] = &ToolResult{
		Content: []Content{{Type: "text", Text: "server error"}},
		IsError: true,
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

	coreTools, err := client.Tools(ctx)
	if err != nil {
		t.Fatalf("Tools failed: %v", err)
	}

	_, err = coreTools[0].Handler(ctx, nil, "{}")
	if err == nil {
		t.Fatal("expected error for isError tool result")
	}
	retryErr, ok := err.(*core.ModelRetryError)
	if !ok {
		t.Fatalf("expected *ModelRetryError, got %T: %v", err, err)
	}
	if retryErr.Message != "server error" {
		t.Errorf("expected 'server error', got '%s'", retryErr.Message)
	}
}

// TestSSEConvertToolHandlerSuccess verifies SSE tool handler invocation.
func TestSSEConvertToolHandlerSuccess(t *testing.T) {
	mock := newMockSSEServer()
	mock.tools = []Tool{
		{
			Name:        "echo",
			Description: "Echo tool",
			InputSchema: json.RawMessage(`{"type":"object","properties":{"msg":{"type":"string"}}}`),
		},
	}
	mock.toolResults["echo"] = &ToolResult{
		Content: []Content{{Type: "text", Text: "echoed back"}},
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

	coreTools, err := client.Tools(ctx)
	if err != nil {
		t.Fatalf("Tools failed: %v", err)
	}

	result, err := coreTools[0].Handler(ctx, nil, `{"msg":"hello"}`)
	if err != nil {
		t.Fatalf("handler failed: %v", err)
	}
	if result != "echoed back" {
		t.Errorf("expected 'echoed back', got '%v'", result)
	}
}
