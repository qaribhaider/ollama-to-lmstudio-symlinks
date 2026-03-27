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

func GetLMStudioCandidates() []string {
	var candidates []string

	// 1. Check LMSTUDIO_MODELS environment variable
	if env := os.Getenv("LMSTUDIO_MODELS"); env != "" {
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
	seen := make(map[string]map[string]bool)
	_ = seen // prevent unused error if I don't use it right away
	
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

		discoveredModels = append(discoveredModels, models.LMStudioModel{
			Name: name,
			Path: path,
		})
		return nil
	})

	return discoveredModels, err
}
