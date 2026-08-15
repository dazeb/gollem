package appserver

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	appcache "github.com/fugue-labs/gollem/appserver/cache"
	"github.com/fugue-labs/gollem/core"
	"github.com/fugue-labs/gollem/provider/anthropic"
	"github.com/fugue-labs/gollem/provider/openai"
)

func TestRuntimeCacheTelemetryMeetsRepeatedStreamGateForNativeProviders(t *testing.T) {
	for _, tc := range []struct {
		name     string
		provider string
		model    string
		newModel func(baseURL string) core.Model
		handler  func(*int) http.Handler
	}{
		{
			name:     "openai",
			provider: "openai",
			model:    "gpt-4o",
			newModel: func(baseURL string) core.Model {
				return openai.New(
					openai.WithAPIKey("deterministic-openai-key"),
					openai.WithBaseURL(baseURL),
					openai.WithModel("gpt-4o"),
				)
			},
			handler: openAILiveCacheFixture,
		},
		{
			name:     "anthropic",
			provider: "anthropic",
			model:    anthropic.ClaudeSonnet46,
			newModel: func(baseURL string) core.Model {
				return anthropic.New(
					anthropic.WithAPIKey("deterministic-anthropic-key"),
					anthropic.WithBaseURL(baseURL),
					anthropic.WithModel(anthropic.ClaudeSonnet46),
				)
			},
			handler: anthropicLiveCacheFixture,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			requests := 0
			server := httptest.NewServer(tc.handler(&requests))
			defer server.Close()

			cacheSvc := appcache.NewService()
			model := newRuntimeCacheTelemetryModel(
				tc.newModel(server.URL),
				RuntimeModelInfo{ProviderID: tc.provider, Model: tc.model},
				cacheSvc.RecordLiveProviderEvent,
			)
			for range 10 {
				stream, err := model.RequestStream(context.Background(), []core.ModelMessage{core.ModelRequest{
					Parts: []core.ModelRequestPart{core.UserPromptPart{Content: "repeat this cacheable request"}},
				}}, nil, &core.ModelRequestParameters{AllowTextOutput: true})
				if err != nil {
					t.Fatalf("RequestStream: %v", err)
				}
				for {
					_, err := stream.Next()
					if errors.Is(err, io.EOF) {
						break
					}
					if err != nil {
						t.Fatalf("stream: %v", err)
					}
				}
				if err := stream.Close(); err != nil {
					t.Fatalf("stream.Close: %v", err)
				}
			}

			stats := cacheSvc.Stats()
			if requests != 10 || stats.TotalRequests != 10 || stats.Misses != 1 || stats.Hits != 9 {
				t.Fatalf("live repeated cache stats = %#v, requests = %d", stats, requests)
			}
			if stats.HitRate < 0.90 {
				t.Fatalf("live repeated cache hit rate = %f, want >= 0.90", stats.HitRate)
			}
			if len(stats.Providers) != 1 || stats.Providers[0].Provider != tc.provider {
				t.Fatalf("live repeated provider stats = %#v", stats.Providers)
			}
			for index, event := range stats.RecentEvents {
				if event.Source != "provider-usage" {
					t.Fatalf("event %d source = %q, want provider-usage", index, event.Source)
				}
				if index == 0 && event.Type != "cache.miss" {
					t.Fatalf("first event = %#v, want cache miss", event)
				}
				if index > 0 && (event.Type != "cache.hit" || event.CacheReadTokens == 0) {
					t.Fatalf("repeat event %d = %#v, want cache hit with read tokens", index, event)
				}
			}
		})
	}
}

func openAILiveCacheFixture(requests *int) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/chat/completions" {
			http.NotFound(w, r)
			return
		}
		*requests = *requests + 1
		cachedTokens := 0
		if *requests > 1 {
			cachedTokens = 90
		}
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = fmt.Fprintf(w, `data: {"id":"cache-openai","object":"chat.completion.chunk","model":"gpt-4o","choices":[{"index":0,"delta":{"content":"cached"},"finish_reason":"stop"}],"usage":{"prompt_tokens":100,"completion_tokens":1,"total_tokens":101,"prompt_tokens_details":{"cached_tokens":%d}}}

data: [DONE]

`, cachedTokens)
	})
}

func anthropicLiveCacheFixture(requests *int) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/messages" {
			http.NotFound(w, r)
			return
		}
		*requests = *requests + 1
		cacheCreation := 90
		cacheRead := 0
		if *requests > 1 {
			cacheCreation = 0
			cacheRead = 90
		}
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = fmt.Fprintf(w, `event: message_start
data: {"type":"message_start","message":{"usage":{"input_tokens":10,"cache_creation_input_tokens":%d,"cache_read_input_tokens":%d}}}

event: content_block_start
data: {"type":"content_block_start","index":0,"content_block":{"type":"text","text":""}}

event: content_block_delta
data: {"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"cached"}}

event: content_block_stop
data: {"type":"content_block_stop","index":0}

event: message_delta
data: {"type":"message_delta","delta":{"stop_reason":"end_turn"},"usage":{"output_tokens":1}}

event: message_stop
data: {"type":"message_stop"}

`, cacheCreation, cacheRead)
	})
}
