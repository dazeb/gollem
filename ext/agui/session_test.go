package agui

import (
	"encoding/json"
	"testing"
	"time"
)

func TestSession_NewEvent_UniqueIDs(t *testing.T) {
	s := NewSession(SessionModeCoreRun)
	s.SetRunID("run_1", "")

	seen := map[string]bool{}
	for range 100 {
		ev := s.NewEvent(EventRunStarted, nil)
		if seen[ev.ID] {
			t.Errorf("duplicate event ID: %s", ev.ID)
		}
		seen[ev.ID] = true
	}
}

func TestSession_NewEvent_MonotonicSequence(t *testing.T) {
	s := NewSession(SessionModeCoreStream)
	s.SetRunID("run_1", "parent_1")

	prev := uint64(0)
	for range 100 {
		ev := s.NewEvent(EventTurnStarted, nil)
		if ev.Sequence <= prev {
			t.Errorf("sequence %d not greater than %d", ev.Sequence, prev)
		}
		prev = ev.Sequence
	}
}

func TestSession_NewEvent_CarriesRunID(t *testing.T) {
	s := NewSession(SessionModeCoreRun)
	s.SetRunID("run_42", "parent_7")

	ev := s.NewEvent(EventToolExecutionStarted, nil)
	if ev.RunID != "run_42" {
		t.Errorf("RunID = %q, want %q", ev.RunID, "run_42")
	}
	if ev.ParentRunID != "parent_7" {
		t.Errorf("ParentRunID = %q, want %q", ev.ParentRunID, "parent_7")
	}
	if ev.SessionID != s.ID {
		t.Errorf("SessionID = %q, want %q", ev.SessionID, s.ID)
	}
}

func TestSession_NewTurnEvent_HasTurnNumber(t *testing.T) {
	s := NewSession(SessionModeCoreRun)
	ev := s.NewTurnEvent(EventTurnStarted, 5, nil)
	if ev.TurnNumber != 5 {
		t.Errorf("TurnNumber = %d, want 5", ev.TurnNumber)
	}
}

func TestSession_StatusTransitions(t *testing.T) {
	s := NewSession(SessionModeCoreRun)
	if s.GetStatus() != SessionStatusStarting {
		t.Errorf("initial status = %q, want %q", s.GetStatus(), SessionStatusStarting)
	}

	s.SetStatus(SessionStatusRunning)
	if s.GetStatus() != SessionStatusRunning {
		t.Errorf("status = %q, want %q", s.GetStatus(), SessionStatusRunning)
	}

	s.SetWaiting("approval")
	if s.GetStatus() != SessionStatusWaiting {
		t.Errorf("status = %q, want %q", s.GetStatus(), SessionStatusWaiting)
	}

	s.SetStatus(SessionStatusCompleted)
	if s.GetStatus() != SessionStatusCompleted {
		t.Errorf("status = %q, want %q", s.GetStatus(), SessionStatusCompleted)
	}
}

func TestSession_CaptureRawEventDoesNotRegressNewerStatus(t *testing.T) {
	s := NewSession(SessionModeCoreStream)
	s.SetStatus(SessionStatusRunning)

	staleWaiting := rawEvent(t, map[string]any{
		"type":      AGUICustom,
		"name":      "gollem.run.waiting",
		"value":     map[string]any{"reason": "approval"},
		"timestamp": time.Now().Add(-time.Second).UnixMilli(),
	})
	s.CaptureRawEvent("raw", staleWaiting, time.Now())

	if got := s.GetStatus(); got != SessionStatusRunning {
		t.Fatalf("status = %q, want %q", got, SessionStatusRunning)
	}
	s.mu.RLock()
	waitingReason := s.WaitingReason
	s.mu.RUnlock()
	if waitingReason != "" {
		t.Fatalf("waiting reason = %q, want empty", waitingReason)
	}

	freshWaiting := rawEvent(t, map[string]any{
		"type":      AGUICustom,
		"name":      "gollem.run.waiting",
		"value":     map[string]any{"reason": "deferred"},
		"timestamp": time.Now().Add(time.Second).UnixMilli(),
	})
	s.CaptureRawEvent("raw", freshWaiting, time.Now().Add(time.Second))

	if got := s.GetStatus(); got != SessionStatusWaiting {
		t.Fatalf("status = %q, want %q", got, SessionStatusWaiting)
	}
	s.mu.RLock()
	waitingReason = s.WaitingReason
	s.mu.RUnlock()
	if waitingReason != "deferred" {
		t.Fatalf("waiting reason = %q, want deferred", waitingReason)
	}
}

func TestNewSession_HasValidID(t *testing.T) {
	s := NewSession(SessionModeCoreRun)
	if len(s.ID) < 10 {
		t.Errorf("session ID too short: %q", s.ID)
	}
	if s.ID[:4] != "ses_" {
		t.Errorf("session ID should start with ses_, got %q", s.ID)
	}
}

func rawEvent(t *testing.T, payload any) json.RawMessage {
	t.Helper()
	data, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("marshal raw event: %v", err)
	}
	return data
}
