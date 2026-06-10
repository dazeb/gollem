package trace

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/fugue-labs/gollem/core"
)

func TestCompatibilityFixturesRead(t *testing.T) {
	fixtures, err := filepath.Glob(filepath.Join("testdata", "compat", "*.trace.json"))
	if err != nil {
		t.Fatalf("glob compatibility fixtures: %v", err)
	}
	if len(fixtures) == 0 {
		t.Fatal("expected at least one compatibility fixture")
	}

	sawCausalFixture := false
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
			if strings.Contains(fixture, "causal") {
				sawCausalFixture = true
				// Current-format fixtures with persisted causal links must
				// decode the causal wire fields and validate exactly as stored.
				if got := artifact.Events[1].CausalParentEventID; got != artifact.Events[0].ID {
					t.Fatalf("causal_parent_event_id did not decode: got %q, want %q", got, artifact.Events[0].ID)
				}
				if children := artifact.Events[0].CausalChildEventIDs; len(children) != 1 || children[0] != artifact.Events[1].ID {
					t.Fatalf("causal_child_event_ids did not decode: %+v", children)
				}
				if err := ValidateArtifact(artifact); err != nil {
					t.Fatalf("ValidateArtifact(%s) error = %v", fixture, err)
				}
				return
			}
			// Legacy fixtures predate persisted causal links and replay
			// policies; the upgrade path is to re-normalize before validating.
			for _, event := range artifact.Events {
				if event.CausalParentEventID != "" || len(event.CausalChildEventIDs) > 0 {
					t.Fatalf("legacy fixture %s unexpectedly carries causal links: %+v", fixture, event)
				}
			}
			artifact.Events = core.NormalizeTraceEvents(artifact.Events)
			if err := ValidateArtifact(artifact); err != nil {
				t.Fatalf("ValidateArtifact(normalized %s) error = %v", fixture, err)
			}
		})
	}
	if !sawCausalFixture {
		t.Fatal("expected a compatibility fixture with persisted causal links")
	}
}
