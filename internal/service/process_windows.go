//go:build windows

package service

import (
	"encoding/csv"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"unsafe"
)

func hideChildWindow(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: 0x08000000}
}

type processRow struct {
	pid         int
	commandLine string
}

func (m *Manager) stopConflictingService(s ServiceInfo) ([]int, error) {
	// Admin is the authority for all GA services. Before starting a managed service,
	// terminate any already-running external instance that matches the discovered
	// GA-root command/script, then restart it under Admin so PID/status/logs are tracked.
	if len(s.Command) < 2 {
		return nil, nil
	}
	rows, err := listPythonProcesses()
	if err != nil {
		return nil, err
	}
	self := os.Getpid()
	var killed []int
	for _, row := range rows {
		if row.pid <= 0 || row.pid == self {
			continue
		}
		if !commandLineMatchesService(row.commandLine, m.GARoot, s.Command) {
			continue
		}
		p, err := os.FindProcess(row.pid)
		if err != nil {
			continue
		}
		if err := p.Kill(); err != nil {
			return killed, err
		}
		killed = append(killed, row.pid)
	}
	return killed, nil
}

func listPythonProcesses() ([]processRow, error) {
	ps := `$ErrorActionPreference='Stop'; Get-CimInstance Win32_Process | Where-Object { $_.CommandLine -and ($_.Name -match '^(python|pythonw)\.exe$' -or $_.CommandLine -match '(?i)python') } | Select-Object ProcessId,CommandLine | ConvertTo-Csv -NoTypeInformation`
	cmd := exec.Command("powershell", "-NoProfile", "-ExecutionPolicy", "Bypass", "-Command", ps)
	hideChildWindow(cmd)
	out, err := cmd.Output()
	if err != nil {
		return nil, err
	}
	r := csv.NewReader(strings.NewReader(string(out)))
	var rows []processRow
	for i := 0; ; i++ {
		rec, err := r.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, err
		}
		if i == 0 || len(rec) < 2 {
			continue
		}
		pid, err := strconv.Atoi(strings.TrimSpace(rec[0]))
		if err != nil {
			continue
		}
		rows = append(rows, processRow{pid: pid, commandLine: rec[1]})
	}
	return rows, nil
}

func commandLineMatchesService(commandLine, gaRoot string, serviceCommand []string) bool {
	if len(serviceCommand) < 2 {
		return false
	}
	cmd := strings.ToLower(normalizePathText(commandLine))
	root := strings.ToLower(normalizePathText(filepath.Clean(gaRoot)))
	py := strings.ToLower(normalizePathText(filepath.Clean(serviceCommand[0])))

	// Require either the configured GA python executable or the GA root path to avoid
	// killing unrelated python processes that happen to use the same script name.
	if !strings.Contains(cmd, py) && !strings.Contains(cmd, root) {
		return false
	}

	for _, arg := range serviceCommand[1:] {
		if strings.HasPrefix(arg, "-") {
			if !strings.Contains(cmd, strings.ToLower(arg)) {
				return false
			}
			continue
		}
		if !strings.HasSuffix(strings.ToLower(arg), ".py") && !strings.HasSuffix(strings.ToLower(arg), ".pyw") {
			continue
		}
		if !commandLineContainsScript(cmd, gaRoot, arg) {
			return false
		}
	}
	return true
}

func commandLineContainsScript(cmd, gaRoot, scriptArg string) bool {
	script := filepath.Clean(filepath.Join(gaRoot, filepath.FromSlash(scriptArg)))
	scriptAbs := strings.ToLower(normalizePathText(script))
	scriptRel := strings.ToLower(normalizePathText(filepath.ToSlash(mustRel(gaRoot, script))))
	return commandLineContainsPathToken(cmd, scriptAbs) || commandLineContainsPathToken(cmd, scriptRel)
}

func commandLineContainsPathToken(cmd, token string) bool {
	if token == "" {
		return false
	}
	for _, field := range strings.Fields(cmd) {
		field = normalizePathText(strings.Trim(field, `'"`))
		if field == token {
			return true
		}
	}
	return false
}

func normalizePathText(s string) string {
	s = strings.ReplaceAll(s, "\\", "/")
	for strings.Contains(s, "//") {
		s = strings.ReplaceAll(s, "//", "/")
	}
	return strings.Trim(s, " \t\r\n\"")
}

func mustRel(base, target string) string {
	if rel, err := filepath.Rel(base, target); err == nil {
		return rel
	}
	return target
}

const (
	processQueryLimitedInformation = 0x1000
	stillActive                    = 259
)

func processAlive(pid int) bool {
	if pid <= 0 {
		return false
	}
	h, err := syscall.OpenProcess(processQueryLimitedInformation, false, uint32(pid))
	if err != nil {
		return false
	}
	defer syscall.CloseHandle(h)
	var code uint32
	r1, _, _ := syscall.NewLazyDLL("kernel32.dll").NewProc("GetExitCodeProcess").Call(uintptr(h), uintptr(unsafe.Pointer(&code)))
	return r1 != 0 && code == stillActive
}
