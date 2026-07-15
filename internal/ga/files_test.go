package ga

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestReadSafeTruncatesFilesWithTooManyLines(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "many-lines.json")
	content := strings.Repeat("\n", maxReadLines+100)
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	detail, err := ReadSafe(root, "many-lines.json")
	if err != nil {
		t.Fatal(err)
	}
	if !detail.Truncated {
		t.Fatal("ReadSafe should mark excessive line count as truncated")
	}
	if got := strings.Count(detail.Content, "\n"); got != maxReadLines {
		t.Fatalf("returned lines = %d, want %d", got, maxReadLines)
	}
	if len(detail.Content) >= len(content) {
		t.Fatalf("content was not shortened: got %d bytes from %d", len(detail.Content), len(content))
	}
}

func TestTailSafeReadsActualEndAfterReadSafeLineLimit(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "many-lines.log")
	var b strings.Builder
	for i := 0; i < maxReadLines+100; i++ {
		b.WriteString("head\n")
	}
	b.WriteString("tail-one\ntail-two")
	if err := os.WriteFile(path, []byte(b.String()), 0644); err != nil {
		t.Fatal(err)
	}

	detail, err := TailSafe(root, "many-lines.log", 2)
	if err != nil {
		t.Fatal(err)
	}
	if detail.Content != "tail-one\ntail-two" {
		t.Fatalf("tail content = %q", detail.Content)
	}
	if !detail.Truncated {
		t.Fatal("TailSafe should mark dropped leading lines as truncated")
	}
}

func TestTailSafeDoesNotTreatMidRuneStartAsBinary(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "utf8-boundary.log")
	content := "世" + strings.Repeat("x", int(maxReadBytes)-7) + "\ntail"
	if got := int64(len([]byte(content))); got != maxReadBytes+1 {
		t.Fatalf("test fixture size = %d, want %d", got, maxReadBytes+1)
	}
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	detail, err := TailSafe(root, "utf8-boundary.log", 1)
	if err != nil {
		t.Fatal(err)
	}
	if detail.Content != "tail" {
		t.Fatalf("tail content = %q", detail.Content)
	}
	if !detail.Truncated {
		t.Fatal("TailSafe should report truncation when dropping leading bytes")
	}
}

func TestSafeResolveAnyAcceptsExplicitAbsolutePathsOutsideRoot(t *testing.T) {
	root := t.TempDir()
	outside := filepath.Join(t.TempDir(), "outside.txt")
	inside := filepath.Join(root, "inside.txt")
	for _, path := range []string{outside, inside} {
		if err := os.WriteFile(path, []byte("ok"), 0644); err != nil {
			t.Fatal(err)
		}
		wantAbs, err := filepath.Abs(path)
		if err != nil {
			t.Fatal(err)
		}
		abs, clean, err := SafeResolveAny(root, path)
		if err != nil {
			t.Fatalf("SafeResolveAny(%q) error = %v, want nil", path, err)
		}
		if filepath.Clean(abs) != filepath.Clean(wantAbs) || clean != filepath.ToSlash(filepath.Clean(wantAbs)) {
			t.Fatalf("SafeResolveAny(%q) = (%q, %q), want (%q, %q)", path, abs, clean, wantAbs, filepath.ToSlash(filepath.Clean(wantAbs)))
		}
	}
}

func TestSafeFileOperationsAllowExplicitAbsolutePathsOutsideRoot(t *testing.T) {
	root := t.TempDir()
	outsideRoot := t.TempDir()
	target := filepath.Join(outsideRoot, "nested", "notes.txt")
	wantPath := filepath.ToSlash(filepath.Clean(target))
	content := "alpha\nneedle\nomega\n"

	written, err := WriteSafe(root, target, content)
	if err != nil {
		t.Fatalf("WriteSafe absolute error = %v, want nil", err)
	}
	if written.Path != wantPath || written.Content != content {
		t.Fatalf("WriteSafe detail = %#v, want path %q and content %q", written, wantPath, content)
	}

	read, err := ReadSafeAny(root, target)
	if err != nil {
		t.Fatalf("ReadSafeAny absolute error = %v, want nil", err)
	}
	if read.Path != wantPath || read.Content != content {
		t.Fatalf("ReadSafeAny detail = %#v, want path %q and content %q", read, wantPath, content)
	}

	items, err := ListSafe(root, filepath.Dir(target))
	if err != nil {
		t.Fatalf("ListSafe absolute error = %v, want nil", err)
	}
	if len(items) != 1 || items[0].Name != filepath.Base(target) || items[0].Path != wantPath {
		t.Fatalf("ListSafe absolute items = %#v, want notes.txt at %q", items, wantPath)
	}

	tail, err := TailSafe(root, target, 2)
	if err != nil {
		t.Fatalf("TailSafe absolute error = %v, want nil", err)
	}
	if tail.Path != wantPath || !strings.Contains(tail.Content, "omega") {
		t.Fatalf("TailSafe absolute detail = %#v, want path %q containing omega", tail, wantPath)
	}

	hits, err := SearchSafe(root, outsideRoot, "needle", 10)
	if err != nil {
		t.Fatalf("SearchSafe absolute error = %v, want nil", err)
	}
	if len(hits) != 1 || hits[0].Path != wantPath || hits[0].Line != 2 {
		t.Fatalf("SearchSafe absolute hits = %#v, want one line-2 hit at %q", hits, wantPath)
	}

	if err := DeleteSafe(root, target); err != nil {
		t.Fatalf("DeleteSafe absolute error = %v, want nil", err)
	}
	if _, err := os.Stat(target); !os.IsNotExist(err) {
		t.Fatalf("target stat after DeleteSafe = %v, want not exist", err)
	}
	if _, err := os.Stat(outsideRoot); err != nil {
		t.Fatalf("outside root was affected: %v", err)
	}
}

func TestSafeFileOperationsKeepRelativePathsSandboxed(t *testing.T) {
	root := t.TempDir()
	escapeName := "escape-" + filepath.Base(root) + ".txt"
	escapeRel := filepath.Join("..", escapeName)

	checks := []struct {
		name string
		call func() error
	}{
		{name: "resolve", call: func() error { _, _, err := SafeResolveAny(root, escapeRel); return err }},
		{name: "read", call: func() error { _, err := ReadSafeAny(root, escapeRel); return err }},
		{name: "tail", call: func() error { _, err := TailSafe(root, escapeRel, 10); return err }},
		{name: "write", call: func() error { _, err := WriteSafe(root, escapeRel, "blocked"); return err }},
		{name: "delete", call: func() error { return DeleteSafe(root, escapeRel) }},
		{name: "list", call: func() error { _, err := ListSafe(root, ".."); return err }},
		{name: "search", call: func() error { _, err := SearchSafe(root, "..", "needle", 10); return err }},
	}
	for _, check := range checks {
		t.Run(check.name, func(t *testing.T) {
			err := check.call()
			if err == nil || !strings.Contains(err.Error(), "escapes GA root") {
				t.Fatalf("error = %v, want escapes GA root", err)
			}
		})
	}
	if _, err := os.Stat(filepath.Join(filepath.Dir(root), escapeName)); !os.IsNotExist(err) {
		t.Fatalf("relative escape target stat = %v, want not exist", err)
	}
}

func TestDeleteSafeRejectsGARootWithoutRemovingIt(t *testing.T) {
	root := t.TempDir()
	marker := filepath.Join(root, "marker.txt")
	if err := os.WriteFile(marker, []byte("keep"), 0644); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{".", root} {
		err := DeleteSafe(root, path)
		if err == nil || !strings.Contains(err.Error(), "cannot delete GA root") {
			t.Fatalf("DeleteSafe(%q) error = %v, want cannot delete GA root", path, err)
		}
		if _, err := os.Stat(marker); err != nil {
			t.Fatalf("marker after DeleteSafe(%q) = %v, want preserved", path, err)
		}
	}
}

func TestIsFilesystemRootRecognizesCurrentVolumeRoot(t *testing.T) {
	root := t.TempDir()
	volumeRoot := filepath.VolumeName(root) + string(os.PathSeparator)
	if !isFilesystemRoot(volumeRoot) {
		t.Fatalf("isFilesystemRoot(%q) = false, want true", volumeRoot)
	}
	if isFilesystemRoot(root) {
		t.Fatalf("isFilesystemRoot(%q) = true, want false", root)
	}
}

func TestSearchSafeStopsAfterMaxHitsWithoutError(t *testing.T) {
	root := t.TempDir()
	for _, name := range []string{"a.txt", "b.txt"} {
		if err := os.WriteFile(filepath.Join(root, name), []byte("needle\n"), 0644); err != nil {
			t.Fatal(err)
		}
	}

	hits, err := SearchSafe(root, ".", "needle", 1)
	if err != nil {
		t.Fatal(err)
	}
	if len(hits) != 1 {
		t.Fatalf("hits = %d, want 1", len(hits))
	}
}

func TestSearchSafeSkipsVeryLongLinesWithoutFailing(t *testing.T) {
	root := t.TempDir()
	longLine := strings.Repeat("x", 128*1024)
	if err := os.WriteFile(filepath.Join(root, "long.txt"), []byte(longLine+"\nneedle\n"), 0644); err != nil {
		t.Fatal(err)
	}

	hits, err := SearchSafe(root, ".", "needle", 10)
	if err != nil {
		t.Fatalf("SearchSafe err = %v, want nil", err)
	}
	if len(hits) != 1 || hits[0].Line != 2 {
		t.Fatalf("hits = %#v, want one hit on line 2", hits)
	}
}

func TestListSafeSkipsSymlinkEscape(t *testing.T) {
	root := t.TempDir()
	outsideDir := t.TempDir()
	outside := filepath.Join(outsideDir, "secret.txt")
	if err := os.WriteFile(outside, []byte("secret"), 0644); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(root, "linked-secret.txt")
	if err := os.Symlink(outside, link); err != nil {
		t.Skipf("symlink unavailable: %v", err)
	}

	items, err := ListSafe(root, ".")
	if err != nil {
		t.Fatalf("ListSafe err = %v, want nil", err)
	}
	for _, item := range items {
		if item.Name == "linked-secret.txt" {
			t.Fatalf("ListSafe item = %#v, want symlink escape skipped", item)
		}
	}
}

func TestListSafeSkipsWindowsJunctionEscape(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("windows junction test")
	}
	root := t.TempDir()
	outsideDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(outsideDir, "secret.txt"), []byte("secret"), 0644); err != nil {
		t.Fatal(err)
	}
	linkDir := filepath.Join(root, "linked-dir")
	out, err := exec.Command("cmd", "/c", "mklink", "/J", linkDir, outsideDir).CombinedOutput()
	if err != nil {
		t.Skipf("junction unavailable: %v out=%s", err, out)
	}

	items, err := ListSafe(root, ".")
	if err != nil {
		t.Fatalf("ListSafe err = %v, want nil", err)
	}
	for _, item := range items {
		if item.Name == "linked-dir" {
			t.Fatalf("ListSafe item = %#v, want junction escape skipped", item)
		}
	}
}

func TestSearchSafeSkipsSymlinkEscape(t *testing.T) {
	root := t.TempDir()
	outsideDir := t.TempDir()
	outside := filepath.Join(outsideDir, "secret.txt")
	if err := os.WriteFile(outside, []byte("needle outside"), 0644); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(root, "linked-secret.txt")
	if err := os.Symlink(outside, link); err != nil {
		t.Skipf("symlink unavailable: %v", err)
	}

	hits, err := SearchSafe(root, ".", "needle", 10)
	if err != nil {
		t.Fatalf("SearchSafe err = %v, want nil", err)
	}
	if len(hits) != 0 {
		t.Fatalf("SearchSafe hits = %#v, want none for symlink escape", hits)
	}
}

func TestSafeResolveRejectsSymlinkEscape(t *testing.T) {
	root := t.TempDir()
	outsideDir := t.TempDir()
	outside := filepath.Join(outsideDir, "secret.txt")
	if err := os.WriteFile(outside, []byte("secret"), 0644); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(root, "link.txt")
	if err := os.Symlink(outside, link); err != nil {
		t.Skipf("symlink unavailable: %v", err)
	}

	if _, _, err := SafeResolve(root, "link.txt"); err == nil || !strings.Contains(err.Error(), "escapes GA root") {
		t.Fatalf("SafeResolve symlink escape err = %v, want escapes GA root", err)
	}
}

func TestWriteSafeRejectsSymlinkParentEscape(t *testing.T) {
	root := t.TempDir()
	outsideDir := t.TempDir()
	linkDir := filepath.Join(root, "linked-dir")
	if err := os.Symlink(outsideDir, linkDir); err != nil {
		t.Skipf("symlink unavailable: %v", err)
	}

	if _, err := WriteSafe(root, "linked-dir/owned.txt", "owned"); err == nil || !strings.Contains(err.Error(), "escapes GA root") {
		t.Fatalf("WriteSafe symlink parent err = %v, want escapes GA root", err)
	}
	if _, err := os.Stat(filepath.Join(outsideDir, "owned.txt")); !os.IsNotExist(err) {
		t.Fatalf("outside file should not be created, stat err=%v", err)
	}
}

func TestLegacyGeneratedModelConfigIsHiddenFromSafeFileAccess(t *testing.T) {
	root := t.TempDir()
	legacy := filepath.Join(root, "mykey_admin.generated.py")
	if err := os.WriteFile(legacy, []byte("secret"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "mykey.py"), []byte("official"), 0644); err != nil {
		t.Fatal(err)
	}

	items, err := ListSafe(root, ".")
	if err != nil {
		t.Fatal(err)
	}
	for _, item := range items {
		if item.Name == "mykey_admin.generated.py" {
			t.Fatalf("legacy generated model config should be hidden from ListSafe: %#v", items)
		}
	}

	for _, tc := range []struct {
		name string
		call func() error
	}{
		{name: "resolve any relative", call: func() error { _, _, err := SafeResolveAny(root, "mykey_admin.generated.py"); return err }},
		{name: "resolve any absolute", call: func() error { _, _, err := SafeResolveAny(root, legacy); return err }},
		{name: "read", call: func() error { _, err := ReadSafe(root, "mykey_admin.generated.py"); return err }},
		{name: "tail", call: func() error { _, err := TailSafe(root, "mykey_admin.generated.py", 10); return err }},
		{name: "write", call: func() error { _, err := WriteSafe(root, "mykey_admin.generated.py", "new secret"); return err }},
		{name: "delete", call: func() error { return DeleteSafe(root, "mykey_admin.generated.py") }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.call()
			if err == nil || !strings.Contains(err.Error(), "legacy generated model config is hidden") {
				t.Fatalf("err=%v, want hidden legacy generated model config", err)
			}
		})
	}

	got, err := os.ReadFile(legacy)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "secret" {
		t.Fatalf("legacy file content changed to %q", got)
	}
}
