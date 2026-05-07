package trace

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"path"
	"strconv"
	"strings"

	"github.com/fugue-labs/gollem/core"
)

// ObjectPut is the canonical trace artifact object written by an
// ObjectStorageExporter.
type ObjectPut struct {
	Key         string
	ContentType string
	Body        []byte
	Metadata    map[string]string
}

// ObjectStore is the minimal durable object-storage API required by the trace
// exporter. Adapters can wrap S3, GCS, Azure Blob Storage, R2, MinIO, or an
// internal object service without pulling provider SDKs into gollem.
type ObjectStore interface {
	PutObject(ctx context.Context, object ObjectPut) error
}

// ObjectKeyFunc builds a durable object key for an exported trace artifact.
// The function should be deterministic for the same trace so Temporal activity
// retries overwrite or deduplicate the same object instead of creating copies.
type ObjectKeyFunc func(runTrace *core.RunTrace, artifact *Artifact) (string, error)

// ObjectStorageExporter writes canonical gollem.trace.v1 artifacts to durable
// object storage.
type ObjectStorageExporter struct {
	store            ObjectStore
	prefix           string
	contentType      string
	artifactMetadata map[string]any
	objectMetadata   map[string]string
	runtimeRecorder  *RuntimeRecorder
	keyFunc          ObjectKeyFunc
}

// ObjectStorageExporterOption configures an ObjectStorageExporter.
type ObjectStorageExporterOption func(*ObjectStorageExporter)

// WithObjectKeyPrefix prefixes the default object key. Leading and trailing
// slashes are ignored so the exporter behaves consistently across S3-like
// providers.
func WithObjectKeyPrefix(prefix string) ObjectStorageExporterOption {
	return func(e *ObjectStorageExporter) {
		e.prefix = strings.Trim(strings.TrimSpace(prefix), "/")
	}
}

// WithObjectKeyFunc overrides the default deterministic object key builder.
func WithObjectKeyFunc(fn ObjectKeyFunc) ObjectStorageExporterOption {
	return func(e *ObjectStorageExporter) {
		e.keyFunc = fn
	}
}

// WithObjectExporterMetadata attaches static metadata inside every canonical
// trace artifact.
func WithObjectExporterMetadata(metadata map[string]any) ObjectStorageExporterOption {
	return func(e *ObjectStorageExporter) {
		e.artifactMetadata = cloneMetadata(metadata)
	}
}

// WithObjectMetadata attaches provider metadata to every stored object.
func WithObjectMetadata(metadata map[string]string) ObjectStorageExporterOption {
	return func(e *ObjectStorageExporter) {
		e.objectMetadata = cloneStringMap(metadata)
	}
}

// WithObjectRuntimeRecorder includes runtime-boundary events recorded from the
// agent's event bus. Pair this with core.WithEventBus on the exported agent.
func WithObjectRuntimeRecorder(recorder *RuntimeRecorder) ObjectStorageExporterOption {
	return func(e *ObjectStorageExporter) {
		e.runtimeRecorder = recorder
	}
}

// WithObjectContentType overrides the content type stored with trace objects.
func WithObjectContentType(contentType string) ObjectStorageExporterOption {
	return func(e *ObjectStorageExporter) {
		e.contentType = strings.TrimSpace(contentType)
	}
}

// NewObjectStorageExporter creates a trace exporter for durable object stores.
func NewObjectStorageExporter(store ObjectStore, opts ...ObjectStorageExporterOption) *ObjectStorageExporter {
	exporter := &ObjectStorageExporter{
		store:       store,
		contentType: OrchestratorArtifactContentType,
	}
	for _, opt := range opts {
		opt(exporter)
	}
	if exporter.keyFunc == nil {
		exporter.keyFunc = exporter.defaultObjectKey
	}
	if exporter.contentType == "" {
		exporter.contentType = OrchestratorArtifactContentType
	}
	return exporter
}

// Export converts a completed core trace to a canonical trace artifact and
// stores it at a deterministic object key.
func (e *ObjectStorageExporter) Export(ctx context.Context, runTrace *core.RunTrace) error {
	if e == nil {
		return errors.New("trace object exporter: nil exporter")
	}
	if e.store == nil {
		return errors.New("trace object exporter: object store is required")
	}
	if runTrace == nil {
		return errors.New("trace object exporter: nil run trace")
	}

	var runtimeEvents []Event
	if e.runtimeRecorder != nil {
		runtimeEvents = e.runtimeRecorder.EventsForTrace(runTrace.RunID)
	}
	artifact, err := FromRunTraceWithSnapshotsAndEvents(runTrace, nil, runtimeEvents, e.artifactMetadata)
	if err != nil {
		return fmt.Errorf("trace object exporter: build artifact: %w", err)
	}
	key, err := e.keyFunc(runTrace, artifact)
	if err != nil {
		return fmt.Errorf("trace object exporter: build object key: %w", err)
	}
	key = strings.Trim(strings.TrimSpace(key), "/")
	if key == "" {
		return errors.New("trace object exporter: object key is required")
	}

	var body bytes.Buffer
	if err := Write(&body, artifact); err != nil {
		return fmt.Errorf("trace object exporter: encode artifact: %w", err)
	}
	return e.store.PutObject(ctx, ObjectPut{
		Key:         key,
		ContentType: e.contentType,
		Body:        body.Bytes(),
		Metadata:    e.buildObjectMetadata(artifact),
	})
}

func (e *ObjectStorageExporter) buildObjectMetadata(artifact *Artifact) map[string]string {
	metadata := map[string]string{
		"schema_version": SchemaVersion,
	}
	if artifact != nil {
		if artifact.SchemaVersion != "" {
			metadata["schema_version"] = artifact.SchemaVersion
		}
		if artifact.Run.ID != "" {
			metadata["run_id"] = artifact.Run.ID
		}
		if artifact.Run.Mode != "" {
			metadata["mode"] = artifact.Run.Mode
		}
		metadata["events"] = strconv.Itoa(len(artifact.Events))
		metadata["steps"] = strconv.Itoa(artifact.Summary.Steps)
	}
	for k, v := range e.objectMetadata {
		metadata[k] = v
	}
	return metadata
}

func (e *ObjectStorageExporter) defaultObjectKey(runTrace *core.RunTrace, artifact *Artifact) (string, error) {
	start := "no-start"
	if artifact != nil && !artifact.Run.StartedAt.IsZero() {
		start = artifact.Run.StartedAt.UTC().Format("20060102T150405")
	}
	runID := "run"
	if runTrace != nil && fileSafeTracePart(runTrace.RunID) != "" {
		runID = fileSafeTracePart(runTrace.RunID)
	} else if artifact != nil && fileSafeTracePart(artifact.Run.ID) != "" {
		runID = fileSafeTracePart(artifact.Run.ID)
	}
	filename := fmt.Sprintf("trace_%s_%s.trace.json", runID, start)
	return joinObjectKeyParts(e.prefix, runID, SchemaVersion, filename), nil
}

func joinObjectKeyParts(parts ...string) string {
	clean := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.Trim(strings.TrimSpace(part), "/")
		if part == "" {
			continue
		}
		clean = append(clean, part)
	}
	if len(clean) == 0 {
		return ""
	}
	return path.Join(clean...)
}

func cloneStringMap(src map[string]string) map[string]string {
	if len(src) == 0 {
		return nil
	}
	dst := make(map[string]string, len(src))
	for k, v := range src {
		dst[k] = v
	}
	return dst
}
