//go:build aix || android || darwin || dragonfly || freebsd || illumos || ios || linux || netbsd || openbsd || solaris || zos

package main

import (
	"errors"
	"fmt"
	"os"

	"golang.org/x/sys/unix"
)

func tryLockAppServerStoreFile(file *os.File) error {
	fd, err := appServerStoreFileDescriptor(file)
	if err != nil {
		return err
	}
	err = unix.Flock(fd, unix.LOCK_EX|unix.LOCK_NB)
	if errors.Is(err, unix.EWOULDBLOCK) || errors.Is(err, unix.EAGAIN) {
		return errAppServerStoreOwned
	}
	if err != nil {
		return fmt.Errorf("lock app-server store: %w", err)
	}
	return nil
}

func unlockAppServerStoreFile(file *os.File) error {
	fd, err := appServerStoreFileDescriptor(file)
	if err != nil {
		return err
	}
	if err := unix.Flock(fd, unix.LOCK_UN); err != nil {
		return fmt.Errorf("unlock app-server store: %w", err)
	}
	return nil
}

func appServerStoreFileDescriptor(file *os.File) (int, error) {
	fd := file.Fd()
	maxInt := int(^uint(0) >> 1)
	if fd > uintptr(maxInt) {
		return 0, fmt.Errorf("app-server store file descriptor %d exceeds int range", fd)
	}
	return int(fd), nil // #nosec G115 -- fd is range-checked above.
}
