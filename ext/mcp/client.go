// Package mcp provides a Model Context Protocol (MCP) client that discovers
// and invokes tools, prompts, and resources from MCP servers via JSON-RPC 2.0.
package mcp

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"sort"
	"sync"
)

// Client communicates with an MCP server over stdio using JSON-RPC 2.0.
type Client struct {
	*clientState

	cmd    *exec.Cmd
	stdin  io.WriteCloser
	stdout *bufio.Reader
	stderr io.ReadCloser

	writeMu   sync.Mutex
	closeOnce sync.Once
}

// StdioClientOption configures stdio MCP client startup.
type StdioClientOption func(*stdioClientConfig)

type stdioClientConfig struct {
	args []string
	env  map[string]string
}

// WithStdioArgs sets process arguments for the MCP server command.
func WithStdioArgs(args ...string) StdioClientOption {
	copied := append([]string(nil), args...)
	return func(cfg *stdioClientConfig) {
		cfg.args = append([]string(nil), copied...)
	}
}

// WithStdioEnv adds environment variables for the MCP server process.
func WithStdioEnv(env map[string]string) StdioClientOption {
	cloned := make(map[string]string, len(env))
	for k, v := range env {
		cloned[k] = v
	}
	return func(cfg *stdioClientConfig) {
		if cfg.env == nil {
			cfg.env = make(map[string]string, len(cloned))
		}
		for k, v := range cloned {
			cfg.env[k] = v
		}
	}
}

// NewStdioClient spawns an MCP server process and connects via stdio.
func NewStdioClient(ctx context.Context, command string, args ...string) (*Client, error) {
	return NewStdioClientWithOptions(ctx, command, WithStdioArgs(args...))
}

// NewStdioClientWithConfig spawns an MCP server process and connects via stdio.
func NewStdioClientWithConfig(ctx context.Context, command string, config ClientConfig, opts ...StdioClientOption) (*Client, error) {
	cfg := stdioClientConfig{}
	for _, opt := range opts {
		if opt != nil {
			opt(&cfg)
		}
	}

	cmd := exec.CommandContext(ctx, command, cfg.args...)
	if len(cfg.env) > 0 {
		env := append([]string(nil), os.Environ()...)
		keys := make([]string, 0, len(cfg.env))
		for k := range cfg.env {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			env = append(env, k+"="+cfg.env[k])
		}
		cmd.Env = env
	}

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
		clientState: newClientState(config),
		cmd:         cmd,
		stdin:       stdin,
		stdout:      bufio.NewReader(stdout),
		stderr:      stderr,
	}

	go c.readLoop()

	if err := c.initialize(ctx); err != nil {
		c.Close()
		return nil, fmt.Errorf("mcp: initialization failed: %w", err)
	}

	return c, nil
}

// NewStdioClientWithOptions spawns an MCP server process and connects via stdio.
func NewStdioClientWithOptions(ctx context.Context, command string, opts ...StdioClientOption) (*Client, error) {
	return NewStdioClientWithConfig(ctx, command, ClientConfig{}, opts...)
}

func (c *Client) initialize(ctx context.Context) error {
	return initializeClient(ctx, c.clientState, c.call, c.notify)
}

// call sends a JSON-RPC request and waits for a response.
func (c *Client) call(ctx context.Context, method string, params any) (json.RawMessage, error) {
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

	c.writeMu.Lock()
	_, err = fmt.Fprintf(c.stdin, "%s\n", data)
	c.writeMu.Unlock()
	if err != nil {
		c.removePending(id)
		return nil, fmt.Errorf("mcp: failed to write request: %w", err)
	}

	return c.awaitResponse(ctx, id, ch)
}

// notify sends a JSON-RPC notification (no response expected).
func (c *Client) notify(_ context.Context, method string, params any) error {
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

	return c.writeJSON(data)
}

func (c *Client) respond(_ context.Context, id any, result any, rpcErr *jsonRPCError) error {
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
	return c.writeJSON(data)
}

func (c *Client) writeJSON(data []byte) error {
	c.writeMu.Lock()
	defer c.writeMu.Unlock()
	_, err := fmt.Fprintf(c.stdin, "%s\n", data)
	return err
}

// readLoop reads JSON-RPC messages from stdout and dispatches them.
func (c *Client) readLoop() {
	for {
		line, err := c.stdout.ReadBytes('\n')
		if err != nil {
			c.shutdown()
			return
		}

		var msg jsonRPCMessage
		if err := json.Unmarshal(line, &msg); err != nil {
			continue
		}
		c.dispatchMessage(&msg, c.respond)
	}
}

// ListTools discovers available tools from the MCP server.
func (c *Client) ListTools(ctx context.Context) ([]Tool, error) {
	return listTools(ctx, c.call)
}

// CallTool invokes a tool on the MCP server.
func (c *Client) CallTool(ctx context.Context, name string, args map[string]any) (*ToolResult, error) {
	return callTool(ctx, c.call, name, args)
}

// ListResources lists resources exposed by the MCP server.
func (c *Client) ListResources(ctx context.Context) ([]Resource, error) {
	return listResources(ctx, c.call)
}

// ReadResource reads a resource from the MCP server.
func (c *Client) ReadResource(ctx context.Context, uri string) (*ReadResourceResult, error) {
	return readResource(ctx, c.call, uri)
}

// ListResourceTemplates lists URI templates exposed by the MCP server.
func (c *Client) ListResourceTemplates(ctx context.Context) ([]ResourceTemplate, error) {
	return listResourceTemplates(ctx, c.call)
}

// ListPrompts lists prompts exposed by the MCP server.
func (c *Client) ListPrompts(ctx context.Context) ([]Prompt, error) {
	return listPrompts(ctx, c.call)
}

// GetPrompt resolves a prompt from the MCP server.
func (c *Client) GetPrompt(ctx context.Context, name string, args map[string]string) (*PromptResult, error) {
	return getPrompt(ctx, c.call, name, args)
}

// NotifyRootsListChanged emits notifications/roots/list_changed to the server.
func (c *Client) NotifyRootsListChanged(ctx context.Context) error {
	return notifyRootsListChanged(ctx, c.notify)
}

// Close shuts down the MCP server process and releases resources.
func (c *Client) Close() error {
	var closeErr error
	c.closeOnce.Do(func() {
		c.shutdown()

		if c.stdin != nil {
			_ = c.stdin.Close()
		}
		if c.stderr != nil {
			_ = c.stderr.Close()
		}
		if c.cmd != nil {
			if c.cmd.Process != nil {
				_ = c.cmd.Process.Kill()
			}
			closeErr = c.cmd.Wait()
		}
	})
	return closeErr
}
