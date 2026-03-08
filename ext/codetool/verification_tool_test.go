package codetool

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestVerificationRecordCreatesEntry(t *testing.T) {
	state := &verificationState{}
	result, err := executeVerificationCommand(state, verificationCommand{
		Command: "record",
		Cmd:     "go test ./...",
		Status:  "pass",
		Summary: "ok all packages",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	m := result.(map[string]any)
	if m["total"] != 1 {
		t.Fatalf("total=%v, want 1", m["total"])
	}
	if m["pass"] != 1 {
		t.Fatalf("pass=%v, want 1", m["pass"])
	}
	if len(state.entries) != 1 {
		t.Fatalf("entries=%d, want 1", len(state.entries))
	}
	e := state.entries[0]
	if e.Command != "go test ./..." {
		t.Fatalf("command=%q", e.Command)
	}
	if e.Status != "pass" {
		t.Fatalf("status=%q", e.Status)
	}
	if e.Freshness != "fresh" {
		t.Fatalf("freshness=%q", e.Freshness)
	}
	if e.Summary != "ok all packages" {
		t.Fatalf("summary=%q", e.Summary)
	}
}

func TestVerificationRecordAutoAssignsID(t *testing.T) {
	state := &verificationState{}
	for _, cmd := range []string{"go test ./...", "go build ./...", "go vet ./..."} {
		if _, err := executeVerificationCommand(state, verificationCommand{
			Command: "record",
			Cmd:     cmd,
			Status:  "pass",
		}); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	}
	if len(state.entries) != 3 {
		t.Fatalf("entries=%d, want 3", len(state.entries))
	}
	for i, want := range []string{"V1", "V2", "V3"} {
		if state.entries[i].ID != want {
			t.Fatalf("entries[%d].ID=%q, want %q", i, state.entries[i].ID, want)
		}
	}
}

func TestVerificationRecordUpdatesExistingEntry(t *testing.T) {
	state := &verificationState{}
	if _, err := executeVerificationCommand(state, verificationCommand{
		Command: "record",
		ID:      "V1",
		Cmd:     "go test ./...",
		Status:  "fail",
		Summary: "FAIL",
	}); err != nil {
		t.Fatal(err)
	}
	// Re-record same ID with pass.
	if _, err := executeVerificationCommand(state, verificationCommand{
		Command: "record",
		ID:      "V1",
		Cmd:     "go test ./...",
		Status:  "pass",
		Summary: "PASS",
	}); err != nil {
		t.Fatal(err)
	}
	if len(state.entries) != 1 {
		t.Fatalf("entries=%d, want 1 (should update in place)", len(state.entries))
	}
	if state.entries[0].Status != "pass" {
		t.Fatalf("status=%q, want pass", state.entries[0].Status)
	}
	if state.entries[0].Freshness != "fresh" {
		t.Fatalf("freshness=%q, want fresh", state.entries[0].Freshness)
	}
}

func TestVerificationStaleMarksAllEntries(t *testing.T) {
	state := &verificationState{
		entries: []VerificationEntry{
			{ID: "V1", Command: "go test ./...", Status: "pass", Freshness: "fresh"},
			{ID: "V2", Command: "go build ./...", Status: "pass", Freshness: "fresh"},
		},
	}
	if _, err := executeVerificationCommand(state, verificationCommand{
		Command: "stale",
		Reason:  "edited main.go",
	}); err != nil {
		t.Fatal(err)
	}
	for i, e := range state.entries {
		if e.Freshness != "stale" {
			t.Fatalf("entries[%d].freshness=%q, want stale", i, e.Freshness)
		}
		if e.StaleBy != "edited main.go" {
			t.Fatalf("entries[%d].stale_by=%q", i, e.StaleBy)
		}
	}
}

func TestVerificationStaleMarksSingleEntry(t *testing.T) {
	state := &verificationState{
		entries: []VerificationEntry{
			{ID: "V1", Command: "go test ./...", Status: "pass", Freshness: "fresh"},
			{ID: "V2", Command: "go build ./...", Status: "pass", Freshness: "fresh"},
		},
	}
	if _, err := executeVerificationCommand(state, verificationCommand{
		Command: "stale",
		ID:      "V1",
		Reason:  "edited main.go",
	}); err != nil {
		t.Fatal(err)
	}
	if state.entries[0].Freshness != "stale" {
		t.Fatalf("V1 freshness=%q, want stale", state.entries[0].Freshness)
	}
	if state.entries[1].Freshness != "fresh" {
		t.Fatalf("V2 freshness=%q, want fresh (should not be affected)", state.entries[1].Freshness)
	}
}

func TestVerificationStaleNonexistentIDReturnsError(t *testing.T) {
	state := &verificationState{
		entries: []VerificationEntry{
			{ID: "V1", Command: "go test ./...", Status: "pass", Freshness: "fresh"},
		},
	}
	_, err := executeVerificationCommand(state, verificationCommand{
		Command: "stale",
		ID:      "V99",
	})
	if err == nil {
		t.Fatal("expected error for nonexistent ID")
	}
}

func TestVerificationGetReturnsAllEntries(t *testing.T) {
	state := &verificationState{
		entries: []VerificationEntry{
			{ID: "V1", Command: "go test ./...", Status: "pass", Freshness: "fresh"},
			{ID: "V2", Command: "go build ./...", Status: "fail", Freshness: "stale"},
		},
	}
	result, err := executeVerificationCommand(state, verificationCommand{Command: "get"})
	if err != nil {
		t.Fatal(err)
	}
	m := result.(map[string]any)
	entries := m["entries"].([]VerificationEntry)
	if len(entries) != 2 {
		t.Fatalf("entries=%d, want 2", len(entries))
	}
}

func TestVerificationSummaryReturnsCounts(t *testing.T) {
	state := &verificationState{
		entries: []VerificationEntry{
			{ID: "V1", Command: "go test ./...", Status: "pass", Freshness: "fresh"},
			{ID: "V2", Command: "go build ./...", Status: "fail", Freshness: "stale", StaleBy: "edit"},
			{ID: "V3", Command: "go vet ./...", Status: "in_progress", Freshness: "fresh"},
		},
	}
	result, err := executeVerificationCommand(state, verificationCommand{Command: "summary"})
	if err != nil {
		t.Fatal(err)
	}
	m := result.(map[string]any)
	if m["total"] != 3 {
		t.Fatalf("total=%v", m["total"])
	}
	if m["pass"] != 1 {
		t.Fatalf("pass=%v", m["pass"])
	}
	if m["fail"] != 1 {
		t.Fatalf("fail=%v", m["fail"])
	}
	if m["stale"] != 1 {
		t.Fatalf("stale=%v", m["stale"])
	}
	if m["in_progress"] != 1 {
		t.Fatalf("in_progress=%v", m["in_progress"])
	}
}

func TestVerificationExportRestoreState(t *testing.T) {
	state := &verificationState{
		entries: []VerificationEntry{
			{ID: "V1", Command: "go test ./...", Status: "pass", Freshness: "fresh", Summary: "ok"},
			{ID: "V2", Command: "go build ./...", Status: "fail", Freshness: "stale", StaleBy: "edit main.go"},
		},
	}
	exported, err := state.ExportState()
	if err != nil {
		t.Fatal(err)
	}
	// Simulate round-trip through JSON (as happens in agent checkpoint).
	b, err := json.Marshal(exported)
	if err != nil {
		t.Fatal(err)
	}
	var restored any
	if err := json.Unmarshal(b, &restored); err != nil {
		t.Fatal(err)
	}

	newState := &verificationState{}
	if err := newState.RestoreState(restored); err != nil {
		t.Fatal(err)
	}
	if len(newState.entries) != 2 {
		t.Fatalf("restored entries=%d, want 2", len(newState.entries))
	}
	if newState.entries[0].ID != "V1" || newState.entries[0].Status != "pass" {
		t.Fatalf("restored V1: id=%q status=%q", newState.entries[0].ID, newState.entries[0].Status)
	}
	if newState.entries[1].StaleBy != "edit main.go" {
		t.Fatalf("restored V2 stale_by=%q", newState.entries[1].StaleBy)
	}
}

func TestVerificationFromToolState(t *testing.T) {
	toolState := map[string]any{
		"verification": map[string]any{
			"entries": []map[string]any{
				{"id": "V1", "command": "go test ./...", "status": "pass", "freshness": "fresh"},
				{"id": "V2", "command": "go build ./...", "status": "fail", "freshness": "stale", "stale_by": "edit"},
			},
		},
	}
	vs, ok := VerificationFromToolState(toolState)
	if !ok {
		t.Fatal("expected ok=true")
	}
	if len(vs.Entries) != 2 {
		t.Fatalf("entries=%d, want 2", len(vs.Entries))
	}
	if vs.Entries[0].ID != "V1" || vs.Entries[0].Status != "pass" {
		t.Fatalf("V1: id=%q status=%q", vs.Entries[0].ID, vs.Entries[0].Status)
	}
	if vs.Entries[1].StaleBy != "edit" {
		t.Fatalf("V2 stale_by=%q", vs.Entries[1].StaleBy)
	}
}

func TestVerificationFromToolStateMissingKey(t *testing.T) {
	_, ok := VerificationFromToolState(map[string]any{"planning": map[string]any{}})
	if ok {
		t.Fatal("expected ok=false when verification key missing")
	}
}

func TestVerificationFromToolStateEmptyEntries(t *testing.T) {
	vs, ok := VerificationFromToolState(map[string]any{
		"verification": map[string]any{},
	})
	if !ok {
		t.Fatal("expected ok=true for empty verification state")
	}
	if len(vs.Entries) != 0 {
		t.Fatalf("entries=%d, want 0", len(vs.Entries))
	}
}

func TestVerificationRecordRequiresCmd(t *testing.T) {
	state := &verificationState{}
	_, err := executeVerificationCommand(state, verificationCommand{
		Command: "record",
		Status:  "pass",
	})
	if err == nil || !strings.Contains(err.Error(), "cmd") {
		t.Fatalf("expected error about cmd, got %v", err)
	}
}

func TestVerificationRecordRequiresStatus(t *testing.T) {
	state := &verificationState{}
	_, err := executeVerificationCommand(state, verificationCommand{
		Command: "record",
		Cmd:     "go test ./...",
	})
	if err == nil || !strings.Contains(err.Error(), "status") {
		t.Fatalf("expected error about status, got %v", err)
	}
}

func TestVerificationRecordClearsStaleOnReRecord(t *testing.T) {
	state := &verificationState{
		entries: []VerificationEntry{
			{ID: "V1", Command: "go test ./...", Status: "pass", Freshness: "stale", StaleBy: "edit"},
		},
	}
	if _, err := executeVerificationCommand(state, verificationCommand{
		Command: "record",
		ID:      "V1",
		Cmd:     "go test ./...",
		Status:  "pass",
		Summary: "ok",
	}); err != nil {
		t.Fatal(err)
	}
	if state.entries[0].Freshness != "fresh" {
		t.Fatalf("freshness=%q, want fresh after re-record", state.entries[0].Freshness)
	}
	if state.entries[0].StaleBy != "" {
		t.Fatalf("stale_by=%q, want empty after re-record", state.entries[0].StaleBy)
	}
}

func TestVerificationUnknownCommand(t *testing.T) {
	state := &verificationState{}
	_, err := executeVerificationCommand(state, verificationCommand{Command: "bogus"})
	if err == nil {
		t.Fatal("expected error for unknown command")
	}
}

func TestVerificationRecordNormalizesIDToUppercase(t *testing.T) {
	state := &verificationState{}
	if _, err := executeVerificationCommand(state, verificationCommand{
		Command: "record",
		ID:      "v1",
		Cmd:     "go test ./...",
		Status:  "pass",
	}); err != nil {
		t.Fatal(err)
	}
	if state.entries[0].ID != "V1" {
		t.Fatalf("ID=%q, want V1 (should normalize to uppercase)", state.entries[0].ID)
	}
	// Re-record with different case should update the same entry.
	if _, err := executeVerificationCommand(state, verificationCommand{
		Command: "record",
		ID:      "V1",
		Cmd:     "go test ./...",
		Status:  "fail",
	}); err != nil {
		t.Fatal(err)
	}
	if len(state.entries) != 1 {
		t.Fatalf("entries=%d, want 1 (should update in place, not create duplicate)", len(state.entries))
	}
	if state.entries[0].Status != "fail" {
		t.Fatalf("status=%q, want fail", state.entries[0].Status)
	}
}

func TestVerificationStaleClearsReasonWhenOmitted(t *testing.T) {
	state := &verificationState{
		entries: []VerificationEntry{
			{ID: "V1", Command: "go test ./...", Status: "pass", Freshness: "fresh"},
		},
	}
	// Mark stale with a reason.
	if _, err := executeVerificationCommand(state, verificationCommand{
		Command: "stale",
		Reason:  "edited main.go",
	}); err != nil {
		t.Fatal(err)
	}
	if state.entries[0].StaleBy != "edited main.go" {
		t.Fatalf("stale_by=%q after first stale", state.entries[0].StaleBy)
	}
	// Mark stale again without a reason — should clear the old reason.
	if _, err := executeVerificationCommand(state, verificationCommand{
		Command: "stale",
	}); err != nil {
		t.Fatal(err)
	}
	if state.entries[0].StaleBy != "" {
		t.Fatalf("stale_by=%q, want empty after stale without reason", state.entries[0].StaleBy)
	}
}

func TestVerificationSummaryDoesNotAliasInternalState(t *testing.T) {
	state := &verificationState{}
	if _, err := executeVerificationCommand(state, verificationCommand{
		Command: "record",
		Cmd:     "go test ./...",
		Status:  "pass",
	}); err != nil {
		t.Fatal(err)
	}
	// Get summary — entries in the result should be a copy.
	result, err := executeVerificationCommand(state, verificationCommand{Command: "summary"})
	if err != nil {
		t.Fatal(err)
	}
	returned := result.(map[string]any)["entries"].([]VerificationEntry)
	if returned[0].Freshness != "fresh" {
		t.Fatalf("returned freshness=%q, want fresh", returned[0].Freshness)
	}
	// Mutate internal state via stale.
	if _, err := executeVerificationCommand(state, verificationCommand{
		Command: "stale",
		Reason:  "edit",
	}); err != nil {
		t.Fatal(err)
	}
	// The previously returned slice must NOT have been mutated.
	if returned[0].Freshness != "fresh" {
		t.Fatalf("returned freshness=%q after stale, want fresh (should be independent copy)", returned[0].Freshness)
	}
}

func TestVerificationRecordMatchesByCommandWhenIDOmitted(t *testing.T) {
	state := &verificationState{}
	// Record fail without explicit ID.
	if _, err := executeVerificationCommand(state, verificationCommand{
		Command: "record",
		Cmd:     "go test ./...",
		Status:  "fail",
		Summary: "FAIL",
	}); err != nil {
		t.Fatal(err)
	}
	if len(state.entries) != 1 {
		t.Fatalf("entries=%d, want 1", len(state.entries))
	}
	// Re-record same command without ID — should update in place.
	if _, err := executeVerificationCommand(state, verificationCommand{
		Command: "record",
		Cmd:     "go test ./...",
		Status:  "pass",
		Summary: "PASS",
	}); err != nil {
		t.Fatal(err)
	}
	if len(state.entries) != 1 {
		t.Fatalf("entries=%d, want 1 (should update existing by command match)", len(state.entries))
	}
	if state.entries[0].Status != "pass" {
		t.Fatalf("status=%q, want pass", state.entries[0].Status)
	}
	if state.entries[0].Freshness != "fresh" {
		t.Fatalf("freshness=%q, want fresh", state.entries[0].Freshness)
	}
}

func TestVerificationRecordCreatesNewEntryForDifferentCommand(t *testing.T) {
	state := &verificationState{}
	if _, err := executeVerificationCommand(state, verificationCommand{
		Command: "record",
		Cmd:     "go test ./...",
		Status:  "pass",
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := executeVerificationCommand(state, verificationCommand{
		Command: "record",
		Cmd:     "go build ./...",
		Status:  "pass",
	}); err != nil {
		t.Fatal(err)
	}
	if len(state.entries) != 2 {
		t.Fatalf("entries=%d, want 2 (different commands should create separate entries)", len(state.entries))
	}
}

func TestVerificationResetClearsAllEntries(t *testing.T) {
	state := &verificationState{
		entries: []VerificationEntry{
			{ID: "V1", Command: "go test ./...", Status: "pass", Freshness: "fresh"},
			{ID: "V2", Command: "go build ./...", Status: "fail", Freshness: "stale"},
		},
	}
	result, err := executeVerificationCommand(state, verificationCommand{Command: "reset"})
	if err != nil {
		t.Fatal(err)
	}
	if len(state.entries) != 0 {
		t.Fatalf("entries=%d, want 0 after reset", len(state.entries))
	}
	m := result.(map[string]any)
	if m["total"] != 0 {
		t.Fatalf("total=%v, want 0", m["total"])
	}
}

func TestVerificationRecordAcceptsInProgressWithSpace(t *testing.T) {
	state := &verificationState{}
	if _, err := executeVerificationCommand(state, verificationCommand{
		Command: "record",
		Cmd:     "go test ./...",
		Status:  "in progress",
	}); err != nil {
		t.Fatal(err)
	}
	if state.entries[0].Status != "in_progress" {
		t.Fatalf("status=%q, want in_progress (should normalize 'in progress' with space)", state.entries[0].Status)
	}
}

func TestVerificationStaleNormalizesIDToUppercase(t *testing.T) {
	state := &verificationState{
		entries: []VerificationEntry{
			{ID: "V1", Command: "go test ./...", Status: "pass", Freshness: "fresh"},
		},
	}
	if _, err := executeVerificationCommand(state, verificationCommand{
		Command: "stale",
		ID:      "v1",
		Reason:  "edit",
	}); err != nil {
		t.Fatal(err)
	}
	if state.entries[0].Freshness != "stale" {
		t.Fatalf("freshness=%q, want stale (lowercase id should match)", state.entries[0].Freshness)
	}
}
