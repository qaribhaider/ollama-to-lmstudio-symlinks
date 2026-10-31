//go:build !windows

package linking

import (
	"fmt"
	"os"
	"path/filepath"
	"syscall"
	"testing"
)

// Inode numbers are only unique within one device, so two hard-linked files on
// different mounts could share a number and be counted as one.
func TestGetHardlinkInfo_IdentityIncludesDevice(t *testing.T) {
	dir := t.TempDir()
	source := filepath.Join(dir, "source.gguf")
	linked := filepath.Join(dir, "linked.gguf")
	os.WriteFile(source, []byte("weights"), 0644)
	if err := os.Link(source, linked); err != nil {
		t.Skipf("hard links not supported here: %v", err)
	}

	info, err := os.Stat(linked)
	if err != nil {
		t.Fatal(err)
	}
	stat := info.Sys().(*syscall.Stat_t)

	isHardlink, id := getHardlinkInfo(linked, info)
	if !isHardlink {
		t.Fatal("expected a hard link to be detected")
	}
	if want := fmt.Sprintf("dev:%d:inode:%d", stat.Dev, stat.Ino); id != want {
		t.Errorf("hard link identity = %q, want %q", id, want)
	}
}
