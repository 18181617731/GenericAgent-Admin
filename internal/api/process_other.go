//go:build !windows

package api

import "os/exec"

func hideChildWindow(cmd *exec.Cmd) {}
