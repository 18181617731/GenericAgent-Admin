//go:build !windows

package main

import (
	"context"
	"os/exec"
)

func newPortableBootstrapCommand(ctx context.Context, pythonExe, bootstrapPy string) *exec.Cmd {
	return exec.CommandContext(ctx, pythonExe, bootstrapPy)
}
