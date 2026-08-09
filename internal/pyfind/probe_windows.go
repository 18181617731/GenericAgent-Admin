//go:build windows

package pyfind

import (
	"os/exec"
	"syscall"
)

func hideProbeWindow(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: 0x08000000}
}
