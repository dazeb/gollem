package mcp

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"

	"github.com/fugue-labs/gollem/core"
)

// SSEClient communicates with an MCP server over the deprecated SSE transport.
// The client connects to an SSE endpoint to receive JSON-RPC messages and posts
// requests to the messages endpoint announced by the server.
type SSEClient struct {
	*clientState

	baseURL    string
	httpClient *http.Client
	headers    map[string]string

	closeOnce   sync.Once
	cancelSSE   context.CancelFunc
	messagesURL string
	ready       chan struct{}
}

// NewSSEClient connects to an MCP server over SSE at the given URL.
// The URL should point to the SSE endpoint (for example, "http://localhost:8080/sse").
func NewSSEClient(ctx context.Context, url string, opts ...SSEOption) (*SSEClient, error) {
	return NewSSEClientWithConfig(ctx, url, ClientConfig{}, opts...)
}

// NewSSEClientWithConfig connects to an MCP server over SSE at the given URL.
func NewSSEClientWithConfig(ctx context.Context, url string, config ClientConfig, opts ...SSEOption) (*SSEClient, error) {
	cfg := defaultRemoteConfig()
	for _, opt := range opts {
		if opt != nil {
			opt(&cfg)
		}
	}

	sseCtx, cancel := context.WithCancel(context.Background())

	c := &SSEClient{
		clientState: newClientState(config),
		baseURL:     url,
		httpClient:  cfg.httpClient,
		headers:     cloneStringMap(cfg.headers),
		cancelSSE:   cancel,
		ready:       make(chan struct{}),
	}

	if err := c.connectSSE(sseCtx); err != nil {
		cancel()
		return nil, fmt.Errorf("mcp: failed to connect SSE: %w", err)
	}

	select {
	case <-c.ready:
	case <-ctx.Done():
		cancel()
		return nil, fmt.Errorf("mcp: timed out waiting for endpoint event: %w", ctx.Err())
	}

	if err := c.initialize(ctx); err != nil {
		c.Close()
		return nil, fmt.Errorf("mcp: initialization failed: %w", err)
	}

	return c, nil
}

func (c *SSEClient) connectSSE(ctx context.Context) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Accept", "text/event-stream")
	applyHeaders(req, c.headers)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}
	if resp.StatusCode != http.StatusOK {
		resp.Body.Close()
		return fmt.Errorf("mcp: SSE connection returned status %d", resp.StatusCode)
	}

	go c.readSSE(resp.Body)
	return nil
}

func (c *SSEClient) readSSE(body io.ReadCloser) {
	if err := readEventStream(body, c.handleSSEEvent); err != nil {
		c.shutdown()
		return
	}
	c.shutdown()
}

func (c *SSEClient) handleSSEEvent(eventType, data string) {
	switch eventType {
	case "endpoint":
		c.mu.Lock()
		c.messagesURL = c.resolveURL(strings.TrimSpace(data))
		c.mu.Unlock()
		select {
		case <-c.ready:
		default:
			close(c.ready)
		}
	default:
		var msg jsonRPCMessage
		if err := json.Unmarshal([]byte(data), &msg); err != nil {
			return
		}
		c.dispatchMessage(&msg, c.respond)
	}
}

func (c *SSEClient) resolveURL(endpoint string) string {
	if strings.HasPrefix(endpoint, "http://") || strings.HasPrefix(endpoint, "https://") {
		return endpoint
	}

	base := c.baseURL
	idx := strings.Index(base, "://")
	if idx < 0 {
		return endpoint
	}
	hostEnd := strings.Index(base[idx+3:], "/")
	if hostEnd < 0 {
		return base + endpoint
	}
	return base[:idx+3+hostEnd] + endpoint
}

func (c *SSEClient) initialize(ctx context.Context) error {
	return initializeClient(ctx, c.clientState, c.call, c.notify)
}

func (c *SSEClient) call(ctx context.Context, method string, params any) (json.RawMessage, error) {
	id, ch, err := c.prepareCall()
	if err != nil {
		return nil, err
	}

	req := jsonRPCRequest{
		JSONRPC: "2.0",
		ID:      id,
		Method:  method,
		Params:  params,
	}

	c.mu.Lock()
	messagesURL := c.messagesURL
	c.mu.Unlock()

	data, err := json.Marshal(req)
	if err != nil {
		c.removePending(id)
		return nil, err
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, messagesURL, bytes.NewReader(data))
	if err != nil {
		c.removePending(id)
		return nil, fmt.Errorf("mcp: failed to create request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	applyHeaders(httpReq, c.headers)
	applyProtocolVersionHeader(httpReq, c.ProtocolVersion())

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		c.removePending(id)
		return nil, fmt.Errorf("mcp: failed to send request: %w", err)
	}
	resp.Body.Close()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusAccepted {
		c.removePending(id)
		return nil, fmt.Errorf("mcp: server returned status %d", resp.StatusCode)
	}

	return c.awaitResponse(ctx, id, ch)
}

func (c *SSEClient) notify(ctx context.Context, method string, params any) error {
	req := struct {
		JSONRPC string `json:"jsonrpc"`
		Method  string `json:"method"`
		Params  any    `json:"params,omitempty"`
	}{
		JSONRPC: "2.0",
		Method:  method,
		Params:  params,
	}

	data, err := json.Marshal(req)
	if err != nil {
		return err
	}

	return c.postMessage(ctx, data)
}

func (c *SSEClient) respond(ctx context.Context, id any, result any, rpcErr *jsonRPCError) error {
	resp := jsonRPCResponse{
		JSONRPC: "2.0",
		ID:      id,
		Result:  result,
		Error:   rpcErr,
	}
	data, err := json.Marshal(resp)
	if err != nil {
		return err
	}
	return c.postMessage(ctx, data)
}

func (c *SSEClient) postMessage(ctx context.Context, data []byte) error {
	c.mu.Lock()
	messagesURL := c.messagesURL
	c.mu.Unlock()

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, messagesURL, bytes.NewReader(data))
	if err != nil {
		return err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	applyHeaders(httpReq, c.headers)
	applyProtocolVersionHeader(httpReq, c.ProtocolVersion())

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return fmt.Errorf("mcp: failed to post message: %w", err)
	}
	resp.Body.Close()
	return nil
}

// ListTools discovers available tools from the MCP server.
func (c *SSEClient) ListTools(ctx context.Context) ([]Tool, error) {
	return listTools(ctx, c.call)
}

// CallTool invokes a tool on the MCP server.
func (c *SSEClient) CallTool(ctx context.Context, name string, args map[string]any) (*ToolResult, error) {
	return callTool(ctx, c.call, name, args)
}

// ListResources lists resources exposed by the MCP server.
func (c *SSEClient) ListResources(ctx context.Context) ([]Resource, error) {
	return listResources(ctx, c.call)
}

// ReadResource reads a resource from the MCP server.
func (c *SSEClient) ReadResource(ctx context.Context, uri string) (*ReadResourceResult, error) {
	return readResource(ctx, c.call, uri)
}

// ListResourceTemplates lists URI templates exposed by the MCP server.
func (c *SSEClient) ListResourceTemplates(ctx context.Context) ([]ResourceTemplate, error) {
	return listResourceTemplates(ctx, c.call)
}

// ListPrompts lists prompts exposed by the MCP server.
func (c *SSEClient) ListPrompts(ctx context.Context) ([]Prompt, error) {
	return listPrompts(ctx, c.call)
}

// GetPrompt resolves a prompt from the MCP server.
func (c *SSEClient) GetPrompt(ctx context.Context, name string, args map[string]string) (*PromptResult, error) {
	return getPrompt(ctx, c.call, name, args)
}

// NotifyRootsListChanged emits notifications/roots/list_changed to the server.
func (c *SSEClient) NotifyRootsListChanged(ctx context.Context) error {
	return notifyRootsListChanged(ctx, c.notify)
}

// Tools converts MCP tools into core.Tool instances backed by the SSE client.
func (c *SSEClient) Tools(ctx context.Context) ([]core.Tool, error) {
	return toolsForSource(ctx, c)
}

// Close shuts down the SSE connection and releases resources.
func (c *SSEClient) Close() error {
	c.closeOnce.Do(func() {
		c.shutdown()
		if c.cancelSSE != nil {
			c.cancelSSE()
		}
	})
	return nil
}

func readEventStream(body io.ReadCloser, onEvent func(eventType, data string)) error {
	defer body.Close()

	scanner := bufio.NewScanner(body)
	scanner.Buffer(make([]byte, 64*1024), 10*1024*1024)

	var eventType string
	var dataLines []string

	dispatch := func() {
		if len(dataLines) == 0 {
			eventType = ""
			dataLines = nil
			return
		}
		currentType := eventType
		if currentType == "" {
			currentType = "message"
		}
		onEvent(currentType, strings.Join(dataLines, "\n"))
		eventType = ""
		dataLines = nil
	}

	for scanner.Scan() {
		line := scanner.Text()
		if line == "" {
			dispatch()
			continue
		}
		if strings.HasPrefix(line, ":") {
			continue
		}
		switch {
		case strings.HasPrefix(line, "event:"):
			eventType = strings.TrimPrefix(line, "event:")
			eventType = strings.TrimPrefix(eventType, " ")
		case strings.HasPrefix(line, "data:"):
			value := strings.TrimPrefix(line, "data:")
			value = strings.TrimPrefix(value, " ")
			dataLines = append(dataLines, value)
		}
	}

	if len(dataLines) > 0 {
		dispatch()
	}

	return scanner.Err()
}
