package main

import (
	"fmt"
	"path/filepath"

	"github.com/qaribhaider/ollama-to-lmstudio-symlinks/internal/linking"
	"github.com/qaribhaider/ollama-to-lmstudio-symlinks/internal/lmstudio"
	"github.com/qaribhaider/ollama-to-lmstudio-symlinks/internal/ollama"
	"github.com/qaribhaider/ollama-to-lmstudio-symlinks/internal/ui"
)

func runStatus(o options) error {
	ui.PrintHeader("Ollama ↔ LM Studio Status Report")
	ui.PrintEmptyLine()

	printEnvironment(o)

	lmsTargetDir := filepath.Join(o.lmstudioDir, o.skipProvider)
	lmsStatus, _ := linking.InspectDirectory(lmsTargetDir)
	printDirStatus("LM Studio Models ("+lmsTargetDir+")", "Directory does not exist yet (no forward symlinks created).", lmsStatus, o.verbose)

	ollamaBlobsDir := filepath.Join(o.ollamaDir, "blobs")
	ollamaStatus, _ := linking.InspectDirectory(ollamaBlobsDir)
	printDirStatus("Ollama Blobs ("+ollamaBlobsDir+")", "Directory does not exist yet (no reverse symlinks created).", ollamaStatus, o.verbose)

	totalSaved := lmsStatus.BytesSaved + ollamaStatus.BytesSaved
	totalActive := lmsStatus.ActiveLinks + ollamaStatus.ActiveLinks
	totalBroken := lmsStatus.BrokenLinks + ollamaStatus.BrokenLinks

	fmt.Println(ui.FormatSummary(
		ui.HeaderStyle.Render("Storage Savings Summary"),
		fmt.Sprintf("Active Symlinks: %d", totalActive),
		fmt.Sprintf("Broken Symlinks: %d", totalBroken),
		fmt.Sprintf("Total Saved:     %s", linking.FormatBytes(totalSaved)),
	))

	return nil
}

func printEnvironment(o options) {
	fmt.Println(ui.HeaderStyle.Render("Executables & Environment"))
	ollamaBin, ollamaErr := ollama.GetOllamaExecutable()
	if ollamaErr == nil && ollamaBin != "" {
		ui.PrintBullet(fmt.Sprintf("Ollama CLI:    installed (%s)", ollamaBin))
	} else {
		ui.PrintWarning("Ollama CLI:    not found in PATH")
	}

	lmsBin, lmsErr := lmstudio.GetLMStudioExecutable()
	if lmsErr == nil && lmsBin != "" {
		ui.PrintBullet(fmt.Sprintf("LM Studio CLI: installed (%s)", lmsBin))
	} else if lmstudio.IsLMStudioInstalled(o.lmstudioDir) {
		ui.PrintBullet("LM Studio:     models directory detected")
	} else {
		ui.PrintWarning("LM Studio:     not detected")
	}
	ui.PrintEmptyLine()
}

// printDirStatus prints one managed directory's link counts and, when verbose,
// each link.
func printDirStatus(title, missingMessage string, status linking.DirStatus, verbose bool) {
	fmt.Println(ui.HeaderStyle.Render(title))
	if !status.Exists {
		ui.PrintMuted(missingMessage)
		ui.PrintEmptyLine()
		return
	}

	ui.PrintBullet(fmt.Sprintf("Active symlinks:  %d", status.ActiveLinks))
	if status.BrokenLinks > 0 {
		ui.PrintError(fmt.Sprintf("Broken symlinks:  %d (run 'ollama-symlinks cleanup' to remove)", status.BrokenLinks))
	} else {
		ui.PrintBullet("Broken symlinks:  0")
	}
	ui.PrintBullet(fmt.Sprintf("Disk space saved: %s", linking.FormatBytes(status.BytesSaved)))
	if verbose {
		for _, f := range status.Files {
			if f.IsBroken {
				ui.PrintMuted(fmt.Sprintf("  ✖ %s -> [broken: %s]", f.Name, f.Target))
			} else {
				ui.PrintMuted(fmt.Sprintf("  ✔ %s (%s)", f.Name, linking.FormatBytes(f.TargetSize)))
			}
		}
	}
	ui.PrintEmptyLine()
}
