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
	models, err := discoverLMStudioModels(tempDir, "ollama", false)
	if err != nil {
		t.Fatalf("discoverLMStudioModels failed: %v", err)
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

func TestCalculateSHA256(t *testing.T) {
	tempDir := t.TempDir()
	filePath := filepath.Join(tempDir, "test.txt")
	content := "hello world"
	if err := os.WriteFile(filePath, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	// echo -n "hello world" | shasum -a 256
	expectedHash := "b94d27b9934d3e08a52e52d7da7dabfac484efe37a5380ee9088f7ace2efcde9"

	hash, err := calculateSHA256(filePath)
	if err != nil {
		t.Fatalf("calculateSHA256 failed: %v", err)
	}

	if hash != expectedHash {
		t.Errorf("Expected hash %s, got %s", expectedHash, hash)
	}
}

func TestProcessLMStudioModelDryRun(t *testing.T) {
	ollamaDir := t.TempDir()
	
	model := LMStudioModel{
		Name: "test-model",
		Path: "/abs/path/to/model.gguf",
	}

	// In dry run, it shouldn't actually call calculateSHA256 if I mock the file?
	// Wait, my implementation calls calculateSHA256 BEFORE checking for dryRun for the hash part.
	// I should create a mock file for it.
	tempDir := t.TempDir()
	mockFile := filepath.Join(tempDir, "mock.gguf")
	os.WriteFile(mockFile, []byte("data"), 0644)
	model.Path = mockFile

	result := processLMStudioModel(model, ollamaDir, "lms", true, false)
	if !result {
		t.Fatal("processLMStudioModel dry run failed")
	}

	// Verify no symlinks were created in blobs
	blobsDir := filepath.Join(ollamaDir, "blobs")
	if _, err := os.Stat(blobsDir); !os.IsNotExist(err) {
		// If it exists, check it's empty
		entries, _ := os.ReadDir(blobsDir)
		if len(entries) > 0 {
			t.Errorf("Blobs directory should be empty in dry run, got %d entries", len(entries))
		}
	}
}
