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

// TestRequestObserver_SSEDeltaFirstToken verifies first-token is timestamped
// on response.output_text.delta, not only at output_item.done (Codex review).
func TestRequestObserver_SSEDeltaFirstToken(t *testing.T) {
	sse := "data: {\"type\":\"response.output_text.delta\",\"delta\":\"Hel\"}\n\n" +
		"data: {\"type\":\"response.output_text.delta\",\"delta\":\"lo\"}\n\n" +
		"data: {\"type\":\"response.output_item.done\",\"item\":{\"type\":\"message\",\"role\":\"assistant\",\"content\":[{\"type\":\"output_text\",\"text\":\"Hello\"}]}}\n\n" +
		"data: {\"type\":\"response.completed\",\"response\":{\"id\":\"r\",\"model\":\"gpt-5\",\"output\":[],\"usage\":{\"input_tokens\":3,\"output_tokens\":1}}}\n\n" +
		"data: [DONE]\n\n"
	server, _ := chatgptSSEHandler(t, sse, []time.Duration{20 * time.Millisecond, 0, 20 * time.Millisecond, 20 * time.Millisecond})
	defer server.Close()

	obs, snapshot := captureObserver()
	p := New(
		WithChatGPTAuth("tok", "acct"),
		WithBaseURL(server.URL+"/chatgpt.com"),
		WithModel("gpt-5"),
		WithRequestObserver(obs),
	)
	if _, err := p.Request(context.Background(), []core.ModelMessage{
		core.ModelRequest{Parts: []core.ModelRequestPart{core.UserPromptPart{Content: "hi"}}},
	}, nil, nil); err != nil {
		t.Fatalf("Request: %v", err)
	}
	tr, _ := snapshot()
	if tr.TimeToFirstToken <= 0 {
		t.Fatal("TimeToFirstToken should be > 0 from the delta event")
	}
	if tr.TimeToFirstToken >= tr.TimeToTerminal {
		t.Errorf("first_token (%v) should precede terminal (%v)", tr.TimeToFirstToken, tr.TimeToTerminal)
	}
}

// TestRequestObserver_SSEResponseFailed verifies in-band response.failed is
// classified and recorded as terminal in the non-stream SSE path.
func TestRequestObserver_SSEResponseFailed(t *testing.T) {
	sse := "data: {\"type\":\"response.failed\",\"response\":{\"id\":\"r\",\"model\":\"gpt-5\",\"incomplete_details\":{\"reason\":\"content_filter\"}}}\n\n" +
		"data: [DONE]\n\n"
	server, _ := chatgptSSEHandler(t, sse, nil)
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
		t.Fatal("expected error from response.failed")
	}
	tr, _ := snapshot()
	if tr.TimeToTerminal <= 0 {
		t.Error("TimeToTerminal should be recorded for response.failed")
	}
	if tr.ErrorClassification == "" {
		t.Error("ErrorClassification should be populated for response.failed")
	}
}

// TestRequestObserver_WebSocketShapeAndCache verifies the WS path records
// request bytes/items and the cache fingerprint (Codex review: WS branch
// previously reported zero/empty).
func TestRequestObserver_WebSocketShapeAndCache(t *testing.T) {
	server := wsFakeServer(t, []responsesWSEvent{
		{Type: "response.completed", Response: &responsesAPIResponse{
			ID: "r", Model: "gpt-5.3-codex",
			Output: []responsesOutputItem{{
				Type: "message", Role: "assistant",
				Content: []responsesContentItem{{Type: "output_text", Text: "ok"}},
			}},
		}},
	}, nil)
	defer server.Close()

	obs, snapshot := captureObserver()
	p := New(
		WithAPIKey("test-key"),
		WithModel("gpt-5.3-codex"),
		WithBaseURL(server.URL),
		WithTransport("websocket"),
		WithPromptCacheKey("my-cache-key"),
		WithRequestObserver(obs),
	)
	defer p.Close()

	if _, err := p.Request(context.Background(), []core.ModelMessage{
		core.ModelRequest{Parts: []core.ModelRequestPart{core.UserPromptPart{Content: "hi"}}},
	}, nil, nil); err != nil {
		t.Fatalf("Request: %v", err)
	}
	tr, _ := snapshot()
	if tr.RequestBytes == 0 {
		t.Error("RequestBytes should be > 0 on the WS path")
	}
	if tr.InputItems == 0 {
		t.Error("InputItems should be > 0 on the WS path")
	}
	if !tr.PromptCacheKeyActive || tr.PromptCacheKeyFingerprint == "" {
		t.Error("cache key should be active with a fingerprint on the WS path")
	}
	if tr.PromptCacheKeyFingerprint != cacheKeyFingerprint("my-cache-key") {
		t.Errorf("fingerprint mismatch: got %q", tr.PromptCacheKeyFingerprint)
	}
}

// TestRequestObserver_WebSocketReusedLeavesHeadersUnset verifies that a reused
// websocket connection does not record a synthetic TimeToHeaders.
func TestRequestObserver_WebSocketReusedLeavesHeadersUnset(t *testing.T) {
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

	obs, snapshot := captureObserver()
	p := New(
		WithAPIKey("test-key"),
		WithModel("gpt-5.3-codex"),
		WithBaseURL(server.URL),
		WithTransport("websocket"),
		WithRequestObserver(obs),
	)
	defer p.Close()

	// First request dials and records TimeToHeaders.
	if _, err := p.Request(context.Background(), []core.ModelMessage{
		core.ModelRequest{Parts: []core.ModelRequestPart{core.UserPromptPart{Content: "one"}}},
	}, nil, nil); err != nil {
		t.Fatalf("first Request: %v", err)
	}
	first, _ := snapshot()
	if first.TimeToHeaders <= 0 {
		t.Fatal("first request should record TimeToHeaders (dial/handshake)")
	}

	// Second request reuses the connection; TimeToHeaders must stay zero.
	if _, err := p.Request(context.Background(), []core.ModelMessage{
		core.ModelRequest{Parts: []core.ModelRequestPart{core.UserPromptPart{Content: "two"}}},
	}, nil, nil); err != nil {
		t.Fatalf("second Request: %v", err)
	}
	second, _ := snapshot()
	if !second.WebSocketConnectionReused {
		t.Error("second request should report a reused connection")
	}
	if second.TimeToHeaders != 0 {
		t.Errorf("reused connection should leave TimeToHeaders zero, got %v", second.TimeToHeaders)
	}
}

// TestRequestObserver_ResponsesStreamLabeledHTTP verifies that when a provider
// is configured for websocket, RequestStream still labels the trace transport
// as http (it always uses HTTP SSE).
func TestRequestObserver_ResponsesStreamLabeledHTTP(t *testing.T) {
	sse := "data: {\"type\":\"response.output_text.delta\",\"delta\":\"ok\"}\n\n" +
		"data: {\"type\":\"response.completed\",\"response\":{\"id\":\"r\",\"model\":\"gpt-5.3-codex\",\"output\":[],\"usage\":{\"input_tokens\":3,\"output_tokens\":1}}}\n\n" +
		"data: [DONE]\n\n"
	server, _ := chatgptSSEHandler(t, sse, nil)
	defer server.Close()

	obs, snapshot := captureObserver()
	p := New(
		WithChatGPTAuth("tok", "acct"),
		WithBaseURL(server.URL+"/chatgpt.com"),
		WithModel("gpt-5.3-codex"),
		WithTransport("websocket"), // configured for WS, but streaming uses HTTP
		WithRequestObserver(obs),
	)
	stream, err := p.RequestStream(context.Background(), []core.ModelMessage{
		core.ModelRequest{Parts: []core.ModelRequestPart{core.UserPromptPart{Content: "hi"}}},
	}, nil, nil)
	if err != nil {
		t.Fatalf("RequestStream: %v", err)
	}
	for {
		if _, err := stream.Next(); err != nil {
			break
		}
	}
	_ = stream.Close()

	tr, _ := snapshot()
	if tr.Transport != transportHTTP {
		t.Errorf("streaming transport = %q, want %q (HTTP SSE even when WS configured)", tr.Transport, transportHTTP)
	}
}

// TestRequestObserver_ResponsesStreamFinishesOnSetupError verifies the observer
// fires (and classifies) when RequestStream setup fails, e.g. a 429.
func TestRequestObserver_ResponsesStreamFinishesOnSetupError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Retry-After", "2")
		http.Error(w, `{"error":{"type":"rate_limit","message":"too many requests"}}`, http.StatusTooManyRequests)
	}))
	defer server.Close()

	obs, snapshot := captureObserver()
	p := New(
		WithChatGPTAuth("tok", "acct"),
		WithBaseURL(server.URL+"/chatgpt.com"),
		WithModel("gpt-5.3-codex"),
		WithRequestObserver(obs),
	)
	_, err := p.RequestStream(context.Background(), []core.ModelMessage{
		core.ModelRequest{Parts: []core.ModelRequestPart{core.UserPromptPart{Content: "hi"}}},
	}, nil, nil)
	if err == nil {
		t.Fatal("expected error from 429")
	}
	tr, ok := snapshot()
	if !ok {
		t.Fatal("observer should fire even when stream setup fails")
	}
	if tr.HTTPStatus != http.StatusTooManyRequests {
		t.Errorf("HTTPStatus = %d, want 429", tr.HTTPStatus)
	}
	if tr.ErrorClassification == "" {
		t.Error("ErrorClassification should be populated")
	}
}

// TestRequestObserver_ChatCompletionsCountsBuiltMessages verifies InputItems
// reflects the number of built API messages, not the input slice length.
func TestRequestObserver_ChatCompletionsCountsBuiltMessages(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(apiResponse{
			ID:    "r",
			Model: "gpt-4o",
			Choices: []apiChoice{{
				Message: apiChatMsg{Role: "assistant", Content: "ok"},
			}},
		})
	}))
	defer server.Close()

	obs, snapshot := captureObserver()
	p := New(
		WithAPIKey("key"),
		WithBaseURL(server.URL),
		WithModel("gpt-4o"),
		WithRequestObserver(obs),
	)
	// A single ModelRequest carrying system + user parts expands into multiple
	// chat-completions messages.
	_, err := p.Request(context.Background(), []core.ModelMessage{
		core.ModelRequest{Parts: []core.ModelRequestPart{
			core.SystemPromptPart{Content: "be brief"},
			core.UserPromptPart{Content: "hi"},
		}},
	}, nil, nil)
	if err != nil {
		t.Fatalf("Request: %v", err)
	}
	tr, _ := snapshot()
	if tr.InputItems < 2 {
		t.Errorf("InputItems = %d, want >= 2 (built messages), not 1 (input slice)", tr.InputItems)
	}
}

// TestRequestInstrumentationResetForRetry verifies retry reset clears transient
// phases so a retried request's trace reflects only the retry attempt.
func TestRequestInstrumentationResetForRetry(t *testing.T) {
	ri := &requestInstrumentation{
		start: time.Now(),
		trace: RequestTrace{Transport: transportWebSocket, Model: "gpt-5"},
	}
	ri.recordHeaders(500)
	ri.recordFirstEvent()
	ri.recordFirstToken()
	ri.recordTerminalFailure("previous_response_not_found")
	ri.recordError(&core.ModelHTTPError{StatusCode: 400, Body: "previous_response_not_found"})
	ri.markWebSocketReused(true)

	ri.resetForRetry()

	if ri.trace.TimeToHeaders != 0 || ri.trace.TimeToFirstEvent != 0 || ri.trace.TimeToFirstToken != 0 || ri.trace.TimeToTerminal != 0 {
		t.Error("reset should zero all phase timings")
	}
	if ri.trace.HTTPStatus != 0 || ri.trace.ErrorClassification != "" || ri.trace.ErrorClass != "" {
		t.Error("reset should clear error fields")
	}
	if ri.trace.WebSocketConnectionReused {
		t.Error("reset should clear the reuse flag (retry dials fresh)")
	}
	// Guards must allow re-recording after reset.
	ri.recordHeaders(200)
	if ri.trace.TimeToHeaders <= 0 || ri.trace.HTTPStatus != 200 {
		t.Error("recordHeaders should work again after reset")
	}
}

// TestRequestInstrumentationRecordTerminalFailure verifies terminal-failure
// records both timing and classification.
func TestRequestInstrumentationRecordTerminalFailure(t *testing.T) {
	ri := &requestInstrumentation{start: time.Now(), trace: RequestTrace{}}
	ri.recordTerminalFailure("response_failed")
	if ri.trace.TimeToTerminal <= 0 {
		t.Error("terminal timing should be recorded")
	}
	if ri.trace.ErrorClassification != "response_failed" {
		t.Errorf("classification = %q, want response_failed", ri.trace.ErrorClassification)
	}
}

func TestRecordedDurationSinceIsNeverZero(t *testing.T) {
	if got := recordedDurationSince(time.Now().Add(time.Hour)); got != time.Nanosecond {
		t.Fatalf("future start duration = %v, want %v", got, time.Nanosecond)
	}
}

// TestEstimateResponsesRequestBytes is a small direct test of the helper used
// to give the WS trace a request size.
func TestEstimateResponsesRequestBytes(t *testing.T) {
	if estimateResponsesRequestBytes(nil) != 0 {
		t.Error("nil request should be 0 bytes")
	}
	req := &responsesRequest{Model: "gpt-5", Input: []map[string]any{responsesMessage("user", "hi")}}
	if n := estimateResponsesRequestBytes(req); n == 0 {
		t.Error("non-empty request should estimate > 0 bytes")
	}
}

// allTracesObserver returns a RequestObserver that collects every trace
// delivered (not just the latest), plus a snapshot function.
func allTracesObserver() (RequestObserver, func() []RequestTrace) {
	var (
		mu     sync.Mutex
		traces []RequestTrace
	)
	obs := func(t RequestTrace) {
		mu.Lock()
		defer mu.Unlock()
		traces = append(traces, t)
	}
	snapshot := func() []RequestTrace {
		mu.Lock()
		defer mu.Unlock()
		return append([]RequestTrace(nil), traces...)
	}
	return obs, snapshot
}

// TestRequestObserver_WebSocketFallbackEmitsTwoTraces verifies that when a
// websocket attempt fails and falls back to HTTP, the observer receives two
// distinct traces: one for the failed WS attempt (with its error) and one for
// the successful HTTP attempt — not a single merged trace.
func TestRequestObserver_WebSocketFallbackEmitsTwoTraces(t *testing.T) {
	var postHits int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/responses" {
			http.NotFound(w, r)
			return
		}
		if r.Method == http.MethodGet && strings.EqualFold(r.Header.Get("Upgrade"), "websocket") {
			// Refuse the upgrade so the WS attempt fails with a connection error.
			w.WriteHeader(http.StatusUpgradeRequired)
			return
		}
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		postHits++
		resp := responsesAPIResponse{
			ID:    "resp_http_fallback",
			Model: "gpt-5.3-codex",
			Output: []responsesOutputItem{{
				Type: "message", Role: "assistant",
				Content: []responsesContentItem{{Type: "output_text", Text: "ok"}},
			}},
			Usage: responsesUsage{InputTokens: 5, OutputTokens: 2},
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	obs, snapshot := allTracesObserver()
	p := New(
		WithAPIKey("test-key"),
		WithModel("gpt-5.3-codex"),
		WithBaseURL(server.URL),
		WithTransport("websocket"),
		WithWebSocketHTTPFallback(true),
		WithRequestObserver(obs),
	)
	defer p.Close()

	if _, err := p.Request(context.Background(), []core.ModelMessage{
		core.ModelRequest{Parts: []core.ModelRequestPart{core.UserPromptPart{Content: "hi"}}},
	}, nil, nil); err != nil {
		t.Fatalf("Request: %v", err)
	}
	if postHits != 1 {
		t.Fatalf("expected one HTTP fallback call, got %d", postHits)
	}

	traces := snapshot()
	if len(traces) != 2 {
		t.Fatalf("expected 2 traces (WS failure + HTTP success), got %d", len(traces))
	}
	wsTrace, httpTrace := traces[0], traces[1]
	if wsTrace.Transport != transportWebSocket {
		t.Errorf("first trace transport = %q, want %q", wsTrace.Transport, transportWebSocket)
	}
	if wsTrace.ErrorClass == "" {
		t.Error("WS failure trace should carry an error class")
	}
	if wsTrace.TotalDuration <= 0 {
		t.Error("WS failure trace should have a total duration")
	}
	if httpTrace.Transport != transportHTTP {
		t.Errorf("fallback trace transport = %q, want %q", httpTrace.Transport, transportHTTP)
	}
	if httpTrace.ErrorClass != "" {
		t.Errorf("HTTP success trace should have no error class, got %q", httpTrace.ErrorClass)
	}
	if httpTrace.TimeToTerminal <= 0 {
		t.Error("HTTP success trace should record terminal timing")
	}
}

// TestRequestObserver_WebSocketRecordsTrimmedPayload verifies that the WS
// continuation path records the trimmed delta payload shape (InputItems equals
// the delta length, not the full input length) so upload-size attribution
// reflects the bytes actually sent on the wire.
func TestRequestObserver_WebSocketRecordsTrimmedPayload(t *testing.T) {
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

	obs, snapshot := allTracesObserver()
	p := New(
		WithAPIKey("test-key"),
		WithModel("gpt-5.3-codex"),
		WithBaseURL(server.URL),
		WithTransport("websocket"),
		WithRequestObserver(obs),
	)
	defer p.Close()

	first := []core.ModelMessage{
		core.ModelRequest{Parts: []core.ModelRequestPart{core.UserPromptPart{Content: "turn one"}}},
	}
	if _, err := p.Request(context.Background(), first, nil, nil); err != nil {
		t.Fatalf("first Request: %v", err)
	}
	// Second request extends the first; the WS path should send only the delta
	// (the assistant reply + new user turn), reusing previous_response_id.
	second := []core.ModelMessage{
		core.ModelRequest{Parts: []core.ModelRequestPart{core.UserPromptPart{Content: "turn one"}}},
		core.ModelResponse{Parts: []core.ModelResponsePart{core.TextPart{Content: "ok"}}},
		core.ModelRequest{Parts: []core.ModelRequestPart{core.UserPromptPart{Content: "turn two"}}},
	}
	if _, err := p.Request(context.Background(), second, nil, nil); err != nil {
		t.Fatalf("second Request: %v", err)
	}

	traces := snapshot()
	if len(traces) != 2 {
		t.Fatalf("expected 2 traces, got %d", len(traces))
	}
	tr1, tr2 := traces[0], traces[1]
	// First request sends the full input (1 item).
	if tr1.InputItems != 1 {
		t.Errorf("first trace InputItems = %d, want 1 (full)", tr1.InputItems)
	}
	if tr1.PreviousResponseIDReused {
		t.Error("first request should not reuse previous_response_id")
	}
	// Second request sends only the trimmed delta. trimContinuationDelta strips
	// leading assistant-generated items (the prior assistant reply is already
	// embodied by previous_response_id), leaving just the new user turn (1
	// item), not the full 3-item input. This proves the trace reflects the wire
	// payload after continuation trimming.
	if !tr2.PreviousResponseIDReused {
		t.Error("second request should reuse previous_response_id")
	}
	if tr2.InputItems != 1 {
		t.Errorf("second trace InputItems = %d, want 1 (trimmed new user turn, not 3)", tr2.InputItems)
	}
	// The trimmed payload should be smaller than the full second request.
	fullSecond := &responsesRequest{Model: "gpt-5.3-codex", Input: []map[string]any{
		responsesMessage("user", "turn one"),
		responsesMessage("assistant", "ok"),
		responsesMessage("user", "turn two"),
	}}
	if fullBytes := estimateResponsesRequestBytes(fullSecond); tr2.RequestBytes >= fullBytes {
		t.Errorf("trimmed RequestBytes = %d should be < full = %d", tr2.RequestBytes, fullBytes)
	}
}
