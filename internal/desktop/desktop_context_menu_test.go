package desktop

import (
	"os"
	"strings"
	"testing"
)

func TestNativeDesktopContextMenusRemainEnabled(t *testing.T) {
	windowsSource, err := os.ReadFile("desktop_host_windows.go")
	if err != nil {
		t.Fatalf("read Windows desktop host: %v", err)
	}
	windowsText := string(windowsSource)
	if !strings.Contains(windowsText, "PutAreDefaultContextMenusEnabled(true)") {
		t.Error("Windows desktop host does not enable WebView2 context menus")
	}
	if strings.Contains(windowsText, "PutAreDefaultContextMenusEnabled(false)") {
		t.Error("Windows desktop host still disables WebView2 context menus")
	}

	darwinSource, err := os.ReadFile("desktop_webview_darwin.m")
	if err != nil {
		t.Fatalf("read macOS desktop host: %v", err)
	}
	darwinText := string(darwinSource)
	for _, blocked := range []string{"removeAllItems", "noContextMenu", "preventDefault();"} {
		if strings.Contains(darwinText, blocked) {
			t.Errorf("macOS desktop host still blocks context menus via %q", blocked)
		}
	}
}
