package anthropic

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/fugue-labs/gollem/core"
)

func captureRequestObserver() (RequestObserver, func() (RequestTrace, bool)) {
	var (
		mu    sync.Mutex
		trace RequestTrace
		got   bool
	)
	return func(value RequestTrace) {
			mu.Lock()
			defer mu.Unlock()
			trace = value
			got = true
		}, func() (RequestTrace, bool) {
			mu.Lock()
			defer mu.Unlock()
			return trace, got
		}
}

func TestRequestObserverCapturesStreamingCacheUsage(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = io.WriteString(w, `event: message_start
data: {"type":"message_start","message":{"usage":{"input_tokens":10,"cache_creation_input_tokens":20,"cache_read_input_tokens":30}}}

event: content_block_start
data: {"type":"content_block_start","index":0,"content_block":{"type":"text","text":""}}

event: content_block_delta
data: {"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"trace"}}

event: content_block_stop
data: {"type":"content_block_stop","index":0}

event: message_delta
data: {"type":"message_delta","delta":{"stop_reason":"end_turn"},"usage":{"output_tokens":1}}

event: message_stop
data: {"type":"message_stop"}

`)
	}))
	defer server.Close()

	observer, snapshot := captureRequestObserver()
	provider := New(WithAPIKey("secret-must-not-leak"), WithBaseURL(server.URL), WithRequestObserver(observer))
	stream, err := provider.RequestStream(context.Background(), []core.ModelMessage{
		core.ModelRequest{Parts: []core.ModelRequestPart{core.UserPromptPart{Content: "private prompt"}}},
	}, nil, &core.ModelRequestParameters{AllowTextOutput: true})
	if err != nil {
		t.Fatalf("RequestStream: %v", err)
	}
	defer stream.Close()
	for {
		_, err := stream.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatalf("stream: %v", err)
		}
	}

	trace, ok := snapshot()
	if !ok {
		t.Fatal("request observer did not fire")
	}
	if trace.Transport != "http" || trace.Model != ClaudeSonnet46 || trace.HTTPStatus != http.StatusOK {
		t.Fatalf("trace identity = %#v", trace)
	}
	if trace.TotalDuration <= 0 || trace.RequestBytes == 0 || trace.InputItems != 1 {
		t.Fatalf("trace request shape = %#v", trace)
	}
	if trace.TimeToHeaders <= 0 || trace.TimeToFirstEvent <= 0 || trace.TimeToFirstToken <= 0 || trace.TimeToTerminal <= 0 {
		t.Fatalf("trace phases = %#v", trace)
	}
	if trace.CacheReadTokens != 30 || trace.CacheWriteTokens != 20 {
		t.Fatalf("trace cache usage = %#v", trace)
	}
	rendered := formatRequestTrace(trace)
	if strings.Contains(rendered, "secret-must-not-leak") || strings.Contains(rendered, "private prompt") || strings.Contains(rendered, server.URL) {
		t.Fatalf("trace renderer leaked sensitive request data: %q", rendered)
	}
}

func TestRequestObserverRedactsHTTPErrorAndRetryAfter(t *testing.T) {
	const secret = "anthropic-provider-body-must-not-leak"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Retry-After", "7")
		w.WriteHeader(http.StatusTooManyRequests)
		_, _ = io.WriteString(w, `{"error":{"type":"rate_limit_error","message":"`+secret+`"}}`)
	}))
	defer server.Close()

	observer, snapshot := captureRequestObserver()
	provider := New(WithAPIKey("secret-must-not-leak"), WithBaseURL(server.URL), WithRequestObserver(observer))
	if _, err := provider.Request(context.Background(), nil, nil, &core.ModelRequestParameters{}); err == nil {
		t.Fatal("Request unexpectedly succeeded")
	}
	trace, ok := snapshot()
	if !ok {
		t.Fatal("request observer did not fire")
	}
	if trace.HTTPStatus != http.StatusTooManyRequests || trace.ErrorClassification != "rate_limited" || trace.ErrorClass != "http" || trace.RetryAfter != 7*time.Second {
		t.Fatalf("HTTP error trace = %#v", trace)
	}
	rendered := formatRequestTrace(trace)
	if strings.Contains(rendered, secret) || strings.Contains(rendered, "secret-must-not-leak") || strings.Contains(rendered, server.URL) {
		t.Fatalf("trace renderer leaked sensitive error data: %q", rendered)
	}
}

func TestRequestObserverClassifiesCancellation(t *testing.T) {
	started := make(chan struct{})
	release := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, _ *http.Request) {
		close(started)
		<-release
	}))
	defer server.Close()

	observer, snapshot := captureRequestObserver()
	provider := New(WithAPIKey("test-key"), WithBaseURL(server.URL), WithRequestObserver(observer))
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	result := make(chan error, 1)
	go func() {
		_, err := provider.Request(ctx, nil, nil, &core.ModelRequestParameters{})
		result <- err
	}()
	select {
	case <-started:
		cancel()
	case <-time.After(time.Second):
		t.Fatal("request did not start")
	}
	if err := <-result; err == nil {
		close(release)
		t.Fatal("Request unexpectedly succeeded")
	}
	close(release)
	trace, ok := snapshot()
	if !ok || trace.ErrorClass != "context" || trace.HTTPStatus != 0 {
		t.Fatalf("cancellation trace = %#v, observer=%t", trace, ok)
	}
}

func TestRequestTraceEnvironmentEnablesDefaultObserver(t *testing.T) {
	t.Setenv("ANTHROPIC_REQUEST_TRACE", "true")
	provider := New(WithAPIKey("test-key"))
	if provider.requestObserver == nil {
		t.Fatal("ANTHROPIC_REQUEST_TRACE did not enable the default observer")
	}
}
