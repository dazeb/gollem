package agui

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"sync"
	"time"
)

// SessionStatus represents the current state of an AGUI session.
type SessionStatus string

const (
	SessionStatusStarting  SessionStatus = "starting"
	SessionStatusRunning   SessionStatus = "running"
	SessionStatusWaiting   SessionStatus = "waiting"
	SessionStatusCompleted SessionStatus = "completed"
	SessionStatusFailed    SessionStatus = "failed"
	SessionStatusCancelled SessionStatus = "cancelled"
	SessionStatusAborted   SessionStatus = "aborted"
)

// SessionMode identifies the gollem backend driving this session.
type SessionMode string

const (
	SessionModeCoreRun    SessionMode = "core-run"
	SessionModeCoreStream SessionMode = "core-stream"
	SessionModeCoreIter   SessionMode = "core-iter"
	SessionModeTemporal   SessionMode = "temporal"
	SessionModeGraph      SessionMode = "graph"
	SessionModeTeam       SessionMode = "team"
)

const defaultReplayCapacity = 10000

// Session tracks the state of an AGUI interaction. It owns a stable session ID
// that survives reconnects and, in Temporal mode, continue-as-new cycles.
//
// Ownership contract: Session (or a session-owned runtime wrapper) is the
// source of truth for replay sequencing, snapshot watermarks, pending approval
// metadata, pending deferred-input metadata, and the live cancel/abort handle.
// Adapter output is transient protocol formatting layered on top of this state.
type Session struct {
	mu sync.RWMutex

	// Identity
	ID          string      `json:"session_id"`
	RunID       string      `json:"run_id"`
	ParentRunID string      `json:"parent_run_id,omitempty"`
	Mode        SessionMode `json:"mode"`

	// Lifecycle
	Status    SessionStatus `json:"status"`
	CreatedAt time.Time     `json:"created_at"`
	UpdatedAt time.Time     `json:"updated_at"`

	// Waiting state
	WaitingReason string `json:"waiting_reason,omitempty"`

	// Replay and reconnect state
	seq    Sequencer
	replay *EventBuffer

	pendingApprovals      map[string]pendingApprovalState
	pendingExternalInputs map[string]pendingExternalInputState
}

type pendingApprovalState struct {
	ToolCallID  string    `json:"tool_call_id"`
	ToolName    string    `json:"tool_name"`
	ArgsJSON    string    `json:"args_json,omitempty"`
	RequestedAt time.Time `json:"requested_at"`
}

type pendingExternalInputState struct {
	ToolCallID  string    `json:"tool_call_id"`
	ToolName    string    `json:"tool_name"`
	ArgsJSON    string    `json:"args_json,omitempty"`
	RequestedAt time.Time `json:"requested_at"`
}

type rawAGUIEnvelope struct {
	Type      string          `json:"type"`
	RunID     string          `json:"runId,omitempty"`
	Name      string          `json:"name,omitempty"`
	Value     json.RawMessage `json:"value,omitempty"`
	Timestamp int64           `json:"timestamp,omitempty"`
}

type rawApprovalRequestedValue struct {
	ToolCallID string `json:"toolCallId"`
	ToolName   string `json:"toolName"`
	ArgsJSON   string `json:"argsJson,omitempty"`
}

type rawApprovalResolvedValue struct {
	ToolCallID string `json:"toolCallId"`
}

type rawDeferredRequestedValue struct {
	ToolCallID string `json:"toolCallId"`
	ToolName   string `json:"toolName"`
	ArgsJSON   string `json:"argsJson,omitempty"`
}

type rawDeferredResolvedValue struct {
	ToolCallID string `json:"toolCallId"`
}

type rawRunWaitingValue struct {
	Reason string `json:"reason"`
}

// NewSession creates a new AGUI session.
func NewSession(mode SessionMode) *Session {
	now := time.Now()
	return &Session{
		ID:                    newSessionID(),
		Mode:                  mode,
		Status:                SessionStatusStarting,
		CreatedAt:             now,
		UpdatedAt:             now,
		pendingApprovals:      make(map[string]pendingApprovalState),
		pendingExternalInputs: make(map[string]pendingExternalInputState),
	}
}

// SetRunID updates the gollem run ID for this session.
func (s *Session) SetRunID(runID, parentRunID string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.RunID = runID
	s.ParentRunID = parentRunID
	s.UpdatedAt = time.Now()
}

// SetStatus transitions the session to a new status.
func (s *Session) SetStatus(status SessionStatus) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.Status = status
	s.UpdatedAt = time.Now()
	if status != SessionStatusWaiting {
		s.WaitingReason = ""
	}
}

// SetWaiting transitions the session to waiting with a reason.
func (s *Session) SetWaiting(reason string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.Status = SessionStatusWaiting
	s.WaitingReason = reason
	s.UpdatedAt = time.Now()
}

// GetStatus returns the current session status.
func (s *Session) GetStatus() SessionStatus {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.Status
}

// ReplayBuffer returns the session-owned replay buffer, creating it if needed.
func (s *Session) ReplayBuffer() *EventBuffer {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.ensureTransportStateLocked(defaultReplayCapacity)
	return s.replay
}

// EnsureReplayBuffer ensures the session has a replay buffer using the requested
// capacity if one has not already been created.
func (s *Session) EnsureReplayBuffer(capacity int) *EventBuffer {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.ensureTransportStateLocked(capacity)
	return s.replay
}

// CaptureRawEvent updates session-owned reconnect state from a raw AG-UI event,
// assigns the next normalized sequence, appends it to replay storage, and
// returns the normalized transport event.
func (s *Session) CaptureRawEvent(eventType string, data json.RawMessage, capturedAt time.Time) Event {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.ensureTransportStateLocked(defaultReplayCapacity)
	dataCopy := append(json.RawMessage(nil), data...)
	s.applyRawEventLocked(dataCopy, capturedAt)
	ev := s.newEventLocked(eventType, dataCopy, capturedAt)
	s.replay.Append(ev)
	return ev
}

// PrepareReconnect captures an atomic replay watermark and returns either the
// exact replay range after lastSeq or a snapshot fallback when replay is
// incomplete.
func (s *Session) PrepareReconnect(lastSeq uint64, capturedAt time.Time) (highWater uint64, replay []Event, snapshot *Event) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.ensureTransportStateLocked(defaultReplayCapacity)
	highWater = s.replay.LastSeq()
	replay, complete := s.replay.Since(lastSeq)
	if !complete {
		snap := s.buildSnapshotLocked(highWater, capturedAt)
		return highWater, nil, &snap
	}
	return highWater, replay, nil
}

// NewEvent creates a new Event with automatic sequencing and session identity.
func (s *Session) NewEvent(eventType string, data any) Event {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.newEventLocked(eventType, MarshalData(data), time.Now())
}

// NewTurnEvent creates a new Event with turn number set.
func (s *Session) NewTurnEvent(eventType string, turnNumber int, data any) Event {
	ev := s.NewEvent(eventType, data)
	ev.TurnNumber = turnNumber
	return ev
}

func (s *Session) newEventLocked(eventType string, data json.RawMessage, capturedAt time.Time) Event {
	return Event{
		ID:          newEventID(),
		Sequence:    s.seq.Next(),
		Type:        eventType,
		SessionID:   s.ID,
		RunID:       s.RunID,
		ParentRunID: s.ParentRunID,
		Timestamp:   capturedAt,
		Data:        data,
	}
}

func (s *Session) ensureTransportStateLocked(capacity int) {
	if capacity <= 0 {
		capacity = defaultReplayCapacity
	}
	if s.replay == nil {
		s.replay = NewEventBuffer(capacity)
	}
	if s.pendingApprovals == nil {
		s.pendingApprovals = make(map[string]pendingApprovalState)
	}
	if s.pendingExternalInputs == nil {
		s.pendingExternalInputs = make(map[string]pendingExternalInputState)
	}
}

func (s *Session) applyRawEventLocked(data json.RawMessage, fallbackTime time.Time) {
	var env rawAGUIEnvelope
	if err := json.Unmarshal(data, &env); err != nil {
		return
	}

	occurredAt := fallbackTime
	if env.Timestamp > 0 {
		occurredAt = time.UnixMilli(env.Timestamp)
	}

	s.UpdatedAt = occurredAt
	if env.RunID != "" {
		s.RunID = env.RunID
	}

	switch env.Type {
	case AGUIRunStarted:
		s.Status = SessionStatusRunning
		s.WaitingReason = ""
	case AGUIRunFinished:
		s.Status = SessionStatusCompleted
		s.WaitingReason = ""
	case AGUIRunError:
		s.Status = SessionStatusFailed
		s.WaitingReason = ""
	case AGUICustom:
		s.applyCustomEventLocked(env.Name, env.Value, occurredAt)
	}
}

func (s *Session) applyCustomEventLocked(name string, value json.RawMessage, occurredAt time.Time) {
	switch name {
	case "gollem.approval.requested":
		var payload rawApprovalRequestedValue
		if err := json.Unmarshal(value, &payload); err != nil || payload.ToolCallID == "" {
			return
		}
		s.pendingApprovals[payload.ToolCallID] = pendingApprovalState{
			ToolCallID:  payload.ToolCallID,
			ToolName:    payload.ToolName,
			ArgsJSON:    payload.ArgsJSON,
			RequestedAt: occurredAt,
		}
	case "gollem.approval.resolved":
		var payload rawApprovalResolvedValue
		if err := json.Unmarshal(value, &payload); err != nil || payload.ToolCallID == "" {
			return
		}
		delete(s.pendingApprovals, payload.ToolCallID)
	case "gollem.deferred.requested":
		var payload rawDeferredRequestedValue
		if err := json.Unmarshal(value, &payload); err != nil || payload.ToolCallID == "" {
			return
		}
		s.pendingExternalInputs[payload.ToolCallID] = pendingExternalInputState{
			ToolCallID:  payload.ToolCallID,
			ToolName:    payload.ToolName,
			ArgsJSON:    payload.ArgsJSON,
			RequestedAt: occurredAt,
		}
	case "gollem.deferred.resolved":
		var payload rawDeferredResolvedValue
		if err := json.Unmarshal(value, &payload); err != nil || payload.ToolCallID == "" {
			return
		}
		delete(s.pendingExternalInputs, payload.ToolCallID)
	case "gollem.run.waiting":
		var payload rawRunWaitingValue
		if err := json.Unmarshal(value, &payload); err != nil {
			return
		}
		s.Status = SessionStatusWaiting
		s.WaitingReason = payload.Reason
	case "gollem.run.resumed":
		s.Status = SessionStatusRunning
		s.WaitingReason = ""
	}
}

func (s *Session) buildSnapshotLocked(snapshotSeq uint64, capturedAt time.Time) Event {
	payload := map[string]any{
		"session_id":              s.ID,
		"run_id":                  s.RunID,
		"parent_run_id":           s.ParentRunID,
		"mode":                    s.Mode,
		"status":                  s.Status,
		"created_at":              s.CreatedAt,
		"updated_at":              s.UpdatedAt,
		"waiting_reason":          s.WaitingReason,
		"pending_approvals":       clonePendingApprovals(s.pendingApprovals),
		"pending_external_inputs": clonePendingExternalInputs(s.pendingExternalInputs),
		"snapshot_sequence":       snapshotSeq,
	}
	return Event{
		ID:          "snapshot_" + newEventID(),
		Sequence:    snapshotSeq,
		Type:        EventSessionSnapshot,
		SessionID:   s.ID,
		RunID:       s.RunID,
		ParentRunID: s.ParentRunID,
		Timestamp:   capturedAt,
		Data:        MarshalData(payload),
	}
}

func clonePendingApprovals(src map[string]pendingApprovalState) map[string]pendingApprovalState {
	if len(src) == 0 {
		return map[string]pendingApprovalState{}
	}
	dst := make(map[string]pendingApprovalState, len(src))
	for k, v := range src {
		dst[k] = v
	}
	return dst
}

func clonePendingExternalInputs(src map[string]pendingExternalInputState) map[string]pendingExternalInputState {
	if len(src) == 0 {
		return map[string]pendingExternalInputState{}
	}
	dst := make(map[string]pendingExternalInputState, len(src))
	for k, v := range src {
		dst[k] = v
	}
	return dst
}

func newSessionID() string {
	return "ses_" + randomHex(12)
}

func newEventID() string {
	return "evt_" + randomHex(8)
}

func randomHex(n int) string {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		panic("agui: crypto/rand failed: " + err.Error())
	}
	return hex.EncodeToString(b)
}
