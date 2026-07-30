package conformance_test

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/fugue-labs/gollem/provider/anthropic"
	"github.com/fugue-labs/gollem/provider/conformance"
	"github.com/fugue-labs/gollem/provider/openai"
)

func TestDeterministicProviderDriverConformance(t *testing.T) {
	openAICancellationReady := make(chan struct{})
	openAIRetry := newRetryFixture()
	openAITimeout := newTimeoutFixture()
	openAIServer := httptest.NewServer(openAIConformanceFixture(t, openAICancellationReady, openAIRetry, openAITimeout))
	defer openAIServer.Close()
	anthropicCancellationReady := make(chan struct{})
	anthropicRetry := newRetryFixture()
	anthropicTimeout := newTimeoutFixture()
	anthropicServer := httptest.NewServer(anthropicConformanceFixture(t, anthropicCancellationReady, anthropicRetry, anthropicTimeout))
	defer anthropicServer.Close()

	cases := []struct {
		name  string
		model func() (conformance.Driver, error)
	}{
		{
			name: "native OpenAI",
			model: func() (conformance.Driver, error) {
				return conformance.Driver{
					Name:                "native OpenAI",
					Model:               openai.New(openai.WithAPIKey("test-openai-key"), openai.WithBaseURL(openAIServer.URL), openai.WithModel("gpt-4o")),
					Claims:              conformance.Claims{ToolCalls: true, Streaming: true, Usage: true, Cancellation: true, PartialStream: true, MalformedStream: true, Retryability: true, RequestTimeout: true},
					CancellationReady:   openAICancellationReady,
					RequestTimeoutReady: openAITimeout.readyFor("gpt-4o"),
					Expectations: conformance.Expectations{
						ResponseText: "openai response",
						ToolName:     "conformance_echo",
						StreamText:   "openai stream",
						PartialText:  "openai partial",
						RetryText:    "openai retry",
					},
				}, nil
			},
		},
		{
			name: "OpenAI-compatible local",
			model: func() (conformance.Driver, error) {
				model, err := openai.NewLocalEndpoint(openai.LocalEndpointConfig{
					BaseURL: openAIServer.URL,
					Model:   "gpt-5.2-codex",
					Token:   "test-local-key",
				})
				if err != nil {
					return conformance.Driver{}, err
				}
				return conformance.Driver{
					Name:                "OpenAI-compatible local",
					Model:               model,
					Claims:              conformance.Claims{ToolCalls: true, Streaming: true, Usage: true, Cancellation: true, PartialStream: true, MalformedStream: true, Retryability: true, RequestTimeout: true},
					CancellationReady:   openAICancellationReady,
					RequestTimeoutReady: openAITimeout.readyFor("gpt-5.2-codex"),
					Expectations: conformance.Expectations{
						ResponseText: "openai response",
						ToolName:     "conformance_echo",
						StreamText:   "openai stream",
						PartialText:  "openai partial",
						RetryText:    "openai retry",
					},
				}, nil
			},
		},
		{
			name: "native Anthropic",
			model: func() (conformance.Driver, error) {
				return conformance.Driver{
					Name:                "native Anthropic",
					Model:               anthropic.New(anthropic.WithAPIKey("test-anthropic-key"), anthropic.WithBaseURL(anthropicServer.URL), anthropic.WithModel(anthropic.ClaudeSonnet46)),
					Claims:              conformance.Claims{ToolCalls: true, Streaming: true, Usage: true, Cancellation: true, PartialStream: true, MalformedStream: true, Retryability: true, RequestTimeout: true},
					CancellationReady:   anthropicCancellationReady,
					RequestTimeoutReady: anthropicTimeout.readyFor(anthropic.ClaudeSonnet46),
					Expectations: conformance.Expectations{
						ResponseText: "anthropic response",
						ToolName:     "conformance_echo",
						StreamText:   "anthropic stream",
						PartialText:  "anthropic partial",
						RetryText:    "anthropic retry",
					},
				}, nil
			},
		},
	}

	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			driver, err := tt.model()
			if err != nil {
				t.Fatalf("new driver: %v", err)
			}
			if err := conformance.Verify(context.Background(), driver); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestVerifyRejectsUnprovenClaims(t *testing.T) {
	err := conformance.Verify(context.Background(), conformance.Driver{
		Name:   "missing tool fixture",
		Model:  openai.New(openai.WithAPIKey("test-key")),
		Claims: conformance.Claims{ToolCalls: true},
	})
	if err == nil {
		t.Fatal("Verify accepted a tool claim without an expected tool")
	}
	err = conformance.Verify(context.Background(), conformance.Driver{
		Name:   "missing cancellation fixture",
		Model:  openai.New(openai.WithAPIKey("test-key")),
		Claims: conformance.Claims{Cancellation: true},
	})
	if err == nil {
		t.Fatal("Verify accepted a cancellation claim without a start signal")
	}
	err = conformance.Verify(context.Background(), conformance.Driver{
		Name:   "missing partial stream fixture",
		Model:  openai.New(openai.WithAPIKey("test-key")),
		Claims: conformance.Claims{PartialStream: true},
	})
	if err == nil {
		t.Fatal("Verify accepted a partial stream claim without expected partial text")
	}
	err = conformance.Verify(context.Background(), conformance.Driver{
		Name:   "missing retry fixture",
		Model:  openai.New(openai.WithAPIKey("test-key")),
		Claims: conformance.Claims{Retryability: true},
	})
	if err == nil {
		t.Fatal("Verify accepted a retry claim without expected retry text")
	}
	err = conformance.Verify(context.Background(), conformance.Driver{
		Name:   "missing timeout fixture",
		Model:  openai.New(openai.WithAPIKey("test-key")),
		Claims: conformance.Claims{RequestTimeout: true},
	})
	if err == nil {
		t.Fatal("Verify accepted a timeout claim without a start signal")
	}
}

func openAIConformanceFixture(t *testing.T, cancellationReady chan<- struct{}, retry *retryFixture, timeout *timeoutFixture) http.Handler {
	t.Helper()
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/chat/completions" {
			t.Fatalf("OpenAI path = %q, want /v1/chat/completions", r.URL.Path)
		}
		if r.Header.Get("Authorization") == "" {
			t.Fatal("OpenAI fixture request had no authorization header")
		}
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Fatalf("read OpenAI request: %v", err)
		}
		var request struct {
			Model  string          `json:"model"`
			Stream bool            `json:"stream"`
			Tools  json.RawMessage `json:"tools"`
		}
		if err := json.Unmarshal(body, &request); err != nil {
			t.Fatalf("decode OpenAI request: %v", err)
		}
		if strings.Contains(string(body), "cancel conformance") {
			waitForCancellation(r, cancellationReady)
			return
		}
		if strings.Contains(string(body), "timeout conformance") {
			timeout.waitForCancellation(r.Context(), request.Model)
			return
		}
		if strings.Contains(string(body), "partial stream conformance") {
			w.Header().Set("Content-Type", "text/event-stream")
			_, _ = fmt.Fprint(w, "data: {\"id\":\"chatcmpl-partial\",\"object\":\"chat.completion.chunk\",\"model\":\"gpt-4o\",\"choices\":[{\"index\":0,\"delta\":{\"content\":\"openai partial\"},\"finish_reason\":null}]}\n\n")
			return
		}
		if strings.Contains(string(body), "malformed stream conformance") {
			w.Header().Set("Content-Type", "text/event-stream")
			_, _ = fmt.Fprint(w, "data: {\"fixture_sensitive\":\n\n")
			return
		}
		if strings.Contains(string(body), "retry conformance") {
			if retry.firstAttempt(request.Model) {
				w.Header().Set("Retry-After", "0")
				w.WriteHeader(http.StatusTooManyRequests)
				_, _ = fmt.Fprint(w, `{"error":{"type":"rate_limit_error"}}`)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			_, _ = fmt.Fprint(w, `{"id":"chatcmpl-retry","object":"chat.completion","model":"gpt-4o","choices":[{"message":{"role":"assistant","content":"openai retry"},"finish_reason":"stop"}],"usage":{"prompt_tokens":3,"completion_tokens":2}}`)
			return
		}
		if request.Stream {
			if len(request.Tools) != 0 && string(request.Tools) != "null" {
				t.Fatalf("stream request unexpectedly included tools: %s", request.Tools)
			}
			w.Header().Set("Content-Type", "text/event-stream")
			_, _ = fmt.Fprint(w, `data: {"id":"chatcmpl-conformance","object":"chat.completion.chunk","model":"gpt-4o","choices":[{"index":0,"delta":{"content":"openai stream"},"finish_reason":"stop"}],"usage":{"prompt_tokens":3,"completion_tokens":2}}

data: [DONE]

`)
			return
		}
		if len(request.Tools) == 0 || string(request.Tools) == "null" {
			t.Fatal("tool-capable request did not include tools")
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprint(w, `{"id":"chatcmpl-conformance","object":"chat.completion","model":"gpt-4o","choices":[{"message":{"role":"assistant","content":"openai response","tool_calls":[{"id":"call_openai","type":"function","function":{"name":"conformance_echo","arguments":"{\"value\":\"ok\"}"}}]},"finish_reason":"tool_calls"}],"usage":{"prompt_tokens":3,"completion_tokens":2}}`)
	})
}

func anthropicConformanceFixture(t *testing.T, cancellationReady chan<- struct{}, retry *retryFixture, timeout *timeoutFixture) http.Handler {
	t.Helper()
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/messages" {
			t.Fatalf("Anthropic path = %q, want /v1/messages", r.URL.Path)
		}
		if r.Header.Get("x-api-key") == "" {
			t.Fatal("Anthropic fixture request had no API key header")
		}
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Fatalf("read Anthropic request: %v", err)
		}
		var request struct {
			Model  string          `json:"model"`
			Stream bool            `json:"stream"`
			Tools  json.RawMessage `json:"tools"`
		}
		if err := json.Unmarshal(body, &request); err != nil {
			t.Fatalf("decode Anthropic request: %v", err)
		}
		if strings.Contains(string(body), "cancel conformance") {
			waitForCancellation(r, cancellationReady)
			return
		}
		if strings.Contains(string(body), "timeout conformance") {
			timeout.waitForCancellation(r.Context(), request.Model)
			return
		}
		if strings.Contains(string(body), "partial stream conformance") {
			w.Header().Set("Content-Type", "text/event-stream")
			_, _ = fmt.Fprint(w, "event: content_block_start\ndata: {\"type\":\"content_block_start\",\"index\":0,\"content_block\":{\"type\":\"text\",\"text\":\"\"}}\n\nevent: content_block_delta\ndata: {\"type\":\"content_block_delta\",\"index\":0,\"delta\":{\"type\":\"text_delta\",\"text\":\"anthropic partial\"}}\n\n")
			return
		}
		if strings.Contains(string(body), "malformed stream conformance") {
			w.Header().Set("Content-Type", "text/event-stream")
			_, _ = fmt.Fprint(w, "event: content_block_delta\ndata: {\"fixture_sensitive\":\n\n")
			return
		}
		if strings.Contains(string(body), "retry conformance") {
			if retry.firstAttempt(request.Model) {
				w.Header().Set("Retry-After", "0")
				w.WriteHeader(http.StatusTooManyRequests)
				_, _ = fmt.Fprint(w, `{"error":{"type":"rate_limit_error"}}`)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			_, _ = fmt.Fprint(w, `{"id":"msg-retry","role":"assistant","model":"claude-sonnet-4-6","content":[{"type":"text","text":"anthropic retry"}],"stop_reason":"end_turn","usage":{"input_tokens":3,"output_tokens":2}}`)
			return
		}
		if request.Stream {
			w.Header().Set("Content-Type", "text/event-stream")
			_, _ = fmt.Fprint(w, `event: message_start
data: {"type":"message_start","message":{"usage":{"input_tokens":3,"output_tokens":0}}}

event: content_block_start
data: {"type":"content_block_start","index":0,"content_block":{"type":"text","text":""}}

event: content_block_delta
data: {"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"anthropic stream"}}

event: content_block_stop
data: {"type":"content_block_stop","index":0}

event: message_delta
data: {"type":"message_delta","delta":{"stop_reason":"end_turn"},"usage":{"output_tokens":2}}

event: message_stop
data: {"type":"message_stop"}

`)
			return
		}
		if len(request.Tools) == 0 || string(request.Tools) == "null" {
			t.Fatal("tool-capable request did not include tools")
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprint(w, `{"id":"msg-conformance","role":"assistant","model":"claude-sonnet-4-6","content":[{"type":"text","text":"anthropic response"},{"type":"tool_use","id":"call_anthropic","name":"conformance_echo","input":{"value":"ok"}}],"stop_reason":"tool_use","usage":{"input_tokens":3,"output_tokens":2}}`)
	})
}

func waitForCancellation(request *http.Request, ready chan<- struct{}) {
	select {
	case ready <- struct{}{}:
	case <-request.Context().Done():
		return
	}
	<-request.Context().Done()
}

type retryFixture struct {
	mu       sync.Mutex
	attempts map[string]int
}

func newRetryFixture() *retryFixture {
	return &retryFixture{attempts: make(map[string]int)}
}

func (f *retryFixture) firstAttempt(model string) bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.attempts[model]++
	return f.attempts[model] == 1
}

type timeoutFixture struct {
	mu    sync.Mutex
	ready map[string]chan struct{}
}

func newTimeoutFixture() *timeoutFixture {
	return &timeoutFixture{ready: make(map[string]chan struct{})}
}

func (f *timeoutFixture) readyFor(model string) <-chan struct{} {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.channelForLocked(model)
}

func (f *timeoutFixture) waitForCancellation(ctx context.Context, model string) {
	f.markStarted(model)
	<-ctx.Done()
}

func (f *timeoutFixture) markStarted(model string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	ready := f.channelForLocked(model)
	select {
	case <-ready:
	default:
		close(ready)
	}
}

func (f *timeoutFixture) channelForLocked(model string) chan struct{} {
	ready := f.ready[model]
	if ready == nil {
		ready = make(chan struct{})
		f.ready[model] = ready
	}
	return ready
}
