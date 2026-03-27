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

// SymlinkInfo holds metadata about a discovered symbolic link
type SymlinkInfo struct {
	Name   string
	Path   string
	Target string
}

// SecureJoin joins a base directory and a user-provided name,
// preventing path traversal (Zip Slip) by ensuring the result
// is within the base directory.
func SecureJoin(base, name string) (string, error) {
	// Clean the name first
	cleanName := filepath.Clean(name)

	// Reject if it's an absolute path
	if filepath.IsAbs(cleanName) {
		return "", fmt.Errorf("absolute path not allowed: %s", name)
	}

	// Reject if it starts with a separator (root-relative on Windows)
	if len(cleanName) > 0 && os.IsPathSeparator(cleanName[0]) {
		return "", fmt.Errorf("absolute or root-relative path not allowed: %s", name)
	}

	result := filepath.Join(base, cleanName)
	rel, err := filepath.Rel(base, result)
	if err != nil {
		return "", err
	}
	// Check for traversal or unexpected root reference
	if strings.HasPrefix(rel, "..") || strings.HasPrefix(rel, string(filepath.Separator)) {
		return "", fmt.Errorf("path traversal or escape attempt detected: %s", name)
	}
	return result, nil
}

// SanitizeModelName ensures the model name only contains safe characters
// for Ollama (alphanumeric, dots, dashes, underscores).
func SanitizeModelName(name string) string {
	var sb strings.Builder
	for _, r := range strings.ToLower(name) {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '.' || r == '-' || r == '_' {
			sb.WriteRune(r)
		} else {
			sb.WriteRune('-') // Replace invalid chars with dash
		}
	}
	return sb.String()
}

func CalculateSHA256(filePath string) (string, error) {
	f, err := os.Open(filepath.Clean(filePath))
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
	modelDir, err := SecureJoin(ollamaProviderDir, model.Name)
	if err != nil {
		fmt.Printf("❌ ERROR: %v\n", err)
		return false
	}
	mainModelPath, err := SecureJoin(modelDir, model.Name+".gguf")
	if err != nil {
		fmt.Printf("❌ ERROR: %v\n", err)
		return false
	}

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
			additionalPath, err := SecureJoin(modelDir, filename)
			if err != nil {
				fmt.Printf("⚠️  Warning: %v\n", err)
				continue
			}
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
	blobPath, err := SecureJoin(filepath.Join(ollamaDir, "blobs"), blobFilename)
	if err != nil {
		fmt.Printf("❌ ERROR: %v\n", err)
		return false
	}

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
		// Ensure model path doesn't contain newlines to prevent Modelfile injection
		if strings.ContainsAny(model.Path, "\n\r") {
			fmt.Printf("❌ ERROR: Invalid model path: contains newlines\n")
			return false
		}

		// Create a temporary Modelfile
		modelfileContent := fmt.Sprintf("FROM %s\n", filepath.Clean(model.Path))
		tmpModelfile, err := os.CreateTemp("", "Modelfile-*")
		if err != nil {
			fmt.Printf("❌ ERROR: Could not create temporary Modelfile: %v\n", err)
			return false
		}
		// Capture name immediately and clean it
		tmpPath := filepath.Clean(tmpModelfile.Name())
		defer os.Remove(tmpPath)

		if _, err := tmpModelfile.WriteString(modelfileContent); err != nil {
			fmt.Printf("❌ ERROR: Could not write to temporary Modelfile: %v\n", err)
			return false
		}
		tmpModelfile.Close()

		if verbose {
			fmt.Printf("  🚀 Registering with Ollama as '%s'...\n", ollamaModelName)
		}

		// Ensure the model name is sanitized for safety
		safeModelName := SanitizeModelName(ollamaModelName)

		// G204: both safeModelName and tmpPath are sanitized/cleaned.
		// Go's exec.Command passes arguments directly to OS, preventing shell injection.
		cmd := exec.Command("ollama", "create", safeModelName, "-f", tmpPath)
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

func ListSymlinks(dir string) ([]SymlinkInfo, error) {
	var symlinks []SymlinkInfo

	// Check if directory exists
	if _, err := os.Stat(dir); os.IsNotExist(err) {
		return nil, nil // Return empty if dir doesn't exist
	}

	err := filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		// Only look for symbolic links
		if info.Mode()&os.ModeSymlink != 0 {
			target, err := os.Readlink(path)
			if err != nil {
				return nil // Skip if we can't read it
			}

			symlinks = append(symlinks, SymlinkInfo{
				Name:   info.Name(),
				Path:   path,
				Target: target,
			})
		}
		return nil
	})

	return symlinks, err
}

func RemoveSymlinks(paths []string, dryRun bool) (int, int) {
	var removed, failed int
	for _, path := range paths {
		if dryRun {
			fmt.Printf("  Would remove: %s\n", path)
			removed++
			continue
		}

		if err := os.Remove(path); err != nil {
			fmt.Printf("❌ ERROR: Could not remove %s: %v\n", path, err)
			failed++
		} else {
			removed++
		}
	}
	return removed, failed
}
