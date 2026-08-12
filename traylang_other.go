//go:build !windows && !darwin

package main

// systemLocale has no source beyond the environment on these platforms, which
// trayLocale has already read by the time it asks.
func systemLocale() string { return "" }
