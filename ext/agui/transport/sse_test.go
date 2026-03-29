package transport

import (
	"bufio"
	"context"
	"encoding/json"
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

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/sse", nil).WithContext(ctx)
	done := make(chan struct{})
	go func() {
		defer close(done)
		h.ServeHTTP(w, req)
	}()

	core.Publish(bus, core.RunStartedEvent{RunID: "run_1", StartedAt: time.Now()})
	eventually(t, func() bool {
		return strings.Contains(w.Body.String(), "id: 1\n")
	})
	cancel()
	<-done

	resp := w.Result()
	if got := resp.Header.Get("Content-Type"); got != "text/event-stream" {
		t.Fatalf("Content-Type = %q, want text/event-stream", got)
	}
	if got := resp.Header.Get("Cache-Control"); got != "no-cache" {
		t.Fatalf("Cache-Control = %q, want no-cache", got)
	}
	if got := resp.Header.Get("Connection"); got != "keep-alive" {
		t.Fatalf("Connection = %q, want keep-alive", got)
	}

	frames := parseSSEFrames(w.Body.String())
	if len(frames) == 0 {
		t.Fatal("expected at least one SSE frame")
	}
	if frames[0].id != "1" {
		t.Fatalf("first frame id = %q, want 1", frames[0].id)
	}
	if !strings.Contains(frames[0].data, `"type":"RUN_STARTED"`) {
		t.Fatalf("unexpected first frame data: %s", frames[0].data)
	}
}

func TestSSEHandlerReconnectReplaysFromLastEventID(t *testing.T) {
	bus := core.NewEventBus()
	adapter := agui.NewAdapter("thread_1")
	defer adapter.Close()

	h := NewSSEHandler(bus, adapter, agui.NewSession(agui.SessionModeCoreStream))

	core.Publish(bus, core.RunStartedEvent{RunID: "run_1", StartedAt: time.Now()})
	core.Publish(bus, core.TurnStartedEvent{RunID: "run_1", TurnNumber: 1, StartedAt: time.Now()})
	eventually(t, func() bool { return h.state.buffer.LastSeq() >= 2 })

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/sse", nil).WithContext(ctx)
	req.Header.Set("Last-Event-ID", "1")
	done := make(chan struct{})
	go func() {
		defer close(done)
		h.ServeHTTP(w, req)
	}()

	eventually(t, func() bool {
		frames := parseSSEFrames(w.Body.String())
		return len(frames) >= 1
	})
	cancel()
	<-done

	frames := parseSSEFrames(w.Body.String())
	if len(frames) != 1 {
		t.Fatalf("expected 1 replayed frame, got %d: %#v", len(frames), frames)
	}
	if frames[0].id != "2" {
		t.Fatalf("replayed frame id = %q, want 2", frames[0].id)
	}
	if !strings.Contains(frames[0].data, `"type":"STEP_STARTED"`) {
		t.Fatalf("unexpected replay frame data: %s", frames[0].data)
	}
}

func TestSSEHandlerGapFallsBackToSnapshot(t *testing.T) {
	bus := core.NewEventBus()
	adapter := agui.NewAdapter("thread_1")
	defer adapter.Close()

	h := NewSSEHandler(bus, adapter, agui.NewSession(agui.SessionModeCoreStream), WithReplayCapacity(3))

	for i := 0; i < 6; i++ {
		core.Publish(bus, core.RunStartedEvent{RunID: "run_1", StartedAt: time.Now()})
	}
	eventually(t, func() bool { return h.state.buffer.LastSeq() >= 6 })

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/sse", nil).WithContext(ctx)
	req.Header.Set("Last-Event-ID", "1")
	done := make(chan struct{})
	go func() {
		defer close(done)
		h.ServeHTTP(w, req)
	}()

	eventually(t, func() bool {
		frames := parseSSEFrames(w.Body.String())
		return len(frames) >= 1
	})
	cancel()
	<-done

	frames := parseSSEFrames(w.Body.String())
	var ev agui.Event
	if err := json.Unmarshal([]byte(frames[0].data), &ev); err != nil {
		t.Fatalf("unmarshal snapshot event: %v", err)
	}
	if ev.Type != agui.EventSessionSnapshot {
		t.Fatalf("first frame type = %q, want %q", ev.Type, agui.EventSessionSnapshot)
	}
	if frames[0].id != "6" {
		t.Fatalf("snapshot frame id = %q, want 6", frames[0].id)
	}
}

func TestSSEHandlerDisconnectCleansUpListener(t *testing.T) {
	bus := core.NewEventBus()
	adapter := agui.NewAdapter("thread_1")
	defer adapter.Close()

	h := NewSSEHandler(bus, adapter, agui.NewSession(agui.SessionModeCoreStream))

	ctx, cancel := context.WithCancel(context.Background())
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/sse", nil).WithContext(ctx)
	done := make(chan struct{})
	go func() {
		defer close(done)
		h.ServeHTTP(w, req)
	}()

	eventually(t, func() bool {
		h.state.mu.Lock()
		defer h.state.mu.Unlock()
		return len(h.state.listeners) == 1
	})

	cancel()
	<-done

	h.state.mu.Lock()
	defer h.state.mu.Unlock()
	if got := len(h.state.listeners); got != 0 {
		t.Fatalf("listener count = %d, want 0", got)
	}
}

type sseFrame struct {
	id   string
	data string
}

func parseSSEFrames(body string) []sseFrame {
	scanner := bufio.NewScanner(strings.NewReader(body))
	var frames []sseFrame
	var current sseFrame
	for scanner.Scan() {
		line := scanner.Text()
		if line == "" {
			if current.id != "" || current.data != "" {
				frames = append(frames, current)
				current = sseFrame{}
			}
			continue
		}
		if strings.HasPrefix(line, "id: ") {
			current.id = strings.TrimPrefix(line, "id: ")
		}
		if strings.HasPrefix(line, "data: ") {
			current.data = strings.TrimPrefix(line, "data: ")
		}
	}
	if current.id != "" || current.data != "" {
		frames = append(frames, current)
	}
	return frames
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
