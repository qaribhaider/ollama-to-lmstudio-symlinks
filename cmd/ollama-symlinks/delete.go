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

	symlinkedModels := findOwnedModels(o.ollamaDir, o.namePrefix, allModels)

	if len(symlinkedModels) == 0 {
		ui.PrintSuccess("No symlinked models found in Ollama")
		return nil
	}

	ui.PrintInfo(fmt.Sprintf("Found %d symlinked models in Ollama:", len(symlinkedModels)))
	for _, m := range symlinkedModels {
		if len(m.MainModelBlobs) == 0 {
			continue
		}
		blobFilename := strings.Replace(m.MainModelBlobs[0], ":", "-", 1)
		targetPath, err := linking.SecureJoin(filepath.Join(o.ollamaDir, "blobs"), blobFilename)

		target := "(unsafe blob path)"
		if err == nil {
			if readTarget, err := os.Readlink(targetPath); err == nil {
				target = readTarget
			} else {
				target = "(missing blob)"
			}
		}
		ui.PrintBullet(fmt.Sprintf("%s -> %s", m.Name, target))
	}
	ui.PrintEmptyLine()

	var options []huh.Option[string]
	for _, m := range symlinkedModels {
		options = append(options, huh.NewOption[string](m.Name, m.Name))
	}

	var toDeleteNames []string
	form := huh.NewForm(
		huh.NewGroup(
			huh.NewMultiSelect[string]().
				Title("Select models to delete").
				Description("Use Space to toggle, Enter to confirm").
				Options(options...).
				Value(&toDeleteNames),
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

	if len(toDeleteNames) == 0 {
		ui.PrintWarning("No valid items selected for deletion")
		return nil
	}

	// Map names back to ModelInfo
	var toDelete []models.ModelInfo
	for _, name := range toDeleteNames {
		for _, m := range symlinkedModels {
			if m.Name == name {
				toDelete = append(toDelete, m)
				break
			}
		}
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

	var removed, failed int
	for _, m := range toDelete {
		if o.dryRun {
			ui.PrintMuted(fmt.Sprintf("Would remove: %s", m.Name))
			removed++
			continue
		}

		if o.verbose {
			ui.PrintMuted(fmt.Sprintf("Deleting %s via 'ollama rm'...", m.Name))
		}

		// Use 'ollama rm' to properly clean up manifest and blob links
		importPath := m.Name
		// Validate model name to prevent accidental flag interpretation
		if matched, _ := regexp.MatchString(`^[a-zA-Z0-9._/-]+:[a-zA-Z0-9._-]+$`, importPath); !matched || strings.Contains(importPath, "..") {
			ui.PrintError(fmt.Sprintf("Refusing to run 'ollama rm' with unsafe model name: %q", importPath))
			failed++
			continue
		}

		cmd := exec.Command(ollamaBin, "rm", importPath)
		output, err := cmd.CombinedOutput()
		if err != nil {
			ui.PrintError(fmt.Sprintf("'ollama rm %s' failed: %v\nOutput: %s", importPath, err, string(output)))
			failed++
		} else {
			removed++
		}
	}

	fmt.Println(ui.FormatSummary(
		ui.HeaderStyle.Render("Results"),
		fmt.Sprintf("Removed: %d", removed),
		fmt.Sprintf("Failed:  %d", failed),
	))
	return nil
}
