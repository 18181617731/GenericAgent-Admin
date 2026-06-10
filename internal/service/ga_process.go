package service

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

type GAProcessInfo struct {
	PID            int    `json:"pid"`
	PPID           int    `json:"ppid,omitempty"`
	Name           string `json:"name,omitempty"`
	CommandLine    string `json:"command_line"`
	ExecutablePath string `json:"executable_path,omitempty"`
	CreationDate   string `json:"creation_date,omitempty"`
	Managed        bool   `json:"managed"`
	ServiceName    string `json:"service_name,omitempty"`
	Kind           string `json:"kind"`
	Risk           string `json:"risk"`
	Reason         string `json:"reason,omitempty"`
	Killable       bool   `json:"killable"`
	Adoptable      bool   `json:"adoptable"`
}

type GAProcessSnapshot struct {
	Root      string          `json:"root"`
	ScannedAt string          `json:"scanned_at"`
	Items     []GAProcessInfo `json:"items"`
}

type GAProcessActionResult struct {
	PID         int            `json:"pid"`
	Killed      bool           `json:"killed,omitempty"`
	Adopted     bool           `json:"adopted,omitempty"`
	ServiceName string         `json:"service_name,omitempty"`
	Process     *GAProcessInfo `json:"process,omitempty"`
	Message     string         `json:"message"`
}

func (m *Manager) ScanGAProcesses() (GAProcessSnapshot, error) {
	rows, err := listPythonProcesses()
	if err != nil {
		return GAProcessSnapshot{}, err
	}
	services := m.Discover()
	m.mu.Lock()
	managedPID := map[int]string{}
	for name, proc := range m.procs {
		if proc == nil || proc.cmd == nil || proc.cmd.Process == nil || proc.ret != nil {
			continue
		}
		managedPID[proc.cmd.Process.Pid] = name
	}
	m.mu.Unlock()

	var items []GAProcessInfo
	self := os.Getpid()
	for _, row := range rows {
		if row.pid <= 0 {
			continue
		}
		info, ok := m.classifyGAProcess(row, services, managedPID)
		if !ok {
			continue
		}
		info.Killable = row.pid != self && info.Risk != "admin"
		info.Adoptable = !info.Managed && info.ServiceName != "" && row.pid != self
		items = append(items, info)
	}
	sort.Slice(items, func(i, j int) bool {
		if items[i].Managed != items[j].Managed {
			return !items[i].Managed
		}
		if items[i].Risk != items[j].Risk {
			return items[i].Risk > items[j].Risk
		}
		return items[i].PID < items[j].PID
	})
	return GAProcessSnapshot{Root: m.GARoot, ScannedAt: time.Now().Format(time.RFC3339), Items: items}, nil
}

func (m *Manager) KillGAProcess(pid int) (GAProcessActionResult, error) {
	if pid <= 0 {
		return GAProcessActionResult{}, errors.New("pid required")
	}
	snap, err := m.ScanGAProcesses()
	if err != nil {
		return GAProcessActionResult{}, err
	}
	for _, item := range snap.Items {
		if item.PID != pid {
			continue
		}
		if item.Managed {
			return GAProcessActionResult{}, fmt.Errorf("pid %d is already managed as %s; use service stop", pid, item.ServiceName)
		}
		if !item.Killable {
			return GAProcessActionResult{}, fmt.Errorf("pid %d is not killable by GA process guard", pid)
		}
		p, err := os.FindProcess(pid)
		if err != nil {
			return GAProcessActionResult{}, err
		}
		if err := p.Kill(); err != nil {
			return GAProcessActionResult{}, err
		}
		return GAProcessActionResult{PID: pid, Killed: true, ServiceName: item.ServiceName, Process: &item, Message: fmt.Sprintf("terminated unmanaged GA process %d", pid)}, nil
	}
	return GAProcessActionResult{}, fmt.Errorf("pid %d is not a recognized GA process under %s", pid, m.GARoot)
}

func (m *Manager) AdoptGAProcess(pid int) (GAProcessActionResult, error) {
	if pid <= 0 {
		return GAProcessActionResult{}, errors.New("pid required")
	}
	snap, err := m.ScanGAProcesses()
	if err != nil {
		return GAProcessActionResult{}, err
	}
	for _, item := range snap.Items {
		if item.PID != pid {
			continue
		}
		if item.Managed {
			return GAProcessActionResult{}, fmt.Errorf("pid %d is already managed as %s", pid, item.ServiceName)
		}
		if !item.Adoptable || item.ServiceName == "" {
			return GAProcessActionResult{}, fmt.Errorf("pid %d cannot be mapped to a GA Admin service", pid)
		}
		// Cross-process adoption into os/exec.Cmd is intentionally not attempted:
		// Go cannot safely attach wait/log pipes to an already-running child owned by another parent.
		// Instead, Admin performs a controlled takeover: kill the orphan and restart the matching service.
		p, err := os.FindProcess(pid)
		if err != nil {
			return GAProcessActionResult{}, err
		}
		if err := p.Kill(); err != nil {
			return GAProcessActionResult{}, err
		}
		svc, err := m.Start(item.ServiceName)
		if err != nil {
			return GAProcessActionResult{PID: pid, Killed: true, ServiceName: item.ServiceName, Process: &item, Message: "orphan terminated but managed restart failed"}, err
		}
		return GAProcessActionResult{PID: pid, Killed: true, Adopted: true, ServiceName: svc.Name, Process: &item, Message: fmt.Sprintf("adopted %s by restarting it under GA Admin", svc.Name)}, nil
	}
	return GAProcessActionResult{}, fmt.Errorf("pid %d is not a recognized GA process under %s", pid, m.GARoot)
}

func (m *Manager) classifyGAProcess(row processRow, services []ServiceInfo, managedPID map[int]string) (GAProcessInfo, bool) {
	cmdRaw := strings.TrimSpace(row.commandLine)
	cmdLower := strings.ToLower(normalizePathText(cmdRaw))
	root := strings.ToLower(normalizePathText(m.GARoot))
	exeLower := strings.ToLower(normalizePathText(row.executablePath))
	if root == "" || (!strings.Contains(cmdLower, root) && !strings.Contains(exeLower, root) && !looksLikeGARelativeCommand(cmdLower)) {
		return GAProcessInfo{}, false
	}
	info := GAProcessInfo{PID: row.pid, PPID: row.ppid, Name: row.name, CommandLine: cmdRaw, ExecutablePath: row.executablePath, CreationDate: row.creationDate, Kind: "python", Risk: "unmanaged", Reason: "GA python process is outside GA Admin management"}
	if name, ok := managedPID[row.pid]; ok {
		info.Managed = true
		info.ServiceName = name
		info.Risk = "managed"
		info.Reason = "tracked by GA Admin service manager"
	}
	if strings.Contains(cmdLower, "agentmain.py") {
		info.Kind = "agentmain"
		info.Risk = "watch"
		info.Reason = "agentmain process can consume model tokens"
	}
	if strings.Contains(cmdLower, "--reflect") || strings.Contains(cmdLower, "reflect/autonomous.py") || strings.Contains(cmdLower, "reflect\\autonomous.py") {
		info.Kind = "reflect"
		info.Risk = "high"
		info.Reason = "reflect/autonomous process can keep consuming model tokens"
	}
	if strings.Contains(cmdLower, "chat_worker") {
		info.Kind = "chat_worker"
		if info.Managed {
			info.Risk = "managed"
		} else {
			info.Risk = "watch"
		}
		info.Reason = "chat worker process; avoid terminating live service unless confirmed"
	}
	if strings.Contains(cmdLower, "genericagent-admin") || strings.Contains(cmdLower, "ga-admin") {
		info.Kind = "admin"
		info.Risk = "admin"
		info.Reason = "GA Admin process"
	}
	for _, svc := range services {
		if commandLineMatchesService(cmdRaw, m.GARoot, svc.Command) {
			info.ServiceName = svc.Name
			if info.Kind == "python" {
				info.Kind = svc.Kind
			}
			if !info.Managed {
				info.Reason = "matches GA Admin service but is not tracked"
			}
			break
		}
	}
	if info.Managed {
		info.Risk = "managed"
	}
	return info, true
}

func looksLikeGARelativeCommand(cmd string) bool {
	return strings.Contains(cmd, "agentmain.py") || strings.Contains(cmd, "reflect/autonomous.py") || strings.Contains(cmd, "chat_worker") || strings.Contains(cmd, filepath.ToSlash(filepath.Join("reflect", "goal_mode.py")))
}
