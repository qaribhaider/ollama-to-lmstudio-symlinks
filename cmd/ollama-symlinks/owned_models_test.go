package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/qaribhaider/ollama-to-lmstudio-symlinks/internal/models"
)

// ownedModelsFixture builds an Ollama blobs dir where each model's blob is
// created by its setup function.
func ownedModelsFixture(t *testing.T, setup map[string]func(blobPath string)) (string, []models.ModelInfo) {
	t.Helper()
	ollamaDir := t.TempDir()
	blobsDir := filepath.Join(ollamaDir, "blobs")
	if err := os.MkdirAll(blobsDir, 0755); err != nil {
		t.Fatal(err)
	}
	var all []models.ModelInfo
	for name, create := range setup {
		create(filepath.Join(blobsDir, "sha256-"+name))
		all = append(all, models.ModelInfo{Name: name + ":latest", MainModelBlobs: []string{"sha256:" + name}})
	}
	return ollamaDir, all
}

func modelNames(ms []models.ModelInfo) map[string]bool {
	out := map[string]bool{}
	for _, m := range ms {
		out[m.Name] = true
	}
	return out
}

func TestFindOwnedModels_IgnoresUserSymlinkedModels(t *testing.T) {
	external := filepath.Join(t.TempDir(), "external.gguf")
	os.WriteFile(external, []byte("weights on another disk"), 0644)

	ollamaDir, all := ownedModelsFixture(t, map[string]func(string){
		"mine-llama": func(p string) { os.Symlink(external, p) },
		"lms-llama":  func(p string) { os.Symlink(external, p) },
	})

	got := modelNames(findOwnedModels(ollamaDir, "lms", all))
	if got["mine-llama:latest"] {
		t.Error("a model the user symlinked themselves must not be offered for deletion")
	}
	if !got["lms-llama:latest"] {
		t.Error("a symlinked model with our prefix must be offered")
	}
}

func TestFindOwnedModels_PlainBlobWithOurPrefixIsNotOurs(t *testing.T) {
	ollamaDir, all := ownedModelsFixture(t, map[string]func(string){
		"lms-pulled": func(p string) { os.WriteFile(p, []byte("pulled with ollama pull"), 0644) },
	})

	if got := findOwnedModels(ollamaDir, "lms", all); len(got) != 0 {
		t.Errorf("a real blob (link count 1) must never be offered, got %v", modelNames(got))
	}
}

func TestFindOwnedModels_HardLinkedBlobWithOurPrefixIsOurs(t *testing.T) {
	source := filepath.Join(t.TempDir(), "source.gguf")
	os.WriteFile(source, []byte("weights"), 0644)

	ollamaDir, all := ownedModelsFixture(t, map[string]func(string){
		"lms-linked": func(p string) {
			if err := os.Link(source, p); err != nil {
				t.Skipf("hard links not supported here: %v", err)
			}
		},
	})

	if got := findOwnedModels(ollamaDir, "lms", all); !modelNames(got)["lms-linked:latest"] {
		t.Errorf("a hard-linked blob with our prefix must be offered, got %v", modelNames(got))
	}
}

func TestFindOwnedModels_UsesConfiguredPrefix(t *testing.T) {
	external := filepath.Join(t.TempDir(), "external.gguf")
	os.WriteFile(external, []byte("weights"), 0644)

	ollamaDir, all := ownedModelsFixture(t, map[string]func(string){
		"org-model": func(p string) { os.Symlink(external, p) },
	})
	// A missing blob with our prefix is a ghost left behind by a removed source.
	all = append(all, models.ModelInfo{Name: "org-ghost:latest", MainModelBlobs: []string{"sha256:ghost"}})

	got := modelNames(findOwnedModels(ollamaDir, "org", all))
	if !got["org-model:latest"] || !got["org-ghost:latest"] {
		t.Errorf("expected both org-prefixed models, got %v", got)
	}
}
