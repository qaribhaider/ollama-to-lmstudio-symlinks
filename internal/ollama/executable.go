package ollama

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"

	"github.com/qaribhaider/ollama-to-lmstudio-symlinks/internal/ui"
)

var (
	// LookPathFunc allows mocking exec.LookPath in unit tests.
	LookPathFunc = exec.LookPath
	// PlatformPathsFunc allows overriding standard platform candidate paths in unit tests.
	PlatformPathsFunc = getPlatformOllamaPaths
)

// GetOllamaExecutable searches for the ollama binary in PATH and common standard locations.
func GetOllamaExecutable() (string, error) {
	// 1. Check system PATH
	if path, err := LookPathFunc("ollama"); err == nil && path != "" {
		return path, nil
	}
	if runtime.GOOS == "windows" {
		if path, err := LookPathFunc("ollama.exe"); err == nil && path != "" {
			return path, nil
		}
	}

	// 2. Check standard platform-specific paths
	candidates := PlatformPathsFunc()
	for _, c := range candidates {
		if info, err := os.Stat(c); err == nil && !info.IsDir() {
			return c, nil
		}
	}

	return "", errors.New("'ollama' executable not found in PATH or standard installation directories")
}

// getPlatformOllamaPaths returns standard installation paths based on operating system.
func getPlatformOllamaPaths() []string {
	var paths []string
	home, _ := os.UserHomeDir()

	switch runtime.GOOS {
	case "windows":
		if local := os.Getenv("LOCALAPPDATA"); local != "" {
			paths = append(paths, filepath.Join(local, "Programs", "Ollama", "ollama.exe"))
		}
		if prog := os.Getenv("ProgramFiles"); prog != "" {
			paths = append(paths, filepath.Join(prog, "Ollama", "ollama.exe"))
		}
		if home != "" {
			paths = append(paths, filepath.Join(home, "AppData", "Local", "Programs", "Ollama", "ollama.exe"))
		}
	case "darwin":
		paths = append(paths,
			"/opt/homebrew/bin/ollama",
			"/usr/local/bin/ollama",
			"/Applications/Ollama.app/Contents/Resources/ollama",
		)
		if home != "" {
			paths = append(paths, filepath.Join(home, "Applications", "Ollama.app", "Contents", "Resources", "ollama"))
		}
	case "linux":
		paths = append(paths,
			"/usr/local/bin/ollama",
			"/usr/bin/ollama",
			"/bin/ollama",
			"/snap/bin/ollama",
		)
		if home != "" {
			paths = append(paths, filepath.Join(home, ".nix-profile", "bin", "ollama"))
		}
	}

	return paths
}

// ValidateOllama checks whether the Ollama CLI is accessible.
// If missing and skipChecks is false, returns a blocking error with user guidance.
// If missing and skipChecks is true, displays a warning and returns fallback "ollama".
func ValidateOllama(skipChecks bool) (string, error) {
	exePath, err := GetOllamaExecutable()
	if err != nil {
		if skipChecks {
			ui.PrintWarning("Warning: 'ollama' executable not found in PATH or standard locations.")
			ui.PrintWarning("Proceeding anyway because --skip-checks was provided.")
			return "ollama", nil
		}
		return "", fmt.Errorf(
			"blocking validation error: 'ollama' executable not found in PATH or standard installation directories\n" +
				"  • Ollama is required to register and manage models.\n" +
				"  • Download & install: https://ollama.com/download\n" +
				"  • If already installed, ensure it is added to your PATH, or run with --skip-checks to bypass",
		)
	}
	return exePath, nil
}
