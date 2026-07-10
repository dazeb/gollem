package appserver

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

var (
	ErrMemoryRootRequired = errors.New("appserver/memory: root is required")
	ErrMemoryRootUnsafe   = errors.New("appserver/memory: refusing unsafe root")
)

type MemoryService struct {
	root string
}

type MemoryResetResponse struct{}

func NewMemoryService(root string) (*MemoryService, error) {
	root = strings.TrimSpace(root)
	if root == "" {
		return nil, ErrMemoryRootRequired
	}
	absRoot, err := filepath.Abs(root)
	if err != nil {
		return nil, fmt.Errorf("resolve memory root: %w", err)
	}
	absRoot = filepath.Clean(absRoot)
	if filepath.Dir(absRoot) == absRoot {
		return nil, fmt.Errorf("%w: %s", ErrMemoryRootUnsafe, absRoot)
	}
	return &MemoryService{root: absRoot}, nil
}

func (s *MemoryService) Root() string {
	if s == nil {
		return ""
	}
	return s.root
}

func (s *MemoryService) Reset(ctx context.Context) (MemoryResetResponse, error) {
	if s == nil || s.root == "" {
		return MemoryResetResponse{}, ErrMemoryRootRequired
	}
	if err := ctx.Err(); err != nil {
		return MemoryResetResponse{}, err
	}
	info, err := os.Lstat(s.root)
	if err != nil {
		if os.IsNotExist(err) {
			return MemoryResetResponse{}, os.MkdirAll(s.root, 0o700)
		}
		return MemoryResetResponse{}, err
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return MemoryResetResponse{}, fmt.Errorf("%w: %s is a symlink", ErrMemoryRootUnsafe, s.root)
	}
	if !info.IsDir() {
		return MemoryResetResponse{}, fmt.Errorf("%w: %s is not a directory", ErrMemoryRootUnsafe, s.root)
	}

	entries, err := os.ReadDir(s.root)
	if err != nil {
		return MemoryResetResponse{}, err
	}
	for _, entry := range entries {
		if err := ctx.Err(); err != nil {
			return MemoryResetResponse{}, err
		}
		if err := os.RemoveAll(filepath.Join(s.root, entry.Name())); err != nil {
			return MemoryResetResponse{}, err
		}
	}
	return MemoryResetResponse{}, nil
}
