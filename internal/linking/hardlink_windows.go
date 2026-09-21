//go:build windows

package linking

import (
	"fmt"
	"os"
	"syscall"
)

func getHardlinkInfo(path string, _ os.FileInfo) (bool, string) {
	ptr, err := syscall.UTF16PtrFromString(path)
	if err != nil {
		return false, ""
	}
	h, err := syscall.CreateFile(ptr, 0, syscall.FILE_SHARE_READ|syscall.FILE_SHARE_WRITE|syscall.FILE_SHARE_DELETE, nil, syscall.OPEN_EXISTING, syscall.FILE_FLAG_BACKUP_SEMANTICS, 0)
	if err != nil {
		return false, ""
	}
	defer syscall.CloseHandle(h)

	var fi syscall.ByHandleFileInformation
	if err := syscall.GetFileInformationByHandle(h, &fi); err != nil {
		return false, ""
	}
	if fi.NumberOfLinks > 1 {
		return true, fmt.Sprintf("win:%d:%d:%d", fi.VolumeSerialNumber, fi.FileIndexHigh, fi.FileIndexLow)
	}
	return false, ""
}
