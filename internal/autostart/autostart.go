package autostart

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
)

const appID = "com.genericagent.admin"
const runName = "GenericAgent Admin"

type Status struct {
	Supported bool   `json:"supported"`
	Enabled   bool   `json:"enabled"`
	Method    string `json:"method"`
	Path      string `json:"path"`
	Target    string `json:"target"`
	Detail    string `json:"detail,omitempty"`
}

func StatusForCurrent() Status {
	target, _ := os.Executable()
	return StatusFor(target)
}

func EnableCurrent() (Status, error) {
	target, err := os.Executable()
	if err != nil {
		return Status{}, err
	}
	return Enable(target)
}

func DisableCurrent() (Status, error) {
	target, _ := os.Executable()
	return Disable(target)
}

func StatusFor(target string) Status {
	s := Status{Target: target}
	supported := runtime.GOOS == "windows" || runtime.GOOS == "darwin"
	s.Supported = supported
	if !supported {
		s.Method = "unsupported"
		s.Detail = "Only Windows HKCU Run and macOS LaunchAgent are supported"
		return s
	}
	if runtime.GOOS == "windows" {
		s.Method = "HKCU Run"
		s.Path = `HKCU\Software\Microsoft\Windows\CurrentVersion\Run`
		out, err := exec.Command("reg", "query", `HKCU\Software\Microsoft\Windows\CurrentVersion\Run`, "/v", runName).CombinedOutput()
		if err != nil {
			s.Enabled = false
			return s
		}
		text := string(out)
		s.Detail = strings.TrimSpace(text)
		s.Enabled = target == "" || strings.Contains(strings.ToLower(text), strings.ToLower(target)) || strings.Contains(strings.ToLower(text), strings.ToLower(filepath.Base(target)))
		return s
	}
	p := launchAgentPath()
	s.Method = "LaunchAgent"
	s.Path = p
	b, err := os.ReadFile(p)
	if err != nil {
		s.Enabled = false
		return s
	}
	text := string(b)
	s.Detail = p
	s.Enabled = target == "" || strings.Contains(text, target)
	return s
}

func Enable(target string) (Status, error) {
	if target == "" {
		return Status{}, errors.New("empty executable path")
	}
	if runtime.GOOS == "windows" {
		cmd := fmt.Sprintf("\"%s\"", target)
		out, err := exec.Command("reg", "add", `HKCU\Software\Microsoft\Windows\CurrentVersion\Run`, "/v", runName, "/t", "REG_SZ", "/d", cmd, "/f").CombinedOutput()
		if err != nil {
			return StatusFor(target), fmt.Errorf("reg add failed: %v: %s", err, strings.TrimSpace(string(out)))
		}
		return StatusFor(target), nil
	}
	if runtime.GOOS == "darwin" {
		p := launchAgentPath()
		if err := os.MkdirAll(filepath.Dir(p), 0755); err != nil {
			return StatusFor(target), err
		}
		plist := fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
  <key>Label</key><string>%s</string>
  <key>ProgramArguments</key>
  <array><string>%s</string></array>
  <key>RunAtLoad</key><true/>
  <key>KeepAlive</key><false/>
  <key>WorkingDirectory</key><string>%s</string>
  <key>StandardOutPath</key><string>%s</string>
  <key>StandardErrorPath</key><string>%s</string>
</dict>
</plist>
`, appID, xmlEscape(target), xmlEscape(filepath.Dir(target)), xmlEscape(filepath.Join(os.TempDir(), "genericagent-admin.out.log")), xmlEscape(filepath.Join(os.TempDir(), "genericagent-admin.err.log")))
		if err := os.WriteFile(p, []byte(plist), 0644); err != nil {
			return StatusFor(target), err
		}
		_ = exec.Command("launchctl", "bootout", fmt.Sprintf("gui/%d", os.Getuid()), p).Run()
		_ = exec.Command("launchctl", "bootstrap", fmt.Sprintf("gui/%d", os.Getuid()), p).Run()
		_ = exec.Command("launchctl", "enable", fmt.Sprintf("gui/%d/%s", os.Getuid(), appID)).Run()
		return StatusFor(target), nil
	}
	return StatusFor(target), errors.New("autostart is only supported on Windows and macOS")
}

func Disable(target string) (Status, error) {
	if runtime.GOOS == "windows" {
		out, err := exec.Command("reg", "delete", `HKCU\Software\Microsoft\Windows\CurrentVersion\Run`, "/v", runName, "/f").CombinedOutput()
		if err != nil && !strings.Contains(strings.ToLower(string(out)), "unable") {
			return StatusFor(target), fmt.Errorf("reg delete failed: %v: %s", err, strings.TrimSpace(string(out)))
		}
		return StatusFor(target), nil
	}
	if runtime.GOOS == "darwin" {
		p := launchAgentPath()
		_ = exec.Command("launchctl", "bootout", fmt.Sprintf("gui/%d", os.Getuid()), p).Run()
		if err := os.Remove(p); err != nil && !os.IsNotExist(err) {
			return StatusFor(target), err
		}
		return StatusFor(target), nil
	}
	return StatusFor(target), errors.New("autostart is only supported on Windows and macOS")
}

func launchAgentPath() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, "Library", "LaunchAgents", appID+".plist")
}

func xmlEscape(s string) string {
	s = strings.ReplaceAll(s, "&", "&amp;")
	s = strings.ReplaceAll(s, "<", "&lt;")
	s = strings.ReplaceAll(s, ">", "&gt;")
	s = strings.ReplaceAll(s, "\"", "&quot;")
	s = strings.ReplaceAll(s, "'", "&apos;")
	return s
}
