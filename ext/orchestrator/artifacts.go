package orchestrator

import (
	"context"
	"time"
)

// Artifact is an immutable blob scoped to a task and optional run attempt.
type Artifact struct {
	ID          string
	TaskID      string
	RunID       string
	Kind        string
	Name        string
	ContentType string
	Body        []byte
	Metadata    map[string]any
	CreatedAt   time.Time
}

// ArtifactSpec describes an artifact emitted by a runner before store persistence.
type ArtifactSpec struct {
	Kind        string
	Name        string
	ContentType string
	Body        []byte
	Metadata    map[string]any
}

// CreateArtifactRequest describes a new artifact to persist.
type CreateArtifactRequest struct {
	TaskID      string
	RunID       string
	Kind        string
	Name        string
	ContentType string
	Body        []byte
	Metadata    map[string]any
}

// ArtifactFilter narrows ListArtifacts results.
type ArtifactFilter struct {
	TaskID string
	RunID  string
	Kind   string
	Name   string
}

// ArtifactStore persists immutable task-scoped artifacts.
type ArtifactStore interface {
	CreateArtifact(ctx context.Context, req CreateArtifactRequest) (*Artifact, error)
	GetArtifact(ctx context.Context, id string) (*Artifact, error)
	ListArtifacts(ctx context.Context, filter ArtifactFilter) ([]*Artifact, error)
}

func cloneArtifactSpecs(src []ArtifactSpec) []ArtifactSpec {
	if len(src) == 0 {
		return nil
	}
	cloned := make([]ArtifactSpec, len(src))
	for i, artifact := range src {
		cloned[i] = ArtifactSpec{
			Kind:        artifact.Kind,
			Name:        artifact.Name,
			ContentType: artifact.ContentType,
			Body:        cloneBytes(artifact.Body),
			Metadata:    cloneAnyMap(artifact.Metadata),
		}
	}
	return cloned
}

func cloneBytes(src []byte) []byte {
	if len(src) == 0 {
		return nil
	}
	cloned := make([]byte, len(src))
	copy(cloned, src)
	return cloned
}
