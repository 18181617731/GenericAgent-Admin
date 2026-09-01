package api

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// Cloning a GA root is deliberately bounded. A source root can contain a
// virtual environment, caches, or accidentally mounted data; none of those
// should make an Admin request unbounded or copy files outside the project.
const (
	instanceCloneMaxBytes   = int64(2 << 30)
	instanceCloneMaxEntries = 100000
)

func cloneGenericAgentProject(src, dest string, copyMemory, copyMyKey bool) (err error) {
	rawSrc := strings.TrimSpace(src)
	rawDest := strings.TrimSpace(dest)
	if rawSrc == "" || rawDest == "" {
		return fmt.Errorf("source and destination directories are required")
	}
	src, err = filepath.Abs(filepath.Clean(rawSrc))
	if err != nil {
		return err
	}
	dest, err = filepath.Abs(filepath.Clean(rawDest))
	if err != nil {
		return err
	}
	if sameRuntimePath(src, dest) {
		return fmt.Errorf("source and destination must be different directories")
	}
	if pathsOverlap(src, dest) {
		return fmt.Errorf("source and destination directories must not contain one another")
	}

	sourceInfo, err := os.Stat(src)
	if err != nil {
		return fmt.Errorf("source instance root is unavailable: %w", err)
	}
	if !sourceInfo.IsDir() {
		return fmt.Errorf("source instance root is not a directory")
	}
	if _, err := os.Lstat(dest); err == nil {
		return fmt.Errorf("destination instance root already exists")
	} else if !os.IsNotExist(err) {
		return fmt.Errorf("check destination instance root: %w", err)
	}

	if err := os.MkdirAll(dest, 0755); err != nil {
		return fmt.Errorf("create destination instance root: %w", err)
	}
	keep := false
	defer func() {
		if !keep {
			_ = os.RemoveAll(dest)
		}
	}()

	var entries int
	var total int64
	err = filepath.WalkDir(src, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		if rel == "." {
			return nil
		}
		if entry.Type()&os.ModeSymlink != 0 {
			if entry.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if entry.IsDir() {
			if skipCloneDirectory(rel, copyMemory) {
				return filepath.SkipDir
			}
			info, err := entry.Info()
			if err != nil {
				return err
			}
			target := filepath.Join(dest, rel)
			return os.MkdirAll(target, info.Mode().Perm())
		}
		if skipCloneFile(rel, copyMyKey) {
			return nil
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		if !info.Mode().IsRegular() {
			return fmt.Errorf("unsupported source entry: %s", rel)
		}
		entries++
		if entries > instanceCloneMaxEntries {
			return fmt.Errorf("source instance has too many files")
		}
		if info.Size() < 0 || info.Size() > instanceCloneMaxBytes-total {
			return fmt.Errorf("source instance exceeds 2 GiB")
		}
		total += info.Size()

		target := filepath.Join(dest, rel)
		if err := os.MkdirAll(filepath.Dir(target), 0755); err != nil {
			return err
		}
		in, err := os.Open(path)
		if err != nil {
			return err
		}
		out, err := os.OpenFile(target, os.O_CREATE|os.O_WRONLY|os.O_EXCL, info.Mode().Perm())
		if err != nil {
			_ = in.Close()
			return err
		}
		written, copyErr := io.Copy(out, io.LimitReader(in, info.Size()+1))
		closeOutErr := out.Close()
		closeInErr := in.Close()
		if copyErr != nil {
			return copyErr
		}
		if closeOutErr != nil {
			return closeOutErr
		}
		if closeInErr != nil {
			return closeInErr
		}
		if written != info.Size() {
			return fmt.Errorf("source file changed while copying: %s", rel)
		}
		return nil
	})
	if err != nil {
		return fmt.Errorf("clone project: %w", err)
	}
	keep = true
	return nil
}

func skipCloneDirectory(rel string, copyMemory bool) bool {
	parts := splitClonePath(rel)
	if len(parts) == 0 {
		return false
	}
	first := strings.ToLower(parts[0])
	if first == "memory" {
		return !copyMemory
	}
	if first == "temp" {
		if len(parts) == 1 {
			return false
		}
		if parts[1] == "autonomous" && len(parts) > 2 && strings.EqualFold(parts[2], "control") {
			return true
		}
		return parts[1] != "autonomous" && parts[1] != "autonomous_reports" && parts[1] != "projects"
	}
	switch first {
	case ".git", ".venv", "venv", "__pycache__", ".pytest_cache", ".mypy_cache", ".ruff_cache", "node_modules":
		return true
	default:
		return false
	}
}

func skipCloneFile(rel string, copyMyKey bool) bool {
	parts := splitClonePath(rel)
	if len(parts) == 0 {
		return true
	}
	base := strings.ToLower(parts[len(parts)-1])
	if strings.EqualFold(parts[0], "temp") {
		if len(parts) == 2 {
			switch base {
			case "pending_drafts.md", "todo.txt", "autonomous_approval_decisions.json", "autonomous_approval_reviews.json":
				return false
			default:
				return true
			}
		}
		return strings.EqualFold(parts[1], "ga-admin-schedule-runs") || strings.EqualFold(parts[1], "autonomous") && len(parts) > 2 && strings.EqualFold(parts[2], "control")
	}
	if base == "mykey.py" {
		return !copyMyKey
	}
	if strings.HasPrefix(base, "mykey.py.") || base == "model_profiles.json" {
		return true
	}
	if base == ".env" || strings.HasPrefix(base, ".env.") {
		return true
	}
	return false
}

func splitClonePath(rel string) []string {
	rel = filepath.ToSlash(filepath.Clean(rel))
	if rel == "." || rel == "" {
		return nil
	}
	return strings.Split(rel, "/")
}

func pathsOverlap(a, b string) bool {
	a, _ = filepath.Abs(filepath.Clean(a))
	b, _ = filepath.Abs(filepath.Clean(b))
	return pathWithin(a, b) || pathWithin(b, a)
}

func pathWithin(parent, child string) bool {
	parent = filepath.Clean(parent)
	child = filepath.Clean(child)
	if sameRuntimePath(parent, child) {
		return true
	}
	rel, err := filepath.Rel(parent, child)
	if err != nil {
		return false
	}
	return rel != ".." && rel != "." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) && !filepath.IsAbs(rel)
}
