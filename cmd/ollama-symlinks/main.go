package main

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"strings"

	"github.com/qaribhaider/ollama-to-lmstudio-symlinks/internal/linking"
	"github.com/qaribhaider/ollama-to-lmstudio-symlinks/internal/lmstudio"
	"github.com/qaribhaider/ollama-to-lmstudio-symlinks/internal/ollama"
)

// Version of the application
var Version = "dev"

func main() {
	if err := runApp(os.Args[1:], os.Stdin); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}

func runApp(args []string, stdin io.Reader) error {
	fs := flag.NewFlagSet("ollama-symlinks", flag.ContinueOnError)

	// Command line flags
	var ollamaDir = fs.String("ollama-dir", ollama.GetDefaultOllamaDir(), "Path to Ollama models directory")
	var lmstudioDir = fs.String("lmstudio-dir", lmstudio.GetDefaultLMStudioDir(), "Path to LM Studio models directory")
	var dryRun = fs.Bool("dry-run", false, "Show what would be done without actually creating symlinks")
	var verbose = fs.Bool("verbose", false, "Enable verbose output")
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

	if *reverse {
		// Check LM Studio dir exists
		if _, err := os.Stat(*lmstudioDir); os.IsNotExist(err) {
			return fmt.Errorf("LM Studio directory does not exist: %s", *lmstudioDir)
		}
		runReverse(*lmstudioDir, *ollamaDir, *namePrefix, *skipProvider, *dryRun, *verbose)
	} else {
		// Check Ollama dir exists
		if _, err := os.Stat(*ollamaDir); os.IsNotExist(err) {
			return fmt.Errorf("Ollama directory does not exist: %s", *ollamaDir)
		}
		runForward(*ollamaDir, *lmstudioDir, *dryRun, *verbose)
	}

	return nil
}

func runDelete(from, ollamaDir, lmstudioDir, skipProvider string, dryRun, verbose bool, stdin io.Reader) error {
	var targetDir string
	if from == "ollama" {
		targetDir = filepath.Join(ollamaDir, "blobs")
	} else if from == "lmstudio" {
		targetDir = filepath.Join(lmstudioDir, skipProvider)
	}

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
