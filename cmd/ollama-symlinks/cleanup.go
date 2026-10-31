package main

import (
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/qaribhaider/ollama-to-lmstudio-symlinks/internal/linking"
	"github.com/qaribhaider/ollama-to-lmstudio-symlinks/internal/ui"
)

func runCleanup(o options, stdin io.Reader) error {
	var totalBroken []linking.SymlinkInfo
	var scanLocations []string

	// 1. LM Studio managed directory
	lmsTargetDir := filepath.Join(o.lmstudioDir, o.skipProvider)
	if _, err := os.Stat(lmsTargetDir); err == nil {
		scanLocations = append(scanLocations, lmsTargetDir)
		ui.PrintSubheader("Scanning for broken symlinks in LM Studio: " + lmsTargetDir)
		broken, err := linking.FindBrokenSymlinks(lmsTargetDir)
		if err != nil {
			ui.PrintWarning(fmt.Sprintf("Could not search for broken symlinks in %s: %v", lmsTargetDir, err))
		} else {
			totalBroken = append(totalBroken, broken...)
		}
	}

	// 2. Ollama blobs directory
	ollamaBlobsDir := filepath.Join(o.ollamaDir, "blobs")
	if _, err := os.Stat(ollamaBlobsDir); err == nil {
		scanLocations = append(scanLocations, ollamaBlobsDir)
		ui.PrintSubheader("Scanning for broken symlinks in Ollama blobs: " + ollamaBlobsDir)
		broken, err := linking.FindBrokenSymlinks(ollamaBlobsDir)
		if err != nil {
			ui.PrintWarning(fmt.Sprintf("Could not search for broken symlinks in %s: %v", ollamaBlobsDir, err))
		} else {
			totalBroken = append(totalBroken, broken...)
		}
	}

	if len(scanLocations) == 0 {
		ui.PrintSuccess("No managed model directories found to clean.")
		return nil
	}

	if len(totalBroken) == 0 {
		ui.PrintSuccess("No broken symbolic links found across LM Studio and Ollama.")
		return nil
	}

	ui.PrintInfo(fmt.Sprintf("Found %d broken symbolic link(s) (targets are missing):", len(totalBroken)))
	for _, link := range totalBroken {
		ui.PrintBullet(fmt.Sprintf("%s -> %s", link.Path, link.Target))
	}
	ui.PrintEmptyLine()

	if o.dryRun {
		ui.PrintWarning("DRY RUN MODE - No items will be removed.")
		return nil
	}

	confirmed, err := askConfirm(fmt.Sprintf("Are you sure you want to delete these %d broken symlink(s)?", len(totalBroken)))
	if err != nil {
		return err
	}
	if !confirmed {
		ui.PrintInfo("Cancelled")
		return nil
	}

	var paths []string
	for _, link := range totalBroken {
		paths = append(paths, link.Path)
	}

	removed, failed := linking.RemoveSymlinks(paths, false)
	fmt.Println(ui.FormatSummary(
		ui.HeaderStyle.Render("Results"),
		fmt.Sprintf("Removed: %d", removed),
		fmt.Sprintf("Failed:  %d", failed),
	))

	// Clean up any empty directories in LM Studio
	if removed > 0 && lmsTargetDir != "" {
		if o.verbose {
			ui.PrintMuted("Cleaning up empty model directories in LM Studio...")
		}
		entries, err := os.ReadDir(lmsTargetDir)
		if err == nil {
			for _, entry := range entries {
				if entry.IsDir() {
					dirPath := filepath.Join(lmsTargetDir, entry.Name())
					if files, err := os.ReadDir(dirPath); err == nil && len(files) == 0 {
						os.Remove(dirPath)
						if o.verbose {
							ui.PrintMuted(fmt.Sprintf("Removed empty directory: %s", entry.Name()))
						}
					}
				}
			}
		}
	}

	return nil
}
