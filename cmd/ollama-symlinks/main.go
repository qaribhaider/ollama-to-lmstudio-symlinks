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

// options holds the flag values shared by every command. Subcommands receive a
// copy, so flags parsed after the subcommand name do not leak back.
type options struct {
	ollamaDir, lmstudioDir, namePrefix, skipProvider string

	dryRun, verbose, deepScan, interactive, useHardlinks, skipChecks, reverse bool
}

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
	var o options
	var showVersion bool
	bindGlobalFlags(fs, &o, &showVersion)
	fs.Usage = func() { printUsage(fs) }

	if err := fs.Parse(args); err != nil {
		return err
	}

	if showVersion {
		fmt.Printf("ollama-symlinks version %s\n", Version)
		return nil
	}

	if o.namePrefix == "" {
		return errors.New("--name-prefix cannot be empty")
	}
	if err := validateProviderName(o.skipProvider); err != nil {
		return err
	}

	if rest := fs.Args(); len(rest) > 0 {
		return runSubcommand(fs.Name(), rest, o, stdin)
	}

	o.ollamaDir = resolveDir(o.ollamaDir, ollama.GetDefaultOllamaDir(), "Ollama", "ollama-dir", ollama.GetOllamaCandidates, ollama.ScanAllDrives, o)
	o.lmstudioDir = resolveDir(o.lmstudioDir, lmstudio.GetDefaultLMStudioDir(), "LM Studio", "lmstudio-dir", lmstudio.GetLMStudioCandidates, lmstudio.ScanAllDrives, o)

	return runDefault(o, stdin)
}

func bindGlobalFlags(fs *flag.FlagSet, o *options, showVersion *bool) {
	fs.StringVar(&o.ollamaDir, "ollama-dir", ollama.GetDefaultOllamaDir(), "Path to Ollama models directory")
	fs.StringVar(&o.lmstudioDir, "lmstudio-dir", lmstudio.GetDefaultLMStudioDir(), "Path to LM Studio models directory")
	fs.BoolVar(&o.dryRun, "dry-run", false, "Show what would be done without actually creating symlinks")
	fs.BoolVar(&o.verbose, "verbose", false, "Enable verbose output")
	fs.BoolVar(&o.deepScan, "deep-scan", false, "Scan all Windows drives for model directories (fallback)")
	fs.BoolVar(showVersion, "version", false, "Show version information")

	fs.BoolVar(&o.interactive, "interactive", true, "Interactive mode (main menu and model selection)")
	fs.BoolVar(&o.interactive, "i", true, "Interactive mode (shorthand)")

	fs.BoolVar(&o.useHardlinks, "hardlinks", false, "Use hard links instead of symlinks (fixes '0 bytes' issue on Windows)")
	fs.BoolVar(&o.skipChecks, "skip-checks", false, "Skip pre-flight validation checks for Ollama and LM Studio")

	// Reverse mode flags
	fs.BoolVar(&o.reverse, "reverse", false, "Link LM Studio models to Ollama (reverse mode)")
	fs.StringVar(&o.namePrefix, "name-prefix", "lms", "Prefix for models created in Ollama")
	fs.StringVar(&o.skipProvider, "skip-provider", "ollama", "Directory name to skip in LM Studio (to avoid circular links)")
}

func printUsage(fs *flag.FlagSet) {
	ui.PrintHeader(fmt.Sprintf("Usage: %s [command] [flags]", fs.Name()))
	ui.PrintEmptyLine()

	fmt.Println(ui.HeaderStyle.Render("Commands"))
	fmt.Printf("  %-12s %s\n", "status", "Show active links, broken symlinks, and disk space savings")
	fmt.Printf("  %-12s %s\n", "delete", "Interactively delete symlinks from Ollama or LM Studio")
	fmt.Printf("  %-12s %s\n", "cleanup", "Auto-discover and remove broken symlinks")
	ui.PrintEmptyLine()

	fmt.Println(ui.HeaderStyle.Render("Flags"))
	printFlagUsage(fs, 18)
	ui.PrintEmptyLine()
}

func printFlagUsage(fs *flag.FlagSet, nameWidth int) {
	fs.VisitAll(func(f *flag.Flag) {
		_, usage := flag.UnquoteUsage(f)
		defaultText := ""
		if f.DefValue != "" && f.DefValue != "false" {
			defaultText = ui.MutedStyle.Render(fmt.Sprintf("(default: %q)", f.DefValue))
		}
		fmt.Printf("  %-*s %s %s\n", nameWidth, "--"+f.Name, usage, defaultText)
	})
}

func newSubcommandFlagSet(prog, name string) *flag.FlagSet {
	fs := flag.NewFlagSet(name, flag.ContinueOnError)
	fs.Usage = func() {
		ui.PrintHeader(fmt.Sprintf("Usage: %s %s [flags]", prog, name))
		ui.PrintEmptyLine()

		fmt.Println(ui.HeaderStyle.Render("Flags"))
		printFlagUsage(fs, 14)
		ui.PrintEmptyLine()
	}
	return fs
}

// bindLocationFlags re-binds the directory flags so they can also be written
// after the subcommand name.
func bindLocationFlags(fs *flag.FlagSet, o *options) {
	fs.StringVar(&o.lmstudioDir, "lmstudio-dir", o.lmstudioDir, "Path to LM Studio models directory")
	fs.StringVar(&o.ollamaDir, "ollama-dir", o.ollamaDir, "Path to Ollama models directory")
	fs.StringVar(&o.skipProvider, "skip-provider", o.skipProvider, "Name of provider directory in LM Studio")
}

func parseSubcommandFlags(fs *flag.FlagSet, args []string, o *options) error {
	if err := fs.Parse(args); err != nil {
		return err
	}
	if len(fs.Args()) > 0 {
		fs.Usage()
		return fmt.Errorf("unexpected argument: %s", fs.Args()[0])
	}
	return validateProviderName(o.skipProvider)
}

func runSubcommand(prog string, args []string, o options, stdin io.Reader) error {
	switch args[0] {
	case "status":
		return runStatusCommand(prog, args[1:], o)
	case "delete":
		return runDeleteCommand(prog, args[1:], o, stdin)
	case "cleanup":
		return runCleanupCommand(prog, args[1:], o, stdin)
	}
	return fmt.Errorf("unknown command %q. Run '%s --help' for usage", args[0], prog)
}

func runStatusCommand(prog string, args []string, o options) error {
	fs := newSubcommandFlagSet(prog, "status")
	fs.BoolVar(&o.verbose, "verbose", o.verbose, "Enable verbose output to list individual symlinks")
	bindLocationFlags(fs, &o)

	if err := parseSubcommandFlags(fs, args, &o); err != nil {
		return err
	}
	return runStatus(o)
}

func runDeleteCommand(prog string, args []string, o options, stdin io.Reader) error {
	fs := newSubcommandFlagSet(prog, "delete")
	from := fs.String("from", "", "Source to delete symlinks from (ollama or lmstudio)")
	fs.BoolVar(&o.dryRun, "dry-run", o.dryRun, "Show what would be deleted without actually removing them")
	fs.BoolVar(&o.verbose, "verbose", o.verbose, "Enable verbose output")
	fs.BoolVar(&o.skipChecks, "skip-checks", o.skipChecks, "Skip pre-flight validation checks for Ollama and LM Studio")
	fs.StringVar(&o.namePrefix, "name-prefix", o.namePrefix, "Prefix for models created in Ollama")
	bindLocationFlags(fs, &o)

	if err := parseSubcommandFlags(fs, args, &o); err != nil {
		return err
	}

	if *from == "" {
		fs.Usage()
		return errors.New("--from flag is required for delete command (ollama or lmstudio)")
	}
	if *from != "ollama" && *from != "lmstudio" {
		return fmt.Errorf("invalid --from value: %s (must be 'ollama' or 'lmstudio')", *from)
	}
	return runDelete(*from, o, stdin)
}

func runCleanupCommand(prog string, args []string, o options, stdin io.Reader) error {
	fs := newSubcommandFlagSet(prog, "cleanup")
	fs.BoolVar(&o.dryRun, "dry-run", o.dryRun, "Show what would be deleted without actually removing them")
	fs.BoolVar(&o.verbose, "verbose", o.verbose, "Enable verbose output")
	fs.BoolVar(&o.skipChecks, "skip-checks", o.skipChecks, "Skip pre-flight validation checks for Ollama and LM Studio")
	bindLocationFlags(fs, &o)

	if err := parseSubcommandFlags(fs, args, &o); err != nil {
		return err
	}
	return runCleanup(o, stdin)
}

// resolveDir returns dir unchanged when the user set it explicitly; otherwise it
// picks the first existing candidate directory, optionally scanning all drives.
func resolveDir(dir, defaultDir, label, flagName string, candidates, scanDrives func() []string, o options) string {
	if dir != defaultDir {
		return dir
	}

	found := candidates()
	if o.deepScan && len(found) == 0 {
		if o.verbose {
			fmt.Printf("🔎 Scanning all Windows drives for %s models...\n", label)
		}
		found = scanDrives()
	}
	if len(found) > 1 {
		fmt.Printf("📂 Found multiple %s model directories:\n", label)
		for i, c := range found {
			fmt.Printf("  [%d] %s\n", i+1, c)
		}
		fmt.Printf("Using the first one. Use --%s to override.\n\n", flagName)
	}
	if len(found) > 0 {
		return found[0]
	}
	return dir
}

func requireDir(path, label, flagName string) error {
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return fmt.Errorf("%s directory does not exist: %s (use --%s or --deep-scan to find it)", label, path, flagName)
	}
	return nil
}

// runDefault handles the no-subcommand case: reverse linking, the interactive
// menu, or forward linking.
func runDefault(o options, stdin io.Reader) error {
	if o.reverse {
		if err := requireDir(o.lmstudioDir, "LM Studio", "lmstudio-dir"); err != nil {
			return err
		}
		return runReverse(o)
	}

	if o.interactive {
		return runMainMenu(o, stdin)
	}

	if err := requireDir(o.ollamaDir, "Ollama", "ollama-dir"); err != nil {
		return err
	}
	return runForward(o)
}

func runMainMenu(o options, stdin io.Reader) error {
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

	o.interactive = true
	switch action {
	case "status":
		return runStatus(o)
	case "forward":
		if err := requireDir(o.ollamaDir, "Ollama", "ollama-dir"); err != nil {
			return err
		}
		return runForward(o)
	case "reverse":
		if err := requireDir(o.lmstudioDir, "LM Studio", "lmstudio-dir"); err != nil {
			return err
		}
		return runReverse(o)
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
		return runDelete(from, o, stdin)
	case "cleanup":
		return runCleanup(o, stdin)
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
