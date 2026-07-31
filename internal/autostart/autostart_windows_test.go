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
