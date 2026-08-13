// Package clipboard puts text on the system clipboard using whatever the
// platform ships with.
package clipboard

import (
	"fmt"
	"os/exec"
	"runtime"
	"strings"
)

// Copy puts text on the system clipboard, so the tray can hand over an address
// that is otherwise only visible in the logs.
func Copy(text string) error {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "windows":
		cmd = exec.Command("clip")
	case "darwin":
		cmd = exec.Command("pbcopy")
	default:
		tool, err := linuxClipboardTool()
		if err != nil {
			return err
		}
		cmd = tool
	}
	hideConsoleWindow(cmd)
	cmd.Stdin = strings.NewReader(text)
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("copy to clipboard: %w", err)
	}
	return nil
}

func linuxClipboardTool() (*exec.Cmd, error) {
	candidates := [][]string{
		{"wl-copy"},
		{"xclip", "-selection", "clipboard"},
		{"xsel", "--clipboard", "--input"},
	}
	for _, candidate := range candidates {
		if path, err := exec.LookPath(candidate[0]); err == nil {
			return exec.Command(path, candidate[1:]...), nil
		}
	}
	return nil, fmt.Errorf("copy to clipboard: install wl-copy, xclip, or xsel")
}
