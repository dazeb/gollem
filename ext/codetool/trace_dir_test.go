package codetool

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/fugue-labs/gollem/core"
	traceutil "github.com/fugue-labs/gollem/ext/trace"
)

func TestAgentOptionsWithTraceDirWritesCanonicalArtifact(t *testing.T) {
	workDir := t.TempDir()
	traceDir := t.TempDir()
	agent := core.NewAgent[string](
		core.NewTestModel(core.TextResponse("done")),
		AgentOptions(workDir, WithTraceDir(traceDir))...,
	)

	result, err := agent.Run(context.Background(), "write an sdk trace")
	if err != nil {
		t.Fatal(err)
	}
	if result.Trace == nil {
		t.Fatal("expected tracing to be enabled")
	}

	entries, err := os.ReadDir(traceDir)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 {
		t.Fatalf("expected 1 trace artifact, got %d", len(entries))
	}

	artifact, err := traceutil.ReadFile(filepath.Join(traceDir, entries[0].Name()))
	if err != nil {
		t.Fatal(err)
	}
	if artifact.SchemaVersion != traceutil.SchemaVersion {
		t.Fatalf("schema version = %q, want %q", artifact.SchemaVersion, traceutil.SchemaVersion)
	}
	if artifact.Run.ID != result.RunID {
		t.Fatalf("artifact run ID = %q, want %q", artifact.Run.ID, result.RunID)
	}
}

func TestAgentOptionsWithTraceDirDoesNotCreateEventBus(t *testing.T) {
	agent := core.NewAgent[string](
		core.NewTestModel(core.TextResponse("done")),
		AgentOptions(t.TempDir(), WithTraceDir(t.TempDir()))...,
	)
	features := agent.ExecutionFeatures()
	if features.HasEventBus {
		t.Fatal("TraceDir without WithEventBus should not create an internal event bus")
	}
	if features.TraceExporters != 1 {
		t.Fatalf("TraceDir should still configure one trace exporter, got %d", features.TraceExporters)
	}
}

func TestAgentOptionsWithTraceDirCapturesArtifactChangedEvent(t *testing.T) {
	workDir := t.TempDir()
	traceDir := t.TempDir()
	bus := core.NewEventBus()
	defer bus.Close()
	agent := core.NewAgent[string](
		core.NewTestModel(
			core.ToolCallResponseWithID("write", `{"path":"out.txt","content":"hello\n"}`, "call-write"),
			core.TextResponse("done"),
		),
		AgentOptions(workDir, WithTraceDir(traceDir), WithEventBus(bus))...,
	)

	result, err := agent.Run(context.Background(), "write a file")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(workDir, "out.txt")); err != nil {
		t.Fatalf("expected output file: %v", err)
	}

	entries, err := os.ReadDir(traceDir)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 {
		t.Fatalf("expected 1 trace artifact, got %d", len(entries))
	}

	artifact, err := traceutil.ReadFile(filepath.Join(traceDir, entries[0].Name()))
	if err != nil {
		t.Fatal(err)
	}
	if artifact.Run.ID != result.RunID {
		t.Fatalf("artifact run ID = %q, want %q", artifact.Run.ID, result.RunID)
	}
	for _, event := range artifact.Events {
		if event.Kind != "artifact.changed" {
			continue
		}
		if event.RequestID != "call-write" {
			t.Fatalf("artifact.changed request id = %q, want call-write", event.RequestID)
		}
		path, _ := event.Payload["path"].(string)
		if !strings.HasSuffix(path, string(filepath.Separator)+"out.txt") {
			t.Fatalf("artifact.changed path = %q", path)
		}
		if event.Payload["operation"] != "create" {
			t.Fatalf("artifact.changed operation = %v, want create", event.Payload["operation"])
		}
		return
	}
	t.Fatalf("missing artifact.changed event in %+v", artifact.Events)
}

func TestAgentOptionsWithTraceDirEmptyDisablesFileExport(t *testing.T) {
	workDir := t.TempDir()
	agent := core.NewAgent[string](
		core.NewTestModel(core.TextResponse("done")),
		AgentOptions(workDir, WithTraceDir(""))...,
	)

	result, err := agent.Run(context.Background(), "do not write an sdk trace")
	if err != nil {
		t.Fatal(err)
	}
	if result.Trace == nil {
		t.Fatal("expected in-memory tracing to remain enabled")
	}

	entries, err := os.ReadDir(workDir)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 0 {
		t.Fatalf("expected no files in workdir, got %d", len(entries))
	}
}
