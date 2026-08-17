//go:build windows

package main

import (
	"os/exec"
	"syscall"
)

func hideChildWindow(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: 0x08000000}
}

func newPortableBootstrapCommand(python, script string) *exec.Cmd {
	cmd := exec.Command(python, script)
	hideChildWindow(cmd)
	return cmd
}
