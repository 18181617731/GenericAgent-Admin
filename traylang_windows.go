//go:build windows

package main

import (
	"unsafe"

	"golang.org/x/sys/windows"
)

var (
	kernel32                     = windows.NewLazySystemDLL("kernel32.dll")
	procGetUserDefaultLocaleName = kernel32.NewProc("GetUserDefaultLocaleName")
)

// LOCALE_NAME_MAX_LENGTH: the longest name the API can return, including its
// terminator.
const localeNameMaxLength = 85

// systemLocale reads the display language the user set for Windows itself,
// which a windowed process never receives through the environment.
func systemLocale() string {
	if err := procGetUserDefaultLocaleName.Find(); err != nil {
		return ""
	}
	buf := make([]uint16, localeNameMaxLength)
	written, _, _ := procGetUserDefaultLocaleName.Call(uintptr(unsafe.Pointer(&buf[0])), uintptr(len(buf)))
	if written == 0 {
		return ""
	}
	return windows.UTF16ToString(buf[:written])
}
