// Package appicon carries the application mark. One icon serves the tray, the
// taskbar, and every desktop window, so it lives in a package both the tray and
// the desktop window backends can embed rather than in either of them.
//
// assets/source_tray_icon.png is the master artwork; the two embedded files are
// generated from it by scripts/icons/build_windows_icon.py.
package appicon

import _ "embed"

// ICO is the multi-frame Windows icon used by the tray, the window class, and
// the executable's resource section.
//
//go:embed assets/tray_windows.ico
var ICO []byte

// PNG is the single-size mark the macOS and Linux trays take.
//
//go:embed assets/tray.png
var PNG []byte
