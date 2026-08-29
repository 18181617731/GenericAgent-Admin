//go:build windows

package api

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
)

func chooseDirectory(start string) (string, error) {
	ps := `$ErrorActionPreference='Stop'; Add-Type -AssemblyName System.Windows.Forms; $d = New-Object System.Windows.Forms.FolderBrowserDialog; $d.Description = 'Select GenericAgent directory'; $d.ShowNewFolderButton = $true; if ($env:GA_ADMIN_BROWSE_START -and (Test-Path -LiteralPath $env:GA_ADMIN_BROWSE_START)) { $d.SelectedPath = $env:GA_ADMIN_BROWSE_START }; if ($d.ShowDialog() -eq [System.Windows.Forms.DialogResult]::OK) { [Console]::OutputEncoding=[System.Text.Encoding]::UTF8; Write-Output $d.SelectedPath }`
	cmd := exec.Command("powershell", "-NoProfile", "-STA", "-Command", ps)
	hideChildWindow(cmd)
	cmd.Env = append(os.Environ(), "GA_ADMIN_BROWSE_START="+start)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("directory picker failed: %s", strings.TrimSpace(string(out)))
	}
	return strings.TrimSpace(string(out)), nil
}
