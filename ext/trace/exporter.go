package trace

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/fugue-labs/gollem/core"
)

// DirectoryExporter writes completed runs as canonical gollem.trace.v1
// artifacts to a directory.
type DirectoryExporter struct {
	dir             string
	metadata        map[string]any
	runtimeRecorder *RuntimeRecorder
}

// DirectoryExporterOption configures a DirectoryExporter.
type DirectoryExporterOption func(*DirectoryExporter)

// WithExporterMetadata attaches static metadata to every exported artifact.
func WithExporterMetadata(metadata map[string]any) DirectoryExporterOption {
	return func(e *DirectoryExporter) {
		e.metadata = cloneMetadata(metadata)
	}
}

// WithRuntimeRecorder includes runtime-boundary events recorded from the
// agent's event bus. Pair this with core.WithEventBus on the exported agent.
func WithRuntimeRecorder(recorder *RuntimeRecorder) DirectoryExporterOption {
	return func(e *DirectoryExporter) {
		e.runtimeRecorder = recorder
	}
}

// NewDirectoryExporter creates a trace exporter that writes canonical trace
// artifacts under dir. The files can be inspected with `gollem trace inspect`
// or opened directly with `gollem trace view`.
func NewDirectoryExporter(dir string, opts ...DirectoryExporterOption) *DirectoryExporter {
	exporter := &DirectoryExporter{dir: dir}
	for _, opt := range opts {
		opt(exporter)
	}
	return exporter
}

// Export converts a completed core trace to a canonical trace artifact and
// writes it to the configured directory.
func (e *DirectoryExporter) Export(_ context.Context, runTrace *core.RunTrace) error {
	if e == nil {
		return errors.New("trace directory exporter: nil exporter")
	}
	if strings.TrimSpace(e.dir) == "" {
		return errors.New("trace directory exporter: directory is required")
	}
	if runTrace == nil {
		return errors.New("trace directory exporter: nil run trace")
	}

	var runtimeEvents []Event
	if e.runtimeRecorder != nil {
		runtimeEvents = e.runtimeRecorder.EventsForTrace(runTrace.RunID)
	}
	artifact, err := FromRunTraceWithSnapshotsAndEvents(runTrace, nil, runtimeEvents, e.metadata)
	if err != nil {
		return err
	}
	return WriteFile(filepath.Join(e.dir, artifactFilename(runTrace)), artifact)
}

func artifactFilename(runTrace *core.RunTrace) string {
	start := runTrace.StartTime
	if start.IsZero() {
		start = time.Now()
	}
	runID := fileSafeTracePart(runTrace.RunID)
	if runID == "" {
		runID = "run"
	}
	return fmt.Sprintf("trace_%s_%s.trace.json", runID, start.Format("20060102T150405"))
}

func fileSafeTracePart(value string) string {
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
	return strings.Trim(b.String(), "._-")
}
