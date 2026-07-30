//go:build windows

package fs

import (
	"os"

	"golang.org/x/sys/windows"
)

func metadataLinkCount(path string, _ os.FileInfo) uint64 {
	file, err := os.Open(path)
	if err != nil {
		return 0
	}
	defer file.Close()
	var info windows.ByHandleFileInformation
	if err := windows.GetFileInformationByHandle(windows.Handle(file.Fd()), &info); err != nil {
		return 0
	}
	return uint64(info.NumberOfLinks)
}
