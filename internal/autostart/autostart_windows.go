//go:build windows

package autostart

import (
	"fmt"
	"strings"

	"golang.org/x/sys/windows/registry"
)

const runKey = `Software\Microsoft\Windows\CurrentVersion\Run`

// registryValue returns the value to store in the Run key.
// --no-browser suppresses the browser launch on autostart.
// --app-root pins config.local.json to the directory used when enabling autostart.
func registryValue(target, appRoot string) string {
	return fmt.Sprintf("%s --no-browser --app-root %s", quoteWindowsArgument(target), quoteWindowsArgument(appRoot))
}

func quoteWindowsArgument(value string) string {
	var quoted strings.Builder
	quoted.Grow(len(value) + 2)
	quoted.WriteByte('"')
	backslashes := 0
	for _, char := range value {
		if char == '\\' {
			backslashes++
			continue
		}
		if char == '"' {
			quoted.WriteString(strings.Repeat(`\`, backslashes*2+1))
			quoted.WriteRune(char)
			backslashes = 0
			continue
		}
		quoted.WriteString(strings.Repeat(`\`, backslashes))
		quoted.WriteRune(char)
		backslashes = 0
	}
	quoted.WriteString(strings.Repeat(`\`, backslashes*2))
	quoted.WriteByte('"')
	return quoted.String()
}

func StatusFor(target, appRoot string) Status {
	s := Status{
		Supported: true,
		Method:    "HKCU Run",
		Path:      `HKCU\` + runKey,
		Target:    target,
	}
	k, err := registry.OpenKey(registry.CURRENT_USER, runKey, registry.QUERY_VALUE)
	if err != nil {
		// Key doesn't exist → not enabled
		s.Enabled = false
		return s
	}
	defer k.Close()

	val, _, err := k.GetStringValue(runName)
	if err != nil {
		s.Enabled = false
		return s
	}
	s.Detail = val
	if target == "" {
		s.Enabled = true
		return s
	}
	// Exact match against the full registered command (case-insensitive)
	s.Enabled = strings.EqualFold(val, registryValue(target, appRoot))
	return s
}

func Enable(target, appRoot string) (Status, error) {
	if target == "" {
		return Status{Supported: true}, fmt.Errorf("empty executable path")
	}
	k, _, err := registry.CreateKey(registry.CURRENT_USER, runKey, registry.SET_VALUE)
	if err != nil {
		return StatusFor(target, appRoot), fmt.Errorf("open registry key: %w", err)
	}
	defer k.Close()

	if err := k.SetStringValue(runName, registryValue(target, appRoot)); err != nil {
		return StatusFor(target, appRoot), fmt.Errorf("set registry value: %w", err)
	}
	return StatusFor(target, appRoot), nil
}

func Disable(target, appRoot string) (Status, error) {
	k, err := registry.OpenKey(registry.CURRENT_USER, runKey, registry.SET_VALUE)
	if err != nil {
		// Key doesn't exist at all — nothing to disable
		return StatusFor(target, appRoot), nil
	}
	defer k.Close()

	err = k.DeleteValue(runName)
	if err != nil && err != registry.ErrNotExist {
		return StatusFor(target, appRoot), fmt.Errorf("delete registry value: %w", err)
	}
	return StatusFor(target, appRoot), nil
}
