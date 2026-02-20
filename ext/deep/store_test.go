package deep

import (
	"os"
	"path/filepath"
	"testing"
)

func TestFileStore_StoreRetrieve(t *testing.T) {
	dir := t.TempDir()
	store, err := NewFileStore(dir)
	if err != nil {
		t.Fatalf("NewFileStore: %v", err)
	}

	content := "This is some test content that was offloaded."
	if err := store.Store("key1", content); err != nil {
		t.Fatalf("Store: %v", err)
	}

	got, err := store.Retrieve("key1")
	if err != nil {
		t.Fatalf("Retrieve: %v", err)
	}
	if got != content {
		t.Errorf("Retrieve = %q, want %q", got, content)
	}
}

func TestFileStore_RetrieveNotFound(t *testing.T) {
	dir := t.TempDir()
	store, err := NewFileStore(dir)
	if err != nil {
		t.Fatalf("NewFileStore: %v", err)
	}

	_, err = store.Retrieve("nonexistent")
	if err == nil {
		t.Fatal("expected error for nonexistent key")
	}
}

func TestFileStore_Delete(t *testing.T) {
	dir := t.TempDir()
	store, err := NewFileStore(dir)
	if err != nil {
		t.Fatalf("NewFileStore: %v", err)
	}

	if err := store.Store("key1", "content"); err != nil {
		t.Fatalf("Store: %v", err)
	}
	if err := store.Delete("key1"); err != nil {
		t.Fatalf("Delete: %v", err)
	}

	// Verify file is gone.
	_, err = os.Stat(filepath.Join(dir, "key1.txt"))
	if !os.IsNotExist(err) {
		t.Errorf("expected file to be deleted, got err: %v", err)
	}

	// Retrieve should fail.
	_, err = store.Retrieve("key1")
	if err == nil {
		t.Fatal("expected error after delete")
	}
}

func TestFileStore_DeleteNonexistent(t *testing.T) {
	dir := t.TempDir()
	store, err := NewFileStore(dir)
	if err != nil {
		t.Fatalf("NewFileStore: %v", err)
	}

	// Should not error.
	if err := store.Delete("nonexistent"); err != nil {
		t.Fatalf("Delete nonexistent: %v", err)
	}
}

func TestFileStore_Cleanup(t *testing.T) {
	dir := t.TempDir()
	store, err := NewFileStore(dir)
	if err != nil {
		t.Fatalf("NewFileStore: %v", err)
	}

	// Store multiple items.
	for i := range 5 {
		key := "key" + string(rune('0'+i))
		if err := store.Store(key, "content "+key); err != nil {
			t.Fatalf("Store %s: %v", key, err)
		}
	}

	if err := store.Cleanup(); err != nil {
		t.Fatalf("Cleanup: %v", err)
	}

	// All files should be removed.
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("ReadDir: %v", err)
	}
	if len(entries) != 0 {
		t.Errorf("expected empty dir after cleanup, got %d entries", len(entries))
	}
}

func TestFileStore_DefaultDir(t *testing.T) {
	store, err := NewFileStore("")
	if err != nil {
		t.Fatalf("NewFileStore with empty dir: %v", err)
	}

	if err := store.Store("test", "content"); err != nil {
		t.Fatalf("Store: %v", err)
	}
	got, err := store.Retrieve("test")
	if err != nil {
		t.Fatalf("Retrieve: %v", err)
	}
	if got != "content" {
		t.Errorf("Retrieve = %q, want %q", got, "content")
	}

	// Cleanup.
	_ = store.Cleanup()
}
