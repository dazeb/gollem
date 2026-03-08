package codetool

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"

	"github.com/fugue-labs/gollem/core"
)

// VerificationEntry represents one recorded verification result (test/build outcome).
type VerificationEntry struct {
	ID        string `json:"id" jsonschema:"description=Identifier (e.g. V1, V2). Auto-assigned if omitted on record."`
	Command   string `json:"command" jsonschema:"description=The verification command that was run"`
	Status    string `json:"status" jsonschema:"description=pass, fail, or in_progress"`
	Freshness string `json:"freshness" jsonschema:"description=fresh or stale"`
	Summary   string `json:"summary,omitempty" jsonschema:"description=Compact result text"`
	StaleBy   string `json:"stale_by,omitempty" jsonschema:"description=What caused staleness"`
}

// VerificationState is the exported state snapshot for the verification tool.
type VerificationState struct {
	Entries []VerificationEntry `json:"entries"`
}

const verificationToolName = "verification"

// VerificationFromToolState decodes the exported verification tool state snapshot.
func VerificationFromToolState(state map[string]any) (VerificationState, bool) {
	if len(state) == 0 {
		return VerificationState{}, false
	}
	raw, ok := state[verificationToolName]
	if !ok {
		return VerificationState{}, false
	}
	m, ok := raw.(map[string]any)
	if !ok {
		return VerificationState{}, false
	}
	entriesRaw, ok := m["entries"]
	if !ok {
		return VerificationState{}, true
	}
	b, err := json.Marshal(entriesRaw)
	if err != nil {
		return VerificationState{}, false
	}
	var entries []VerificationEntry
	if err := json.Unmarshal(b, &entries); err != nil {
		return VerificationState{}, false
	}
	return VerificationState{Entries: entries}, true
}

// CurrentVerification returns the currently exported verification state for this run context.
func CurrentVerification(rc *core.RunContext) (VerificationState, bool) {
	if rc == nil {
		return VerificationState{}, false
	}
	return VerificationFromToolState(rc.ToolState())
}

type verificationCommand struct {
	Command string `json:"command" jsonschema:"description=record|stale|get|summary|reset"`
	ID      string `json:"id,omitempty" jsonschema:"description=Entry ID for stale command (omit to mark all stale)"`
	Cmd     string `json:"cmd,omitempty" jsonschema:"description=The verification command that was run (for record)"`
	Status  string `json:"status,omitempty" jsonschema:"description=pass|fail|in_progress (for record)"`
	Summary string `json:"summary,omitempty" jsonschema:"description=Compact result text (for record)"`
	Reason  string `json:"reason,omitempty" jsonschema:"description=What caused staleness (for stale command)"`
}

type verificationState struct {
	mu      sync.Mutex
	entries []VerificationEntry
}

func (s *verificationState) ExportState() (any, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	cp := make([]VerificationEntry, len(s.entries))
	copy(cp, s.entries)
	return map[string]any{"entries": cp}, nil
}

func (s *verificationState) RestoreState(state any) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	m, ok := state.(map[string]any)
	if !ok {
		return errors.New("invalid verification state")
	}
	if raw, ok := m["entries"]; ok {
		b, err := json.Marshal(raw)
		if err != nil {
			return fmt.Errorf("marshal verification state entries: %w", err)
		}
		var entries []VerificationEntry
		if err := json.Unmarshal(b, &entries); err != nil {
			return fmt.Errorf("unmarshal verification state entries: %w", err)
		}
		s.entries = entries
	}
	return nil
}

// VerificationTool creates a tool that tracks verification results (test/build outcomes).
// Freshness is model-driven (the model calls "stale" after edits), complementary to the
// automatic staleness detection in verification.go's checkpoint system. Both coexist:
// the checkpoint gates completion based on real tool activity, while this tool gives the
// model an explicit way to track and report its own verification state.
func VerificationTool() core.Tool {
	state := &verificationState{}

	tool := core.FuncTool[verificationCommand](
		"verification",
		"Track verification results (test/build outcomes). "+
			"Commands: "+
			"'record' (record a test/build result with cmd, status pass|fail|in_progress, and optional summary), "+
			"'stale' (mark verifications stale after code changes — use id for one entry or omit for all), "+
			"'get' (return all entries), "+
			"'summary' (return pass/fail/stale counts), "+
			"'reset' (clear all verification entries for a fresh start). "+
			"Use after running tests or builds to track verification state.",
		func(_ context.Context, cmd verificationCommand) (any, error) {
			return executeVerificationCommand(state, cmd)
		},
		core.WithToolSequential(true),
	)
	tool.Stateful = state
	return tool
}

func executeVerificationCommand(state *verificationState, cmd verificationCommand) (any, error) {
	state.mu.Lock()
	defer state.mu.Unlock()

	switch strings.ToLower(strings.TrimSpace(cmd.Command)) {
	case "record":
		cmdText := strings.TrimSpace(cmd.Cmd)
		if cmdText == "" {
			return nil, errors.New("record requires cmd (the verification command that was run)")
		}
		status := normalizeVerificationStatus(cmd.Status)
		if status == "" {
			return nil, errors.New("record requires status in {pass, fail, in_progress}")
		}
		id := strings.ToUpper(strings.TrimSpace(cmd.ID))
		if id == "" {
			// Match existing entry by command text before allocating a new ID.
			for i := range state.entries {
				if strings.EqualFold(state.entries[i].Command, cmdText) {
					id = state.entries[i].ID
					break
				}
			}
			if id == "" {
				id = nextVerificationID(state.entries)
			}
		}
		// Update existing entry with same ID, or append new one.
		found := false
		for i := range state.entries {
			if strings.EqualFold(state.entries[i].ID, id) {
				state.entries[i].Command = cmdText
				state.entries[i].Status = status
				state.entries[i].Freshness = "fresh"
				state.entries[i].Summary = strings.TrimSpace(cmd.Summary)
				state.entries[i].StaleBy = ""
				found = true
				break
			}
		}
		if !found {
			state.entries = append(state.entries, VerificationEntry{
				ID:        id,
				Command:   cmdText,
				Status:    status,
				Freshness: "fresh",
				Summary:   strings.TrimSpace(cmd.Summary),
			})
		}
		return verificationSummaryLocked(state.entries), nil

	case "stale":
		reason := strings.TrimSpace(cmd.Reason)
		targetID := strings.ToUpper(strings.TrimSpace(cmd.ID))
		if targetID == "" {
			// Mark all entries stale.
			for i := range state.entries {
				state.entries[i].Freshness = "stale"
				state.entries[i].StaleBy = reason
			}
		} else {
			found := false
			for i := range state.entries {
				if strings.EqualFold(state.entries[i].ID, targetID) {
					state.entries[i].Freshness = "stale"
					state.entries[i].StaleBy = reason
					found = true
					break
				}
			}
			if !found {
				return nil, fmt.Errorf("verification entry %q not found", targetID)
			}
		}
		return verificationSummaryLocked(state.entries), nil

	case "get":
		cp := make([]VerificationEntry, len(state.entries))
		copy(cp, state.entries)
		return map[string]any{
			"status":  "ok",
			"entries": cp,
		}, nil

	case "summary":
		return verificationSummaryLocked(state.entries), nil

	case "reset":
		state.entries = nil
		return verificationSummaryLocked(state.entries), nil

	default:
		return nil, fmt.Errorf("unknown command %q (use record, stale, get, reset, or summary)", cmd.Command)
	}
}

func verificationSummaryLocked(entries []VerificationEntry) map[string]any {
	total := len(entries)
	pass, fail, stale, inProgress := 0, 0, 0, 0
	for _, e := range entries {
		if strings.EqualFold(strings.TrimSpace(e.Freshness), "stale") {
			stale++
		}
		switch normalizeVerificationStatus(e.Status) {
		case "pass":
			pass++
		case "fail":
			fail++
		case "in_progress":
			inProgress++
		}
	}
	// Defensive copy — the returned map is serialized outside the lock.
	cp := make([]VerificationEntry, len(entries))
	copy(cp, entries)
	return map[string]any{
		"status":      "ok",
		"total":       total,
		"pass":        pass,
		"fail":        fail,
		"stale":       stale,
		"in_progress": inProgress,
		"entries":     cp,
	}
}

func normalizeVerificationStatus(status string) string {
	switch strings.ToLower(strings.TrimSpace(status)) {
	case "pass", "passed":
		return "pass"
	case "fail", "failed":
		return "fail"
	case "in_progress", "in-progress", "in progress", "running":
		return "in_progress"
	default:
		return ""
	}
}

func nextVerificationID(entries []VerificationEntry) string {
	maxN := 0
	for _, e := range entries {
		id := strings.TrimSpace(strings.ToUpper(e.ID))
		if len(id) < 2 || id[0] != 'V' {
			continue
		}
		var n int
		if _, err := fmt.Sscanf(id, "V%d", &n); err == nil && n > maxN {
			maxN = n
		}
	}
	return fmt.Sprintf("V%d", maxN+1)
}
