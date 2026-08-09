//go:build windows

package api

import "testing"

func TestNewChatHubBridgeCommandHidesWindow(t *testing.T) {
	cmd := newChatHubBridgeCommand("python.exe", "chat_hub_bridge.py")
	if cmd.SysProcAttr == nil {
		t.Fatal("chat hub bridge command must configure Windows process attributes")
	}
	if !cmd.SysProcAttr.HideWindow {
		t.Fatal("chat hub bridge command must hide the child console window")
	}
	const createNoWindow = 0x08000000
	if cmd.SysProcAttr.CreationFlags&createNoWindow == 0 {
		t.Fatalf("creation flags = %#x, want CREATE_NO_WINDOW", cmd.SysProcAttr.CreationFlags)
	}
}
