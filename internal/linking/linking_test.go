package linking

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/qaribhaider/ollama-to-lmstudio-symlinks/internal/models"
)

func TestCalculateSHA256(t *testing.T) {
	tempDir := t.TempDir()
	filePath := filepath.Join(tempDir, "test.txt")
	content := "hello world"
	if err := os.WriteFile(filePath, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	// echo -n "hello world" | shasum -a 256
	expectedHash := "b94d27b9934d3e08a52e52d7da7dabfac484efe37a5380ee9088f7ace2efcde9"

	hash, err := CalculateSHA256(filePath)
	if err != nil {
		t.Fatalf("CalculateSHA256 failed: %v", err)
	}

	if hash != expectedHash {
		t.Errorf("Expected hash %s, got %s", expectedHash, hash)
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
	model := models.ModelInfo{
		Name:          "test-model-latest",
		MainModelBlob: "sha256:blob11111",
		AdditionalBlobs: map[string]string{
			"sha256:blob22222": "test-model-latest-projector.bin",
		},
	}

	// 2. Set up LM studio destination dir
	providerDir := filepath.Join(lmstudioDir, "ollama")

	// 3. Process the model normally
	result := ProcessModel(model, ollamaDir, providerDir, false, false)
	if !result {
		t.Fatal("ProcessModel returned false, expected true to indicate successful creation")
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
}

func TestProcessLMStudioModelDryRun(t *testing.T) {
	ollamaDir := t.TempDir()
	
	model := models.LMStudioModel{
		Name: "test-model",
		Path: "/abs/path/to/model.gguf",
	}

	tempDir := t.TempDir()
	mockFile := filepath.Join(tempDir, "mock.gguf")
	os.WriteFile(mockFile, []byte("data"), 0644)
	model.Path = mockFile

	result := ProcessLMStudioModel(model, ollamaDir, "lms", true, false)
	if !result {
		t.Fatal("ProcessLMStudioModel dry run failed")
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
