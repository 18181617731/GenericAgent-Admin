//go:build darwin

package main

import (
	"os/exec"
	"strings"
)

// systemLocale asks macOS for the user's language. An app launched from Finder
// inherits none of the POSIX locale variables, so the defaults database is the
// only place a windowed process can read it.
func systemLocale() string {
	out, err := exec.Command("defaults", "read", "-g", "AppleLocale").Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}
