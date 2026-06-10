package core

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestJSONFileExporter(t *testing.T) {
	dir := t.TempDir()
	exporter := NewJSONFileExporter(dir)

	model := NewTestModel(TextResponse("traced"))
	agent := NewAgent[string](model, WithTraceExporter[string](exporter))

	_, err := agent.Run(context.Background(), "test")
	if err != nil {
		t.Fatal(err)
	}

	// Check that a JSON file was created.
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 {
		t.Fatalf("expected 1 trace file, got %d", len(entries))
	}

	data, err := os.ReadFile(filepath.Join(dir, entries[0].Name()))
	if err != nil {
		t.Fatal(err)
	}

	var artifact TraceArtifact
	if err := json.Unmarshal(data, &artifact); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	if artifact.SchemaVersion != TraceArtifactSchemaVersion {
		t.Fatalf("schema version = %q, want %q", artifact.SchemaVersion, TraceArtifactSchemaVersion)
	}
	if artifact.Run.Prompt != "test" {
		t.Errorf("expected prompt 'test', got %q", artifact.Run.Prompt)
	}
	if !artifact.Summary.Success {
		t.Error("expected trace success=true")
	}
	if artifact.Trace == nil {
		t.Fatal("expected embedded core trace")
	}
}

func TestTraceExportersCoverErrorAndFanoutBranches(t *testing.T) {
	if err := NewJSONFileExporter(t.TempDir()).Export(context.Background(), nil); err == nil {
		t.Fatal("expected nil run trace export error")
	}
	if got := safeTraceFilenamePart(" ../bad run!* "); got != "bad_run" {
		t.Fatalf("safe filename = %q", got)
	}
	if got := safeTraceFilenamePart("..."); got != "run" {
		t.Fatalf("empty safe filename = %q", got)
	}
	var buf bytes.Buffer
	console := NewConsoleExporter(&buf)
	if err := console.Export(context.Background(), &RunTrace{
		RunID:   "failed-run",
		Prompt:  "prompt",
		Error:   "boom",
		Success: false,
		Usage:   RunUsage{Usage: Usage{InputTokens: 1, OutputTokens: 2}, Requests: 1},
		Steps:   []TraceStep{{Kind: TraceToolCall, Duration: time.Millisecond}},
	}); err != nil {
		t.Fatalf("console export: %v", err)
	}
	if !strings.Contains(buf.String(), "Error: boom") || !strings.Contains(buf.String(), "tool_call") {
		t.Fatalf("console output missing error/step:\n%s", buf.String())
	}
	err := NewMultiExporter(&failingExporter{}, console).Export(context.Background(), &RunTrace{RunID: "fanout"})
	if err == nil {
		t.Fatal("expected first exporter error")
	}
}

func TestConsoleExporter(t *testing.T) {
	var buf bytes.Buffer
	exporter := NewConsoleExporter(&buf)

	model := NewTestModel(TextResponse("console"))
	agent := NewAgent[string](model, WithTraceExporter[string](exporter))

	_, err := agent.Run(context.Background(), "hello")
	if err != nil {
		t.Fatal(err)
	}

	output := buf.String()
	if !strings.Contains(output, "Trace") {
		t.Errorf("expected trace output, got %q", output)
	}
	if !strings.Contains(output, "Prompt: hello") {
		t.Errorf("expected prompt in output, got %q", output)
	}
	if !strings.Contains(output, "Success: true") {
		t.Errorf("expected success in output, got %q", output)
	}
}

func TestMultiExporter(t *testing.T) {
	var buf1, buf2 bytes.Buffer
	exp1 := NewConsoleExporter(&buf1)
	exp2 := NewConsoleExporter(&buf2)
	multi := NewMultiExporter(exp1, exp2)

	model := NewTestModel(TextResponse("multi"))
	agent := NewAgent[string](model, WithTraceExporter[string](multi))

	_, err := agent.Run(context.Background(), "test")
	if err != nil {
		t.Fatal(err)
	}

	if buf1.Len() == 0 {
		t.Error("exporter 1 received no output")
	}
	if buf2.Len() == 0 {
		t.Error("exporter 2 received no output")
	}
}

func TestWithTraceExporter_ImplicitTracing(t *testing.T) {
	var buf bytes.Buffer
	exporter := NewConsoleExporter(&buf)

	model := NewTestModel(TextResponse("implicit"))
	// Don't add WithTracing explicitly — it should be enabled by the exporter.
	agent := NewAgent[string](model, WithTraceExporter[string](exporter))

	result, err := agent.Run(context.Background(), "test")
	if err != nil {
		t.Fatal(err)
	}
	if result.Trace == nil {
		t.Error("expected trace to be set when exporter is configured")
	}
}

func TestTraceArtifactProjectedToolEventsUseTurnNumbers(t *testing.T) {
	now := time.Date(2026, 5, 6, 12, 0, 0, 0, time.UTC)
	trace := &RunTrace{
		RunID:     "run-step-test",
		Prompt:    "step mapping",
		StartTime: now,
		EndTime:   now.Add(3 * time.Second),
		Success:   true,
		Requests: []RequestTrace{
			{
				RequestID:  "req-1",
				TurnNumber: 1,
				StartedAt:  now.Add(100 * time.Millisecond),
				EndedAt:    now.Add(200 * time.Millisecond),
				Response:   &RequestTraceResponse{FinishReason: FinishReasonToolCall},
			},
			{
				RequestID:  "req-2",
				TurnNumber: 2,
				StartedAt:  now.Add(2 * time.Second),
				EndedAt:    now.Add(2100 * time.Millisecond),
				Response:   &RequestTraceResponse{FinishReason: FinishReasonStop},
			},
		},
		Steps: []TraceStep{
			{Kind: TraceModelRequest, Timestamp: now.Add(100 * time.Millisecond)},
			{Kind: TraceModelResponse, Timestamp: now.Add(200 * time.Millisecond)},
			{Kind: TraceToolCall, Timestamp: now.Add(300 * time.Millisecond), Data: map[string]any{"tool_call_id": "call-1", "tool_name": "write"}},
			{Kind: TraceToolResult, Timestamp: now.Add(400 * time.Millisecond), Data: map[string]any{"tool_call_id": "call-1", "tool_name": "write"}},
		},
	}
	artifact, err := NewTraceArtifact(trace, nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, event := range artifact.Events {
		switch event.Kind {
		case "tool.called", "tool.completed":
			if event.Step != 1 {
				t.Fatalf("%s step = %d, want turn 1: %+v", event.Kind, event.Step, artifact.Events)
			}
		}
	}
}

type failingExporter struct{}

func (e *failingExporter) Export(_ context.Context, _ *RunTrace) error {
	return context.DeadlineExceeded
}

func TestTraceExporter_ErrorHandling(t *testing.T) {
	model := NewTestModel(TextResponse("ok"))
	agent := NewAgent[string](model, WithTraceExporter[string](&failingExporter{}))

	// Run should succeed even if exporter fails.
	result, err := agent.Run(context.Background(), "test")
	if err != nil {
		t.Fatal("expected run to succeed despite exporter error")
	}
	if result.Output != "ok" {
		t.Errorf("expected 'ok', got %q", result.Output)
	}
}
