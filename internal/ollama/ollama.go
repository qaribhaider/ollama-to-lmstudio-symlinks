package ollama

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/qaribhaider/ollama-to-lmstudio-symlinks/internal/models"
)

func GetDefaultOllamaDir() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".ollama", "models")
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
		manifestData, err := os.ReadFile(path)
		if err != nil {
			if verbose {
				fmt.Printf("⚠️  Warning: Could not read manifest %s: %v\n", path, err)
			}
			return nil
		}

		var manifest models.OllamaManifest
		if err := json.Unmarshal(manifestData, &manifest); err != nil {
			if verbose {
				fmt.Printf("⚠️  Warning: Could not parse manifest %s: %v\n", path, err)
			}
			return nil
		}

		// Extract model name from path
		// Path format: .../manifests/registry.ollama.ai/library/model_name/variant
		relativePath := filepath.ToSlash(strings.TrimPrefix(path, manifestsDir))
		pathParts := strings.Split(strings.Trim(relativePath, "/"), "/")

		if len(pathParts) < 3 {
			if verbose {
				fmt.Printf("⚠️  Warning: Unexpected manifest path format: %s\n", path)
			}
			return nil
		}

		// Extract model name and variant
		modelName := pathParts[len(pathParts)-2]
		variant := pathParts[len(pathParts)-1]
		fullModelName := fmt.Sprintf("%s-%s", modelName, variant)

		// Parse layers to find model components
		modelInfo := models.ModelInfo{
			Name:            fullModelName,
			AdditionalBlobs: make(map[string]string),
		}

		for _, layer := range manifest.Layers {
			switch layer.MediaType {
			case "application/vnd.ollama.image.model":
				modelInfo.MainModelBlob = layer.Digest
			case "application/vnd.ollama.image.projector":
				// For multimodal models like llava
				projectorName := fmt.Sprintf("%s-projector.bin", fullModelName)
				modelInfo.AdditionalBlobs[layer.Digest] = projectorName
			}
		}

		if modelInfo.MainModelBlob == "" {
			if verbose {
				fmt.Printf("⚠️  Warning: No main model blob found for %s\n", fullModelName)
			}
			return nil
		}

		discoveredModels = append(discoveredModels, modelInfo)
		return nil
	})

	return discoveredModels, err
}
