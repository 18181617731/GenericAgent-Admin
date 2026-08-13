//go:build windows

package main

import (
	"encoding/binary"
	"log"
	"os"
	"path/filepath"
	"strings"
	"unsafe"

	"golang.org/x/sys/windows"
)

// Window chrome that WebView2 does not manage: the caption icon and the
// caption colour. Both are set from Go because the page inside the window has
// no say over the frame Windows draws around it.
var (
	dwmapi                       = windows.NewLazySystemDLL("dwmapi.dll")
	procDwmSetWindowAttribute    = dwmapi.NewProc("DwmSetWindowAttribute")
	procCreateIconFromResourceEx = user32.NewProc("CreateIconFromResourceEx")
	procSendMessageW             = user32.NewProc("SendMessageW")
	procGetSystemMetrics         = user32.NewProc("GetSystemMetrics")
	procSetWindowPos             = user32.NewProc("SetWindowPos")
	procGetWindowRect            = user32.NewProc("GetWindowRect")

	procSetProcessDpiAwarenessContext = user32.NewProc("SetProcessDpiAwarenessContext")
	procSetProcessDPIAware            = user32.NewProc("SetProcessDPIAware")
	procGetDpiForSystem               = user32.NewProc("GetDpiForSystem")

	shcore                     = windows.NewLazySystemDLL("shcore.dll")
	procSetProcessDpiAwareness = shcore.NewProc("SetProcessDpiAwareness")
)

const (
	darkThemeMarker  = "dark"
	lightThemeMarker = "light"
)

const (
	// DPI_AWARENESS_CONTEXT_PER_MONITOR_AWARE_V2 is the handle value -4.
	dpiPerMonitorAwareV2 = ^uintptr(3)
	// PROCESS_PER_MONITOR_DPI_AWARE for the Windows 8.1 era entry point.
	processPerMonitorDPIAware = 2
	baselineDPI               = 96
)

// enableHiDPI declares that this process draws in real pixels. Without it
// Windows renders the whole window at 96 DPI and stretches the result to the
// monitor's scale, which turns every glyph in the WebView soft. It must run
// before the first window exists, and the process keeps the awareness it was
// given for its lifetime.
func enableHiDPI() {
	// Per-monitor v2 (Windows 10 1703+) also scales the caption and dialogs;
	// the older entry points cover earlier releases and each may be missing
	// entirely, so every one is probed before it is called.
	if procSetProcessDpiAwarenessContext.Find() == nil {
		if ok, _, _ := procSetProcessDpiAwarenessContext.Call(dpiPerMonitorAwareV2); ok != 0 {
			return
		}
	}
	if procSetProcessDpiAwareness.Find() == nil {
		if hr, _, _ := procSetProcessDpiAwareness.Call(processPerMonitorDPIAware); hr == 0 {
			return
		}
	}
	if procSetProcessDPIAware.Find() == nil {
		_, _, _ = procSetProcessDPIAware.Call()
	}
}

// scaleForDPI converts a size written in layout pixels into the device pixels
// a DPI-aware window is measured in. Skipping this would keep the numbers
// honest and the windows visibly smaller on a scaled display.
func scaleForDPI(value int) int {
	if value <= 0 {
		return value
	}
	return value * systemDPI() / baselineDPI
}

func systemDPI() int {
	if procGetDpiForSystem.Find() != nil {
		return baselineDPI
	}
	dpi, _, _ := procGetDpiForSystem.Call()
	if dpi == 0 {
		return baselineDPI
	}
	return int(dpi)
}

const (
	wmSetIcon = 0x0080
	iconSmall = 0
	iconBig   = 1

	smCXScreen = 0
	smCYScreen = 1
	smCXIcon   = 11
	smCYIcon   = 12
	smCXSmIcon = 49
	smCYSmIcon = 50

	lrDefaultColor = 0
	// Version stamped into icon resources; CreateIconFromResourceEx wants the
	// 3.0 format that every .ico file uses.
	iconResourceVersion = 0x00030000

	// DWMWA_USE_IMMERSIVE_DARK_MODE moved between Windows 10 releases, so the
	// newer attribute is tried first and the older one is the fallback.
	dwmwaUseImmersiveDarkMode       = 20
	dwmwaUseImmersiveDarkModeBefore = 19

	swpNoSize       = 0x0001
	swpNoMove       = 0x0002
	swpNoZOrder     = 0x0004
	swpNoActivate   = 0x0010
	swpFrameChanged = 0x0020
)

// setWindowIcon replaces the placeholder caption icon with the app icon, which
// also fixes the taskbar and Alt-Tab entries. The executable carries no icon
// resource, so the icon is built from the embedded .ico at runtime.
func setWindowIcon(hwnd uintptr, ico []byte) {
	if hwnd == 0 || len(ico) == 0 {
		return
	}
	if icon := iconFromICO(ico, systemMetric(smCXIcon), systemMetric(smCYIcon)); icon != 0 {
		_, _, _ = procSendMessageW.Call(hwnd, wmSetIcon, iconBig, icon)
	}
	if icon := iconFromICO(ico, systemMetric(smCXSmIcon), systemMetric(smCYSmIcon)); icon != 0 {
		_, _, _ = procSendMessageW.Call(hwnd, wmSetIcon, iconSmall, icon)
	}
}

// iconFromICO picks the frame closest to the requested size out of an .ico
// file held in memory and turns it into an HICON.
//
// The directory is read here instead of by LookupIconIdFromDirectoryEx, which
// wants the resource form of that table: its entries end in a two-byte
// resource id where a file's end in a four-byte offset, so a file walks it out
// of step after the first entry and it answers with whatever the drift lands
// on — for this icon, the 16px frame at every size, stretched by the call
// below into a blurred caption and taskbar icon.
func iconFromICO(ico []byte, cx, cy int) uintptr {
	offset, length := bestICOFrame(ico, cx, cy)
	if length == 0 {
		return 0
	}
	icon, _, _ := procCreateIconFromResourceEx.Call(
		uintptr(unsafe.Pointer(&ico[offset])), uintptr(length),
		1, iconResourceVersion, uintptr(cx), uintptr(cy), lrDefaultColor)
	return icon
}

// bestICOFrame locates the frame to draw the icon at the requested size from.
func bestICOFrame(ico []byte, cx, cy int) (offset, length int) {
	const header, entry = 6, 16
	if len(ico) < header || cx <= 0 || cy <= 0 {
		return 0, 0
	}
	want := cx
	if cy > want {
		want = cy
	}
	best := 0
	for i := 0; i < int(binary.LittleEndian.Uint16(ico[4:])); i++ {
		at := header + i*entry
		if at+entry > len(ico) {
			break
		}
		side := int(ico[at])
		if side == 0 {
			side = 256
		}
		size := int(binary.LittleEndian.Uint32(ico[at+8:]))
		start := int(binary.LittleEndian.Uint32(ico[at+12:]))
		if size <= 0 || start < header || start+size > len(ico) {
			continue
		}
		if length == 0 || closerICOSide(side, best, want) {
			best, offset, length = side, start, size
		}
	}
	return offset, length
}

// closerICOSide reports whether a frame of side a suits a target of want
// better than one of side b. Reaching the target beats falling short of it,
// because shrinking a frame keeps detail that stretching one cannot invent.
func closerICOSide(a, b, want int) bool {
	if (a >= want) != (b >= want) {
		return a >= want
	}
	if a >= want {
		return a < b
	}
	return a > b
}

// setTitleBarTheme paints the caption dark or light so the frame stops
// clashing with the palette the user picked inside the app.
func setTitleBarTheme(hwnd uintptr, dark bool) {
	if hwnd == 0 {
		return
	}
	flag := int32(0)
	if dark {
		flag = 1
	}
	for _, attribute := range []uintptr{dwmwaUseImmersiveDarkMode, dwmwaUseImmersiveDarkModeBefore} {
		result, _, _ := procDwmSetWindowAttribute.Call(
			hwnd, attribute, uintptr(unsafe.Pointer(&flag)), unsafe.Sizeof(flag))
		if result == 0 {
			break
		}
	}
	repaintCaption(hwnd)
}

// repaintCaption forces Windows 10 to redraw the frame it has already painted.
// Neither SWP_FRAMECHANGED nor RedrawWindow moves it; a one-pixel resize
// round-trip does, and is invisible next to the theme switch that caused it.
func repaintCaption(hwnd uintptr) {
	var rect struct{ left, top, right, bottom int32 }
	if ok, _, _ := procGetWindowRect.Call(hwnd, uintptr(unsafe.Pointer(&rect))); ok == 0 {
		_, _, _ = procSetWindowPos.Call(hwnd, 0, 0, 0, 0, 0,
			swpNoSize|swpNoMove|swpNoZOrder|swpNoActivate|swpFrameChanged)
		return
	}
	width := uintptr(rect.right - rect.left)
	height := uintptr(rect.bottom - rect.top)
	const flags = swpNoMove | swpNoZOrder | swpNoActivate
	_, _, _ = procSetWindowPos.Call(hwnd, 0, 0, 0, width, height+1, flags)
	_, _, _ = procSetWindowPos.Call(hwnd, 0, 0, 0, width, height, flags)
}

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

func systemMetric(index int) int {
	value, _, _ := procGetSystemMetrics.Call(uintptr(index))
	return int(value)
}
