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
	result := ProcessModel(model, ollamaDir, providerDir, false, false, false)
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

	result := ProcessLMStudioModel(model, ollamaDir, "lms", true, false, false)
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

func TestListSymlinks(t *testing.T) {
	tempDir := t.TempDir()

	// Create a real file
	realFile := filepath.Join(tempDir, "real.txt")
	os.WriteFile(realFile, []byte("data"), 0644)

	// Create a symlink
	linkPath := filepath.Join(tempDir, "link.txt")
	os.Symlink(realFile, linkPath)

	// Create a subdirectory with another symlink
	subDir := filepath.Join(tempDir, "sub")
	os.Mkdir(subDir, 0755)
	subLinkPath := filepath.Join(subDir, "sublink.txt")
	os.Symlink(realFile, subLinkPath)

	links, err := ListSymlinks(tempDir)
	if err != nil {
		t.Fatalf("ListSymlinks failed: %v", err)
	}

	// Should find 3 items (1 real file/hard link, 2 symlinks)
	if len(links) != 3 {
		t.Fatalf("Expected 3 items, got %d", len(links))
	}

	// Verify names
	names := map[string]bool{}
	for _, l := range links {
		names[l.Name] = true
	}
	if !names["link.txt"] || !names["sublink.txt"] || !names["real.txt"] {
		t.Errorf("Did not find expected names: %v", names)
	}
}

func TestRemoveSymlinks(t *testing.T) {
	tempDir := t.TempDir()
	
	// Create real file, symlink, and a directory
	realFile := filepath.Join(tempDir, "real.txt")
	os.WriteFile(realFile, []byte("data"), 0644)
	linkPath := filepath.Join(tempDir, "link.txt")
	os.Symlink(realFile, linkPath)
	dirPath := filepath.Join(tempDir, "a_directory")
	os.Mkdir(dirPath, 0755)

	// Test trying to remove a directory (should fail as non-link/non-file)
	removed, failed := RemoveSymlinks([]string{dirPath}, false)
	if removed != 0 || failed != 1 {
		t.Errorf("Directory expected 0 removed, 1 failed, got %d/%d", removed, failed)
	}
	if _, err := os.Stat(dirPath); os.IsNotExist(err) {
		t.Error("Directory was accidentally deleted!")
	}

	// Test dry run
	removed, failed = RemoveSymlinks([]string{linkPath}, true)
	if removed != 1 || failed != 0 {
		t.Errorf("Dry run: expected 1 removed, 0 failed, got %d/%d", removed, failed)
	}
	if _, err := os.Lstat(linkPath); os.IsNotExist(err) {
		t.Error("Dry run should not have removed the file")
	}

	// Test actual removal
	removed, failed = RemoveSymlinks([]string{linkPath}, false)
	if removed != 1 || failed != 0 {
		t.Errorf("Actual: expected 1 removed, 0 failed, got %d/%d", removed, failed)
	}
	if _, err := os.Lstat(linkPath); !os.IsNotExist(err) {
		t.Error("Actual removal failed to delete the symlink")
	}
	
	// Verify real file still exists
	if _, err := os.Stat(realFile); os.IsNotExist(err) {
		t.Error("Real file was accidentally deleted!")
	}
}

func TestSecureJoin(t *testing.T) {
	base := "/base/path"
	
	tests := []struct {
		name    string
		input   string
		wantErr bool
	}{
		{"Valid simple", "model", false},
		{"Valid subpath", "publisher/model", false},
		{"Traversal attempt", "../../etc/passwd", true},
		{"Trailing traversal", "model/../..", true},
		{"Absolute path attempt", "/etc/passwd", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := SecureJoin(base, tt.input)
			if (err != nil) != tt.wantErr {
				t.Errorf("SecureJoin(%s, %s) error = %v, wantErr %v", base, tt.input, err, tt.wantErr)
			}
		})
	}
}
func TestFindBrokenSymlinks(t *testing.T) {
	tempDir := t.TempDir()

	// 1. Create a real file
	realFile := filepath.Join(tempDir, "real.txt")
	os.WriteFile(realFile, []byte("data"), 0644)

	// 2. Create a valid symlink
	validLink := filepath.Join(tempDir, "valid.link")
	os.Symlink(realFile, validLink)

	// 3. Create a broken symlink
	brokenLink := filepath.Join(tempDir, "broken.link")
	os.Symlink(filepath.Join(tempDir, "missing.txt"), brokenLink)

	// 4. Run discovery
	broken, err := FindBrokenSymlinks(tempDir)
	if err != nil {
		t.Fatalf("FindBrokenSymlinks failed: %v", err)
	}

	// Should find exactly 1 broken link
	if len(broken) != 1 {
		t.Fatalf("Expected 1 broken link, got %d", len(broken))
	}

	if broken[0].Path != brokenLink {
		t.Errorf("Expected broken link path %s, got %s", brokenLink, broken[0].Path)
	}
}
