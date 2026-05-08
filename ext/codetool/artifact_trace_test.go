package codetool

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	"github.com/fugue-labs/gollem/core"
)

func TestMutationToolsPublishArtifactChangedEvents(t *testing.T) {
	dir := t.TempDir()
	bus := core.NewEventBus()
	defer bus.Close()

	var events []core.ArtifactChangedEvent
	core.Subscribe(bus, func(ev core.ArtifactChangedEvent) {
		events = append(events, ev)
	})

	rc := &core.RunContext{
		RunID:       "run-1",
		ToolCallID:  "call-write",
		ParentRunID: "parent-1",
		EventBus:    bus,
	}

	writeTool := Write(WithWorkDir(dir))
	if _, err := writeTool.Handler(context.Background(), rc, `{"path":"a.txt","content":"one\ntwo\n"}`); err != nil {
		t.Fatalf("write failed: %v", err)
	}

	rc.ToolCallID = "call-edit"
	editTool := Edit(WithWorkDir(dir))
	if _, err := editTool.Handler(context.Background(), rc, `{"path":"a.txt","old_string":"two","new_string":"three"}`); err != nil {
		t.Fatalf("edit failed: %v", err)
	}

	rc.ToolCallID = "call-multi"
	multiTool := MultiEdit(WithWorkDir(dir))
	if _, err := multiTool.Handler(context.Background(), rc, `{"edits":[{"path":"a.txt","old_string":"three","new_string":"four"}]}`); err != nil {
		t.Fatalf("multi_edit failed: %v", err)
	}

	if len(events) != 3 {
		t.Fatalf("expected 3 artifact events, got %d: %#v", len(events), events)
	}

	wantPath := filepath.Join(dir, "a.txt")
	checks := []struct {
		callID    string
		toolName  string
		operation string
	}{
		{"call-write", "write", "create"},
		{"call-edit", "edit", "edit"},
		{"call-multi", "multi_edit", "multi_edit"},
	}
	for i, want := range checks {
		if events[i].RunID != "run-1" || events[i].ParentRunID != "parent-1" {
			t.Fatalf("event %d run identity mismatch: %#v", i, events[i])
		}
		if events[i].ToolCallID != want.callID || events[i].ToolName != want.toolName {
			t.Fatalf("event %d tool identity mismatch: %#v", i, events[i])
		}
		if events[i].Operation != want.operation {
			t.Fatalf("event %d operation = %q, want %q", i, events[i].Operation, want.operation)
		}
		if events[i].Path != wantPath {
			t.Fatalf("event %d path = %q, want %q", i, events[i].Path, wantPath)
		}
		if events[i].Bytes <= 0 || events[i].ChangedAt.IsZero() {
			t.Fatalf("event %d missing mutation metadata: %#v", i, events[i])
		}
		if events[i].AfterSHA256 == "" {
			t.Fatalf("event %d missing after hash: %#v", i, events[i])
		}
		if events[i].Diff == "" {
			t.Fatalf("event %d missing diff: %#v", i, events[i])
		}
		if events[i].ContentEncoding != "utf-8" || events[i].AfterContent == "" || events[i].ContentOmittedReason != "" {
			t.Fatalf("event %d missing content snapshot: %#v", i, events[i])
		}
	}
	if events[0].BeforeSHA256 != "" || !strings.Contains(events[0].Diff, "+++ b/") || !strings.Contains(events[0].Diff, "+one") {
		t.Fatalf("create diff missing new content: %#v", events[0])
	}
	if events[0].BeforeContent != "" || events[0].AfterContent != "one\ntwo\n" {
		t.Fatalf("create content snapshot mismatch: %#v", events[0])
	}
	if events[1].BeforeSHA256 == "" || strings.Contains(events[1].Diff, "-one") || strings.Contains(events[1].Diff, "+one") || !strings.Contains(events[1].Diff, " one") || !strings.Contains(events[1].Diff, "-two") || !strings.Contains(events[1].Diff, "+three") {
		t.Fatalf("edit diff missing before/after content: %#v", events[1])
	}
	if events[1].BeforeContent != "one\ntwo\n" || events[1].AfterContent != "one\nthree\n" {
		t.Fatalf("edit content snapshot mismatch: %#v", events[1])
	}
}
