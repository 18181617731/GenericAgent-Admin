//go:build darwin

package autostart

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
)

func StatusFor(target, appRoot string) Status {
	s := Status{
		Supported: true,
		Method:    "LaunchAgent",
		Path:      launchAgentPath(),
		Target:    target,
	}
	b, err := os.ReadFile(s.Path)
	if err != nil {
		s.Enabled = false
		return s
	}
	text := string(b)
	s.Detail = s.Path
	if target == "" {
		s.Enabled = true
		return s
	}
	// Check that the plist contains the exact executable and config-root arguments.
	s.Enabled = containsString(text, xmlEscape(target)) &&
		containsString(text, xmlEscape(appRoot)) &&
		containsString(text, "--app-root")
	return s
}

func Enable(target, appRoot string) (Status, error) {
	if target == "" {
		return Status{Supported: true}, fmt.Errorf("empty executable path")
	}
	p := launchAgentPath()
	if err := os.MkdirAll(filepath.Dir(p), 0755); err != nil {
		return StatusFor(target, appRoot), err
	}
	workingDir := appRoot
	if workingDir == "" {
		workingDir = filepath.Dir(target)
	}
	plist := fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
  <key>Label</key><string>%s</string>
  <key>ProgramArguments</key>
  <array>
    <string>%s</string>
    <string>--no-browser</string>
    <string>--app-root</string>
    <string>%s</string>
  </array>
  <key>RunAtLoad</key><true/>
  <key>KeepAlive</key><false/>
  <key>WorkingDirectory</key><string>%s</string>
  <key>StandardOutPath</key><string>%s</string>
  <key>StandardErrorPath</key><string>%s</string>
</dict>
</plist>
`, appID,
		xmlEscape(target),
		xmlEscape(appRoot),
		xmlEscape(workingDir),
		xmlEscape(filepath.Join(os.TempDir(), "genericagent-admin.out.log")),
		xmlEscape(filepath.Join(os.TempDir(), "genericagent-admin.err.log")),
	)
	if err := writeFileAtomic(p, []byte(plist), 0644); err != nil {
		return StatusFor(target, appRoot), err
	}
	uid := fmt.Sprintf("gui/%d", os.Getuid())
	_ = launchctl("bootout", uid, p)
	_ = launchctl("bootstrap", uid, p)
	_ = launchctl("enable", fmt.Sprintf("%s/%s", uid, appID))
	return StatusFor(target, appRoot), nil
}

func Disable(target, appRoot string) (Status, error) {
	p := launchAgentPath()
	uid := fmt.Sprintf("gui/%d", os.Getuid())
	_ = launchctl("bootout", uid, p)
	if err := os.Remove(p); err != nil && !os.IsNotExist(err) {
		return StatusFor(target, appRoot), err
	}
	return StatusFor(target, appRoot), nil
}

func launchAgentPath() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, "Library", "LaunchAgents", appID+".plist")
}

func launchctl(args ...string) error {
	cmd := exec.Command("launchctl", args...)
	return cmd.Run()
}

// containsString reports whether text contains substr as a whole line content
// (used to verify the executable path appears inside a <string> tag).
func containsString(text, substr string) bool {
	return len(substr) > 0 && len(text) > 0 &&
		(indexOf(text, substr) >= 0)
}

func indexOf(s, substr string) int {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return i
		}
	}
	return -1
}
