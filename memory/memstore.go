package memory

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"time"
)

// MemoryStore is a thread-safe in-memory implementation of Store.
type MemoryStore struct {
	mu   sync.RWMutex
	docs map[string]*Document // keyed by "namespace:key"
}

// NewMemoryStore creates a new in-memory Store.
func NewMemoryStore() Store {
	return &MemoryStore{
		docs: make(map[string]*Document),
	}
}

// Put stores a value under the given namespace and key.
func (s *MemoryStore) Put(_ context.Context, namespace []string, key string, value any) error {
	valueMap, err := toMap(value)
	if err != nil {
		return fmt.Errorf("memory store put: %w", err)
	}

	compositeKey := makeCompositeKey(namespace, key)
	now := time.Now()

	s.mu.Lock()
	defer s.mu.Unlock()

	if existing, ok := s.docs[compositeKey]; ok {
		existing.Value = valueMap
		existing.UpdatedAt = now
	} else {
		s.docs[compositeKey] = &Document{
			Namespace: namespace,
			Key:       key,
			Value:     valueMap,
			CreatedAt: now,
			UpdatedAt: now,
		}
	}

	return nil
}

// Get retrieves a single document by namespace and key.
func (s *MemoryStore) Get(_ context.Context, namespace []string, key string) (*Document, error) {
	compositeKey := makeCompositeKey(namespace, key)

	s.mu.RLock()
	defer s.mu.RUnlock()

	doc, ok := s.docs[compositeKey]
	if !ok {
		return nil, nil
	}

	return copyDocument(doc), nil
}

// List returns all documents in the given namespace.
func (s *MemoryStore) List(_ context.Context, namespace []string) ([]*Document, error) {
	prefix := makeNamespacePrefix(namespace)

	s.mu.RLock()
	defer s.mu.RUnlock()

	var results []*Document
	for k, doc := range s.docs {
		if strings.HasPrefix(k, prefix) {
			results = append(results, copyDocument(doc))
		}
	}

	return results, nil
}

// Search performs a substring search across documents in the given namespace.
// It JSON-serializes each document's value and checks for a case-insensitive
// substring match against the query.
func (s *MemoryStore) Search(_ context.Context, namespace []string, query string, limit int) ([]*Document, error) {
	prefix := makeNamespacePrefix(namespace)
	lowerQuery := strings.ToLower(query)

	s.mu.RLock()
	defer s.mu.RUnlock()

	var results []*Document
	for k, doc := range s.docs {
		if !strings.HasPrefix(k, prefix) {
			continue
		}

		// Serialize value to JSON for substring matching.
		data, err := json.Marshal(doc.Value)
		if err != nil {
			continue
		}

		if strings.Contains(strings.ToLower(string(data)), lowerQuery) {
			results = append(results, copyDocument(doc))
			if limit > 0 && len(results) >= limit {
				break
			}
		}
	}

	return results, nil
}

// Delete removes a document by namespace and key.
func (s *MemoryStore) Delete(_ context.Context, namespace []string, key string) error {
	compositeKey := makeCompositeKey(namespace, key)

	s.mu.Lock()
	defer s.mu.Unlock()

	delete(s.docs, compositeKey)
	return nil
}

// makeCompositeKey builds a composite key from namespace and key.
func makeCompositeKey(namespace []string, key string) string {
	return makeNamespacePrefix(namespace) + key
}

// makeNamespacePrefix builds a prefix string from namespace parts.
func makeNamespacePrefix(namespace []string) string {
	if len(namespace) == 0 {
		return ":"
	}
	return strings.Join(namespace, ":") + ":"
}

// toMap converts a value to map[string]any. If the value is already a
// map[string]any it is returned directly; otherwise it is round-tripped
// through JSON.
func toMap(value any) (map[string]any, error) {
	if m, ok := value.(map[string]any); ok {
		return m, nil
	}

	data, err := json.Marshal(value)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal value: %w", err)
	}

	var m map[string]any
	if unmarshalErr := json.Unmarshal(data, &m); unmarshalErr != nil {
		// If it's not an object, wrap in a "value" key.
		return map[string]any{"value": value}, nil //nolint:nilerr // intentional: non-object JSON is wrapped, not an error
	}
	return m, nil
}

// copyDocument returns a shallow copy of a Document.
func copyDocument(doc *Document) *Document {
	ns := make([]string, len(doc.Namespace))
	copy(ns, doc.Namespace)

	valueCopy := make(map[string]any, len(doc.Value))
	for k, v := range doc.Value {
		valueCopy[k] = v
	}

	return &Document{
		Namespace: ns,
		Key:       doc.Key,
		Value:     valueCopy,
		CreatedAt: doc.CreatedAt,
		UpdatedAt: doc.UpdatedAt,
	}
}

// Verify MemoryStore implements Store.
var _ Store = (*MemoryStore)(nil)
