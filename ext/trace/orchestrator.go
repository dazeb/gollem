package trace

import (
	"bytes"

	"github.com/fugue-labs/gollem/core"
	"github.com/fugue-labs/gollem/ext/orchestrator"
)

const (
	// OrchestratorArtifactKind is the artifact kind used for canonical Gollem
	// trace artifacts persisted from orchestrator-backed task runs.
	OrchestratorArtifactKind = "gollem.trace"
	// OrchestratorArtifactContentType identifies JSON trace artifacts in task stores.
	OrchestratorArtifactContentType = "application/vnd.gollem.trace+json"
)

// OrchestratorArtifactSpec serializes a traced task run into an immutable
// orchestrator artifact. It returns false when the run was not traced.
func OrchestratorArtifactSpec[T any](task *orchestrator.Task, result *core.RunResult[T], metadata map[string]any) (orchestrator.ArtifactSpec, bool, error) {
	if result == nil || result.Trace == nil {
		return orchestrator.ArtifactSpec{}, false, nil
	}
	merged := cloneMetadata(metadata)
	if merged == nil {
		merged = make(map[string]any)
	}
	if task != nil {
		merged["orchestrator_task_id"] = task.ID
		merged["orchestrator_task_kind"] = task.Kind
		merged["orchestrator_task_attempt"] = task.Attempt
		if task.Run != nil {
			merged["orchestrator_run_id"] = task.Run.ID
			merged["orchestrator_worker_id"] = task.Run.WorkerID
			merged["orchestrator_attempt"] = task.Run.Attempt
		}
	}
	artifact, err := FromRunTrace(result.Trace, merged)
	if err != nil {
		return orchestrator.ArtifactSpec{}, false, err
	}
	WithCost(artifact, result.Cost)
	artifact.Run.Mode = "orchestrator"

	var body bytes.Buffer
	if err := Write(&body, artifact); err != nil {
		return orchestrator.ArtifactSpec{}, false, err
	}
	name := "trace.json"
	if task != nil && task.ID != "" {
		name = task.ID + ".trace.json"
	}
	return orchestrator.ArtifactSpec{
		Kind:        OrchestratorArtifactKind,
		Name:        name,
		ContentType: OrchestratorArtifactContentType,
		Body:        body.Bytes(),
		Metadata: compactMap(map[string]any{
			"schema_version": artifact.SchemaVersion,
			"run_id":         artifact.Run.ID,
			"mode":           artifact.Run.Mode,
			"events":         len(artifact.Events),
			"steps":          artifact.Summary.Steps,
		}),
	}, true, nil
}
