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
	"github.com/qaribhaider/ollama-to-lmstudio-symlinks/internal/ui"
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

func createLink(source, target string, forceHardlink bool) error {
	if forceHardlink {
		if err := os.Link(source, target); err == nil {
			return nil
		}
		// Fallback to symlink if hard link fails (e.g. cross-device)
		return os.Symlink(source, target)
	}
	return os.Symlink(source, target)
}

func ProcessModel(model models.ModelInfo, ollamaDir, ollamaProviderDir string, dryRun, verbose, useHardlinks bool) bool {
	// Sanitize name for directory usage (replace : with - for tags)
	safeDirName := strings.Replace(model.Name, ":", "-", -1)
	modelDir, err := SecureJoin(ollamaProviderDir, safeDirName)
	if err != nil {
		ui.PrintError(fmt.Sprintf("%v", err))
		return false
	}
	mainModelPath, err := SecureJoin(modelDir, safeDirName+".gguf")
	if err != nil {
		ui.PrintError(fmt.Sprintf("%v", err))
		return false
	}

	// Check if main model symlink already exists
	if info, err := os.Lstat(mainModelPath); err == nil {
		if info.Mode()&os.ModeSymlink == 0 {
			ui.PrintWarning(fmt.Sprintf("%s exists but is NOT a symlink — skipping", mainModelPath))
		} else {
			ui.PrintInfo(fmt.Sprintf("SKIPPED: %s (already exists)", model.Name))
		}
		return false
	}

	ui.PrintInfo(fmt.Sprintf("CREATING: %s", model.Name))

	if !dryRun {
		// Create model directory
		if err := os.MkdirAll(modelDir, 0755); err != nil {
			ui.PrintError(fmt.Sprintf("Could not create directory for %s: %v", model.Name, err))
			return false
		}

		// Create main model link(s)
		for i, blobDigest := range model.MainModelBlobs {
			// Convert digest format from "sha256:hash" to "sha256-hash" for blob filename
			blobFilename := strings.Replace(blobDigest, ":", "-", 1)
			sourcePath, err := SecureJoin(filepath.Join(ollamaDir, "blobs"), blobFilename)
			if err != nil {
				ui.PrintError(fmt.Sprintf("unsafe blob path from digest %q: %v", blobDigest, err))
				return false
			}

			destFilename := safeDirName + ".gguf"
			if len(model.MainModelBlobs) > 1 {
				// Use standard llama.cpp sharding convention: model-00001-of-00003.gguf
				destFilename = fmt.Sprintf("%s-%05d-of-%05d.gguf", safeDirName, i+1, len(model.MainModelBlobs))
			}
			
			linkPath, err := SecureJoin(modelDir, destFilename)
			if err != nil {
				ui.PrintError(fmt.Sprintf("unsafe link path for %s: %v", destFilename, err))
				return false
			}

			if err := createLink(sourcePath, linkPath, useHardlinks); err != nil {
				ui.PrintError(fmt.Sprintf("Could not create link for %s (part %d): %v", model.Name, i+1, err))
				return false
			}

			if verbose {
				linkType := "symlink"
				if useHardlinks {
					linkType = "hard link"
				}
				ui.PrintSuccess(fmt.Sprintf("Main model (%s part %d): %s -> %s", linkType, i+1, linkPath, sourcePath))
			}
		}

		// Create additional component symlinks (e.g., projector for llava)
		for blobHash, filename := range model.AdditionalBlobs {
			additionalPath, err := SecureJoin(modelDir, filename)
			if err != nil {
				ui.PrintWarning(fmt.Sprintf("%v", err))
				continue
			}
			// Convert digest format from "sha256:hash" to "sha256-hash" for blob filename
			blobFilename := strings.Replace(blobHash, ":", "-", 1)
			additionalSource, err := SecureJoin(filepath.Join(ollamaDir, "blobs"), blobFilename)
			if err != nil {
				ui.PrintWarning(fmt.Sprintf("unsafe additional blob path %q: %v", blobHash, err))
				continue
			}

			// Skip if already exists
			if _, err := os.Lstat(additionalPath); err == nil {
				if verbose {
					ui.PrintInfo(fmt.Sprintf("Additional component %s already exists", filename))
				}
				continue
			}

			if err := createLink(additionalSource, additionalPath, useHardlinks); err != nil {
				ui.PrintWarning(fmt.Sprintf("Could not create additional link %s: %v", filename, err))
			} else if verbose {
				linkType := "symlink"
				if useHardlinks {
					linkType = "hard link"
				}
				ui.PrintSuccess(fmt.Sprintf("Additional (%s): %s -> %s", linkType, additionalPath, additionalSource))
			}
		}
	} else {
		// Dry run - just show what would be done
		for i, blobDigest := range model.MainModelBlobs {
			blobFilename := strings.Replace(blobDigest, ":", "-", 1)
			sourcePath, err := SecureJoin(filepath.Join(ollamaDir, "blobs"), blobFilename)
			if err != nil {
				ui.PrintError(fmt.Sprintf("unsafe blob path for dry run: %v", err))
				return false
			}
			
			destFilename := safeDirName + ".gguf"
			if len(model.MainModelBlobs) > 1 {
				destFilename = fmt.Sprintf("%s-%05d-of-%05d.gguf", safeDirName, i+1, len(model.MainModelBlobs))
			}
			linkPath, _ := SecureJoin(modelDir, destFilename)
			ui.PrintMuted(fmt.Sprintf("Would create: %s -> %s", linkPath, sourcePath))
		}

		for blobHash, filename := range model.AdditionalBlobs {
			additionalPath := filepath.Join(modelDir, filename)
			blobFilename := strings.Replace(blobHash, ":", "-", 1)
			additionalSource, err := SecureJoin(filepath.Join(ollamaDir, "blobs"), blobFilename)
			if err != nil {
				continue
			}
			ui.PrintMuted(fmt.Sprintf("Would create: %s -> %s", additionalPath, additionalSource))
		}
	}

	return true
}

func ProcessLMStudioModel(model models.LMStudioModel, ollamaDir, namePrefix string, dryRun, verbose, useHardlinks bool) bool {
	ui.PrintInfo(fmt.Sprintf("PROCESSING: %s", model.Name))
	
	if verbose {
		ui.PrintMuted(fmt.Sprintf("File: %s", model.Path))
	}

	// 1. Calculate SHA256
	if verbose {
		ui.PrintMuted("Calculating SHA256...")
	}
	hash, err := CalculateSHA256(model.Path)
	if err != nil {
		ui.PrintError(fmt.Sprintf("Could not calculate hash: %v", err))
		return false
	}
	if verbose {
		ui.PrintMuted(fmt.Sprintf("done: %s", hash))
	}

	blobFilename := "sha256-" + hash
	blobPath, err := SecureJoin(filepath.Join(ollamaDir, "blobs"), blobFilename)
	if err != nil {
		ui.PrintError(fmt.Sprintf("%v", err))
		return false
	}

	// 2. Warn if source file might be inaccessible to system Ollama
	if os.PathSeparator == '/' && strings.HasPrefix(ollamaDir, "/var/lib/ollama") {
		// Naive check: if parent or file isn't at least group-executable/readable
		// Just a friendly heads-up to the user
		if info, err := os.Stat(model.Path); err == nil {
			if info.Mode()&0004 == 0 { // Not world-readable
				ui.PrintWarning(fmt.Sprintf("Note: %s is not world-readable. Ensure the 'ollama' service user has permission to read it.", filepath.Base(model.Path)))
			}
		}
	}

	// 2. Create symlink in blobs if it doesn't exist
	if !dryRun {
		// Ensure blobs directory exists
		if err := os.MkdirAll(filepath.Join(ollamaDir, "blobs"), 0755); err != nil {
			if os.IsPermission(err) && strings.HasPrefix(ollamaDir, "/var/lib/ollama") {
				ui.PrintError(fmt.Sprintf("Permission denied creating directory in %s: %v\n\n"+
					"It looks like you're using a system Ollama installation. Ensure the directory is writable by the 'ollama' group:\n\n"+
					"  sudo chgrp -R ollama %s\n"+
					"  sudo chmod -R g+w %s", ollamaDir, err, ollamaDir, ollamaDir))
			} else {
				ui.PrintError(fmt.Sprintf("Could not create blobs directory: %v", err))
			}
			return false
		}

		if info, err := os.Lstat(blobPath); err != nil {
			if os.IsNotExist(err) {
				if err := createLink(model.Path, blobPath, useHardlinks); err != nil {
					if os.IsPermission(err) && strings.HasPrefix(ollamaDir, "/var/lib/ollama") {
						ui.PrintError(fmt.Sprintf("Permission denied creating link in blobs: %v\n\n"+
							"It looks like you're using a system Ollama installation. To allow linking without sudo, "+
							"ensure your user is in the 'ollama' group and the directory has group write permissions:\n\n"+
							"  sudo chgrp -R ollama %s\n"+
							"  sudo chmod -R g+w %s\n\n"+
							"If you just added yourself to the group, you may need to logout and login again.", err, ollamaDir, ollamaDir))
					} else {
						ui.PrintError(fmt.Sprintf("Could not create link in blobs: %v", err))
					}
					return false
				}
				if verbose {
					linkType := "symlink"
					if useHardlinks {
						linkType = "hard link"
					}
					ui.PrintSuccess(fmt.Sprintf("Created blob %s: %s", linkType, blobPath))
				}
			} else {
				ui.PrintError(fmt.Sprintf("Could not access blob path: %v", err))
				return false
			}
		} else {
			if info.Mode()&os.ModeSymlink == 0 {
				ui.PrintWarning(fmt.Sprintf("%s exists but is NOT a symlink — skipping", blobFilename))
			} else if verbose {
				ui.PrintInfo(fmt.Sprintf("Blob already exists: %s", blobFilename))
			}
		}
	} else {
		ui.PrintMuted(fmt.Sprintf("Would create blob symlink: %s -> %s", blobPath, model.Path))
	}

	// 3. Register with Ollama using 'ollama create'
	ollamaModelName := fmt.Sprintf("%s-%s", namePrefix, model.Name)
		if !dryRun {
		// Ensure model path doesn't contain newlines to prevent Modelfile injection
		if strings.ContainsAny(model.Path, "\n\r") {
			ui.PrintError("Invalid model path: contains newlines")
			return false
		}

		// Create a temporary Modelfile
		modelfileContent := fmt.Sprintf("FROM %s\n", filepath.Clean(model.Path))
		tmpModelfile, err := os.CreateTemp("", "Modelfile-*")
		if err != nil {
			ui.PrintError(fmt.Sprintf("Could not create temporary Modelfile: %v", err))
			return false
		}
		// Capture name immediately and clean it
		tmpPath := filepath.Clean(tmpModelfile.Name())
		defer os.Remove(tmpPath)

		if _, err := tmpModelfile.WriteString(modelfileContent); err != nil {
			ui.PrintError(fmt.Sprintf("Could not write to temporary Modelfile: %v", err))
			return false
		}
		tmpModelfile.Close()

		if verbose {
			ui.PrintInfo(fmt.Sprintf("Registering with Ollama as '%s'...", ollamaModelName))
		}

		// Ensure the model name is sanitized for safety
		safeModelName := SanitizeModelName(ollamaModelName)

		// G204: both safeModelName and tmpPath are sanitized/cleaned.
		// Go's exec.Command passes arguments directly to OS, preventing shell injection.
		cmd := exec.Command("ollama", "create", safeModelName, "-f", tmpPath)
		output, err := cmd.CombinedOutput()
		if err != nil {
			ui.PrintError(fmt.Sprintf("'ollama create' failed: %v\nOutput: %s", err, string(output)))
			return false
		}
		if verbose {
			ui.PrintSuccess("Registered successfully")
		}
	} else {
		ui.PrintMuted(fmt.Sprintf("Would register with Ollama as: %s", ollamaModelName))
	}

	return true
}

func ListSymlinks(dir string) ([]SymlinkInfo, error) {
	var symlinks []SymlinkInfo

	// Check if directory exists
	if _, err := os.Stat(dir); os.IsNotExist(err) {
		return nil, nil // Return empty if dir doesn't exist
	}

	err := filepath.WalkDir(dir, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}

		// Look for symbolic links or regular files (could be hard links)
		if d.Type()&os.ModeSymlink != 0 || d.Type().IsRegular() {
			target := ""
			if d.Type()&os.ModeSymlink != 0 {
				target, err = os.Readlink(path)
				if err != nil {
					return nil // Skip if we can't read it
				}
			} else {
				target = "(file or hard link)"
			}

			symlinks = append(symlinks, SymlinkInfo{
				Name:   d.Name(),
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
			ui.PrintMuted(fmt.Sprintf("Would remove: %s", path))
			removed++
			continue
		}

		info, err := os.Lstat(path)
		if err != nil {
			ui.PrintError(fmt.Sprintf("Could not stat %s: %v", path, err))
			failed++
			continue
		}
		// Allow removing symlinks or regular files
		if info.Mode()&os.ModeSymlink == 0 && !info.Mode().IsRegular() {
			ui.PrintError(fmt.Sprintf("Refusing to remove non-link/non-file: %s", path))
			failed++
			continue
		}

		if err := os.Remove(path); err != nil {
			ui.PrintError(fmt.Sprintf("Could not remove %s: %v", path, err))
			failed++
		} else {
			removed++
		}
	}
	return removed, failed
}

// FindBrokenSymlinks recursively finds all symbolic links in dir
// whose targets do not exist.
func FindBrokenSymlinks(dir string) ([]SymlinkInfo, error) {
	var broken []SymlinkInfo

	if _, err := os.Stat(dir); os.IsNotExist(err) {
		return nil, nil
	}

	err := filepath.WalkDir(dir, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}

		// Check if it's a symlink
		if d.Type()&os.ModeSymlink != 0 {
			target, err := os.Readlink(path)
			if err != nil {
				return nil // Skip if unreadable
			}

			// Check if target exists
			// We use os.Stat(path) - if it returns IsNotExist, then the target of the symlink is missing.
			_, err = os.Stat(path)
			if os.IsNotExist(err) {
				broken = append(broken, SymlinkInfo{
					Name:   d.Name(),
					Path:   path,
					Target: target,
				})
			}
		}
		return nil
	})

	return broken, err
}
