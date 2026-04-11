package main

import (
	"errors"
	"flag"
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

// Version of the application
var Version = "dev"

func main() {
	if err := runApp(os.Args[1:], os.Stdin); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			os.Exit(0)
		}
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}

func runApp(args []string, stdin io.Reader) error {
	fs := flag.NewFlagSet("ollama-symlinks", flag.ContinueOnError)
	fs.Usage = func() {
		ui.PrintHeader(fmt.Sprintf("Usage: %s [command] [flags]", fs.Name()))
		ui.PrintEmptyLine()
		
		fmt.Println(ui.HeaderStyle.Render("Commands"))
		fmt.Printf("  %-12s %s\n", "delete", "Interactively delete symlinks from Ollama or LM Studio")
		fmt.Printf("  %-12s %s\n", "cleanup", "Auto-discover and remove broken symlinks")
		ui.PrintEmptyLine()

		fmt.Println(ui.HeaderStyle.Render("Flags"))
		fs.VisitAll(func(f *flag.Flag) {
			_, usage := flag.UnquoteUsage(f)
			defaultText := ""
			if f.DefValue != "" && f.DefValue != "false" {
				defaultText = ui.MutedStyle.Render(fmt.Sprintf("(default: %q)", f.DefValue))
			}
			fmt.Printf("  %-18s %s %s\n", "--"+f.Name, usage, defaultText)
		})
		ui.PrintEmptyLine()
	}

	// Command line flags
	var ollamaDir = fs.String("ollama-dir", ollama.GetDefaultOllamaDir(), "Path to Ollama models directory")
	var lmstudioDir = fs.String("lmstudio-dir", lmstudio.GetDefaultLMStudioDir(), "Path to LM Studio models directory")
	var dryRun = fs.Bool("dry-run", false, "Show what would be done without actually creating symlinks")
	var verbose = fs.Bool("verbose", false, "Enable verbose output")
	var deepScan = fs.Bool("deep-scan", false, "Scan all Windows drives for model directories (fallback)")
	var showVersion = fs.Bool("version", false, "Show version information")

	var useHardlinks = fs.Bool("hardlinks", false, "Use hard links instead of symlinks (fixes '0 bytes' issue on Windows)")

	// Reverse mode flags
	var reverse = fs.Bool("reverse", false, "Link LM Studio models to Ollama (reverse mode)")
	var namePrefix = fs.String("name-prefix", "lms", "Prefix for models created in Ollama")
	var skipProvider = fs.String("skip-provider", "ollama", "Directory name to skip in LM Studio (to avoid circular links)")

	if err := fs.Parse(args); err != nil {
		return err
	}

	if *showVersion {
		fmt.Printf("ollama-symlinks version %s\n", Version)
		return nil
	}

	// Validation
	if *namePrefix == "" {
		return errors.New("--name-prefix cannot be empty")
	}

	// Simple subcommand handling
	remainingArgs := fs.Args()
	if len(remainingArgs) > 0 && remainingArgs[0] == "delete" {
		// New FlagSet for delete subcommand
		deleteFs := flag.NewFlagSet("delete", flag.ContinueOnError)
		deleteFs.Usage = func() {
			ui.PrintHeader(fmt.Sprintf("Usage: %s %s [flags]", fs.Name(), deleteFs.Name()))
			ui.PrintEmptyLine()
			
			fmt.Println(ui.HeaderStyle.Render("Flags"))
			deleteFs.VisitAll(func(f *flag.Flag) {
				_, usage := flag.UnquoteUsage(f)
				defaultText := ""
				if f.DefValue != "" && f.DefValue != "false" {
					defaultText = ui.MutedStyle.Render(fmt.Sprintf("(default: %q)", f.DefValue))
				}
				fmt.Printf("  %-14s %s %s\n", "--"+f.Name, usage, defaultText)
			})
			ui.PrintEmptyLine()
		}
		from := deleteFs.String("from", "", "Source to delete symlinks from (ollama or lmstudio)")
		deleteDryRun := deleteFs.Bool("dry-run", *dryRun, "Show what would be deleted without actually removing them")
		deleteVerbose := deleteFs.Bool("verbose", *verbose, "Enable verbose output")
		
		if err := deleteFs.Parse(remainingArgs[1:]); err != nil {
			return err
		}

		if len(deleteFs.Args()) > 0 {
			deleteFs.Usage()
			return fmt.Errorf("unexpected argument: %s", deleteFs.Args()[0])
		}

		if *from == "" {
			deleteFs.Usage()
			return errors.New("--from flag is required for delete command (ollama or lmstudio)")
		}

		if *from != "ollama" && *from != "lmstudio" {
			return fmt.Errorf("invalid --from value: %s (must be 'ollama' or 'lmstudio')", *from)
		}

		return runDelete(*from, *ollamaDir, *lmstudioDir, *skipProvider, *deleteDryRun, *deleteVerbose, stdin)
	}

	if len(remainingArgs) > 0 {
		if remainingArgs[0] == "cleanup" {
			cleanupFs := flag.NewFlagSet("cleanup", flag.ContinueOnError)
			cleanupFs.Usage = func() {
				ui.PrintHeader(fmt.Sprintf("Usage: %s %s [flags]", fs.Name(), cleanupFs.Name()))
				ui.PrintEmptyLine()
				
				fmt.Println(ui.HeaderStyle.Render("Flags"))
				cleanupFs.VisitAll(func(f *flag.Flag) {
					_, usage := flag.UnquoteUsage(f)
					defaultText := ""
					if f.DefValue != "" && f.DefValue != "false" {
						defaultText = ui.MutedStyle.Render(fmt.Sprintf("(default: %q)", f.DefValue))
					}
					fmt.Printf("  %-14s %s %s\n", "--"+f.Name, usage, defaultText)
				})
				ui.PrintEmptyLine()
			}
			
			cleanupDryRun := cleanupFs.Bool("dry-run", *dryRun, "Show what would be deleted without actually removing them")
			cleanupVerbose := cleanupFs.Bool("verbose", *verbose, "Enable verbose output")
			
			if err := cleanupFs.Parse(remainingArgs[1:]); err != nil {
				return err
			}
			
			if len(cleanupFs.Args()) > 0 {
				cleanupFs.Usage()
				return fmt.Errorf("unexpected argument: %s", cleanupFs.Args()[0])
			}
			
			return runCleanup(*lmstudioDir, *cleanupDryRun, *cleanupVerbose, stdin)
		}

		// Unknown command
		return fmt.Errorf("unknown command %q. Run '%s --help' for usage", remainingArgs[0], fs.Name())
	}

	// Path resolution with candidates
	if *ollamaDir == ollama.GetDefaultOllamaDir() {
		candidates := ollama.GetOllamaCandidates()
		if *deepScan && len(candidates) == 0 {
			if *verbose {
				fmt.Println("🔎 Scaling all Windows drives for Ollama models...")
			}
			candidates = ollama.ScanAllDrives()
		}
		if len(candidates) > 1 {
			fmt.Printf("📂 Found multiple Ollama model directories:\n")
			for i, c := range candidates {
				fmt.Printf("  [%d] %s\n", i+1, c)
			}
			fmt.Printf("Using the first one. Use --ollama-dir to override.\n\n")
		}
		if len(candidates) > 0 {
			*ollamaDir = candidates[0]
		}
	}

	if *lmstudioDir == lmstudio.GetDefaultLMStudioDir() {
		candidates := lmstudio.GetLMStudioCandidates()
		if *deepScan && len(candidates) == 0 {
			if *verbose {
				fmt.Println("🔎 Scaling all Windows drives for LM Studio models...")
			}
			candidates = lmstudio.ScanAllDrives()
		}
		if len(candidates) > 1 {
			fmt.Printf("📂 Found multiple LM Studio model directories:\n")
			for i, c := range candidates {
				fmt.Printf("  [%d] %s\n", i+1, c)
			}
			fmt.Printf("Using the first one. Use --lmstudio-dir to override.\n\n")
		}
		if len(candidates) > 0 {
			*lmstudioDir = candidates[0]
		}
	}

	if *reverse {
		// Check LM Studio dir exists
		if _, err := os.Stat(*lmstudioDir); os.IsNotExist(err) {
			return fmt.Errorf("LM Studio directory does not exist: %s. Use --lmstudio-dir or --deep-scan to help find it.", *lmstudioDir)
		}
		if err := runReverse(*lmstudioDir, *ollamaDir, *namePrefix, *skipProvider, *dryRun, *verbose, *useHardlinks); err != nil {
			return err
		}
	} else {
		// Check Ollama dir exists
		if _, err := os.Stat(*ollamaDir); os.IsNotExist(err) {
			return fmt.Errorf("Ollama directory does not exist: %s. Use --ollama-dir or --deep-scan to help find it.", *ollamaDir)
		}
		if err := runForward(*ollamaDir, *lmstudioDir, *dryRun, *verbose, *useHardlinks); err != nil {
			return err
		}
	}

	return nil
}

func runDelete(from, ollamaDir, lmstudioDir, skipProvider string, dryRun, verbose bool, stdin io.Reader) error {
	if from == "ollama" {
		return runDeleteOllama(ollamaDir, dryRun, verbose, stdin)
	}

	targetDir := filepath.Join(lmstudioDir, skipProvider)
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

	if !dryRun {
		var confirm bool
		err = huh.NewConfirm().
			Title(fmt.Sprintf("Are you sure you want to delete %d symlinks?", len(toDelete))).
			Affirmative("Yes").
			Negative("No").
			Value(&confirm).
			Run()

		if err != nil || !confirm {
			ui.PrintInfo("Cancelled")
			return nil
		}
	}

	removed, failed := linking.RemoveSymlinks(toDelete, dryRun)
	fmt.Println(ui.FormatSummary(
		ui.HeaderStyle.Render("Results"),
		fmt.Sprintf("Removed: %d", removed),
		fmt.Sprintf("Failed:  %d", failed),
	))
	return nil
}

func runDeleteOllama(ollamaDir string, dryRun, verbose bool, stdin io.Reader) error {
	// Check if Ollama manifests and blobs dirs exist
	manifestsDir := filepath.Join(ollamaDir, "manifests")
	if _, err := os.Stat(manifestsDir); os.IsNotExist(err) {
		return fmt.Errorf("target directory for deletion does not exist: %s (manifests not found)", ollamaDir)
	}

	ui.PrintSubheader("Scanning Ollama models for symlinks...")
	allModels, err := ollama.DiscoverModels(ollamaDir, verbose)
	if err != nil {
		return fmt.Errorf("could not discover models: %w", err)
	}

	var symlinkedModels []models.ModelInfo
	for _, m := range allModels {
		if len(m.MainModelBlobs) == 0 {
			continue
		}
		// Check if the manifest uses a symlinked blob
		// The blob filename is "sha256-hash" (no colon)
		blobFilename := strings.Replace(m.MainModelBlobs[0], ":", "-", 1)
		blobPath := filepath.Join(ollamaDir, "blobs", blobFilename)
		
		info, err := os.Lstat(blobPath)
		isOurModel := strings.HasPrefix(m.Name, "lms-") || strings.HasPrefix(m.Name, "lms:")
		
		if err == nil {
			if info.Mode()&os.ModeSymlink != 0 {
				symlinkedModels = append(symlinkedModels, m)
			} else if info.Mode().IsRegular() && isOurModel {
				// Regular file but with our prefix - likely a hard link
				symlinkedModels = append(symlinkedModels, m)
			}
		} else if os.IsNotExist(err) && isOurModel {
			// Also include broken models (ghosts) if they have our prefix
			symlinkedModels = append(symlinkedModels, m)
		}
	}

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
		targetPath, err := linking.SecureJoin(filepath.Join(ollamaDir, "blobs"), blobFilename)
		
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

	if !dryRun {
		var confirm bool
		err = huh.NewConfirm().
			Title(fmt.Sprintf("Are you sure you want to delete %d models from Ollama?", len(toDelete))).
			Affirmative("Yes").
			Negative("No").
			Value(&confirm).
			Run()

		if err != nil || !confirm {
			ui.PrintInfo("Cancelled")
			return nil
		}
	}

	var removed, failed int
	for _, m := range toDelete {
		if dryRun {
			ui.PrintMuted(fmt.Sprintf("Would remove: %s", m.Name))
			removed++
			continue
		}

		if verbose {
			ui.PrintMuted(fmt.Sprintf("Deleting %s via 'ollama rm'...", m.Name))
		}
		
		// Use 'ollama rm' to properly clean up manifest and blob links
		importPath := m.Name
		// Validate model name to prevent accidental flag interpretation
		if matched, _ := regexp.MatchString(`^[a-zA-Z0-9._-]+:[a-zA-Z0-9._-]+$`, importPath); !matched {
			ui.PrintError(fmt.Sprintf("Refusing to run 'ollama rm' with unsafe model name: %q", importPath))
			failed++
			continue
		}
		
		cmd := exec.Command("ollama", "rm", importPath)
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

func runForward(ollamaDir, lmstudioDir string, dryRun, verbose, useHardlinks bool) error {
	ui.PrintHeader("Ollama → LM Studio Linker")
	ui.PrintInfo(fmt.Sprintf("Ollama directory: %s", ollamaDir))
	ui.PrintInfo(fmt.Sprintf("Target LM Studio directory: %s", lmstudioDir))

	if dryRun {
		ui.PrintWarning("DRY RUN MODE - No changes will be made")
	}
	ui.PrintEmptyLine()

	// Discover models
	models, err := ollama.DiscoverModels(ollamaDir, verbose)
	if err != nil {
		return fmt.Errorf("error discovering models: %w", err)
	}

	if len(models) == 0 {
		ui.PrintError("No models found in Ollama directory")
		return nil
	}

	ui.PrintSubheader(fmt.Sprintf("Found %d models", len(models)))
	for _, model := range models {
		ui.PrintBullet(model.Name)
	}
	ui.PrintEmptyLine()

	// Create ollama provider directory
	ollamaProviderDir := filepath.Join(lmstudioDir, "ollama")
	if !dryRun {
		if err := os.MkdirAll(ollamaProviderDir, 0755); err != nil {
			return fmt.Errorf("error creating ollama provider directory: %w", err)
		}
	}

	// Process each model
	var created, skipped int
	for _, model := range models {
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

func runReverse(lmstudioDir, ollamaDir, namePrefix, skipProvider string, dryRun, verbose, useHardlinks bool) error {
	ui.PrintHeader("LM Studio → Ollama Linker")
	ui.PrintInfo(fmt.Sprintf("LM Studio directory: %s", lmstudioDir))
	ui.PrintInfo(fmt.Sprintf("Target Ollama directory: %s", ollamaDir))
	if dryRun {
		ui.PrintWarning("DRY RUN MODE - No changes will be made")
	}
	ui.PrintEmptyLine()

	// Discover models
	models, err := lmstudio.DiscoverLMStudioModels(lmstudioDir, skipProvider, verbose)
	if err != nil {
		return fmt.Errorf("error discovering models: %w", err)
	}

	if len(models) == 0 {
		ui.PrintError("No eligible models found in LM Studio directory")
		return nil
	}

	ui.PrintSubheader(fmt.Sprintf("Found %d eligible models", len(models)))
	for _, model := range models {
		ui.PrintBullet(fmt.Sprintf("%s (%s)", model.Name, model.Path))
	}
	ui.PrintEmptyLine()

	// Process each model
	var created, skipped int
	for _, model := range models {
		result := linking.ProcessLMStudioModel(model, ollamaDir, namePrefix, dryRun, verbose, useHardlinks)
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

func runCleanup(lmstudioDir string, dryRun, verbose bool, stdin io.Reader) error {
	targetDir := filepath.Join(lmstudioDir, "ollama")
	
	// Check if target dir exists
	if _, err := os.Stat(targetDir); os.IsNotExist(err) {
		ui.PrintSuccess("No managed 'ollama' directory found in LM Studio. Nothing to clean.")
		return nil
	}

	ui.PrintSubheader("Scanning for broken symlinks in: " + targetDir)
	brokenLinks, err := linking.FindBrokenSymlinks(targetDir)
	if err != nil {
		return fmt.Errorf("could not search for broken symlinks: %w", err)
	}

	if len(brokenLinks) == 0 {
		ui.PrintSuccess("No broken symbolic links found.")
		return nil
	}

	ui.PrintInfo(fmt.Sprintf("Found %d broken symbolic links (targets are missing):", len(brokenLinks)))
	for _, link := range brokenLinks {
		ui.PrintBullet(fmt.Sprintf("%s -> %s", link.Path, link.Target))
	}
	ui.PrintEmptyLine()

	if dryRun {
		ui.PrintWarning("DRY RUN MODE - No items will be removed.")
		return nil
	}

	var confirm bool
	err = huh.NewConfirm().
		Title(fmt.Sprintf("Are you sure you want to delete these %d broken symlinks?", len(brokenLinks))).
		Affirmative("Yes").
		Negative("No").
		Value(&confirm).
		Run()

	if err != nil || !confirm {
		ui.PrintInfo("Cancelled")
		return nil
	}

	var paths []string
	for _, link := range brokenLinks {
		paths = append(paths, link.Path)
	}

	removed, failed := linking.RemoveSymlinks(paths, false)
	fmt.Println(ui.FormatSummary(
		ui.HeaderStyle.Render("Results"),
		fmt.Sprintf("Removed: %d", removed),
		fmt.Sprintf("Failed:  %d", failed),
	))
	
	// Try to remove empty directories
	if removed > 0 {
		if verbose {
			ui.PrintMuted("Cleaning up empty model directories...")
		}
		entries, err := os.ReadDir(targetDir)
		if err == nil {
			for _, entry := range entries {
				if entry.IsDir() {
					dirPath := filepath.Join(targetDir, entry.Name())
					// Only remove if empty
					if files, err := os.ReadDir(dirPath); err == nil && len(files) == 0 {
						os.Remove(dirPath)
						if verbose {
							ui.PrintMuted(fmt.Sprintf("Removed empty directory: %s", entry.Name()))
						}
					}
				}
			}
		}
	}

	return nil
}
