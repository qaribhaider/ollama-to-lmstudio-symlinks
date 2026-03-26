package lmstudio

import (
	"os"
	"path/filepath"
	"testing"
)

func TestGetDefaultDirs(t *testing.T) {
	home, err := os.UserHomeDir()
	if err != nil {
		t.Fatalf("Failed to get user home dir: %v", err)
	}

	lmStudioDir := GetDefaultLMStudioDir()
	expectedLMStudio := filepath.Join(home, ".cache", "lm-studio", "models")
	if lmStudioDir != expectedLMStudio {
		t.Errorf("Expected LM Studio dir %s, got %s", expectedLMStudio, lmStudioDir)
	}
}

func TestDiscoverLMStudioModels(t *testing.T) {
	tempDir := t.TempDir()

	// 1. Create a regular GGUF file
	publisherDir := filepath.Join(tempDir, "publisher", "model1")
	if err := os.MkdirAll(publisherDir, 0755); err != nil {
		t.Fatal(err)
	}
	ggufPath := filepath.Join(publisherDir, "test.gguf")
	if err := os.WriteFile(ggufPath, []byte("mock gguf content"), 0644); err != nil {
		t.Fatal(err)
	}

	// 2. Create a symlink (should be skipped)
	symlinkPath := filepath.Join(publisherDir, "link.gguf")
	if err := os.Symlink(ggufPath, symlinkPath); err != nil {
		t.Fatal(err)
	}

	// 3. Create a provider directory to skip
	skipDir := filepath.Join(tempDir, "ollama")
	if err := os.MkdirAll(skipDir, 0755); err != nil {
		t.Fatal(err)
	}
	skipGGUF := filepath.Join(skipDir, "should-skip.gguf")
	if err := os.WriteFile(skipGGUF, []byte("should be skipped"), 0644); err != nil {
		t.Fatal(err)
	}

	// 4. Run discovery
	models, err := DiscoverLMStudioModels(tempDir, "ollama", false)
	if err != nil {
		t.Fatalf("DiscoverLMStudioModels failed: %v", err)
	}

	// Expect exactly 1 model (the real GGUF, skipping symlink and ollama dir)
	if len(models) != 1 {
		t.Fatalf("Expected 1 model, got %d", len(models))
	}

	if models[0].Path != ggufPath {
		t.Errorf("Expected path %s, got %s", ggufPath, models[0].Path)
	}

	expectedName := "publisher-model1-test"
	if models[0].Name != expectedName {
		t.Errorf("Expected name %s, got %s", expectedName, models[0].Name)
	}
}
