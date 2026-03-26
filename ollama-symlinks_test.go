package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestGetDefaultDirs(t *testing.T) {
	home, err := os.UserHomeDir()
	if err != nil {
		t.Fatalf("Failed to get user home dir: %v", err)
	}

	ollamaDir := getDefaultOllamaDir()
	expectedOllama := filepath.Join(home, ".ollama", "models")
	if ollamaDir != expectedOllama {
		t.Errorf("Expected ollama dir %s, got %s", expectedOllama, ollamaDir)
	}

	lmStudioDir := getDefaultLMStudioDir()
	expectedLMStudio := filepath.Join(home, ".cache", "lm-studio", "models")
	if lmStudioDir != expectedLMStudio {
		t.Errorf("Expected LM Studio dir %s, got %s", expectedLMStudio, lmStudioDir)
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

	manifest := OllamaManifest{}
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
	models, err := discoverModels(tempDir, false)
	if err != nil {
		t.Fatalf("discoverModels failed: %v", err)
	}

	if len(models) != 1 {
		t.Fatalf("Expected exactly 1 model to be discovered, got %d", len(models))
	}

	model := models[0]
	expectedName := "test-model-latest"
	if model.Name != expectedName {
		t.Errorf("Expected model name %s, got %s", expectedName, model.Name)
	}
	if model.MainModelBlob != "sha256:12345abcdef" {
		t.Errorf("Expected main model blob sha256:12345abcdef, got %s", model.MainModelBlob)
	}
	if model.AdditionalBlobs["sha256:fedcba54321"] != expectedName+"-projector.bin" {
		t.Errorf("Expected projector blob mapping, but was not found or incorrect")
	}
}

func TestProcessModel(t *testing.T) {
	ollamaDir := t.TempDir()
	lmstudioDir := t.TempDir()

	// 1. Create mock standard target blobs
	blobsDir := filepath.Join(ollamaDir, "blobs")
	if err := os.MkdirAll(blobsDir, 0755); err != nil {
		t.Fatalf("Failed to create mock blobs dir: %v", err)
	}

	mainBlobHash := "sha256-blob11111"
	projectorBlobHash := "sha256-blob22222"
	os.WriteFile(filepath.Join(blobsDir, mainBlobHash), []byte("mock main file"), 0644)
	os.WriteFile(filepath.Join(blobsDir, projectorBlobHash), []byte("mock projector file"), 0644)

	// Define our parsed ModelInfo
	model := ModelInfo{
		Name:          "test-model-latest",
		MainModelBlob: "sha256:blob11111", // Using colons to ensure 'processModel' converts them properly
		AdditionalBlobs: map[string]string{
			"sha256:blob22222": "test-model-latest-projector.bin",
		},
	}

	// 2. Set up LM studio destination dir
	providerDir := filepath.Join(lmstudioDir, "ollama")

	// 3. Process the model normally
	result := processModel(model, ollamaDir, providerDir, false, false)
	if !result {
		t.Fatal("processModel returned false, expected true to indicate successful creation")
	}

	// 4. Assert Symlinks Were Created Successfully
	modelDir := filepath.Join(providerDir, model.Name)
	mainSymlink := filepath.Join(modelDir, model.Name+".gguf")

	info, err := os.Lstat(mainSymlink)
	if err != nil {
		t.Fatalf("Main symlink was not created in target directory: %v", err)
	}
	if info.Mode()&os.ModeSymlink == 0 {
		t.Errorf("Target is not a symbolic link: %s", mainSymlink)
	}

	projectorSymlink := filepath.Join(modelDir, "test-model-latest-projector.bin")
	infoProj, err := os.Lstat(projectorSymlink)
	if err != nil {
		t.Fatalf("Projector symlink was not created: %v", err)
	}
	if infoProj.Mode()&os.ModeSymlink == 0 {
		t.Errorf("Target is not a symbolic link: %s", projectorSymlink)
	}

	// 5. Run again to test SKIP mechanism (should return false because symlinks already exist)
	result2 := processModel(model, ollamaDir, providerDir, false, false)
	if result2 {
		t.Fatal("processModel returned true on second run, expected false to skip existing symlinks")
	}
}

func TestProcessModelDryRun(t *testing.T) {
	ollamaDir := t.TempDir()
	lmstudioDir := t.TempDir()

	model := ModelInfo{
		Name:          "test-model-latest",
		MainModelBlob: "sha256:111111",
	}

	providerDir := filepath.Join(lmstudioDir, "ollama")

	result := processModel(model, ollamaDir, providerDir, true, false)
	if !result {
		t.Fatal("processModel returned false on dry run, expected true")
	}

	// Verify target provider directory was effectively NOT created
	modelDir := filepath.Join(providerDir, model.Name)
	if _, err := os.Stat(modelDir); !os.IsNotExist(err) {
		t.Errorf("Model directory should not exist in dry run - expected no changes")
	}
}
