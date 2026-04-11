package mcp

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/fugue-labs/gollem/core"
)

// HTTPClient communicates with an MCP server over the streamable HTTP transport.
// It supports direct JSON responses, SSE responses to POST requests, and an
// optional background SSE stream for notifications and asynchronous responses.
type HTTPClient struct {
	*clientState

	endpoint   string
	httpClient *http.Client
	headers    map[string]string

	streamMu        sync.Mutex
	cancelStream    context.CancelFunc
	streamConnected bool
	sessionID       string

	closeOnce sync.Once
}

// NewHTTPClient connects to an MCP server over the streamable HTTP transport.
func NewHTTPClient(ctx context.Context, url string, opts ...HTTPClientOption) (*HTTPClient, error) {
	return NewHTTPClientWithConfig(ctx, url, ClientConfig{}, opts...)
}

// NewHTTPClientWithConfig connects to an MCP server over the streamable HTTP transport.
func NewHTTPClientWithConfig(ctx context.Context, url string, config ClientConfig, opts ...HTTPClientOption) (*HTTPClient, error) {
	cfg := defaultRemoteConfig()
	for _, opt := range opts {
		if opt != nil {
			opt(&cfg)
		}
	}

	c := &HTTPClient{
		clientState: newClientState(config),
		endpoint:    url,
		httpClient:  cfg.httpClient,
		headers:     cloneStringMap(cfg.headers),
	}

	if err := c.initialize(ctx); err != nil {
		c.Close()
		return nil, fmt.Errorf("mcp: initialization failed: %w", err)
	}

	c.ensureNotificationStream()
	return c, nil
}

func (c *HTTPClient) initialize(ctx context.Context) error {
	return initializeClient(ctx, c.clientState, c.call, c.notify)
}

func (c *HTTPClient) call(ctx context.Context, method string, params any) (json.RawMessage, error) {
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

	data, err := json.Marshal(req)
	if err != nil {
		c.removePending(id)
		return nil, err
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, c.endpoint, bytes.NewReader(data))
	if err != nil {
		c.removePending(id)
		return nil, fmt.Errorf("mcp: failed to create request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Accept", "application/json, text/event-stream")
	applyHeaders(httpReq, c.headers)
	applyProtocolVersionHeader(httpReq, c.ProtocolVersion())
	if sessionID := c.getSessionID(); sessionID != "" {
		httpReq.Header.Set("Mcp-Session-Id", sessionID)
	}

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		c.removePending(id)
		return nil, fmt.Errorf("mcp: failed to send request: %w", err)
	}
	c.captureSessionID(resp)
	defer func() {
		if resp.Body != nil {
			resp.Body.Close()
		}
	}()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusAccepted {
		c.removePending(id)
		respBody, _ := io.ReadAll(resp.Body)
		if len(respBody) > 0 {
			return nil, fmt.Errorf("mcp: server returned status %d: %s", resp.StatusCode, strings.TrimSpace(string(respBody)))
		}
		return nil, fmt.Errorf("mcp: server returned status %d", resp.StatusCode)
	}

	contentType := resp.Header.Get("Content-Type")
	switch {
	case strings.Contains(contentType, "text/event-stream"):
		stream := resp.Body
		resp.Body = nil
		go func(callID int64, stream io.ReadCloser) {
			_ = readEventStream(stream, c.handleEvent)
			c.failPendingCall(callID)
		}(id, stream)
	case strings.Contains(contentType, "application/json"):
		respBody, readErr := io.ReadAll(resp.Body)
		if readErr != nil {
			c.removePending(id)
			return nil, fmt.Errorf("mcp: failed to read response body: %w", readErr)
		}
		if len(bytes.TrimSpace(respBody)) > 0 {
			var msg jsonRPCMessage
			if err := json.Unmarshal(respBody, &msg); err != nil {
				c.removePending(id)
				return nil, fmt.Errorf("mcp: failed to decode response: %w", err)
			}
			c.dispatchMessage(&msg, c.respond)
		}
	default:
		_, _ = io.Copy(io.Discard, resp.Body)
		if resp.StatusCode == http.StatusAccepted {
			c.ensureNotificationStream()
			if !c.waitForStream(ctx, 250*time.Millisecond) {
				c.removePending(id)
				return nil, errors.New("mcp: HTTP request accepted but no event stream is connected")
			}
		}
	}

	return c.awaitResponse(ctx, id, ch)
}

func (c *HTTPClient) notify(ctx context.Context, method string, params any) error {
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

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, c.endpoint, bytes.NewReader(data))
	if err != nil {
		return err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Accept", "application/json, text/event-stream")
	applyHeaders(httpReq, c.headers)
	applyProtocolVersionHeader(httpReq, c.ProtocolVersion())
	if sessionID := c.getSessionID(); sessionID != "" {
		httpReq.Header.Set("Mcp-Session-Id", sessionID)
	}

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return fmt.Errorf("mcp: failed to send notification: %w", err)
	}
	c.captureSessionID(resp)
	_, _ = io.Copy(io.Discard, resp.Body)
	resp.Body.Close()
	return nil
}

func (c *HTTPClient) respond(ctx context.Context, id any, result any, rpcErr *jsonRPCError) error {
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

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, c.endpoint, bytes.NewReader(data))
	if err != nil {
		return err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Accept", "application/json, text/event-stream")
	applyHeaders(httpReq, c.headers)
	applyProtocolVersionHeader(httpReq, c.ProtocolVersion())
	if sessionID := c.getSessionID(); sessionID != "" {
		httpReq.Header.Set("Mcp-Session-Id", sessionID)
	}

	httpResp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return fmt.Errorf("mcp: failed to send response: %w", err)
	}
	c.captureSessionID(httpResp)
	_, _ = io.Copy(io.Discard, httpResp.Body)
	httpResp.Body.Close()
	return nil
}

// ListTools discovers available tools from the MCP server.
func (c *HTTPClient) ListTools(ctx context.Context) ([]Tool, error) {
	return listTools(ctx, c.call)
}

// CallTool invokes a tool on the MCP server.
func (c *HTTPClient) CallTool(ctx context.Context, name string, args map[string]any) (*ToolResult, error) {
	return callTool(ctx, c.call, name, args)
}

// ListResources lists resources exposed by the MCP server.
func (c *HTTPClient) ListResources(ctx context.Context) ([]Resource, error) {
	return listResources(ctx, c.call)
}

// ReadResource reads a resource from the MCP server.
func (c *HTTPClient) ReadResource(ctx context.Context, uri string) (*ReadResourceResult, error) {
	return readResource(ctx, c.call, uri)
}

// ListResourceTemplates lists URI templates exposed by the MCP server.
func (c *HTTPClient) ListResourceTemplates(ctx context.Context) ([]ResourceTemplate, error) {
	return listResourceTemplates(ctx, c.call)
}

// ListPrompts lists prompts exposed by the MCP server.
func (c *HTTPClient) ListPrompts(ctx context.Context) ([]Prompt, error) {
	return listPrompts(ctx, c.call)
}

// GetPrompt resolves a prompt from the MCP server.
func (c *HTTPClient) GetPrompt(ctx context.Context, name string, args map[string]string) (*PromptResult, error) {
	return getPrompt(ctx, c.call, name, args)
}

// NotifyRootsListChanged emits notifications/roots/list_changed to the server.
func (c *HTTPClient) NotifyRootsListChanged(ctx context.Context) error {
	return notifyRootsListChanged(ctx, c.notify)
}

// Tools converts MCP tools into core.Tool instances that call back to the
// remote MCP server.
func (c *HTTPClient) Tools(ctx context.Context) ([]core.Tool, error) {
	return toolsForSource(ctx, c)
}

func (c *HTTPClient) ensureNotificationStream() {
	c.streamMu.Lock()
	if c.cancelStream != nil {
		c.streamMu.Unlock()
		return
	}
	streamCtx, cancel := context.WithCancel(context.Background())
	c.cancelStream = cancel
	c.streamMu.Unlock()

	go c.readNotificationStream(streamCtx)
}

func (c *HTTPClient) readNotificationStream(ctx context.Context) {
	defer func() {
		c.streamMu.Lock()
		c.streamConnected = false
		c.cancelStream = nil
		c.streamMu.Unlock()
	}()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.endpoint, nil)
	if err != nil {
		return
	}
	req.Header.Set("Accept", "text/event-stream")
	applyHeaders(req, c.headers)
	applyProtocolVersionHeader(req, c.ProtocolVersion())
	if sessionID := c.getSessionID(); sessionID != "" {
		req.Header.Set("Mcp-Session-Id", sessionID)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return
	}
	c.captureSessionID(resp)
	if resp.StatusCode != http.StatusOK || !strings.Contains(resp.Header.Get("Content-Type"), "text/event-stream") {
		_, _ = io.Copy(io.Discard, resp.Body)
		resp.Body.Close()
		return
	}

	c.streamMu.Lock()
	c.streamConnected = true
	c.streamMu.Unlock()

	if err := readEventStream(resp.Body, c.handleEvent); err != nil && ctx.Err() == nil {
		c.failPending()
		return
	}
	if ctx.Err() == nil {
		c.failPending()
	}
}

func (c *HTTPClient) handleEvent(_ string, data string) {
	var msg jsonRPCMessage
	if err := json.Unmarshal([]byte(data), &msg); err != nil {
		return
	}
	c.dispatchMessage(&msg, c.respond)
}

func (c *HTTPClient) captureSessionID(resp *http.Response) {
	if resp == nil {
		return
	}
	sessionID := resp.Header.Get("Mcp-Session-Id")
	if sessionID == "" {
		return
	}
	c.streamMu.Lock()
	c.sessionID = sessionID
	c.streamMu.Unlock()
}

func (c *HTTPClient) getSessionID() string {
	c.streamMu.Lock()
	defer c.streamMu.Unlock()
	return c.sessionID
}

func (c *HTTPClient) isStreamConnected() bool {
	c.streamMu.Lock()
	defer c.streamMu.Unlock()
	return c.streamConnected
}

func (c *HTTPClient) waitForStream(ctx context.Context, timeout time.Duration) bool {
	if c.isStreamConnected() {
		return true
	}

	timer := time.NewTimer(timeout)
	defer timer.Stop()
	ticker := time.NewTicker(10 * time.Millisecond)
	defer ticker.Stop()

	for {
		if c.isStreamConnected() {
			return true
		}
		select {
		case <-ctx.Done():
			return false
		case <-timer.C:
			return c.isStreamConnected()
		case <-ticker.C:
		}
	}
}

// Close shuts down background stream state and best-effort closes the MCP session.
func (c *HTTPClient) Close() error {
	c.closeOnce.Do(func() {
		c.shutdown()

		c.streamMu.Lock()
		cancel := c.cancelStream
		sessionID := c.sessionID
		c.streamMu.Unlock()

		if cancel != nil {
			cancel()
		}

		if sessionID != "" {
			req, err := http.NewRequestWithContext(context.Background(), http.MethodDelete, c.endpoint, nil)
			if err == nil {
				applyHeaders(req, c.headers)
				req.Header.Set("Mcp-Session-Id", sessionID)
				resp, err := c.httpClient.Do(req)
				if err == nil {
					_, _ = io.Copy(io.Discard, resp.Body)
					resp.Body.Close()
				}
			}
		}
	})
	return nil
}
