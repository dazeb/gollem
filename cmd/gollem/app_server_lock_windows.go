//go:build windows

package main

import (
	"errors"
	"fmt"
	"os"

	"golang.org/x/sys/windows"
)

func tryLockAppServerStoreFile(file *os.File) error {
	var overlapped windows.Overlapped
	err := windows.LockFileEx(
		windows.Handle(file.Fd()),
		windows.LOCKFILE_EXCLUSIVE_LOCK|windows.LOCKFILE_FAIL_IMMEDIATELY,
		0,
		1,
		0,
		&overlapped,
	)
	if errors.Is(err, windows.ERROR_LOCK_VIOLATION) {
		return errAppServerStoreOwned
	}
	if err != nil {
		return fmt.Errorf("lock app-server store: %w", err)
	}
	return nil
}

func unlockAppServerStoreFile(file *os.File) error {
	var overlapped windows.Overlapped
	if err := windows.UnlockFileEx(windows.Handle(file.Fd()), 0, 1, 0, &overlapped); err != nil {
		return fmt.Errorf("unlock app-server store: %w", err)
	}
	return nil
}
