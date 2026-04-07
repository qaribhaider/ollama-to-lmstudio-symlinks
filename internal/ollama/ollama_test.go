package ollama

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/qaribhaider/ollama-to-lmstudio-symlinks/internal/models"
)

func TestGetDefaultDirs(t *testing.T) {
	// Test default behavior
	home, err := os.UserHomeDir()
	if err != nil {
		t.Fatalf("Failed to get user home dir: %v", err)
	}

	ollamaDir := GetDefaultOllamaDir()
	expectedOllama := filepath.Join(home, ".ollama", "models")
	if ollamaDir != expectedOllama {
		t.Errorf("Expected ollama dir %s, got %s", expectedOllama, ollamaDir)
	}

	// Test OLLAMA_MODELS env var
	customPath := filepath.Join(t.TempDir(), "custom", "ollama", "models")
	os.MkdirAll(customPath, 0755)
	os.Setenv("OLLAMA_MODELS", customPath)
	defer os.Unsetenv("OLLAMA_MODELS")

	ollamaDir = GetDefaultOllamaDir()
	if ollamaDir != customPath {
		t.Errorf("Expected custom ollama dir %s, got %s", customPath, ollamaDir)
	}
}

func TestGetOllamaCandidates(t *testing.T) {
	tempDir := t.TempDir()
	
	// Create mock directories
	path1 := filepath.Join(tempDir, "opt1")
	path2 := filepath.Join(tempDir, "opt2")
	os.MkdirAll(path1, 0755)
	os.MkdirAll(path2, 0755)

	// Set env var to point to one of them
	os.Setenv("OLLAMA_MODELS", path1)
	defer os.Unsetenv("OLLAMA_MODELS")

	candidates := GetOllamaCandidates()
	
	foundPath1 := false
	for _, c := range candidates {
		if c == path1 {
			foundPath1 = true
			break
		}
	}
	if !foundPath1 {
		t.Errorf("Expected to find %s in candidates, but it was missing", path1)
	}
}

func TestValidateDigest(t *testing.T) {
	valid64 := strings.Repeat("a", 64)
	tests := []struct {
		name    string
		digest  string
		wantErr bool
	}{
		{"Valid", "sha256:" + valid64, false},
		{"Missing prefix", valid64, true},
		{"Wrong prefix", "md5:" + valid64, true},
		{"Too short", "sha256:abc", true},
		{"Invalid chars", "sha256:" + strings.Repeat("Z", 64), true},
		{"Path Traversal", "sha256:../../etc/passwd", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := ValidateDigest(tt.digest); (err != nil) != tt.wantErr {
				t.Errorf("ValidateDigest(%q) error = %v, wantErr %v", tt.digest, err, tt.wantErr)
			}
		})
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
			Digest:    "sha256:" + strings.Repeat("1", 64),
		},
		{
			MediaType: "application/vnd.ollama.image.projector",
			Digest:    "sha256:" + strings.Repeat("2", 64),
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
	if model.MainModelBlob != "sha256:"+strings.Repeat("1", 64) {
		t.Errorf("Expected main model blob sha256:111..., got %s", model.MainModelBlob)
	}
	// Projector name uses - instead of :
	expectedProjectorName := "test-model-latest-projector.bin"
	if model.AdditionalBlobs["sha256:"+strings.Repeat("2", 64)] != expectedProjectorName {
		t.Errorf("Expected projector name %s, got %s", expectedProjectorName, model.AdditionalBlobs["sha256:"+strings.Repeat("2", 64)])
	}
}

func TestDiscoverModelsResilience(t *testing.T) {
	tempDir := t.TempDir()
	manifestsDir := filepath.Join(tempDir, "manifests")

	// 1. Create a valid MLX manifest
	mlxModelName := "mlx-model"
	mlxModelDir := filepath.Join(manifestsDir, "registry.ollama.ai", "library", mlxModelName)
	os.MkdirAll(mlxModelDir, 0755)
	
	mlxManifest := models.OllamaManifest{}
	mlxManifest.Layers = []struct {
		MediaType string `json:"mediaType"`
		Digest    string `json:"digest"`
		Size      int64  `json:"size"`
	}{
		{
			MediaType: "application/vnd.ollama.image.model.mlx",
			Digest:    "sha256:" + strings.Repeat("3", 64),
		},
	}
	mlxData, _ := json.Marshal(mlxManifest)
	os.WriteFile(filepath.Join(mlxModelDir, "latest"), mlxData, 0644)

	// 2. Create an invalid model manifest (no model weights)
	invalidModelName := "invalid-model"
	invalidModelDir := filepath.Join(manifestsDir, "registry.ollama.ai", "library", invalidModelName)
	os.MkdirAll(invalidModelDir, 0755)
	
	invalidManifest := models.OllamaManifest{}
	invalidManifest.Layers = []struct {
		MediaType string `json:"mediaType"`
		Digest    string `json:"digest"`
		Size      int64  `json:"size"`
	}{
		{
			MediaType: "application/vnd.ollama.image.template",
			Digest:    "sha256:" + strings.Repeat("4", 64),
		},
	}
	invalidData, _ := json.Marshal(invalidManifest)
	os.WriteFile(filepath.Join(invalidModelDir, "latest"), invalidData, 0644)

	// Run directory discovery
	discoveredModels, err := DiscoverModels(tempDir, false)
	if err != nil {
		t.Fatalf("DiscoverModels failed: %v", err)
	}

	// Should have only 1 model (mlx-model). invalid-model should be skipped.
	if len(discoveredModels) != 1 {
		t.Fatalf("Expected exactly 1 model to be discovered, got %d", len(discoveredModels))
	}

	if discoveredModels[0].Name != "mlx-model:latest" {
		t.Errorf("Expected mlx-model:latest, got %s", discoveredModels[0].Name)
	}
}
