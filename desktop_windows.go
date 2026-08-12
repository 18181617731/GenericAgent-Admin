//go:build windows

package main

import (
	"errors"
	"fmt"
	"log"
	"os"

	webview2 "github.com/jchv/go-webview2"
	"github.com/jchv/go-webview2/webviewloader"
	"golang.org/x/sys/windows"
)

var (
	user32                  = windows.NewLazySystemDLL("user32.dll")
	procIsIconic            = user32.NewProc("IsIconic")
	procShowWindow          = user32.NewProc("ShowWindow")
	procSetForegroundWindow = user32.NewProc("SetForegroundWindow")
)

const swRestore = 9

type webview2Window struct {
	view webview2.WebView
}

func (w *webview2Window) Focus() {
	hwnd := uintptr(w.view.Window())
	if hwnd == 0 {
		return
	}
	if minimized, _, _ := procIsIconic.Call(hwnd); minimized != 0 {
		_, _, _ = procShowWindow.Call(hwnd, swRestore)
	}
	_, _, _ = procSetForegroundWindow.Call(hwnd)
}

// Navigate hands the request to the window's own thread: WebView2 only
// accepts calls on the thread that created it.
func (w *webview2Window) Navigate(url string) {
	w.view.Dispatch(func() { w.view.Navigate(url) })
}

func (w *webview2Window) Close() {
	w.view.Destroy()
}

// runDesktopWindow creates a WebView2 window on the caller's locked OS thread
// and pumps its message loop until the window closes.
func runDesktopWindow(spec desktopWindowSpec, ready func(desktopWindow)) (err error) {
	defer func() {
		if recovered := recover(); recovered != nil {
			err = fmt.Errorf("webview2 window crashed: %v", recovered)
		}
	}()

	// WebView2 aborts the whole process when it fails to create its browser
	// environment, so verify the runtime and the data directory up front where
	// failures are still recoverable.
	if _, versionErr := webviewloader.GetInstalledVersion(); versionErr != nil {
		return fmt.Errorf("webview2 runtime is not installed: %w", versionErr)
	}
	if spec.DataPath != "" {
		if mkErr := os.MkdirAll(spec.DataPath, 0o755); mkErr != nil {
			return fmt.Errorf("prepare webview2 data directory %s: %w", spec.DataPath, mkErr)
		}
	}

	// Sizes in the spec are layout pixels; the window is measured in device
	// pixels, which differ as soon as the display is scaled.
	view := webview2.NewWithOptions(webview2.WebViewOptions{
		DataPath:  spec.DataPath,
		AutoFocus: true,
		WindowOptions: webview2.WindowOptions{
			Title:  spec.Title,
			Width:  uint(scaleForDPI(spec.Width)),
			Height: uint(scaleForDPI(spec.Height)),
			Center: true,
		},
	})
	if view == nil {
		return errors.New("webview2 window could not be created")
	}
	if spec.MinWidth > 0 && spec.MinHeight > 0 {
		view.SetSize(scaleForDPI(spec.MinWidth), scaleForDPI(spec.MinHeight), webview2.HintMin)
	}

	hwnd := uintptr(view.Window())
	setWindowIcon(hwnd, appIconICO)

	// The page owns the palette and reports it once it has loaded; until then
	// the caption uses whatever the last session ended on. Binding has to
	// happen before the first navigation.
	themeState := windowThemeStatePath(spec.DataPath)
	dark := readWindowTheme(themeState)
	if dark {
		setTitleBarTheme(hwnd, true)
	}
	confirmed := false
	if bindErr := view.Bind(nativeThemeBinding, func(next bool) {
		// The first report is applied even when it matches the remembered
		// value: that guess was made before the window had painted anything,
		// and a caption stuck on the wrong colour would never correct itself.
		if confirmed && next == dark {
			return
		}
		confirmed, dark = true, next
		setTitleBarTheme(hwnd, next)
		writeWindowTheme(themeState, next)
	}); bindErr != nil {
		log.Printf("desktop window %q cannot follow the app theme: %v", spec.Name, bindErr)
	}

	view.Navigate(spec.URL)

	ready(&webview2Window{view: view})
	view.Run()
	return nil
}
