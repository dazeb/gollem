//go:build !aix && !android && !darwin && !dragonfly && !freebsd && !illumos && !ios && !linux && !netbsd && !openbsd && !solaris && !windows && !zos

package main

import (
	"fmt"
	"os"
	"runtime"
)

func tryLockAppServerStoreFile(*os.File) error {
	return fmt.Errorf("app-server store ownership is unavailable on %s", runtime.GOOS)
}

func unlockAppServerStoreFile(*os.File) error {
	return nil
}
