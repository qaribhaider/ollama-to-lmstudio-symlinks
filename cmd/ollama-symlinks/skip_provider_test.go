package main

import (
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func captureStdout(t *testing.T, fn func()) string {
	t.Helper()
	old := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	os.Stdout = w
	defer func() { os.Stdout = old }()

	fn()
	w.Close()
	out, _ := io.ReadAll(r)
	return string(out)
}

func TestRunApp_SkipProvider_RejectsNonDirectoryNames(t *testing.T) {
	dir := t.TempDir()
	base := []string{"--ollama-dir", dir, "--lmstudio-dir", dir}

	for _, bad := range []string{"", ".", "..", "a/b", `a\b`} {
		cases := map[string][]string{
			"root":    append([]string{"--interactive=false", "--dry-run", "--skip-checks", "--skip-provider=" + bad}, base...),
			"status":  append(append([]string{"status"}, base...), "--skip-provider="+bad),
			"delete":  append(append([]string{"delete", "--from", "lmstudio", "--skip-checks"}, base...), "--skip-provider="+bad),
			"cleanup": append(append([]string{"cleanup", "--dry-run", "--skip-checks"}, base...), "--skip-provider="+bad),
		}
		for name, args := range cases {
			err := runApp(args, strings.NewReader(""))
			if err == nil || !strings.Contains(err.Error(), "plain directory name") {
				t.Errorf("%s with --skip-provider=%q: expected a 'plain directory name' error, got %v", name, bad, err)
			}
		}
	}
}

func TestRunApp_Forward_HonoursSkipProvider(t *testing.T) {
	tempDir := t.TempDir()
	ollamaDir := filepath.Join(tempDir, "ollama")
	lmsDir := filepath.Join(tempDir, "lmstudio")
	digest := strings.Repeat("a", 64)

	manifestDir := filepath.Join(ollamaDir, "manifests", "registry.ollama.ai", "library", "tiny")
	os.MkdirAll(manifestDir, 0755)
	os.MkdirAll(filepath.Join(ollamaDir, "blobs"), 0755)
	os.MkdirAll(lmsDir, 0755)
	os.WriteFile(filepath.Join(ollamaDir, "blobs", "sha256-"+digest), []byte("weights"), 0644)
	manifest := `{"layers":[{"mediaType":"application/vnd.ollama.image.model","digest":"sha256:` + digest + `"}]}`
	os.WriteFile(filepath.Join(manifestDir, "latest"), []byte(manifest), 0644)

	err := runApp([]string{
		"--interactive=false", "--skip-checks",
		"--ollama-dir", ollamaDir, "--lmstudio-dir", lmsDir,
		"--skip-provider", "custom",
	}, strings.NewReader(""))
	if err != nil {
		t.Fatalf("forward run failed: %v", err)
	}

	link := filepath.Join(lmsDir, "custom", "tiny-latest", "tiny-latest.gguf")
	if _, err := os.Lstat(link); err != nil {
		t.Errorf("expected link under the configured provider directory: %v", err)
	}
	if _, err := os.Stat(filepath.Join(lmsDir, "ollama")); err == nil {
		t.Error("forward mode must not use the default 'ollama' directory when --skip-provider is set")
	}
}

func TestRunApp_Cleanup_HonoursSkipProvider(t *testing.T) {
	tempDir := t.TempDir()
	ollamaDir := filepath.Join(tempDir, "ollama")
	lmsDir := filepath.Join(tempDir, "lmstudio")
	custom := filepath.Join(lmsDir, "custom")
	os.MkdirAll(filepath.Join(ollamaDir, "blobs"), 0755)
	os.MkdirAll(custom, 0755)
	os.Symlink(filepath.Join(tempDir, "missing.gguf"), filepath.Join(custom, "broken.gguf"))

	var runErr error
	out := captureStdout(t, func() {
		runErr = runApp([]string{
			"cleanup", "--dry-run", "--skip-checks", "--skip-provider", "custom",
			"--ollama-dir", ollamaDir, "--lmstudio-dir", lmsDir,
		}, strings.NewReader(""))
	})
	if runErr != nil {
		t.Fatalf("cleanup failed: %v", runErr)
	}
	if !strings.Contains(out, "broken.gguf") {
		t.Errorf("cleanup should scan the configured provider directory, output:\n%s", out)
	}
}
