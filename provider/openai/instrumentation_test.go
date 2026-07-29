package openai

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/fugue-labs/gollem/core"
	"github.com/gorilla/websocket"
)

// captureObserver returns a RequestObserver and a snapshot function that
// safely copies the latest trace under a mutex.
func captureObserver() (RequestObserver, func() (RequestTrace, bool)) {
	var (
		mu    sync.Mutex
		trace RequestTrace
		got   bool
	)
	obs := func(t RequestTrace) {
		mu.Lock()
		defer mu.Unlock()
		trace = t
		got = true
	}
	snapshot := func() (RequestTrace, bool) {
		mu.Lock()
		defer mu.Unlock()
		return trace, got
	}
	return obs, snapshot
}

// chatgptSSEHandler returns an httptest.Server that behaves like the ChatGPT
// backend: it streams the provided SSE frames with an optional per-frame delay
// and records the received request body for assertions.
func chatgptSSEHandler(t *testing.T, sse string, delays []time.Duration) (*httptest.Server, *[]byte) {
	t.Helper()
	var (
		mu   sync.Mutex
		body []byte
	)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		reqBody, _ := io.ReadAll(r.Body)
		mu.Lock()
		body = reqBody
		mu.Unlock()
		w.Header().Set("Content-Type", "text/event-stream")
		flusher, _ := w.(http.Flusher)
		frames := strings.Split(sse, "\n\n")
		for i, frame := range frames {
			if i < len(delays) && delays[i] > 0 {
				time.Sleep(delays[i])
			}
			_, _ = io.WriteString(w, frame+"\n\n")
			if flusher != nil {
				flusher.Flush()
			}
		}
	}))
	return server, &body
}

func TestRequestObserver_HTTPRequestCapturesPhases(t *testing.T) {
	// Insert measurable delays between SSE frames so first-event and terminal
	// timings are clearly distinguishable and > 0.
	sse := "data: {\"type\":\"response.output_item.done\",\"item\":{\"type\":\"message\",\"role\":\"assistant\",\"content\":[{\"type\":\"output_text\",\"text\":\"hi\"}]}}\n\n" +
		"data: {\"type\":\"response.completed\",\"response\":{\"id\":\"resp_1\",\"model\":\"gpt-5\",\"output\":[],\"usage\":{\"input_tokens\":3,\"output_tokens\":1}}}\n\n" +
		"data: [DONE]\n\n"
	server, recvBody := chatgptSSEHandler(t, sse, []time.Duration{30 * time.Millisecond, 30 * time.Millisecond})
	defer server.Close()

	obs, snapshot := captureObserver()
	p := New(
		WithChatGPTAuth("test-access-token", "test-account-id"),
		WithBaseURL(server.URL+"/chatgpt.com"),
		WithModel("gpt-5"),
		WithTokenRefresher(func() (string, error) { return "test-access-token", nil }),
		WithRequestObserver(obs),
	)

	resp, err := p.Request(context.Background(), []core.ModelMessage{
		core.ModelRequest{Parts: []core.ModelRequestPart{core.UserPromptPart{Content: "ping"}}},
	}, nil, nil)
	if err != nil {
		t.Fatalf("Request: %v", err)
	}
	if got := resp.TextContent(); got != "hi" {
		t.Fatalf("text = %q, want hi", got)
	}

	tr, ok := snapshot()
	if !ok {
		t.Fatal("expected observer to fire")
	}
	if tr.Transport != transportHTTP {
		t.Errorf("Transport = %q, want %q", tr.Transport, transportHTTP)
	}
	if tr.TotalDuration <= 0 {
		t.Error("TotalDuration should be > 0")
	}
	if tr.RequestBytes == 0 {
		t.Error("RequestBytes should be recorded")
	}
	if tr.HTTPStatus != http.StatusOK {
		t.Errorf("HTTPStatus = %d, want 200", tr.HTTPStatus)
	}
	if tr.TimeToHeaders <= 0 {
		t.Error("TimeToHeaders should be > 0")
	}
	if tr.TimeToFirstEvent <= 0 {
		t.Error("TimeToFirstEvent should be > 0")
	}
	if tr.TimeToFirstToken <= 0 {
		t.Error("TimeToFirstToken should be > 0 (output_item.done)")
	}
	if tr.TimeToTerminal <= 0 {
		t.Error("TimeToTerminal should be > 0")
	}
	if !tr.TokenRefreshInvoked {
		t.Error("TokenRefreshInvoked should be true")
	}
	if tr.TokenRefreshDuration < 0 {
		t.Error("TokenRefreshDuration should be >= 0")
	}
	if !tr.PromptCacheKeyActive {
		t.Error("PromptCacheKeyActive should be true")
	}
	if tr.PromptCacheKeyFingerprint == "" {
		t.Error("PromptCacheKeyFingerprint should be non-empty")
	}
	// The request should carry the prompt cache key (not the fingerprint).
	if !strings.Contains(string(*recvBody), "prompt_cache_key") {
		t.Error("request body should contain prompt_cache_key")
	}
}

func TestRequestObserver_HTTPRequestStreamCapturesPhases(t *testing.T) {
	sse := "data: {\"type\":\"response.output_text.delta\",\"delta\":\"Hel\"}\n\n" +
		"data: {\"type\":\"response.output_text.delta\",\"delta\":\"lo\"}\n\n" +
		"data: {\"type\":\"response.completed\",\"response\":{\"id\":\"resp_2\",\"model\":\"gpt-5\",\"output\":[],\"usage\":{\"input_tokens\":3,\"output_tokens\":2}}}\n\n" +
		"data: [DONE]\n\n"
	server, _ := chatgptSSEHandler(t, sse, []time.Duration{20 * time.Millisecond, 0, 20 * time.Millisecond})
	defer server.Close()

	obs, snapshot := captureObserver()
	p := New(
		WithChatGPTAuth("tok", "acct"),
		WithBaseURL(server.URL+"/chatgpt.com"),
		WithModel("gpt-5"),
		WithRequestObserver(obs),
	)

	stream, err := p.RequestStream(context.Background(), []core.ModelMessage{
		core.ModelRequest{Parts: []core.ModelRequestPart{core.UserPromptPart{Content: "hi"}}},
	}, nil, nil)
	if err != nil {
		t.Fatalf("RequestStream: %v", err)
	}
	for {
		ev, err := stream.Next()
		if err != nil {
			break
		}
		_ = ev
	}
	if err := stream.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	tr, ok := snapshot()
	if !ok {
		t.Fatal("expected observer to fire after draining stream")
	}
	if tr.TimeToFirstToken <= 0 {
		t.Error("TimeToFirstToken should be > 0 for streaming")
	}
	if tr.TimeToTerminal <= 0 {
		t.Error("TimeToTerminal should be > 0")
	}
	if tr.TotalDuration <= 0 {
		t.Error("TotalDuration should be > 0")
	}
}

// wsFakeServer returns an httptest.Server that upgrades to a websocket and
// streams the given event types with optional inter-event delays.
func wsFakeServer(t *testing.T, events []responsesWSEvent, delays []time.Duration) *httptest.Server {
	t.Helper()
	upgrader := websocket.Upgrader{CheckOrigin: func(*http.Request) bool { return true }}
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		defer conn.Close()
		if _, _, err := conn.ReadMessage(); err != nil {
			return
		}
		for i, ev := range events {
			if i < len(delays) && delays[i] > 0 {
				time.Sleep(delays[i])
			}
			if err := conn.WriteJSON(ev); err != nil {
				return
			}
		}
	}))
}

func TestRequestObserver_WebSocketCapturesPhases(t *testing.T) {
	terminal := responsesWSEvent{
		Type: "response.completed",
		Response: &responsesAPIResponse{
			ID: "resp_ws_1", Model: "gpt-5.3-codex",
			Output: []responsesOutputItem{{
				Type: "message", Role: "assistant",
				Content: []responsesContentItem{{Type: "output_text", Text: "ok"}},
			}},
		},
	}
	server := wsFakeServer(t, []responsesWSEvent{
		{Type: "response.output_item.done", Item: &responsesOutputItem{
			Type: "message", Role: "assistant",
			Content: []responsesContentItem{{Type: "output_text", Text: "ok"}},
		}},
		terminal,
	}, []time.Duration{40 * time.Millisecond, 40 * time.Millisecond})
	defer server.Close()

	obs, snapshot := captureObserver()
	p := New(
		WithAPIKey("test-key"),
		WithModel("gpt-5.3-codex"),
		WithBaseURL(server.URL),
		WithTransport("websocket"),
		WithRequestObserver(obs),
	)

	if _, err := p.Request(context.Background(), []core.ModelMessage{
		core.ModelRequest{Parts: []core.ModelRequestPart{core.UserPromptPart{Content: "hi"}}},
	}, nil, nil); err != nil {
		t.Fatalf("Request: %v", err)
	}

	tr, ok := snapshot()
	if !ok {
		t.Fatal("expected observer to fire")
	}
	if tr.Transport != transportWebSocket {
		t.Errorf("Transport = %q, want %q", tr.Transport, transportWebSocket)
	}
	if tr.TimeToHeaders <= 0 {
		t.Error("TimeToHeaders should be > 0 (dial/handshake)")
	}
	if tr.TimeToFirstEvent <= 0 {
		t.Error("TimeToFirstEvent should be > 0")
	}
	if tr.TimeToFirstToken <= 0 {
		t.Error("TimeToFirstToken should be > 0")
	}
	if tr.TimeToTerminal <= 0 {
		t.Error("TimeToTerminal should be > 0")
	}
}

func TestRequestObserver_WebSocketConnectionReuseAndContinuation(t *testing.T) {
	// The fake server always succeeds; the second request should reuse the
	// connection and use previous_response_id continuation.
	upgrader := websocket.Upgrader{CheckOrigin: func(*http.Request) bool { return true }}
	var (
		mu            sync.Mutex
		received      []responsesWSCreateEvent
		connDialCount int
	)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		connDialCount++
		mu.Unlock()
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		defer conn.Close()
		for {
			_, payload, err := conn.ReadMessage()
			if err != nil {
				return
			}
			var ev responsesWSCreateEvent
			if json.Unmarshal(payload, &ev) != nil {
				return
			}
			mu.Lock()
			received = append(received, ev)
			idx := len(received)
			mu.Unlock()
			_ = conn.WriteJSON(responsesWSEvent{
				Type: "response.completed",
				Response: &responsesAPIResponse{
					ID:    responsesIDForIndex(idx),
					Model: "gpt-5.3-codex",
					Output: []responsesOutputItem{{
						Type: "message", Role: "assistant",
						Content: []responsesContentItem{{Type: "output_text", Text: "ok"}},
					}},
				},
			})
		}
	}))
	defer server.Close()

	obs, snapshot := captureObserver()
	p := New(
		WithAPIKey("test-key"),
		WithModel("gpt-5.3-codex"),
		WithBaseURL(server.URL),
		WithTransport("websocket"),
		WithRequestObserver(obs),
	)

	first := []core.ModelMessage{
		core.ModelRequest{Parts: []core.ModelRequestPart{core.UserPromptPart{Content: "turn one"}}},
	}
	if _, err := p.Request(context.Background(), first, nil, nil); err != nil {
		t.Fatalf("first Request: %v", err)
	}
	tr1, _ := snapshot()
	if tr1.WebSocketConnectionReused {
		t.Error("first request should NOT reuse a connection")
	}

	second := []core.ModelMessage{
		core.ModelRequest{Parts: []core.ModelRequestPart{core.UserPromptPart{Content: "turn one"}}},
		core.ModelResponse{Parts: []core.ModelResponsePart{core.TextPart{Content: "ok"}}},
		core.ModelRequest{Parts: []core.ModelRequestPart{core.UserPromptPart{Content: "turn two"}}},
	}
	if _, err := p.Request(context.Background(), second, nil, nil); err != nil {
		t.Fatalf("second Request: %v", err)
	}
	tr2, _ := snapshot()
	if !tr2.WebSocketConnectionReused {
		t.Error("second request should reuse the websocket connection")
	}
	if !tr2.PreviousResponseIDReused {
		t.Error("second request should reuse previous_response_id")
	}

	mu.Lock()
	defer mu.Unlock()
	if connDialCount != 1 {
		t.Errorf("expected exactly 1 dial, got %d", connDialCount)
	}
}

func responsesIDForIndex(i int) string {
	switch i {
	case 1:
		return "resp_a"
	case 2:
		return "resp_b"
	default:
		return "resp_x"
	}
}

func TestRequestObserver_NilObserverIsNoOp(t *testing.T) {
	server, _ := chatgptSSEHandler(t,
		"data: {\"type\":\"response.completed\",\"response\":{\"id\":\"r\",\"model\":\"gpt-5\",\"output\":[{\"type\":\"message\",\"role\":\"assistant\",\"content\":[{\"type\":\"output_text\",\"text\":\"ok\"}]}],\"usage\":{\"input_tokens\":1,\"output_tokens\":1}}}\n\ndata: [DONE]\n\n",
		nil)
	defer server.Close()

	// No WithRequestObserver at all; must not panic and must succeed.
	p := New(
		WithChatGPTAuth("tok", "acct"),
		WithBaseURL(server.URL+"/chatgpt.com"),
		WithModel("gpt-5"),
	)
	resp, err := p.Request(context.Background(), []core.ModelMessage{
		core.ModelRequest{Parts: []core.ModelRequestPart{core.UserPromptPart{Content: "hi"}}},
	}, nil, nil)
	if err != nil {
		t.Fatalf("Request: %v", err)
	}
	if got := resp.TextContent(); got != "ok" {
		t.Fatalf("text = %q, want ok", got)
	}
}

func TestRequestObserver_RecordsSanitizedErrorClassification(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Retry-After", "5")
		http.Error(w, `{"error":{"type":"rate_limit_exceeded","message":"too many requests"}}`, http.StatusTooManyRequests)
	}))
	defer server.Close()

	obs, snapshot := captureObserver()
	p := New(
		WithChatGPTAuth("tok", "acct"),
		WithBaseURL(server.URL+"/chatgpt.com"),
		WithModel("gpt-5"),
		WithRequestObserver(obs),
	)

	_, err := p.Request(context.Background(), []core.ModelMessage{
		core.ModelRequest{Parts: []core.ModelRequestPart{core.UserPromptPart{Content: "hi"}}},
	}, nil, nil)
	if err == nil {
		t.Fatal("expected error from 429")
	}
	tr, ok := snapshot()
	if !ok {
		t.Fatal("expected observer to fire even on error")
	}
	if tr.HTTPStatus != http.StatusTooManyRequests {
		t.Errorf("HTTPStatus = %d, want 429", tr.HTTPStatus)
	}
	if tr.ErrorClassification == "" {
		t.Error("ErrorClassification should be non-empty")
	}
	if !strings.Contains(tr.ErrorClassification, "rate") {
		t.Errorf("ErrorClassification = %q, want a rate-limit marker", tr.ErrorClassification)
	}
	if tr.ErrorClass != "http" {
		t.Errorf("ErrorClass = %q, want http", tr.ErrorClass)
	}
	if tr.RetryAfter != 5*time.Second {
		t.Errorf("RetryAfter = %v, want 5s", tr.RetryAfter)
	}
}

func TestRequestObserver_NoCredentialsInTrace(t *testing.T) {
	const (
		accessToken  = "AKIA-SENSITIVE-ACCESS-TOKEN-12345"
		refreshToken = "rt-super-secret-refresh-67890"
		accountID    = "acct-SUPER-SECRET-ACCOUNT"
		deviceCode   = "DEVICE-CODE-LEAK-ME"
	)
	server, _ := chatgptSSEHandler(t,
		"data: {\"type\":\"response.completed\",\"response\":{\"id\":\"r\",\"model\":\"gpt-5\",\"output\":[{\"type\":\"message\",\"role\":\"assistant\",\"content\":[{\"type\":\"output_text\",\"text\":\"ok\"}]}],\"usage\":{\"input_tokens\":1,\"output_tokens\":1}}}\n\ndata: [DONE]\n\n",
		nil)
	defer server.Close()

	obs, snapshot := captureObserver()
	p := New(
		WithChatGPTAuth(accessToken, accountID),
		WithBaseURL(server.URL+"/chatgpt.com"),
		WithModel("gpt-5"),
		WithTokenRefresher(func() (string, error) { return accessToken, nil }),
		WithRequestObserver(obs),
	)

	if _, err := p.Request(context.Background(), []core.ModelMessage{
		core.ModelRequest{Parts: []core.ModelRequestPart{core.UserPromptPart{Content: "hi"}}},
	}, nil, nil); err != nil {
		t.Fatalf("Request: %v", err)
	}

	tr, ok := snapshot()
	if !ok {
		t.Fatal("expected observer to fire")
	}
	// Marshal the trace and assert none of the secret material appears anywhere.
	b, err := json.Marshal(tr)
	if err != nil {
		t.Fatalf("marshal trace: %v", err)
	}
	rendered := formatRequestTrace(tr)
	for _, secret := range []string{accessToken, refreshToken, accountID, deviceCode} {
		if strings.Contains(string(b), secret) {
			t.Errorf("trace JSON leaked secret %q:\n%s", secret, string(b))
		}
		if strings.Contains(rendered, secret) {
			t.Errorf("trace rendering leaked secret %q:\n%s", secret, rendered)
		}
	}
	// The cache key fingerprint is a sha256 prefix, not the key itself.
	if tr.PromptCacheKeyFingerprint != "" && len(tr.PromptCacheKeyFingerprint) > 16 {
		t.Errorf("fingerprint too long, suspected raw key: %q", tr.PromptCacheKeyFingerprint)
	}
}

func TestRequestObserver_TokenRefreshDurationMeasured(t *testing.T) {
	// The refresher sleeps a fixed amount so TokenRefreshDuration reflects it.
	const refreshSleep = 60 * time.Millisecond
	server, _ := chatgptSSEHandler(t,
		"data: {\"type\":\"response.completed\",\"response\":{\"id\":\"r\",\"model\":\"gpt-5\",\"output\":[{\"type\":\"message\",\"role\":\"assistant\",\"content\":[{\"type\":\"output_text\",\"text\":\"ok\"}]}],\"usage\":{\"input_tokens\":1,\"output_tokens\":1}}}\n\ndata: [DONE]\n\n",
		nil)
	defer server.Close()

	obs, snapshot := captureObserver()
	p := New(
		WithChatGPTAuth("tok", "acct"),
		WithBaseURL(server.URL+"/chatgpt.com"),
		WithModel("gpt-5"),
		WithTokenRefresher(func() (string, error) {
			time.Sleep(refreshSleep)
			return "tok", nil
		}),
		WithRequestObserver(obs),
	)

	if _, err := p.Request(context.Background(), []core.ModelMessage{
		core.ModelRequest{Parts: []core.ModelRequestPart{core.UserPromptPart{Content: "hi"}}},
	}, nil, nil); err != nil {
		t.Fatalf("Request: %v", err)
	}
	tr, ok := snapshot()
	if !ok {
		t.Fatal("expected observer to fire")
	}
	if !tr.TokenRefreshInvoked {
		t.Fatal("TokenRefreshInvoked should be true")
	}
	if tr.TokenRefreshDuration < refreshSleep {
		t.Errorf("TokenRefreshDuration = %v, want >= %v", tr.TokenRefreshDuration, refreshSleep)
	}
}

func TestCacheKeyFingerprintStableAndShort(t *testing.T) {
	fp := cacheKeyFingerprint("some-cache-key")
	if len(fp) != 8 {
		t.Fatalf("fingerprint length = %d, want 8", len(fp))
	}
	if cacheKeyFingerprint("some-cache-key") != fp {
		t.Error("fingerprint should be stable for the same key")
	}
	if cacheKeyFingerprint("different-key") == fp {
		t.Error("fingerprint should differ for a different key")
	}
	if cacheKeyFingerprint("") != "" {
		t.Error("empty key should yield empty fingerprint")
	}
}

func TestErrorClassCategories(t *testing.T) {
	if errorClass(nil) != "" {
		t.Error("nil error should classify as empty")
	}
	if got := errorClass(&core.ModelHTTPError{StatusCode: 500}); got != "http" {
		t.Errorf("http error class = %q, want http", got)
	}
	if got := errorClass(&ModelIdentityError{}); got != "identity" {
		t.Errorf("identity error class = %q, want identity", got)
	}
	if got := errorClass(context.Canceled); got != "context" {
		t.Errorf("canceled class = %q, want context", got)
	}
	if got := errorClass(io.ErrUnexpectedEOF); got != "transport" {
		t.Errorf("transport error class = %q, want transport", got)
	}
}

func TestDefaultStderrRequestObserverDoesNotPanic(t *testing.T) {
	obs := DefaultStderrRequestObserver()
	obs(RequestTrace{Transport: transportHTTP, Model: "gpt-5"})
}
