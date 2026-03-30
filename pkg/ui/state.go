package ui

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/fugue-labs/gollem/core"
	"github.com/fugue-labs/gollem/ext/agui"
	"github.com/fugue-labs/gollem/ext/agui/transport"
)

// RunStarter starts a newly-created UI run against the provided live runtime.
type RunStarter interface {
	StartRun(ctx context.Context, runtime *RunRuntime, req RunStartRequest) error
}

// RunStarterFunc adapts a function into a RunStarter.
type RunStarterFunc func(ctx context.Context, runtime *RunRuntime, req RunStartRequest) error

// StartRun implements RunStarter.
func (fn RunStarterFunc) StartRun(ctx context.Context, runtime *RunRuntime, req RunStartRequest) error {
	return fn(ctx, runtime, req)
}

// RunRuntime exposes the live state and transports owned by a UI run.
type RunRuntime struct {
	RunID          string
	EventBus       *core.EventBus
	Session        *agui.Session
	ApprovalBridge *agui.ApprovalBridge
	Adapter        *agui.Adapter
}

// ConsumeStream drains a core stream into AG-UI adapter events and returns the
// final typed result after streaming completes.
func (rt *RunRuntime) ConsumeStream(stream *core.StreamResult[string]) (*core.RunResult[string], error) {
	if stream == nil {
		return nil, errors.New("ui: nil stream")
	}
	defer stream.Close()
	if err := agui.ConsumeStream(rt.Adapter, stream.StreamEvents()); err != nil {
		return nil, err
	}
	return stream.Result()
}

// PendingApprovalView is sidebar-friendly metadata for one unresolved approval.
type PendingApprovalView struct {
	ToolCallID  string
	ToolName    string
	ArgsJSON    string
	RequestedAt time.Time
}

// RunView is the UI projection for one run.
type RunView struct {
	ID               string
	Title            string
	Status           string
	Provider         string
	Model            string
	Summary          string
	Prompt           string
	StartedAt        time.Time
	UpdatedAt        time.Time
	Events           []string
	SessionID        string
	Usage            core.RunUsage
	WaitingReason    string
	PendingApprovals []PendingApprovalView
}

// RunStartRequest is the accepted POST /runs/start payload.
type RunStartRequest struct {
	Title    string `json:"title"`
	Summary  string `json:"summary"`
	Prompt   string `json:"prompt"`
	Provider string `json:"provider"`
	Model    string `json:"model"`
}

// RunRecord owns the live runtime objects and projected UI state for one run.
type RunRecord struct {
	mu sync.RWMutex

	id      string
	title   string
	summary string
	prompt  string

	provider string
	model    string

	startedAt time.Time
	updatedAt time.Time
	status    string

	usage         core.RunUsage
	waitingReason string
	events        []string

	pendingApprovals map[string]PendingApprovalView

	bus     *core.EventBus
	session *agui.Session
	adapter *agui.Adapter
	bridge  *agui.ApprovalBridge
	sse     http.Handler
	cancel  context.CancelFunc
}

func newRunRecord(now time.Time, runID string, req RunStartRequest) *RunRecord {
	bus := core.NewEventBus()
	session := agui.NewSession(agui.SessionModeCoreStream)
	session.SetRunID(runID, "")
	adapter := agui.NewAdapter(runID)
	bridge := agui.NewApprovalBridge()

	r := &RunRecord{
		id:               runID,
		title:            firstNonEmpty(req.Title, "Run "+runID),
		summary:          firstNonEmpty(req.Summary, trimSummary(req.Prompt)),
		prompt:           strings.TrimSpace(req.Prompt),
		provider:         firstNonEmpty(req.Provider, "ui"),
		model:            firstNonEmpty(req.Model, "stream"),
		startedAt:        now,
		updatedAt:        now,
		status:           "starting",
		pendingApprovals: make(map[string]PendingApprovalView),
		bus:              bus,
		session:          session,
		adapter:          adapter,
		bridge:           bridge,
	}
	r.sse = transport.NewSSEHandler(bus, adapter, session)
	r.attachRuntimeProjection()
	return r
}

// ID returns the stable run identifier.
func (r *RunRecord) ID() string { return r.id }

// Runtime returns the live runtime handles for this run.
func (r *RunRecord) Runtime() *RunRuntime {
	return &RunRuntime{
		RunID:          r.id,
		EventBus:       r.bus,
		Session:        r.session,
		ApprovalBridge: r.bridge,
		Adapter:        r.adapter,
	}
}

// EventBus returns the per-run event bus.
func (r *RunRecord) EventBus() *core.EventBus { return r.bus }

// Session returns the AG-UI session state.
func (r *RunRecord) Session() *agui.Session { return r.session }

// ApprovalBridge returns the live approval bridge.
func (r *RunRecord) ApprovalBridge() *agui.ApprovalBridge { return r.bridge }

// CancelFunc returns the current cancel function, if set.
func (r *RunRecord) CancelFunc() context.CancelFunc {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.cancel
}

func (r *RunRecord) setCancel(cancel context.CancelFunc) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.cancel = cancel
}

func (r *RunRecord) setStatus(status string, at time.Time) {
	if at.IsZero() {
		at = time.Now().UTC()
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.status = status
	r.updatedAt = at.UTC()
}

func (r *RunRecord) failStart(err error) {
	at := time.Now().UTC()
	r.mu.Lock()
	defer r.mu.Unlock()
	r.status = "failed"
	r.updatedAt = at
	r.events = append(r.events, "run_start_failed")
	if err != nil {
		r.summary = strings.TrimSpace(err.Error())
	}
}

func (r *RunRecord) closeRuntime() {
	if r.adapter != nil {
		r.adapter.Close()
	}
}

func (r *RunRecord) sseHandler() http.Handler {
	return r.sse
}

func (r *RunRecord) actionHandler() http.Handler {
	return transport.NewActionHandler(transport.ActionHandlerConfig{
		Runtimes: map[string]*transport.SessionRuntime{
			r.session.ID: {
				Session:        r.session,
				ApprovalBridge: r.bridge,
				Cancel:         r.CancelFunc(),
			},
		},
	})
}

// Snapshot returns a race-safe read model for templates and handlers.
func (r *RunRecord) Snapshot() RunView {
	r.mu.RLock()
	defer r.mu.RUnlock()

	pending := make([]PendingApprovalView, 0, len(r.pendingApprovals))
	for _, item := range r.pendingApprovals {
		pending = append(pending, item)
	}
	sort.Slice(pending, func(i, j int) bool {
		return pending[i].RequestedAt.Before(pending[j].RequestedAt)
	})

	return RunView{
		ID:               r.id,
		Title:            r.title,
		Status:           r.status,
		Provider:         r.provider,
		Model:            r.model,
		Summary:          r.summary,
		Prompt:           r.prompt,
		StartedAt:        r.startedAt,
		UpdatedAt:        r.updatedAt,
		Events:           append([]string(nil), r.events...),
		SessionID:        r.session.ID,
		Usage:            r.usage,
		WaitingReason:    r.waitingReason,
		PendingApprovals: pending,
	}
}

func (r *RunRecord) attachRuntimeProjection() {
	core.Subscribe(r.bus, func(ev core.RunStartedEvent) {
		r.applyRuntimeEvent(ev.RuntimeEventType(), ev.RuntimeOccurredAt(), func() {
			r.status = "running"
			r.prompt = firstNonEmpty(r.prompt, ev.Prompt)
			r.session.SetRunID(ev.RunID, ev.ParentRunID)
		})
	})
	core.Subscribe(r.bus, func(ev core.RunCompletedEvent) {
		r.applyRuntimeEvent(ev.RuntimeEventType(), ev.RuntimeOccurredAt(), func() {
			r.waitingReason = ""
			switch {
			case ev.Deferred:
				r.status = "waiting"
			case ev.Success:
				r.status = "completed"
			default:
				r.status = "failed"
			}
		})
	})
	core.Subscribe(r.bus, func(ev core.TurnStartedEvent) {
		r.applyRuntimeEvent(ev.RuntimeEventType(), ev.RuntimeOccurredAt(), nil)
	})
	core.Subscribe(r.bus, func(ev core.TurnCompletedEvent) {
		r.applyRuntimeEvent(ev.RuntimeEventType(), ev.RuntimeOccurredAt(), nil)
	})
	core.Subscribe(r.bus, func(ev core.ModelRequestStartedEvent) {
		r.applyRuntimeEvent(ev.RuntimeEventType(), ev.RuntimeOccurredAt(), nil)
	})
	core.Subscribe(r.bus, func(ev core.ModelResponseCompletedEvent) {
		r.applyRuntimeEvent(ev.RuntimeEventType(), ev.RuntimeOccurredAt(), func() {
			r.usage.IncrRequest(core.Usage{InputTokens: ev.InputTokens, OutputTokens: ev.OutputTokens})
		})
	})
	core.Subscribe(r.bus, func(ev core.ToolCalledEvent) {
		r.applyRuntimeEvent(ev.RuntimeEventType(), ev.RuntimeOccurredAt(), func() {
			r.usage.IncrToolCall()
		})
	})
	core.Subscribe(r.bus, func(ev core.ToolCompletedEvent) {
		r.applyRuntimeEvent(ev.RuntimeEventType(), ev.RuntimeOccurredAt(), nil)
	})
	core.Subscribe(r.bus, func(ev core.ToolFailedEvent) {
		r.applyRuntimeEvent(ev.RuntimeEventType(), ev.RuntimeOccurredAt(), nil)
	})
	core.Subscribe(r.bus, func(ev core.ApprovalRequestedEvent) {
		r.applyRuntimeEvent(ev.RuntimeEventType(), ev.RuntimeOccurredAt(), func() {
			r.status = "waiting"
			r.waitingReason = "approval"
			r.pendingApprovals[ev.ToolCallID] = PendingApprovalView{
				ToolCallID:  ev.ToolCallID,
				ToolName:    ev.ToolName,
				ArgsJSON:    ev.ArgsJSON,
				RequestedAt: ev.RequestedAt,
			}
		})
	})
	core.Subscribe(r.bus, func(ev core.ApprovalResolvedEvent) {
		r.applyRuntimeEvent(ev.RuntimeEventType(), ev.RuntimeOccurredAt(), func() {
			delete(r.pendingApprovals, ev.ToolCallID)
			if len(r.pendingApprovals) == 0 && r.status == "waiting" && r.waitingReason == "approval" {
				r.status = "running"
				r.waitingReason = ""
			}
		})
	})
	core.Subscribe(r.bus, func(ev core.DeferredRequestedEvent) {
		r.applyRuntimeEvent(ev.RuntimeEventType(), ev.RuntimeOccurredAt(), nil)
	})
	core.Subscribe(r.bus, func(ev core.DeferredResolvedEvent) {
		r.applyRuntimeEvent(ev.RuntimeEventType(), ev.RuntimeOccurredAt(), nil)
	})
	core.Subscribe(r.bus, func(ev core.RunWaitingEvent) {
		r.applyRuntimeEvent(ev.RuntimeEventType(), ev.RuntimeOccurredAt(), func() {
			r.status = "waiting"
			r.waitingReason = ev.Reason
		})
	})
	core.Subscribe(r.bus, func(ev core.RunResumedEvent) {
		r.applyRuntimeEvent(ev.RuntimeEventType(), ev.RuntimeOccurredAt(), func() {
			r.status = "running"
			r.waitingReason = ""
		})
	})
}

func (r *RunRecord) applyRuntimeEvent(eventType string, at time.Time, mutate func()) {
	if at.IsZero() {
		at = time.Now().UTC()
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if mutate != nil {
		mutate()
	}
	r.updatedAt = at.UTC()
	r.events = append(r.events, eventType)
}

// RunStateStore is an in-memory registry of live runs.
type RunStateStore struct {
	mu     sync.RWMutex
	runs   map[string]*RunRecord
	now    func() time.Time
	nextID func() string
}

// NewRunStateStore constructs an empty run store.
func NewRunStateStore() *RunStateStore {
	return &RunStateStore{
		runs:   make(map[string]*RunRecord),
		now:    time.Now,
		nextID: newRunID,
	}
}

func (s *RunStateStore) create(req RunStartRequest) *RunRecord {
	record := newRunRecord(s.now().UTC(), s.nextID(), req)
	s.mu.Lock()
	s.runs[record.id] = record
	s.mu.Unlock()
	return record
}

func (s *RunStateStore) get(runID string) (*RunRecord, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	record, ok := s.runs[strings.TrimSpace(runID)]
	return record, ok
}

func (s *RunStateStore) listViews() []RunView {
	s.mu.RLock()
	records := make([]*RunRecord, 0, len(s.runs))
	for _, record := range s.runs {
		records = append(records, record)
	}
	s.mu.RUnlock()

	views := make([]RunView, 0, len(records))
	for _, record := range records {
		views = append(views, record.Snapshot())
	}
	sort.Slice(views, func(i, j int) bool {
		return views[i].UpdatedAt.After(views[j].UpdatedAt)
	})
	return views
}

func trimSummary(prompt string) string {
	prompt = strings.TrimSpace(prompt)
	if prompt == "" {
		return "Live UI-managed run"
	}
	const max = 96
	if len(prompt) <= max {
		return prompt
	}
	return strings.TrimSpace(prompt[:max]) + "…"
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		trimmed := strings.TrimSpace(value)
		if trimmed != "" {
			return trimmed
		}
	}
	return ""
}

func newRunID() string {
	return fmt.Sprintf("run_%d", time.Now().UTC().UnixNano())
}
