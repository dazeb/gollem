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
					ReasoningModel:      openai.New(openai.WithAPIKey("test-openai-key"), openai.WithBaseURL(openAIServer.URL), openai.WithModel("gpt-5")),
					Claims:              conformance.Claims{ToolCalls: true, Streaming: true, Usage: true, Cancellation: true, PartialStream: true, MalformedStream: true, DisconnectStream: true, Retryability: true, RequestTimeout: true, StreamTimeout: true, ReasoningVisibility: true},
					CancellationReady:   openAICancellationReady,
					RequestTimeoutReady: openAITimeout.readyFor("gpt-4o"),
					Expectations: conformance.Expectations{
						ResponseText:      "openai response",
						ToolName:          "conformance_echo",
						StreamText:        "openai stream",
						PartialText:       "openai partial",
						DisconnectText:    "openai disconnect",
						RetryText:         "openai retry",
						StreamTimeoutText: "openai deadline",
						ReasoningText:     "openai reasoning",
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
					Claims:              conformance.Claims{ToolCalls: true, Streaming: true, Usage: true, Cancellation: true, PartialStream: true, MalformedStream: true, DisconnectStream: true, Retryability: true, RequestTimeout: true, StreamTimeout: true},
					CancellationReady:   openAICancellationReady,
					RequestTimeoutReady: openAITimeout.readyFor("gpt-5.2-codex"),
					Expectations: conformance.Expectations{
						ResponseText:      "openai response",
						ToolName:          "conformance_echo",
						StreamText:        "openai stream",
						PartialText:       "openai partial",
						DisconnectText:    "openai disconnect",
						RetryText:         "openai retry",
						StreamTimeoutText: "openai deadline",
					},
				}, nil
			},
		},
		{
			name: "native Anthropic",
			model: func() (conformance.Driver, error) {
				model := anthropic.New(anthropic.WithAPIKey("test-anthropic-key"), anthropic.WithBaseURL(anthropicServer.URL), anthropic.WithModel(anthropic.ClaudeSonnet46))
				return conformance.Driver{
					Name:                "native Anthropic",
					Model:               model,
					ReasoningModel:      model,
					Claims:              conformance.Claims{ToolCalls: true, Streaming: true, Usage: true, Cancellation: true, PartialStream: true, MalformedStream: true, DisconnectStream: true, Retryability: true, RequestTimeout: true, StreamTimeout: true, ReasoningVisibility: true},
					CancellationReady:   anthropicCancellationReady,
					RequestTimeoutReady: anthropicTimeout.readyFor(anthropic.ClaudeSonnet46),
					Expectations: conformance.Expectations{
						ResponseText:      "anthropic response",
						ToolName:          "conformance_echo",
						StreamText:        "anthropic stream",
						PartialText:       "anthropic partial",
						DisconnectText:    "anthropic disconnect",
						RetryText:         "anthropic retry",
						StreamTimeoutText: "anthropic deadline",
						ReasoningText:     "anthropic reasoning",
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
		Name:   "missing disconnect stream fixture",
		Model:  openai.New(openai.WithAPIKey("test-key")),
		Claims: conformance.Claims{DisconnectStream: true},
	})
	if err == nil {
		t.Fatal("Verify accepted a disconnect stream claim without expected partial text")
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
	err = conformance.Verify(context.Background(), conformance.Driver{
		Name:   "missing stream timeout fixture",
		Model:  openai.New(openai.WithAPIKey("test-key")),
		Claims: conformance.Claims{StreamTimeout: true},
	})
	if err == nil {
		t.Fatal("Verify accepted a stream timeout claim without expected partial text")
	}
	err = conformance.Verify(context.Background(), conformance.Driver{
		Name:   "missing reasoning model",
		Model:  openai.New(openai.WithAPIKey("test-key")),
		Claims: conformance.Claims{ReasoningVisibility: true},
	})
	if err == nil {
		t.Fatal("Verify accepted a reasoning claim without a reasoning model")
	}
	err = conformance.Verify(context.Background(), conformance.Driver{
		Name:           "missing reasoning fixture",
		Model:          openai.New(openai.WithAPIKey("test-key")),
		ReasoningModel: openai.New(openai.WithAPIKey("test-key")),
		Claims:         conformance.Claims{ReasoningVisibility: true},
	})
	if err == nil {
		t.Fatal("Verify accepted a reasoning claim without expected reasoning text")
	}
}

func openAIConformanceFixture(t *testing.T, cancellationReady chan<- struct{}, retry *retryFixture, timeout *timeoutFixture) http.Handler {
	t.Helper()
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/v1/responses" {
			openAIReasoningConformanceFixture(t, w, r)
			return
		}
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
		if strings.Contains(string(body), "stream timeout conformance") {
			writeDeadlineBoundSSE(w, r, `data: {"id":"chatcmpl-deadline","object":"chat.completion.chunk","model":"gpt-4o","choices":[{"index":0,"delta":{"content":"openai deadline"},"finish_reason":null}]}

`)
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
		if strings.Contains(string(body), "disconnect stream conformance") {
			writeTruncatedSSE(w, `data: {"id":"chatcmpl-disconnect","object":"chat.completion.chunk","model":"gpt-4o","choices":[{"index":0,"delta":{"content":"openai disconnect"},"finish_reason":null}]}

`)
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

func openAIReasoningConformanceFixture(t *testing.T, w http.ResponseWriter, r *http.Request) {
	t.Helper()
	if r.Header.Get("Authorization") == "" {
		t.Fatal("OpenAI reasoning fixture request had no authorization header")
	}
	body, err := io.ReadAll(r.Body)
	if err != nil {
		t.Fatalf("read OpenAI reasoning request: %v", err)
	}
	var request struct {
		Model  string `json:"model"`
		Stream bool   `json:"stream"`
	}
	if err := json.Unmarshal(body, &request); err != nil {
		t.Fatalf("decode OpenAI reasoning request: %v", err)
	}
	if request.Model != "gpt-5" || !request.Stream || !strings.Contains(string(body), "reasoning conformance") {
		t.Fatalf("unexpected OpenAI reasoning request: model=%q stream=%t body=%s", request.Model, request.Stream, body)
	}
	w.Header().Set("Content-Type", "text/event-stream")
	_, _ = fmt.Fprint(w, `data: {"type":"response.output_item.added","output_index":0,"item":{"type":"reasoning","summary":[]}}

data: {"type":"response.reasoning_summary_text.delta","output_index":0,"delta":"openai reasoning"}

data: {"type":"response.output_item.done","output_index":0,"item":{"type":"reasoning","summary":[{"type":"summary_text","text":"openai reasoning"}]}}

data: {"type":"response.output_text.delta","delta":"openai reasoning answer"}

data: {"type":"response.completed","response":{"id":"resp-reasoning","model":"gpt-5","output":[{"type":"reasoning","summary":[{"type":"summary_text","text":"openai reasoning"}]},{"type":"message","role":"assistant","content":[{"type":"output_text","text":"openai reasoning answer"}]}],"usage":{"input_tokens":3,"output_tokens":2}}}

data: [DONE]

`)
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
		if strings.Contains(string(body), "stream timeout conformance") {
			writeDeadlineBoundSSE(w, r, `event: content_block_start
data: {"type":"content_block_start","index":0,"content_block":{"type":"text","text":""}}

event: content_block_delta
data: {"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"anthropic deadline"}}

`)
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
		if strings.Contains(string(body), "disconnect stream conformance") {
			writeTruncatedSSE(w, `event: content_block_start
data: {"type":"content_block_start","index":0,"content_block":{"type":"text","text":""}}

event: content_block_delta
data: {"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"anthropic disconnect"}}

`)
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
		if strings.Contains(string(body), "reasoning conformance") {
			if !request.Stream {
				t.Fatal("Anthropic reasoning request was not streaming")
			}
			w.Header().Set("Content-Type", "text/event-stream")
			_, _ = fmt.Fprint(w, `event: message_start
data: {"type":"message_start","message":{"usage":{"input_tokens":3,"output_tokens":0}}}

event: content_block_start
data: {"type":"content_block_start","index":0,"content_block":{"type":"thinking","thinking":"","signature":""}}

event: content_block_delta
data: {"type":"content_block_delta","index":0,"delta":{"type":"thinking_delta","thinking":"anthropic reasoning"}}

event: content_block_stop
data: {"type":"content_block_stop","index":0}

event: content_block_start
data: {"type":"content_block_start","index":1,"content_block":{"type":"text","text":""}}

event: content_block_delta
data: {"type":"content_block_delta","index":1,"delta":{"type":"text_delta","text":"anthropic reasoning answer"}}

event: content_block_stop
data: {"type":"content_block_stop","index":1}

event: message_delta
data: {"type":"message_delta","delta":{"stop_reason":"end_turn"},"usage":{"output_tokens":2}}

event: message_stop
data: {"type":"message_stop"}

`)
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

func writeTruncatedSSE(w http.ResponseWriter, body string) {
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Content-Length", fmt.Sprintf("%d", len(body)+1))
	_, _ = fmt.Fprint(w, body)
	if flusher, ok := w.(http.Flusher); ok {
		flusher.Flush()
	}
}

func writeDeadlineBoundSSE(w http.ResponseWriter, request *http.Request, body string) {
	w.Header().Set("Content-Type", "text/event-stream")
	_, _ = fmt.Fprint(w, body)
	if flusher, ok := w.(http.Flusher); ok {
		flusher.Flush()
	}
	<-request.Context().Done()
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
