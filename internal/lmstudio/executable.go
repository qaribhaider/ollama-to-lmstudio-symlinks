package lmstudio

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
	PlatformPathsFunc = getPlatformLMStudioPaths
	// CandidatesFunc allows overriding candidate directory discovery in unit tests.
	CandidatesFunc = GetLMStudioCandidates
)

// GetLMStudioExecutable searches for the LM Studio CLI (lms) or desktop application in PATH and standard locations.
func GetLMStudioExecutable() (string, error) {
	// 1. Check system PATH for 'lms' CLI
	if path, err := LookPathFunc("lms"); err == nil && path != "" {
		return path, nil
	}
	if runtime.GOOS == "windows" {
		if path, err := LookPathFunc("lms.exe"); err == nil && path != "" {
			return path, nil
		}
	}

	// 2. Check standard platform-specific paths
	candidates := PlatformPathsFunc()
	for _, c := range candidates {
		if info, err := os.Stat(c); err == nil {
			// On macOS, .app is a directory but valid application bundle
			if runtime.GOOS == "darwin" && filepath.Ext(c) == ".app" && info.IsDir() {
				return c, nil
			}
			if !info.IsDir() {
				return c, nil
			}
		}
	}

	return "", errors.New("LM Studio CLI or application not found in PATH or standard installation directories")
}

// getPlatformLMStudioPaths returns common paths for the lms CLI and desktop app.
func getPlatformLMStudioPaths() []string {
	var paths []string
	home, _ := os.UserHomeDir()

	switch runtime.GOOS {
	case "windows":
		if home != "" {
			paths = append(paths,
				filepath.Join(home, ".cache", "lm-studio", "bin", "lms.exe"),
				filepath.Join(home, ".lmstudio", "bin", "lms.exe"),
			)
		}
		if local := os.Getenv("LOCALAPPDATA"); local != "" {
			paths = append(paths,
				filepath.Join(local, "Programs", "LM Studio", "LM Studio.exe"),
				filepath.Join(local, "LM-Studio", "LM Studio.exe"),
			)
		}
		if prog := os.Getenv("ProgramFiles"); prog != "" {
			paths = append(paths, filepath.Join(prog, "LM Studio", "LM Studio.exe"))
		}
	case "darwin":
		paths = append(paths,
			"/usr/local/bin/lms",
			"/Applications/LM Studio.app",
		)
		if home != "" {
			paths = append(paths,
				filepath.Join(home, ".cache", "lm-studio", "bin", "lms"),
				filepath.Join(home, ".lmstudio", "bin", "lms"),
				filepath.Join(home, "Applications", "LM Studio.app"),
			)
		}
	case "linux":
		paths = append(paths,
			"/usr/local/bin/lms",
			"/usr/bin/lm-studio",
			"/usr/local/bin/lm-studio",
		)
		if home != "" {
			paths = append(paths,
				filepath.Join(home, ".cache", "lm-studio", "bin", "lms"),
				filepath.Join(home, ".lmstudio", "bin", "lms"),
				filepath.Join(home, ".local", "bin", "lm-studio"),
			)
		}
	}

	return paths
}

// IsLMStudioInstalled checks if LM Studio is present on the system via CLI, desktop app, or model directories.
func IsLMStudioInstalled(targetDir string) bool {
	// Check CLI or application
	if _, err := GetLMStudioExecutable(); err == nil {
		return true
	}

	// Check existing candidate model directories
	if len(CandidatesFunc()) > 0 {
		return true
	}

	// Check if the specified targetDir actually exists on disk
	if targetDir != "" {
		if info, err := os.Stat(targetDir); err == nil && info.IsDir() {
			return true
		}
	}

	return false
}

// ValidateLMStudio checks whether LM Studio is detected.
// If missing and skipChecks is false, returns a blocking error with guidance.
// If missing and skipChecks is true, displays a warning and proceeds.
func ValidateLMStudio(skipChecks bool, targetDir string) error {
	if IsLMStudioInstalled(targetDir) {
		return nil
	}

	if skipChecks {
		ui.PrintWarning("Warning: LM Studio installation or models directory was not detected.")
		ui.PrintWarning("Proceeding anyway because --skip-checks was provided.")
		return nil
	}

	return fmt.Errorf(
		"blocking validation error: LM Studio installation or models directory not found\n" +
			"  • LM Studio is required for this operation.\n" +
			"  • Download & install: https://lmstudio.ai\n" +
			"  • If using a custom directory, specify --lmstudio-dir, or run with --skip-checks to bypass",
	)
}
