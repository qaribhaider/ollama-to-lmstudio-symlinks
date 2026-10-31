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
	"github.com/qaribhaider/ollama-to-lmstudio-symlinks/internal/ollama"
	"github.com/qaribhaider/ollama-to-lmstudio-symlinks/internal/ui"
)

// SymlinkInfo holds metadata about a discovered symbolic link
type SymlinkInfo struct {
	Name   string
	Path   string
	Target string
}

const maxOllamaModelNameLength = 80

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
	// Compare whole path elements: "..foo" is a legitimate file name, ".." is not.
	if rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) || strings.HasPrefix(rel, string(filepath.Separator)) {
		return "", fmt.Errorf("path traversal or escape attempt detected: %s", name)
	}
	return result, nil
}

// SanitizeModelName ensures the model name only contains safe characters
// for Ollama (alphanumeric, dots, and dashes).
func SanitizeModelName(name string) string {
	var sb strings.Builder
	lastWasSeparator := false
	for _, r := range strings.ToLower(name) {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			sb.WriteRune(r)
			lastWasSeparator = false
		} else if (r == '.' || r == '-') && !lastWasSeparator {
			sb.WriteRune(r)
			lastWasSeparator = true
		} else if !lastWasSeparator {
			sb.WriteRune('-')
			lastWasSeparator = true
		}
	}
	sanitized := strings.Trim(sb.String(), ".-")
	if len(sanitized) <= maxOllamaModelNameLength {
		return sanitized
	}

	hash := sha256.Sum256([]byte(sanitized))
	suffix := hex.EncodeToString(hash[:])[:8]
	prefixLength := maxOllamaModelNameLength - len(suffix) - 1
	prefix := strings.Trim(sanitized[:prefixLength], ".-")
	return prefix + "-" + suffix
}

func CalculateSHA256(filePath string) (string, error) {
	f, err := os.Open(filepath.Clean(filePath))
	if err != nil {
		return "", err
	}
	// Read-only file: a Close error cannot lose data.
	defer func() { _ = f.Close() }()

	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}

	return hex.EncodeToString(h.Sum(nil)), nil
}

func createLink(source, target string, forceHardlink bool) error {
	if forceHardlink {
		// No symlink fallback: users pick --hardlinks because symlinks do not
		// work for them (e.g. Windows), so a silent symlink would look linked
		// but not behave.
		if err := os.Link(source, target); err != nil {
			return fmt.Errorf("could not create hard link (hard links cannot cross drives; re-run without --hardlinks to use symlinks): %w", err)
		}
		return nil
	}
	return os.Symlink(source, target)
}

// shardLinkName returns the link filename for shard index i of total. Multi-shard
// models use the llama.cpp convention: model-00001-of-00003.gguf.
func shardLinkName(base string, i, total int) string {
	if total <= 1 {
		return base + ".gguf"
	}
	return fmt.Sprintf("%s-%05d-of-%05d.gguf", base, i+1, total)
}

func allShardLinksExist(modelDir, base string, total int) bool {
	for i := 0; i < max(total, 1); i++ {
		path, err := SecureJoin(modelDir, shardLinkName(base, i, total))
		if err != nil {
			return false
		}
		if _, err := os.Lstat(path); err != nil {
			return false
		}
	}
	return true
}

func ProcessModel(model models.ModelInfo, ollamaDir, ollamaProviderDir string, dryRun, verbose, useHardlinks bool) bool {
	// Sanitize name for directory usage (replace : and / with - for tags and namespaces)
	safeDirName := strings.NewReplacer(":", "-", "/", "-").Replace(model.Name)
	modelDir, err := SecureJoin(ollamaProviderDir, safeDirName)
	if err != nil {
		ui.PrintError(fmt.Sprintf("%v", err))
		return false
	}
	mainModelPath, err := SecureJoin(modelDir, shardLinkName(safeDirName, 0, len(model.MainModelBlobs)))
	if err != nil {
		ui.PrintError(fmt.Sprintf("%v", err))
		return false
	}

	// The model only counts as already linked when every shard link exists; a
	// partial set left by an interrupted run is completed below.
	if info, err := os.Lstat(mainModelPath); err == nil && allShardLinksExist(modelDir, safeDirName, len(model.MainModelBlobs)) {
		if info.Mode()&os.ModeSymlink == 0 && (!useHardlinks || !info.Mode().IsRegular()) {
			ui.PrintWarning(fmt.Sprintf("%s exists but is NOT a symlink — skipping", mainModelPath))
		} else {
			ui.PrintInfo(fmt.Sprintf("SKIPPED: %s (already exists)", model.Name))
		}
		return false
	}

	ui.PrintInfo(fmt.Sprintf("CREATING: %s", model.Name))

	if dryRun {
		return previewModelLinks(model, ollamaDir, modelDir, safeDirName)
	}
	return createModelLinks(model, ollamaDir, modelDir, safeDirName, verbose, useHardlinks)
}

// linkKind names the kind of link being created, for log messages.
func linkKind(useHardlinks bool) string {
	if useHardlinks {
		return "hard link"
	}
	return "symlink"
}

// blobSource resolves the Ollama blob file for a "sha256:hash" digest.
func blobSource(ollamaDir, digest string) (string, error) {
	// Digest format "sha256:hash" becomes the blob filename "sha256-hash".
	return SecureJoin(filepath.Join(ollamaDir, "blobs"), strings.Replace(digest, ":", "-", 1))
}

// createModelLinks links every shard and additional component of model into
// modelDir. Links created by a failed call are removed so a retry starts clean.
func createModelLinks(model models.ModelInfo, ollamaDir, modelDir, safeDirName string, verbose, useHardlinks bool) bool {
	if err := os.MkdirAll(modelDir, 0755); err != nil {
		ui.PrintError(fmt.Sprintf("Could not create directory for %s: %v", model.Name, err))
		return false
	}

	created, ok := linkMainShards(model, ollamaDir, modelDir, safeDirName, verbose, useHardlinks)
	if !ok {
		for _, p := range created {
			_ = os.Remove(p)
		}
		return false
	}

	linkAdditionalBlobs(model, ollamaDir, modelDir, verbose, useHardlinks)
	return true
}

// linkMainShards creates the model's shard links and returns the paths it created.
func linkMainShards(model models.ModelInfo, ollamaDir, modelDir, safeDirName string, verbose, useHardlinks bool) (created []string, ok bool) {
	total := len(model.MainModelBlobs)
	for i, blobDigest := range model.MainModelBlobs {
		sourcePath, err := blobSource(ollamaDir, blobDigest)
		if err != nil {
			ui.PrintError(fmt.Sprintf("unsafe blob path from digest %q: %v", blobDigest, err))
			return created, false
		}

		destFilename := shardLinkName(safeDirName, i, total)
		linkPath, err := SecureJoin(modelDir, destFilename)
		if err != nil {
			ui.PrintError(fmt.Sprintf("unsafe link path for %s: %v", destFilename, err))
			return created, false
		}

		if _, err := os.Lstat(linkPath); err == nil {
			continue // shard linked by an earlier, interrupted run
		}

		if err := createLink(sourcePath, linkPath, useHardlinks); err != nil {
			ui.PrintError(fmt.Sprintf("Could not create link for %s (part %d): %v", model.Name, i+1, err))
			return created, false
		}
		created = append(created, linkPath)

		if verbose {
			ui.PrintSuccess(fmt.Sprintf("Main model (%s part %d): %s -> %s", linkKind(useHardlinks), i+1, linkPath, sourcePath))
		}
	}
	return created, true
}

// linkAdditionalBlobs links extra components such as a vision projector. A
// failure here is a warning because the main model is already usable.
func linkAdditionalBlobs(model models.ModelInfo, ollamaDir, modelDir string, verbose, useHardlinks bool) {
	for blobDigest, filename := range model.AdditionalBlobs {
		additionalPath, err := SecureJoin(modelDir, filename)
		if err != nil {
			ui.PrintWarning(fmt.Sprintf("%v", err))
			continue
		}
		additionalSource, err := blobSource(ollamaDir, blobDigest)
		if err != nil {
			ui.PrintWarning(fmt.Sprintf("unsafe additional blob path %q: %v", blobDigest, err))
			continue
		}

		if _, err := os.Lstat(additionalPath); err == nil {
			if verbose {
				ui.PrintInfo(fmt.Sprintf("Additional component %s already exists", filename))
			}
			continue
		}

		if err := createLink(additionalSource, additionalPath, useHardlinks); err != nil {
			ui.PrintWarning(fmt.Sprintf("Could not create additional link %s: %v", filename, err))
		} else if verbose {
			ui.PrintSuccess(fmt.Sprintf("Additional (%s): %s -> %s", linkKind(useHardlinks), additionalPath, additionalSource))
		}
	}
}

// previewModelLinks prints what createModelLinks would do for a dry run.
func previewModelLinks(model models.ModelInfo, ollamaDir, modelDir, safeDirName string) bool {
	for i, blobDigest := range model.MainModelBlobs {
		sourcePath, err := blobSource(ollamaDir, blobDigest)
		if err != nil {
			ui.PrintError(fmt.Sprintf("unsafe blob path for dry run: %v", err))
			return false
		}

		linkPath, _ := SecureJoin(modelDir, shardLinkName(safeDirName, i, len(model.MainModelBlobs)))
		ui.PrintMuted(fmt.Sprintf("Would create: %s -> %s", linkPath, sourcePath))
	}

	for blobDigest, filename := range model.AdditionalBlobs {
		additionalSource, err := blobSource(ollamaDir, blobDigest)
		if err != nil {
			continue
		}
		ui.PrintMuted(fmt.Sprintf("Would create: %s -> %s", filepath.Join(modelDir, filename), additionalSource))
	}
	return true
}

func ProcessLMStudioModel(model models.LMStudioModel, ollamaDir, namePrefix, ollamaBin string, dryRun, verbose, useHardlinks bool) bool {
	if ollamaBin == "" {
		if resolved, err := ollama.GetOllamaExecutable(); err == nil && resolved != "" {
			ollamaBin = resolved
		} else {
			ollamaBin = "ollama"
		}
	}

	ui.PrintInfo(fmt.Sprintf("PROCESSING: %s", model.Name))
	if verbose {
		printLMStudioModelDetails(model)
	}

	if !linkModelBlobs(model, ollamaDir, dryRun, verbose, useHardlinks) {
		return false
	}

	ollamaModelName := SanitizeModelName(fmt.Sprintf("%s-%s", namePrefix, model.Name))
	if dryRun {
		previewRegistration(model, ollamaModelName)
		return true
	}
	return registerWithOllama(model, ollamaModelName, ollamaBin, verbose)
}

func printLMStudioModelDetails(model models.LMStudioModel) {
	ui.PrintMuted(fmt.Sprintf("File: %s", model.Path))
	for i, sp := range model.ShardPaths {
		ui.PrintMuted(fmt.Sprintf("Shard %d: %s", i+2, sp))
	}
	if model.ProjectorPath != "" {
		ui.PrintMuted(fmt.Sprintf("Projector: %s", model.ProjectorPath))
	}
}

// linkModelBlobs links the primary file, companion shards and projector into
// Ollama's blobs directory.
func linkModelBlobs(model models.LMStudioModel, ollamaDir string, dryRun, verbose, useHardlinks bool) bool {
	if _, ok := linkBlob(model.Path, ollamaDir, dryRun, verbose, useHardlinks); !ok {
		return false
	}

	for i, shardPath := range model.ShardPaths {
		if verbose {
			ui.PrintInfo(fmt.Sprintf("Linking shard blob %d: %s", i+2, filepath.Base(shardPath)))
		}
		if _, ok := linkBlob(shardPath, ollamaDir, dryRun, verbose, useHardlinks); !ok {
			return false
		}
	}

	if model.ProjectorPath != "" {
		if verbose {
			ui.PrintInfo(fmt.Sprintf("Linking vision projector: %s", filepath.Base(model.ProjectorPath)))
		}
		if _, ok := linkBlob(model.ProjectorPath, ollamaDir, dryRun, verbose, useHardlinks); !ok {
			return false
		}
	}
	return true
}

func previewRegistration(model models.LMStudioModel, ollamaModelName string) {
	switch {
	case model.ProjectorPath != "":
		ui.PrintMuted(fmt.Sprintf("Would register vision model with Ollama as: %s (projector: %s)", ollamaModelName, filepath.Base(model.ProjectorPath)))
	case len(model.ShardPaths) > 0:
		ui.PrintMuted(fmt.Sprintf("Would register sharded model (%d shards) with Ollama as: %s", len(model.ShardPaths)+1, ollamaModelName))
	default:
		ui.PrintMuted(fmt.Sprintf("Would register with Ollama as: %s", ollamaModelName))
	}
}

// registerWithOllama runs `ollama create` with a temporary Modelfile.
func registerWithOllama(model models.LMStudioModel, ollamaModelName, ollamaBin string, verbose bool) bool {
	if msg := modelfilePathProblem(model); msg != "" {
		ui.PrintError(msg)
		return false
	}

	modelfilePath, err := writeTempModelfile(model)
	if err != nil {
		ui.PrintError(err.Error())
		return false
	}
	defer func() { _ = os.Remove(modelfilePath) }()

	if verbose {
		ui.PrintInfo(fmt.Sprintf("Registering with Ollama as '%s'...", ollamaModelName))
	}

	// G204: both ollamaModelName and modelfilePath are sanitized/cleaned.
	// Go's exec.Command passes arguments directly to OS, preventing shell injection.
	output, err := exec.Command(ollamaBin, "create", ollamaModelName, "-f", modelfilePath).CombinedOutput()
	if err != nil {
		ui.PrintError(fmt.Sprintf("'ollama create' failed: %v\nOutput: %s", err, string(output)))
		return false
	}
	if verbose {
		ui.PrintSuccess("Registered successfully")
	}
	return true
}

// modelfilePathProblem returns an error message when a path contains a newline,
// which would let a crafted file name inject extra Modelfile directives.
func modelfilePathProblem(model models.LMStudioModel) string {
	if strings.ContainsAny(model.Path, "\n\r") {
		return "Invalid model path: contains newlines"
	}
	for _, sp := range model.ShardPaths {
		if strings.ContainsAny(sp, "\n\r") {
			return "Invalid shard path: contains newlines"
		}
	}
	if strings.ContainsAny(model.ProjectorPath, "\n\r") {
		return "Invalid projector path: contains newlines"
	}
	return ""
}

// writeTempModelfile writes the Modelfile for model and returns its path; the
// caller removes it.
func writeTempModelfile(model models.LMStudioModel) (string, error) {
	content := fmt.Sprintf("FROM %s\n", filepath.Clean(model.Path))
	if model.ProjectorPath != "" {
		content += fmt.Sprintf("FROM %s\n", filepath.Clean(model.ProjectorPath))
	}

	f, err := os.CreateTemp("", "Modelfile-*")
	if err != nil {
		return "", fmt.Errorf("could not create temporary Modelfile: %w", err)
	}
	path := filepath.Clean(f.Name())

	if _, err := f.WriteString(content); err != nil {
		_ = f.Close()
		_ = os.Remove(path)
		return "", fmt.Errorf("could not write to temporary Modelfile: %w", err)
	}
	if err := f.Close(); err != nil {
		_ = os.Remove(path)
		return "", fmt.Errorf("could not write to temporary Modelfile: %w", err)
	}
	return path, nil
}

// isSystemOllama reports whether ollamaDir is a system-wide install, whose
// directories usually belong to the 'ollama' service user.
func isSystemOllama(ollamaDir string) bool {
	return strings.HasPrefix(ollamaDir, "/var/lib/ollama") || strings.HasPrefix(ollamaDir, "/usr/share/ollama")
}

// systemOllamaGroupHint returns the shell commands that give the 'ollama'
// group write access to a system-wide install.
func systemOllamaGroupHint(ollamaDir string) string {
	return fmt.Sprintf("  sudo chgrp -R ollama %s\n  sudo chmod -R g+w %s", ollamaDir, ollamaDir)
}

// linkBlob links sourcePath into Ollama's blobs directory under its content hash.
func linkBlob(sourcePath, ollamaDir string, dryRun, verbose, useHardlinks bool) (string, bool) {
	if verbose {
		ui.PrintMuted(fmt.Sprintf("Calculating SHA256 for %s...", filepath.Base(sourcePath)))
	}
	hash, err := CalculateSHA256(sourcePath)
	if err != nil {
		ui.PrintError(fmt.Sprintf("Could not calculate hash for %s: %v", filepath.Base(sourcePath), err))
		return "", false
	}
	if verbose {
		ui.PrintMuted(fmt.Sprintf("done: %s", hash))
	}

	blobFilename := "sha256-" + hash
	blobPath, err := SecureJoin(filepath.Join(ollamaDir, "blobs"), blobFilename)
	if err != nil {
		ui.PrintError(fmt.Sprintf("%v", err))
		return "", false
	}

	warnIfUnreadableBySystemOllama(sourcePath, ollamaDir)

	if dryRun {
		ui.PrintMuted(fmt.Sprintf("Would create blob symlink: %s -> %s", blobPath, sourcePath))
		return blobPath, true
	}

	if err := os.MkdirAll(filepath.Join(ollamaDir, "blobs"), 0755); err != nil {
		if os.IsPermission(err) && isSystemOllama(ollamaDir) {
			ui.PrintError(fmt.Sprintf("Permission denied creating directory in %s: %v\n\n"+
				"It looks like you're using a system Ollama installation. Ensure the directory is writable by the 'ollama' group:\n\n%s",
				ollamaDir, err, systemOllamaGroupHint(ollamaDir)))
		} else {
			ui.PrintError(fmt.Sprintf("Could not create blobs directory: %v", err))
		}
		return "", false
	}

	if !removeDanglingBlobLink(blobPath, blobFilename) {
		return "", false
	}
	if !ensureBlobLink(sourcePath, blobPath, blobFilename, ollamaDir, verbose, useHardlinks) {
		return "", false
	}
	return blobPath, true
}

// warnIfUnreadableBySystemOllama notes sources the 'ollama' service user could not read.
func warnIfUnreadableBySystemOllama(sourcePath, ollamaDir string) {
	if os.PathSeparator != '/' || !isSystemOllama(ollamaDir) {
		return
	}
	if info, err := os.Stat(sourcePath); err == nil && info.Mode()&0004 == 0 { // Not world-readable
		ui.PrintWarning(fmt.Sprintf("Note: %s is not world-readable. Ensure the 'ollama' service user has permission to read it.", filepath.Base(sourcePath)))
	}
}

// removeDanglingBlobLink deletes a blob link whose source moved or was deleted,
// which would make Ollama fail to load the model. The blob name is the content
// hash, so replacing it with a link to the file we just hashed is safe.
func removeDanglingBlobLink(blobPath, blobFilename string) bool {
	info, err := os.Lstat(blobPath)
	if err != nil || info.Mode()&os.ModeSymlink == 0 {
		return true
	}
	if _, statErr := os.Stat(blobPath); !os.IsNotExist(statErr) {
		return true
	}

	if err := os.Remove(blobPath); err != nil {
		ui.PrintError(fmt.Sprintf("Could not replace broken blob link %s: %v", blobFilename, err))
		return false
	}
	ui.PrintWarning(fmt.Sprintf("Replacing broken blob link: %s", blobFilename))
	return true
}

// ensureBlobLink creates the blob link unless something is already there.
func ensureBlobLink(sourcePath, blobPath, blobFilename, ollamaDir string, verbose, useHardlinks bool) bool {
	info, err := os.Lstat(blobPath)
	if err == nil {
		if info.Mode()&os.ModeSymlink == 0 {
			ui.PrintWarning(fmt.Sprintf("%s exists but is NOT a symlink — skipping", blobFilename))
		} else if verbose {
			ui.PrintInfo(fmt.Sprintf("Blob already exists: %s", blobFilename))
		}
		return true
	}
	if !os.IsNotExist(err) {
		ui.PrintError(fmt.Sprintf("Could not access blob path: %v", err))
		return false
	}

	if err := createLink(sourcePath, blobPath, useHardlinks); err != nil {
		if os.IsPermission(err) && isSystemOllama(ollamaDir) {
			ui.PrintError(fmt.Sprintf("Permission denied creating link in blobs: %v\n\n"+
				"It looks like you're using a system Ollama installation. To allow linking without sudo, "+
				"ensure your user is in the 'ollama' group and the directory has group write permissions:\n\n%s\n\n"+
				"If you just added yourself to the group, you may need to logout and login again.", err, systemOllamaGroupHint(ollamaDir)))
		} else {
			ui.PrintError(fmt.Sprintf("Could not create link in blobs: %v", err))
		}
		return false
	}
	if verbose {
		ui.PrintSuccess(fmt.Sprintf("Created blob %s: %s", linkKind(useHardlinks), blobPath))
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

		info, infoErr := d.Info()
		if infoErr != nil || !IsManagedLink(info) {
			return nil
		}

		target := "(hard link)"
		if info.Mode()&os.ModeSymlink != 0 {
			target, err = os.Readlink(path)
			if err != nil {
				return nil // Skip if we can't read it
			}
		}

		symlinks = append(symlinks, SymlinkInfo{
			Name:   d.Name(),
			Path:   path,
			Target: target,
		})
		return nil
	})

	return symlinks, err
}

// IsManagedLink reports whether info describes something this tool created: a
// symlink, or a regular file with more than one hard link. A regular file with a
// single link is original data and must never be listed or removed.
func IsManagedLink(info os.FileInfo) bool {
	if info.Mode()&os.ModeSymlink != 0 {
		return true
	}
	if !info.Mode().IsRegular() {
		return false
	}
	isHardlink, _ := getHardlinkInfo("", info)
	return isHardlink
}

func RemoveSymlinks(paths []string, dryRun bool) (int, int) {
	var removed, failed int
	for _, path := range paths {
		info, err := os.Lstat(path)
		if err != nil {
			ui.PrintError(fmt.Sprintf("Could not stat %s: %v", path, err))
			failed++
			continue
		}
		if !IsManagedLink(info) {
			ui.PrintError(fmt.Sprintf("Refusing to remove %s: not a symlink or hard link", path))
			failed++
			continue
		}

		if dryRun {
			ui.PrintMuted(fmt.Sprintf("Would remove: %s", path))
			removed++
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
