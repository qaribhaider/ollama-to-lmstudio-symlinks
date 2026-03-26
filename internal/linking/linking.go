package linking

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/qaribhaider/ollama-to-lmstudio-symlinks/internal/models"
)

func CalculateSHA256(filePath string) (string, error) {
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

func ProcessModel(model models.ModelInfo, ollamaDir, ollamaProviderDir string, dryRun, verbose bool) bool {
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

func ProcessLMStudioModel(model models.LMStudioModel, ollamaDir, namePrefix string, dryRun, verbose bool) bool {
	fmt.Printf("🔗 PROCESSING: %s\n", model.Name)
	
	if verbose {
		fmt.Printf("  📄 File: %s\n", model.Path)
	}

	// 1. Calculate SHA256
	if verbose {
		fmt.Print("  🧮 Calculating SHA256... ")
	}
	hash, err := CalculateSHA256(model.Path)
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
