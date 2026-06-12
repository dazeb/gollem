package mcp

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/fugue-labs/gollem/core"
)

func TestHTTPServerTransportSamplingRoundTrip(t *testing.T) {
	server := NewServer(WithServerInfo(ServerInfo{Name: "sleepy-test", Version: "0.1.0"}))
	server.AddTool(Tool{
		Name:        "ask_client",
		Description: "Ask the connected client to sample a response",
		InputSchema: mustRawJSON([]byte(`{"type":"object","properties":{"prompt":{"type":"string"}},"required":["prompt"]}`)),
	}, func(ctx context.Context, rc *RequestContext, params map[string]any) (*ToolResult, error) {
		prompt, _ := params["prompt"].(string)
		resp, err := rc.CreateMessage(ctx, &CreateMessageParams{
			Messages: []SamplingMessage{{
				Role:    "user",
				Content: MarshalSamplingContent(Content{Type: "text", Text: prompt}),
			}},
			MaxTokens: 32,
		})
		if err != nil {
			return nil, err
		}
		blocks, err := ParseSamplingContent(resp.Content)
		if err != nil {
			return nil, err
		}
		if len(blocks) == 0 {
			return textToolResult(""), nil
		}
		return textToolResult(blocks[0].Text), nil
	})

	httpServer := httptest.NewServer(NewHTTPServerTransport(server))
	defer httpServer.Close()

	clientModel := &recordingModel{
		requestFn: func(_ context.Context, messages []core.ModelMessage, settings *core.ModelSettings, _ *core.ModelRequestParameters) (*core.ModelResponse, error) {
			if len(messages) != 1 {
				t.Fatalf("unexpected nested sampling messages: %+v", messages)
			}
			req := messages[0].(core.ModelRequest)
			if got := req.Parts[0].(core.UserPromptPart).Content; got != "hello from transport" {
				t.Fatalf("unexpected prompt: %q", got)
			}
			if settings == nil || settings.MaxTokens == nil || *settings.MaxTokens != 32 {
				t.Fatalf("unexpected settings: %+v", settings)
			}
			return &core.ModelResponse{
				ModelName:    "client-model",
				FinishReason: core.FinishReasonStop,
				Parts: []core.ModelResponsePart{
					core.TextPart{Content: "client says hi"},
				},
			}, nil
		},
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	client, err := NewHTTPClientWithConfig(ctx, httpServer.URL, ClientConfig{
		SamplingHandler: ModelSamplingHandler(clientModel),
	})
	if err != nil {
		t.Fatalf("failed to create HTTP client: %v", err)
	}
	defer client.Close()

	tools, err := client.ListTools(ctx)
	if err != nil {
		t.Fatalf("ListTools failed: %v", err)
	}
	if len(tools) != 1 || tools[0].Name != "ask_client" {
		t.Fatalf("unexpected tools: %+v", tools)
	}

	result, err := client.CallTool(ctx, "ask_client", map[string]any{"prompt": "hello from transport"})
	if err != nil {
		t.Fatalf("CallTool failed: %v", err)
	}
	if got := result.TextContent(); got != "client says hi" {
		t.Fatalf("unexpected tool result: %q", got)
	}
}

func TestHTTPServerTransportRunClosesSessionsOnCancel(t *testing.T) {
	transport := NewHTTPServerTransport(NewServer())
	session := transport.newSession(httptest.NewRequest(http.MethodPost, "/", nil))

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		done <- transport.Run(ctx)
	}()

	cancel()

	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("Run returned %v, want %v", err, context.Canceled)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for transport.Run to return")
	}

	select {
	case <-session.ctx.Done():
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for session context to close")
	}

	transport.mu.Lock()
	sessionCount := len(transport.sessions)
	transport.mu.Unlock()
	if sessionCount != 0 {
		t.Fatalf("expected all sessions to be closed, found %d", sessionCount)
	}

	session.mu.Lock()
	closed := session.closed
	session.mu.Unlock()
	if !closed {
		t.Fatal("expected session to be marked closed")
	}
}

// TestHTTPServerTransportSessionContextFunc verifies the per-session
// context hook: identity derived from the initializing HTTP request
// (e.g. a workspace resolved from the Authorization header) reaches
// tool handlers through their ctx, for the whole life of the session.
func TestHTTPServerTransportSessionContextFunc(t *testing.T) {
	type wsKey struct{}

	server := NewServer(WithServerInfo(ServerInfo{Name: "ctx-test", Version: "0.1.0"}))
	server.AddTool(Tool{
		Name:        "whoami",
		Description: "Report the session identity",
		InputSchema: mustRawJSON([]byte(`{"type":"object"}`)),
	}, func(ctx context.Context, _ *RequestContext, _ map[string]any) (*ToolResult, error) {
		ws, _ := ctx.Value(wsKey{}).(string)
		return textToolResult(ws), nil
	})

	transport := NewHTTPServerTransport(server)
	transport.SetSessionContextFunc(func(r *http.Request) context.Context {
		return context.WithValue(context.Background(), wsKey{}, r.Header.Get("X-Test-Workspace"))
	})
	httpServer := httptest.NewServer(transport)
	defer httpServer.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	client, err := NewHTTPClientWithConfig(ctx, httpServer.URL, ClientConfig{},
		WithHeaders(map[string]string{"X-Test-Workspace": "ws-42"}))
	if err != nil {
		t.Fatalf("failed to create HTTP client: %v", err)
	}
	defer client.Close()

	result, err := client.CallTool(ctx, "whoami", map[string]any{})
	if err != nil {
		t.Fatalf("CallTool failed: %v", err)
	}
	if got := result.TextContent(); got != "ws-42" {
		t.Fatalf("session context value did not reach the tool handler: %q", got)
	}
}
