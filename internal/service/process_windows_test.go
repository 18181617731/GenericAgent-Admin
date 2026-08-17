//go:build windows

package service

import (
	"reflect"
	"testing"
)

func TestListPythonProcessesCommandHidesWindow(t *testing.T) {
	cmd := newListPythonProcessesCommand("Write-Output test")
	if cmd.SysProcAttr == nil {
		t.Fatal("process scan command must configure Windows process attributes")
	}
	if !cmd.SysProcAttr.HideWindow {
		t.Fatal("process scan command must hide the child console window")
	}
	const createNoWindow = 0x08000000
	if cmd.SysProcAttr.CreationFlags&createNoWindow == 0 {
		t.Fatalf("creation flags = %#x, want CREATE_NO_WINDOW", cmd.SysProcAttr.CreationFlags)
	}
	wantArgs := []string{"powershell", "-NoProfile", "-ExecutionPolicy", "Bypass", "-Command", "Write-Output test"}
	if !reflect.DeepEqual(cmd.Args, wantArgs) {
		t.Fatalf("command args = %#v, want %#v", cmd.Args, wantArgs)
	}
}
