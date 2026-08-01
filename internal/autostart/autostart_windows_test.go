//go:build windows

package autostart

import "testing"

func TestRegistryValuePinsApplicationRoot(t *testing.T) {
	target := `C:\Program Files\GenericAgent Admin\ga-admin.exe`
	root := `D:\GenericAgent Workspace`
	want := `"C:\Program Files\GenericAgent Admin\ga-admin.exe" --no-browser --app-root "D:\GenericAgent Workspace"`

	if got := registryValue(target, root); got != want {
		t.Fatalf("registryValue() = %q, want %q", got, want)
	}
}

func TestRegistryValueEscapesTrailingBackslash(t *testing.T) {
	target := `G:\ga-admin-test\ga-admin.exe`
	root := `G:\ga-admin-test\`
	want := `"G:\ga-admin-test\ga-admin.exe" --no-browser --app-root "G:\ga-admin-test\\"`

	if got := registryValue(target, root); got != want {
		t.Fatalf("registryValue() = %q, want %q", got, want)
	}
}
