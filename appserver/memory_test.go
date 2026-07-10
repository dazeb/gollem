package appserver

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestMemoryServiceResetClearsContentsSafely(t *testing.T) {
	root := filepath.Join(t.TempDir(), "memories")
	if err := os.MkdirAll(filepath.Join(root, "rollout_summaries"), 0o700); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, "MEMORY.md"), []byte("stale"), 0o600); err != nil {
		t.Fatalf("WriteFile MEMORY: %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, "rollout_summaries", "old.md"), []byte("old"), 0o600); err != nil {
		t.Fatalf("WriteFile rollout: %v", err)
	}
	outside := filepath.Join(t.TempDir(), "outside.txt")
	if err := os.WriteFile(outside, []byte("outside"), 0o600); err != nil {
		t.Fatalf("WriteFile outside: %v", err)
	}
	if err := os.Symlink(outside, filepath.Join(root, "outside-link")); err != nil {
		t.Fatalf("Symlink: %v", err)
	}

	service, err := NewMemoryService(root)
	if err != nil {
		t.Fatalf("NewMemoryService: %v", err)
	}
	if _, err := service.Reset(context.Background()); err != nil {
		t.Fatalf("Reset: %v", err)
	}
	entries, err := os.ReadDir(root)
	if err != nil {
		t.Fatalf("ReadDir root: %v", err)
	}
	if len(entries) != 0 {
		t.Fatalf("memory root entries after reset = %#v", entries)
	}
	if _, err := os.Stat(outside); err != nil {
		t.Fatalf("outside symlink target should be preserved: %v", err)
	}
}

func TestMemoryServiceRejectsUnsafeRoots(t *testing.T) {
	if _, err := NewMemoryService(string(filepath.Separator)); !errors.Is(err, ErrMemoryRootUnsafe) {
		t.Fatalf("root path error = %v, want ErrMemoryRootUnsafe", err)
	}

	target := t.TempDir()
	link := filepath.Join(t.TempDir(), "memory-link")
	if err := os.Symlink(target, link); err != nil {
		t.Fatalf("Symlink: %v", err)
	}
	service, err := NewMemoryService(link)
	if err != nil {
		t.Fatalf("NewMemoryService symlink root: %v", err)
	}
	if _, err := service.Reset(context.Background()); !errors.Is(err, ErrMemoryRootUnsafe) {
		t.Fatalf("symlink root reset error = %v, want ErrMemoryRootUnsafe", err)
	}
}
