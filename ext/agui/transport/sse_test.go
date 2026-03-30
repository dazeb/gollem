package transport

import (
	"bufio"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/fugue-labs/gollem/core"
	"github.com/fugue-labs/gollem/ext/agui"
)

func TestSSEHandlerSetsStreamingHeadersAndFrames(t *testing.T) {
	bus := core.NewEventBus()
	adapter := agui.NewAdapter("thread_1")
	defer adapter.Close()

	h := NewSSEHandler(bus, adapter, agui.NewSession(agui.SessionModeCoreStream))
	server := httptest.NewServer(h)
	defer server.Close()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	resp := mustOpenSSE(t, ctx, server.URL, "")
	defer resp.Body.Close()

	if got := resp.Header.Get("Content-Type"); got != "text/event-stream" {
		t.Fatalf("Content-Type = %q, want text/event-stream", got)
	}
	if got := resp.Header.Get("Cache-Control"); got != "no-cache" {
		t.Fatalf("Cache-Control = %q, want no-cache", got)
	}
	if got := resp.Header.Get("Connection"); got != "keep-alive" {
		t.Fatalf("Connection = %q, want keep-alive", got)
	}

	core.Publish(bus, core.RunStartedEvent{RunID: "run_1", StartedAt: time.Now()})
	frame := readSSEFrame(t, resp.Body)
	if frame.id != "1" {
		t.Fatalf("first frame id = %q, want 1", frame.id)
	}
	if !strings.Contains(frame.data, `"type":"RUN_STARTED"`) {
		t.Fatalf("unexpected first frame data: %s", frame.data)
	}
}

func TestSSEHandlerReconnectReplaysFromLastEventID(t *testing.T) {
	bus := core.NewEventBus()
	adapter := agui.NewAdapter("thread_1")
	defer adapter.Close()

	h := NewSSEHandler(bus, adapter, agui.NewSession(agui.SessionModeCoreStream))
	server := httptest.NewServer(h)
	defer server.Close()

	core.Publish(bus, core.RunStartedEvent{RunID: "run_1", StartedAt: time.Now()})
	core.Publish(bus, core.TurnStartedEvent{RunID: "run_1", TurnNumber: 1, StartedAt: time.Now()})
	eventually(t, func() bool { return h.state.buffer.LastSeq() >= 2 })

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	resp := mustOpenSSE(t, ctx, server.URL, "1")
	defer resp.Body.Close()

	frame := readSSEFrame(t, resp.Body)
	if frame.id != "2" {
		t.Fatalf("replayed frame id = %q, want 2", frame.id)
	}
	if !strings.Contains(frame.data, `"type":"STEP_STARTED"`) {
		t.Fatalf("unexpected replay frame data: %s", frame.data)
	}
}

func TestSSEHandlerGapFallsBackToSnapshot(t *testing.T) {
	bus := core.NewEventBus()
	adapter := agui.NewAdapter("thread_1")
	defer adapter.Close()

	h := NewSSEHandler(bus, adapter, agui.NewSession(agui.SessionModeCoreStream), WithReplayCapacity(3))
	server := httptest.NewServer(h)
	defer server.Close()

	for range 6 {
		core.Publish(bus, core.RunStartedEvent{RunID: "run_1", StartedAt: time.Now()})
	}
	eventually(t, func() bool { return h.state.buffer.LastSeq() >= 6 })

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	resp := mustOpenSSE(t, ctx, server.URL, "1")
	defer resp.Body.Close()

	frame := readSSEFrame(t, resp.Body)
	var ev agui.Event
	if err := json.Unmarshal([]byte(frame.data), &ev); err != nil {
		t.Fatalf("unmarshal snapshot event: %v", err)
	}
	if ev.Type != agui.EventSessionSnapshot {
		t.Fatalf("first frame type = %q, want %q", ev.Type, agui.EventSessionSnapshot)
	}
	if frame.id != "6" {
		t.Fatalf("snapshot frame id = %q, want 6", frame.id)
	}
}

func TestSSEHandlerDisconnectCleansUpListener(t *testing.T) {
	bus := core.NewEventBus()
	adapter := agui.NewAdapter("thread_1")
	defer adapter.Close()

	h := NewSSEHandler(bus, adapter, agui.NewSession(agui.SessionModeCoreStream))
	server := httptest.NewServer(h)
	defer server.Close()

	ctx, cancel := context.WithCancel(context.Background())
	resp := mustOpenSSE(t, ctx, server.URL, "")

	eventually(t, func() bool {
		h.state.mu.Lock()
		defer h.state.mu.Unlock()
		return len(h.state.listeners) == 1
	})

	cancel()
	resp.Body.Close()
	eventually(t, func() bool {
		h.state.mu.Lock()
		defer h.state.mu.Unlock()
		return len(h.state.listeners) == 0
	})
}

type sseFrame struct {
	id   string
	data string
}

func mustOpenSSE(t *testing.T, ctx context.Context, url, lastEventID string) *http.Response {
	t.Helper()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	if lastEventID != "" {
		req.Header.Set("Last-Event-ID", lastEventID)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("open SSE stream: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		t.Fatalf("status = %d, body=%s", resp.StatusCode, string(body))
	}
	return resp
}

func readSSEFrame(t *testing.T, body io.Reader) sseFrame {
	t.Helper()
	scanner := bufio.NewScanner(body)
	var frame sseFrame
	for scanner.Scan() {
		line := scanner.Text()
		if line == "" {
			if frame.id != "" || frame.data != "" {
				return frame
			}
			continue
		}
		if strings.HasPrefix(line, "id: ") {
			frame.id = strings.TrimPrefix(line, "id: ")
			continue
		}
		if strings.HasPrefix(line, "data: ") {
			frame.data = strings.TrimPrefix(line, "data: ")
		}
	}
	if err := scanner.Err(); err != nil {
		t.Fatalf("read SSE frame: %v", err)
	}
	t.Fatal("no SSE frame received")
	return sseFrame{}
}

func eventually(t *testing.T, fn func() bool) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if fn() {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("condition not met before timeout")
}
