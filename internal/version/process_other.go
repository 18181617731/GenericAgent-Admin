//go:build !windows

package version

import "os/exec"

func hideChildWindow(cmd *exec.Cmd) {}

func detachChildProcess(cmd *exec.Cmd) {}
