//go:build integration

package trace

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"sort"
	"strings"
	"testing"
	"time"
)

type gcsCLIObjectStore struct {
	bucket string
	binary string
}

func (s gcsCLIObjectStore) PutObject(ctx context.Context, object ObjectPut) error {
	if strings.TrimSpace(s.bucket) == "" {
		return fmt.Errorf("gcs bucket is required")
	}
	binary := s.binary
	if binary == "" {
		binary = "gsutil"
	}
	args := make([]string, 0, 2+len(object.Metadata)*2+3)
	if object.ContentType != "" {
		args = append(args, "-h", "Content-Type:"+object.ContentType)
	}
	keys := make([]string, 0, len(object.Metadata))
	for key := range object.Metadata {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		args = append(args, "-h", "x-goog-meta-"+key+":"+object.Metadata[key])
	}
	args = append(args, "cp", "-", gcsURI(s.bucket, object.Key))
	cmd := exec.CommandContext(ctx, binary, args...)
	cmd.Stdin = bytes.NewReader(object.Body)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("gsutil cp: %w: %s", err, strings.TrimSpace(stderr.String()))
	}
	return nil
}

func TestGCSObjectStorageExporterIntegration(t *testing.T) {
	if os.Getenv("GOLLEM_GCS_TRACE_INTEGRATION") != "1" {
		t.Skip("set GOLLEM_GCS_TRACE_INTEGRATION=1 and GOLLEM_GCS_TRACE_BUCKET to run")
	}
	bucket := os.Getenv("GOLLEM_GCS_TRACE_BUCKET")
	if bucket == "" {
		t.Fatal("GOLLEM_GCS_TRACE_BUCKET is required")
	}
	gsutil, err := exec.LookPath("gsutil")
	if err != nil {
		t.Fatalf("gsutil is required for GCS integration test: %v", err)
	}

	prefix := strings.Trim(os.Getenv("GOLLEM_GCS_TRACE_PREFIX"), "/")
	if prefix == "" {
		prefix = fmt.Sprintf("gollem-trace-integration/%d", time.Now().UnixNano())
	}
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		_ = runGSUtil(ctx, gsutil, "rm", "-r", gcsURI(bucket, prefix))
	})

	now := time.Date(2026, 5, 6, 12, 0, 0, 0, time.UTC)
	store := gcsCLIObjectStore{bucket: bucket, binary: gsutil}
	exporter := NewObjectStorageExporter(store,
		WithObjectKeyPrefix(prefix),
		WithObjectMetadata(map[string]string{"integration": "gcs"}),
		WithObjectExporterMetadata(map[string]any{"storage": "gcs"}),
	)

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	runTrace := sampleRunTrace(now)
	if err := exporter.Export(ctx, runTrace); err != nil {
		t.Fatalf("first GCS export failed: %v", err)
	}
	if err := exporter.Export(ctx, runTrace); err != nil {
		t.Fatalf("second GCS export failed: %v", err)
	}

	key := joinObjectKeyParts(prefix, "run-1", SchemaVersion, "trace_run-1_20260506T120000.trace.json")
	body, err := runGSUtilOutput(ctx, gsutil, "cat", gcsURI(bucket, key))
	if err != nil {
		t.Fatalf("read GCS object: %v", err)
	}
	artifact, err := Read(bytes.NewReader(body))
	if err != nil {
		t.Fatalf("decode GCS trace artifact: %v", err)
	}
	if artifact.SchemaVersion != SchemaVersion || artifact.Run.ID != "run-1" {
		t.Fatalf("unexpected artifact identity: schema=%q run=%q", artifact.SchemaVersion, artifact.Run.ID)
	}
	if artifact.Metadata["storage"] != "gcs" {
		t.Fatalf("expected artifact storage metadata, got %+v", artifact.Metadata)
	}

	stat, err := runGSUtilOutput(ctx, gsutil, "stat", gcsURI(bucket, key))
	if err != nil {
		t.Fatalf("stat GCS object: %v", err)
	}
	statText := string(stat)
	for _, want := range []string{
		"Content-Type:",
		"application/vnd.gollem.trace+json",
		"integration:",
		"gcs",
		"run_id:",
		"run-1",
		"schema_version:",
		"gollem.trace.v1",
	} {
		if !strings.Contains(statText, want) {
			t.Fatalf("GCS object metadata missing %q:\n%s", want, statText)
		}
	}
}

func gcsURI(bucket, key string) string {
	return "gs://" + strings.Trim(bucket, "/") + "/" + strings.Trim(key, "/")
}

func runGSUtil(ctx context.Context, binary string, args ...string) error {
	_, err := runGSUtilOutput(ctx, binary, args...)
	return err
}

func runGSUtilOutput(ctx context.Context, binary string, args ...string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, binary, args...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("gsutil %s: %w: %s", strings.Join(args, " "), err, strings.TrimSpace(stderr.String()))
	}
	return stdout.Bytes(), nil
}
