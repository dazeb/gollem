package codetool

import (
	"context"
	"testing"
	"time"

	"github.com/fugue-labs/gollem/core"
	traceutil "github.com/fugue-labs/gollem/ext/trace"
)

type noopTraceExporter struct{}

func (noopTraceExporter) Export(context.Context, *core.RunTrace) error { return nil }

func TestManagedTraceExporterKeepsRecorderSubscribedAcrossExports(t *testing.T) {
	bus := core.NewEventBus()
	defer bus.Close()
	recorder := traceutil.NewRuntimeRecorder(bus)
	exporter := &managedTraceExporter{
		exporter: noopTraceExporter{},
		recorder: recorder,
	}
	now := time.Date(2026, 5, 6, 12, 0, 0, 0, time.UTC)

	core.Publish(bus, core.RunStartedEvent{RunID: "run-1", StartedAt: now})
	if got := len(recorder.Events()); got != 1 {
		t.Fatalf("events before export = %d, want 1", got)
	}
	if err := exporter.Export(context.Background(), &core.RunTrace{RunID: "run-1"}); err != nil {
		t.Fatalf("Export() error = %v", err)
	}
	core.Publish(bus, core.RunStartedEvent{RunID: "run-2", StartedAt: now.Add(time.Second)})
	if got := len(recorder.Events()); got != 2 {
		t.Fatalf("events after first export = %d, want recorder to remain subscribed", got)
	}
	if err := exporter.Export(context.Background(), &core.RunTrace{RunID: "run-2"}); err != nil {
		t.Fatalf("second Export() error = %v", err)
	}
}
