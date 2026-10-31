//go:build !smoke

package main

import (
	"os"
	"testing"

	"github.com/qaribhaider/ollama-to-lmstudio-symlinks/internal/ollama"
)

// TestMain keeps every test away from the developer's real `ollama`. The stub
// path does not exist, so any attempt to run it fails immediately and nothing
// is registered in a real library. Tests that need different behaviour override
// the hooks themselves and restore this stub afterwards.
func TestMain(m *testing.M) {
	ollama.LookPathFunc = func(string) (string, error) { return "/nonexistent/stub/ollama", nil }
	ollama.PlatformPathsFunc = func() []string { return nil }
	os.Exit(m.Run())
}
