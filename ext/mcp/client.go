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

// NewStdioClient spawns an MCP server process and connects via stdio. The
// 2026-07-28 protocol is stateless: no initialize handshake is performed.
// Clients MAY call Discover to learn server identity and capabilities.
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

	return c, nil
}

// NewStdioClientWithOptions spawns an MCP server process and connects via stdio.
func NewStdioClientWithOptions(ctx context.Context, command string, opts ...StdioClientOption) (*Client, error) {
	return NewStdioClientWithConfig(ctx, command, ClientConfig{}, opts...)
}

// call sends a JSON-RPC request and waits for a response. params is expected to
// be a json.RawMessage already carrying the stateless _meta envelope (built via
// mergeRequestMeta by the shared roundTrip driver).
func (c *Client) call(ctx context.Context, method string, params any) (json.RawMessage, error) {
	id, ch, err := c.prepareCall()
	if err != nil {
		return nil, err
	}

	req := jsonRPCRequest{
		JSONRPC: "2.0",
		ID:      id,
		Method:  method,
	}
	if raw, ok := params.(json.RawMessage); ok && len(raw) > 0 {
		req.Params = raw
	} else if params != nil {
		data, mErr := json.Marshal(params)
		if mErr != nil {
			c.removePending(id)
			return nil, mErr
		}
		req.Params = data
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
		JSONRPC string          `json:"jsonrpc"`
		Method  string          `json:"method"`
		Params  json.RawMessage `json:"params,omitempty"`
	}{
		JSONRPC: "2.0",
		Method:  method,
	}
	if raw, ok := params.(json.RawMessage); ok {
		req.Params = raw
	} else if params != nil {
		data, err := json.Marshal(params)
		if err != nil {
			return err
		}
		req.Params = data
	}

	return c.writeJSON(mustRawJSON(marshalRequest(req)))
}

func marshalRequest(req any) []byte {
	data, err := json.Marshal(req)
	if err != nil {
		return nil
	}
	return data
}

func (c *Client) respond(_ context.Context, id any, result any, rpcErr *jsonRPCError) error {
	resp := jsonRPCResponse{
		JSONRPC: "2.0",
		ID:      id,
		Error:   rpcErr,
	}
	if raw, ok := result.(json.RawMessage); ok {
		resp.Result = raw
	} else if result != nil {
		data, err := json.Marshal(result)
		if err != nil {
			return err
		}
		resp.Result = data
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

// Discover calls server/discover and caches the server's identity, capabilities,
// and instructions on the client state. It is optional; clients MAY call it
// before any other request to select a protocol version up front.
func (c *Client) Discover(ctx context.Context) (*DiscoverResult, error) {
	return discover(ctx, c.call, c.clientState)
}

// ListTools discovers available tools from the MCP server.
func (c *Client) ListTools(ctx context.Context) ([]Tool, error) {
	return listTools(ctx, c.call, c.clientState)
}

// CallTool invokes a tool on the MCP server. tools/call is MRTR-eligible: if the
// server returns input_required, the client fulfills the input requests and
// retries automatically.
func (c *Client) CallTool(ctx context.Context, name string, args map[string]any) (*ToolResult, error) {
	return callTool(ctx, c.call, c.clientState, name, args)
}

// ListResources lists resources exposed by the MCP server.
func (c *Client) ListResources(ctx context.Context) ([]Resource, error) {
	return listResources(ctx, c.call, c.clientState)
}

// ReadResource reads a resource from the MCP server. resources/read is
// MRTR-eligible.
func (c *Client) ReadResource(ctx context.Context, uri string) (*ReadResourceResult, error) {
	return readResource(ctx, c.call, c.clientState, uri)
}

// ListResourceTemplates lists URI templates exposed by the MCP server.
func (c *Client) ListResourceTemplates(ctx context.Context) ([]ResourceTemplate, error) {
	return listResourceTemplates(ctx, c.call, c.clientState)
}

// ListPrompts lists prompts exposed by the MCP server.
func (c *Client) ListPrompts(ctx context.Context) ([]Prompt, error) {
	return listPrompts(ctx, c.call, c.clientState)
}

// GetPrompt resolves a prompt from the MCP server. prompts/get is MRTR-eligible.
func (c *Client) GetPrompt(ctx context.Context, name string, args map[string]string) (*PromptResult, error) {
	return getPrompt(ctx, c.call, c.clientState, name, args)
}

// Listen opens a subscriptions/listen stream over the shared stdio channel. It
// registers handler for every server notification, sends subscriptions/listen
// (the server replies with notifications/subscriptions/acknowledged and then
// fans out notifications on the same channel), and blocks until ctx is
// cancelled. On cancellation it unregisters the handler and sends
// notifications/cancelled referencing the listen request id so the server
// gracefully closes the subscription. Notifications are also delivered to any
// handlers registered via OnNotification.
func (c *Client) Listen(ctx context.Context, filter SubscriptionFilter, handler func(Notification)) error {
	unregister := c.OnNotification("", handler)

	params := SubscribeParams{Notifications: filter}
	paramsRaw, err := mergeRequestMeta(params, c.requestMeta())
	if err != nil {
		unregister()
		return fmt.Errorf("mcp: failed to build subscriptions/listen params: %w", err)
	}
	id := c.nextID.Add(1)
	req := jsonRPCRequest{
		JSONRPC: "2.0",
		ID:      id,
		Method:  "subscriptions/listen",
		Params:  paramsRaw,
	}
	data, err := json.Marshal(req)
	if err != nil {
		unregister()
		return err
	}
	c.writeMu.Lock()
	_, werr := fmt.Fprintf(c.stdin, "%s\n", data)
	c.writeMu.Unlock()
	if werr != nil {
		unregister()
		return fmt.Errorf("mcp: failed to send subscriptions/listen: %w", werr)
	}

	<-ctx.Done()
	unregister()

	// Cooperatively cancel the subscription so the server can close it and
	// reclaim the hub entry.
	idJSON, _ := json.Marshal(id)
	cancelParams := map[string]any{"requestId": json.RawMessage(idJSON)}
	_ = c.notify(context.Background(), "notifications/cancelled", cancelParams)
	return ctx.Err()
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
