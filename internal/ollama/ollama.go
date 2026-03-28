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

// ValidateDigest ensures a digest matches the expected format (sha256:64hexchars)
// to prevent path traversal when constructing blob paths.
func ValidateDigest(digest string) error {
	// Pattern matched lazily to avoid init cycles if needed, but safe at package level
	// Actually we can just define it inside or as a package var.
	matched, _ := regexp.MatchString(`^sha256:[0-9a-f]{64}$`, digest)
	if !matched {
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

func GetOllamaCandidates() []string {
	var candidates []string

	// 1. Check OLLAMA_MODELS environment variable
	if env := os.Getenv("OLLAMA_MODELS"); env != "" {
		candidates = append(candidates, filepath.Clean(env))
	}

	// 2. Default home directory location
	home, err := os.UserHomeDir()
	if err == nil {
		candidates = append(candidates, filepath.Join(home, ".ollama", "models"))
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

		// Parse manifest file
		// G304: path is derived from filepath.Walk, which scans the manifestsDir.
		// We clean the path to ensure it's normalized before reading.
		manifestData, err := os.ReadFile(filepath.Clean(path))
		if err != nil {
			return fmt.Errorf("could not read manifest %s: %w", path, err)
		}

		var manifest models.OllamaManifest
		if err := json.Unmarshal(manifestData, &manifest); err != nil {
			return fmt.Errorf("could not parse manifest %s: %w", path, err)
		}

		// Extract model name from path
		// Path format: .../manifests/registry.ollama.ai/library/model_name/variant
		relativePath := filepath.ToSlash(strings.TrimPrefix(path, manifestsDir))
		pathParts := strings.Split(strings.Trim(relativePath, "/"), "/")

		if len(pathParts) < 3 {
			return fmt.Errorf("unexpected manifest path format: %s", path)
		}

		// Extract model name and variant
		modelName := pathParts[len(pathParts)-2]
		variant := pathParts[len(pathParts)-1]
		fullModelName := fmt.Sprintf("%s:%s", modelName, variant)

		// Parse layers to find model components
		modelInfo := models.ModelInfo{
			Name:            fullModelName,
			AdditionalBlobs: make(map[string]string),
		}

		for _, layer := range manifest.Layers {
			if err := ValidateDigest(layer.Digest); err != nil {
				return fmt.Errorf("invalid layer in %s: %w", path, err)
			}

			switch layer.MediaType {
			case "application/vnd.ollama.image.model":
				modelInfo.MainModelBlob = layer.Digest
			case "application/vnd.ollama.image.projector":
				// For multimodal models like llava
				// Ensure filename is safe for filesystem
				safeProjectorName := strings.Replace(fullModelName, ":", "-", -1)
				projectorName := fmt.Sprintf("%s-projector.bin", safeProjectorName)
				modelInfo.AdditionalBlobs[layer.Digest] = projectorName
			}
		}

		if modelInfo.MainModelBlob == "" {
			return fmt.Errorf("no main model blob found for %s", fullModelName)
		}

		discoveredModels = append(discoveredModels, modelInfo)
		return nil
	})

	return discoveredModels, err
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
