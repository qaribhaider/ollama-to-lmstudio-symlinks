//go:build !windows

package linking

import (
	"fmt"
	"os"
	"syscall"
)

func getHardlinkInfo(_ string, info os.FileInfo) (bool, string) {
	if sys, ok := info.Sys().(*syscall.Stat_t); ok && sys.Nlink > 1 {
		return true, fmt.Sprintf("inode:%d", sys.Ino)
	}
	return false, ""
}
