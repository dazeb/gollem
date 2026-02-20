// Package mcp provides a Model Context Protocol (MCP) client that discovers
// and invokes tools from MCP servers via the stdio transport using JSON-RPC 2.0.
package mcp

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os/exec"
	"sync"
	"sync/atomic"
)

// Client communicates with an MCP server over stdio using JSON-RPC 2.0.
type Client struct {
	cmd    *exec.Cmd
	stdin  io.WriteCloser
	stdout *bufio.Reader
	stderr io.ReadCloser

	mu        sync.Mutex
	nextID    atomic.Int64
	pending   map[int64]chan *jsonRPCResponse
	closed    bool
	closeOnce sync.Once
}

// jsonRPCRequest is a JSON-RPC 2.0 request.
type jsonRPCRequest struct {
	JSONRPC string `json:"jsonrpc"`
	ID      int64  `json:"id"`
	Method  string `json:"method"`
	Params  any    `json:"params,omitempty"`
}

// jsonRPCResponse is a JSON-RPC 2.0 response.
type jsonRPCResponse struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      int64           `json:"id"`
	Result  json.RawMessage `json:"result,omitempty"`
	Error   *jsonRPCError   `json:"error,omitempty"`
}

// jsonRPCError is a JSON-RPC 2.0 error.
type jsonRPCError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

func (e *jsonRPCError) Error() string {
	return fmt.Sprintf("JSON-RPC error %d: %s", e.Code, e.Message)
}

// NewStdioClient spawns an MCP server process and connects via stdio.
func NewStdioClient(ctx context.Context, command string, args ...string) (*Client, error) {
	cmd := exec.CommandContext(ctx, command, args...)

	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, fmt.Errorf("mcp: failed to create stdin pipe: %w", err)
	}

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, fmt.Errorf("mcp: failed to create stdout pipe: %w", err)
	}

	stderr, err := cmd.StderrPipe()
	if err != nil {
		return nil, fmt.Errorf("mcp: failed to create stderr pipe: %w", err)
	}

	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("mcp: failed to start server: %w", err)
	}

	c := &Client{
		cmd:     cmd,
		stdin:   stdin,
		stdout:  bufio.NewReader(stdout),
		stderr:  stderr,
		pending: make(map[int64]chan *jsonRPCResponse),
	}

	// Start reading responses in background.
	go c.readLoop()

	// Perform initialization handshake.
	if err := c.initialize(ctx); err != nil {
		c.Close()
		return nil, fmt.Errorf("mcp: initialization failed: %w", err)
	}

	return c, nil
}

// initialize performs the MCP initialization handshake.
func (c *Client) initialize(ctx context.Context) error {
	params := map[string]any{
		"protocolVersion": "2024-11-05",
		"capabilities":    map[string]any{},
		"clientInfo": map[string]any{
			"name":    "gollem",
			"version": "1.0.0",
		},
	}

	_, err := c.call(ctx, "initialize", params)
	if err != nil {
		return err
	}

	// Send initialized notification (no response expected).
	return c.notify("notifications/initialized", nil)
}

// call sends a JSON-RPC request and waits for a response.
func (c *Client) call(ctx context.Context, method string, params any) (json.RawMessage, error) {
	id := c.nextID.Add(1)

	req := jsonRPCRequest{
		JSONRPC: "2.0",
		ID:      id,
		Method:  method,
		Params:  params,
	}

	ch := make(chan *jsonRPCResponse, 1)

	c.mu.Lock()
	if c.closed {
		c.mu.Unlock()
		return nil, errors.New("mcp: client is closed")
	}
	c.pending[id] = ch
	c.mu.Unlock()

	data, err := json.Marshal(req)
	if err != nil {
		return nil, err
	}

	c.mu.Lock()
	_, err = fmt.Fprintf(c.stdin, "%s\n", data)
	c.mu.Unlock()
	if err != nil {
		return nil, fmt.Errorf("mcp: failed to write request: %w", err)
	}

	select {
	case resp := <-ch:
		if resp.Error != nil {
			return nil, resp.Error
		}
		return resp.Result, nil
	case <-ctx.Done():
		c.mu.Lock()
		delete(c.pending, id)
		c.mu.Unlock()
		return nil, ctx.Err()
	}
}

// notify sends a JSON-RPC notification (no response expected).
func (c *Client) notify(method string, params any) error {
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

	c.mu.Lock()
	defer c.mu.Unlock()
	_, err = fmt.Fprintf(c.stdin, "%s\n", data)
	return err
}

// readLoop reads JSON-RPC responses from stdout and dispatches them.
func (c *Client) readLoop() {
	for {
		line, err := c.stdout.ReadBytes('\n')
		if err != nil {
			c.mu.Lock()
			c.closed = true
			// Wake up all pending callers.
			for _, ch := range c.pending {
				close(ch)
			}
			c.pending = make(map[int64]chan *jsonRPCResponse)
			c.mu.Unlock()
			return
		}

		var resp jsonRPCResponse
		if err := json.Unmarshal(line, &resp); err != nil {
			continue
		}

		// Skip notifications (no ID).
		if resp.ID == 0 && resp.Result == nil && resp.Error == nil {
			continue
		}

		c.mu.Lock()
		ch, ok := c.pending[resp.ID]
		if ok {
			delete(c.pending, resp.ID)
		}
		c.mu.Unlock()

		if ok {
			ch <- &resp
		}
	}
}

// ListTools discovers available tools from the MCP server.
func (c *Client) ListTools(ctx context.Context) ([]Tool, error) {
	result, err := c.call(ctx, "tools/list", nil)
	if err != nil {
		return nil, fmt.Errorf("mcp: tools/list failed: %w", err)
	}

	var listResult struct {
		Tools []Tool `json:"tools"`
	}
	if err := json.Unmarshal(result, &listResult); err != nil {
		return nil, fmt.Errorf("mcp: failed to parse tools list: %w", err)
	}

	return listResult.Tools, nil
}

// CallTool invokes a tool on the MCP server.
func (c *Client) CallTool(ctx context.Context, name string, args map[string]any) (*ToolResult, error) {
	params := map[string]any{
		"name":      name,
		"arguments": args,
	}

	result, err := c.call(ctx, "tools/call", params)
	if err != nil {
		return nil, fmt.Errorf("mcp: tools/call failed: %w", err)
	}

	var toolResult ToolResult
	if err := json.Unmarshal(result, &toolResult); err != nil {
		return nil, fmt.Errorf("mcp: failed to parse tool result: %w", err)
	}

	return &toolResult, nil
}

// Close shuts down the MCP server process and releases resources.
func (c *Client) Close() error {
	var closeErr error
	c.closeOnce.Do(func() {
		c.mu.Lock()
		c.closed = true
		c.mu.Unlock()

		c.stdin.Close()
		if c.cmd.Process != nil {
			_ = c.cmd.Process.Kill()
		}
		closeErr = c.cmd.Wait()
	})
	return closeErr
}
