package linking

import (
	"os"
	"path/filepath"
	"testing"
)

func TestFormatBytes(t *testing.T) {
	tests := []struct {
		input    int64
		expected string
	}{
		{500, "500 B"},
		{1024, "1.00 KB"},
		{1536, "1.50 KB"},
		{1048576, "1.00 MB"},
		{1073741824, "1.00 GB"},
		{5368709120, "5.00 GB"},
	}

	for _, tt := range tests {
		got := FormatBytes(tt.input)
		if got != tt.expected {
			t.Errorf("FormatBytes(%d) = %s, want %s", tt.input, got, tt.expected)
		}
	}
}

func TestInspectDirectory(t *testing.T) {
	tempDir := t.TempDir()

	// Non-existent directory
	st, err := InspectDirectory(filepath.Join(tempDir, "does-not-exist"))
	if err != nil {
		t.Fatalf("unexpected error on non-existent dir: %v", err)
	}
	if st.Exists {
		t.Errorf("expected Exists=false, got true")
	}

	// Create real target files
	targetDir := filepath.Join(tempDir, "targets")
	os.MkdirAll(targetDir, 0755)

	target1 := filepath.Join(targetDir, "model1.gguf")
	target2 := filepath.Join(targetDir, "model2.gguf")
	os.WriteFile(target1, make([]byte, 1024*1024), 0644)       // 1MB
	os.WriteFile(target2, make([]byte, 2*1024*1024), 0644)     // 2MB

	// Directory to inspect
	inspectDir := filepath.Join(tempDir, "managed")
	os.MkdirAll(inspectDir, 0755)

	// Symlink 1: active link to target1
	link1 := filepath.Join(inspectDir, "link1.gguf")
	if err := os.Symlink(target1, link1); err != nil {
		t.Fatal(err)
	}

	// Symlink 2: active link to target2
	link2 := filepath.Join(inspectDir, "link2.gguf")
	if err := os.Symlink(target2, link2); err != nil {
		t.Fatal(err)
	}

	// Symlink 3: duplicate link to target1 (deduplication check)
	link3 := filepath.Join(inspectDir, "link3.gguf")
	if err := os.Symlink(target1, link3); err != nil {
		t.Fatal(err)
	}

	// Symlink 4: broken symlink (points to missing file)
	linkBroken := filepath.Join(inspectDir, "broken.gguf")
	if err := os.Symlink(filepath.Join(targetDir, "missing.gguf"), linkBroken); err != nil {
		t.Fatal(err)
	}

	st, err = InspectDirectory(inspectDir)
	if err != nil {
		t.Fatalf("InspectDirectory failed: %v", err)
	}

	if !st.Exists {
		t.Errorf("expected Exists=true")
	}
	if st.TotalLinks != 4 {
		t.Errorf("expected TotalLinks=4, got %d", st.TotalLinks)
	}
	if st.ActiveLinks != 3 {
		t.Errorf("expected ActiveLinks=3, got %d", st.ActiveLinks)
	}
	if st.BrokenLinks != 1 {
		t.Errorf("expected BrokenLinks=1, got %d", st.BrokenLinks)
	}

	// BytesSaved should count target1 (1MB) + target2 (2MB) once = 3MB
	expectedBytes := int64(3 * 1024 * 1024)
	if st.BytesSaved != expectedBytes {
		t.Errorf("expected BytesSaved=%d, got %d", expectedBytes, st.BytesSaved)
	}
}
