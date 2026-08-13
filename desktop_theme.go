package main

import (
	"log"
	"os"
	"path/filepath"
	"strings"
)

const (
	darkThemeMarker  = "dark"
	lightThemeMarker = "light"
)

// windowThemeStatePath is where the last caption colour is remembered. The
// palette lives in the page's local storage, which Go cannot read, so without
// this note every launch would flash a light caption before the page loads.
func windowThemeStatePath(dataPath string) string {
	if dataPath == "" {
		return ""
	}
	return filepath.Join(dataPath, "window-theme")
}

func readWindowTheme(path string) bool {
	if path == "" {
		return false
	}
	data, err := os.ReadFile(path)
	return err == nil && strings.TrimSpace(string(data)) == darkThemeMarker
}

func writeWindowTheme(path string, dark bool) {
	if path == "" {
		return
	}
	marker := lightThemeMarker
	if dark {
		marker = darkThemeMarker
	}
	if err := os.WriteFile(path, []byte(marker+"\n"), 0o644); err != nil {
		log.Printf("remember window theme: %v", err)
	}
}
