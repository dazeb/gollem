package mcp

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/fugue-labs/gollem/core"
)

// HTTPClient communicates with an MCP server over the streamable HTTP
// transport. The 2026-07-28 protocol is stateless: there is no session id, no
// initialize handshake, and no SSE resumability. POST carries a single
// JSON-RPC request (with the stateless _meta envelope) and returns the result
// inline as application/json. The one long-lived stream is subscriptions/listen,
// delivered as a POST whose response is text/event-stream (see Listen).
type HTTPClient struct {
	*clientState

	endpoint   string
	httpClient *http.Client
	headers    map[string]string

	closeOnce sync.Once
}

// NewHTTPClient connects to an MCP server over the streamable HTTP transport.
// No initialize handshake is performed; clients MAY call Discover to learn the
// server's identity and capabilities.
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

	return c, nil
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

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, c.endpoint, bytes.NewReader(data))
	if err != nil {
		c.removePending(id)
		return nil, fmt.Errorf("mcp: failed to create request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Accept", "application/json, text/event-stream")
	httpReq.Header.Set("Mcp-Method", method)
	if name := mcpNameForRequest(method, req.Params); name != "" {
		httpReq.Header.Set("Mcp-Name", name)
	}
	applyHeaders(httpReq, c.headers)
	applyProtocolVersionHeader(httpReq, c.ProtocolVersion())

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		c.removePending(id)
		return nil, fmt.Errorf("mcp: failed to send request: %w", err)
	}
	defer func() {
		if resp.Body != nil {
			resp.Body.Close()
		}
	}()

	if resp.StatusCode != http.StatusOK {
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
	}

	return c.awaitResponse(ctx, id, ch)
}

func (c *HTTPClient) respond(ctx context.Context, id any, result any, rpcErr *jsonRPCError) error {
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

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, c.endpoint, bytes.NewReader(data))
	if err != nil {
		return err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	applyHeaders(httpReq, c.headers)
	applyProtocolVersionHeader(httpReq, c.ProtocolVersion())

	httpResp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return fmt.Errorf("mcp: failed to send response: %w", err)
	}
	_, _ = io.Copy(io.Discard, httpResp.Body)
	httpResp.Body.Close()
	return nil
}

func (c *HTTPClient) handleEvent(_ string, data string) {
	var msg jsonRPCMessage
	if err := json.Unmarshal([]byte(data), &msg); err != nil {
		return
	}
	c.dispatchMessage(&msg, c.respond)
}

// mcpNameForRequest extracts the Mcp-Name header value for a request: the tool
// name for tools/call, the prompt name for prompts/get, and the resource uri
// for resources/read. Empty for methods without a name.
func mcpNameForRequest(method string, params json.RawMessage) string {
	if len(params) == 0 {
		return ""
	}
	var probe struct {
		Name string `json:"name"`
		URI  string `json:"uri"`
	}
	if err := json.Unmarshal(params, &probe); err != nil {
		return ""
	}
	switch method {
	case "tools/call", "prompts/get":
		return probe.Name
	case "resources/read":
		return probe.URI
	default:
		return ""
	}
}

// Discover calls server/discover and caches the server's identity, capabilities,
// and instructions on the client state.
func (c *HTTPClient) Discover(ctx context.Context) (*DiscoverResult, error) {
	return discover(ctx, c.call, c.clientState)
}

// ListTools discovers available tools from the MCP server.
func (c *HTTPClient) ListTools(ctx context.Context) ([]Tool, error) {
	return listTools(ctx, c.call, c.clientState)
}

// CallTool invokes a tool on the MCP server. tools/call is MRTR-eligible.
func (c *HTTPClient) CallTool(ctx context.Context, name string, args map[string]any) (*ToolResult, error) {
	return callTool(ctx, c.call, c.clientState, name, args)
}

// ListResources lists resources exposed by the MCP server.
func (c *HTTPClient) ListResources(ctx context.Context) ([]Resource, error) {
	return listResources(ctx, c.call, c.clientState)
}

// ReadResource reads a resource from the MCP server. resources/read is MRTR-eligible.
func (c *HTTPClient) ReadResource(ctx context.Context, uri string) (*ReadResourceResult, error) {
	return readResource(ctx, c.call, c.clientState, uri)
}

// ListResourceTemplates lists URI templates exposed by the MCP server.
func (c *HTTPClient) ListResourceTemplates(ctx context.Context) ([]ResourceTemplate, error) {
	return listResourceTemplates(ctx, c.call, c.clientState)
}

// ListPrompts lists prompts exposed by the MCP server.
func (c *HTTPClient) ListPrompts(ctx context.Context) ([]Prompt, error) {
	return listPrompts(ctx, c.call, c.clientState)
}

// GetPrompt resolves a prompt from the MCP server. prompts/get is MRTR-eligible.
func (c *HTTPClient) GetPrompt(ctx context.Context, name string, args map[string]string) (*PromptResult, error) {
	return getPrompt(ctx, c.call, c.clientState, name, args)
}

// Tools converts MCP tools into core.Tool instances that call back to the
// remote MCP server.
func (c *HTTPClient) Tools(ctx context.Context) ([]core.Tool, error) {
	return toolsForSource(ctx, c)
}

// OnNotification registers a handler for server notifications delivered over
// the subscriptions/listen stream. The HTTP client dispatches notifications
// received via Listen to these handlers in addition to the per-call handler
// passed to Listen. It is inherited from clientState.OnNotification.

// Listen opens a subscriptions/listen stream to the server. It POSTs the
// request with the stateless _meta envelope, reads the text/event-stream
// response, and dispatches each notification (including the
// notifications/subscriptions/acknowledged first message) to handler and to any
// handlers registered via OnNotification. Listen blocks until the stream closes
// (server graceful closure or transport drop) or ctx is cancelled. Cancelling
// ctx closes the HTTP request, which the server treats as a client disconnect.
func (c *HTTPClient) Listen(ctx context.Context, filter SubscriptionFilter, handler func(Notification)) error {
	params := SubscribeParams{Notifications: filter}
	paramsRaw, err := mergeRequestMeta(params, c.requestMeta())
	if err != nil {
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
		return err
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, c.endpoint, bytes.NewReader(data))
	if err != nil {
		return fmt.Errorf("mcp: failed to create subscriptions/listen request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Accept", "text/event-stream, application/json")
	httpReq.Header.Set("Mcp-Method", "subscriptions/listen")
	applyHeaders(httpReq, c.headers)
	applyProtocolVersionHeader(httpReq, c.ProtocolVersion())

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return fmt.Errorf("mcp: subscriptions/listen request failed: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		if len(bytes.TrimSpace(body)) > 0 {
			return fmt.Errorf("mcp: subscriptions/listen returned status %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
		}
		return fmt.Errorf("mcp: subscriptions/listen returned status %d", resp.StatusCode)
	}

	return readEventStream(resp.Body, func(eventType, data string) {
		var msg jsonRPCMessage
		if err := json.Unmarshal([]byte(data), &msg); err != nil {
			return
		}
		if msg.Method != "" {
			note := Notification{Method: msg.Method, Params: append(json.RawMessage(nil), msg.Params...)}
			if handler != nil {
				handler(note)
			}
			c.dispatchNotification(note)
			return
		}
		// A message with an id and no method is the graceful-closure response
		// to the original subscriptions/listen request; the stream is ending.
	})
}

// GetTask retrieves the current state of a task by id.
func (c *HTTPClient) GetTask(ctx context.Context, taskID string) (*Task, error) {
	return getTask(ctx, c.call, c.clientState, taskID)
}

// UpdateTask submits input responses for a task awaiting input (status
// input_required). Unknown or already-satisfied keys are ignored by the server.
func (c *HTTPClient) UpdateTask(ctx context.Context, taskID string, responses InputResponses) error {
	return updateTask(ctx, c.call, c.clientState, taskID, responses)
}

// CancelTask requests cooperative cancellation of a task. The task may still
// reach a non-cancelled terminal state depending on application progress.
func (c *HTTPClient) CancelTask(ctx context.Context, taskID string) error {
	return cancelTask(ctx, c.call, c.clientState, taskID)
}

// CallToolAwait invokes a tool and, if the server returns a task, polls
// tasks/get honoring pollIntervalMs until the task reaches a terminal state.
// On completed it unmarshals the task's Result into a ToolResult and returns
// it. On failed/cancelled it returns an error. On input_required it returns an
// *InputRequiredTaskError carrying the task so the caller can UpdateTask and
// retry. poll, if non-nil, is invoked for every observed non-terminal task
// state (useful for progress reporting). The client must have advertised the
// tasks extension (see WithTasksExtension) before using this method.
func (c *HTTPClient) CallToolAwait(ctx context.Context, name string, args map[string]any, poll func(*Task)) (*ToolResult, error) {
	raw, err := callToolRaw(ctx, c.call, c.clientState, name, args)
	if err != nil {
		return nil, fmt.Errorf("mcp: tools/call failed: %w", err)
	}
	rt := readResultType(raw)
	if rt == ResultTypeComplete {
		var toolResult ToolResult
		if err := json.Unmarshal(raw, &toolResult); err != nil {
			return nil, fmt.Errorf("mcp: failed to parse tool result: %w", err)
		}
		return &toolResult, nil
	}
	if rt != ResultTypeTask {
		return nil, fmt.Errorf("mcp: unexpected resultType %q for tools/call", rt)
	}
	var ctr CreateTaskResult
	if err := json.Unmarshal(raw, &ctr); err != nil {
		return nil, fmt.Errorf("mcp: failed to parse task result: %w", err)
	}
	task := ctr.Task
	for {
		if poll != nil {
			poll(&task)
		}
		switch task.Status {
		case TaskStatusCompleted:
			var toolResult ToolResult
			if len(task.Result) > 0 {
				if err := json.Unmarshal(task.Result, &toolResult); err != nil {
					return nil, fmt.Errorf("mcp: failed to parse completed task result: %w", err)
				}
			}
			return &toolResult, nil
		case TaskStatusFailed:
			if task.Error != nil {
				return nil, task.Error
			}
			return nil, fmt.Errorf("mcp: task %s failed", task.TaskID)
		case TaskStatusCancelled:
			return nil, fmt.Errorf("mcp: task %s cancelled", task.TaskID)
		case TaskStatusInputRequired:
			return nil, &InputRequiredTaskError{Task: task}
		}
		wait := taskPollInterval(task.PollIntervalMs)
		select {
		case <-time.After(wait):
		case <-ctx.Done():
			return nil, ctx.Err()
		}
		got, err := c.GetTask(ctx, task.TaskID)
		if err != nil {
			return nil, err
		}
		task = *got
	}
}

// InputRequiredTaskError is returned by CallToolAwait when a task reaches the
// input_required state. The caller inspects Task.InputRequests, calls
// UpdateTask with the responses, and polls GetTask (or retries CallToolAwait
// on a fresh call) to resume.
type InputRequiredTaskError struct {
	Task Task
}

func (e *InputRequiredTaskError) Error() string {
	return "mcp: task " + e.Task.TaskID + " requires input"
}

// taskPollInterval derives the poll delay from the server's pollIntervalMs
// hint, with a sane default when unset.
func taskPollInterval(hint *int64) time.Duration {
	if hint != nil && *hint > 0 {
		return time.Duration(*hint) * time.Millisecond
	}
	return 250 * time.Millisecond
}

// Close releases HTTP client state. The 2026-07-28 protocol has no session to
// tear down (no Mcp-Session-Id, no DELETE).
func (c *HTTPClient) Close() error {
	c.closeOnce.Do(func() {
		c.shutdown()
	})
	return nil
}
