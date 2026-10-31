package main

import (
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/charmbracelet/huh"
	"github.com/qaribhaider/ollama-to-lmstudio-symlinks/internal/linking"
	"github.com/qaribhaider/ollama-to-lmstudio-symlinks/internal/lmstudio"
	"github.com/qaribhaider/ollama-to-lmstudio-symlinks/internal/models"
	"github.com/qaribhaider/ollama-to-lmstudio-symlinks/internal/ollama"
	"github.com/qaribhaider/ollama-to-lmstudio-symlinks/internal/ui"
)

func runDelete(from string, o options, stdin io.Reader) error {
	if from == "ollama" {
		return runDeleteOllama(o, stdin)
	}

	targetDir := filepath.Join(o.lmstudioDir, o.skipProvider)
	if err := lmstudio.ValidateLMStudio(o.skipChecks, targetDir); err != nil {
		return err
	}
	// Check target dir exists
	if _, err := os.Stat(targetDir); os.IsNotExist(err) {
		return fmt.Errorf("target directory for deletion does not exist: %s", targetDir)
	}

	ui.PrintSubheader("Scanning for symlinks in: " + targetDir)
	links, err := linking.ListSymlinks(targetDir)
	if err != nil {
		return fmt.Errorf("could not list symlinks: %w", err)
	}

	if len(links) == 0 {
		ui.PrintSuccess("No symbolic links found in " + targetDir)
		return nil
	}

	ui.PrintInfo(fmt.Sprintf("Found %d symbolic links:", len(links)))
	for _, link := range links {
		ui.PrintBullet(fmt.Sprintf("%s -> %s", link.Name, link.Target))
	}
	ui.PrintEmptyLine()

	var options []huh.Option[string]
	for _, link := range links {
		options = append(options, huh.NewOption[string](link.Name, link.Path))
	}

	var toDelete []string
	form := huh.NewForm(
		huh.NewGroup(
			huh.NewMultiSelect[string]().
				Title("Select symlinks to delete").
				Description("Use Space to toggle, Enter to confirm").
				Options(options...).
				Value(&toDelete),
		),
	)

	err = form.Run()
	if err != nil {
		if errors.Is(err, huh.ErrUserAborted) {
			ui.PrintInfo("Cancelled")
			return nil
		}
		return err
	}

	if len(toDelete) == 0 {
		ui.PrintWarning("No valid items selected for deletion")
		return nil
	}

	if !o.dryRun {
		confirmed, err := askConfirm(fmt.Sprintf("Are you sure you want to delete %d symlinks?", len(toDelete)))
		if err != nil {
			return err
		}
		if !confirmed {
			ui.PrintInfo("Cancelled")
			return nil
		}
	}

	removed, failed := linking.RemoveSymlinks(toDelete, o.dryRun)
	fmt.Println(ui.FormatSummary(
		ui.HeaderStyle.Render("Results"),
		fmt.Sprintf("Removed: %d", removed),
		fmt.Sprintf("Failed:  %d", failed),
	))
	return nil
}

// findOwnedModels returns the models in allModels that reverse mode registered
// into Ollama, i.e. the ones it is safe to offer for deletion.
func findOwnedModels(ollamaDir, namePrefix string, allModels []models.ModelInfo) []models.ModelInfo {
	var owned []models.ModelInfo
	for _, m := range allModels {
		if len(m.MainModelBlobs) == 0 {
			continue
		}
		// Check if the manifest uses a symlinked blob
		// The blob filename is "sha256-hash" (no colon)
		blobFilename := strings.Replace(m.MainModelBlobs[0], ":", "-", 1)
		blobPath := filepath.Join(ollamaDir, "blobs", blobFilename)

		// Both conditions are required: the name alone would match a model the
		// user pulled themselves, and a link alone would match blobs they
		// symlinked by hand.
		if !strings.HasPrefix(m.Name, namePrefix+"-") && !strings.HasPrefix(m.Name, namePrefix+":") {
			continue
		}

		info, err := os.Lstat(blobPath)
		switch {
		case err == nil && linking.IsManagedLink(info):
			owned = append(owned, m)
		case os.IsNotExist(err):
			// Ghost left behind after the source file was removed.
			owned = append(owned, m)
		}
	}
	return owned
}

// ollamaModelNamePattern matches name:tag. The first character must be
// alphanumeric so a name can never be read as a command-line flag by `ollama rm`.
var ollamaModelNamePattern = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9._/-]*:[a-zA-Z0-9._-]+$`)

func isSafeOllamaModelName(name string) bool {
	return ollamaModelNamePattern.MatchString(name) && !strings.Contains(name, "..")
}

func runDeleteOllama(o options, stdin io.Reader) error {
	// Check if Ollama manifests and blobs dirs exist
	manifestsDir := filepath.Join(o.ollamaDir, "manifests")
	if _, err := os.Stat(manifestsDir); os.IsNotExist(err) {
		return fmt.Errorf("target directory for deletion does not exist: %s (manifests not found)", o.ollamaDir)
	}

	// Pre-flight check: validate Ollama CLI
	ollamaBin, err := ollama.ValidateOllama(o.skipChecks)
	if err != nil {
		return err
	}

	ui.PrintSubheader("Scanning Ollama models for symlinks...")
	allModels, err := ollama.DiscoverModels(o.ollamaDir, o.verbose)
	if err != nil {
		return fmt.Errorf("could not discover models: %w", err)
	}

	owned := findOwnedModels(o.ollamaDir, o.namePrefix, allModels)
	if len(owned) == 0 {
		ui.PrintSuccess("No symlinked models found in Ollama")
		return nil
	}

	printOwnedModels(o.ollamaDir, owned)

	toDelete, proceed, err := chooseModelsToDelete(owned)
	if err != nil || !proceed {
		return err
	}

	if !o.dryRun {
		confirmed, err := askConfirm(fmt.Sprintf("Are you sure you want to delete %d models from Ollama?", len(toDelete)))
		if err != nil {
			return err
		}
		if !confirmed {
			ui.PrintInfo("Cancelled")
			return nil
		}
	}

	removed, failed := removeOllamaModels(ollamaBin, toDelete, o)
	printResults(
		fmt.Sprintf("Removed: %d", removed),
		fmt.Sprintf("Failed:  %d", failed),
	)
	return nil
}

// printOwnedModels lists each model with the file its first blob points to.
func printOwnedModels(ollamaDir string, owned []models.ModelInfo) {
	ui.PrintInfo(fmt.Sprintf("Found %d symlinked models in Ollama:", len(owned)))
	for _, m := range owned {
		blobFilename := strings.Replace(m.MainModelBlobs[0], ":", "-", 1)
		blobPath, err := linking.SecureJoin(filepath.Join(ollamaDir, "blobs"), blobFilename)

		target := "(unsafe blob path)"
		if err == nil {
			if readTarget, err := os.Readlink(blobPath); err == nil {
				target = readTarget
			} else {
				target = "(missing blob)"
			}
		}
		ui.PrintBullet(fmt.Sprintf("%s -> %s", m.Name, target))
	}
	ui.PrintEmptyLine()
}

// chooseModelsToDelete asks which of owned to delete. Nothing is preselected,
// unlike linking, because deletion is the riskier direction. proceed is false
// when the user cancelled or selected nothing.
func chooseModelsToDelete(owned []models.ModelInfo) (chosen []models.ModelInfo, proceed bool, err error) {
	var options []huh.Option[string]
	for _, m := range owned {
		options = append(options, huh.NewOption[string](m.Name, m.Name))
	}

	var names []string
	err = huh.NewForm(
		huh.NewGroup(
			huh.NewMultiSelect[string]().
				Title("Select models to delete").
				Description("Use Space to toggle, Enter to confirm").
				Options(options...).
				Value(&names),
		),
	).Run()
	if err != nil {
		if errors.Is(err, huh.ErrUserAborted) {
			ui.PrintInfo("Cancelled")
			return nil, false, nil
		}
		return nil, false, err
	}

	if len(names) == 0 {
		ui.PrintWarning("No valid items selected for deletion")
		return nil, false, nil
	}
	return filterByNames(owned, names, func(m models.ModelInfo) string { return m.Name }), true, nil
}

// removeOllamaModels deletes each model through `ollama rm`, which also cleans up
// the manifest and blob links.
func removeOllamaModels(ollamaBin string, toDelete []models.ModelInfo, o options) (removed, failed int) {
	for _, m := range toDelete {
		if o.dryRun {
			ui.PrintMuted(fmt.Sprintf("Would remove: %s", m.Name))
			removed++
			continue
		}

		if o.verbose {
			ui.PrintMuted(fmt.Sprintf("Deleting %s via 'ollama rm'...", m.Name))
		}

		if !isSafeOllamaModelName(m.Name) {
			ui.PrintError(fmt.Sprintf("Refusing to run 'ollama rm' with unsafe model name: %q", m.Name))
			failed++
			continue
		}

		output, err := exec.Command(ollamaBin, "rm", m.Name).CombinedOutput()
		if err != nil {
			ui.PrintError(fmt.Sprintf("'ollama rm %s' failed: %v\nOutput: %s", m.Name, err, string(output)))
			failed++
		} else {
			removed++
		}
	}
	return removed, failed
}
