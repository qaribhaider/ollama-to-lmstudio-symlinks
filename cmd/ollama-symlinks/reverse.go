package main

import (
	"errors"
	"fmt"
	"path/filepath"

	"github.com/qaribhaider/ollama-to-lmstudio-symlinks/internal/linking"
	"github.com/qaribhaider/ollama-to-lmstudio-symlinks/internal/lmstudio"
	"github.com/qaribhaider/ollama-to-lmstudio-symlinks/internal/models"
	"github.com/qaribhaider/ollama-to-lmstudio-symlinks/internal/ollama"
	"github.com/qaribhaider/ollama-to-lmstudio-symlinks/internal/ui"
)

func runReverse(o options) error {
	ui.PrintHeader("LM Studio → Ollama Linker")
	ui.PrintInfo(fmt.Sprintf("LM Studio directory: %s", o.lmstudioDir))
	ui.PrintInfo(fmt.Sprintf("Target Ollama directory: %s", o.ollamaDir))
	if o.dryRun {
		ui.PrintWarning("DRY RUN MODE - No changes will be made")
	}
	ui.PrintEmptyLine()

	// Pre-flight checks
	ollamaBin, err := ollama.ValidateOllama(o.skipChecks)
	if err != nil {
		return err
	}
	if err := lmstudio.ValidateLMStudio(o.skipChecks, o.lmstudioDir); err != nil {
		return err
	}

	discoveredModels, err := lmstudio.DiscoverLMStudioModels(o.lmstudioDir, o.skipProvider, o.verbose)
	if err != nil {
		return fmt.Errorf("error discovering models: %w", err)
	}
	if len(discoveredModels) == 0 {
		return errors.New("no eligible models found in LM Studio directory")
	}

	toLink, proceed, err := chooseLMStudioModels(discoveredModels, o.interactive)
	if err != nil || !proceed {
		return err
	}

	created, skipped := countLinked(toLink, func(m models.LMStudioModel) bool {
		return linking.ProcessLMStudioModel(m, o.ollamaDir, o.namePrefix, ollamaBin, o.dryRun, o.verbose, o.useHardlinks)
	})

	printResults(
		fmt.Sprintf("Linked:  %d", created),
		fmt.Sprintf("Skipped: %d", skipped),
	)
	if created > 0 && !o.dryRun {
		ui.PrintSuccess(fmt.Sprintf("Models are now available in Ollama with the '%s-' prefix", o.namePrefix))
	}
	return nil
}

// chooseLMStudioModels returns the models to link: the user's selection when
// interactive, otherwise everything. proceed is false when nothing should run.
func chooseLMStudioModels(discovered []models.LMStudioModel, interactive bool) (chosen []models.LMStudioModel, proceed bool, err error) {
	name := func(m models.LMStudioModel) string { return m.Name }
	if interactive {
		return selectModels(discovered, name, func(m models.LMStudioModel) string {
			return m.Name + modelAnnotation(m)
		})
	}
	printFound("eligible models", discovered, func(m models.LMStudioModel) string {
		return fmt.Sprintf("%s (%s)%s", m.Name, m.Path, modelAnnotation(m))
	})
	return discovered, true, nil
}

// modelAnnotation describes a vision projector or shard count for display.
func modelAnnotation(m models.LMStudioModel) string {
	if m.ProjectorPath != "" {
		return fmt.Sprintf(" [vision: %s]", filepath.Base(m.ProjectorPath))
	}
	if len(m.ShardPaths) > 0 {
		return fmt.Sprintf(" [%d shards]", len(m.ShardPaths)+1)
	}
	return ""
}
