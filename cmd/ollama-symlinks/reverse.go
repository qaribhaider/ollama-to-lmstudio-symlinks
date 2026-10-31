package main

import (
	"errors"
	"fmt"
	"path/filepath"

	"github.com/charmbracelet/huh"
	"github.com/qaribhaider/ollama-to-lmstudio-symlinks/internal/linking"
	"github.com/qaribhaider/ollama-to-lmstudio-symlinks/internal/lmstudio"
	"github.com/qaribhaider/ollama-to-lmstudio-symlinks/internal/models"
	"github.com/qaribhaider/ollama-to-lmstudio-symlinks/internal/ollama"
	"github.com/qaribhaider/ollama-to-lmstudio-symlinks/internal/ui"
)

func runReverse(lmstudioDir, ollamaDir, namePrefix, skipProvider string, dryRun, verbose, useHardlinks, interactive, skipChecks bool) error {
	ui.PrintHeader("LM Studio → Ollama Linker")
	ui.PrintInfo(fmt.Sprintf("LM Studio directory: %s", lmstudioDir))
	ui.PrintInfo(fmt.Sprintf("Target Ollama directory: %s", ollamaDir))
	if dryRun {
		ui.PrintWarning("DRY RUN MODE - No changes will be made")
	}
	ui.PrintEmptyLine()

	// Pre-flight check: validate Ollama CLI
	ollamaBin, err := ollama.ValidateOllama(skipChecks)
	if err != nil {
		return err
	}

	// Pre-flight check: validate LM Studio
	if err := lmstudio.ValidateLMStudio(skipChecks, lmstudioDir); err != nil {
		return err
	}

	// Discover models
	discoveredModels, err := lmstudio.DiscoverLMStudioModels(lmstudioDir, skipProvider, verbose)
	if err != nil {
		return fmt.Errorf("error discovering models: %w", err)
	}

	if len(discoveredModels) == 0 {
		return errors.New("no eligible models found in LM Studio directory")
	}

	if interactive {
		var options []huh.Option[string]
		for _, m := range discoveredModels {
			label := m.Name
			if m.ProjectorPath != "" {
				label = fmt.Sprintf("%s [vision: %s]", m.Name, filepath.Base(m.ProjectorPath))
			} else if len(m.ShardPaths) > 0 {
				label = fmt.Sprintf("%s [%d shards]", m.Name, len(m.ShardPaths)+1)
			}
			options = append(options, huh.NewOption(label, m.Name).Selected(true))
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

		var filtered []models.LMStudioModel
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
		ui.PrintSubheader(fmt.Sprintf("Found %d eligible models", len(discoveredModels)))
		for _, model := range discoveredModels {
			extra := ""
			if model.ProjectorPath != "" {
				extra = fmt.Sprintf(" [vision: %s]", filepath.Base(model.ProjectorPath))
			} else if len(model.ShardPaths) > 0 {
				extra = fmt.Sprintf(" [%d shards]", len(model.ShardPaths)+1)
			}
			ui.PrintBullet(fmt.Sprintf("%s (%s)%s", model.Name, model.Path, extra))
		}
		ui.PrintEmptyLine()
	}

	// Process each model
	var created, skipped int
	for _, model := range discoveredModels {
		result := linking.ProcessLMStudioModel(model, ollamaDir, namePrefix, ollamaBin, dryRun, verbose, useHardlinks)
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
		ui.PrintSuccess(fmt.Sprintf("Models are now available in Ollama with the '%s-' prefix", namePrefix))
	}
	return nil
}
