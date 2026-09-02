//go:build windows

package main

import (
	"context"
	"testing"
)

func TestPortableBootstrapCommandHidesWindow(t *testing.T) {
	cmd := newPortableBootstrapCommand(context.Background(), "python.exe", "bootstrap.py")
	if cmd.SysProcAttr == nil {
		t.Fatal("portable bootstrap command must configure Windows process attributes")
	}
	if !cmd.SysProcAttr.HideWindow {
		t.Fatal("portable bootstrap command must hide the child console window")
	}
	if cmd.SysProcAttr.CreationFlags&createNoWindow == 0 {
		t.Fatalf("creation flags = %#x, want CREATE_NO_WINDOW", cmd.SysProcAttr.CreationFlags)
	}
}
