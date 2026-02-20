package memory

import (
	"context"
	"time"
)

// Store is a persistent key-value document store with namespace scoping.
// Namespaces provide hierarchical isolation, allowing documents to be
// organized into logical groups (e.g., ["user", "preferences"]).
type Store interface {
	// Put stores a value under the given namespace and key.
	// If a document with the same namespace and key already exists, it is overwritten.
	Put(ctx context.Context, namespace []string, key string, value any) error

	// Get retrieves a single document by namespace and key.
	// Returns nil if the document does not exist.
	Get(ctx context.Context, namespace []string, key string) (*Document, error)

	// List returns all documents in the given namespace.
	List(ctx context.Context, namespace []string) ([]*Document, error)

	// Search performs a text search across documents in the given namespace.
	// Returns up to limit matching documents.
	Search(ctx context.Context, namespace []string, query string, limit int) ([]*Document, error)

	// Delete removes a document by namespace and key.
	// Returns nil if the document does not exist.
	Delete(ctx context.Context, namespace []string, key string) error
}

// Document represents a stored item in the memory store.
type Document struct {
	Namespace []string       `json:"namespace"`
	Key       string         `json:"key"`
	Value     map[string]any `json:"value"`
	CreatedAt time.Time      `json:"created_at"`
	UpdatedAt time.Time      `json:"updated_at"`
}
