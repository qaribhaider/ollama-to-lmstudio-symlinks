package ollama

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/qaribhaider/ollama-to-lmstudio-symlinks/internal/models"
)

var digestPattern = regexp.MustCompile(`^sha256:[0-9a-f]{64}$`)

// ValidateDigest ensures a digest matches the expected format (sha256:64hexchars)
// to prevent path traversal when constructing blob paths.
func ValidateDigest(digest string) error {
	if !digestPattern.MatchString(digest) {
		return fmt.Errorf("invalid digest format: %q", digest)
	}
	return nil
}

func GetDefaultOllamaDir() string {
	candidates := GetOllamaCandidates()
	if len(candidates) > 0 {
		return candidates[0]
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".ollama", "models")
}

func isSafePath(p string) bool {
	clean := filepath.Clean(p)
	if clean == "." || clean == "/" || clean == `\` || len(clean) == 2 && clean[1] == ':' {
		return false // Reject roots like /, \, C:
	}
	if len(clean) == 3 && clean[1] == ':' && clean[2] == '\\' {
		return false // Reject C:\
	}
	return true
}

// systemModelDirs lists where system-wide Ollama installs keep their models:
// the official install script's service user (/usr/share/ollama) and distro
// packages such as Arch Linux (/var/lib/ollama).
func systemModelDirs() []string {
	return []string{
		"/usr/share/ollama/.ollama/models",
		"/var/lib/ollama/.ollama/models",
	}
}

func GetOllamaCandidates() []string {
	var candidates []string

	// 1. Check OLLAMA_MODELS environment variable
	if env := os.Getenv("OLLAMA_MODELS"); env != "" && isSafePath(env) {
		candidates = append(candidates, filepath.Clean(env))
	}

	// 2. Default home directory location
	home, err := os.UserHomeDir()
	if err == nil {
		candidates = append(candidates, filepath.Join(home, ".ollama", "models"))
	}

	// 3. Linux system-wide installs
	if os.PathSeparator == '/' && os.Getenv("OS") != "Windows_NT" {
		candidates = append(candidates, systemModelDirs()...)
	}

	// 3. Windows-specific locations
	if os.Getenv("OS") == "Windows_NT" || os.PathSeparator == '\\' {
		// LocalAppData
		if local := os.Getenv("LOCALAPPDATA"); local != "" {
			candidates = append(candidates, filepath.Join(local, "Programs", "Ollama", "models"))
		}
		// Explicit AppData/Local path if environment variable is missing
		if home != "" {
			candidates = append(candidates, filepath.Join(home, "AppData", "Local", "Programs", "Ollama", "models"))
		}
	}

	// Filter to only include directories that actually exist
	var existing []string
	seen := make(map[string]bool)
	for _, c := range candidates {
		c = filepath.Clean(c)
		if seen[c] {
			continue
		}
		seen[c] = true
		if info, err := os.Stat(c); err == nil && info.IsDir() {
			existing = append(existing, c)
		}
	}

	return existing
}

// DiscoverModels lists the models in ollamaDir's manifests. Unreadable or
// invalid manifests are skipped so one bad file cannot hide the rest.
func DiscoverModels(ollamaDir string, verbose bool) ([]models.ModelInfo, error) {
	manifestsDir := filepath.Join(ollamaDir, "manifests")
	var discoveredModels []models.ModelInfo

	err := filepath.Walk(manifestsDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		// Skip directories and hidden files
		if info.IsDir() || strings.HasPrefix(info.Name(), ".") {
			return nil
		}

		relativePath := filepath.ToSlash(strings.TrimPrefix(path, manifestsDir))
		pathParts := strings.Split(strings.Trim(relativePath, "/"), "/")
		if len(pathParts) < 2 {
			if verbose {
				fmt.Printf("⚠️  Skipping non-model file in manifests: %s\n", path)
			}
			return nil
		}

		manifest, err := readManifest(path)
		if err != nil {
			if verbose {
				fmt.Printf("⚠️  Skipping manifest %s: %v\n", path, err)
			}
			return nil
		}

		fullModelName := modelNameFromManifestPath(pathParts)
		modelInfo, err := classifyLayers(fullModelName, manifest)
		if err != nil {
			// One corrupt manifest must not hide the rest of the library.
			fmt.Printf("⚠️  Skipping %s: %v\n", fullModelName, err)
			return nil
		}

		if len(modelInfo.MainModelBlobs) == 0 {
			if verbose {
				fmt.Printf("⚠️  Skipping %s: no main model blob found (unsupported media type or sharded model)\n", fullModelName)
			}
			return nil
		}

		discoveredModels = append(discoveredModels, modelInfo)
		return nil
	})

	return discoveredModels, err
}

func readManifest(path string) (models.OllamaManifest, error) {
	var manifest models.OllamaManifest
	// G304: path is derived from filepath.Walk, which scans the manifestsDir.
	// We clean the path to ensure it's normalized before reading.
	data, err := os.ReadFile(filepath.Clean(path))
	if err != nil {
		return manifest, fmt.Errorf("could not read: %w", err)
	}
	if err := json.Unmarshal(data, &manifest); err != nil {
		return manifest, fmt.Errorf("unparseable: %w", err)
	}
	return manifest, nil
}

// modelNameFromManifestPath builds "name:variant" from the path parts below the
// manifests directory:
//
//	registry.ollama.ai/library/model_name/variant -> model_name:variant
//	registry.ollama.ai/author/model_name/variant  -> author/model_name:variant
//	hf.co/author/model_name/variant               -> hf.co/author/model_name:variant
func modelNameFromManifestPath(pathParts []string) string {
	variant := pathParts[len(pathParts)-1]

	var identifier string
	switch {
	case len(pathParts) == 2:
		identifier = pathParts[0]
	case pathParts[0] == "registry.ollama.ai" && pathParts[1] == "library":
		identifier = strings.Join(pathParts[2:len(pathParts)-1], "/")
	case pathParts[0] == "registry.ollama.ai":
		identifier = strings.Join(pathParts[1:len(pathParts)-1], "/")
	default:
		identifier = strings.Join(pathParts[:len(pathParts)-1], "/")
	}
	return fmt.Sprintf("%s:%s", identifier, variant)
}

// classifyLayers sorts a manifest's layers into the main model blobs and extra
// components such as a vision projector. It fails on an invalid digest.
func classifyLayers(fullModelName string, manifest models.OllamaManifest) (models.ModelInfo, error) {
	modelInfo := models.ModelInfo{
		Name:            fullModelName,
		AdditionalBlobs: make(map[string]string),
	}

	for _, layer := range manifest.Layers {
		if err := ValidateDigest(layer.Digest); err != nil {
			return modelInfo, fmt.Errorf("invalid layer: %w", err)
		}

		switch {
		case strings.HasPrefix(layer.MediaType, "application/vnd.ollama.image.model"):
			modelInfo.MainModelBlobs = append(modelInfo.MainModelBlobs, layer.Digest)
		case layer.MediaType == "application/vnd.ollama.image.projector":
			// For multimodal models like llava. The filename follows the LM Studio
			// mmproj convention and must be safe for the filesystem.
			safeName := strings.NewReplacer(":", "-", "/", "-").Replace(fullModelName)
			modelInfo.AdditionalBlobs[layer.Digest] = fmt.Sprintf("mmproj-%s.gguf", safeName)
		}
	}
	return modelInfo, nil
}

func ScanAllDrives() []string {
	if os.Getenv("OS") != "Windows_NT" && os.PathSeparator != '\\' {
		return nil
	}

	var found []string
	// Common relative paths on Windows
	relPaths := []string{
		filepath.Join(".ollama", "models"),
		filepath.Join("AppData", "Local", "Programs", "Ollama", "models"),
		filepath.Join("AppData", "Local", "Ollama", "models"),
	}

	for _, driveLetter := range "ABCDEFGHIJKLMNOPQRSTUVWXYZ" {
		drive := string(driveLetter) + ":\\"
		// Use os.Stat to check if drive root is accessible
		if _, err := os.Stat(drive); err != nil {
			continue
		}

		// Check drive root (some users put models there)
		if info, err := os.Stat(filepath.Join(drive, "ollama", "models")); err == nil && info.IsDir() {
			found = append(found, filepath.Join(drive, "ollama", "models"))
		}

		// Check user profiles on this drive
		usersDir := filepath.Join(drive, "Users")
		entries, err := os.ReadDir(usersDir)
		if err != nil {
			continue
		}

		for _, entry := range entries {
			if !entry.IsDir() {
				continue
			}
			userProfile := filepath.Join(usersDir, entry.Name())
			for _, rel := range relPaths {
				target := filepath.Join(userProfile, rel)
				if info, err := os.Stat(target); err == nil && info.IsDir() {
					found = append(found, target)
				}
			}
		}
	}

	return found
}
