// Package trace provides canonical trace artifact tooling for Gollem agent runs.
package trace

import (
	"io"

	"github.com/fugue-labs/gollem/core"
)

const SchemaVersion = core.TraceArtifactSchemaVersion

type Artifact = core.TraceArtifact
type RunMetadata = core.TraceRunMetadata
type Event = core.TraceEvent
type RedactionMetadata = core.TraceRedactionMetadata
type SnapshotRecord = core.TraceSnapshotRecord
type Summary = core.TraceSummary
type EvaluatorSummary = core.TraceEvaluatorSummary

// WithCost attaches a run cost snapshot to an artifact summary.
func WithCost(artifact *Artifact, cost *core.RunCost) *Artifact {
	return core.WithTraceArtifactCost(artifact, cost)
}

// FromRunTrace converts the core trace into the canonical artifact shape while
// preserving the original trace payload.
func FromRunTrace(runTrace *core.RunTrace, metadata map[string]any) (*Artifact, error) {
	return core.NewTraceArtifact(runTrace, metadata)
}

// FromRunTraceWithSnapshots converts a core trace plus optional run snapshots
// into the canonical artifact shape.
func FromRunTraceWithSnapshots(runTrace *core.RunTrace, snapshots []*core.RunSnapshot, metadata map[string]any) (*Artifact, error) {
	return core.NewTraceArtifactWithSnapshots(runTrace, snapshots, metadata)
}

// FromRunTraceWithSnapshotsAndEvents converts a core trace plus optional
// snapshots and already-canonical runtime events into the artifact shape.
func FromRunTraceWithSnapshotsAndEvents(runTrace *core.RunTrace, snapshots []*core.RunSnapshot, runtimeEvents []Event, metadata map[string]any) (*Artifact, error) {
	return core.NewTraceArtifactWithSnapshotsAndEvents(runTrace, snapshots, runtimeEvents, metadata)
}

// EncodeSnapshotRecords converts run snapshots into JSON-safe artifact records.
func EncodeSnapshotRecords(snapshots []*core.RunSnapshot) ([]SnapshotRecord, error) {
	return core.EncodeTraceSnapshotRecords(snapshots)
}

// DecodeSnapshotRecord decodes a stored snapshot record into a run snapshot.
func DecodeSnapshotRecord(record SnapshotRecord) (*core.RunSnapshot, error) {
	return core.DecodeTraceSnapshotRecord(record)
}

// Read decodes a canonical artifact. Older raw core.RunTrace JSON files are
// accepted only as an import compatibility path and converted immediately.
func Read(r io.Reader) (*Artifact, error) {
	return core.ReadTraceArtifact(r)
}

// ReadFile reads a trace artifact from path. A path of "-" reads stdin.
func ReadFile(path string) (*Artifact, error) {
	return core.ReadTraceArtifactFile(path)
}

// Write writes an artifact as stable, indented JSON.
func Write(w io.Writer, artifact *Artifact) error {
	return core.WriteTraceArtifact(w, artifact)
}

// WriteFile writes an artifact to path. A path of "-" writes stdout.
func WriteFile(path string, artifact *Artifact) error {
	return core.WriteTraceArtifactFile(path, artifact)
}

func compactMap(src map[string]any) map[string]any {
	out := make(map[string]any, len(src))
	for k, v := range src {
		if isZeroPayloadValue(v) {
			continue
		}
		out[k] = v
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func isZeroPayloadValue(v any) bool {
	switch t := v.(type) {
	case nil:
		return true
	case string:
		return t == ""
	case int:
		return t == 0
	case int64:
		return t == 0
	default:
		return false
	}
}

func cloneMetadata(src map[string]any) map[string]any {
	if len(src) == 0 {
		return nil
	}
	dst := make(map[string]any, len(src))
	for k, v := range src {
		dst[k] = v
	}
	return dst
}
