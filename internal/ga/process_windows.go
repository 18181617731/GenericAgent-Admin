//go:build windows

package ga

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
)

func hideChildWindow(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: 0x08000000}
}

func windowlessPythonPath(pythonPath string) string {
	base := strings.ToLower(filepath.Base(pythonPath))
	if base != "python.exe" && base != "python3.exe" {
		return pythonPath
	}
	candidate := filepath.Join(filepath.Dir(pythonPath), "pythonw.exe")
	if st, err := os.Stat(candidate); err == nil && !st.IsDir() {
		return candidate
	}
	return pythonPath
}
