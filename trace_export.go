package gollem

import "context"

// TraceExporter exports a completed RunTrace to an external system.
type TraceExporter interface {
	Export(ctx context.Context, trace *RunTrace) error
}
