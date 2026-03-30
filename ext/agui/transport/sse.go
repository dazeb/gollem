package transport

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/fugue-labs/gollem/core"
	"github.com/fugue-labs/gollem/ext/agui"
)

const rawAGUIEventType = "agui.raw"

// SSEOption customizes the AGUI SSE transport.
type SSEOption func(*sseConfig)

type sseConfig struct {
	replayCapacity int
	now            func() time.Time
}

// WithReplayCapacity sets the in-memory replay buffer size.
func WithReplayCapacity(capacity int) SSEOption {
	return func(cfg *sseConfig) {
		cfg.replayCapacity = capacity
	}
}

// SSEHandler exposes AG-UI adapter output over Server-Sent Events.
//
// The adapter emits raw AG-UI protocol JSON. This transport adopts each raw
// payload into the session-owned normalized replay log, uses Event.Sequence as
// the SSE id, and writes the original raw AG-UI JSON as the SSE data body.
type SSEHandler struct {
	state *streamState
}

// NewSSEHandler creates an SSE handler wired to a single adapter/event-bus/session.
// The adapter subscribes to the bus once at construction time; reconnecting HTTP
// clients only add/remove per-request listeners and do not duplicate live
// runtime subscriptions.
func NewSSEHandler(bus *core.EventBus, adapter *agui.Adapter, session *agui.Session, opts ...SSEOption) *SSEHandler {
	cfg := sseConfig{
		replayCapacity: 10000,
		now:            time.Now,
	}
	for _, opt := range opts {
		if opt != nil {
			opt(&cfg)
		}
	}
	return &SSEHandler{state: newStreamState(bus, adapter, session, cfg)}
}

// SSE is a convenience alias for NewSSEHandler.
func SSE(bus *core.EventBus, adapter *agui.Adapter, session *agui.Session, opts ...SSEOption) http.Handler {
	return NewSSEHandler(bus, adapter, session, opts...)
}

// ServeHTTP implements http.Handler.
func (h *SSEHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	h.state.serveHTTP(w, r)
}

type streamState struct {
	mu sync.Mutex

	session *agui.Session
	buffer  *agui.EventBuffer
	now     func() time.Time

	nextListenerID uint64
	listeners      map[uint64]*liveListener
}

type liveListener struct {
	ch   chan agui.Event
	done chan struct{}
}

func newStreamState(bus *core.EventBus, adapter *agui.Adapter, session *agui.Session, cfg sseConfig) *streamState {
	if bus == nil {
		panic("agui/transport: event bus is required")
	}
	if adapter == nil {
		panic("agui/transport: adapter is required")
	}
	if session == nil {
		session = agui.NewSession(agui.SessionModeCoreStream)
	}

	buffer := session.EnsureReplayBuffer(cfg.replayCapacity)
	state := &streamState{
		session:   session,
		buffer:    buffer,
		now:       cfg.now,
		listeners: make(map[uint64]*liveListener),
	}
	adapter.OnEvent(state.capture)
	adapter.SubscribeTo(bus)
	return state
}

func (s *streamState) capture(data json.RawMessage) {
	s.mu.Lock()
	ev := s.session.CaptureRawEvent(rawAGUIEventType, data, s.now())
	listeners := make([]*liveListener, 0, len(s.listeners))
	for _, listener := range s.listeners {
		listeners = append(listeners, listener)
	}
	s.mu.Unlock()

	for _, listener := range listeners {
		select {
		case listener.ch <- ev:
		case <-listener.done:
		default:
			// Keep the runtime non-blocking for slow clients. Missed live frames are
			// recoverable through Last-Event-ID replay on reconnect.
		}
	}
}

func (s *streamState) serveHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	lastSeq, err := parseResumeCursor(r)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming not supported", http.StatusInternalServerError)
		return
	}

	listenerID, live, highWater, replay, snapshot := s.attachAndPrepareReplay(lastSeq)
	defer s.removeListener(listenerID)

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.WriteHeader(http.StatusOK)
	flusher.Flush()

	if snapshot != nil {
		if err := writeSSE(w, *snapshot); err != nil {
			return
		}
		flusher.Flush()
		highWater = snapshot.Sequence
	}

	for _, ev := range replay {
		if err := writeSSE(w, ev); err != nil {
			return
		}
		flusher.Flush()
	}

	if err := s.drainQueued(w, r, live, highWater, flusher); err != nil {
		return
	}

	for {
		select {
		case <-r.Context().Done():
			return
		case ev := <-live:
			if ev.Sequence <= highWater {
				continue
			}
			if err := writeSSE(w, ev); err != nil {
				return
			}
			flusher.Flush()
		}
	}
}

func (s *streamState) attachAndPrepareReplay(lastSeq uint64) (uint64, <-chan agui.Event, uint64, []agui.Event, *agui.Event) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.nextListenerID++
	listenerID := s.nextListenerID
	listener := &liveListener{
		ch:   make(chan agui.Event, 256),
		done: make(chan struct{}),
	}
	s.listeners[listenerID] = listener

	highWater, replay, snapshot := s.session.PrepareReconnect(lastSeq, s.now())
	if snapshot != nil {
		return listenerID, listener.ch, highWater, nil, snapshot
	}

	trimmed := make([]agui.Event, 0, len(replay))
	for _, ev := range replay {
		if ev.Sequence <= highWater {
			trimmed = append(trimmed, ev)
		}
	}
	return listenerID, listener.ch, highWater, trimmed, nil
}

func (s *streamState) removeListener(listenerID uint64) {
	s.mu.Lock()
	defer s.mu.Unlock()

	listener, ok := s.listeners[listenerID]
	if !ok {
		return
	}
	delete(s.listeners, listenerID)
	close(listener.done)
}

func (s *streamState) drainQueued(w http.ResponseWriter, r *http.Request, live <-chan agui.Event, minSeq uint64, flusher http.Flusher) error {
	for {
		select {
		case <-r.Context().Done():
			return r.Context().Err()
		case ev := <-live:
			if ev.Sequence <= minSeq {
				continue
			}
			if err := writeSSE(w, ev); err != nil {
				return err
			}
			flusher.Flush()
		default:
			return nil
		}
	}
}

func parseResumeCursor(r *http.Request) (uint64, error) {
	if raw := strings.TrimSpace(r.Header.Get("Last-Event-ID")); raw != "" {
		seq, err := strconv.ParseUint(raw, 10, 64)
		if err != nil {
			return 0, fmt.Errorf("invalid Last-Event-ID: %w", err)
		}
		return seq, nil
	}
	if raw := strings.TrimSpace(r.URL.Query().Get("last_seq")); raw != "" {
		seq, err := strconv.ParseUint(raw, 10, 64)
		if err != nil {
			return 0, fmt.Errorf("invalid last_seq: %w", err)
		}
		return seq, nil
	}
	return 0, nil
}

func writeSSE(w http.ResponseWriter, ev agui.Event) error {
	payload, err := eventPayload(ev)
	if err != nil {
		return err
	}
	_, err = fmt.Fprintf(w, "id: %d\ndata: %s\n\n", ev.Sequence, payload)
	return err
}

func eventPayload(ev agui.Event) ([]byte, error) {
	if ev.Type == rawAGUIEventType {
		if len(ev.Data) == 0 {
			return []byte("{}"), nil
		}
		return ev.Data, nil
	}
	return json.Marshal(ev)
}
