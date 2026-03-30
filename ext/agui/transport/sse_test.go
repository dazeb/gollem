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
	resp := mustOpenSSE(t, newSSERequest(t, ctx, server.URL, ""))
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

	reader := newSSEStreamReader(t, resp.Body)
	frame := reader.Next()
	if frame.id != "1" {
		t.Fatalf("first frame id = %q, want 1", frame.id)
	}
	if len(frame.rawLines) == 0 || !strings.HasPrefix(frame.rawLines[0], "data: ") {
		t.Fatalf("expected SSE data framing, got lines %v", frame.rawLines)
	}
	if got := aguiTypeFromFrame(t, frame); got != agui.AGUIRunStarted {
		t.Fatalf("first frame type = %q, want %q", got, agui.AGUIRunStarted)
	}
}

func TestWriteSSEPrefixesEveryPayloadLineWithData(t *testing.T) {
	rec := httptest.NewRecorder()
	ev := agui.Event{
		Sequence: 7,
		Type:     rawAGUIEventType,
		Data:     json.RawMessage("{\n\"type\":\"RUN_STARTED\",\n\"runId\":\"run_1\"\n}"),
	}

	if err := writeSSE(rec, ev); err != nil {
		t.Fatalf("writeSSE returned error: %v", err)
	}

	got := rec.Body.String()
	if !strings.HasPrefix(got, "id: 7\n") {
		t.Fatalf("frame = %q, want id prefix", got)
	}
	for _, line := range strings.Split(strings.TrimSuffix(got, "\n\n"), "\n") {
		if strings.HasPrefix(line, "id: ") {
			continue
		}
		if !strings.HasPrefix(line, "data: ") {
			t.Fatalf("frame line %q missing data prefix in %q", line, got)
		}
	}

	frame := newSSEStreamReader(t, strings.NewReader(got)).Next()
	if frame.id != "7" {
		t.Fatalf("frame id = %q, want 7", frame.id)
	}
	if got := aguiTypeFromFrame(t, frame); got != agui.AGUIRunStarted {
		t.Fatalf("frame type = %q, want %q", got, agui.AGUIRunStarted)
	}
}

func TestSSEHandlerFreshClientReplaysBufferedEventsAndStreamsLive(t *testing.T) {
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
	resp := mustOpenSSE(t, newSSERequest(t, ctx, server.URL, ""))
	defer resp.Body.Close()

	reader := newSSEStreamReader(t, resp.Body)
	frame1 := reader.Next()
	frame2 := reader.Next()
	if frame1.id != "1" || frame2.id != "2" {
		t.Fatalf("fresh replay ids = [%s %s], want [1 2]", frame1.id, frame2.id)
	}
	if got := aguiTypeFromFrame(t, frame1); got != agui.AGUIRunStarted {
		t.Fatalf("frame1 type = %q, want %q", got, agui.AGUIRunStarted)
	}
	if got := aguiTypeFromFrame(t, frame2); got != agui.AGUIStepStarted {
		t.Fatalf("frame2 type = %q, want %q", got, agui.AGUIStepStarted)
	}

	core.Publish(bus, core.TurnCompletedEvent{RunID: "run_1", TurnNumber: 1, CompletedAt: time.Now()})
	frame3 := reader.Next()
	if frame3.id != "3" {
		t.Fatalf("live frame id = %q, want 3", frame3.id)
	}
	if got := aguiTypeFromFrame(t, frame3); got != agui.AGUIStepFinished {
		t.Fatalf("frame3 type = %q, want %q", got, agui.AGUIStepFinished)
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
	core.Publish(bus, core.TurnCompletedEvent{RunID: "run_1", TurnNumber: 1, CompletedAt: time.Now()})
	eventually(t, func() bool { return h.state.buffer.LastSeq() >= 3 })

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	resp := mustOpenSSE(t, newSSERequest(t, ctx, server.URL, "2"))
	defer resp.Body.Close()

	reader := newSSEStreamReader(t, resp.Body)
	frame := reader.Next()
	if frame.id != "3" {
		t.Fatalf("replayed frame id = %q, want 3", frame.id)
	}
	if got := aguiTypeFromFrame(t, frame); got != agui.AGUIStepFinished {
		t.Fatalf("replayed frame type = %q, want %q", got, agui.AGUIStepFinished)
	}

	core.Publish(bus, core.RunCompletedEvent{RunID: "run_1", Success: true, CompletedAt: time.Now()})
	live := reader.Next()
	if live.id != "4" {
		t.Fatalf("live frame id after resume = %q, want 4", live.id)
	}
	if got := aguiTypeFromFrame(t, live); got != agui.AGUIRunFinished {
		t.Fatalf("live frame type = %q, want %q", got, agui.AGUIRunFinished)
	}
}

func TestSSEHandlerReconnectReplaysFromLastSeqQuery(t *testing.T) {
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
	resp := mustOpenSSE(t, newSSERequest(t, ctx, server.URL+"?last_seq=1", ""))
	defer resp.Body.Close()

	frame := newSSEStreamReader(t, resp.Body).Next()
	if frame.id != "2" {
		t.Fatalf("replayed frame id = %q, want 2", frame.id)
	}
	if got := aguiTypeFromFrame(t, frame); got != agui.AGUIStepStarted {
		t.Fatalf("replayed frame type = %q, want %q", got, agui.AGUIStepStarted)
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
	resp := mustOpenSSE(t, newSSERequest(t, ctx, server.URL, "1"))
	defer resp.Body.Close()

	frame := newSSEStreamReader(t, resp.Body).Next()
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
	var payload map[string]any
	if err := json.Unmarshal(ev.Data, &payload); err != nil {
		t.Fatalf("unmarshal snapshot payload: %v", err)
	}
	if got := payload["snapshot_sequence"]; got != float64(6) {
		t.Fatalf("snapshot_sequence = %v, want 6", got)
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
	resp := mustOpenSSE(t, newSSERequest(t, ctx, server.URL, ""))

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
	id       string
	data     string
	rawLines []string
}

type sseStreamReader struct {
	t       *testing.T
	scanner *bufio.Scanner
}

func newSSEStreamReader(t *testing.T, body io.Reader) *sseStreamReader {
	t.Helper()
	scanner := bufio.NewScanner(body)
	scanner.Buffer(make([]byte, 0, 1024), 1024*1024)
	return &sseStreamReader{t: t, scanner: scanner}
}

func (r *sseStreamReader) Next() sseFrame {
	r.t.Helper()
	var frame sseFrame
	var dataLines []string
	for r.scanner.Scan() {
		line := r.scanner.Text()
		if line == "" {
			if frame.id != "" || len(dataLines) > 0 {
				frame.data = strings.Join(dataLines, "\n")
				return frame
			}
			continue
		}
		if strings.HasPrefix(line, "id: ") {
			frame.id = strings.TrimPrefix(line, "id: ")
			continue
		}
		if strings.HasPrefix(line, "data: ") {
			frame.rawLines = append(frame.rawLines, line)
			dataLines = append(dataLines, strings.TrimPrefix(line, "data: "))
		}
	}
	if err := r.scanner.Err(); err != nil {
		r.t.Fatalf("read SSE frame: %v", err)
	}
	r.t.Fatal("no SSE frame received")
	return sseFrame{}
}

func newSSERequest(t *testing.T, ctx context.Context, url, lastEventID string) *http.Request {
	t.Helper()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	if lastEventID != "" {
		req.Header.Set("Last-Event-ID", lastEventID)
	}
	return req
}

func mustOpenSSE(t *testing.T, req *http.Request) *http.Response {
	t.Helper()
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

func aguiTypeFromFrame(t *testing.T, frame sseFrame) string {
	t.Helper()
	var payload struct {
		Type string `json:"type"`
	}
	if err := json.Unmarshal([]byte(frame.data), &payload); err != nil {
		t.Fatalf("unmarshal frame data: %v\nframe=%+v", err, frame)
	}
	return payload.Type
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
