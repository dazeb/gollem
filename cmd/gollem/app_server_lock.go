package main

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

var errAppServerStoreOwned = errors.New("app-server store is already owned by another daemon")

type appServerStoreLock struct {
	file *os.File
}

func acquireAppServerStoreLock(flags appServerFlags) (*appServerStoreLock, error) {
	workDir, err := filepath.Abs(flags.workDir)
	if err != nil {
		return nil, fmt.Errorf("resolve workdir: %w", err)
	}
	storePath, err := resolveAppServerStorePath(workDir, flags.storePath)
	if err != nil {
		return nil, err
	}
	if storePath == ":memory:" {
		return &appServerStoreLock{}, nil
	}

	lockPath := storePath + ".owner.lock"
	// lockPath is derived from the canonical store path resolved above.
	file, err := os.OpenFile(lockPath, os.O_CREATE|os.O_RDWR, 0o600) // #nosec G703
	if err != nil {
		return nil, fmt.Errorf("open app-server store lock: %w", err)
	}
	if err := tryLockAppServerStoreFile(file); err != nil {
		_ = file.Close()
		return nil, err
	}
	if err := file.Truncate(0); err != nil {
		_ = unlockAppServerStoreFile(file)
		_ = file.Close()
		return nil, fmt.Errorf("truncate app-server store lock: %w", err)
	}
	if _, err := file.Seek(0, 0); err != nil {
		_ = unlockAppServerStoreFile(file)
		_ = file.Close()
		return nil, fmt.Errorf("seek app-server store lock: %w", err)
	}
	if _, err := fmt.Fprintf(file, "%d\n", os.Getpid()); err != nil {
		_ = unlockAppServerStoreFile(file)
		_ = file.Close()
		return nil, fmt.Errorf("write app-server store owner: %w", err)
	}
	return &appServerStoreLock{file: file}, nil
}

func (l *appServerStoreLock) Close() error {
	if l == nil || l.file == nil {
		return nil
	}
	file := l.file
	l.file = nil
	return errors.Join(unlockAppServerStoreFile(file), file.Close())
}
