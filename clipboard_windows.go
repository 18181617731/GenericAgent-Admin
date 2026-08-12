//go:build windows

package main

import (
	"os/exec"

	"golang.org/x/sys/windows"
)

// hideConsoleWindow keeps clip.exe from flashing a console window in front of
// the user: the admin app is a GUI process and the tray click should look like
// nothing happened beyond the copy itself.
func hideConsoleWindow(cmd *exec.Cmd) {
	cmd.SysProcAttr = &windows.SysProcAttr{HideWindow: true, CreationFlags: windows.CREATE_NO_WINDOW}
}
