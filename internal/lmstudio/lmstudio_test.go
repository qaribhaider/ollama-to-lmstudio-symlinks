package lmstudio

import (
	"bytes"
	"encoding/binary"
	"os"
	"path/filepath"
	"testing"
)

func TestGetDefaultDirs(t *testing.T) {
	// Test default behavior
	home, err := os.UserHomeDir()
	if err != nil {
		t.Fatalf("Failed to get user home dir: %v", err)
	}

	lmStudioDir := GetDefaultLMStudioDir()
	expectedLMStudio := filepath.Join(home, ".cache", "lm-studio", "models")
	if lmStudioDir != expectedLMStudio {
		t.Errorf("Expected LM Studio dir %s, got %s", expectedLMStudio, lmStudioDir)
	}

	// Test LMSTUDIO_MODELS env var
	customPath := filepath.Join(t.TempDir(), "custom", "lm-studio", "models")
	os.MkdirAll(customPath, 0755)
	os.Setenv("LMSTUDIO_MODELS", customPath)
	defer os.Unsetenv("LMSTUDIO_MODELS")

	lmStudioDir = GetDefaultLMStudioDir()
	if lmStudioDir != customPath {
		t.Errorf("Expected custom LM Studio dir %s, got %s", customPath, lmStudioDir)
	}
}

func TestGetLMStudioCandidates(t *testing.T) {
	tempDir := t.TempDir()
	
	// Create mock directories
	path1 := filepath.Join(tempDir, "opt1")
	path2 := filepath.Join(tempDir, "opt2")
	os.MkdirAll(path1, 0755)
	os.MkdirAll(path2, 0755)

	// Set env var to point to one of them
	os.Setenv("LMSTUDIO_MODELS", path1)
	defer os.Unsetenv("LMSTUDIO_MODELS")

	candidates := GetLMStudioCandidates()
	
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

	// Test unsafe path rejection
	os.Setenv("LMSTUDIO_MODELS", "/")
	candidates = GetLMStudioCandidates()
	for _, c := range candidates {
		if c == "/" || c == `\` {
			t.Errorf("Unsafe path %s was not rejected!", c)
		}
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

func writeSyntheticGGUF(t *testing.T, path, arch string) {
	buf := new(bytes.Buffer)
	buf.Write([]byte{'G', 'G', 'U', 'F'})
	binary.Write(buf, binary.LittleEndian, uint32(3))
	binary.Write(buf, binary.LittleEndian, uint64(1))
	binary.Write(buf, binary.LittleEndian, uint64(1))

	key := "general.architecture"
	binary.Write(buf, binary.LittleEndian, uint64(len(key)))
	buf.WriteString(key)
	binary.Write(buf, binary.LittleEndian, uint32(8)) // string type
	binary.Write(buf, binary.LittleEndian, uint64(len(arch)))
	buf.WriteString(arch)

	if err := os.WriteFile(path, buf.Bytes(), 0644); err != nil {
		t.Fatal(err)
	}
}

func TestDeduceModelRole(t *testing.T) {
	tempDir := t.TempDir()

	// 1. Valid GGUF with base architecture 'llama'
	llamaPath := filepath.Join(tempDir, "model.gguf")
	writeSyntheticGGUF(t, llamaPath, "llama")
	isProj, arch := DeduceModelRole(llamaPath)
	if isProj || arch != "llama" {
		t.Errorf("expected (false, 'llama'), got (%v, '%s')", isProj, arch)
	}

	// 2. Valid GGUF with base architecture 'llama' but named mmproj (header takes precedence)
	weirdBase := filepath.Join(tempDir, "mmproj-weird.gguf")
	writeSyntheticGGUF(t, weirdBase, "llama")
	isProj, arch = DeduceModelRole(weirdBase)
	if isProj || arch != "llama" {
		t.Errorf("expected header to take precedence (false, 'llama'), got (%v, '%s')", isProj, arch)
	}

	// 3. Valid GGUF with projector architecture 'clip'
	clipPath := filepath.Join(tempDir, "vision.gguf")
	writeSyntheticGGUF(t, clipPath, "clip")
	isProj, arch = DeduceModelRole(clipPath)
	if !isProj || arch != "clip" {
		t.Errorf("expected (true, 'clip'), got (%v, '%s')", isProj, arch)
	}

	// 4. Valid GGUF with projector architecture 'siglip'
	siglipPath := filepath.Join(tempDir, "adapter.gguf")
	writeSyntheticGGUF(t, siglipPath, "siglip")
	isProj, arch = DeduceModelRole(siglipPath)
	if !isProj || arch != "siglip" {
		t.Errorf("expected (true, 'siglip'), got (%v, '%s')", isProj, arch)
	}

	// 5. Plain text / mock files (fallback to filename)
	mockProj := filepath.Join(tempDir, "mmproj-gemma-f16.gguf")
	os.WriteFile(mockProj, []byte("plain text not gguf"), 0644)
	isProj, arch = DeduceModelRole(mockProj)
	if !isProj {
		t.Errorf("expected fallback text match to detect projector, got false")
	}

	mockBase := filepath.Join(tempDir, "gemma-q4_0.gguf")
	os.WriteFile(mockBase, []byte("plain text not gguf"), 0644)
	isProj, arch = DeduceModelRole(mockBase)
	if isProj {
		t.Errorf("expected fallback text match to detect base model, got true")
	}
}

func TestDiscoverLMStudioModels_MultimodalPairing(t *testing.T) {
	tempDir := t.TempDir()
	modelDir := filepath.Join(tempDir, "lmstudio-community", "gemma-4-12B-it-QAT-GGUF")
	if err := os.MkdirAll(modelDir, 0755); err != nil {
		t.Fatal(err)
	}

	basePath := filepath.Join(modelDir, "gemma-4-12b-it-qat-q4_0.gguf")
	writeSyntheticGGUF(t, basePath, "gemma4")

	projPath := filepath.Join(modelDir, "mmproj-gemma-4-12b-it-qat-bf16.gguf")
	writeSyntheticGGUF(t, projPath, "clip")

	models, err := DiscoverLMStudioModels(tempDir, "ollama", true)
	if err != nil {
		t.Fatalf("DiscoverLMStudioModels failed: %v", err)
	}

	// Expect exactly 1 model discovered (base model), with ProjectorPath set!
	// mmproj should NOT be a standalone model.
	if len(models) != 1 {
		t.Fatalf("expected 1 model, got %d", len(models))
	}

	m := models[0]
	if m.Path != basePath {
		t.Errorf("expected model path %s, got %s", basePath, m.Path)
	}
	if m.ProjectorPath != projPath {
		t.Errorf("expected projector path %s, got %s", projPath, m.ProjectorPath)
	}
}

func TestDiscoverLMStudioModels_MultipleQuantSharedProjector(t *testing.T) {
	tempDir := t.TempDir()
	modelDir := filepath.Join(tempDir, "author", "vision-model")
	if err := os.MkdirAll(modelDir, 0755); err != nil {
		t.Fatal(err)
	}

	baseQ4 := filepath.Join(modelDir, "model-q4_k_m.gguf")
	writeSyntheticGGUF(t, baseQ4, "llama")

	baseQ8 := filepath.Join(modelDir, "model-q8_0.gguf")
	writeSyntheticGGUF(t, baseQ8, "llama")

	projPath := filepath.Join(modelDir, "mmproj-model-f16.gguf")
	writeSyntheticGGUF(t, projPath, "clip")

	models, err := DiscoverLMStudioModels(tempDir, "ollama", false)
	if err != nil {
		t.Fatalf("DiscoverLMStudioModels failed: %v", err)
	}

	// Both base quantizations should be discovered, each paired with the projector!
	if len(models) != 2 {
		t.Fatalf("expected 2 models, got %d", len(models))
	}

	for _, m := range models {
		if m.ProjectorPath != projPath {
			t.Errorf("model %s expected ProjectorPath %s, got %s", m.Name, projPath, m.ProjectorPath)
		}
	}
}

func TestDiscoverLMStudioModels_OrphanProjector(t *testing.T) {
	tempDir := t.TempDir()
	modelDir := filepath.Join(tempDir, "author", "orphan-projector")
	if err := os.MkdirAll(modelDir, 0755); err != nil {
		t.Fatal(err)
	}

	projPath := filepath.Join(modelDir, "mmproj-standalone.gguf")
	writeSyntheticGGUF(t, projPath, "clip")

	models, err := DiscoverLMStudioModels(tempDir, "ollama", true)
	if err != nil {
		t.Fatalf("DiscoverLMStudioModels failed: %v", err)
	}

	// Expect 0 models discovered because projector has no base model in directory
	if len(models) != 0 {
		t.Fatalf("expected 0 models for orphan projector, got %d", len(models))
	}
}

func TestDiscoverLMStudioModels_ShardedModel(t *testing.T) {
	tempDir := t.TempDir()
	modelDir := filepath.Join(tempDir, "meta-llama", "Meta-Llama-3-70B-Instruct-GGUF")
	if err := os.MkdirAll(modelDir, 0755); err != nil {
		t.Fatal(err)
	}

	shard1 := filepath.Join(modelDir, "Meta-Llama-3-70B-Instruct-Q4_K_M-00001-of-00003.gguf")
	shard2 := filepath.Join(modelDir, "Meta-Llama-3-70B-Instruct-Q4_K_M-00002-of-00003.gguf")
	shard3 := filepath.Join(modelDir, "Meta-Llama-3-70B-Instruct-Q4_K_M-00003-of-00003.gguf")

	writeSyntheticGGUF(t, shard1, "llama")
	writeSyntheticGGUF(t, shard2, "llama")
	writeSyntheticGGUF(t, shard3, "llama")

	models, err := DiscoverLMStudioModels(tempDir, "ollama", true)
	if err != nil {
		t.Fatalf("DiscoverLMStudioModels failed: %v", err)
	}

	// Expect exactly 1 model representing all 3 shards
	if len(models) != 1 {
		t.Fatalf("expected 1 model, got %d", len(models))
	}

	m := models[0]
	if m.Path != shard1 {
		t.Errorf("expected primary shard path %s, got %s", shard1, m.Path)
	}

	if len(m.ShardPaths) != 2 {
		t.Fatalf("expected 2 companion shards, got %d", len(m.ShardPaths))
	}

	if m.ShardPaths[0] != shard2 || m.ShardPaths[1] != shard3 {
		t.Errorf("companion shards out of order: got %v", m.ShardPaths)
	}

	// Verify name does NOT retain the shard suffix "-00001-of-00003"
	if m.Name == "" || bytes.Contains([]byte(m.Name), []byte("00001-of-00003")) {
		t.Errorf("expected cleaned model name without shard suffix, got %s", m.Name)
	}
}

func TestDiscoverLMStudioModels_ShardedAndVision(t *testing.T) {
	tempDir := t.TempDir()
	modelDir := filepath.Join(tempDir, "author", "vision-sharded")
	if err := os.MkdirAll(modelDir, 0755); err != nil {
		t.Fatal(err)
	}

	shard1 := filepath.Join(modelDir, "model-00001-of-00002.gguf")
	shard2 := filepath.Join(modelDir, "model-00002-of-00002.gguf")
	proj := filepath.Join(modelDir, "mmproj-model.gguf")

	writeSyntheticGGUF(t, shard1, "llama")
	writeSyntheticGGUF(t, shard2, "llama")
	writeSyntheticGGUF(t, proj, "clip")

	models, err := DiscoverLMStudioModels(tempDir, "ollama", false)
	if err != nil {
		t.Fatalf("DiscoverLMStudioModels failed: %v", err)
	}

	if len(models) != 1 {
		t.Fatalf("expected 1 model, got %d", len(models))
	}

	m := models[0]
	if m.Path != shard1 {
		t.Errorf("expected primary path %s, got %s", shard1, m.Path)
	}
	if len(m.ShardPaths) != 1 || m.ShardPaths[0] != shard2 {
		t.Errorf("expected companion shard %s, got %v", shard2, m.ShardPaths)
	}
	if m.ProjectorPath != proj {
		t.Errorf("expected projector %s, got %s", proj, m.ProjectorPath)
	}
}

func TestDiscoverLMStudioModels_IncompleteShards(t *testing.T) {
	tempDir := t.TempDir()
	modelDir := filepath.Join(tempDir, "author", "broken-sharded")
	if err := os.MkdirAll(modelDir, 0755); err != nil {
		t.Fatal(err)
	}

	// Missing shard 1 - only shard 2 is present
	shard2 := filepath.Join(modelDir, "model-00002-of-00003.gguf")
	writeSyntheticGGUF(t, shard2, "llama")

	models, err := DiscoverLMStudioModels(tempDir, "ollama", true)
	if err != nil {
		t.Fatalf("DiscoverLMStudioModels failed: %v", err)
	}

	// Should be skipped because shard 1 is missing
	if len(models) != 0 {
		t.Fatalf("expected 0 models for missing shard 1, got %d", len(models))
	}
}
