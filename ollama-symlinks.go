package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
)

// OllamaManifest represents the structure of an Ollama model manifest
type OllamaManifest struct {
	SchemaVersion int    `json:"schemaVersion"`
	MediaType     string `json:"mediaType"`
	Config        struct {
		MediaType string `json:"mediaType"`
		Digest    string `json:"digest"`
		Size      int64  `json:"size"`
	} `json:"config"`
	Layers []struct {
		MediaType string `json:"mediaType"`
		Digest    string `json:"digest"`
		Size      int64  `json:"size"`
	} `json:"layers"`
}

// ModelInfo holds information about a discovered model
type ModelInfo struct {
	Name            string
	MainModelBlob   string
	AdditionalBlobs map[string]string // blob_hash -> suggested_filename
}

func main() {
	// Command line flags
	var ollamaDir = flag.String("ollama-dir", getDefaultOllamaDir(), "Path to Ollama models directory")
	var lmstudioDir = flag.String("lmstudio-dir", getDefaultLMStudioDir(), "Path to LM Studio models directory")
	var dryRun = flag.Bool("dry-run", false, "Show what would be done without actually creating symlinks")
	var verbose = flag.Bool("verbose", false, "Enable verbose output")
	flag.Parse()

	fmt.Printf("🔍 Scanning Ollama models in: %s\n", *ollamaDir)
	fmt.Printf("🎯 Target LM Studio directory: %s\n", *lmstudioDir)
	if *dryRun {
		fmt.Println("🧪 DRY RUN MODE - No changes will be made")
	}
	fmt.Println()

	// Discover models
	models, err := discoverModels(*ollamaDir, *verbose)
	if err != nil {
		log.Fatalf("Error discovering models: %v", err)
	}

	if len(models) == 0 {
		fmt.Println("❌ No models found in Ollama directory")
		return
	}

	fmt.Printf("📦 Found %d models:\n", len(models))
	for _, model := range models {
		fmt.Printf("  • %s\n", model.Name)
	}
	fmt.Println()

	// Create ollama provider directory
	ollamaProviderDir := filepath.Join(*lmstudioDir, "ollama")
	if !*dryRun {
		if err := os.MkdirAll(ollamaProviderDir, 0755); err != nil {
			log.Fatalf("Error creating ollama provider directory: %v", err)
		}
	}

	// Process each model
	var created, skipped int
	for _, model := range models {
		result := processModel(model, *ollamaDir, ollamaProviderDir, *dryRun, *verbose)
		if result {
			created++
		} else {
			skipped++
		}
	}

	// Summary
	fmt.Println()
	fmt.Printf("✅ Summary: %d created, %d skipped\n", created, skipped)
	if created > 0 && !*dryRun {
		fmt.Printf("🎉 Models are now available in LM Studio under the 'ollama' provider\n")
	}
}

func getDefaultOllamaDir() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".ollama", "models")
}

func getDefaultLMStudioDir() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".cache", "lm-studio", "models")
}

func discoverModels(ollamaDir string, verbose bool) ([]ModelInfo, error) {
	manifestsDir := filepath.Join(ollamaDir, "manifests")
	var models []ModelInfo

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

		var manifest OllamaManifest
		if err := json.Unmarshal(manifestData, &manifest); err != nil {
			if verbose {
				fmt.Printf("⚠️  Warning: Could not parse manifest %s: %v\n", path, err)
			}
			return nil
		}

		// Extract model name from path
		// Path format: .../manifests/registry.ollama.ai/library/model_name/variant
		relativePath := strings.TrimPrefix(path, manifestsDir)
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
		modelInfo := ModelInfo{
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

		models = append(models, modelInfo)
		return nil
	})

	return models, err
}

func processModel(model ModelInfo, ollamaDir, ollamaProviderDir string, dryRun, verbose bool) bool {
	modelDir := filepath.Join(ollamaProviderDir, model.Name)
	mainModelPath := filepath.Join(modelDir, model.Name+".gguf")

	// Check if main model symlink already exists
	if _, err := os.Lstat(mainModelPath); err == nil {
		fmt.Printf("⏭️  SKIPPED: %s (already exists)\n", model.Name)
		return false
	}

	fmt.Printf("🔗 CREATING: %s\n", model.Name)

	if !dryRun {
		// Create model directory
		if err := os.MkdirAll(modelDir, 0755); err != nil {
			fmt.Printf("❌ ERROR: Could not create directory for %s: %v\n", model.Name, err)
			return false
		}

		// Create main model symlink
		// Convert digest format from "sha256:hash" to "sha256-hash" for blob filename
		blobFilename := strings.Replace(model.MainModelBlob, ":", "-", 1)
		sourcePath := filepath.Join(ollamaDir, "blobs", blobFilename)
		if err := os.Symlink(sourcePath, mainModelPath); err != nil {
			fmt.Printf("❌ ERROR: Could not create symlink for %s: %v\n", model.Name, err)
			return false
		}

		if verbose {
			fmt.Printf("  ✅ Main model: %s -> %s\n", mainModelPath, sourcePath)
		}

		// Create additional component symlinks (e.g., projector for llava)
		for blobHash, filename := range model.AdditionalBlobs {
			additionalPath := filepath.Join(modelDir, filename)
			// Convert digest format from "sha256:hash" to "sha256-hash" for blob filename
			blobFilename := strings.Replace(blobHash, ":", "-", 1)
			additionalSource := filepath.Join(ollamaDir, "blobs", blobFilename)

			// Skip if already exists
			if _, err := os.Lstat(additionalPath); err == nil {
				if verbose {
					fmt.Printf("  ⏭️  Additional component %s already exists\n", filename)
				}
				continue
			}

			if err := os.Symlink(additionalSource, additionalPath); err != nil {
				fmt.Printf("⚠️  Warning: Could not create additional symlink %s: %v\n", filename, err)
			} else if verbose {
				fmt.Printf("  ✅ Additional: %s -> %s\n", additionalPath, additionalSource)
			}
		}
	} else {
		// Dry run - just show what would be done
		blobFilename := strings.Replace(model.MainModelBlob, ":", "-", 1)
		sourcePath := filepath.Join(ollamaDir, "blobs", blobFilename)
		fmt.Printf("  Would create: %s -> %s\n", mainModelPath, sourcePath)

		for blobHash, filename := range model.AdditionalBlobs {
			additionalPath := filepath.Join(modelDir, filename)
			blobFilename := strings.Replace(blobHash, ":", "-", 1)
			additionalSource := filepath.Join(ollamaDir, "blobs", blobFilename)
			fmt.Printf("  Would create: %s -> %s\n", additionalPath, additionalSource)
		}
	}

	return true
}
