//go:build !windows

package main

func runDesktopWindow(_ desktopWindowSpec, _ func(desktopWindow)) error {
	return errDesktopWindowUnsupported
}

// enableHiDPI is a Windows concern; every other platform this app builds for
// hands the process real pixels already.
func enableHiDPI() {}
