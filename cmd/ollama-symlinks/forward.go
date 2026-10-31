package main

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/charmbracelet/huh"
	"github.com/qaribhaider/ollama-to-lmstudio-symlinks/internal/linking"
	"github.com/qaribhaider/ollama-to-lmstudio-symlinks/internal/lmstudio"
	"github.com/qaribhaider/ollama-to-lmstudio-symlinks/internal/models"
	"github.com/qaribhaider/ollama-to-lmstudio-symlinks/internal/ollama"
	"github.com/qaribhaider/ollama-to-lmstudio-symlinks/internal/ui"
)

func runForward(ollamaDir, lmstudioDir, skipProvider string, dryRun, verbose, useHardlinks, interactive, skipChecks bool) error {
	ui.PrintHeader("Ollama → LM Studio Linker")
	ui.PrintInfo(fmt.Sprintf("Ollama directory: %s", ollamaDir))
	ui.PrintInfo(fmt.Sprintf("Target LM Studio directory: %s", lmstudioDir))

	if dryRun {
		ui.PrintWarning("DRY RUN MODE - No changes will be made")
	}
	ui.PrintEmptyLine()

	// Pre-flight checks
	if _, err := ollama.ValidateOllama(skipChecks); err != nil {
		return err
	}
	if err := lmstudio.ValidateLMStudio(skipChecks, lmstudioDir); err != nil {
		return err
	}

	// Discover models
	discoveredModels, err := ollama.DiscoverModels(ollamaDir, verbose)
	if err != nil {
		return fmt.Errorf("error discovering models: %w", err)
	}

	if len(discoveredModels) == 0 {
		return errors.New("no models found in Ollama directory")
	}

	if interactive {
		var options []huh.Option[string]
		for _, m := range discoveredModels {
			options = append(options, huh.NewOption(m.Name, m.Name).Selected(true))
		}
		var selectedNames []string
		err = huh.NewMultiSelect[string]().
			Title("Select models to link").
			Description("Use Space to toggle, Enter to confirm").
			Options(options...).
			Value(&selectedNames).
			Run()

		if err != nil {
			if errors.Is(err, huh.ErrUserAborted) {
				ui.PrintInfo("Cancelled")
				return nil
			}
			return err
		}

		if len(selectedNames) == 0 {
			ui.PrintWarning("No models selected")
			return nil
		}

		var filtered []models.ModelInfo
		for _, name := range selectedNames {
			for _, m := range discoveredModels {
				if m.Name == name {
					filtered = append(filtered, m)
					break
				}
			}
		}
		discoveredModels = filtered
	} else {
		ui.PrintSubheader(fmt.Sprintf("Found %d models", len(discoveredModels)))
		for _, model := range discoveredModels {
			ui.PrintBullet(model.Name)
		}
		ui.PrintEmptyLine()
	}

	// Create ollama provider directory
	ollamaProviderDir := filepath.Join(lmstudioDir, skipProvider)
	if !dryRun {
		if err := os.MkdirAll(ollamaProviderDir, 0755); err != nil {
			return fmt.Errorf("error creating ollama provider directory: %w", err)
		}
	}

	// Process each model
	var created, skipped int
	for _, model := range discoveredModels {
		result := linking.ProcessModel(model, ollamaDir, ollamaProviderDir, dryRun, verbose, useHardlinks)
		if result {
			created++
		} else {
			skipped++
		}
	}

	// Summary
	fmt.Println(ui.FormatSummary(
		ui.HeaderStyle.Render("Results"),
		fmt.Sprintf("Linked:  %d", created),
		fmt.Sprintf("Skipped: %d", skipped),
	))

	if created > 0 && !dryRun {
		ui.PrintSuccess("Models are now available in LM Studio under the 'ollama' provider")
	}
	return nil
}
