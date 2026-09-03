//go:build windows

package main

import (
	"context"
	"reflect"
	"testing"
)

func TestPortableBootstrapCommandArgumentsAndWindow(t *testing.T) {
	cmd := newPortableBootstrapCommand(context.Background(), "python.exe", "bootstrap.py")
	if cmd.SysProcAttr == nil {
		t.Fatal("portable bootstrap command must configure Windows process attributes")
	}
	if !cmd.SysProcAttr.HideWindow {
		t.Fatal("portable bootstrap command must hide the child console window")
	}
	const createNoWindow = 0x08000000
	if cmd.SysProcAttr.CreationFlags&createNoWindow == 0 {
		t.Fatalf("creation flags = %#x, want CREATE_NO_WINDOW", cmd.SysProcAttr.CreationFlags)
	}
	wantArgs := []string{"python.exe", "bootstrap.py"}
	if !reflect.DeepEqual(cmd.Args, wantArgs) {
		t.Fatalf("command args = %#v, want %#v", cmd.Args, wantArgs)
	}
}
