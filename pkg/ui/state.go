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
	"github.com/google/uuid"
)

const (
	maxStoredRunActivities = 64
	maxRecentRunActivities = 12
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
		return nil, errNilUIStream
	}
	defer stream.Close()
	if err := agui.ConsumeStream(rt.Adapter, stream.StreamEvents()); err != nil {
		return nil, err
	}
	return stream.Result()
}

var errNilUIStream = errors.New("ui: nil stream")

// PendingApprovalView is sidebar-friendly metadata for one unresolved approval.
type PendingApprovalView struct {
	ToolCallID  string
	ToolName    string
	ArgsJSON    string
	RequestedAt time.Time
}

// RunActivityView is a human-readable projection of one recent runtime event.
type RunActivityView struct {
	Type        string
	Label       string
	Detail      string
	OccurredAt  time.Time
	IsWaiting   bool
	IsError     bool
	ToolCallID  string
	ToolName    string
	TurnNumber  int
	FinishState string
}

// RunWaitingView is renderer-friendly waiting context for the current run state.
type RunWaitingView struct {
	Active      bool
	Reason      string
	Label       string
	Detail      string
	PendingKind string
}

// RunControlsView exposes control-oriented metadata for templates and renderers.
type RunControlsView struct {
	CanAbort             bool
	CanApproveTools      bool
	PendingApprovalCount int
	HasRecentActivity    bool
	LastEventLabel       string
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
	RecentActivity   []RunActivityView
	SessionID        string
	Usage            core.RunUsage
	WaitingReason    string
	Waiting          RunWaitingView
	PendingApprovals []PendingApprovalView
	Controls         RunControlsView
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
	activities    []RunActivityView

	pendingApprovals map[string]PendingApprovalView

	bus     *core.EventBus
	session *agui.Session
	adapter *agui.Adapter
	bridge  *agui.ApprovalBridge
	sse     http.Handler
	action  http.Handler
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
	r.action = transport.NewActionHandler(transport.ActionHandlerConfig{Sessions: r})
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

// Get resolves this run's transport runtime by session ID.
func (r *RunRecord) Get(sessionID string) (*transport.SessionRuntime, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	if r.session == nil || strings.TrimSpace(sessionID) != r.session.ID {
		return nil, false
	}
	return &transport.SessionRuntime{
		Session:        r.session,
		ApprovalBridge: r.bridge,
		Cancel:         r.cancel,
	}, true
}

func (r *RunRecord) setCancel(cancel context.CancelFunc) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.cancel = cancel
}

func (r *RunRecord) setStatusLocked(status string, at time.Time) {
	r.status = status
	r.updatedAt = at.UTC()
}

func (r *RunRecord) appendActivityLocked(activity RunActivityView) {
	activity.Type = strings.TrimSpace(activity.Type)
	if activity.Type == "" {
		return
	}
	if activity.OccurredAt.IsZero() {
		activity.OccurredAt = r.updatedAt
	}
	if activity.OccurredAt.IsZero() {
		activity.OccurredAt = time.Now().UTC()
	}
	activity.OccurredAt = activity.OccurredAt.UTC()
	activity.Label = firstNonEmpty(activity.Label, humanizeRuntimeEventType(activity.Type))
	activity.Detail = strings.TrimSpace(activity.Detail)
	r.activities = append(r.activities, activity)
	if len(r.activities) > maxStoredRunActivities {
		start := len(r.activities) - maxStoredRunActivities
		r.activities = append([]RunActivityView(nil), r.activities[start:]...)
	}
}

func (r *RunRecord) hasRuntimeEvent(eventType string) bool {
	r.mu.RLock()
	defer r.mu.RUnlock()
	for _, seen := range r.events {
		if seen == eventType {
			return true
		}
	}
	return false
}

func (r *RunRecord) markAborted(at time.Time) {
	if at.IsZero() {
		at = time.Now().UTC()
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.setStatusLocked("aborted", at)
	r.session.SetStatus(agui.SessionStatusAborted)
	if r.waitingReason == "" && len(r.pendingApprovals) > 0 {
		r.waitingReason = "approval"
	}
	r.updatedAt = at.UTC()
	r.events = append(r.events, "run_aborted")
	r.appendActivityLocked(RunActivityView{
		Type:        "run_aborted",
		Label:       "Run aborted",
		Detail:      "Execution stopped before completion.",
		OccurredAt:  at,
		IsError:     true,
		FinishState: "aborted",
	})
}

func (r *RunRecord) failStart(err error) {
	at := time.Now().UTC()
	r.mu.Lock()
	defer r.mu.Unlock()
	r.status = "failed"
	r.updatedAt = at
	r.session.SetStatus(agui.SessionStatusFailed)
	r.events = append(r.events, "run_start_failed")
	if err != nil {
		r.summary = strings.TrimSpace(err.Error())
	}
	r.appendActivityLocked(RunActivityView{
		Type:        "run_start_failed",
		Label:       "Run failed to start",
		Detail:      summarizeInline(errString(err, "starter returned an error"), 120),
		OccurredAt:  at,
		IsError:     true,
		FinishState: "failed",
	})
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
	return r.action
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

	recent := snapshotRecentActivities(r.activities)
	waiting := buildWaitingView(r.status, r.waitingReason, len(pending))
	lastLabel := waiting.Label
	if len(recent) > 0 {
		lastLabel = recent[len(recent)-1].Label
	}

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
		RecentActivity:   recent,
		SessionID:        r.session.ID,
		Usage:            r.usage,
		WaitingReason:    r.waitingReason,
		Waiting:          waiting,
		PendingApprovals: pending,
		Controls: RunControlsView{
			CanAbort:             statusAllowsAbort(r.status),
			CanApproveTools:      len(pending) > 0,
			PendingApprovalCount: len(pending),
			HasRecentActivity:    len(recent) > 0,
			LastEventLabel:       firstNonEmpty(lastLabel, humanizeRunStatus(r.status)),
		},
	}
}

func (r *RunRecord) attachRuntimeProjection() {
	core.Subscribe(r.bus, func(ev core.RunStartedEvent) {
		r.applyRuntimeEvent(ev.RuntimeEventType(), ev.RuntimeOccurredAt(), func() {
			r.setStatusLocked("running", ev.RuntimeOccurredAt())
			r.prompt = firstNonEmpty(r.prompt, ev.Prompt)
			r.session.SetRunID(ev.RunID, ev.ParentRunID)
			r.session.SetStatus(agui.SessionStatusRunning)
		}, func() RunActivityView {
			return activityForRunStarted(ev)
		})
	})
	core.Subscribe(r.bus, func(ev core.RunCompletedEvent) {
		r.applyRuntimeEvent(ev.RuntimeEventType(), ev.RuntimeOccurredAt(), func() {
			switch {
			case ev.Deferred:
				r.setStatusLocked("waiting", ev.RuntimeOccurredAt())
				if strings.TrimSpace(r.waitingReason) == "" {
					r.waitingReason = "deferred"
				}
				r.session.SetWaiting(r.waitingReason)
			case ev.Success:
				r.setStatusLocked("completed", ev.RuntimeOccurredAt())
				r.waitingReason = ""
				r.session.SetStatus(agui.SessionStatusCompleted)
			case isCanceledRunError(ev.Error):
				r.setStatusLocked("aborted", ev.RuntimeOccurredAt())
				r.session.SetStatus(agui.SessionStatusAborted)
			default:
				r.setStatusLocked("failed", ev.RuntimeOccurredAt())
				r.waitingReason = ""
				r.session.SetStatus(agui.SessionStatusFailed)
			}
		}, func() RunActivityView {
			return activityForRunCompleted(ev, r.waitingReason)
		})
	})
	core.Subscribe(r.bus, func(ev core.TurnStartedEvent) {
		r.applyRuntimeEvent(ev.RuntimeEventType(), ev.RuntimeOccurredAt(), nil, func() RunActivityView {
			return activityForTurnStarted(ev)
		})
	})
	core.Subscribe(r.bus, func(ev core.TurnCompletedEvent) {
		r.applyRuntimeEvent(ev.RuntimeEventType(), ev.RuntimeOccurredAt(), nil, func() RunActivityView {
			return activityForTurnCompleted(ev)
		})
	})
	core.Subscribe(r.bus, func(ev core.ModelRequestStartedEvent) {
		r.applyRuntimeEvent(ev.RuntimeEventType(), ev.RuntimeOccurredAt(), nil, func() RunActivityView {
			return activityForModelRequestStarted(ev)
		})
	})
	core.Subscribe(r.bus, func(ev core.ModelResponseCompletedEvent) {
		r.applyRuntimeEvent(ev.RuntimeEventType(), ev.RuntimeOccurredAt(), func() {
			r.usage.IncrRequest(core.Usage{InputTokens: ev.InputTokens, OutputTokens: ev.OutputTokens})
		}, func() RunActivityView {
			return activityForModelResponseCompleted(ev)
		})
	})
	core.Subscribe(r.bus, func(ev core.ToolCalledEvent) {
		r.applyRuntimeEvent(ev.RuntimeEventType(), ev.RuntimeOccurredAt(), func() {
			r.usage.IncrToolCall()
		}, func() RunActivityView {
			return activityForToolCalled(ev)
		})
	})
	core.Subscribe(r.bus, func(ev core.ToolCompletedEvent) {
		r.applyRuntimeEvent(ev.RuntimeEventType(), ev.RuntimeOccurredAt(), nil, func() RunActivityView {
			return activityForToolCompleted(ev)
		})
	})
	core.Subscribe(r.bus, func(ev core.ToolFailedEvent) {
		r.applyRuntimeEvent(ev.RuntimeEventType(), ev.RuntimeOccurredAt(), nil, func() RunActivityView {
			return activityForToolFailed(ev)
		})
	})
	core.Subscribe(r.bus, func(ev core.ApprovalRequestedEvent) {
		r.applyRuntimeEvent(ev.RuntimeEventType(), ev.RuntimeOccurredAt(), func() {
			r.setStatusLocked("waiting", ev.RuntimeOccurredAt())
			r.waitingReason = "approval"
			r.pendingApprovals[ev.ToolCallID] = PendingApprovalView{
				ToolCallID:  ev.ToolCallID,
				ToolName:    ev.ToolName,
				ArgsJSON:    ev.ArgsJSON,
				RequestedAt: ev.RequestedAt,
			}
			r.session.SetWaiting("approval")
		}, func() RunActivityView {
			return activityForApprovalRequested(ev)
		})
	})
	core.Subscribe(r.bus, func(ev core.ApprovalResolvedEvent) {
		r.applyRuntimeEvent(ev.RuntimeEventType(), ev.RuntimeOccurredAt(), func() {
			delete(r.pendingApprovals, ev.ToolCallID)
			if len(r.pendingApprovals) == 0 && r.status == "waiting" && r.waitingReason == "approval" {
				r.setStatusLocked("running", ev.RuntimeOccurredAt())
				r.waitingReason = ""
				r.session.SetStatus(agui.SessionStatusRunning)
			}
		}, func() RunActivityView {
			return activityForApprovalResolved(ev)
		})
	})
	core.Subscribe(r.bus, func(ev core.DeferredRequestedEvent) {
		r.applyRuntimeEvent(ev.RuntimeEventType(), ev.RuntimeOccurredAt(), nil, func() RunActivityView {
			return activityForDeferredRequested(ev)
		})
	})
	core.Subscribe(r.bus, func(ev core.DeferredResolvedEvent) {
		r.applyRuntimeEvent(ev.RuntimeEventType(), ev.RuntimeOccurredAt(), nil, func() RunActivityView {
			return activityForDeferredResolved(ev)
		})
	})
	core.Subscribe(r.bus, func(ev core.RunWaitingEvent) {
		r.applyRuntimeEvent(ev.RuntimeEventType(), ev.RuntimeOccurredAt(), func() {
			r.setStatusLocked("waiting", ev.RuntimeOccurredAt())
			r.waitingReason = ev.Reason
			r.session.SetWaiting(ev.Reason)
		}, func() RunActivityView {
			return activityForRunWaiting(ev)
		})
	})
	core.Subscribe(r.bus, func(ev core.RunResumedEvent) {
		r.applyRuntimeEvent(ev.RuntimeEventType(), ev.RuntimeOccurredAt(), func() {
			r.setStatusLocked("running", ev.RuntimeOccurredAt())
			r.waitingReason = ""
			r.session.SetStatus(agui.SessionStatusRunning)
		}, func() RunActivityView {
			return activityForRunResumed(ev)
		})
	})
}

func (r *RunRecord) applyRuntimeEvent(eventType string, at time.Time, mutate func(), buildActivity func() RunActivityView) {
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
	if buildActivity != nil {
		activity := buildActivity()
		if activity.Type == "" {
			activity.Type = eventType
		}
		if activity.OccurredAt.IsZero() {
			activity.OccurredAt = at
		}
		r.appendActivityLocked(activity)
	}
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
	for {
		runID := strings.TrimSpace(s.nextID())
		if runID == "" {
			runID = newRunID()
		}

		s.mu.Lock()
		if _, exists := s.runs[runID]; exists {
			s.mu.Unlock()
			continue
		}
		record := newRunRecord(s.now().UTC(), runID, req)
		s.runs[runID] = record
		s.mu.Unlock()
		return record
	}
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

func snapshotRecentActivities(src []RunActivityView) []RunActivityView {
	if len(src) == 0 {
		return nil
	}
	start := 0
	if len(src) > maxRecentRunActivities {
		start = len(src) - maxRecentRunActivities
	}
	return append([]RunActivityView(nil), src[start:]...)
}

func buildWaitingView(status, reason string, pendingApprovalCount int) RunWaitingView {
	reason = strings.TrimSpace(reason)
	if reason == "" && pendingApprovalCount > 0 {
		reason = "approval"
	}
	active := status == "waiting" || reason != "" || pendingApprovalCount > 0
	if !active {
		return RunWaitingView{}
	}
	label, detail, kind := waitingPresentation(reason, pendingApprovalCount)
	return RunWaitingView{
		Active:      true,
		Reason:      reason,
		Label:       label,
		Detail:      detail,
		PendingKind: kind,
	}
}

func waitingPresentation(reason string, pendingApprovalCount int) (label, detail, kind string) {
	reason = strings.TrimSpace(reason)
	switch reason {
	case "approval":
		label = "Waiting for approval"
		kind = "approval"
		if pendingApprovalCount > 0 {
			detail = fmt.Sprintf("%d pending tool approval%s.", pendingApprovalCount, pluralSuffix(pendingApprovalCount))
		} else {
			detail = "A tool call needs approval before the run can continue."
		}
	case "deferred":
		label = "Waiting for deferred input"
		kind = "deferred"
		detail = "The run will resume when deferred input is provided."
	case "approval_and_deferred":
		label = "Waiting for approval and deferred input"
		kind = "mixed"
		if pendingApprovalCount > 0 {
			detail = fmt.Sprintf("%d tool approval%s pending and deferred input still required.", pendingApprovalCount, pluralSuffix(pendingApprovalCount))
		} else {
			detail = "Approval and deferred input are both required before the run can continue."
		}
	case "":
		label = "Waiting"
		detail = "The run is paused."
	default:
		label = "Waiting"
		detail = humanizeIdentifier(reason)
		kind = "custom"
	}
	return label, detail, kind
}

func activityForRunStarted(ev core.RunStartedEvent) RunActivityView {
	return RunActivityView{
		Type:       ev.RuntimeEventType(),
		Label:      "Run started",
		Detail:     summarizeInline(firstNonEmpty(ev.Prompt, "Prompt received."), 120),
		OccurredAt: ev.RuntimeOccurredAt(),
	}
}

func activityForRunCompleted(ev core.RunCompletedEvent, waitingReason string) RunActivityView {
	activity := RunActivityView{
		Type:       ev.RuntimeEventType(),
		OccurredAt: ev.RuntimeOccurredAt(),
	}
	switch {
	case ev.Deferred:
		label, detail, _ := waitingPresentation(waitingReason, 0)
		activity.Label = "Run deferred"
		activity.Detail = firstNonEmpty(detail, label)
		activity.IsWaiting = true
		activity.FinishState = "waiting"
	case ev.Success:
		activity.Label = "Run completed"
		activity.Detail = "Finished successfully."
		activity.FinishState = "completed"
	case isCanceledRunError(ev.Error):
		activity.Label = "Run aborted"
		activity.Detail = summarizeInline(firstNonEmpty(ev.Error, "Execution cancelled."), 120)
		activity.IsError = true
		activity.FinishState = "aborted"
	default:
		activity.Label = "Run failed"
		activity.Detail = summarizeInline(firstNonEmpty(ev.Error, "Run failed."), 120)
		activity.IsError = true
		activity.FinishState = "failed"
	}
	return activity
}

func activityForTurnStarted(ev core.TurnStartedEvent) RunActivityView {
	return RunActivityView{
		Type:       ev.RuntimeEventType(),
		Label:      "Turn started",
		Detail:     fmt.Sprintf("Turn %d began.", ev.TurnNumber),
		OccurredAt: ev.RuntimeOccurredAt(),
		TurnNumber: ev.TurnNumber,
	}
}

func activityForTurnCompleted(ev core.TurnCompletedEvent) RunActivityView {
	detail := fmt.Sprintf("Turn %d finished", ev.TurnNumber)
	if ev.Error != "" {
		return RunActivityView{
			Type:       ev.RuntimeEventType(),
			Label:      "Turn failed",
			Detail:     summarizeInline(ev.Error, 120),
			OccurredAt: ev.RuntimeOccurredAt(),
			IsError:    true,
			TurnNumber: ev.TurnNumber,
		}
	}
	flags := make([]string, 0, 2)
	if ev.HasText {
		flags = append(flags, "text")
	}
	if ev.HasToolCalls {
		flags = append(flags, "tool calls")
	}
	if len(flags) > 0 {
		detail += " · " + strings.Join(flags, ", ")
	}
	return RunActivityView{
		Type:       ev.RuntimeEventType(),
		Label:      "Turn completed",
		Detail:     detail + ".",
		OccurredAt: ev.RuntimeOccurredAt(),
		TurnNumber: ev.TurnNumber,
	}
}

func activityForModelRequestStarted(ev core.ModelRequestStartedEvent) RunActivityView {
	detail := fmt.Sprintf("%d messages sent to the model", ev.MessageCount)
	if ev.TurnNumber > 0 {
		detail = fmt.Sprintf("Turn %d · %s", ev.TurnNumber, detail)
	}
	return RunActivityView{
		Type:       ev.RuntimeEventType(),
		Label:      "Model request",
		Detail:     detail + ".",
		OccurredAt: ev.RuntimeOccurredAt(),
		TurnNumber: ev.TurnNumber,
	}
}

func activityForModelResponseCompleted(ev core.ModelResponseCompletedEvent) RunActivityView {
	parts := make([]string, 0, 5)
	if ev.TurnNumber > 0 {
		parts = append(parts, fmt.Sprintf("Turn %d", ev.TurnNumber))
	}
	parts = append(parts, fmt.Sprintf("%d input", ev.InputTokens), fmt.Sprintf("%d output", ev.OutputTokens))
	if finish := strings.TrimSpace(ev.FinishReason); finish != "" {
		parts = append(parts, finish)
	}
	if ev.HasToolCalls {
		parts = append(parts, "tools")
	}
	if ev.HasText {
		parts = append(parts, "text")
	}
	return RunActivityView{
		Type:       ev.RuntimeEventType(),
		Label:      "Model response",
		Detail:     strings.Join(parts, " · "),
		OccurredAt: ev.RuntimeOccurredAt(),
		TurnNumber: ev.TurnNumber,
	}
}

func activityForToolCalled(ev core.ToolCalledEvent) RunActivityView {
	detail := firstNonEmpty(ev.ToolName, "tool")
	if args := summarizeInline(ev.ArgsJSON, 96); args != "" {
		detail += " · " + args
	}
	return RunActivityView{
		Type:       ev.RuntimeEventType(),
		Label:      "Tool called",
		Detail:     detail,
		OccurredAt: ev.RuntimeOccurredAt(),
		ToolCallID: ev.ToolCallID,
		ToolName:   ev.ToolName,
	}
}

func activityForToolCompleted(ev core.ToolCompletedEvent) RunActivityView {
	detail := firstNonEmpty(ev.ToolName, ev.ToolCallID, "tool")
	if result := summarizeInline(ev.Result, 96); result != "" {
		detail += " · " + result
	}
	return RunActivityView{
		Type:       ev.RuntimeEventType(),
		Label:      "Tool completed",
		Detail:     detail,
		OccurredAt: ev.RuntimeOccurredAt(),
		ToolCallID: ev.ToolCallID,
		ToolName:   ev.ToolName,
	}
}

func activityForToolFailed(ev core.ToolFailedEvent) RunActivityView {
	detail := firstNonEmpty(ev.ToolName, ev.ToolCallID, "tool")
	if failure := summarizeInline(ev.Error, 96); failure != "" {
		detail += " · " + failure
	}
	return RunActivityView{
		Type:       ev.RuntimeEventType(),
		Label:      "Tool failed",
		Detail:     detail,
		OccurredAt: ev.RuntimeOccurredAt(),
		IsError:    true,
		ToolCallID: ev.ToolCallID,
		ToolName:   ev.ToolName,
	}
}

func activityForApprovalRequested(ev core.ApprovalRequestedEvent) RunActivityView {
	detail := firstNonEmpty(ev.ToolName, ev.ToolCallID, "tool")
	if args := summarizeInline(ev.ArgsJSON, 96); args != "" {
		detail += " · " + args
	}
	return RunActivityView{
		Type:       ev.RuntimeEventType(),
		Label:      "Approval requested",
		Detail:     detail,
		OccurredAt: ev.RuntimeOccurredAt(),
		IsWaiting:  true,
		ToolCallID: ev.ToolCallID,
		ToolName:   ev.ToolName,
	}
}

func activityForApprovalResolved(ev core.ApprovalResolvedEvent) RunActivityView {
	label := "Approval granted"
	detail := firstNonEmpty(ev.ToolName, ev.ToolCallID, "tool")
	if !ev.Approved {
		label = "Approval denied"
	}
	return RunActivityView{
		Type:       ev.RuntimeEventType(),
		Label:      label,
		Detail:     detail,
		OccurredAt: ev.RuntimeOccurredAt(),
		IsError:    !ev.Approved,
		ToolCallID: ev.ToolCallID,
		ToolName:   ev.ToolName,
	}
}

func activityForDeferredRequested(ev core.DeferredRequestedEvent) RunActivityView {
	detail := firstNonEmpty(ev.ToolName, ev.ToolCallID, "deferred input")
	if args := summarizeInline(ev.ArgsJSON, 96); args != "" {
		detail += " · " + args
	}
	return RunActivityView{
		Type:       ev.RuntimeEventType(),
		Label:      "Deferred input requested",
		Detail:     detail,
		OccurredAt: ev.RuntimeOccurredAt(),
		IsWaiting:  true,
		ToolCallID: ev.ToolCallID,
		ToolName:   ev.ToolName,
	}
}

func activityForDeferredResolved(ev core.DeferredResolvedEvent) RunActivityView {
	label := "Deferred input received"
	if ev.IsError {
		label = "Deferred input failed"
	}
	detail := firstNonEmpty(ev.ToolName, ev.ToolCallID, "deferred input")
	if content := summarizeInline(ev.Content, 96); content != "" {
		detail += " · " + content
	}
	return RunActivityView{
		Type:       ev.RuntimeEventType(),
		Label:      label,
		Detail:     detail,
		OccurredAt: ev.RuntimeOccurredAt(),
		IsError:    ev.IsError,
		ToolCallID: ev.ToolCallID,
		ToolName:   ev.ToolName,
	}
}

func activityForRunWaiting(ev core.RunWaitingEvent) RunActivityView {
	label, detail, _ := waitingPresentation(ev.Reason, 0)
	return RunActivityView{
		Type:       ev.RuntimeEventType(),
		Label:      label,
		Detail:     detail,
		OccurredAt: ev.RuntimeOccurredAt(),
		IsWaiting:  true,
	}
}

func activityForRunResumed(ev core.RunResumedEvent) RunActivityView {
	return RunActivityView{
		Type:       ev.RuntimeEventType(),
		Label:      "Run resumed",
		Detail:     "Execution continued.",
		OccurredAt: ev.RuntimeOccurredAt(),
	}
}

func statusAllowsAbort(status string) bool {
	switch strings.TrimSpace(status) {
	case "completed", "failed", "aborted", "cancelled":
		return false
	default:
		return true
	}
}

func pluralSuffix(n int) string {
	if n == 1 {
		return ""
	}
	return "s"
}

func humanizeRuntimeEventType(eventType string) string {
	eventType = strings.TrimSpace(eventType)
	if eventType == "" {
		return "Activity"
	}
	return humanizeIdentifier(eventType)
}

func humanizeRunStatus(status string) string {
	status = strings.TrimSpace(status)
	if status == "" {
		return "Run status"
	}
	return humanizeIdentifier(status)
}

func humanizeIdentifier(value string) string {
	value = strings.TrimSpace(strings.ReplaceAll(value, "_", " "))
	if value == "" {
		return ""
	}
	parts := strings.Fields(value)
	for i, part := range parts {
		if part == "" {
			continue
		}
		parts[i] = strings.ToUpper(part[:1]) + part[1:]
	}
	return strings.Join(parts, " ")
}

func summarizeInline(value string, max int) string {
	value = strings.Join(strings.Fields(strings.TrimSpace(value)), " ")
	if value == "" || max <= 0 {
		return value
	}
	if len(value) <= max {
		return value
	}
	if max == 1 {
		return "…"
	}
	return strings.TrimSpace(value[:max-1]) + "…"
}

func errString(err error, fallback string) string {
	if err == nil {
		return fallback
	}
	return err.Error()
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
	return "run_" + uuid.NewString()
}

func isCanceledRunError(message string) bool {
	message = strings.ToLower(strings.TrimSpace(message))
	return message == "context canceled" || message == "context cancelled" || strings.Contains(message, "context canceled") || strings.Contains(message, "context cancelled")
}
