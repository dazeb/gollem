package trace

import (
	"path/filepath"
	"testing"
)

func TestCompatibilityFixturesRead(t *testing.T) {
	fixtures, err := filepath.Glob(filepath.Join("testdata", "compat", "*.trace.json"))
	if err != nil {
		t.Fatalf("glob compatibility fixtures: %v", err)
	}
	if len(fixtures) == 0 {
		t.Fatal("expected at least one compatibility fixture")
	}

	for _, fixture := range fixtures {
		t.Run(filepath.Base(fixture), func(t *testing.T) {
			artifact, err := ReadFile(fixture)
			if err != nil {
				t.Fatalf("ReadFile(%s) error = %v", fixture, err)
			}
			if artifact.SchemaVersion != SchemaVersion {
				t.Fatalf("schema version = %q, want %q", artifact.SchemaVersion, SchemaVersion)
			}
			if artifact.Run.ID == "" {
				t.Fatal("expected fixture run id")
			}
		})
	}
}
