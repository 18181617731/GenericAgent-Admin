//go:build windows

package service

import (
	"encoding/csv"
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

func findSharedHubPID(port int) (int, bool) {
	script := "$owners=Get-NetTCPConnection -State Listen -LocalPort " + strconv.Itoa(port) + " -ErrorAction SilentlyContinue | Select-Object -ExpandProperty OwningProcess -Unique; foreach ($procId in $owners) { $p=Get-CimInstance Win32_Process -Filter (\"ProcessId = \" + $procId) -ErrorAction SilentlyContinue; if ($null -ne $p) { [Console]::Out.WriteLine((\"{0}`t{1}\" -f $procId,$p.CommandLine)) } }"
	cmd := exec.Command("powershell", "-NoProfile", "-ExecutionPolicy", "Bypass", "-Command", script)
	hideChildWindow(cmd)
	out, err := cmd.Output()
	if err != nil {
		return 0, false
	}
	for _, line := range strings.Split(string(out), "\n") {
		parts := strings.SplitN(strings.TrimSpace(line), "\t", 2)
		if len(parts) != 2 || !isSharedHubCommandLine(parts[1]) {
			continue
		}
		pid, err := strconv.Atoi(strings.TrimSpace(parts[0]))
		if err == nil && pid > 0 {
			return pid, true
		}
	}
	return 0, false
}

type processRow struct {
	pid            int
	ppid           int
	name           string
	commandLine    string
	executablePath string
	creationDate   string
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
		var stopErr error
		if s.Kind == "guardian" {
			stopErr = killProcessTree(row.pid)
		} else {
			stopErr = p.Kill()
		}
		if stopErr != nil && processAlive(row.pid) {
			err := stopErr
			return killed, err
		}
		killed = append(killed, row.pid)
	}
	return killed, nil
}

func killProcessTree(pid int) error {
	cmd := exec.Command("taskkill", "/PID", strconv.Itoa(pid), "/T", "/F")
	hideChildWindow(cmd)
	return cmd.Run()
}

func stopManagedProcess(kind string, pid int) error {
	if kind == "guardian" {
		return killProcessTree(pid)
	}
	p, err := os.FindProcess(pid)
	if err != nil {
		return err
	}
	return p.Kill()
}

func (m *Manager) scanExternalServicePIDs(services []ServiceInfo) map[string]int {
	rows, err := listPythonProcesses()
	if err != nil {
		return map[string]int{}
	}
	result := map[string]int{}
	scores := map[string]int{}
	for _, row := range rows {
		for _, svc := range services {
			if !commandLineMatchesService(row.commandLine, m.GARoot, svc.Command) {
				continue
			}
			score := externalServiceMatchScore(row, svc)
			if score > scores[svc.Name] || score == scores[svc.Name] && (result[svc.Name] == 0 || row.pid < result[svc.Name]) {
				result[svc.Name] = row.pid
				scores[svc.Name] = score
			}
		}
	}
	return result
}

func externalServiceMatchScore(row processRow, svc ServiceInfo) int {
	score := 1
	expected := strings.ToLower(normalizePathText(serviceExecutable(svc.Command[0])))
	executable := strings.ToLower(normalizePathText(row.executablePath))
	commandLine := strings.ToLower(normalizePathText(row.commandLine))
	if executable == expected {
		score += 4
	}
	if strings.Contains(commandLine, expected) {
		score += 2
	}
	return score
}

func listPythonProcesses() ([]processRow, error) {
	ps := `$ErrorActionPreference='Stop'; Get-CimInstance Win32_Process | Where-Object { $_.CommandLine -and ($_.Name -match '^(python|pythonw)\.exe$' -or $_.CommandLine -match '(?i)python|agentmain\.py|chat_worker') } | Select-Object ProcessId,ParentProcessId,Name,CommandLine,ExecutablePath,CreationDate | ConvertTo-Csv -NoTypeInformation`
	cmd := exec.Command("powershell", "-NoProfile", "-ExecutionPolicy", "Bypass", "-Command", ps)
	out, err := cmd.Output()
	if err != nil {
		return nil, err
	}
	r := csv.NewReader(strings.NewReader(string(out)))
	recs, err := r.ReadAll()
	if err != nil {
		return nil, err
	}
	if len(recs) <= 1 {
		return nil, nil
	}
	var rows []processRow
	for _, rec := range recs[1:] {
		if len(rec) < 6 {
			continue
		}
		pid, _ := strconv.Atoi(strings.TrimSpace(rec[0]))
		ppid, _ := strconv.Atoi(strings.TrimSpace(rec[1]))
		if pid <= 0 {
			continue
		}
		rows = append(rows, processRow{
			pid:            pid,
			ppid:           ppid,
			name:           strings.TrimSpace(rec[2]),
			commandLine:    rec[3],
			executablePath: strings.TrimSpace(rec[4]),
			creationDate:   strings.TrimSpace(rec[5]),
		})
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
