//go:build windows

package main

import (
	"context"
	"os/exec"
	"syscall"
)

const createNoWindow = 0x08000000

func newPortableBootstrapCommand(ctx context.Context, pythonExe, bootstrapPy string) *exec.Cmd {
	cmd := exec.CommandContext(ctx, pythonExe, bootstrapPy)
	cmd.SysProcAttr = &syscall.SysProcAttr{
		HideWindow:    true,
		CreationFlags: createNoWindow,
	}
	return cmd
}
