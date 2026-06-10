package codetool

import (
	"context"

	"github.com/fugue-labs/gollem/core"
	traceutil "github.com/fugue-labs/gollem/ext/trace"
)

type managedTraceExporter struct {
	exporter core.TraceExporter
	recorder *traceutil.RuntimeRecorder
}

func (e *managedTraceExporter) Export(ctx context.Context, trace *core.RunTrace) error {
	return e.exporter.Export(ctx, trace)
}
