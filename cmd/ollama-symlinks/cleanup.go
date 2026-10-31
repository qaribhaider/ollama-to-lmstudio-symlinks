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
	lmsTargetDir := filepath.Join(o.lmstudioDir, o.skipProvider)
	locations := []struct{ label, dir string }{
		{"LM Studio", lmsTargetDir},
		{"Ollama blobs", filepath.Join(o.ollamaDir, "blobs")},
	}

	var totalBroken []linking.SymlinkInfo
	scanned := 0
	for _, loc := range locations {
		broken, ok := scanForBroken(loc.label, loc.dir)
		if ok {
			scanned++
		}
		totalBroken = append(totalBroken, broken...)
	}

	if scanned == 0 {
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
	printResults(
		fmt.Sprintf("Removed: %d", removed),
		fmt.Sprintf("Failed:  %d", failed),
	)

	if removed > 0 {
		removeEmptySubdirs(lmsTargetDir, o.verbose)
	}
	return nil
}

// scanForBroken reports the broken links in dir. scanned is false when dir does
// not exist, so callers can tell "nothing to clean" from "nothing broken".
func scanForBroken(label, dir string) (broken []linking.SymlinkInfo, scanned bool) {
	if _, err := os.Stat(dir); err != nil {
		return nil, false
	}

	ui.PrintSubheader(fmt.Sprintf("Scanning for broken symlinks in %s: %s", label, dir))
	broken, err := linking.FindBrokenSymlinks(dir)
	if err != nil {
		ui.PrintWarning(fmt.Sprintf("Could not search for broken symlinks in %s: %v", dir, err))
		return nil, true
	}
	return broken, true
}

// removeEmptySubdirs deletes the model folders left empty once their links are gone.
func removeEmptySubdirs(dir string, verbose bool) {
	if verbose {
		ui.PrintMuted("Cleaning up empty model directories in LM Studio...")
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return
	}
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		subdir := filepath.Join(dir, entry.Name())
		files, err := os.ReadDir(subdir)
		if err != nil || len(files) > 0 {
			continue
		}
		if err := os.Remove(subdir); err == nil && verbose {
			ui.PrintMuted(fmt.Sprintf("Removed empty directory: %s", entry.Name()))
		}
	}
}
