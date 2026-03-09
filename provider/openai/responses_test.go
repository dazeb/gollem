package openai

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/fugue-labs/gollem/core"
)

func TestChatGPTAuth_PromptCacheKey(t *testing.T) {
	// Verify that ChatGPT auth mode auto-generates a prompt cache key
	// and that applyChatGPTRequirements preserves it.
	p := New(WithChatGPTAuth("token", "acct"))
	if p.promptCacheKey == "" {
		t.Fatal("expected auto-generated promptCacheKey for ChatGPT endpoint")
	}

	// Build a request and apply ChatGPT requirements.
	req := &responsesRequest{
		Model:                "gpt-5",
		PromptCacheKey:       p.promptCacheKey,
		MaxOutputTokens:      128000,
		ServiceTier:          "default",
		PromptCacheRetention: "24h",
	}
	p.applyChatGPTRequirements(req)

	// prompt_cache_key should be preserved.
	if req.PromptCacheKey != p.promptCacheKey {
		t.Errorf("expected prompt_cache_key preserved as %q, got %q", p.promptCacheKey, req.PromptCacheKey)
	}
	// Other unsupported fields should be cleared.
	if req.PromptCacheRetention != "" {
		t.Errorf("expected prompt_cache_retention cleared, got %q", req.PromptCacheRetention)
	}
	if req.ServiceTier != "" {
		t.Errorf("expected service_tier cleared, got %q", req.ServiceTier)
	}
	if req.MaxOutputTokens != 0 {
		t.Errorf("expected max_output_tokens cleared, got %d", req.MaxOutputTokens)
	}
}

func TestChatGPTAuth_PromptCacheKeyInRequest(t *testing.T) {
	// Verify that prompt_cache_key is sent in the request body when using
	// ChatGPT auth, even with a custom base URL (proxy/test scenario).
	// The hasChatGPTAuth() fallback ensures caching works through proxies.
	var receivedCacheKey string
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		var req map[string]any
		json.Unmarshal(body, &req)
		if key, ok := req["prompt_cache_key"].(string); ok {
			receivedCacheKey = key
		}
		// Return plain JSON (test server, not chatgpt.com).
		json.NewEncoder(w).Encode(responsesAPIResponse{
			Output: []responsesOutputItem{
				{Type: "message", Content: []responsesContentItem{{Type: "output_text", Text: "ok"}}},
			},
		})
	}))
	defer ts.Close()

	// WithBaseURL overrides chatgptBaseURL, so isChatGPTEndpoint() is false.
	// But hasChatGPTAuth() is true via chatgptAccountID, so caching still works.
	p := New(
		WithChatGPTAuth("token", "acct"),
		WithBaseURL(ts.URL),
		WithPromptCacheKey("test-cache-key"),
	)

	_, err := p.Request(context.Background(), []core.ModelMessage{
		core.ModelRequest{Parts: []core.ModelRequestPart{core.UserPromptPart{Content: "hi"}}},
	}, nil, nil)
	if err != nil {
		t.Fatalf("Request: %v", err)
	}

	if receivedCacheKey != "test-cache-key" {
		t.Errorf("expected prompt_cache_key='test-cache-key' in request body, got %q", receivedCacheKey)
	}
}

func TestCompatibleEndpoint_NoCacheKey(t *testing.T) {
	// Verify that non-OpenAI/non-ChatGPT endpoints (xAI, etc.) do NOT
	// get prompt_cache_key, even if one is configured on the provider.
	var receivedBody map[string]any
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		json.Unmarshal(body, &receivedBody)
		json.NewEncoder(w).Encode(responsesAPIResponse{
			Output: []responsesOutputItem{
				{Type: "message", Content: []responsesContentItem{{Type: "output_text", Text: "ok"}}},
			},
		})
	}))
	defer ts.Close()

	p := New(
		WithModel("grok-3"),
		WithAPIKey("test-key"),
		WithBaseURL(ts.URL),
		WithPromptCacheKey("should-not-appear"),
	)
	p.useResponses = true

	_, err := p.Request(context.Background(), []core.ModelMessage{
		core.ModelRequest{Parts: []core.ModelRequestPart{core.UserPromptPart{Content: "hi"}}},
	}, nil, nil)
	if err != nil {
		t.Fatalf("Request: %v", err)
	}

	if _, has := receivedBody["prompt_cache_key"]; has {
		t.Error("expected no prompt_cache_key for non-OpenAI endpoint, but it was present")
	}
}
