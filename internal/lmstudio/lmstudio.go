package lmstudio

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/qaribhaider/ollama-to-lmstudio-symlinks/internal/models"
)

func GetDefaultLMStudioDir() string {
	candidates := GetLMStudioCandidates()
	if len(candidates) > 0 {
		return candidates[0]
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".cache", "lm-studio", "models")
}

func isSafePath(p string) bool {
	clean := filepath.Clean(p)
	if clean == "." || clean == "/" || clean == `\` || len(clean) == 2 && clean[1] == ':' {
		return false
	}
	if len(clean) == 3 && clean[1] == ':' && clean[2] == '\\' {
		return false
	}
	return true
}

func GetLMStudioCandidates() []string {
	var candidates []string

	// 1. Check LMSTUDIO_MODELS environment variable
	if env := os.Getenv("LMSTUDIO_MODELS"); env != "" && isSafePath(env) {
		candidates = append(candidates, filepath.Clean(env))
	}

	// 2. Default location
	home, err := os.UserHomeDir()
	if err == nil {
		candidates = append(candidates, filepath.Join(home, ".cache", "lm-studio", "models"))
		candidates = append(candidates, filepath.Join(home, ".lmstudio", "models"))
	}

	// 3. Windows-specific locations
	if os.Getenv("OS") == "Windows_NT" || os.PathSeparator == '\\' {
		if local := os.Getenv("LOCALAPPDATA"); local != "" {
			candidates = append(candidates, filepath.Join(local, "LMStudio", "models"))
			candidates = append(candidates, filepath.Join(local, "lm-studio", "models"))
		}
		if appdata := os.Getenv("APPDATA"); appdata != "" {
			candidates = append(candidates, filepath.Join(appdata, "LM Studio", "models"))
		}
		if home != "" {
			candidates = append(candidates, filepath.Join(home, "AppData", "Local", "LM Studio", "models"))
			candidates = append(candidates, filepath.Join(home, "AppData", "Roaming", "LM Studio", "models"))
		}
	}

	// Filter to only include directories that actually exist
	var existing []string
	
	unique := make(map[string]bool)
	for _, c := range candidates {
		c = filepath.Clean(c)
		if unique[c] {
			continue
		}
		unique[c] = true
		if info, err := os.Stat(c); err == nil && info.IsDir() {
			existing = append(existing, c)
		}
	}

	return existing
}

func DiscoverLMStudioModels(lmstudioDir, skipProvider string, verbose bool) ([]models.LMStudioModel, error) {
	var discoveredModels []models.LMStudioModel

	err := filepath.Walk(lmstudioDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		// Skip the provider directory we use for forward linking
		if info.IsDir() && info.Name() == skipProvider {
			return filepath.SkipDir
		}

		// Check if it's a symlink FIRST to avoid IsDir early returns on symlinked folders
		if info.Mode()&os.ModeSymlink != 0 {
			if verbose {
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

		discoveredModels = append(discoveredModels, models.LMStudioModel{
			Name: name,
			Path: path,
		})
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
		filepath.Join(".cache", "lm-studio", "models"),
		filepath.Join(".lmstudio", "models"),
		filepath.Join("AppData", "Local", "LMStudio", "models"),
		filepath.Join("AppData", "Local", "lm-studio", "models"),
		filepath.Join("AppData", "Roaming", "LM Studio", "models"),
	}

	for _, driveLetter := range "ABCDEFGHIJKLMNOPQRSTUVWXYZ" {
		drive := string(driveLetter) + ":\\"
		// Use os.Stat to check if drive root is accessible
		if _, err := os.Stat(drive); err != nil {
			continue
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
