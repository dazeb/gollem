package openai

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/fugue-labs/gollem/core"
	"github.com/gorilla/websocket"
)

// These benchmarks are the first in the repo (see issue #242). They serve two
// purposes:
//   - BenchmarkChatGPTRequest_*: end-to-end ChatGPT-auth request latency against
//     a minimal local backend, across the HTTP and WebSocket transports, with
//     and without a request observer. They are the reproducible phase-attribution
//     harness the issue's acceptance criteria call for (run with -benchmem).
//   - BenchmarkInstrumentationOverhead: isolates the per-request cost of having
//     an observer installed vs. nil, so we can prove the nil path is free and
//     quantify the observed path overhead.
//
// The ChatGPT endpoint is simulated by appending "/chatgpt.com" to the test
// server URL, the same trick used elsewhere in this package's tests to make
// isChatGPTEndpoint() return true while physically hitting the local server.

// fixedChatGPTSSE is a minimal ChatGPT-backend SSE response.
const fixedChatGPTSSE = "data: {\"type\":\"response.output_item.done\",\"item\":{\"type\":\"message\",\"role\":\"assistant\",\"content\":[{\"type\":\"output_text\",\"text\":\"ok\"}]}}\n\n" +
	"data: {\"type\":\"response.completed\",\"response\":{\"id\":\"r\",\"model\":\"gpt-5\",\"output\":[],\"usage\":{\"input_tokens\":3,\"output_tokens\":1}}}\n\n" +
	"data: [DONE]\n\n"

func chatgptProvider(serverURL, model string, opts ...Option) *Provider {
	all := append([]Option{
		WithChatGPTAuth("token", "acct"),
		WithBaseURL(serverURL + "/chatgpt.com"),
		WithModel(model),
	}, opts...)
	return New(all...)
}

func BenchmarkChatGPTRequest_HTTP_NoObserver(b *testing.B) {
	benchChatGPTRequest(b, false)
}

func BenchmarkChatGPTRequest_HTTP_WithObserver(b *testing.B) {
	benchChatGPTRequest(b, true)
}

func benchChatGPTRequest(b *testing.B, withObserver bool) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.Write([]byte(fixedChatGPTSSE))
	}))
	defer server.Close()

	var opts []Option
	if withObserver {
		// A no-op observer still exercises the full timing-accumulation path.
		opts = append(opts, WithRequestObserver(func(RequestTrace) {}))
	}
	p := chatgptProvider(server.URL, "gpt-5", opts...)

	msgs := []core.ModelMessage{
		core.ModelRequest{Parts: []core.ModelRequestPart{core.UserPromptPart{Content: "hi"}}},
	}
	ctx := context.Background()

	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		if _, err := p.Request(ctx, msgs, nil, nil); err != nil {
			b.Fatalf("Request: %v", err)
		}
	}
}

func BenchmarkChatGPTRequest_WebSocket_WithObserver(b *testing.B) {
	upgrader := websocket.Upgrader{CheckOrigin: func(*http.Request) bool { return true }}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		defer conn.Close()
		for {
			if _, _, err := conn.ReadMessage(); err != nil {
				return
			}
			_ = conn.WriteJSON(responsesWSEvent{
				Type: "response.completed",
				Response: &responsesAPIResponse{
					ID: "r", Model: "gpt-5.3-codex",
					Output: []responsesOutputItem{{
						Type: "message", Role: "assistant",
						Content: []responsesContentItem{{Type: "output_text", Text: "ok"}},
					}},
				},
			})
		}
	}))
	defer server.Close()

	p := New(
		WithAPIKey("test-key"),
		WithModel("gpt-5.3-codex"),
		WithBaseURL(server.URL),
		WithTransport("websocket"),
		WithRequestObserver(func(RequestTrace) {}),
	)
	defer p.Close()

	msgs := []core.ModelMessage{
		core.ModelRequest{Parts: []core.ModelRequestPart{core.UserPromptPart{Content: "hi"}}},
	}
	ctx := context.Background()

	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		if _, err := p.Request(ctx, msgs, nil, nil); err != nil {
			b.Fatalf("Request: %v", err)
		}
	}
}

// BenchmarkInstrumentationOverhead isolates the marginal cost of an installed
// observer by comparing the nil-observer and no-op-observer paths against the
// same local backend.
func BenchmarkInstrumentationOverhead(b *testing.B) {
	ctx := context.Background()
	msgs := []core.ModelMessage{
		core.ModelRequest{Parts: []core.ModelRequestPart{core.UserPromptPart{Content: "hi"}}},
	}

	b.Run("nil_observer", func(b *testing.B) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "text/event-stream")
			w.Write([]byte(fixedChatGPTSSE))
		}))
		defer server.Close()
		p := chatgptProvider(server.URL, "gpt-5")
		b.ReportAllocs()
		b.ResetTimer()
		for range b.N {
			if _, err := p.Request(ctx, msgs, nil, nil); err != nil {
				b.Fatalf("Request: %v", err)
			}
		}
	})

	b.Run("noop_observer", func(b *testing.B) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "text/event-stream")
			w.Write([]byte(fixedChatGPTSSE))
		}))
		defer server.Close()
		p := chatgptProvider(server.URL, "gpt-5", WithRequestObserver(func(RequestTrace) {}))
		b.ReportAllocs()
		b.ResetTimer()
		for range b.N {
			if _, err := p.Request(ctx, msgs, nil, nil); err != nil {
				b.Fatalf("Request: %v", err)
			}
		}
	})
}
