//go:build windows

package version

import (
	"os/exec"
	"testing"
)

func TestDetachedUpdateProcessBreaksAwayFromParentJob(t *testing.T) {
	cmd := exec.Command("cmd.exe", "/C", "exit", "0")
	detachChildProcess(cmd)
	const requiredFlags = 0x00000008 | 0x00000200 | 0x01000000
	var flags uint32
	if cmd.SysProcAttr != nil {
		flags = cmd.SysProcAttr.CreationFlags
	}
	if flags&requiredFlags != requiredFlags {
		t.Fatalf("detached update flags = %#x, want all %#x", flags, requiredFlags)
	}
}
