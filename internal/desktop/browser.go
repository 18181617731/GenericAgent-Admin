package desktop

import (
	"os/exec"
	"runtime"
)

// openBrowser hands a URL to the system browser. It is the fallback for every
// platform and situation where a native window cannot be created.
func openBrowser(url string) {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "windows":
		cmd = exec.Command("rundll32", "url.dll,FileProtocolHandler", url)
	case "darwin":
		cmd = exec.Command("open", url)
	default:
		cmd = exec.Command("xdg-open", url)
	}
	hideChildWindow(cmd)
	_ = cmd.Start()
}
