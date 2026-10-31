//go:build !windows

package linking

import (
	"fmt"
	"os"
	"syscall"
)

func getHardlinkInfo(_ string, info os.FileInfo) (bool, string) {
	if sys, ok := info.Sys().(*syscall.Stat_t); ok && sys.Nlink > 1 {
		// Inode numbers repeat across devices, so the device is part of the identity.
		return true, fmt.Sprintf("dev:%d:inode:%d", sys.Dev, sys.Ino)
	}
	return false, ""
}
