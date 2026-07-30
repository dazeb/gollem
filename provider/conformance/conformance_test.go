package conformance_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/fugue-labs/gollem/provider/anthropic"
	"github.com/fugue-labs/gollem/provider/conformance"
	"github.com/fugue-labs/gollem/provider/openai"
)

func TestDeterministicProviderDriverConformance(t *testing.T) {
	openAIServer := httptest.NewServer(openAIConformanceFixture(t))
	defer openAIServer.Close()
	anthropicServer := httptest.NewServer(anthropicConformanceFixture(t))
	defer anthropicServer.Close()

	cases := []struct {
		name  string
		model func() (conformance.Driver, error)
	}{
		{
			name: "native OpenAI",
			model: func() (conformance.Driver, error) {
				return conformance.Driver{
					Name:   "native OpenAI",
					Model:  openai.New(openai.WithAPIKey("test-openai-key"), openai.WithBaseURL(openAIServer.URL), openai.WithModel("gpt-4o")),
					Claims: conformance.Claims{ToolCalls: true, Streaming: true, Usage: true},
					Expectations: conformance.Expectations{
						ResponseText: "openai response",
						ToolName:     "conformance_echo",
						StreamText:   "openai stream",
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
					Name:   "OpenAI-compatible local",
					Model:  model,
					Claims: conformance.Claims{ToolCalls: true, Streaming: true, Usage: true},
					Expectations: conformance.Expectations{
						ResponseText: "openai response",
						ToolName:     "conformance_echo",
						StreamText:   "openai stream",
					},
				}, nil
			},
		},
		{
			name: "native Anthropic",
			model: func() (conformance.Driver, error) {
				return conformance.Driver{
					Name:   "native Anthropic",
					Model:  anthropic.New(anthropic.WithAPIKey("test-anthropic-key"), anthropic.WithBaseURL(anthropicServer.URL), anthropic.WithModel(anthropic.ClaudeSonnet46)),
					Claims: conformance.Claims{ToolCalls: true, Streaming: true, Usage: true},
					Expectations: conformance.Expectations{
						ResponseText: "anthropic response",
						ToolName:     "conformance_echo",
						StreamText:   "anthropic stream",
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
}

func openAIConformanceFixture(t *testing.T) http.Handler {
	t.Helper()
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/chat/completions" {
			t.Fatalf("OpenAI path = %q, want /v1/chat/completions", r.URL.Path)
		}
		if r.Header.Get("Authorization") == "" {
			t.Fatal("OpenAI fixture request had no authorization header")
		}
		var request struct {
			Stream bool            `json:"stream"`
			Tools  json.RawMessage `json:"tools"`
		}
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Fatalf("decode OpenAI request: %v", err)
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

func anthropicConformanceFixture(t *testing.T) http.Handler {
	t.Helper()
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/messages" {
			t.Fatalf("Anthropic path = %q, want /v1/messages", r.URL.Path)
		}
		if r.Header.Get("x-api-key") == "" {
			t.Fatal("Anthropic fixture request had no API key header")
		}
		var request struct {
			Stream bool            `json:"stream"`
			Tools  json.RawMessage `json:"tools"`
		}
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Fatalf("decode Anthropic request: %v", err)
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
