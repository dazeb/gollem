package appserver

import (
	"bytes"
	"encoding/base64"
	"strings"
	"testing"
	"time"

	"github.com/fugue-labs/gollem/appserver/protocol"
	toolprocess "github.com/fugue-labs/gollem/appserver/tools/process"
)

func TestOperationalBackgroundTerminalProjectionIsBoundedAndRedacted(t *testing.T) {
	now := time.Now().UTC()
	record := operationalBackgroundTerminal("/workspace", &toolprocess.Snapshot{
		ID:        "process-1",
		PID:       42,
		Command:   "/usr/bin/printf",
		Args:      []string{"--token", "super-secret"},
		WorkDir:   "/workspace/subdir",
		Status:    toolprocess.StatusCompleted,
		ExitCode:  0,
		StartedAt: now,
		EndedAt:   now.Add(time.Second),
		Stdout:    []byte("super-secret"),
		Stderr:    []byte("super-secret-error"),
		Error:     "super-secret-error",
	})
	if record.ID != "process-1" || record.TerminalID != record.ID || record.ProcessID != record.ID ||
		record.Command != "printf" || record.Title != "printf" || record.WorkDir != "subdir" ||
		record.ArgumentCount != 2 || !record.CommandRedacted || record.ExitCode == nil || *record.ExitCode != 0 {
		t.Fatalf("terminal projection = %#v", record)
	}
	if strings.Contains(record.Command, "secret") || strings.Contains(record.WorkDir, "workspace") {
		t.Fatalf("terminal projection leaked native metadata: %#v", record)
	}

	shell := operationalBackgroundTerminal("/workspace", &toolprocess.Snapshot{
		ID:      "process-2",
		Command: "printf super-secret",
		Shell:   true,
		WorkDir: "/outside/workspace",
		Status:  toolprocess.StatusRunning,
	})
	if shell.Command != "shell command" || shell.WorkDir != "." || !shell.CommandRedacted ||
		!shell.MetadataTruncated || shell.ExitCode != nil {
		t.Fatalf("shell terminal projection = %#v", shell)
	}
}

func TestOperationalTerminalOutputUsesBoundedTail(t *testing.T) {
	value := append(bytes.Repeat([]byte("a"), operationalTerminalOutputMaxBytes), []byte("tail")...)
	encoded, truncated := operationalTerminalOutput(value, false)
	decoded, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		t.Fatalf("decode terminal output: %v", err)
	}
	if !truncated || len(decoded) != operationalTerminalOutputMaxBytes || !bytes.HasSuffix(decoded, []byte("tail")) {
		t.Fatalf("bounded terminal output = %d bytes, truncated=%t, suffix=%q", len(decoded), truncated, decoded[len(decoded)-4:])
	}
	if encoded, truncated := operationalTerminalOutput([]byte{0, 0xff}, false); truncated || encoded != "AP8=" {
		t.Fatalf("binary terminal output = %q, truncated=%t", encoded, truncated)
	}
}

func TestOperationalDisplayTextNormalizesControlAndInvalidUTF8(t *testing.T) {
	value, normalized := operationalDisplayText("branch\nname\xff")
	if !normalized || strings.ContainsAny(value, "\n\r\t") || !strings.Contains(value, "\uFFFD") {
		t.Fatalf("normalized display text = %q, %v", value, normalized)
	}
}

func TestOperationalTerminalTerminateApprovalItemIDBindsOpaqueTarget(t *testing.T) {
	first := operationalTerminalTerminateApprovalItemID("process-1")
	second := operationalTerminalTerminateApprovalItemID("process-2")
	if !strings.HasPrefix(first, "terminal-terminate-sha256:") ||
		len(strings.TrimPrefix(first, "terminal-terminate-sha256:")) != 64 {
		t.Fatalf("approval item id = %q", first)
	}
	if first == second {
		t.Fatal("different process ids produced the same approval item id")
	}
	if strings.Contains(first, "process-1") {
		t.Fatalf("approval item id leaked native process id: %q", first)
	}
	if again := operationalTerminalTerminateApprovalItemID("process-1"); again != first {
		t.Fatalf("approval item id changed: %q != %q", again, first)
	}
}

func TestOperationalPageBoundsRejectsStaleAndMalformedCursors(t *testing.T) {
	params := protocol.OperationalListParams{Limit: 2}
	start, end, next, rpcErr := operationalPageBounds(params, "inventory", "sha256:one", 5)
	if rpcErr != nil || start != 0 || end != 2 || next == "" {
		t.Fatalf("first page = %d:%d cursor=%q error=%#v", start, end, next, rpcErr)
	}
	start, end, next, rpcErr = operationalPageBounds(
		protocol.OperationalListParams{Limit: 2, Cursor: next}, "inventory", "sha256:one", 5,
	)
	if rpcErr != nil || start != 2 || end != 4 || next == "" {
		t.Fatalf("second page = %d:%d cursor=%q error=%#v", start, end, next, rpcErr)
	}
	if _, _, _, rpcErr := operationalPageBounds(
		protocol.OperationalListParams{Cursor: next}, "inventory", "sha256:two", 5,
	); rpcErr == nil || !strings.Contains(rpcErr.Message, "stale") {
		t.Fatalf("stale cursor error = %#v", rpcErr)
	}

	malformed := base64.RawURLEncoding.EncodeToString(
		[]byte(`{"version":1,"kind":"inventory","snapshotId":"sha256:one","offset":2} {}`),
	)
	if _, _, _, rpcErr := operationalPageBounds(
		protocol.OperationalListParams{Cursor: malformed}, "inventory", "sha256:one", 5,
	); rpcErr == nil {
		t.Fatal("cursor with trailing JSON succeeded")
	}
}

func TestBoundedOperationalTerminalsCapsCleanReceipts(t *testing.T) {
	terminals := make([]protocol.BackgroundTerminal, operationalListMaxLimit+3)
	for i := range terminals {
		terminals[i].ID = strings.Repeat("x", i+1)
	}
	bounded, truncated := boundedOperationalTerminals(terminals)
	if !truncated || len(bounded) != operationalListMaxLimit {
		t.Fatalf("bounded terminals = %d, truncated=%v", len(bounded), truncated)
	}
	bounded[0].ID = "changed"
	if terminals[0].ID == "changed" {
		t.Fatal("bounded terminals alias caller storage")
	}
}
