package hatchpet

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestManifestContainsEmbeddedSkill(t *testing.T) {
	manifest, bytes, err := Manifest()
	if err != nil {
		t.Fatalf("manifest: %v", err)
	}
	if len(manifest) < 10 {
		t.Fatalf("expected embedded hatch-pet files, got %d", len(manifest))
	}
	if bytes < 100000 {
		t.Fatalf("expected hatch-pet payload over 100KB, got %d", bytes)
	}
	var foundSkill bool
	for _, entry := range manifest {
		if entry.Path == "SKILL.md" && entry.Size > 0 {
			foundSkill = true
			break
		}
	}
	if !foundSkill {
		t.Fatalf("manifest missing SKILL.md: %+v", manifest)
	}
}

func TestExportEmbeddedSkill(t *testing.T) {
	dir := filepath.Join(t.TempDir(), SkillName)
	st, err := Export(dir, false)
	if err != nil {
		t.Fatalf("export: %v", err)
	}
	if !st.Exported || len(st.Missing) != 0 {
		t.Fatalf("unexpected export status: %+v", st)
	}
	if st.Files < 10 || st.Bytes < 100000 {
		t.Fatalf("unexpected payload summary: files=%d bytes=%d", st.Files, st.Bytes)
	}
	if _, err := os.Stat(filepath.Join(dir, "SKILL.md")); err != nil {
		t.Fatalf("missing exported SKILL.md: %v", err)
	}
	st2, err := StatusAt(dir)
	if err != nil {
		t.Fatalf("status: %v", err)
	}
	if !st2.Exported || len(st2.Missing) != 0 {
		t.Fatalf("status did not detect exported skill: %+v", st2)
	}
}

func TestUnsafeFilesystemRoot(t *testing.T) {
	for _, p := range []string{"", " ", ".", string(filepath.Separator)} {
		if !unsafeFilesystemRoot(p) {
			t.Fatalf("unsafeFilesystemRoot(%q)=false, want true", p)
		}
	}
	if runtime.GOOS == "windows" {
		for _, p := range []string{`C:`, `C:\`, `C:/`} {
			if !unsafeFilesystemRoot(p) {
				t.Fatalf("unsafeFilesystemRoot(%q)=false, want true", p)
			}
		}
	}
	if unsafeFilesystemRoot(filepath.Join(t.TempDir(), SkillName)) {
		t.Fatal("temp export dir detected as unsafe root")
	}
}

func TestExportRejectsUnsafeRoot(t *testing.T) {
	for _, p := range []string{" ", ".", string(filepath.Separator)} {
		if _, err := Export(p, false); err == nil {
			t.Fatalf("Export(%q) succeeded, want unsafe path error", p)
		}
	}
}

func TestInstallMemorySOPsRejectsUnsafeRoot(t *testing.T) {
	for _, p := range []string{"", " ", ".", string(filepath.Separator)} {
		if _, err := InstallMemorySOPs(p, false); err == nil {
			t.Fatalf("InstallMemorySOPs(%q) succeeded, want unsafe ga_root error", p)
		}
	}
}
