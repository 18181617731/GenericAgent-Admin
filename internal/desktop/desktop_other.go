//go:build !windows && !darwin

package desktop

func runDesktopWindow(_ desktopWindowSpec, _ func(desktopWindow)) error {
	return errDesktopWindowUnsupported
}

// EnableHiDPI is a Windows concern; Linux already hands the process real
// pixels. macOS has its own desktop backend.
func EnableHiDPI() {}
