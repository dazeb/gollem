package deep

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
)

// ContextStore persists and retrieves offloaded context.
type ContextStore interface {
	Store(key string, content string) error
	Retrieve(key string) (string, error)
	Delete(key string) error
	Cleanup() error
}

// FileStore stores offloaded content on the filesystem.
type FileStore struct {
	dir  string
	mu   sync.Mutex
	keys map[string]string // key -> file path
}

// NewFileStore creates a file-based context store in the given directory.
// If dir is empty, uses os.TempDir().
func NewFileStore(dir string) (*FileStore, error) {
	if dir == "" {
		dir = filepath.Join(os.TempDir(), "gollem-context")
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, fmt.Errorf("creating context store directory: %w", err)
	}
	return &FileStore{
		dir:  dir,
		keys: make(map[string]string),
	}, nil
}

// Store saves content under the given key.
func (fs *FileStore) Store(key string, content string) error {
	fs.mu.Lock()
	defer fs.mu.Unlock()

	path := filepath.Join(fs.dir, key+".txt")
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		return fmt.Errorf("storing content for key %q: %w", key, err)
	}
	fs.keys[key] = path
	return nil
}

// Retrieve loads content for the given key.
func (fs *FileStore) Retrieve(key string) (string, error) {
	fs.mu.Lock()
	path, ok := fs.keys[key]
	fs.mu.Unlock()

	if !ok {
		return "", fmt.Errorf("key %q not found", key)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("reading content for key %q: %w", key, err)
	}
	return string(data), nil
}

// Delete removes content for the given key.
func (fs *FileStore) Delete(key string) error {
	fs.mu.Lock()
	path, ok := fs.keys[key]
	if ok {
		delete(fs.keys, key)
	}
	fs.mu.Unlock()

	if !ok {
		return nil
	}
	if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("deleting content for key %q: %w", key, err)
	}
	return nil
}

// Cleanup removes all stored content and the store directory.
func (fs *FileStore) Cleanup() error {
	fs.mu.Lock()
	keys := make(map[string]string, len(fs.keys))
	for k, v := range fs.keys {
		keys[k] = v
	}
	fs.keys = make(map[string]string)
	fs.mu.Unlock()

	var errs []error
	for _, path := range keys {
		if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
			errs = append(errs, err)
		}
	}
	if len(errs) > 0 {
		return fmt.Errorf("cleanup errors: %v", errs)
	}
	return nil
}
