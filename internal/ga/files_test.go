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

func TestSafeResolveAnyRejectsAbsolutePathOutsideRoot(t *testing.T) {
	root := t.TempDir()
	outsideDir := t.TempDir()
	outside := filepath.Join(outsideDir, "outside.txt")
	if err := os.WriteFile(outside, []byte("secret"), 0644); err != nil {
		t.Fatal(err)
	}

	if _, _, err := SafeResolveAny(root, outside); err == nil || !strings.Contains(err.Error(), "escapes GA root") {
		t.Fatalf("SafeResolveAny outside absolute err = %v, want escapes GA root", err)
	}

	inside := filepath.Join(root, "inside.txt")
	if err := os.WriteFile(inside, []byte("ok"), 0644); err != nil {
		t.Fatal(err)
	}
	abs, rel, err := SafeResolveAny(root, inside)
	if err != nil {
		t.Fatal(err)
	}
	if abs != inside || rel != "inside.txt" {
		t.Fatalf("SafeResolveAny inside = (%q, %q), want (%q, inside.txt)", abs, rel, inside)
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
