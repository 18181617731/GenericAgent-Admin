//go:build !windows

package clipboard

import "os/exec"

func hideConsoleWindow(_ *exec.Cmd) {}
