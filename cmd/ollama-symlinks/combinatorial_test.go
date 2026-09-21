package main

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

func TestRunApp_CombinatorialFilesystem(t *testing.T) {
	tempDir := t.TempDir()
	ollamaDir := filepath.Join(tempDir, "ollama")
	lmstudioDir := filepath.Join(tempDir, "lmstudio")
	
	// Create required directories under ollamaDir
	os.MkdirAll(filepath.Join(ollamaDir, "blobs"), 0755)
	os.MkdirAll(filepath.Join(ollamaDir, "manifests", "registry.ollama.ai", "library"), 0755)
	
	// Create various models in lmstudio dir
	models := []string{
		"standard/model/model.gguf",
		"namespaced/vision/model.gguf",
		"namespaced/vision/mmproj.gguf",
		"sharded/model/model-00001-of-00002.gguf",
		"sharded/model/model-00002-of-00002.gguf",
		"combo/vision-sharded/model-00001-of-00002.gguf",
		"combo/vision-sharded/model-00002-of-00002.gguf",
		"combo/vision-sharded/mmproj-00001-of-00002.gguf",
		"combo/vision-sharded/mmproj-00002-of-00002.gguf",
	}
	
	for _, m := range models {
		p := filepath.Join(lmstudioDir, filepath.FromSlash(m))
		os.MkdirAll(filepath.Dir(p), 0755)
		os.WriteFile(p, []byte("fake gguf content for " + m), 0644)
	}

	// Run reverse command (LM Studio -> Ollama)
	args := []string{
		"--interactive=false",
		"--reverse",
		"--lmstudio-dir", lmstudioDir,
		"--ollama-dir", ollamaDir,
		"--skip-checks",
		"--verbose",
	}

	var stdin bytes.Buffer
	err := runApp(args, &stdin)
	if err != nil {
		t.Fatalf("runApp reverse failed: %v", err)
	}

	// Verify that symlinks were created in ollama blobs directory
	blobs, _ := os.ReadDir(filepath.Join(ollamaDir, "blobs"))
	if len(blobs) == 0 {
		t.Errorf("Expected blobs to be created in ollama/blobs, got none")
	}
	
	// Now let's setup for --forward. Since ollama create didn't run, we manually create a fake manifest.
	fakeManifest := `{"schemaVersion": 2,"mediaType": "application/vnd.docker.distribution.manifest.v2+json","config": {"mediaType": "application/vnd.docker.container.image.v1+json","digest": "sha256:configdigest","size": 123},"layers": [{"mediaType": "application/vnd.ollama.image.model","digest": "sha256:1111222233334444555566667777888899990000111122223333444455556666","size": 123}]}`
	manifestPath := filepath.Join(ollamaDir, "manifests", "registry.ollama.ai", "library", "mytestmodel", "latest")
	os.MkdirAll(filepath.Dir(manifestPath), 0755)
	os.WriteFile(manifestPath, []byte(fakeManifest), 0644)
	
	// And create the fake blob it points to
	blobPath := filepath.Join(ollamaDir, "blobs", "sha256-1111222233334444555566667777888899990000111122223333444455556666")
	os.WriteFile(blobPath, []byte("fake blob data"), 0644)

	lmstudioFwdDir := filepath.Join(tempDir, "lmstudio-fwd")
	os.MkdirAll(lmstudioFwdDir, 0755)
	
	args = []string{
		"--interactive=false",
		"--ollama-dir", ollamaDir,
		"--lmstudio-dir", lmstudioFwdDir,
		"--skip-checks",
		"--verbose",
	}
	
	err = runApp(args, &stdin)
	if err != nil {
		t.Fatalf("runApp forward failed: %v", err)
	}
	
	// Verify that symlinks were created in new lmstudio dir
	fwdModelsPath := filepath.Join(lmstudioFwdDir, "ollama")
	if _, err := os.Stat(fwdModelsPath); os.IsNotExist(err) {
		t.Errorf("Expected forwarded models in %s, but it does not exist", fwdModelsPath)
	}
}
