package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRunApp_Validation(t *testing.T) {
	tempDir := t.TempDir()
	
	// Create a dummy dir that exists
	existsDir := filepath.Join(tempDir, "exists")
	os.Mkdir(existsDir, 0755)

	tests := []struct {
		name        string
		args        []string
		wantErr     bool
		errContains string
	}{
		{
			name:        "Empty name prefix",
			args:        []string{"--name-prefix", ""},
			wantErr:     true,
			errContains: "--name-prefix cannot be empty",
		},
		{
			name:        "Non-existent ollama-dir (Forward)",
			args:        []string{"--ollama-dir", filepath.Join(tempDir, "non-existent")},
			wantErr:     true,
			errContains: "Ollama directory does not exist",
		},
		{
			name:        "Non-existent lmstudio-dir (Reverse)",
			args:        []string{"--reverse", "--lmstudio-dir", filepath.Join(tempDir, "non-existent")},
			wantErr:     true,
			errContains: "LM Studio directory does not exist",
		},
		{
			name:        "Delete without from",
			args:        []string{"--ollama-dir", existsDir, "delete"},
			wantErr:     true,
			errContains: "--from flag is required",
		},
		{
			name:        "Delete with invalid from",
			args:        []string{"--ollama-dir", existsDir, "delete", "--from", "invalid"},
			wantErr:     true,
			errContains: "invalid --from value",
		},
		{
			name:        "Delete with non-existent target dir",
			args:        []string{"--ollama-dir", filepath.Join(tempDir, "non-existent"), "delete", "--from", "ollama"},
			wantErr:     true,
			errContains: "directory for deletion does not exist",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Using strings.NewReader("") as dummy stdin
			err := runApp(tt.args, strings.NewReader(""))
			if (err != nil) != tt.wantErr {
				t.Errorf("runApp() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if tt.wantErr && !strings.Contains(err.Error(), tt.errContains) {
				t.Errorf("runApp() error = %v, wantContains %v", err, tt.errContains)
			}
		})
	}
}

func TestRunApp_Version(t *testing.T) {
	err := runApp([]string{"--version"}, strings.NewReader(""))
	if err != nil {
		t.Errorf("runApp(--version) error = %v", err)
	}
}
