package main

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/qaribhaider/ollama-to-lmstudio-symlinks/internal/linking"
	"github.com/qaribhaider/ollama-to-lmstudio-symlinks/internal/lmstudio"
	"github.com/qaribhaider/ollama-to-lmstudio-symlinks/internal/models"
	"github.com/qaribhaider/ollama-to-lmstudio-symlinks/internal/ollama"
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
		fmt.Fprintf(fs.Output(), "Usage of %s:\n", fs.Name())
		fmt.Fprintf(fs.Output(), "\nFlags:\n")
		fs.VisitAll(func(f *flag.Flag) {
			_, usage := flag.UnquoteUsage(f)
			fmt.Fprintf(fs.Output(), "  --%s\t%s", f.Name, usage)
			if f.DefValue != "" && f.DefValue != "false" {
				fmt.Fprintf(fs.Output(), " (default %q)", f.DefValue)
			}
			fmt.Fprint(fs.Output(), "\n")
		})
		fmt.Fprintf(fs.Output(), "\nCommands:\n")
		fmt.Fprintf(fs.Output(), "  delete\tInteractively delete symlinks from Ollama or LM Studio\n")
		fmt.Fprintf(fs.Output(), "\nRun '%s [command] --help' for more information on a command.\n", fs.Name())
	}

	// Command line flags
	var ollamaDir = fs.String("ollama-dir", ollama.GetDefaultOllamaDir(), "Path to Ollama models directory")
	var lmstudioDir = fs.String("lmstudio-dir", lmstudio.GetDefaultLMStudioDir(), "Path to LM Studio models directory")
	var dryRun = fs.Bool("dry-run", false, "Show what would be done without actually creating symlinks")
	var verbose = fs.Bool("verbose", false, "Enable verbose output")
	var deepScan = fs.Bool("deep-scan", false, "Scan all Windows drives for model directories (fallback)")
	var showVersion = fs.Bool("version", false, "Show version information")

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
			fmt.Fprintf(deleteFs.Output(), "Usage of %s %s:\n", fs.Name(), deleteFs.Name())
			deleteFs.VisitAll(func(f *flag.Flag) {
				_, usage := flag.UnquoteUsage(f)
				fmt.Fprintf(deleteFs.Output(), "  --%s\t%s", f.Name, usage)
				if f.DefValue != "" && f.DefValue != "false" {
					fmt.Fprintf(deleteFs.Output(), " (default %q)", f.DefValue)
				}
				fmt.Fprint(deleteFs.Output(), "\n")
			})
		}
		from := deleteFs.String("from", "", "Source to delete symlinks from (ollama or lmstudio)")
		deleteDryRun := deleteFs.Bool("dry-run", *dryRun, "Show what would be deleted without actually removing them")
		deleteVerbose := deleteFs.Bool("verbose", *verbose, "Enable verbose output")
		
		if err := deleteFs.Parse(remainingArgs[1:]); err != nil {
			return err
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
			return runCleanup(*lmstudioDir, *dryRun, *verbose, stdin)
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
		runReverse(*lmstudioDir, *ollamaDir, *namePrefix, *skipProvider, *dryRun, *verbose)
	} else {
		// Check Ollama dir exists
		if _, err := os.Stat(*ollamaDir); os.IsNotExist(err) {
			return fmt.Errorf("Ollama directory does not exist: %s. Use --ollama-dir or --deep-scan to help find it.", *ollamaDir)
		}
		runForward(*ollamaDir, *lmstudioDir, *dryRun, *verbose)
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

	fmt.Printf("🔍 Scanning for symlinks in: %s\n", targetDir)
	links, err := linking.ListSymlinks(targetDir)
	if err != nil {
		return fmt.Errorf("could not list symlinks: %w", err)
	}

	if len(links) == 0 {
		fmt.Printf("✅ No symbolic links found in %s\n", targetDir)
		return nil
	}

	fmt.Printf("📦 Found %d symbolic links:\n", len(links))
	for i, link := range links {
		fmt.Printf("  %d) %s -> %s\n", i+1, link.Name, link.Target)
	}
	fmt.Println()

	fmt.Print("📝 Enter numbers to delete (e.g. 1, 2, 5) or 'all' (or 'q' to quit): ")
	var input string
	fmt.Fscanln(stdin, &input)

	if input == "q" || input == "" {
		fmt.Println("👋 Cancelled")
		return nil
	}

	var toDelete []string
	if input == "all" {
		for _, link := range links {
			toDelete = append(toDelete, link.Path)
		}
	} else {
		// Parse comma-separated numbers
		parts := strings.Split(input, ",")
		for _, p := range parts {
			var idx int
			_, err := fmt.Sscanf(strings.TrimSpace(p), "%d", &idx)
			if err != nil || idx < 1 || idx > len(links) {
				fmt.Printf("⚠️  Skipping invalid selection: %s\n", p)
				continue
			}
			toDelete = append(toDelete, links[idx-1].Path)
		}
	}

	if len(toDelete) == 0 {
		fmt.Println("🤷 No valid items selected for deletion")
		return nil
	}

	if !dryRun {
		fmt.Printf("⚠️  Are you sure you want to delete %d symlinks? (y/n): ", len(toDelete))
		var confirm string
		fmt.Fscanln(stdin, &confirm)
		if strings.ToLower(confirm) != "y" {
			fmt.Println("👋 Cancelled")
			return nil
		}
	}

	removed, failed := linking.RemoveSymlinks(toDelete, dryRun)
	fmt.Printf("\n✅ Summary: %d removed, %d failed\n", removed, failed)
	return nil
}

func runDeleteOllama(ollamaDir string, dryRun, verbose bool, stdin io.Reader) error {
	// Check if Ollama manifests and blobs dirs exist
	manifestsDir := filepath.Join(ollamaDir, "manifests")
	if _, err := os.Stat(manifestsDir); os.IsNotExist(err) {
		return fmt.Errorf("target directory for deletion does not exist: %s (manifests not found)", ollamaDir)
	}

	fmt.Printf("🔍 Scanning Ollama models for symlinks...\n")
	allModels, err := ollama.DiscoverModels(ollamaDir, verbose)
	if err != nil {
		return fmt.Errorf("could not discover models: %w", err)
	}

	var symlinkedModels []models.ModelInfo
	for _, m := range allModels {
		// Check if the manifest uses a symlinked blob
		// The blob filename is "sha256-hash" (no colon)
		blobFilename := strings.Replace(m.MainModelBlob, ":", "-", 1)
		blobPath := filepath.Join(ollamaDir, "blobs", blobFilename)
		
		info, err := os.Lstat(blobPath)
		if err == nil && info.Mode()&os.ModeSymlink != 0 {
			symlinkedModels = append(symlinkedModels, m)
		} else if os.IsNotExist(err) && strings.HasPrefix(m.Name, "lms-") {
			// Also include broken models (ghosts) if they have our prefix
			symlinkedModels = append(symlinkedModels, m)
		}
	}

	if len(symlinkedModels) == 0 {
		fmt.Println("✅ No symlinked models found in Ollama")
		return nil
	}

	fmt.Printf("📦 Found %d symlinked models in Ollama:\n", len(symlinkedModels))
	for i, m := range symlinkedModels {
		blobFilename := strings.Replace(m.MainModelBlob, ":", "-", 1)
		target, err := os.Readlink(filepath.Join(ollamaDir, "blobs", blobFilename))
		if err != nil {
			target = "(missing blob)"
		}
		fmt.Printf("  %d) %s -> %s\n", i+1, m.Name, target)
	}
	fmt.Println()

	fmt.Print("📝 Enter numbers to delete (e.g. 1, 2, 5) or 'all' (or 'q' to quit): ")
	var input string
	fmt.Fscanln(stdin, &input)

	if input == "q" || input == "" {
		fmt.Println("👋 Cancelled")
		return nil
	}

	var toDelete []models.ModelInfo
	if input == "all" {
		toDelete = symlinkedModels
	} else {
		parts := strings.Split(input, ",")
		for _, p := range parts {
			var idx int
			_, err := fmt.Sscanf(strings.TrimSpace(p), "%d", &idx)
			if err != nil || idx < 1 || idx > len(symlinkedModels) {
				fmt.Printf("⚠️  Skipping invalid selection: %s\n", p)
				continue
			}
			toDelete = append(toDelete, symlinkedModels[idx-1])
		}
	}

	if len(toDelete) == 0 {
		fmt.Println("🤷 No valid items selected for deletion")
		return nil
	}

	if !dryRun {
		fmt.Printf("⚠️  Are you sure you want to delete %d models from Ollama? (y/n): ", len(toDelete))
		var confirm string
		fmt.Fscanln(stdin, &confirm)
		if strings.ToLower(confirm) != "y" {
			fmt.Println("👋 Cancelled")
			return nil
		}
	}

	var removed, failed int
	for _, m := range toDelete {
		if dryRun {
			fmt.Printf("  Would remove: %s\n", m.Name)
			removed++
			continue
		}

		if verbose {
			fmt.Printf("  🚀 Deleting %s via 'ollama rm'...\n", m.Name)
		}
		
		// Use 'ollama rm' to properly clean up manifest and blob links
		importPath := m.Name
		// If it has no tag, it might need one? ollama.DiscoverModels adds -latest or similar?
		// Actually ollama.DiscoverModels extracts variant.
		
		cmd := exec.Command("ollama", "rm", importPath)
		output, err := cmd.CombinedOutput()
		if err != nil {
			fmt.Printf("❌ ERROR: 'ollama rm %s' failed: %v\nOutput: %s\n", importPath, err, string(output))
			failed++
		} else {
			removed++
		}
	}

	fmt.Printf("\n✅ Summary: %d removed, %d failed\n", removed, failed)
	return nil
}

func runForward(ollamaDir, lmstudioDir string, dryRun, verbose bool) {
	fmt.Printf("🔍 Scanning Ollama models in: %s\n", ollamaDir)
	fmt.Printf("🎯 Target LM Studio directory: %s\n", lmstudioDir)
	if dryRun {
		fmt.Println("🧪 DRY RUN MODE - No changes will be made")
	}
	fmt.Println()

	// Discover models
	models, err := ollama.DiscoverModels(ollamaDir, verbose)
	if err != nil {
		log.Fatalf("Error discovering models: %v", err)
	}

	if len(models) == 0 {
		fmt.Println("❌ No models found in Ollama directory")
		return
	}

	fmt.Printf("📦 Found %d models:\n", len(models))
	for _, model := range models {
		fmt.Printf("  • %s\n", model.Name)
	}
	fmt.Println()

	// Create ollama provider directory
	ollamaProviderDir := filepath.Join(lmstudioDir, "ollama")
	if !dryRun {
		if err := os.MkdirAll(ollamaProviderDir, 0755); err != nil {
			log.Fatalf("Error creating ollama provider directory: %v", err)
		}
	}

	// Process each model
	var created, skipped int
	for _, model := range models {
		result := linking.ProcessModel(model, ollamaDir, ollamaProviderDir, dryRun, verbose)
		if result {
			created++
		} else {
			skipped++
		}
	}

	// Summary
	fmt.Println()
	fmt.Printf("✅ Summary: %d created, %d skipped\n", created, skipped)
	if created > 0 && !dryRun {
		fmt.Printf("🎉 Models are now available in LM Studio under the 'ollama' provider\n")
	}
}

func runReverse(lmstudioDir, ollamaDir, namePrefix, skipProvider string, dryRun, verbose bool) {
	fmt.Printf("🔍 Scanning LM Studio models in: %s\n", lmstudioDir)
	fmt.Printf("🎯 Target Ollama directory: %s\n", ollamaDir)
	if dryRun {
		fmt.Println("🧪 DRY RUN MODE - No changes will be made")
	}
	fmt.Println()

	// Discover models
	models, err := lmstudio.DiscoverLMStudioModels(lmstudioDir, skipProvider, verbose)
	if err != nil {
		log.Fatalf("Error discovering models: %v", err)
	}

	if len(models) == 0 {
		fmt.Println("❌ No eligible models found in LM Studio directory")
		return
	}

	fmt.Printf("📦 Found %d eligible models:\n", len(models))
	for _, model := range models {
		fmt.Printf("  • %s (%s)\n", model.Name, model.Path)
	}
	fmt.Println()

	// Process each model
	var created, skipped int
	for _, model := range models {
		result := linking.ProcessLMStudioModel(model, ollamaDir, namePrefix, dryRun, verbose)
		if result {
			created++
		} else {
			skipped++
		}
	}

	// Summary
	fmt.Println()
	fmt.Printf("✅ Summary: %d created, %d skipped\n", created, skipped)
	if created > 0 && !dryRun {
		fmt.Printf("🎉 Models are now available in Ollama with the '%s-' prefix\n", namePrefix)
	}
}

func runCleanup(lmstudioDir string, dryRun, verbose bool, stdin io.Reader) error {
	targetDir := filepath.Join(lmstudioDir, "ollama")
	
	// Check if target dir exists
	if _, err := os.Stat(targetDir); os.IsNotExist(err) {
		fmt.Printf("✅ No managed 'ollama' directory found in LM Studio. Nothing to clean.\n")
		return nil
	}

	fmt.Printf("🔍 Scanning for broken symlinks in: %s\n", targetDir)
	brokenLinks, err := linking.FindBrokenSymlinks(targetDir)
	if err != nil {
		return fmt.Errorf("could not search for broken symlinks: %w", err)
	}

	if len(brokenLinks) == 0 {
		fmt.Printf("✅ No broken symbolic links found.\n")
		return nil
	}

	fmt.Printf("📦 Found %d broken symbolic links (targets are missing):\n", len(brokenLinks))
	for i, link := range brokenLinks {
		fmt.Printf("  %d) %s -> %s\n", i+1, link.Path, link.Target)
	}
	fmt.Println()

	if dryRun {
		fmt.Println("🧪 DRY RUN MODE - No items will be removed.")
		return nil
	}

	fmt.Printf("⚠️  Are you sure you want to delete these %d broken symlinks? (y/n): ", len(brokenLinks))
	var confirm string
	fmt.Fscanln(stdin, &confirm)
	if strings.ToLower(confirm) != "y" {
		fmt.Println("👋 Cancelled")
		return nil
	}

	var paths []string
	for _, link := range brokenLinks {
		paths = append(paths, link.Path)
	}

	removed, failed := linking.RemoveSymlinks(paths, false)
	fmt.Printf("\n✅ Summary: %d removed, %d failed\n", removed, failed)
	
	// Try to remove empty directories
	if removed > 0 {
		if verbose {
			fmt.Println("📂 Cleaning up empty model directories...")
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
							fmt.Printf("  🗑️  Removed empty directory: %s\n", entry.Name())
						}
					}
				}
			}
		}
	}

	return nil
}
