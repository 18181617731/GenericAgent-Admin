//go:build !windows && !darwin

package autostart

import "errors"

var errUnsupported = errors.New("autostart is only supported on Windows and macOS")

func StatusFor(target, appRoot string) Status {
	return Status{
		Supported: false,
		Method:    "unsupported",
		Target:    target,
		Detail:    "Only Windows HKCU Run and macOS LaunchAgent are supported",
	}
}

func Enable(target, appRoot string) (Status, error) {
	return StatusFor(target, appRoot), errUnsupported
}

func Disable(target, appRoot string) (Status, error) {
	return StatusFor(target, appRoot), errUnsupported
}
