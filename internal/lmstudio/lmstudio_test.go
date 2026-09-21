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
