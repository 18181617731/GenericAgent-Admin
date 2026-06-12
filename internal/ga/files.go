package ga

import (
	"bufio"
	"errors"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
	"unicode/utf8"
)

type SafeFileEntry struct {
	Name    string    `json:"name"`
	Path    string    `json:"path"`
	Kind    string    `json:"kind"`
	Size    int64     `json:"size,omitempty"`
	ModTime time.Time `json:"mod_time,omitempty"`
}

type SafeFileDetail struct {
	Path      string    `json:"path"`
	Name      string    `json:"name"`
	Kind      string    `json:"kind"`
	Size      int64     `json:"size,omitempty"`
	ModTime   time.Time `json:"mod_time,omitempty"`
	Content   string    `json:"content,omitempty"`
	Truncated bool      `json:"truncated,omitempty"`
}

type FileSearchHit struct {
	Path    string `json:"path"`
	Line    int    `json:"line"`
	Preview string `json:"preview"`
}

const maxReadBytes int64 = 512 * 1024
const maxReadLines = 5000
const maxSearchFileBytes int64 = 1024 * 1024

func SafeResolve(root, rel string) (string, string, error) {
	if root == "" {
		return "", "", errors.New("GA root is empty")
	}
	clean := filepath.Clean(strings.TrimPrefix(strings.ReplaceAll(rel, "\\", "/"), "/"))
	if clean == "." {
		clean = ""
	}
	if strings.HasPrefix(clean, "..") || filepath.IsAbs(clean) {
		return "", "", errors.New("path escapes GA root")
	}
	rootAbs, err := filepath.Abs(root)
	if err != nil {
		return "", "", err
	}
	full := filepath.Join(rootAbs, filepath.FromSlash(clean))
	return ensureWithinRoot(rootAbs, full)
}

func SafeResolveAny(root, p string) (string, string, error) {
	if root == "" {
		return "", "", errors.New("GA root is empty")
	}
	rootAbs, err := filepath.Abs(root)
	if err != nil {
		return "", "", err
	}
	p = strings.TrimSpace(p)
	if filepath.IsAbs(p) {
		return ensureWithinRoot(rootAbs, p)
	}
	return SafeResolve(root, p)
}

func ensureWithinRoot(rootAbs, full string) (string, string, error) {
	fullAbs, err := filepath.Abs(full)
	if err != nil {
		return "", "", err
	}
	relToRoot, err := filepath.Rel(rootAbs, fullAbs)
	if err != nil {
		return "", "", err
	}
	if relToRoot == ".." || strings.HasPrefix(relToRoot, ".."+string(filepath.Separator)) {
		return "", "", errors.New("path escapes GA root")
	}
	if err := ensureRealPathWithinRoot(rootAbs, fullAbs); err != nil {
		return "", "", err
	}
	if relToRoot == "." {
		relToRoot = ""
	}
	return fullAbs, filepath.ToSlash(relToRoot), nil
}

func ensureRealPathWithinRoot(rootAbs, fullAbs string) error {
	rootReal, err := filepath.EvalSymlinks(rootAbs)
	if err != nil {
		return err
	}
	fullReal, err := filepath.EvalSymlinks(fullAbs)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	relReal, err := filepath.Rel(rootReal, fullReal)
	if err != nil {
		return err
	}
	if relReal == ".." || strings.HasPrefix(relReal, ".."+string(filepath.Separator)) {
		return errors.New("path escapes GA root")
	}
	return nil
}

func ensureWriteParentWithinRoot(root, abs string) error {
	rootAbs, err := filepath.Abs(root)
	if err != nil {
		return err
	}
	parent := filepath.Dir(abs)
	if err := os.MkdirAll(parent, 0755); err != nil {
		return err
	}
	return ensureRealPathWithinRoot(rootAbs, parent)
}

func ListSafe(root, rel string) ([]SafeFileEntry, error) {
	full, clean, err := SafeResolve(root, rel)
	if err != nil {
		return nil, err
	}
	rootAbs, err := filepath.Abs(root)
	if err != nil {
		return nil, err
	}
	items, err := os.ReadDir(full)
	if err != nil {
		return nil, err
	}
	out := []SafeFileEntry{}
	for _, de := range items {
		if strings.HasPrefix(de.Name(), ".") || de.Name() == "__pycache__" {
			continue
		}
		if de.Type()&(os.ModeSymlink|os.ModeIrregular) != 0 {
			continue
		}
		child := filepath.Join(full, de.Name())
		if err := ensureRealPathWithinRoot(rootAbs, child); err != nil {
			continue
		}
		info, err := de.Info()
		if err != nil {
			continue
		}
		kind := "file"
		if de.IsDir() {
			kind = "dir"
		}
		out = append(out, SafeFileEntry{Name: de.Name(), Path: filepath.ToSlash(filepath.Join(clean, de.Name())), Kind: kind, Size: info.Size(), ModTime: info.ModTime()})
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Kind != out[j].Kind {
			return out[i].Kind == "dir"
		}
		return strings.ToLower(out[i].Name) < strings.ToLower(out[j].Name)
	})
	return out, nil
}

func ReadSafe(root, rel string) (SafeFileDetail, error) {
	full, clean, err := SafeResolve(root, rel)
	if err != nil {
		return SafeFileDetail{}, err
	}
	info, err := os.Stat(full)
	if err != nil {
		return SafeFileDetail{}, err
	}
	kind := "file"
	if info.IsDir() {
		kind = "dir"
	}
	d := SafeFileDetail{Path: clean, Name: filepath.Base(full), Kind: kind, Size: info.Size(), ModTime: info.ModTime()}
	if info.IsDir() {
		return d, nil
	}
	limit := maxReadBytes
	if info.Size() < limit {
		limit = info.Size()
	}
	f, err := os.Open(full)
	if err != nil {
		return d, err
	}
	defer f.Close()
	buf := make([]byte, limit)
	n, err := io.ReadFull(f, buf)
	if err != nil && err != io.ErrUnexpectedEOF && err != io.EOF {
		return d, err
	}
	buf = buf[:n]
	if !utf8.Valid(buf) {
		d.Content = "[binary or non-utf8 file]"
		d.Truncated = info.Size() > limit
		return d, nil
	}
	content := string(buf)
	lineCount := 0
	cutAt := -1
	for i, r := range content {
		if r == '\n' {
			lineCount++
			if lineCount >= maxReadLines {
				cutAt = i + 1
				break
			}
		}
	}
	if cutAt >= 0 && cutAt < len(content) {
		d.Content = content[:cutAt]
		d.Truncated = true
	} else {
		d.Content = content
		d.Truncated = info.Size() > limit
	}
	return d, nil
}

func TailSafe(root, rel string, lines int) (SafeFileDetail, error) {
	if lines <= 0 || lines > 2000 {
		lines = 200
	}
	full, clean, err := SafeResolve(root, rel)
	if err != nil {
		return SafeFileDetail{}, err
	}
	info, err := os.Stat(full)
	if err != nil {
		return SafeFileDetail{}, err
	}
	kind := "file"
	if info.IsDir() {
		kind = "dir"
	}
	d := SafeFileDetail{Path: clean, Name: filepath.Base(full), Kind: kind, Size: info.Size(), ModTime: info.ModTime()}
	if info.IsDir() {
		return d, nil
	}

	readBytes := maxReadBytes
	if info.Size() < readBytes {
		readBytes = info.Size()
	}
	f, err := os.Open(full)
	if err != nil {
		return d, err
	}
	defer f.Close()
	start := info.Size() - readBytes
	buf := make([]byte, readBytes)
	n, err := f.ReadAt(buf, start)
	if err != nil && err != io.EOF {
		return d, err
	}
	buf = buf[:n]
	if start > 0 {
		for len(buf) > 0 && !utf8.RuneStart(buf[0]) {
			buf = buf[1:]
			start++
		}
	}
	if !utf8.Valid(buf) {
		d.Content = "[binary or non-utf8 file]"
		d.Truncated = start > 0
		return d, nil
	}
	parts := strings.Split(string(buf), "\n")
	if len(parts) > lines {
		d.Content = strings.Join(parts[len(parts)-lines:], "\n")
		d.Truncated = true
	} else {
		d.Content = string(buf)
		d.Truncated = start > 0
	}
	return d, nil
}

func WriteSafe(root, rel, content string) (SafeFileDetail, error) {
	abs, clean, err := SafeResolve(root, rel)
	if err != nil {
		return SafeFileDetail{}, err
	}
	if clean == "" {
		return SafeFileDetail{}, errors.New("path is empty")
	}
	if strings.HasSuffix(clean, "/") || strings.HasSuffix(clean, "\\") {
		return SafeFileDetail{}, errors.New("cannot write directory")
	}
	if !utf8.ValidString(content) {
		return SafeFileDetail{}, errors.New("content is not valid UTF-8")
	}
	if err := ensureWriteParentWithinRoot(root, abs); err != nil {
		return SafeFileDetail{}, err
	}
	if err := writeFileAtomic(abs, []byte(content), 0644); err != nil {
		return SafeFileDetail{}, err
	}
	return ReadSafe(root, clean)
}

func DeleteSafe(root, rel string) error {
	abs, clean, err := SafeResolveAny(root, rel)
	if err != nil {
		return err
	}
	if clean == "" {
		return errors.New("cannot delete GA root")
	}
	if _, err := os.Stat(abs); err != nil {
		return err
	}
	return os.RemoveAll(abs)
}

func SearchSafe(root, rel, q string, maxHits int) ([]FileSearchHit, error) {
	q = strings.TrimSpace(q)
	if q == "" {
		return []FileSearchHit{}, nil
	}
	if maxHits <= 0 || maxHits > 500 {
		maxHits = 100
	}
	base, _, err := SafeResolve(root, rel)
	if err != nil {
		return nil, err
	}
	rootAbs, _ := filepath.Abs(root)
	hits := []FileSearchHit{}
	lowerQ := strings.ToLower(q)
	err = filepath.WalkDir(base, func(p string, de os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if len(hits) >= maxHits {
			return filepath.SkipAll
		}
		name := de.Name()
		if strings.HasPrefix(name, ".") || name == "__pycache__" {
			if de.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if de.IsDir() {
			return nil
		}
		if err := ensureRealPathWithinRoot(rootAbs, p); err != nil {
			return nil
		}
		info, err := de.Info()
		if err != nil {
			return err
		}
		if info.Size() > maxSearchFileBytes {
			return nil
		}
		f, err := os.Open(p)
		if err != nil {
			return err
		}
		reader := bufio.NewReader(f)
		lineNo := 0
		for {
			line, err := reader.ReadString('\n')
			if len(line) > 0 {
				lineNo++
				if len(line) <= 64*1024 && strings.Contains(strings.ToLower(line), lowerQ) {
					rel, _ := filepath.Rel(rootAbs, p)
					preview := strings.TrimSpace(line)
					if len([]rune(preview)) > 240 {
						preview = string([]rune(preview)[:240]) + "..."
					}
					hits = append(hits, FileSearchHit{Path: filepath.ToSlash(rel), Line: lineNo, Preview: preview})
					if len(hits) >= maxHits {
						break
					}
				}
			}
			if err == io.EOF {
				break
			}
			if err != nil {
				_ = f.Close()
				return err
			}
		}
		return f.Close()
	})
	if err != nil && err != filepath.SkipAll {
		return hits, err
	}
	return hits, nil
}
