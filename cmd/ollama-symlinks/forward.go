package main

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/qaribhaider/ollama-to-lmstudio-symlinks/internal/linking"
	"github.com/qaribhaider/ollama-to-lmstudio-symlinks/internal/lmstudio"
	"github.com/qaribhaider/ollama-to-lmstudio-symlinks/internal/models"
	"github.com/qaribhaider/ollama-to-lmstudio-symlinks/internal/ollama"
	"github.com/qaribhaider/ollama-to-lmstudio-symlinks/internal/ui"
)

func runForward(o options) error {
	ui.PrintHeader("Ollama → LM Studio Linker")
	ui.PrintInfo(fmt.Sprintf("Ollama directory: %s", o.ollamaDir))
	ui.PrintInfo(fmt.Sprintf("Target LM Studio directory: %s", o.lmstudioDir))

	if o.dryRun {
		ui.PrintWarning("DRY RUN MODE - No changes will be made")
	}
	ui.PrintEmptyLine()

	// Pre-flight checks
	if _, err := ollama.ValidateOllama(o.skipChecks); err != nil {
		return err
	}
	if err := lmstudio.ValidateLMStudio(o.skipChecks, o.lmstudioDir); err != nil {
		return err
	}

	discoveredModels, err := ollama.DiscoverModels(o.ollamaDir, o.verbose)
	if err != nil {
		return fmt.Errorf("error discovering models: %w", err)
	}
	if len(discoveredModels) == 0 {
		return errors.New("no models found in Ollama directory")
	}

	toLink, proceed, err := chooseOllamaModels(discoveredModels, o.interactive)
	if err != nil || !proceed {
		return err
	}

	providerDir := filepath.Join(o.lmstudioDir, o.skipProvider)
	if !o.dryRun {
		if err := os.MkdirAll(providerDir, 0755); err != nil {
			return fmt.Errorf("error creating %s provider directory: %w", o.skipProvider, err)
		}
	}

	created, skipped := countLinked(toLink, func(m models.ModelInfo) bool {
		return linking.ProcessModel(m, o.ollamaDir, providerDir, o.dryRun, o.verbose, o.useHardlinks)
	})

	printResults(
		fmt.Sprintf("Linked:  %d", created),
		fmt.Sprintf("Skipped: %d", skipped),
	)
	if created > 0 && !o.dryRun {
		ui.PrintSuccess(fmt.Sprintf("Models are now available in LM Studio under the '%s' provider", o.skipProvider))
	}
	return nil
}

// chooseOllamaModels returns the models to link: the user's selection when
// interactive, otherwise everything. proceed is false when nothing should run.
func chooseOllamaModels(discovered []models.ModelInfo, interactive bool) (chosen []models.ModelInfo, proceed bool, err error) {
	name := func(m models.ModelInfo) string { return m.Name }
	if interactive {
		return selectModels(discovered, name, name)
	}
	printFound("models", discovered, name)
	return discovered, true, nil
}
