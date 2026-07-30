package openai

import (
	"context"
	"encoding/json"
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/fugue-labs/gollem/core"
)

func TestNormalizeLocalEndpointConfig(t *testing.T) {
	tests := []struct {
		name    string
		baseURL string
		want    string
		valid   bool
	}{
		{name: "loopback IPv4", baseURL: "http://127.0.0.1:8080/v1/", want: "http://127.0.0.1:8080", valid: true},
		{name: "localhost", baseURL: "https://localhost:8443", want: "https://localhost:8443", valid: true},
		{name: "loopback IPv6", baseURL: "http://[::1]:8080/v1", want: "http://[::1]:8080", valid: true},
		{name: "remote host", baseURL: "https://example.com", valid: false},
		{name: "non HTTP scheme", baseURL: "file:///tmp/model", valid: false},
		{name: "unexpected path", baseURL: "http://127.0.0.1:8080/api", valid: false},
		{name: "userinfo", baseURL: "http://token@127.0.0.1:8080", valid: false},
		{name: "query", baseURL: "http://127.0.0.1:8080?token=secret", valid: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := NormalizeLocalEndpointConfig(LocalEndpointConfig{BaseURL: tt.baseURL, Model: "local-tool-model"})
			if tt.valid {
				if err != nil {
					t.Fatalf("NormalizeLocalEndpointConfig: %v", err)
				}
				if got.BaseURL != tt.want {
					t.Fatalf("BaseURL = %q, want %q", got.BaseURL, tt.want)
				}
				return
			}
			if err == nil {
				t.Fatal("NormalizeLocalEndpointConfig unexpectedly succeeded")
			}
		})
	}
}

func TestLocalEndpointForcesChatCompletionsAndDoesNotInheritOpenAIKey(t *testing.T) {
	t.Setenv("OPENAI_API_KEY", "remote-openai-key")
	t.Setenv("OPENAI_PROMPT_CACHE_KEY", "remote-cache-key")
	t.Setenv("OPENAI_PROMPT_CACHE_RETENTION", "24h")
	t.Setenv("OPENAI_SERVICE_TIER", "priority")
	t.Setenv("OPENAI_TRANSPORT", "websocket")
	t.Setenv("OPENAI_REASONING_SUMMARY", "detailed")
	t.Setenv("OPENAI_TEXT_VERBOSITY", "high")
	var gotAuthorization string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/chat/completions" {
			t.Fatalf("path = %q, want /v1/chat/completions", r.URL.Path)
		}
		gotAuthorization = r.Header.Get("Authorization")
		if got := r.Header.Get("ChatGPT-Account-ID"); got != "" {
			t.Fatalf("local request inherited ChatGPT account identity %q", got)
		}
		var request map[string]any
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		for _, key := range []string{"prompt_cache_key", "prompt_cache_retention", "service_tier", "reasoning", "text"} {
			if _, ok := request[key]; ok {
				t.Fatalf("local request inherited OpenAI setting %q: %#v", key, request)
			}
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"chatcmpl-local","object":"chat.completion","model":"gpt-5.2-codex","choices":[{"message":{"role":"assistant","content":"local reply"},"finish_reason":"stop"}]}`))
	}))
	defer server.Close()

	p, err := NewLocalEndpoint(
		LocalEndpointConfig{BaseURL: server.URL, Model: "gpt-5.2-codex"},
		WithChatGPTAuth("remote-chatgpt-token", "remote-account"),
		WithTokenRefresher(func() (string, error) { return "refreshed-remote-token", nil }),
		WithResumedResponsesChain("resp_remote", []string{"remote-input"}),
	)
	if err != nil {
		t.Fatalf("NewLocalEndpoint: %v", err)
	}
	response, err := p.Request(context.Background(), localEndpointTestMessages(), nil, nil)
	if err != nil {
		t.Fatalf("Request: %v", err)
	}
	if got := response.TextContent(); got != "local reply" {
		t.Fatalf("response = %q, want local reply", got)
	}
	if gotAuthorization != "Bearer local" {
		t.Fatalf("Authorization = %q, want local key", gotAuthorization)
	}
	profile := p.Profile()
	if !profile.SupportsToolCalls || !profile.SupportsStreaming || profile.SupportsStructuredOutput || profile.SupportsVision {
		t.Fatalf("local profile = %#v", profile)
	}
}

func TestLocalEndpointRedactsUnavailableEndpoint(t *testing.T) {
	listener, err := (&net.ListenConfig{}).Listen(context.Background(), "tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	baseURL := "http://" + listener.Addr().String()
	if err := listener.Close(); err != nil {
		t.Fatalf("close listener: %v", err)
	}

	p, err := NewLocalEndpoint(LocalEndpointConfig{BaseURL: baseURL, Model: "local-tool-model"})
	if err != nil {
		t.Fatalf("NewLocalEndpoint: %v", err)
	}
	_, err = p.Request(context.Background(), localEndpointTestMessages(), nil, nil)
	if err == nil {
		t.Fatal("Request unexpectedly succeeded")
	}
	if strings.Contains(err.Error(), baseURL) || strings.Contains(err.Error(), listener.Addr().String()) {
		t.Fatalf("unavailable endpoint error leaked endpoint: %v", err)
	}
	if got := err.Error(); got != "openai: local endpoint unavailable" {
		t.Fatalf("error = %q, want redacted local endpoint error", got)
	}
}

func TestProbeLocalEndpointUsesDiscoveryAndRedactsFailure(t *testing.T) {
	const token = "local-probe-token"
	var authorization string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/v1/models" {
			t.Fatalf("probe request = %s %s, want GET /v1/models", r.Method, r.URL.Path)
		}
		authorization = r.Header.Get("Authorization")
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":[]}`))
	}))
	defer server.Close()

	if err := ProbeLocalEndpoint(context.Background(), LocalEndpointConfig{
		BaseURL: server.URL,
		Model:   "local-tool-model",
		Token:   token,
	}, nil); err != nil {
		t.Fatalf("ProbeLocalEndpoint: %v", err)
	}
	if authorization != "Bearer "+token {
		t.Fatalf("Authorization = %q, want local token", authorization)
	}

	unavailable := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
		_, _ = w.Write([]byte("unavailable at /private/path"))
	}))
	defer unavailable.Close()
	err := ProbeLocalEndpoint(context.Background(), LocalEndpointConfig{
		BaseURL: unavailable.URL,
		Model:   "local-tool-model",
		Token:   token,
	}, nil)
	if !errors.Is(err, ErrLocalEndpointUnavailable) {
		t.Fatalf("unavailable probe error = %v, want ErrLocalEndpointUnavailable", err)
	}
	if strings.Contains(err.Error(), unavailable.URL) || strings.Contains(err.Error(), token) {
		t.Fatalf("unavailable probe leaked local details: %v", err)
	}
}

func localEndpointTestMessages() []core.ModelMessage {
	return []core.ModelMessage{
		core.ModelRequest{Parts: []core.ModelRequestPart{core.UserPromptPart{Content: "hello"}}},
	}
}
