package api

import (
	"context"
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
	instanceCloneMaxBytes        = int64(2 << 30)
	instanceCloneMaxEntries      = 100000
	instanceCloneReservationFile = ".ga-admin-clone-in-progress"
)

type cloneProgressFunc func(copiedBytes, totalBytes int64, copiedEntries, totalEntries int)

type cloneSourceStats struct {
	totalBytes   int64
	totalEntries int
}

// reserveCloneDestination creates the empty marker-backed directory that the
// config store requires before an initializing instance can be persisted. The
// marker lets the background copier distinguish this reservation from an
// unrelated pre-existing directory.
func reserveCloneDestination(rawDest string) (string, error) {
	dest, err := filepath.Abs(filepath.Clean(strings.TrimSpace(rawDest)))
	if err != nil {
		return "", err
	}
	if dest == "." || strings.TrimSpace(rawDest) == "" {
		return "", fmt.Errorf("destination instance root is required")
	}
	if _, err := os.Lstat(dest); err == nil {
		return "", fmt.Errorf("destination instance root already exists")
	} else if !os.IsNotExist(err) {
		return "", fmt.Errorf("check destination instance root: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(dest), 0755); err != nil {
		return "", fmt.Errorf("create destination parent: %w", err)
	}
	if err := os.Mkdir(dest, 0755); err != nil {
		if os.IsExist(err) {
			return "", fmt.Errorf("destination instance root already exists")
		}
		return "", fmt.Errorf("create destination instance root: %w", err)
	}
	marker := filepath.Join(dest, instanceCloneReservationFile)
	file, err := os.OpenFile(marker, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		_ = os.RemoveAll(dest)
		return "", fmt.Errorf("reserve destination instance root: %w", err)
	}
	if _, err := file.WriteString("reserved for GenericAgent-Admin clone\n"); err != nil {
		_ = file.Close()
		_ = os.RemoveAll(dest)
		return "", fmt.Errorf("write destination reservation: %w", err)
	}
	if err := file.Close(); err != nil {
		_ = os.RemoveAll(dest)
		return "", fmt.Errorf("close destination reservation: %w", err)
	}
	return dest, nil
}

func resetCloneDestinationForResume(rawDest string) error {
	dest, err := filepath.Abs(filepath.Clean(strings.TrimSpace(rawDest)))
	if err != nil {
		return err
	}
	if err := os.RemoveAll(dest); err != nil {
		return err
	}
	_, err = reserveCloneDestination(dest)
	return err
}

// cloneGenericAgentProject is kept as the synchronous compatibility wrapper
// used by callers and tests that do not need cancellation or progress.
func cloneGenericAgentProject(src, dest string, copyMemory, copyMyKey bool) error {
	return cloneGenericAgentProjectWithContext(context.Background(), src, dest, copyMemory, copyMyKey, nil)
}

// cloneGenericAgentProjectWithContext copies only the source files that make
// up a runnable GA project. It performs a bounded preflight scan before
// creating the destination, then reports byte/file progress while copying so
// a long-running clone can be surfaced by the Admin UI.
func cloneGenericAgentProjectWithContext(ctx context.Context, src, dest string, copyMemory, copyMyKey bool, onProgress cloneProgressFunc) (err error) {
	if ctx == nil {
		ctx = context.Background()
	}
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
	reservedDestination := false
	if info, err := os.Lstat(dest); err == nil {
		if !info.IsDir() {
			return fmt.Errorf("destination instance root already exists")
		}
		// The API creates a marker-backed empty directory before saving the
		// initializing instance. This makes the path satisfy config validation
		// while preventing an unrelated pre-existing directory from being used.
		marker := filepath.Join(dest, instanceCloneReservationFile)
		if markerInfo, markerErr := os.Stat(marker); markerErr != nil || !markerInfo.Mode().IsRegular() {
			return fmt.Errorf("destination instance root already exists")
		}
		reservedDestination = true
	} else if !os.IsNotExist(err) {
		return fmt.Errorf("check destination instance root: %w", err)
	} else if err := os.MkdirAll(dest, 0755); err != nil {
		return fmt.Errorf("create destination instance root: %w", err)
	}
	stats, err := scanCloneSource(ctx, src, copyMemory, copyMyKey)
	if err != nil {
		return fmt.Errorf("scan source instance: %w", err)
	}
	keep := false
	defer func() {
		if !keep {
			_ = os.RemoveAll(dest)
		}
	}()
	if reservedDestination {
		if err := os.Remove(filepath.Join(dest, instanceCloneReservationFile)); err != nil {
			return fmt.Errorf("prepare destination instance root: %w", err)
		}
	}

	if onProgress != nil {
		onProgress(0, stats.totalBytes, 0, stats.totalEntries)
	}
	var entries int
	var total int64
	err = filepath.WalkDir(src, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if err := ctx.Err(); err != nil {
			return err
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
		written, copyErr := io.Copy(out, io.LimitReader(contextReader{ctx: ctx, reader: in}, info.Size()+1))
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
		total += written
		if onProgress != nil {
			onProgress(total, stats.totalBytes, entries, stats.totalEntries)
		}
		return nil
	})
	if err != nil {
		return fmt.Errorf("clone project: %w", err)
	}
	keep = true
	return nil
}

type contextReader struct {
	ctx    context.Context
	reader io.Reader
}

func (r contextReader) Read(p []byte) (int, error) {
	if err := r.ctx.Err(); err != nil {
		return 0, err
	}
	return r.reader.Read(p)
}

func scanCloneSource(ctx context.Context, src string, copyMemory, copyMyKey bool) (stats cloneSourceStats, err error) {
	err = filepath.WalkDir(src, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		if entry.Type()&os.ModeSymlink != 0 {
			if entry.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		if rel == "." {
			return nil
		}
		if entry.IsDir() {
			if skipCloneDirectory(rel, copyMemory) {
				return filepath.SkipDir
			}
			return nil
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
		stats.totalEntries++
		if stats.totalEntries > instanceCloneMaxEntries {
			return fmt.Errorf("source instance has too many files")
		}
		if info.Size() < 0 || info.Size() > instanceCloneMaxBytes-stats.totalBytes {
			return fmt.Errorf("source instance exceeds 2 GiB")
		}
		stats.totalBytes += info.Size()
		return nil
	})
	return stats, err
}

func skipCloneDirectory(rel string, copyMemory bool) bool {
	parts := splitClonePath(rel)
	if len(parts) == 0 {
		return false
	}
	// Cargo keeps compiled Rust dependencies under nested target directories.
	// They can be many gigabytes and are reproducible build output, not part of
	// a runnable GenericAgent source tree, so never copy them into a clone.
	for _, part := range parts {
		if strings.EqualFold(part, "target") {
			return true
		}
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
