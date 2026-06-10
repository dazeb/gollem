package trace

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/fugue-labs/gollem/core"
)

func TestDirectoryExporterWritesCanonicalArtifact(t *testing.T) {
	dir := t.TempDir()
	bus := core.NewEventBus()
	defer bus.Close()
	recorder := NewRuntimeRecorder(bus)
	defer recorder.Close()

	exporter := NewDirectoryExporter(dir,
		WithRuntimeRecorder(recorder),
		WithExporterMetadata(map[string]any{"sdk": "test"}),
	)
	agent := core.NewAgent[string](
		core.NewTestModel(core.TextResponse("ok")),
		core.WithEventBus[string](bus),
		core.WithTraceExporter[string](exporter),
	)

	result, err := agent.Run(context.Background(), "record canonical artifact")
	if err != nil {
		t.Fatal(err)
	}
	if result.Trace == nil {
		t.Fatal("expected trace to be enabled")
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 {
		t.Fatalf("expected 1 trace artifact, got %d", len(entries))
	}

	artifact, err := ReadFile(filepath.Join(dir, entries[0].Name()))
	if err != nil {
		t.Fatal(err)
	}
	if artifact.SchemaVersion != SchemaVersion {
		t.Fatalf("schema version = %q, want %q", artifact.SchemaVersion, SchemaVersion)
	}
	if artifact.Run.ID != result.RunID {
		t.Fatalf("artifact run ID = %q, want %q", artifact.Run.ID, result.RunID)
	}
	if artifact.Trace == nil {
		t.Fatal("expected embedded core trace")
	}
	if artifact.Metadata["sdk"] != "test" {
		t.Fatalf("metadata sdk = %v, want test", artifact.Metadata["sdk"])
	}
	if len(artifact.Events) == 0 {
		t.Fatal("expected projected trace events")
	}
}

func TestDirectoryExporterRequiresDirectory(t *testing.T) {
	err := NewDirectoryExporter("").Export(context.Background(), &core.RunTrace{RunID: "run-1"})
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestDirectoryExporterErrorsAndFilenameFallbacks(t *testing.T) {
	var nilExporter *DirectoryExporter
	if err := nilExporter.Export(context.Background(), &core.RunTrace{RunID: "run-1"}); err == nil {
		t.Fatal("expected nil exporter error")
	}
	if err := NewDirectoryExporter(t.TempDir()).Export(context.Background(), nil); err == nil {
		t.Fatal("expected nil run trace error")
	}
	dir := t.TempDir()
	exporter := NewDirectoryExporter(dir)
	if err := exporter.Export(context.Background(), &core.RunTrace{Success: true}); err != nil {
		t.Fatalf("Export() fallback filename error = %v", err)
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("read dir: %v", err)
	}
	if len(entries) != 1 || !strings.HasPrefix(entries[0].Name(), "trace_run_") {
		t.Fatalf("unexpected fallback artifact filename: %+v", entries)
	}
	if got := fileSafeTracePart(" ../bad run!* "); got != "bad_run" {
		t.Fatalf("fileSafeTracePart = %q", got)
	}
}

type memoryObjectStore struct {
	mu   sync.Mutex
	puts []ObjectPut
	err  error
}

func (s *memoryObjectStore) PutObject(_ context.Context, object ObjectPut) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.err != nil {
		return s.err
	}
	cloned := ObjectPut{
		Key:         object.Key,
		ContentType: object.ContentType,
		Body:        append([]byte(nil), object.Body...),
		Metadata:    cloneStringMap(object.Metadata),
	}
	s.puts = append(s.puts, cloned)
	return nil
}

func (s *memoryObjectStore) last() ObjectPut {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.puts[len(s.puts)-1]
}

func TestObjectStorageExporterWritesIdempotentCanonicalArtifact(t *testing.T) {
	now := time.Date(2026, 5, 6, 12, 0, 0, 0, time.UTC)
	store := &memoryObjectStore{}
	exporter := NewObjectStorageExporter(store,
		WithObjectKeyPrefix("/prod/gollem/"),
		WithObjectExporterMetadata(map[string]any{"deployment": "prod"}),
		WithObjectMetadata(map[string]string{"tenant": "acme"}),
	)
	runTrace := sampleRunTrace(now)

	if err := exporter.Export(context.Background(), runTrace); err != nil {
		t.Fatalf("first Export() error = %v", err)
	}
	if err := exporter.Export(context.Background(), runTrace); err != nil {
		t.Fatalf("second Export() error = %v", err)
	}
	if len(store.puts) != 2 {
		t.Fatalf("expected 2 object puts, got %d", len(store.puts))
	}
	if store.puts[0].Key != store.puts[1].Key {
		t.Fatalf("expected idempotent object key, got %q and %q", store.puts[0].Key, store.puts[1].Key)
	}
	if string(store.puts[0].Body) != string(store.puts[1].Body) {
		t.Fatal("expected idempotent object body")
	}
	if want := "prod/gollem/run-1/gollem.trace.v1/trace_run-1_20260506T120000.trace.json"; store.puts[0].Key != want {
		t.Fatalf("object key = %q, want %q", store.puts[0].Key, want)
	}
	if store.puts[0].ContentType != OrchestratorArtifactContentType {
		t.Fatalf("content type = %q, want %q", store.puts[0].ContentType, OrchestratorArtifactContentType)
	}
	if store.puts[0].Metadata["tenant"] != "acme" {
		t.Fatalf("object metadata tenant = %q, want acme", store.puts[0].Metadata["tenant"])
	}
	if store.puts[0].Metadata["schema_version"] != SchemaVersion || store.puts[0].Metadata["run_id"] != "run-1" {
		t.Fatalf("object metadata missing trace identity: %+v", store.puts[0].Metadata)
	}

	artifact, err := Read(strings.NewReader(string(store.puts[0].Body)))
	if err != nil {
		t.Fatalf("read stored artifact: %v", err)
	}
	if artifact.SchemaVersion != SchemaVersion {
		t.Fatalf("schema version = %q, want %q", artifact.SchemaVersion, SchemaVersion)
	}
	if artifact.Run.ID != "run-1" {
		t.Fatalf("run ID = %q, want run-1", artifact.Run.ID)
	}
	if artifact.Metadata["deployment"] != "prod" {
		t.Fatalf("artifact metadata deployment = %v, want prod", artifact.Metadata["deployment"])
	}
}

func TestObjectStorageExporterSupportsCustomKeyAndErrors(t *testing.T) {
	store := &memoryObjectStore{}
	exporter := NewObjectStorageExporter(store, WithObjectKeyFunc(func(_ *core.RunTrace, artifact *Artifact) (string, error) {
		return "custom/" + artifact.Run.ID + ".json", nil
	}))

	if err := exporter.Export(context.Background(), &core.RunTrace{RunID: "run-custom"}); err != nil {
		t.Fatalf("Export() error = %v", err)
	}
	if got := store.last().Key; got != "custom/run-custom.json" {
		t.Fatalf("custom key = %q, want custom/run-custom.json", got)
	}

	errStore := &memoryObjectStore{err: errors.New("store unavailable")}
	err := NewObjectStorageExporter(errStore).Export(context.Background(), &core.RunTrace{RunID: "run-1"})
	if err == nil || !strings.Contains(err.Error(), "store unavailable") {
		t.Fatalf("expected store error, got %v", err)
	}
}

func TestObjectStorageExporterErrorsAndOptionFallbacks(t *testing.T) {
	var nilExporter *ObjectStorageExporter
	if err := nilExporter.Export(context.Background(), &core.RunTrace{RunID: "run-1"}); err == nil {
		t.Fatal("expected nil object exporter error")
	}
	if err := NewObjectStorageExporter(nil).Export(context.Background(), &core.RunTrace{RunID: "run-1"}); err == nil {
		t.Fatal("expected missing object store error")
	}
	store := &memoryObjectStore{}
	if err := NewObjectStorageExporter(store).Export(context.Background(), nil); err == nil {
		t.Fatal("expected nil run trace error")
	}
	keyErr := NewObjectStorageExporter(store, WithObjectKeyFunc(func(*core.RunTrace, *Artifact) (string, error) {
		return "", errors.New("bad key")
	})).Export(context.Background(), &core.RunTrace{RunID: "run-1"})
	if keyErr == nil || !strings.Contains(keyErr.Error(), "bad key") {
		t.Fatalf("expected key function error, got %v", keyErr)
	}
	emptyKeyErr := NewObjectStorageExporter(store, WithObjectKeyFunc(func(*core.RunTrace, *Artifact) (string, error) {
		return " / ", nil
	})).Export(context.Background(), &core.RunTrace{RunID: "run-1"})
	if emptyKeyErr == nil || !strings.Contains(emptyKeyErr.Error(), "object key is required") {
		t.Fatalf("expected empty key error, got %v", emptyKeyErr)
	}
	fallbackContentType := NewObjectStorageExporter(store, WithObjectContentType(" "))
	if err := fallbackContentType.Export(context.Background(), &core.RunTrace{RunID: "run-content-type", Success: true}); err != nil {
		t.Fatalf("fallback content type export: %v", err)
	}
	if got := store.last().ContentType; got != OrchestratorArtifactContentType {
		t.Fatalf("content type = %q, want %q", got, OrchestratorArtifactContentType)
	}
}
