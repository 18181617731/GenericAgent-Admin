//go:build !windows

package service

import "os/exec"

func hideChildWindow(cmd *exec.Cmd) {}
