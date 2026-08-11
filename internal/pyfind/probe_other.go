//go:build !windows

package pyfind

import "os/exec"

func hideProbeWindow(cmd *exec.Cmd) {}
