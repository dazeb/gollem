package agui

import (
	"crypto/rand"
	"encoding/hex"
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

// Session tracks the state of an AGUI interaction. It owns a stable session ID
// that survives reconnects and, in Temporal mode, continue-as-new cycles.
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

	// Replay
	seq Sequencer
}

// NewSession creates a new AGUI session.
func NewSession(mode SessionMode) *Session {
	now := time.Now()
	return &Session{
		ID:        newSessionID(),
		Mode:      mode,
		Status:    SessionStatusStarting,
		CreatedAt: now,
		UpdatedAt: now,
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

// NewEvent creates a new Event with automatic sequencing and session identity.
func (s *Session) NewEvent(eventType string, data any) Event {
	s.mu.RLock()
	runID := s.RunID
	parentRunID := s.ParentRunID
	s.mu.RUnlock()

	return Event{
		ID:          newEventID(),
		Sequence:    s.seq.Next(),
		Type:        eventType,
		SessionID:   s.ID,
		RunID:       runID,
		ParentRunID: parentRunID,
		Timestamp:   time.Now(),
		Data:        MarshalData(data),
	}
}

// NewTurnEvent creates a new Event with turn number set.
func (s *Session) NewTurnEvent(eventType string, turnNumber int, data any) Event {
	ev := s.NewEvent(eventType, data)
	ev.TurnNumber = turnNumber
	return ev
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
