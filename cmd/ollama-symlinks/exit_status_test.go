package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/qaribhaider/ollama-to-lmstudio-symlinks/internal/lmstudio"
	"github.com/qaribhaider/ollama-to-lmstudio-symlinks/internal/ollama"
)

func TestRunApp_ReturnedPreFlightErrorIsNotAlsoPrinted(t *testing.T) {
	oldOllamaLookPath := ollama.LookPathFunc
	oldLMSLookPath := lmstudio.LookPathFunc
	oldLMSPlatform := lmstudio.PlatformPathsFunc
	oldLMSCandidates := lmstudio.CandidatesFunc
	defer func() {
		ollama.LookPathFunc = oldOllamaLookPath
		lmstudio.LookPathFunc = oldLMSLookPath
		lmstudio.PlatformPathsFunc = oldLMSPlatform
		lmstudio.CandidatesFunc = oldLMSCandidates
	}()
	ollama.LookPathFunc = func(string) (string, error) { return "/mock/ollama", nil }
	lmstudio.LookPathFunc = func(string) (string, error) { return "", exec.ErrNotFound }
	lmstudio.PlatformPathsFunc = func() []string { return nil }
	lmstudio.CandidatesFunc = func() []string { return nil }

	tempDir := t.TempDir()
	ollamaDir := filepath.Join(tempDir, "ollama")
	os.MkdirAll(ollamaDir, 0755)

	var runErr error
	out := captureStdout(t, func() {
		runErr = runApp([]string{
			"--interactive=false",
			"--ollama-dir", ollamaDir,
			"--lmstudio-dir", filepath.Join(tempDir, "missing-lms"),
		}, strings.NewReader(""))
	})

	if runErr == nil {
		t.Fatal("expected a pre-flight error")
	}
	// main() prints the returned error, so runApp printing it too shows it twice.
	if strings.Contains(out, "blocking validation error") {
		t.Errorf("error was printed by runApp as well as returned:\n%s", out)
	}
}

func TestRunApp_EmptyLibrariesAreReportedAsErrors(t *testing.T) {
	tempDir := t.TempDir()
	ollamaDir := filepath.Join(tempDir, "ollama")
	lmsDir := filepath.Join(tempDir, "lmstudio")
	os.MkdirAll(filepath.Join(ollamaDir, "manifests"), 0755)
	os.MkdirAll(filepath.Join(ollamaDir, "blobs"), 0755)
	os.MkdirAll(lmsDir, 0755)

	cases := map[string]struct {
		args []string
		want string
	}{
		"forward": {[]string{"--interactive=false", "--skip-checks"}, "no models found"},
		"reverse": {[]string{"--interactive=false", "--skip-checks", "--reverse"}, "no eligible models"},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			args := append(tc.args, "--ollama-dir", ollamaDir, "--lmstudio-dir", lmsDir)
			err := runApp(args, strings.NewReader(""))
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Errorf("expected an error containing %q so scripts can detect it, got %v", tc.want, err)
			}
		})
	}
}
