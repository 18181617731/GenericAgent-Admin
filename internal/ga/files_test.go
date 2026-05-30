package ga

import (
	"os"
	"path/filepath"
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
