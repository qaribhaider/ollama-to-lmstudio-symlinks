package lmstudio

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
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

var shardRegex = regexp.MustCompile(`(?i)^(.*?)[-_](\d+)-of-(\d+)\.gguf$`)

type shardInfo struct {
	path  string
	part  int
	total int
}

// isCompleteShardSet reports whether shards (sorted by part) are exactly parts
// 1..N of a single N-part model. Linking a partial set would register a model
// Ollama cannot load.
func isCompleteShardSet(shards []shardInfo) bool {
	total := shards[0].total
	if len(shards) != total {
		return false
	}
	for i, s := range shards {
		if s.total != total || s.part != i+1 {
			return false
		}
	}
	return true
}

func generateModelName(lmstudioDir, path string) string {
	rel, _ := filepath.Rel(lmstudioDir, path)
	parts := strings.Split(rel, string(os.PathSeparator))

	var name string
	if len(parts) >= 2 {
		name = strings.Join(parts[:len(parts)-1], "-")
		filename := strings.TrimSuffix(parts[len(parts)-1], ".gguf")
		filename = strings.TrimSuffix(filename, ".GGUF")
		if m := shardRegex.FindStringSubmatch(parts[len(parts)-1]); len(m) == 4 {
			filename = m[1]
		}
		if !strings.Contains(name, filename) {
			name = name + "-" + filename
		}
	} else {
		filename := strings.TrimSuffix(filepath.Base(path), ".gguf")
		filename = strings.TrimSuffix(filename, ".GGUF")
		if m := shardRegex.FindStringSubmatch(filepath.Base(path)); len(m) == 4 {
			filename = m[1]
		}
		name = filename
	}

	name = strings.ToLower(name)
	name = strings.ReplaceAll(name, ".", "-")
	name = strings.ReplaceAll(name, " ", "-")
	return name
}

// DiscoverLMStudioModels finds linkable models under lmstudioDir, pairing vision
// projectors with their models and grouping shard sets. skipProvider is the
// directory used for forward links and is never scanned.
func DiscoverLMStudioModels(lmstudioDir, skipProvider string, verbose bool) ([]models.LMStudioModel, error) {
	dirList, filesByDir, err := collectGGUFFiles(lmstudioDir, skipProvider, verbose)
	if err != nil {
		return nil, err
	}

	var discovered []models.LMStudioModel
	for _, dir := range dirList {
		discovered = append(discovered, modelsInDir(lmstudioDir, filesByDir[dir], verbose)...)
	}
	return discovered, nil
}

// collectGGUFFiles walks lmstudioDir and groups real (non-symlink) .gguf files by
// directory, returning the directories in walk order.
func collectGGUFFiles(lmstudioDir, skipProvider string, verbose bool) (dirList []string, filesByDir map[string][]string, err error) {
	filesByDir = make(map[string][]string)

	err = filepath.Walk(lmstudioDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			if path == lmstudioDir {
				return err
			}
			// An unreadable subdirectory must not hide the rest of the library.
			fmt.Printf("⚠️  Skipping %s: %v\n", path, err)
			return nil
		}

		// Skip the provider directory we use for forward linking. Only the one
		// directly under the root: a model folder elsewhere may share its name,
		// and some filesystems (macOS) are case-insensitive.
		if info.IsDir() && filepath.Dir(path) == filepath.Clean(lmstudioDir) && strings.EqualFold(info.Name(), skipProvider) {
			return filepath.SkipDir
		}

		// Check if it's a symlink FIRST to avoid IsDir early returns on symlinked folders
		if info.Mode()&os.ModeSymlink != 0 {
			if verbose {
				fmt.Printf("⏭️  Skipping symlink: %s\n", path)
			}
			return nil
		}

		if info.IsDir() || !strings.HasSuffix(strings.ToLower(info.Name()), ".gguf") {
			return nil
		}

		dir := filepath.Dir(path)
		if _, exists := filesByDir[dir]; !exists {
			dirList = append(dirList, dir)
		}
		filesByDir[dir] = append(filesByDir[dir], path)
		return nil
	})
	return dirList, filesByDir, err
}

// modelsInDir turns the .gguf files of one directory into models.
func modelsInDir(lmstudioDir string, files []string, verbose bool) []models.LMStudioModel {
	baseModels, projectors := splitProjectors(files, verbose)
	if len(baseModels) == 0 {
		if verbose {
			for _, proj := range projectors {
				fmt.Printf("⚠️  Skipping orphan vision projector: %s\n", proj)
			}
		}
		return nil
	}

	standalone, shardsByPrefix := splitShards(baseModels)

	var found []models.LMStudioModel
	for _, baseFile := range standalone {
		model := models.LMStudioModel{Name: generateModelName(lmstudioDir, baseFile), Path: baseFile}
		pairProjector(&model, projectors, "model", verbose)
		found = append(found, model)
	}
	return append(found, shardedModels(lmstudioDir, shardsByPrefix, projectors, verbose)...)
}

// splitProjectors separates vision projectors from base models.
func splitProjectors(files []string, verbose bool) (baseModels, projectors []string) {
	for _, file := range files {
		isProj, arch := DeduceModelRole(file)
		if !isProj {
			baseModels = append(baseModels, file)
			continue
		}

		projectors = append(projectors, file)
		if !verbose {
			continue
		}
		if arch != "" {
			fmt.Printf("🔍 Detected vision projector: %s (arch: %s)\n", filepath.Base(file), arch)
		} else {
			fmt.Printf("🔍 Detected vision projector: %s\n", filepath.Base(file))
		}
	}
	return baseModels, projectors
}

// splitShards separates single-file models from numbered shards, which are
// grouped by lowercase name prefix.
func splitShards(baseModels []string) (standalone []string, shardsByPrefix map[string][]shardInfo) {
	shardsByPrefix = make(map[string][]shardInfo)
	for _, baseFile := range baseModels {
		m := shardRegex.FindStringSubmatch(filepath.Base(baseFile))
		if len(m) == 4 {
			part, err1 := strconv.Atoi(m[2])
			total, err2 := strconv.Atoi(m[3])
			if err1 == nil && err2 == nil && total >= 1 && part >= 1 && part <= total {
				prefix := strings.ToLower(m[1])
				shardsByPrefix[prefix] = append(shardsByPrefix[prefix], shardInfo{path: baseFile, part: part, total: total})
				continue
			}
		}
		standalone = append(standalone, baseFile)
	}
	return standalone, shardsByPrefix
}

// shardedModels builds one model per complete shard set, in prefix order.
func shardedModels(lmstudioDir string, shardsByPrefix map[string][]shardInfo, projectors []string, verbose bool) []models.LMStudioModel {
	prefixes := make([]string, 0, len(shardsByPrefix))
	for prefix := range shardsByPrefix {
		prefixes = append(prefixes, prefix)
	}
	sort.Strings(prefixes)

	var found []models.LMStudioModel
	for _, prefix := range prefixes {
		shards := shardsByPrefix[prefix]
		sort.Slice(shards, func(i, j int) bool { return shards[i].part < shards[j].part })

		if !isCompleteShardSet(shards) {
			if verbose {
				fmt.Printf("⚠️  Skipping incomplete sharded model group %q: found %d shards that do not form a complete set\n", prefix, len(shards))
			}
			continue
		}

		primaryFile := shards[0].path
		var companionShards []string
		for _, s := range shards[1:] {
			companionShards = append(companionShards, s.path)
		}

		model := models.LMStudioModel{
			Name:       generateModelName(lmstudioDir, primaryFile),
			Path:       primaryFile,
			ShardPaths: companionShards,
		}
		pairProjector(&model, projectors, "sharded model", verbose)
		if verbose {
			fmt.Printf("📦 Grouped %d shards for model '%s' (primary: %s)\n", len(shards), model.Name, filepath.Base(primaryFile))
		}
		found = append(found, model)
	}
	return found
}

// pairProjector attaches the best-matching vision projector to model, if any.
func pairProjector(model *models.LMStudioModel, projectors []string, kind string, verbose bool) {
	if len(projectors) == 0 {
		return
	}
	model.ProjectorPath = matchProjector(model.Path, projectors)
	if verbose && model.ProjectorPath != "" {
		fmt.Printf("🔗 Paired %s '%s' with vision projector: %s\n", kind, model.Name, filepath.Base(model.ProjectorPath))
	}
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
