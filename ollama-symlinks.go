package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// Version of the application
var Version = "dev"

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

// LMStudioModel holds information about a model found in LM Studio
type LMStudioModel struct {
	Name string
	Path string
}

func main() {
	// Command line flags
	var ollamaDir = flag.String("ollama-dir", getDefaultOllamaDir(), "Path to Ollama models directory")
	var lmstudioDir = flag.String("lmstudio-dir", getDefaultLMStudioDir(), "Path to LM Studio models directory")
	var dryRun = flag.Bool("dry-run", false, "Show what would be done without actually creating symlinks")
	var verbose = flag.Bool("verbose", false, "Enable verbose output")
	var showVersion = flag.Bool("version", false, "Show version information")

	// Reverse mode flags
	var reverse = flag.Bool("reverse", false, "Link LM Studio models to Ollama (reverse mode)")
	var namePrefix = flag.String("name-prefix", "lms", "Prefix for models created in Ollama")
	var skipProvider = flag.String("skip-provider", "ollama", "Directory name to skip in LM Studio (to avoid circular links)")

	flag.Parse()

	if *showVersion {
		fmt.Printf("ollama-symlinks version %s\n", Version)
		os.Exit(0)
	}

	if *reverse {
		runReverse(*lmstudioDir, *ollamaDir, *namePrefix, *skipProvider, *dryRun, *verbose)
	} else {
		runForward(*ollamaDir, *lmstudioDir, *dryRun, *verbose)
	}
}

func runForward(ollamaDir, lmstudioDir string, dryRun, verbose bool) {
	fmt.Printf("🔍 Scanning Ollama models in: %s\n", ollamaDir)
	fmt.Printf("🎯 Target LM Studio directory: %s\n", lmstudioDir)
	if dryRun {
		fmt.Println("🧪 DRY RUN MODE - No changes will be made")
	}
	fmt.Println()

	// Discover models
	models, err := discoverModels(ollamaDir, verbose)
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
	ollamaProviderDir := filepath.Join(lmstudioDir, "ollama")
	if !dryRun {
		if err := os.MkdirAll(ollamaProviderDir, 0755); err != nil {
			log.Fatalf("Error creating ollama provider directory: %v", err)
		}
	}

	// Process each model
	var created, skipped int
	for _, model := range models {
		result := processModel(model, ollamaDir, ollamaProviderDir, dryRun, verbose)
		if result {
			created++
		} else {
			skipped++
		}
	}

	// Summary
	fmt.Println()
	fmt.Printf("✅ Summary: %d created, %d skipped\n", created, skipped)
	if created > 0 && !dryRun {
		fmt.Printf("🎉 Models are now available in LM Studio under the 'ollama' provider\n")
	}
}

func runReverse(lmstudioDir, ollamaDir, namePrefix, skipProvider string, dryRun, verbose bool) {
	fmt.Printf("🔍 Scanning LM Studio models in: %s\n", lmstudioDir)
	fmt.Printf("🎯 Target Ollama directory: %s\n", ollamaDir)
	if dryRun {
		fmt.Println("🧪 DRY RUN MODE - No changes will be made")
	}
	fmt.Println()

	// Discover models
	models, err := discoverLMStudioModels(lmstudioDir, skipProvider, verbose)
	if err != nil {
		log.Fatalf("Error discovering models: %v", err)
	}

	if len(models) == 0 {
		fmt.Println("❌ No eligible models found in LM Studio directory")
		return
	}

	fmt.Printf("📦 Found %d eligible models:\n", len(models))
	for _, model := range models {
		fmt.Printf("  • %s (%s)\n", model.Name, model.Path)
	}
	fmt.Println()

	// Process each model
	var created, skipped int
	for _, model := range models {
		result := processLMStudioModel(model, ollamaDir, namePrefix, dryRun, verbose)
		if result {
			created++
		} else {
			skipped++
		}
	}

	// Summary
	fmt.Println()
	fmt.Printf("✅ Summary: %d created, %d skipped\n", created, skipped)
	if created > 0 && !dryRun {
		fmt.Printf("🎉 Models are now available in Ollama with the '%s-' prefix\n", namePrefix)
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

func discoverLMStudioModels(lmstudioDir, skipProvider string, verbose bool) ([]LMStudioModel, error) {
	var models []LMStudioModel

	err := filepath.Walk(lmstudioDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		// Skip the provider directory we use for forward linking
		if info.IsDir() && info.Name() == skipProvider {
			return filepath.SkipDir
		}

		// Check if it's a symlink - SKIP it per user request
		if info.Mode()&os.ModeSymlink != 0 {
			if verbose && !info.IsDir() {
				fmt.Printf("⏭️  Skipping symlink: %s\n", path)
			}
			return nil
		}

		// Only process files
		if info.IsDir() {
			return nil
		}

		// Filter for GGUF files
		if !strings.HasSuffix(strings.ToLower(info.Name()), ".gguf") {
			return nil
		}

		// Extract a nice name from path
		// Path usually is .../models/publisher/model_id/filename.gguf
		rel, _ := filepath.Rel(lmstudioDir, path)
		parts := strings.Split(rel, string(os.PathSeparator))
		
		var name string
		if len(parts) >= 2 {
			// Join publisher and model_id or filename
			name = strings.Join(parts[:len(parts)-1], "-")
			// Add quantization part if it's in the filename but not in the folder name
			filename := strings.TrimSuffix(parts[len(parts)-1], ".gguf")
			if !strings.Contains(name, filename) {
				name = name + "-" + filename
			}
		} else {
			name = strings.TrimSuffix(info.Name(), ".gguf")
		}

		// Sanitize name for Ollama (lowercase, no dots in middle usually, but dashes are fine)
		name = strings.ToLower(name)
		name = strings.ReplaceAll(name, ".", "-")
		name = strings.ReplaceAll(name, " ", "-")

		models = append(models, LMStudioModel{
			Name: name,
			Path: path,
		})
		return nil
	})

	return models, err
}

func calculateSHA256(filePath string) (string, error) {
	f, err := os.Open(filePath)
	if err != nil {
		return "", err
	}
	defer f.Close()

	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}

	return hex.EncodeToString(h.Sum(nil)), nil
}

func processLMStudioModel(model LMStudioModel, ollamaDir, namePrefix string, dryRun, verbose bool) bool {
	fmt.Printf("🔗 PROCESSING: %s\n", model.Name)
	
	if verbose {
		fmt.Printf("  📄 File: %s\n", model.Path)
	}

	// 1. Calculate SHA256
	if verbose {
		fmt.Print("  🧮 Calculating SHA256... ")
	}
	hash, err := calculateSHA256(model.Path)
	if err != nil {
		fmt.Printf("❌ ERROR: Could not calculate hash: %v\n", err)
		return false
	}
	if verbose {
		fmt.Printf("done: %s\n", hash)
	}

	blobFilename := "sha256-" + hash
	blobPath := filepath.Join(ollamaDir, "blobs", blobFilename)

	// 2. Create symlink in blobs if it doesn't exist
	if !dryRun {
		// Ensure blobs directory exists
		if err := os.MkdirAll(filepath.Join(ollamaDir, "blobs"), 0755); err != nil {
			fmt.Printf("❌ ERROR: Could not create blobs directory: %v\n", err)
			return false
		}

		if _, err := os.Lstat(blobPath); err != nil {
			if os.IsNotExist(err) {
				if err := os.Symlink(model.Path, blobPath); err != nil {
					fmt.Printf("❌ ERROR: Could not create symlink in blobs: %v\n", err)
					return false
				}
				if verbose {
					fmt.Printf("  ✅ Created blob symlink: %s\n", blobPath)
				}
			} else {
				fmt.Printf("❌ ERROR: Could not access blob path: %v\n", err)
				return false
			}
		} else {
			if verbose {
				fmt.Printf("  ⏭️  Blob already exists: %s\n", blobFilename)
			}
		}
	} else {
		fmt.Printf("  Would create blob symlink: %s -> %s\n", blobPath, model.Path)
	}

	// 3. Register with Ollama using 'ollama create'
	ollamaModelName := fmt.Sprintf("%s-%s", namePrefix, model.Name)
	if !dryRun {
		// Create a temporary Modelfile
		modelfileContent := fmt.Sprintf("FROM %s\n", model.Path)
		tmpModelfile, err := os.CreateTemp("", "Modelfile-*")
		if err != nil {
			fmt.Printf("❌ ERROR: Could not create temporary Modelfile: %v\n", err)
			return false
		}
		defer os.Remove(tmpModelfile.Name())

		if _, err := tmpModelfile.WriteString(modelfileContent); err != nil {
			fmt.Printf("❌ ERROR: Could not write to temporary Modelfile: %v\n", err)
			return false
		}
		tmpModelfile.Close()

		if verbose {
			fmt.Printf("  🚀 Registering with Ollama as '%s'...\n", ollamaModelName)
		}

		cmd := exec.Command("ollama", "create", ollamaModelName, "-f", tmpModelfile.Name())
		output, err := cmd.CombinedOutput()
		if err != nil {
			fmt.Printf("❌ ERROR: 'ollama create' failed: %v\nOutput: %s\n", err, string(output))
			return false
		}
		if verbose {
			fmt.Printf("  ✅ Registered successfully\n")
		}
	} else {
		fmt.Printf("  Would register with Ollama as: %s\n", ollamaModelName)
	}

	return true
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
