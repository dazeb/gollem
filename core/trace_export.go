package core

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// TraceExporter exports a completed RunTrace to an external system.
type TraceExporter interface {
	Export(ctx context.Context, trace *RunTrace) error
}

// JSONFileExporter writes canonical gollem.trace.v1 JSON artifacts to a
// directory. The name is kept for source compatibility; use
// NewTraceFileExporter in new code.
type JSONFileExporter struct {
	dir string
}

// TraceFileExporter writes canonical gollem.trace.v1 JSON artifacts to a directory.
type TraceFileExporter = JSONFileExporter

// NewTraceFileExporter creates a trace exporter that writes canonical
// gollem.trace.v1 artifacts.
func NewTraceFileExporter(dir string) *TraceFileExporter {
	return NewJSONFileExporter(dir)
}

// NewJSONFileExporter creates a trace exporter that writes canonical
// gollem.trace.v1 artifacts. The function name is kept for compatibility with
// existing callers.
func NewJSONFileExporter(dir string) *JSONFileExporter {
	return &JSONFileExporter{dir: dir}
}

func (e *JSONFileExporter) Export(_ context.Context, trace *RunTrace) error {
	if trace == nil {
		return errors.New("trace exporter: nil run trace")
	}
	if err := os.MkdirAll(e.dir, 0o755); err != nil {
		return fmt.Errorf("trace exporter: create dir: %w", err)
	}

	filename := fmt.Sprintf("trace_%s_%s.trace.json", safeTraceFilenamePart(trace.RunID), trace.StartTime.Format("20060102T150405"))
	path := filepath.Join(e.dir, filename)

	artifact, err := NewTraceArtifact(trace, nil)
	if err != nil {
		return fmt.Errorf("trace exporter: build artifact: %w", err)
	}
	return WriteTraceArtifactFile(path, artifact)
}

// ConsoleExporter prints a human-readable trace summary to an io.Writer.
type ConsoleExporter struct {
	w io.Writer
}

// NewConsoleExporter creates a trace exporter that writes to an io.Writer.
func NewConsoleExporter(w io.Writer) *ConsoleExporter {
	return &ConsoleExporter{w: w}
}

func (e *ConsoleExporter) Export(_ context.Context, trace *RunTrace) error {
	fmt.Fprintf(e.w, "=== Trace %s ===\n", trace.RunID)
	fmt.Fprintf(e.w, "Prompt: %s\n", trace.Prompt)
	fmt.Fprintf(e.w, "Duration: %s\n", trace.Duration)
	fmt.Fprintf(e.w, "Success: %v\n", trace.Success)
	if trace.Error != "" {
		fmt.Fprintf(e.w, "Error: %s\n", trace.Error)
	}
	fmt.Fprintf(e.w, "Steps: %d\n", len(trace.Steps))
	fmt.Fprintf(e.w, "Requests: %d, Tool Calls: %d\n", trace.Usage.Requests, trace.Usage.ToolCalls)
	fmt.Fprintf(e.w, "Tokens: in=%d out=%d\n", trace.Usage.InputTokens, trace.Usage.OutputTokens)

	for i, step := range trace.Steps {
		fmt.Fprintf(e.w, "  [%d] %s (%.0fms)\n", i, step.Kind, float64(step.Duration)/float64(time.Millisecond))
	}
	return nil
}

// MultiExporter fans out to multiple exporters.
type MultiExporter struct {
	exporters []TraceExporter
}

// NewMultiExporter creates an exporter that fans out to multiple exporters.
func NewMultiExporter(exporters ...TraceExporter) *MultiExporter {
	return &MultiExporter{exporters: exporters}
}

func (e *MultiExporter) Export(ctx context.Context, trace *RunTrace) error {
	var firstErr error
	for _, exp := range e.exporters {
		if err := exp.Export(ctx, trace); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	return firstErr
}

// WithTraceExporter adds a trace exporter to the agent.
// Tracing is automatically enabled when an exporter is set.
func WithTraceExporter[T any](exporter TraceExporter) AgentOption[T] {
	return func(a *Agent[T]) {
		a.traceExporters = append(a.traceExporters, exporter)
		a.tracingEnabled = true // implicitly enable tracing
	}
}

func safeTraceFilenamePart(value string) string {
	var b strings.Builder
	for _, r := range value {
		switch {
		case r >= 'a' && r <= 'z':
			b.WriteRune(r)
		case r >= 'A' && r <= 'Z':
			b.WriteRune(r)
		case r >= '0' && r <= '9':
			b.WriteRune(r)
		case r == '-', r == '_', r == '.':
			b.WriteRune(r)
		default:
			b.WriteByte('_')
		}
	}
	part := strings.Trim(b.String(), "._-")
	if part == "" {
		return "run"
	}
	return part
}
