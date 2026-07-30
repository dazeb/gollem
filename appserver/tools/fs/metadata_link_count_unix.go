//go:build aix || android || darwin || dragonfly || freebsd || illumos || ios || linux || netbsd || openbsd || solaris

package fs

import (
	"os"
	"syscall"
)

func metadataLinkCount(_ string, info os.FileInfo) uint64 {
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok {
		return 0
	}
	return metadataUnsignedLinkCount(stat.Nlink)
}

func metadataUnsignedLinkCount[T ~uint16 | ~uint32 | ~uint64](count T) uint64 {
	return uint64(count)
}
