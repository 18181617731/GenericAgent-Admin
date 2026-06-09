package hatchpet

import (
	"embed"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

const (
	SkillName    = "hatch-pet"
	embeddedRoot = "skill"
	memoryRoot   = "memory"
	insightLine  = "桌宠: ga_admin_pets_sop(GA桌宠资产/9行spritesheet) | hatch_pet_sop(Codex宠物/9行spritesheet)"
)

// Skill embeds the complete Codex hatch-pet skill so GA Admin builds remain self-contained.
//
//go:embed skill
var Skill embed.FS

// MemorySOPs embeds the GA memory SOPs needed by GenericAgent after Admin installs pet tooling.
//
//go:embed memory/ga_admin_pets_sop.md memory/hatch_pet_sop.md
var MemorySOPs embed.FS

type FileEntry struct {
	Path string `json:"path"`
	Size int64  `json:"size"`
}

type Status struct {
	SkillName        string        `json:"skill_name"`
	EmbeddedRoot     string        `json:"embedded_root"`
	DefaultExportDir string        `json:"default_export_dir"`
	ExportPath       string        `json:"export_path"`
	Exported         bool          `json:"exported"`
	Files            int           `json:"files"`
	Bytes            int64         `json:"bytes"`
	Manifest         []FileEntry   `json:"manifest,omitempty"`
	Missing          []string      `json:"missing,omitempty"`
	Memory           *MemoryStatus `json:"memory,omitempty"`
}

type MemoryStatus struct {
	GARoot         string      `json:"ga_root"`
	MemoryDir      string      `json:"memory_dir"`
	InsightPath    string      `json:"insight_path"`
	Installed      bool        `json:"installed"`
	InsightUpdated bool        `json:"insight_updated"`
	Files          int         `json:"files"`
	Bytes          int64       `json:"bytes"`
	Manifest       []FileEntry `json:"manifest,omitempty"`
	Missing        []string    `json:"missing,omitempty"`
}

func unsafeFilesystemRoot(p string) bool {
	clean := filepath.Clean(strings.TrimSpace(p))
	if clean == "" || clean == "." {
		return true
	}
	vol := filepath.VolumeName(clean)
	rest := strings.TrimPrefix(clean, vol)
	rest = filepath.Clean(rest)
	return rest == "" || rest == "." || rest == string(filepath.Separator)
}

func DefaultExportDir() (string, error) {
	cwd, err := os.Getwd()
	if err != nil {
		return "", err
	}
	return filepath.Join(cwd, "tools", SkillName), nil
}

func DefaultExportDirForGARoot(gaRoot string) (string, error) {
	cleanRoot := filepath.Clean(strings.TrimSpace(gaRoot))
	if cleanRoot == "" || cleanRoot == "." || cleanRoot == string(filepath.Separator) {
		return "", fmt.Errorf("ga_root is required for GA Admin hatch-pet tool export")
	}
	return filepath.Join(cleanRoot, "tools", SkillName), nil
}

func FS() (fs.FS, error) { return fs.Sub(Skill, embeddedRoot) }

func Manifest() ([]FileEntry, int64, error) {
	var entries []FileEntry
	var total int64
	err := fs.WalkDir(Skill, embeddedRoot, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		info, err := d.Info()
		if err != nil {
			return err
		}
		rel := strings.TrimPrefix(strings.TrimPrefix(p, embeddedRoot), "/")
		entries = append(entries, FileEntry{Path: rel, Size: info.Size()})
		total += info.Size()
		return nil
	})
	sort.Slice(entries, func(i, j int) bool { return entries[i].Path < entries[j].Path })
	return entries, total, err
}

func StatusAt(dest string) (Status, error) {
	if dest == "" {
		var err error
		dest, err = DefaultExportDir()
		if err != nil {
			return Status{}, err
		}
	}
	manifest, bytes, err := Manifest()
	if err != nil {
		return Status{}, err
	}
	st := Status{SkillName: SkillName, EmbeddedRoot: embeddedRoot, DefaultExportDir: dest, ExportPath: dest, Files: len(manifest), Bytes: bytes, Manifest: manifest}
	for _, entry := range manifest {
		if _, err := os.Stat(filepath.Join(dest, filepath.FromSlash(entry.Path))); err != nil {
			st.Missing = append(st.Missing, entry.Path)
		}
	}
	st.Exported = len(st.Missing) == 0 && len(manifest) > 0
	return st, nil
}

func Export(dest string, overwrite bool) (Status, error) {
	if dest == "" {
		var err error
		dest, err = DefaultExportDir()
		if err != nil {
			return Status{}, err
		}
	}
	cleanDest := filepath.Clean(dest)
	if unsafeFilesystemRoot(dest) {
		return Status{}, fmt.Errorf("refusing to export hatch-pet to unsafe path %q", dest)
	}
	err := fs.WalkDir(Skill, embeddedRoot, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel := strings.TrimPrefix(strings.TrimPrefix(p, embeddedRoot), "/")
		if rel == "" {
			return os.MkdirAll(cleanDest, 0o755)
		}
		out := filepath.Join(cleanDest, filepath.FromSlash(rel))
		if d.IsDir() {
			return os.MkdirAll(out, 0o755)
		}
		if !overwrite {
			if _, err := os.Stat(out); err == nil {
				return nil
			} else if !os.IsNotExist(err) {
				return err
			}
		}
		data, err := fs.ReadFile(Skill, p)
		if err != nil {
			return err
		}
		if err := os.MkdirAll(filepath.Dir(out), 0o755); err != nil {
			return err
		}
		return os.WriteFile(out, data, 0o644)
	})
	if err != nil {
		return Status{}, err
	}
	return StatusAt(cleanDest)
}

func MemoryManifest() ([]FileEntry, int64, error) {
	var entries []FileEntry
	var total int64
	err := fs.WalkDir(MemorySOPs, memoryRoot, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		info, err := d.Info()
		if err != nil {
			return err
		}
		rel := strings.TrimPrefix(strings.TrimPrefix(p, memoryRoot), "/")
		entries = append(entries, FileEntry{Path: rel, Size: info.Size()})
		total += info.Size()
		return nil
	})
	sort.Slice(entries, func(i, j int) bool { return entries[i].Path < entries[j].Path })
	return entries, total, err
}

func MemoryStatusAt(gaRoot string) (MemoryStatus, error) {
	cleanRoot := filepath.Clean(strings.TrimSpace(gaRoot))
	if cleanRoot == "" || cleanRoot == "." {
		return MemoryStatus{}, fmt.Errorf("ga_root is required")
	}
	info, err := os.Stat(cleanRoot)
	if err != nil {
		return MemoryStatus{}, err
	}
	if !info.IsDir() {
		return MemoryStatus{}, fmt.Errorf("ga_root is not a directory")
	}
	memoryDir := filepath.Join(cleanRoot, "memory")
	insightPath := filepath.Join(memoryDir, "global_mem_insight.txt")
	manifest, bytes, err := MemoryManifest()
	if err != nil {
		return MemoryStatus{}, err
	}
	st := MemoryStatus{GARoot: cleanRoot, MemoryDir: memoryDir, InsightPath: insightPath, Files: len(manifest), Bytes: bytes, Manifest: manifest}
	for _, entry := range manifest {
		if _, err := os.Stat(filepath.Join(memoryDir, filepath.FromSlash(entry.Path))); err != nil {
			st.Missing = append(st.Missing, entry.Path)
		}
	}
	if data, err := os.ReadFile(insightPath); err == nil {
		st.InsightUpdated = strings.Contains(string(data), "ga_admin_pets_sop") && strings.Contains(string(data), "hatch_pet_sop")
	}
	st.Installed = len(st.Missing) == 0 && st.InsightUpdated && len(manifest) > 0
	return st, nil
}

func InstallMemorySOPs(gaRoot string, overwrite bool) (MemoryStatus, error) {
	cleanRoot := filepath.Clean(strings.TrimSpace(gaRoot))
	if unsafeFilesystemRoot(gaRoot) {
		return MemoryStatus{}, fmt.Errorf("refusing to install memory SOPs to unsafe ga_root %q", gaRoot)
	}
	info, err := os.Stat(cleanRoot)
	if err != nil {
		return MemoryStatus{}, err
	}
	if !info.IsDir() {
		return MemoryStatus{}, fmt.Errorf("ga_root is not a directory")
	}
	memoryDir := filepath.Join(cleanRoot, "memory")
	if err := os.MkdirAll(memoryDir, 0o755); err != nil {
		return MemoryStatus{}, err
	}
	if err := fs.WalkDir(MemorySOPs, memoryRoot, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel := strings.TrimPrefix(strings.TrimPrefix(p, memoryRoot), "/")
		if rel == "" || d.IsDir() {
			return nil
		}
		out := filepath.Join(memoryDir, filepath.FromSlash(rel))
		if !overwrite {
			if _, err := os.Stat(out); err == nil {
				return nil
			} else if !os.IsNotExist(err) {
				return err
			}
		}
		data, err := fs.ReadFile(MemorySOPs, p)
		if err != nil {
			return err
		}
		if err := os.MkdirAll(filepath.Dir(out), 0o755); err != nil {
			return err
		}
		return os.WriteFile(out, data, 0o644)
	}); err != nil {
		return MemoryStatus{}, err
	}
	if err := ensureInsight(memoryDir); err != nil {
		return MemoryStatus{}, err
	}
	return MemoryStatusAt(cleanRoot)
}

func ensureInsight(memoryDir string) error {
	path := filepath.Join(memoryDir, "global_mem_insight.txt")
	data, err := os.ReadFile(path)
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	content := string(data)
	if content == "" {
		content = "# [Global Memory Insight]\n"
	}
	if strings.Contains(content, "ga_admin_pets_sop") && strings.Contains(content, "hatch_pet_sop") {
		return nil
	}
	if !strings.HasSuffix(content, "\n") {
		content += "\n"
	}
	content += insightLine + "\n"
	return os.WriteFile(path, []byte(content), 0o644)
}
