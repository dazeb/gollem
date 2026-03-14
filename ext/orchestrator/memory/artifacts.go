package memory

import (
	"context"
	"fmt"
	"time"

	"github.com/fugue-labs/gollem/core"
	"github.com/fugue-labs/gollem/ext/orchestrator"
)

// CreateArtifact implements orchestrator.ArtifactStore.
func (s *Store) CreateArtifact(_ context.Context, req orchestrator.CreateArtifactRequest) (*orchestrator.Artifact, error) {
	if req.TaskID == "" {
		return nil, orchestrator.ErrArtifactTaskRequired
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	if _, ok := s.tasks[req.TaskID]; !ok {
		return nil, orchestrator.ErrTaskNotFound
	}

	s.nextArtifact++
	now := time.Now()
	artifact := &orchestrator.Artifact{
		ID:          fmt.Sprintf("artifact-%d", s.nextArtifact),
		TaskID:      req.TaskID,
		RunID:       req.RunID,
		Kind:        req.Kind,
		Name:        req.Name,
		ContentType: req.ContentType,
		Body:        cloneBytes(req.Body),
		Metadata:    cloneAnyMap(req.Metadata),
		CreatedAt:   now,
	}
	s.artifacts[artifact.ID] = artifact
	s.artifactOrder = append(s.artifactOrder, artifact.ID)
	s.publishArtifactCreated(artifact)
	return cloneArtifact(artifact), nil
}

// GetArtifact implements orchestrator.ArtifactStore.
func (s *Store) GetArtifact(_ context.Context, id string) (*orchestrator.Artifact, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	artifact, ok := s.artifacts[id]
	if !ok {
		return nil, orchestrator.ErrArtifactNotFound
	}
	return cloneArtifact(artifact), nil
}

// ListArtifacts implements orchestrator.ArtifactStore.
func (s *Store) ListArtifacts(_ context.Context, filter orchestrator.ArtifactFilter) ([]*orchestrator.Artifact, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	var artifacts []*orchestrator.Artifact
	for _, id := range s.artifactOrder {
		artifact, ok := s.artifacts[id]
		if !ok || !matchesArtifactFilter(artifact, filter) {
			continue
		}
		artifacts = append(artifacts, cloneArtifact(artifact))
	}
	return artifacts, nil
}

func (s *Store) publishArtifactCreated(artifact *orchestrator.Artifact) {
	if s.eventBus == nil || artifact == nil {
		return
	}
	core.PublishAsync(s.eventBus, orchestrator.ArtifactCreatedEvent{
		ArtifactID:  artifact.ID,
		TaskID:      artifact.TaskID,
		RunID:       artifact.RunID,
		Kind:        artifact.Kind,
		Name:        artifact.Name,
		ContentType: artifact.ContentType,
		SizeBytes:   len(artifact.Body),
		CreatedAt:   artifact.CreatedAt,
	})
}

func matchesArtifactFilter(artifact *orchestrator.Artifact, filter orchestrator.ArtifactFilter) bool {
	if artifact == nil {
		return false
	}
	if filter.TaskID != "" && artifact.TaskID != filter.TaskID {
		return false
	}
	if filter.RunID != "" && artifact.RunID != filter.RunID {
		return false
	}
	if filter.Kind != "" && artifact.Kind != filter.Kind {
		return false
	}
	if filter.Name != "" && artifact.Name != filter.Name {
		return false
	}
	return true
}

func cloneArtifact(src *orchestrator.Artifact) *orchestrator.Artifact {
	if src == nil {
		return nil
	}
	return &orchestrator.Artifact{
		ID:          src.ID,
		TaskID:      src.TaskID,
		RunID:       src.RunID,
		Kind:        src.Kind,
		Name:        src.Name,
		ContentType: src.ContentType,
		Body:        cloneBytes(src.Body),
		Metadata:    cloneAnyMap(src.Metadata),
		CreatedAt:   src.CreatedAt,
	}
}

func cloneBytes(src []byte) []byte {
	if len(src) == 0 {
		return nil
	}
	cloned := make([]byte, len(src))
	copy(cloned, src)
	return cloned
}
