package linking

import (
	"fmt"
	"os"
	"path/filepath"
)

// DirStatus holds statistics for a managed model symlink directory.
type DirStatus struct {
	Path        string
	Exists      bool
	TotalLinks  int
	ActiveLinks int
	BrokenLinks int
	BytesSaved  int64
	Files       []SymlinkDetail
}

// SymlinkDetail contains details of an individual managed symlink.
type SymlinkDetail struct {
	Name       string
	Path       string
	Target     string
	IsBroken   bool
	TargetSize int64
}

// InspectDirectory analyzes symlinks in a directory (e.g. lmstudio/ollama or ollama/blobs).
func InspectDirectory(dir string) (DirStatus, error) {
	status := DirStatus{
		Path: dir,
	}

	info, err := os.Stat(dir)
	if os.IsNotExist(err) || (err == nil && !info.IsDir()) {
		return status, nil
	}
	if err != nil {
		return status, err
	}
	status.Exists = true

	countedTargets := make(map[string]bool)

	err = filepath.WalkDir(dir, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}

		if d.Type()&os.ModeSymlink != 0 {
			status.TotalLinks++
			target, readErr := os.Readlink(path)
			detail := SymlinkDetail{
				Name: d.Name(),
				Path: path,
			}
			if readErr != nil {
				detail.IsBroken = true
				status.BrokenLinks++
				status.Files = append(status.Files, detail)
				return nil
			}

			detail.Target = target
			targetPath := target
			if !filepath.IsAbs(targetPath) {
				targetPath = filepath.Join(filepath.Dir(path), target)
			}

			targetInfo, statErr := os.Stat(targetPath)
			if statErr != nil {
				detail.IsBroken = true
				status.BrokenLinks++
			} else {
				status.ActiveLinks++
				detail.TargetSize = targetInfo.Size()
				canonicalTarget := filepath.Clean(targetPath)
				if !countedTargets[canonicalTarget] {
					countedTargets[canonicalTarget] = true
					status.BytesSaved += targetInfo.Size()
				}
			}
			status.Files = append(status.Files, detail)
		}
		return nil
	})

	return status, err
}

// FormatBytes formats byte counts into human-readable string (e.g., 1.45 GB).
func FormatBytes(b int64) string {
	const unit = 1024
	if b < unit {
		return fmt.Sprintf("%d B", b)
	}
	div, exp := int64(unit), 0
	for n := b / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.2f %cB", float64(b)/float64(div), "KMGTPE"[exp])
}
