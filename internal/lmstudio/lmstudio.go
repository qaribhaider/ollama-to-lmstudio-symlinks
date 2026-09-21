package lmstudio

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/qaribhaider/ollama-to-lmstudio-symlinks/internal/gguf"
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

// DeduceModelRole inspects a file to determine if it is a vision/multimodal projector
// or a base model. It uses GGUF header inspection as the primary authoritative source,
// and falls back to filename/text heuristics if the header cannot be read.
func DeduceModelRole(path string) (isProjector bool, arch string) {
	// 1. Authoritative check: GGUF binary header
	if info, err := gguf.InspectGGUF(path); err == nil && info != nil {
		if info.IsProjector {
			return true, info.Architecture
		}
		if info.Architecture != "" {
			// Definite base model (e.g. llama, gemma, qwen2)
			return false, info.Architecture
		}
	}

	// 2. Secondary fallback: filename text heuristics
	filename := strings.ToLower(filepath.Base(path))
	if strings.Contains(filename, "mmproj") ||
		strings.HasPrefix(filename, "projector-") ||
		strings.Contains(filename, "-projector") ||
		strings.Contains(filename, "_projector") {
		return true, ""
	}

	return false, ""
}

func matchProjector(basePath string, projectors []string) string {
	if len(projectors) == 0 {
		return ""
	}
	if len(projectors) == 1 {
		return projectors[0]
	}

	baseName := strings.ToLower(filepath.Base(basePath))
	bestMatch := projectors[0]
	bestScore := -1

	for _, p := range projectors {
		pName := strings.ToLower(filepath.Base(p))
		cleanP := strings.ReplaceAll(pName, "mmproj-", "")
		cleanP = strings.ReplaceAll(cleanP, "-mmproj", "")
		cleanP = strings.ReplaceAll(cleanP, "_mmproj", "")
		cleanP = strings.TrimSuffix(cleanP, ".gguf")

		score := 0
		tokens := strings.FieldsFunc(cleanP, func(r rune) bool {
			return r == '-' || r == '_' || r == '.'
		})
		for _, token := range tokens {
			if len(token) > 1 && strings.Contains(baseName, token) {
				score += len(token)
			}
		}

		if score > bestScore {
			bestScore = score
			bestMatch = p
		}
	}

	return bestMatch
}

func generateModelName(lmstudioDir, path string) string {
	rel, _ := filepath.Rel(lmstudioDir, path)
	parts := strings.Split(rel, string(os.PathSeparator))

	var name string
	if len(parts) >= 2 {
		name = strings.Join(parts[:len(parts)-1], "-")
		filename := strings.TrimSuffix(parts[len(parts)-1], ".gguf")
		if !strings.Contains(name, filename) {
			name = name + "-" + filename
		}
	} else {
		name = strings.TrimSuffix(filepath.Base(path), ".gguf")
	}

	name = strings.ToLower(name)
	name = strings.ReplaceAll(name, ".", "-")
	name = strings.ReplaceAll(name, " ", "-")
	return name
}

func DiscoverLMStudioModels(lmstudioDir, skipProvider string, verbose bool) ([]models.LMStudioModel, error) {
	var dirList []string
	filesByDir := make(map[string][]string)

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

		dir := filepath.Dir(path)
		if _, exists := filesByDir[dir]; !exists {
			dirList = append(dirList, dir)
		}
		filesByDir[dir] = append(filesByDir[dir], path)
		return nil
	})
	if err != nil {
		return nil, err
	}

	var discoveredModels []models.LMStudioModel

	for _, dir := range dirList {
		files := filesByDir[dir]
		var baseModels []string
		var projectors []string

		for _, file := range files {
			isProj, arch := DeduceModelRole(file)
			if isProj {
				projectors = append(projectors, file)
				if verbose {
					if arch != "" {
						fmt.Printf("🔍 Detected vision projector: %s (arch: %s)\n", filepath.Base(file), arch)
					} else {
						fmt.Printf("🔍 Detected vision projector: %s\n", filepath.Base(file))
					}
				}
			} else {
				baseModels = append(baseModels, file)
			}
		}

		if len(baseModels) == 0 {
			if len(projectors) > 0 && verbose {
				for _, proj := range projectors {
					fmt.Printf("⚠️  Skipping orphan vision projector: %s\n", proj)
				}
			}
			continue
		}

		for _, baseFile := range baseModels {
			name := generateModelName(lmstudioDir, baseFile)
			model := models.LMStudioModel{
				Name: name,
				Path: baseFile,
			}
			if len(projectors) > 0 {
				model.ProjectorPath = matchProjector(baseFile, projectors)
				if verbose && model.ProjectorPath != "" {
					fmt.Printf("🔗 Paired model '%s' with vision projector: %s\n", name, filepath.Base(model.ProjectorPath))
				}
			}
			discoveredModels = append(discoveredModels, model)
		}
	}

	return discoveredModels, nil
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
