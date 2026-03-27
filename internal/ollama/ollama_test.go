package ollama

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/qaribhaider/ollama-to-lmstudio-symlinks/internal/models"
)

func TestGetDefaultDirs(t *testing.T) {
	home, err := os.UserHomeDir()
	if err != nil {
		t.Fatalf("Failed to get user home dir: %v", err)
	}

	ollamaDir := GetDefaultOllamaDir()
	expectedOllama := filepath.Join(home, ".ollama", "models")
	if ollamaDir != expectedOllama {
		t.Errorf("Expected ollama dir %s, got %s", expectedOllama, ollamaDir)
	}
}

func TestDiscoverModels(t *testing.T) {
	tempDir := t.TempDir()
	manifestsDir := filepath.Join(tempDir, "manifests")

	// Create a valid manifest directory tree
	modelName := "test-model"
	variant := "latest"
	modelDir := filepath.Join(manifestsDir, "registry.ollama.ai", "library", modelName)
	if err := os.MkdirAll(modelDir, 0755); err != nil {
		t.Fatalf("Failed to create mock manifest directory: %v", err)
	}

	manifest := models.OllamaManifest{}
	manifest.Layers = []struct {
		MediaType string `json:"mediaType"`
		Digest    string `json:"digest"`
		Size      int64  `json:"size"`
	}{
		{
			MediaType: "application/vnd.ollama.image.model",
			Digest:    "sha256:12345abcdef",
		},
		{
			MediaType: "application/vnd.ollama.image.projector",
			Digest:    "sha256:fedcba54321",
		},
	}
	manifestData, err := json.Marshal(manifest)
	if err != nil {
		t.Fatalf("Failed to marshal manifest: %v", err)
	}

	// Write the mock manifest variant file
	variantPath := filepath.Join(modelDir, variant)
	if err := os.WriteFile(variantPath, manifestData, 0644); err != nil {
		t.Fatalf("Failed to write mock manifest: %v", err)
	}

	// Optional: add a hidden file to ensure it's skipped
	os.WriteFile(filepath.Join(modelDir, ".DS_Store"), []byte("junk"), 0644)

	// Run directory discovery
	discoveredModels, err := DiscoverModels(tempDir, false)
	if err != nil {
		t.Fatalf("DiscoverModels failed: %v", err)
	}

	if len(discoveredModels) != 1 {
		t.Fatalf("Expected exactly 1 model to be discovered, got %d", len(discoveredModels))
	}

	model := discoveredModels[0]
	expectedName := "test-model:latest"
	if model.Name != expectedName {
		t.Errorf("Expected model name %s, got %s", expectedName, model.Name)
	}
	if model.MainModelBlob != "sha256:12345abcdef" {
		t.Errorf("Expected main model blob sha256:12345abcdef, got %s", model.MainModelBlob)
	}
	// Projector name uses - instead of :
	expectedProjectorName := "test-model-latest-projector.bin"
	if model.AdditionalBlobs["sha256:fedcba54321"] != expectedProjectorName {
		t.Errorf("Expected projector name %s, got %s", expectedProjectorName, model.AdditionalBlobs["sha256:fedcba54321"])
	}
}
