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
	if relToRoot == "." {
		relToRoot = ""
	}
	return fullAbs, filepath.ToSlash(relToRoot), nil
}

func ListSafe(root, rel string) ([]SafeFileEntry, error) {
	full, clean, err := SafeResolve(root, rel)
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
	d.Content = string(buf)
	d.Truncated = info.Size() > limit
	return d, nil
}

func TailSafe(root, rel string, lines int) (SafeFileDetail, error) {
	if lines <= 0 || lines > 2000 {
		lines = 200
	}
	d, err := ReadSafe(root, rel)
	if err != nil {
		return d, err
	}
	parts := strings.Split(d.Content, "\n")
	if len(parts) > lines {
		d.Content = strings.Join(parts[len(parts)-lines:], "\n")
		d.Truncated = true
	}
	return d, nil
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
		if err != nil || len(hits) >= maxHits {
			return nil
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
		info, err := de.Info()
		if err != nil || info.Size() > maxSearchFileBytes {
			return nil
		}
		f, err := os.Open(p)
		if err != nil {
			return nil
		}
		defer f.Close()
		s := bufio.NewScanner(f)
		s.Buffer(make([]byte, 0, 64*1024), 1024*1024)
		lineNo := 0
		for s.Scan() {
			lineNo++
			line := s.Text()
			if strings.Contains(strings.ToLower(line), lowerQ) {
				rel, _ := filepath.Rel(rootAbs, p)
				preview := strings.TrimSpace(line)
				if len([]rune(preview)) > 240 {
					preview = string([]rune(preview)[:240]) + "…"
				}
				hits = append(hits, FileSearchHit{Path: filepath.ToSlash(rel), Line: lineNo, Preview: preview})
				if len(hits) >= maxHits {
					break
				}
			}
		}
		return nil
	})
	return hits, err
}
