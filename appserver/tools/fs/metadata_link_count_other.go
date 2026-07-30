//go:build !aix && !android && !darwin && !dragonfly && !freebsd && !illumos && !ios && !linux && !netbsd && !openbsd && !solaris && !windows

package fs

import "os"

func metadataLinkCount(_ string, _ os.FileInfo) uint64 {
	return 0
}
