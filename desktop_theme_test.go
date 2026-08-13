package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestWindowThemeStatePathEmpty(t *testing.T) {
	if got := windowThemeStatePath(""); got != "" {
		t.Fatalf("windowThemeStatePath(\"\") = %q, want empty", got)
	}
}

func TestWindowThemeRoundTrip(t *testing.T) {
	dir := t.TempDir()
	path := windowThemeStatePath(dir)
	if path != filepath.Join(dir, "window-theme") {
		t.Fatalf("path = %q", path)
	}
	if readWindowTheme(path) {
		t.Fatal("missing file should read as light")
	}
	writeWindowTheme(path, true)
	if !readWindowTheme(path) {
		t.Fatal("expected dark after write")
	}
	writeWindowTheme(path, false)
	if readWindowTheme(path) {
		t.Fatal("expected light after write")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if got := string(data); got != lightThemeMarker+"\n" {
		t.Fatalf("file contents = %q", got)
	}
}

func TestWriteWindowThemeSkipsEmptyPath(t *testing.T) {
	writeWindowTheme("", true)
}
