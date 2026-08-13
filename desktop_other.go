//go:build !windows && !darwin

package main

func runDesktopWindow(_ desktopWindowSpec, _ func(desktopWindow)) error {
	return errDesktopWindowUnsupported
}

// enableHiDPI is a Windows concern; Linux already hands the process real
// pixels. macOS has its own desktop backend.
func enableHiDPI() {}
