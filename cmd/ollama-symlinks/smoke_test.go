//go:build smoke

package main

// Manual smoke tests against a real Ollama. Run with `make smoke`; they are not
// part of `go test ./...` or CI.
//
// They start their own `ollama serve` on a random port with a temporary model
// store, so a developer's real library and running Ollama are never touched.
// A small model is downloaded once into a cache under the user cache directory
// and cloned into each run with hard links.

import (
	"bytes"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/qaribhaider/ollama-to-lmstudio-symlinks/internal/linking"
	"github.com/qaribhaider/ollama-to-lmstudio-symlinks/internal/ollama"
)

func smokeModel() string {
	if m := os.Getenv("SMOKE_MODEL"); m != "" {
		return m
	}
	return "smollm2:135m"
}

func realOllama(t *testing.T) string {
	t.Helper()
	bin, err := exec.LookPath("ollama")
	if err != nil {
		t.Skip("ollama is not installed")
	}
	return bin
}

// startOllama runs a private server on modelsDir and returns its host:port.
func startOllama(t *testing.T, bin, modelsDir string) string {
	t.Helper()

	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	host := l.Addr().String()
	l.Close()

	var logs bytes.Buffer
	cmd := exec.Command(bin, "serve")
	cmd.Env = append(os.Environ(), "OLLAMA_HOST="+host, "OLLAMA_MODELS="+modelsDir)
	cmd.Stdout, cmd.Stderr = &logs, &logs
	if err := cmd.Start(); err != nil {
		t.Fatalf("could not start ollama serve: %v", err)
	}
	t.Cleanup(func() {
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
	})

	deadline := time.Now().Add(60 * time.Second)
	for time.Now().Before(deadline) {
		resp, err := http.Get("http://" + host + "/api/version")
		if err == nil {
			resp.Body.Close()
			return host
		}
		time.Sleep(250 * time.Millisecond)
	}
	t.Fatalf("ollama serve did not become ready on %s:\n%s", host, logs.String())
	return ""
}

func ollamaCmd(host, bin string, args ...string) *exec.Cmd {
	cmd := exec.Command(bin, args...)
	cmd.Env = append(os.Environ(), "OLLAMA_HOST="+host)
	return cmd
}

// cachedStore returns a model store holding the smoke model, downloading it on
// the first run.
func cachedStore(t *testing.T, bin string) string {
	t.Helper()
	cacheRoot, err := os.UserCacheDir()
	if err != nil {
		t.Fatal(err)
	}
	store := filepath.Join(cacheRoot, "ollama-symlinks-smoke", "models")
	if err := os.MkdirAll(store, 0o755); err != nil {
		t.Fatal(err)
	}

	if found, _ := ollama.DiscoverModels(store, false); len(found) > 0 {
		return store
	}

	t.Logf("downloading %s into %s (one time)", smokeModel(), store)
	host := startOllama(t, bin, store)
	if out, err := ollamaCmd(host, bin, "pull", smokeModel()).CombinedOutput(); err != nil {
		t.Fatalf("ollama pull %s failed: %v\n%s", smokeModel(), err, out)
	}
	return store
}

// cloneStore copies src into dst, hard-linking files so large blobs cost nothing.
func cloneStore(t *testing.T, src, dst string) {
	t.Helper()
	err := filepath.Walk(src, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(src, path)
		target := filepath.Join(dst, rel)
		if info.IsDir() {
			return os.MkdirAll(target, 0o755)
		}
		return linkOrCopy(path, target)
	})
	if err != nil {
		t.Fatalf("could not clone model store: %v", err)
	}
}

func linkOrCopy(src, dst string) error {
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}
	if err := os.Link(src, dst); err == nil {
		return nil
	}
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer out.Close()
	_, err = io.Copy(out, in)
	return err
}

// describeBlobs lists what is in a store's blobs directory, for failure output.
func describeBlobs(store string) string {
	entries, err := os.ReadDir(filepath.Join(store, "blobs"))
	if err != nil {
		return err.Error()
	}
	var b strings.Builder
	for _, e := range entries {
		info, err := os.Lstat(filepath.Join(store, "blobs", e.Name()))
		if err != nil {
			continue
		}
		kind := "file"
		if info.Mode()&os.ModeSymlink != 0 {
			kind = "symlink"
		}
		b.WriteString("  " + e.Name() + "  " + kind + "\n")
	}

	b.WriteString("manifests:\n")
	_ = filepath.Walk(filepath.Join(store, "manifests"), func(path string, info os.FileInfo, err error) error {
		if err == nil && !info.IsDir() {
			rel, _ := filepath.Rel(store, path)
			body, _ := os.ReadFile(path)
			b.WriteString("  " + rel + "\n    " + string(body) + "\n")
		}
		return nil
	})
	return b.String()
}

func hasGGUFMagic(path string) bool {
	f, err := os.Open(path)
	if err != nil {
		return false
	}
	defer f.Close()
	magic := make([]byte, 4)
	_, err = io.ReadFull(f, magic)
	return err == nil && string(magic) == "GGUF"
}

func TestSmoke_ForwardLinksRealModelIntoLMStudio(t *testing.T) {
	bin := realOllama(t)
	cache := cachedStore(t, bin)

	store := filepath.Join(t.TempDir(), "ollama")
	lmstudio := filepath.Join(t.TempDir(), "lmstudio")
	cloneStore(t, cache, store)
	os.MkdirAll(lmstudio, 0o755)

	args := []string{"--interactive=false", "--skip-checks", "--ollama-dir", store, "--lmstudio-dir", lmstudio}
	if err := runApp(args, strings.NewReader("")); err != nil {
		t.Fatalf("forward run failed: %v", err)
	}

	dirName := strings.NewReplacer(":", "-", "/", "-").Replace(smokeModel())
	link := filepath.Join(lmstudio, "ollama", dirName, dirName+".gguf")
	if info, err := os.Lstat(link); err != nil || info.Mode()&os.ModeSymlink == 0 {
		t.Fatalf("expected a symlink at %s, got %v / %v", link, info, err)
	}
	if !hasGGUFMagic(link) {
		t.Errorf("%s does not resolve to a GGUF file", link)
	}

	if err := runApp([]string{"status", "--ollama-dir", store, "--lmstudio-dir", lmstudio}, strings.NewReader("")); err != nil {
		t.Errorf("status failed: %v", err)
	}
	if err := runApp(args, strings.NewReader("")); err != nil {
		t.Errorf("a second forward run must be a harmless no-op, got %v", err)
	}
}

func TestSmoke_ReverseRegistersThenDeleteRemovesOnlyOurModel(t *testing.T) {
	bin := realOllama(t)
	cache := cachedStore(t, bin)

	// A GGUF to import: the model's own weights, copied out of the cache.
	models, err := ollama.DiscoverModels(cache, false)
	if err != nil || len(models) == 0 {
		t.Fatalf("no cached model to import: %v", err)
	}
	blob := filepath.Join(cache, "blobs", strings.Replace(models[0].MainModelBlobs[0], ":", "-", 1))
	lmstudio := filepath.Join(t.TempDir(), "lmstudio")
	source := filepath.Join(lmstudio, "pub", "tiny", "tiny.gguf")
	if err := linkOrCopy(blob, source); err != nil {
		t.Fatalf("could not stage the GGUF: %v", err)
	}

	// A separate empty store, so the blob is not already known to Ollama.
	store := filepath.Join(t.TempDir(), "ollama")
	os.MkdirAll(store, 0o755)
	host := startOllama(t, bin, store)
	t.Setenv("OLLAMA_HOST", host)

	args := []string{"--interactive=false", "--reverse", "--skip-checks", "--name-prefix", "smoke",
		"--ollama-dir", store, "--lmstudio-dir", lmstudio}
	if err := runApp(args, strings.NewReader("")); err != nil {
		t.Fatalf("reverse run failed: %v", err)
	}

	list, err := ollamaCmd(host, bin, "list").CombinedOutput()
	if err != nil || !strings.Contains(string(list), "smoke-pub-tiny") {
		t.Fatalf("expected smoke-pub-tiny in `ollama list`, got err=%v:\n%s", err, list)
	}

	// The tool must find exactly the model it registered, and nothing else.
	discovered, err := ollama.DiscoverModels(store, false)
	if err != nil {
		t.Fatal(err)
	}
	owned := findOwnedModels(store, "smoke", discovered)
	if len(owned) != 1 {
		t.Fatalf("expected 1 owned model, got %d.\ndiscovered: %+v\nblobs:\n%s", len(owned), discovered, describeBlobs(store))
	}

	removed, failed := removeOllamaModels(bin, owned, options{})
	if removed != 1 || failed != 0 {
		t.Fatalf("removeOllamaModels = (%d removed, %d failed), want (1, 0)", removed, failed)
	}

	list, _ = ollamaCmd(host, bin, "list").CombinedOutput()
	if strings.Contains(string(list), "smoke-pub-tiny") {
		t.Errorf("model still listed after delete:\n%s", list)
	}
	if broken, _ := linking.FindBrokenSymlinks(filepath.Join(store, "blobs")); len(broken) != 0 {
		t.Errorf("delete left broken blob links: %+v", broken)
	}
	if !hasGGUFMagic(source) {
		t.Fatal("the original GGUF in LM Studio must survive deleting the Ollama model")
	}
}
