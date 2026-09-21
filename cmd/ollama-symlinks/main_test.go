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

func TestRunApp_Validation(t *testing.T) {
	tempDir := t.TempDir()
	
	// Create a dummy dir that exists
	existsDir := filepath.Join(tempDir, "exists")
	os.Mkdir(existsDir, 0755)

	tests := []struct {
		name        string
		args        []string
		wantErr     bool
		errContains string
	}{
		{
			name:        "Empty name prefix",
			args:        []string{"--interactive=false", "--name-prefix", ""},
			wantErr:     true,
			errContains: "--name-prefix cannot be empty",
		},
		{
			name:        "Non-existent ollama-dir (Forward)",
			args:        []string{"--interactive=false", "--ollama-dir", filepath.Join(tempDir, "non-existent")},
			wantErr:     true,
			errContains: "Ollama directory does not exist",
		},
		{
			name:        "Non-existent lmstudio-dir (Reverse)",
			args:        []string{"--interactive=false", "--reverse", "--lmstudio-dir", filepath.Join(tempDir, "non-existent")},
			wantErr:     true,
			errContains: "LM Studio directory does not exist",
		},
		{
			name:        "Delete without from",
			args:        []string{"--interactive=false", "--ollama-dir", existsDir, "delete"},
			wantErr:     true,
			errContains: "--from flag is required",
		},
		{
			name:        "Delete with invalid from",
			args:        []string{"--interactive=false", "--ollama-dir", existsDir, "delete", "--from", "invalid"},
			wantErr:     true,
			errContains: "invalid --from value",
		},
		{
			name:        "Delete with non-existent target dir",
			args:        []string{"--interactive=false", "--ollama-dir", filepath.Join(tempDir, "non-existent"), "delete", "--from", "ollama"},
			wantErr:     true,
			errContains: "directory for deletion does not exist",
		},
		{
			name:        "Unknown subcommand",
			args:        []string{"--interactive=false", "help"},
			wantErr:     true,
			errContains: "unknown command \"help\"",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Using strings.NewReader("") as dummy stdin
			err := runApp(tt.args, strings.NewReader(""))
			if (err != nil) != tt.wantErr {
				t.Errorf("runApp() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if tt.wantErr && !strings.Contains(err.Error(), tt.errContains) {
				t.Errorf("runApp() error = %v, wantContains %v", err, tt.errContains)
			}
		})
	}
}

func TestRunApp_Version(t *testing.T) {
	err := runApp([]string{"--version"}, strings.NewReader(""))
	if err != nil {
		t.Errorf("runApp(--version) error = %v", err)
	}
}

func TestRunApp_PreFlight_Reverse_Blocking(t *testing.T) {
	oldLookPath := ollama.LookPathFunc
	oldPlatformPaths := ollama.PlatformPathsFunc
	defer func() {
		ollama.LookPathFunc = oldLookPath
		ollama.PlatformPathsFunc = oldPlatformPaths
	}()

	ollama.LookPathFunc = func(file string) (string, error) {
		return "", exec.ErrNotFound
	}
	ollama.PlatformPathsFunc = func() []string { return nil }

	tempDir := t.TempDir()
	lmsDir := filepath.Join(tempDir, "lmstudio", "models")
	ollamaDir := filepath.Join(tempDir, "ollama", "models")
	os.MkdirAll(lmsDir, 0755)
	os.MkdirAll(ollamaDir, 0755)

	err := runApp([]string{
		"--interactive=false",
		"--reverse",
		"--lmstudio-dir", lmsDir,
		"--ollama-dir", ollamaDir,
	}, strings.NewReader(""))

	if err == nil {
		t.Fatal("expected blocking validation error, got nil")
	}
	if !strings.Contains(err.Error(), "blocking validation error: 'ollama' executable not found") {
		t.Errorf("unexpected error message: %v", err)
	}
}

func TestRunApp_PreFlight_Reverse_SkipChecks(t *testing.T) {
	oldLookPath := ollama.LookPathFunc
	oldPlatformPaths := ollama.PlatformPathsFunc
	defer func() {
		ollama.LookPathFunc = oldLookPath
		ollama.PlatformPathsFunc = oldPlatformPaths
	}()

	ollama.LookPathFunc = func(file string) (string, error) {
		return "", exec.ErrNotFound
	}
	ollama.PlatformPathsFunc = func() []string { return nil }

	tempDir := t.TempDir()
	lmsDir := filepath.Join(tempDir, "lmstudio", "models")
	ollamaDir := filepath.Join(tempDir, "ollama", "models")
	os.MkdirAll(lmsDir, 0755)
	os.MkdirAll(ollamaDir, 0755)

	// With --skip-checks and --dry-run, it should bypass the pre-flight block
	err := runApp([]string{
		"--interactive=false",
		"--reverse",
		"--skip-checks",
		"--dry-run",
		"--lmstudio-dir", lmsDir,
		"--ollama-dir", ollamaDir,
	}, strings.NewReader(""))

	if err != nil {
		t.Fatalf("expected nil error with --skip-checks, got %v", err)
	}
}

func TestRunApp_PreFlight_DeleteOllama_Blocking(t *testing.T) {
	oldLookPath := ollama.LookPathFunc
	oldPlatformPaths := ollama.PlatformPathsFunc
	defer func() {
		ollama.LookPathFunc = oldLookPath
		ollama.PlatformPathsFunc = oldPlatformPaths
	}()

	ollama.LookPathFunc = func(file string) (string, error) {
		return "", exec.ErrNotFound
	}
	ollama.PlatformPathsFunc = func() []string { return nil }

	tempDir := t.TempDir()
	ollamaDir := filepath.Join(tempDir, "ollama", "models")
	os.MkdirAll(filepath.Join(ollamaDir, "manifests"), 0755)
	os.MkdirAll(filepath.Join(ollamaDir, "blobs"), 0755)

	err := runApp([]string{
		"--interactive=false",
		"--ollama-dir", ollamaDir,
		"delete",
		"--from", "ollama",
	}, strings.NewReader(""))

	if err == nil {
		t.Fatal("expected blocking validation error, got nil")
	}
	if !strings.Contains(err.Error(), "blocking validation error: 'ollama' executable not found") {
		t.Errorf("unexpected error message: %v", err)
	}
}

func TestRunApp_PreFlight_DeleteOllama_SkipChecks(t *testing.T) {
	oldLookPath := ollama.LookPathFunc
	oldPlatformPaths := ollama.PlatformPathsFunc
	defer func() {
		ollama.LookPathFunc = oldLookPath
		ollama.PlatformPathsFunc = oldPlatformPaths
	}()

	ollama.LookPathFunc = func(file string) (string, error) {
		return "", exec.ErrNotFound
	}
	ollama.PlatformPathsFunc = func() []string { return nil }

	tempDir := t.TempDir()
	ollamaDir := filepath.Join(tempDir, "ollama", "models")
	os.MkdirAll(filepath.Join(ollamaDir, "manifests"), 0755)
	os.MkdirAll(filepath.Join(ollamaDir, "blobs"), 0755)

	err := runApp([]string{
		"--interactive=false",
		"--skip-checks",
		"--dry-run",
		"--ollama-dir", ollamaDir,
		"delete",
		"--from", "ollama",
	}, strings.NewReader(""))

	if err != nil {
		t.Fatalf("expected nil error when empty and skip-checks, got %v", err)
	}
}

func TestRunApp_PreFlight_Forward_LMStudio_Blocking(t *testing.T) {
	oldOllamaLookPath := ollama.LookPathFunc
	defer func() { ollama.LookPathFunc = oldOllamaLookPath }()
	ollama.LookPathFunc = func(file string) (string, error) {
		return "/mock/ollama", nil
	}

	oldLMSLookPath := lmstudio.LookPathFunc
	oldLMSPlatform := lmstudio.PlatformPathsFunc
	oldLMSCandidates := lmstudio.CandidatesFunc
	defer func() {
		lmstudio.LookPathFunc = oldLMSLookPath
		lmstudio.PlatformPathsFunc = oldLMSPlatform
		lmstudio.CandidatesFunc = oldLMSCandidates
	}()
	lmstudio.LookPathFunc = func(file string) (string, error) {
		return "", exec.ErrNotFound
	}
	lmstudio.PlatformPathsFunc = func() []string { return nil }
	lmstudio.CandidatesFunc = func() []string { return nil }

	tempDir := t.TempDir()
	ollamaDir := filepath.Join(tempDir, "ollama", "models")
	os.MkdirAll(ollamaDir, 0755)
	nonExistentLMSDir := filepath.Join(tempDir, "non-existent-lms", "models")

	err := runApp([]string{
		"--interactive=false",
		"--ollama-dir", ollamaDir,
		"--lmstudio-dir", nonExistentLMSDir,
	}, strings.NewReader(""))

	if err == nil {
		t.Fatal("expected blocking validation error for LM Studio, got nil")
	}
	if !strings.Contains(err.Error(), "blocking validation error: LM Studio installation or models directory not found") {
		t.Errorf("unexpected error message: %v", err)
	}
}

func TestRunApp_PreFlight_Forward_LMStudio_SkipChecks(t *testing.T) {
	oldOllamaLookPath := ollama.LookPathFunc
	defer func() { ollama.LookPathFunc = oldOllamaLookPath }()
	ollama.LookPathFunc = func(file string) (string, error) {
		return "/mock/ollama", nil
	}

	oldLMSLookPath := lmstudio.LookPathFunc
	oldLMSPlatform := lmstudio.PlatformPathsFunc
	oldLMSCandidates := lmstudio.CandidatesFunc
	defer func() {
		lmstudio.LookPathFunc = oldLMSLookPath
		lmstudio.PlatformPathsFunc = oldLMSPlatform
		lmstudio.CandidatesFunc = oldLMSCandidates
	}()
	lmstudio.LookPathFunc = func(file string) (string, error) {
		return "", exec.ErrNotFound
	}
	lmstudio.PlatformPathsFunc = func() []string { return nil }
	lmstudio.CandidatesFunc = func() []string { return nil }

	tempDir := t.TempDir()
	ollamaDir := filepath.Join(tempDir, "ollama", "models")
	os.MkdirAll(filepath.Join(ollamaDir, "manifests"), 0755)
	nonExistentLMSDir := filepath.Join(tempDir, "non-existent-lms", "models")

	err := runApp([]string{
		"--interactive=false",
		"--skip-checks",
		"--dry-run",
		"--ollama-dir", ollamaDir,
		"--lmstudio-dir", nonExistentLMSDir,
	}, strings.NewReader(""))

	if err != nil {
		t.Fatalf("expected nil error with --skip-checks, got %v", err)
	}
}

func TestRunApp_Status(t *testing.T) {
	tempDir := t.TempDir()
	ollamaDir := filepath.Join(tempDir, "ollama", "models")
	lmsDir := filepath.Join(tempDir, "lmstudio", "models")
	os.MkdirAll(filepath.Join(ollamaDir, "blobs"), 0755)
	os.MkdirAll(filepath.Join(lmsDir, "ollama"), 0755)

	// Create a dummy model file and symlink
	realModel := filepath.Join(tempDir, "real.gguf")
	os.WriteFile(realModel, make([]byte, 1024), 0644)

	link := filepath.Join(lmsDir, "ollama", "test-model.gguf")
	if err := os.Symlink(realModel, link); err != nil {
		t.Fatal(err)
	}

	err := runApp([]string{
		"status",
		"--ollama-dir", ollamaDir,
		"--lmstudio-dir", lmsDir,
		"--verbose",
	}, strings.NewReader(""))

	if err != nil {
		t.Fatalf("runApp(status) returned error: %v", err)
	}
}

func TestRunApp_Cleanup_DualDirectory(t *testing.T) {
	tempDir := t.TempDir()
	ollamaDir := filepath.Join(tempDir, "ollama", "models")
	lmsDir := filepath.Join(tempDir, "lmstudio", "models")
	blobsDir := filepath.Join(ollamaDir, "blobs")
	lmsManagedDir := filepath.Join(lmsDir, "ollama")
	os.MkdirAll(blobsDir, 0755)
	os.MkdirAll(lmsManagedDir, 0755)

	// Create broken symlink in LM Studio
	brokenLMS := filepath.Join(lmsManagedDir, "broken-lms.gguf")
	os.Symlink(filepath.Join(tempDir, "missing-blob.bin"), brokenLMS)

	// Create broken symlink in Ollama blobs
	brokenBlob := filepath.Join(blobsDir, "sha256-missing")
	os.Symlink(filepath.Join(tempDir, "missing-model.gguf"), brokenBlob)

	// Test cleanup in dry-run mode
	err := runApp([]string{
		"cleanup",
		"--dry-run",
		"--ollama-dir", ollamaDir,
		"--lmstudio-dir", lmsDir,
	}, strings.NewReader(""))

	if err != nil {
		t.Fatalf("runApp(cleanup --dry-run) error = %v", err)
	}

	// Symlinks should still exist after dry-run
	if _, err := os.Lstat(brokenLMS); err != nil {
		t.Errorf("expected broken LMS link to exist after dry-run")
	}
	if _, err := os.Lstat(brokenBlob); err != nil {
		t.Errorf("expected broken blob link to exist after dry-run")
	}
}

func TestRunApp_Delete_NamePrefix(t *testing.T) {
	oldLookPath := ollama.LookPathFunc
	oldPlatformPaths := ollama.PlatformPathsFunc
	defer func() {
		ollama.LookPathFunc = oldLookPath
		ollama.PlatformPathsFunc = oldPlatformPaths
	}()

	ollama.LookPathFunc = func(file string) (string, error) {
		return "/mock/ollama", nil
	}
	ollama.PlatformPathsFunc = func() []string { return nil }

	tempDir := t.TempDir()
	ollamaDir := filepath.Join(tempDir, "ollama", "models")
	os.MkdirAll(filepath.Join(ollamaDir, "manifests"), 0755)
	os.MkdirAll(filepath.Join(ollamaDir, "blobs"), 0755)

	// Test delete with custom name-prefix in dry-run
	err := runApp([]string{
		"--interactive=false",
		"--ollama-dir", ollamaDir,
		"delete",
		"--from", "ollama",
		"--name-prefix", "custom-prefix",
		"--dry-run",
	}, strings.NewReader(""))

	if err != nil {
		t.Fatalf("runApp(delete --name-prefix custom-prefix) error = %v", err)
	}
}


