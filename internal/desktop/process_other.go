//go:build !windows

package desktop

import "os/exec"

func hideChildWindow(cmd *exec.Cmd) {}
