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

	counted := make(map[string]bool)

	err = filepath.WalkDir(dir, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}

		switch {
		case d.Type()&os.ModeSymlink != 0:
			status.addSymlink(path, d.Name(), counted)
		case d.Type().IsRegular():
			status.addHardlink(path, d.Name(), counted)
		}
		return nil
	})

	return status, err
}

// addSymlink records one symlink. counted holds the files whose size has
// already been credited, so many links to one file save its size once.
func (s *DirStatus) addSymlink(path, name string, counted map[string]bool) {
	s.TotalLinks++
	detail := SymlinkDetail{Name: name, Path: path}

	target, err := os.Readlink(path)
	if err != nil {
		detail.IsBroken = true
		s.BrokenLinks++
		s.Files = append(s.Files, detail)
		return
	}
	detail.Target = target

	targetPath := target
	if !filepath.IsAbs(targetPath) {
		targetPath = filepath.Join(filepath.Dir(path), target)
	}

	targetInfo, err := os.Stat(targetPath)
	if err != nil {
		detail.IsBroken = true
		s.BrokenLinks++
		s.Files = append(s.Files, detail)
		return
	}

	s.ActiveLinks++
	detail.TargetSize = targetInfo.Size()

	// Resolve the whole chain so two links reaching one file count once.
	canonicalTarget := filepath.Clean(targetPath)
	if resolved, err := filepath.EvalSymlinks(path); err == nil {
		canonicalTarget = resolved
	}
	s.creditSavings(canonicalTarget, targetInfo.Size(), counted)
	s.Files = append(s.Files, detail)
}

// addHardlink records a regular file that shares its data with another path.
// Files with a single link are original data and are ignored.
func (s *DirStatus) addHardlink(path, name string, counted map[string]bool) {
	info, err := os.Stat(path)
	if err != nil {
		return
	}
	isHardlink, id := getHardlinkInfo(path, info)
	if !isHardlink {
		return
	}

	s.TotalLinks++
	s.ActiveLinks++
	s.creditSavings(id, info.Size(), counted)
	s.Files = append(s.Files, SymlinkDetail{
		Name:       name,
		Path:       path,
		Target:     "(hardlink)",
		TargetSize: info.Size(),
	})
}

func (s *DirStatus) creditSavings(key string, size int64, counted map[string]bool) {
	if counted[key] {
		return
	}
	counted[key] = true
	s.BytesSaved += size
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
