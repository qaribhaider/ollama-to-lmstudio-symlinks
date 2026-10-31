package main

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// Tests must never run the developer's real `ollama`: with --skip-checks the
// binary name falls back to a bare "ollama", which exec resolves from PATH, and
// a real `ollama create` would register models in their library.
func TestNoRealOllamaIsExecuted(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("uses a shell script as the fake binary")
	}

	binDir := t.TempDir()
	marker := filepath.Join(t.TempDir(), "ollama-was-called")
	script := "#!/bin/sh\ntouch '" + marker + "'\nexit 1\n"
	if err := os.WriteFile(filepath.Join(binDir, "ollama"), []byte(script), 0755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", binDir+string(os.PathListSeparator)+os.Getenv("PATH"))

	tempDir := t.TempDir()
	lmstudioDir := filepath.Join(tempDir, "lmstudio")
	ollamaDir := filepath.Join(tempDir, "ollama")
	os.MkdirAll(filepath.Join(lmstudioDir, "pub", "model"), 0755)
	os.MkdirAll(filepath.Join(ollamaDir, "blobs"), 0755)
	os.WriteFile(filepath.Join(lmstudioDir, "pub", "model", "model.gguf"), []byte("fake"), 0644)

	// The result is irrelevant; only whether a binary was executed matters.
	_ = runApp([]string{
		"--interactive=false", "--reverse", "--skip-checks",
		"--lmstudio-dir", lmstudioDir, "--ollama-dir", ollamaDir,
	}, strings.NewReader(""))

	if _, err := os.Stat(marker); err == nil {
		t.Fatal("a test executed the ollama found on PATH")
	}
}
