package api

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestDarwinDirectoryPickerUsesNativeOpenPanel(t *testing.T) {
	_, currentFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("locate test source")
	}
	sourcePath := filepath.Join(filepath.Dir(currentFile), "directory_picker_darwin.go")
	source, err := os.ReadFile(sourcePath)
	if err != nil {
		t.Fatalf("read Darwin picker source: %v", err)
	}
	text := string(source)
	for _, want := range []string{
		"NSOpenPanel",
		"panel.canChooseFiles = NO",
		"panel.canChooseDirectories = YES",
		"panel.canCreateDirectories = YES",
		"dispatch_sync(dispatch_get_main_queue()",
		"panel.directoryURL",
	} {
		if !strings.Contains(text, want) {
			t.Errorf("Darwin picker source missing %q", want)
		}
	}
}
