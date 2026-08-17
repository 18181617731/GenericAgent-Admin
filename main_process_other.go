//go:build !windows

package main

import "os/exec"

func hideChildWindow(cmd *exec.Cmd) {}

func newPortableBootstrapCommand(python, script string) *exec.Cmd {
	return exec.Command(python, script)
}
