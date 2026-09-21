package linking

import (
	"os"
	"path/filepath"
	"runtime"
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

func TestSanitizeModelName(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{
			name: "keeps simple ollama name",
			in:   "lms-publisher-model-q4-k-m",
			want: "lms-publisher-model-q4-k-m",
		},
		{
			name: "replaces underscores from lm studio quantization names",
			in:   "lms-jiunsong-supergemma4-26b-uncensored-fast-v2-q4_k_m",
			want: "lms-jiunsong-supergemma4-26b-uncensored-fast-v2-q4-k-m",
		},
		{
			name: "collapses punctuation and trims separators",
			in:   "--L3.2__8X3B MOE/Q4_K_S--",
			want: "l3.2-8x3b-moe-q4-k-s",
		},
		{
			name: "shortens long lm studio names for ollama parser",
			in:   "lms-jiunsong-supergemma4-26b-uncensored-gguf-v2-supergemma4-26b-uncensored-fast-v2-q4_k_m",
			want: "lms-jiunsong-supergemma4-26b-uncensored-gguf-v2-supergemma4-26b-uncenso-894c964c",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := SanitizeModelName(tt.in); got != tt.want {
				t.Errorf("SanitizeModelName(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}

func TestSanitizeModelNameLengthLimit(t *testing.T) {
	got := SanitizeModelName("lms-davidau-llama-3-2-8x3b-moe-dark-champion-instruct-uncensored-abliterated-18-4b-gguf-l3-2-8x3b-moe-dark-champion-inst-18-4b-uncen-ablit_d_au-q4_k_s")
	if len(got) > maxOllamaModelNameLength {
		t.Fatalf("SanitizeModelName returned %d chars, want at most %d: %q", len(got), maxOllamaModelNameLength, got)
	}
	if got != "lms-davidau-llama-3-2-8x3b-moe-dark-champion-instruct-uncensored-ablite-d36bb606" {
		t.Errorf("SanitizeModelName returned %q", got)
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
		Name:           "test-model-latest",
		MainModelBlobs: []string{"sha256:blob11111"},
		AdditionalBlobs: map[string]string{
			"sha256:blob22222": "mmproj-test-model-latest.gguf",
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

	projectorSymlink := filepath.Join(modelDir, "mmproj-test-model-latest.gguf")
	infoProj, err := os.Lstat(projectorSymlink)
	if err != nil {
		t.Fatalf("Projector symlink was not created: %v", err)
	}
	if infoProj.Mode()&os.ModeSymlink == 0 {
		t.Errorf("Target is not a symbolic link: %s", projectorSymlink)
	}
}

func TestProcessModel_Sharded_Idempotency(t *testing.T) {
	tempDir := t.TempDir()
	ollamaDir := filepath.Join(tempDir, "ollama")
	blobsDir := filepath.Join(ollamaDir, "blobs")
	os.MkdirAll(blobsDir, 0755)

	shard1Blob := filepath.Join(blobsDir, "sha256-shard111")
	shard2Blob := filepath.Join(blobsDir, "sha256-shard222")
	os.WriteFile(shard1Blob, []byte("shard 1"), 0644)
	os.WriteFile(shard2Blob, []byte("shard 2"), 0644)

	providerDir := filepath.Join(tempDir, "lmstudio", "ollama")

	model := models.ModelInfo{
		Name: "sharded-model:latest",
		MainModelBlobs: []string{
			"sha256:shard111",
			"sha256:shard222",
		},
	}

	// 1. First run should create the sharded links
	res1 := ProcessModel(model, ollamaDir, providerDir, false, false, false)
	if !res1 {
		t.Fatal("first run of ProcessModel for sharded model failed")
	}

	modelDir := filepath.Join(providerDir, "sharded-model-latest")
	shard1Link := filepath.Join(modelDir, "sharded-model-latest-00001-of-00002.gguf")
	shard2Link := filepath.Join(modelDir, "sharded-model-latest-00002-of-00002.gguf")

	if fi, err := os.Lstat(shard1Link); err != nil || fi.Mode()&os.ModeSymlink == 0 {
		t.Fatalf("expected shard 1 symlink at %s", shard1Link)
	}
	if fi, err := os.Lstat(shard2Link); err != nil || fi.Mode()&os.ModeSymlink == 0 {
		t.Fatalf("expected shard 2 symlink at %s", shard2Link)
	}

	// 2. Second run should detect existing links and return false (skipped/idempotent), not error!
	res2 := ProcessModel(model, ollamaDir, providerDir, false, false, false)
	if res2 {
		t.Errorf("second run expected to return false (already exists), got true")
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

	result := ProcessLMStudioModel(model, ollamaDir, "lms", "", true, false, false)
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

func TestProcessLMStudioModel_ExecutionFailure(t *testing.T) {
	ollamaDir := t.TempDir()
	model := models.LMStudioModel{
		Name: "test-fail-model",
	}

	tempDir := t.TempDir()
	mockFile := filepath.Join(tempDir, "mock.gguf")
	os.WriteFile(mockFile, []byte("data"), 0644)
	model.Path = mockFile

	// Passing a non-existent binary path should fail at the exec.Command stage and return false
	result := ProcessLMStudioModel(model, ollamaDir, "lms", "/non/existent/binary/path/ollama", false, false, false)
	if result {
		t.Errorf("expected ProcessLMStudioModel to return false on execution failure, got true")
	}
}

func TestProcessLMStudioModel_VisionProjector_DryRun(t *testing.T) {
	ollamaDir := t.TempDir()
	tempDir := t.TempDir()

	baseFile := filepath.Join(tempDir, "model.gguf")
	os.WriteFile(baseFile, []byte("base model data"), 0644)

	projFile := filepath.Join(tempDir, "mmproj.gguf")
	os.WriteFile(projFile, []byte("projector data"), 0644)

	model := models.LMStudioModel{
		Name:          "vision-model",
		Path:          baseFile,
		ProjectorPath: projFile,
	}

	result := ProcessLMStudioModel(model, ollamaDir, "lms", "", true, true, false)
	if !result {
		t.Fatal("ProcessLMStudioModel dry run with projector failed")
	}

	// Verify no blobs were created
	blobsDir := filepath.Join(ollamaDir, "blobs")
	if entries, err := os.ReadDir(blobsDir); err == nil && len(entries) > 0 {
		t.Errorf("expected blobs to be empty in dry run, got %d entries", len(entries))
	}
}

func TestProcessLMStudioModel_VisionProjector_Success(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("skipping shell script mock on Windows")
	}

	ollamaDir := t.TempDir()
	tempDir := t.TempDir()

	baseFile := filepath.Join(tempDir, "model.gguf")
	os.WriteFile(baseFile, []byte("base model data"), 0644)

	projFile := filepath.Join(tempDir, "mmproj.gguf")
	os.WriteFile(projFile, []byte("projector data"), 0644)

	// Create mock ollama binary that verifies the Modelfile contents
	mockOllama := filepath.Join(tempDir, "mock_ollama")
	script := `#!/bin/sh
# $1=create $2=model_name $3=-f $4=modelfile
if [ "$1" != "create" ]; then
    echo "unexpected command $1" >&2
    exit 1
fi
if [ "$3" != "-f" ]; then
    echo "expected -f flag" >&2
    exit 1
fi
content=$(cat "$4")
echo "$content" | grep -F "FROM ` + baseFile + `" >/dev/null || { echo "missing base FROM" >&2; exit 1; }
echo "$content" | grep -F "FROM ` + projFile + `" >/dev/null || { echo "missing projector FROM" >&2; exit 1; }
exit 0
`
	if err := os.WriteFile(mockOllama, []byte(script), 0755); err != nil {
		t.Fatal(err)
	}

	model := models.LMStudioModel{
		Name:          "vision-model",
		Path:          baseFile,
		ProjectorPath: projFile,
	}

	result := ProcessLMStudioModel(model, ollamaDir, "lms", mockOllama, false, true, false)
	if !result {
		t.Fatal("ProcessLMStudioModel with projector failed")
	}

	// Verify both blobs were created as symlinks
	baseHash, _ := CalculateSHA256(baseFile)
	projHash, _ := CalculateSHA256(projFile)

	baseBlob := filepath.Join(ollamaDir, "blobs", "sha256-"+baseHash)
	projBlob := filepath.Join(ollamaDir, "blobs", "sha256-"+projHash)

	if fi, err := os.Lstat(baseBlob); err != nil || fi.Mode()&os.ModeSymlink == 0 {
		t.Errorf("expected base blob symlink at %s", baseBlob)
	}
	if fi, err := os.Lstat(projBlob); err != nil || fi.Mode()&os.ModeSymlink == 0 {
		t.Errorf("expected projector blob symlink at %s", projBlob)
	}
}

func TestProcessLMStudioModel_VisionProjector_NewlineInjection(t *testing.T) {
	ollamaDir := t.TempDir()
	tempDir := t.TempDir()

	baseFile := filepath.Join(tempDir, "model.gguf")
	os.WriteFile(baseFile, []byte("base model data"), 0644)

	model := models.LMStudioModel{
		Name:          "vision-model",
		Path:          baseFile,
		ProjectorPath: "/path/with/newline\n/proj.gguf",
	}

	result := ProcessLMStudioModel(model, ollamaDir, "lms", "", false, false, false)
	if result {
		t.Errorf("expected newline in projector path to be rejected, got true")
	}
}

func TestProcessLMStudioModel_VisionProjector_MissingProjector(t *testing.T) {
	ollamaDir := t.TempDir()
	tempDir := t.TempDir()

	baseFile := filepath.Join(tempDir, "model.gguf")
	os.WriteFile(baseFile, []byte("base model data"), 0644)

	model := models.LMStudioModel{
		Name:          "vision-model",
		Path:          baseFile,
		ProjectorPath: "/non/existent/mmproj.gguf",
	}

	result := ProcessLMStudioModel(model, ollamaDir, "lms", "", false, false, false)
	if result {
		t.Errorf("expected missing projector file to return false, got true")
	}
}

func TestProcessLMStudioModel_Sharded_DryRun(t *testing.T) {
	ollamaDir := t.TempDir()
	tempDir := t.TempDir()

	shard1 := filepath.Join(tempDir, "model-00001-of-00002.gguf")
	shard2 := filepath.Join(tempDir, "model-00002-of-00002.gguf")
	os.WriteFile(shard1, []byte("shard 1 content"), 0644)
	os.WriteFile(shard2, []byte("shard 2 content"), 0644)

	model := models.LMStudioModel{
		Name:       "sharded-model",
		Path:       shard1,
		ShardPaths: []string{shard2},
	}

	result := ProcessLMStudioModel(model, ollamaDir, "lms", "", true, true, false)
	if !result {
		t.Fatal("expected ProcessLMStudioModel sharded dry run to succeed")
	}

	// In dry-run, no blobs should have been created on disk
	blobsDir := filepath.Join(ollamaDir, "blobs")
	if entries, err := os.ReadDir(blobsDir); err == nil && len(entries) > 0 {
		t.Errorf("expected no blobs created in dry-run, found %d", len(entries))
	}
}

func TestProcessLMStudioModel_Sharded_Success(t *testing.T) {
	ollamaDir := t.TempDir()
	tempDir := t.TempDir()

	shard1 := filepath.Join(tempDir, "model-00001-of-00002.gguf")
	shard2 := filepath.Join(tempDir, "model-00002-of-00002.gguf")
	os.WriteFile(shard1, []byte("shard 1 payload"), 0644)
	os.WriteFile(shard2, []byte("shard 2 payload"), 0644)

	// Create mock ollama executable
	mockOllama := filepath.Join(tempDir, "ollama")
	var scriptContent string
	if os.Getenv("OS") == "Windows_NT" {
		scriptContent = "@echo off\r\nexit /b 0\r\n"
		mockOllama += ".bat"
	} else {
		scriptContent = "#!/bin/sh\nexit 0\n"
	}
	if err := os.WriteFile(mockOllama, []byte(scriptContent), 0755); err != nil {
		t.Fatal(err)
	}

	model := models.LMStudioModel{
		Name:       "sharded-model",
		Path:       shard1,
		ShardPaths: []string{shard2},
	}

	result := ProcessLMStudioModel(model, ollamaDir, "lms", mockOllama, false, true, false)
	if !result {
		t.Fatal("expected ProcessLMStudioModel with shards to succeed")
	}

	// Verify both shard 1 and shard 2 blobs exist in ollamaDir/blobs
	hash1, err := CalculateSHA256(shard1)
	if err != nil {
		t.Fatal(err)
	}
	hash2, err := CalculateSHA256(shard2)
	if err != nil {
		t.Fatal(err)
	}

	blob1 := filepath.Join(ollamaDir, "blobs", "sha256-"+hash1)
	blob2 := filepath.Join(ollamaDir, "blobs", "sha256-"+hash2)

	info1, err := os.Lstat(blob1)
	if err != nil || (info1.Mode()&os.ModeSymlink == 0) {
		t.Errorf("expected shard 1 blob symlink at %s", blob1)
	}

	info2, err := os.Lstat(blob2)
	if err != nil || (info2.Mode()&os.ModeSymlink == 0) {
		t.Errorf("expected shard 2 blob symlink at %s", blob2)
	}
}

func TestProcessLMStudioModel_Sharded_NewlineInjection(t *testing.T) {
	ollamaDir := t.TempDir()
	tempDir := t.TempDir()

	shard1 := filepath.Join(tempDir, "model-00001-of-00002.gguf")
	os.WriteFile(shard1, []byte("shard 1 content"), 0644)

	model := models.LMStudioModel{
		Name:       "sharded-model",
		Path:       shard1,
		ShardPaths: []string{"/bad/path\n/shard2.gguf"},
	}

	result := ProcessLMStudioModel(model, ollamaDir, "lms", "", false, false, false)
	if result {
		t.Errorf("expected newline in shard path to be rejected, got true")
	}
}


