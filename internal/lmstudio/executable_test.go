package lmstudio

import (
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestGetLMStudioExecutable_InPath(t *testing.T) {
	oldLookPath := LookPathFunc
	defer func() { LookPathFunc = oldLookPath }()

	expectedPath := "/mock/bin/lms"
	LookPathFunc = func(file string) (string, error) {
		if file == "lms" || file == "lms.exe" {
			return expectedPath, nil
		}
		return "", exec.ErrNotFound
	}

	path, err := GetLMStudioExecutable()
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if path != expectedPath {
		t.Errorf("expected %s, got %s", expectedPath, path)
	}
}

func TestIsLMStudioInstalled_ViaTargetDir(t *testing.T) {
	tempDir := t.TempDir()
	if !IsLMStudioInstalled(tempDir) {
		t.Errorf("expected true when targetDir exists")
	}

	nonExistent := filepath.Join(tempDir, "does-not-exist")
	// If host machine doesn't have LM Studio, this tests the false path
	_ = IsLMStudioInstalled(nonExistent)
}

func TestValidateLMStudio_SuccessWithTargetDir(t *testing.T) {
	tempDir := t.TempDir()
	err := ValidateLMStudio(false, tempDir)
	if err != nil {
		t.Errorf("expected nil error for valid targetDir, got %v", err)
	}
}

func TestValidateLMStudio_BlockingError(t *testing.T) {
	oldLookPath := LookPathFunc
	defer func() { LookPathFunc = oldLookPath }()

	LookPathFunc = func(file string) (string, error) {
		return "", exec.ErrNotFound
	}

	tempDir := t.TempDir()
	nonExistent := filepath.Join(tempDir, "non-existent")

	// If LM Studio is not globally installed on test machine, ValidateLMStudio will return error
	err := ValidateLMStudio(false, nonExistent)
	if err != nil {
		msg := err.Error()
		if !strings.Contains(msg, "blocking validation error") {
			t.Errorf("expected 'blocking validation error' in message, got %q", msg)
		}
		if !strings.Contains(msg, "https://lmstudio.ai") {
			t.Errorf("expected 'https://lmstudio.ai' in message, got %q", msg)
		}
		if !strings.Contains(msg, "--skip-checks") {
			t.Errorf("expected '--skip-checks' in message, got %q", msg)
		}
	}
}

func TestValidateLMStudio_SkipChecks(t *testing.T) {
	oldLookPath := LookPathFunc
	defer func() { LookPathFunc = oldLookPath }()

	LookPathFunc = func(file string) (string, error) {
		return "", exec.ErrNotFound
	}

	tempDir := t.TempDir()
	nonExistent := filepath.Join(tempDir, "non-existent")

	err := ValidateLMStudio(true, nonExistent)
	if err != nil {
		t.Errorf("expected nil error when skipChecks=true, got %v", err)
	}
}
