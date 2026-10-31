package main

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/charmbracelet/huh"
	"github.com/qaribhaider/ollama-to-lmstudio-symlinks/internal/lmstudio"
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
		fmt.Printf("  %-12s %s\n", "status", "Show active links, broken symlinks, and disk space savings")
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

	var interactive = fs.Bool("interactive", true, "Interactive mode (main menu and model selection)")
	fs.BoolVar(interactive, "i", true, "Interactive mode (shorthand)")

	var useHardlinks = fs.Bool("hardlinks", false, "Use hard links instead of symlinks (fixes '0 bytes' issue on Windows)")
	var skipChecks = fs.Bool("skip-checks", false, "Skip pre-flight validation checks for Ollama and LM Studio")

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
	if err := validateProviderName(*skipProvider); err != nil {
		return err
	}

	// Simple subcommand handling
	remainingArgs := fs.Args()
	if len(remainingArgs) > 0 && remainingArgs[0] == "status" {
		statusFs := flag.NewFlagSet("status", flag.ContinueOnError)
		statusFs.Usage = func() {
			ui.PrintHeader(fmt.Sprintf("Usage: %s %s [flags]", fs.Name(), statusFs.Name()))
			ui.PrintEmptyLine()

			fmt.Println(ui.HeaderStyle.Render("Flags"))
			statusFs.VisitAll(func(f *flag.Flag) {
				_, usage := flag.UnquoteUsage(f)
				defaultText := ""
				if f.DefValue != "" && f.DefValue != "false" {
					defaultText = ui.MutedStyle.Render(fmt.Sprintf("(default: %q)", f.DefValue))
				}
				fmt.Printf("  %-14s %s %s\n", "--"+f.Name, usage, defaultText)
			})
			ui.PrintEmptyLine()
		}

		statusVerbose := statusFs.Bool("verbose", *verbose, "Enable verbose output to list individual symlinks")
		statusFs.StringVar(lmstudioDir, "lmstudio-dir", *lmstudioDir, "Path to LM Studio models directory")
		statusFs.StringVar(ollamaDir, "ollama-dir", *ollamaDir, "Path to Ollama models directory")
		statusFs.StringVar(skipProvider, "skip-provider", *skipProvider, "Name of provider directory in LM Studio")

		if err := statusFs.Parse(remainingArgs[1:]); err != nil {
			return err
		}

		if len(statusFs.Args()) > 0 {
			statusFs.Usage()
			return fmt.Errorf("unexpected argument: %s", statusFs.Args()[0])
		}

		if err := validateProviderName(*skipProvider); err != nil {
			return err
		}

		return runStatus(*ollamaDir, *lmstudioDir, *skipProvider, *statusVerbose)
	}

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
		deleteSkipChecks := deleteFs.Bool("skip-checks", *skipChecks, "Skip pre-flight validation checks for Ollama and LM Studio")
		deleteNamePrefix := deleteFs.String("name-prefix", *namePrefix, "Prefix for models created in Ollama")

		// Re-bind global flags to deleteFs so they can be parsed after the subcommand
		deleteFs.StringVar(ollamaDir, "ollama-dir", *ollamaDir, "Path to Ollama models directory")
		deleteFs.StringVar(lmstudioDir, "lmstudio-dir", *lmstudioDir, "Path to LM Studio models directory")
		deleteFs.StringVar(skipProvider, "skip-provider", *skipProvider, "Name of provider directory in LM Studio")

		if err := deleteFs.Parse(remainingArgs[1:]); err != nil {
			return err
		}
		if err := validateProviderName(*skipProvider); err != nil {
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

		return runDelete(*from, *ollamaDir, *lmstudioDir, *deleteNamePrefix, *skipProvider, *deleteDryRun, *deleteVerbose, *deleteSkipChecks, stdin)
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
			cleanupSkipChecks := cleanupFs.Bool("skip-checks", *skipChecks, "Skip pre-flight validation checks for Ollama and LM Studio")

			// Re-bind global flags to cleanupFs so they can be parsed after the subcommand
			cleanupFs.StringVar(lmstudioDir, "lmstudio-dir", *lmstudioDir, "Path to LM Studio models directory")
			cleanupFs.StringVar(ollamaDir, "ollama-dir", *ollamaDir, "Path to Ollama models directory")
			cleanupFs.StringVar(skipProvider, "skip-provider", *skipProvider, "Name of provider directory in LM Studio")

			if err := cleanupFs.Parse(remainingArgs[1:]); err != nil {
				return err
			}
			if err := validateProviderName(*skipProvider); err != nil {
				return err
			}

			if len(cleanupFs.Args()) > 0 {
				cleanupFs.Usage()
				return fmt.Errorf("unexpected argument: %s", cleanupFs.Args()[0])
			}

			return runCleanup(*lmstudioDir, *ollamaDir, *skipProvider, *cleanupDryRun, *cleanupVerbose, *cleanupSkipChecks, stdin)
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
		if err := runReverse(*lmstudioDir, *ollamaDir, *namePrefix, *skipProvider, *dryRun, *verbose, *useHardlinks, *interactive, *skipChecks); err != nil {
			return err
		}
	} else {
		if *interactive {
			return runMainMenu(*ollamaDir, *lmstudioDir, *namePrefix, *skipProvider, *dryRun, *verbose, *useHardlinks, *skipChecks, stdin)
		}

		// Check Ollama dir exists
		if _, err := os.Stat(*ollamaDir); os.IsNotExist(err) {
			return fmt.Errorf("Ollama directory does not exist: %s. Use --ollama-dir or --deep-scan to help find it.", *ollamaDir)
		}
		if err := runForward(*ollamaDir, *lmstudioDir, *skipProvider, *dryRun, *verbose, *useHardlinks, false, *skipChecks); err != nil {
			return err
		}
	}

	return nil
}

func runMainMenu(ollamaDir, lmstudioDir, namePrefix, skipProvider string, dryRun, verbose, useHardlinks, skipChecks bool, stdin io.Reader) error {
	var action string
	err := huh.NewSelect[string]().
		Title("What would you like to do?").
		Options(
			huh.NewOption("View status and storage savings", "status"),
			huh.NewOption("Link Ollama models to LM Studio", "forward"),
			huh.NewOption("Link LM Studio models to Ollama (Reverse)", "reverse"),
			huh.NewOption("Delete existing symlinks", "delete"),
			huh.NewOption("Cleanup broken symlinks", "cleanup"),
			huh.NewOption("Exit", "exit"),
		).
		Value(&action).
		Run()

	if err != nil {
		if errors.Is(err, huh.ErrUserAborted) {
			return nil
		}
		return err
	}

	switch action {
	case "status":
		return runStatus(ollamaDir, lmstudioDir, skipProvider, verbose)
	case "forward":
		// Check Ollama dir exists
		if _, err := os.Stat(ollamaDir); os.IsNotExist(err) {
			return fmt.Errorf("Ollama directory does not exist: %s. Use --ollama-dir or --deep-scan to help find it.", ollamaDir)
		}
		return runForward(ollamaDir, lmstudioDir, skipProvider, dryRun, verbose, useHardlinks, true, skipChecks)
	case "reverse":
		// Check LM Studio dir exists
		if _, err := os.Stat(lmstudioDir); os.IsNotExist(err) {
			return fmt.Errorf("LM Studio directory does not exist: %s. Use --lmstudio-dir or --deep-scan to help find it.", lmstudioDir)
		}
		return runReverse(lmstudioDir, ollamaDir, namePrefix, skipProvider, dryRun, verbose, useHardlinks, true, skipChecks)
	case "delete":
		var from string
		err := huh.NewSelect[string]().
			Title("Delete symlinks from where?").
			Options(
				huh.NewOption("From Ollama", "ollama"),
				huh.NewOption("From LM Studio", "lmstudio"),
			).
			Value(&from).
			Run()
		if err != nil {
			if errors.Is(err, huh.ErrUserAborted) {
				return nil
			}
			return err
		}
		return runDelete(from, ollamaDir, lmstudioDir, namePrefix, skipProvider, dryRun, verbose, skipChecks, stdin)
	case "cleanup":
		return runCleanup(lmstudioDir, ollamaDir, skipProvider, dryRun, verbose, skipChecks, stdin)
	}

	return nil
}

// validateProviderName rejects values that would point the LM Studio provider
// directory outside its own folder (empty, ".", "..", or containing a separator),
// since delete and cleanup operate on everything inside it.
func validateProviderName(name string) error {
	if name == "" || name == "." || name == ".." || strings.ContainsAny(name, `/\`) {
		return fmt.Errorf("--skip-provider must be a plain directory name, got %q", name)
	}
	return nil
}
