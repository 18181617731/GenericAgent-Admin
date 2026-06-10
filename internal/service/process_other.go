//go:build !windows

package service

import (
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
)

func hideChildWindow(cmd *exec.Cmd) {}

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
		if err := p.Kill(); err != nil {
			return killed, err
		}
		killed = append(killed, row.pid)
	}
	return killed, nil
}

func listPythonProcesses() ([]processRow, error) {
	out, err := exec.Command("ps", "-eo", "pid=,ppid=,comm=,args=").Output()
	if err != nil {
		return nil, err
	}
	var rows []processRow
	for _, line := range strings.Split(string(out), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) < 4 {
			continue
		}
		pid, err := strconv.Atoi(fields[0])
		if err != nil || pid <= 0 {
			continue
		}
		ppid, _ := strconv.Atoi(fields[1])
		cmd := strings.Join(fields[3:], " ")
		lower := strings.ToLower(cmd + " " + fields[2])
		if !strings.Contains(lower, "python") && !strings.Contains(lower, "agentmain.py") && !strings.Contains(lower, "chat_worker") {
			continue
		}
		rows = append(rows, processRow{pid: pid, ppid: ppid, name: fields[2], commandLine: cmd})
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

func processAlive(pid int) bool {
	if pid <= 0 {
		return false
	}
	err := syscall.Kill(pid, 0)
	return err == nil
}
