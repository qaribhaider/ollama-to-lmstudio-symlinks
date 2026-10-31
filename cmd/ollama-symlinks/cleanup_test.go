package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestRemoveEmptySubdirs_KeepsFoldersWithContent(t *testing.T) {
	dir := t.TempDir()
	empty := filepath.Join(dir, "empty-model")
	full := filepath.Join(dir, "full-model")
	os.MkdirAll(empty, 0755)
	os.MkdirAll(full, 0755)
	os.WriteFile(filepath.Join(full, "model.gguf"), []byte("x"), 0644)

	removeEmptySubdirs(dir, false)

	if _, err := os.Stat(empty); !os.IsNotExist(err) {
		t.Error("empty model folder should have been removed")
	}
	if _, err := os.Stat(full); err != nil {
		t.Errorf("folder with content must be kept: %v", err)
	}
}
