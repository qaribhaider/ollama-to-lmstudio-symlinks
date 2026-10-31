package main

import (
	"fmt"
	"path/filepath"

	"github.com/qaribhaider/ollama-to-lmstudio-symlinks/internal/linking"
	"github.com/qaribhaider/ollama-to-lmstudio-symlinks/internal/lmstudio"
	"github.com/qaribhaider/ollama-to-lmstudio-symlinks/internal/ollama"
	"github.com/qaribhaider/ollama-to-lmstudio-symlinks/internal/ui"
)

func runStatus(ollamaDir, lmstudioDir, skipProvider string, verbose bool) error {
	ui.PrintHeader("Ollama ↔ LM Studio Status Report")
	ui.PrintEmptyLine()

	// 1. Executables & Environment
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
	} else if lmstudio.IsLMStudioInstalled(lmstudioDir) {
		ui.PrintBullet("LM Studio:     models directory detected")
	} else {
		ui.PrintWarning("LM Studio:     not detected")
	}
	ui.PrintEmptyLine()

	// 2. LM Studio Managed Links (lmstudioDir/skipProvider)
	lmsTargetDir := filepath.Join(lmstudioDir, skipProvider)
	lmsStatus, _ := linking.InspectDirectory(lmsTargetDir)

	fmt.Println(ui.HeaderStyle.Render("LM Studio Models (" + lmsTargetDir + ")"))
	if !lmsStatus.Exists {
		ui.PrintMuted("Directory does not exist yet (no forward symlinks created).")
	} else {
		ui.PrintBullet(fmt.Sprintf("Active symlinks:  %d", lmsStatus.ActiveLinks))
		if lmsStatus.BrokenLinks > 0 {
			ui.PrintError(fmt.Sprintf("Broken symlinks:  %d (run 'ollama-symlinks cleanup' to remove)", lmsStatus.BrokenLinks))
		} else {
			ui.PrintBullet("Broken symlinks:  0")
		}
		ui.PrintBullet(fmt.Sprintf("Disk space saved: %s", linking.FormatBytes(lmsStatus.BytesSaved)))
		if verbose && len(lmsStatus.Files) > 0 {
			for _, f := range lmsStatus.Files {
				if f.IsBroken {
					ui.PrintMuted(fmt.Sprintf("  ✖ %s -> [broken: %s]", f.Name, f.Target))
				} else {
					ui.PrintMuted(fmt.Sprintf("  ✔ %s (%s)", f.Name, linking.FormatBytes(f.TargetSize)))
				}
			}
		}
	}
	ui.PrintEmptyLine()

	// 3. Ollama Blobs Managed Links (ollamaDir/blobs)
	ollamaBlobsDir := filepath.Join(ollamaDir, "blobs")
	ollamaStatus, _ := linking.InspectDirectory(ollamaBlobsDir)

	fmt.Println(ui.HeaderStyle.Render("Ollama Blobs (" + ollamaBlobsDir + ")"))
	if !ollamaStatus.Exists {
		ui.PrintMuted("Directory does not exist yet (no reverse symlinks created).")
	} else {
		ui.PrintBullet(fmt.Sprintf("Active symlinks:  %d", ollamaStatus.ActiveLinks))
		if ollamaStatus.BrokenLinks > 0 {
			ui.PrintError(fmt.Sprintf("Broken symlinks:  %d (run 'ollama-symlinks cleanup' to remove)", ollamaStatus.BrokenLinks))
		} else {
			ui.PrintBullet("Broken symlinks:  0")
		}
		ui.PrintBullet(fmt.Sprintf("Disk space saved: %s", linking.FormatBytes(ollamaStatus.BytesSaved)))
		if verbose && len(ollamaStatus.Files) > 0 {
			for _, f := range ollamaStatus.Files {
				if f.IsBroken {
					ui.PrintMuted(fmt.Sprintf("  ✖ %s -> [broken: %s]", f.Name, f.Target))
				} else {
					ui.PrintMuted(fmt.Sprintf("  ✔ %s (%s)", f.Name, linking.FormatBytes(f.TargetSize)))
				}
			}
		}
	}
	ui.PrintEmptyLine()

	// 4. Overall Disk Savings Summary
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
