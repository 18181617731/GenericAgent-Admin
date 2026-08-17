//go:build windows

package api

import (
	"reflect"
	"testing"
)

func TestManualScheduleRunCommandHidesWindow(t *testing.T) {
	cmd := newManualScheduleRunCommand("python.exe", "manual-123", 2)
	if cmd.SysProcAttr == nil {
		t.Fatal("manual schedule command must configure Windows process attributes")
	}
	if !cmd.SysProcAttr.HideWindow {
		t.Fatal("manual schedule command must hide the child console window")
	}
	const createNoWindow = 0x08000000
	if cmd.SysProcAttr.CreationFlags&createNoWindow == 0 {
		t.Fatalf("creation flags = %#x, want CREATE_NO_WINDOW", cmd.SysProcAttr.CreationFlags)
	}
	wantArgs := []string{"python.exe", "agentmain.py", "--task", "ga-admin-schedule-runs/manual-123", "--nobg", "--llm_no", "2"}
	if !reflect.DeepEqual(cmd.Args, wantArgs) {
		t.Fatalf("command args = %#v, want %#v", cmd.Args, wantArgs)
	}
}
