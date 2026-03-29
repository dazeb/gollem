package agui

import (
	"testing"
)

func TestEventBuffer_AppendAndSince(t *testing.T) {
	buf := NewEventBuffer(100)
	s := &Session{ID: "ses_1", Mode: SessionModeCoreRun}

	for i := 0; i < 10; i++ {
		buf.Append(s.NewEvent(EventRunStarted, nil))
	}

	if buf.Len() != 10 {
		t.Fatalf("expected 10 events, got %d", buf.Len())
	}

	// Since(0) returns all.
	events, complete := buf.Since(0)
	if !complete {
		t.Error("expected complete=true for Since(0)")
	}
	if len(events) != 10 {
		t.Errorf("expected 10 events, got %d", len(events))
	}

	// Since(5) returns events 6-10.
	events, complete = buf.Since(5)
	if !complete {
		t.Error("expected complete=true")
	}
	if len(events) != 5 {
		t.Errorf("expected 5 events after seq 5, got %d", len(events))
	}
	if events[0].Sequence != 6 {
		t.Errorf("first event sequence = %d, want 6", events[0].Sequence)
	}

	// Since(last) returns nil.
	events, complete = buf.Since(buf.LastSeq())
	if !complete {
		t.Error("expected complete=true")
	}
	if events != nil {
		t.Errorf("expected nil events after last seq, got %d", len(events))
	}
}

func TestEventBuffer_Eviction(t *testing.T) {
	buf := NewEventBuffer(10)
	s := &Session{ID: "ses_1", Mode: SessionModeCoreRun}

	// Fill to capacity and beyond.
	for i := 0; i < 15; i++ {
		buf.Append(s.NewEvent(EventRunStarted, nil))
	}

	if buf.Len() > 10 {
		t.Errorf("buffer should not exceed capacity, got %d", buf.Len())
	}

	// Since(0) returns all available (fresh client).
	events, complete := buf.Since(0)
	if !complete {
		t.Error("expected complete=true for fresh client")
	}
	if len(events) != buf.Len() {
		t.Errorf("expected %d events, got %d", buf.Len(), len(events))
	}

	// Since(1) should detect gap (sequence 1 was evicted).
	events, complete = buf.Since(1)
	if complete {
		t.Error("expected complete=false for evicted sequence")
	}
	if events != nil {
		t.Error("expected nil events on gap")
	}
}

func TestEventBuffer_Empty(t *testing.T) {
	buf := NewEventBuffer(10)

	events, complete := buf.Since(0)
	if !complete {
		t.Error("expected complete=true for empty buffer")
	}
	if events != nil {
		t.Error("expected nil events for empty buffer")
	}
	if buf.LastSeq() != 0 {
		t.Errorf("expected LastSeq=0 for empty buffer, got %d", buf.LastSeq())
	}
}

func TestEventBuffer_SingleElement(t *testing.T) {
	buf := NewEventBuffer(10)
	s := &Session{ID: "ses_1", Mode: SessionModeCoreRun}

	buf.Append(s.NewEvent(EventRunStarted, nil))

	events, complete := buf.Since(0)
	if !complete {
		t.Error("expected complete=true")
	}
	if len(events) != 1 {
		t.Errorf("expected 1 event, got %d", len(events))
	}

	events, complete = buf.Since(1)
	if !complete {
		t.Error("expected complete=true")
	}
	if events != nil {
		t.Error("expected nil events after only event")
	}
}

func TestEventBuffer_All(t *testing.T) {
	buf := NewEventBuffer(100)
	s := &Session{ID: "ses_1", Mode: SessionModeCoreRun}

	for i := 0; i < 5; i++ {
		buf.Append(s.NewEvent(EventRunStarted, nil))
	}

	all := buf.All()
	if len(all) != 5 {
		t.Errorf("expected 5 events, got %d", len(all))
	}

	// Modifying the returned slice should not affect the buffer.
	all[0] = Event{}
	origAll := buf.All()
	if origAll[0].Sequence == 0 {
		t.Error("returned slice should be a copy")
	}
}

func TestSequencer_Monotonic(t *testing.T) {
	var s Sequencer
	prev := uint64(0)
	for i := 0; i < 100; i++ {
		next := s.Next()
		if next <= prev {
			t.Errorf("sequence %d is not greater than %d", next, prev)
		}
		prev = next
	}
}
