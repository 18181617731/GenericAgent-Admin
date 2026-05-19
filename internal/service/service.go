package service

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"sync"
)

type ServiceInfo struct {
	Name       string   `json:"name"`
	Kind       string   `json:"kind"`
	Command    []string `json:"command"`
	Running    bool     `json:"running"`
	PID        *int     `json:"pid"`
	ReturnCode *int     `json:"returncode"`
}

type runningProc struct {
	cmd *exec.Cmd
	ret *int
}

type Manager struct {
	GARoot      string
	BufferLines int
	mu          sync.Mutex
	procs       map[string]*runningProc
	buffers     map[string][]string
}

func NewManager(gaRoot string, bufferLines int) *Manager {
	if bufferLines <= 0 {
		bufferLines = 1000
	}
	return &Manager{GARoot: gaRoot, BufferLines: bufferLines, procs: map[string]*runningProc{}, buffers: map[string][]string{}}
}

func (m *Manager) SetRoot(root string, bufferLines int) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.GARoot = root
	if bufferLines > 0 {
		m.BufferLines = bufferLines
	}
}

func (m *Manager) python() string {
	cands := []string{}
	if runtime.GOOS == "windows" {
		cands = append(cands, filepath.Join(m.GARoot, ".venv", "Scripts", "python.exe"), filepath.Join(m.GARoot, "venv", "Scripts", "python.exe"))
	} else {
		cands = append(cands, filepath.Join(m.GARoot, ".venv", "bin", "python"), filepath.Join(m.GARoot, "venv", "bin", "python"))
	}
	for _, c := range cands {
		if st, err := os.Stat(c); err == nil && !st.IsDir() {
			return c
		}
	}
	return "python"
}

func excluded(name string) bool {
	switch name {
	case "chatapp_common.py":
		return true
	}
	return false
}

func existsFile(p string) bool {
	st, err := os.Stat(p)
	return err == nil && !st.IsDir()
}

func (m *Manager) addIfExists(out *[]ServiceInfo, name, kind string, command []string) {
	if len(command) == 0 {
		return
	}
	if existsFile(filepath.Join(m.GARoot, name)) {
		*out = append(*out, ServiceInfo{Name: filepath.ToSlash(name), Kind: kind, Command: command})
	}
}

func (m *Manager) Discover() []ServiceInfo {
	py := m.python()
	var out []ServiceInfo

	// GA native lifecycle entries. These are not under frontends/reflect but are essential to taking over GA.
	m.addIfExists(&out, "hub.pyw", "core", []string{py, "hub.pyw"})
	m.addIfExists(&out, "launch.py", "core", []string{py, "launch.py"})
	m.addIfExists(&out, "agent_loop.py", "core", []string{py, "agent_loop.py"})
	m.addIfExists(&out, filepath.Join("reflect", "scheduler.py"), "task", []string{py, filepath.ToSlash(filepath.Join("reflect", "scheduler.py"))})
	m.addIfExists(&out, filepath.Join("reflect", "autonomous.py"), "reflect", []string{py, "agentmain.py", "--reflect", filepath.ToSlash(filepath.Join("reflect", "autonomous.py"))})
	m.addIfExists(&out, filepath.Join("reflect", "goal_mode.py"), "reflect", []string{py, "agentmain.py", "--reflect", filepath.ToSlash(filepath.Join("reflect", "goal_mode.py"))})

	reflectDir := filepath.Join(m.GARoot, "reflect")
	if entries, err := os.ReadDir(reflectDir); err == nil {
		sort.Slice(entries, func(i, j int) bool { return strings.ToLower(entries[i].Name()) < strings.ToLower(entries[j].Name()) })
		seen := map[string]bool{}
		for _, s := range out {
			seen[s.Name] = true
		}
		for _, e := range entries {
			name := e.Name()
			if e.IsDir() || !strings.HasSuffix(name, ".py") || strings.HasPrefix(name, "_") || excluded(name) {
				continue
			}
			rel := filepath.ToSlash(filepath.Join("reflect", name))
			if seen[rel] {
				continue
			}
			out = append(out, ServiceInfo{Name: rel, Kind: "reflect", Command: []string{py, "agentmain.py", "--reflect", rel}})
		}
	}
	frontDir := filepath.Join(m.GARoot, "frontends")
	if entries, err := os.ReadDir(frontDir); err == nil {
		sort.Slice(entries, func(i, j int) bool { return strings.ToLower(entries[i].Name()) < strings.ToLower(entries[j].Name()) })
		for _, e := range entries {
			name := e.Name()
			lowerName := strings.ToLower(name)
			stem := strings.TrimSuffix(lowerName, ".py")
			if e.IsDir() || !strings.HasSuffix(lowerName, ".py") || !strings.HasSuffix(stem, "app") || strings.HasPrefix(name, "_") || excluded(name) {
				continue
			}
			rel := filepath.ToSlash(filepath.Join("frontends", name))
			cmd := []string{py, rel}
			if strings.Contains(strings.ToLower(name), "stapp") {
				cmd = []string{py, "-m", "streamlit", "run", rel, "--server.headless=true"}
			}
			out = append(out, ServiceInfo{Name: rel, Kind: "frontend", Command: cmd})
		}
	}
	for i := range out {
		out[i] = m.withState(out[i])
	}
	return out
}

func (m *Manager) withState(s ServiceInfo) ServiceInfo {
	m.mu.Lock()
	defer m.mu.Unlock()
	if p, ok := m.procs[s.Name]; ok {
		if p.cmd.Process != nil && p.ret == nil {
			pid := p.cmd.Process.Pid
			s.PID = &pid
			s.Running = true
		}
		if p.ret != nil {
			s.ReturnCode = p.ret
		}
	}
	return s
}

func (m *Manager) Find(name string) (ServiceInfo, bool) {
	for _, s := range m.Discover() {
		if s.Name == name {
			return s, true
		}
	}
	return ServiceInfo{}, false
}

func (m *Manager) Start(name string) (ServiceInfo, error) {
	s, ok := m.Find(name)
	if !ok {
		return s, errors.New("service not found")
	}
	m.mu.Lock()
	if p, ok := m.procs[name]; ok && p.cmd.Process != nil && p.ret == nil {
		m.mu.Unlock()
		return m.withState(s), nil
	}
	m.buffers[name] = []string{fmt.Sprintf("$ %s", strings.Join(s.Command, " "))}
	m.mu.Unlock()
	cmd := exec.Command(s.Command[0], s.Command[1:]...)
	cmd.Dir = m.GARoot
	hideChildWindow(cmd)
	cmd.Env = append(os.Environ(), "PYTHONUNBUFFERED=1")
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return s, err
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return s, err
	}
	if err := cmd.Start(); err != nil {
		return s, err
	}
	m.mu.Lock()
	m.procs[name] = &runningProc{cmd: cmd}
	m.mu.Unlock()
	go m.readPipe(name, stdout)
	go m.readPipe(name, stderr)
	go func() {
		err := cmd.Wait()
		code := 0
		if err != nil {
			if ee, ok := err.(*exec.ExitError); ok {
				code = ee.ExitCode()
			} else {
				code = -1
			}
		}
		m.mu.Lock()
		if p := m.procs[name]; p != nil {
			p.ret = &code
		}
		m.appendLocked(name, fmt.Sprintf("[process exited rc=%d]", code))
		m.mu.Unlock()
	}()
	return m.withState(s), nil
}

func (m *Manager) readPipe(name string, r io.Reader) {
	s := bufio.NewScanner(r)
	buf := make([]byte, 0, 64*1024)
	s.Buffer(buf, 1024*1024)
	for s.Scan() {
		m.mu.Lock()
		m.appendLocked(name, s.Text())
		m.mu.Unlock()
	}
}

func (m *Manager) appendLocked(name, line string) {
	b := append(m.buffers[name], line)
	if len(b) > m.BufferLines {
		b = b[len(b)-m.BufferLines:]
	}
	m.buffers[name] = b
}

func (m *Manager) Stop(name string) error {
	m.mu.Lock()
	p := m.procs[name]
	m.mu.Unlock()
	if p == nil || p.cmd.Process == nil || p.ret != nil {
		return nil
	}
	if err := p.cmd.Process.Kill(); err != nil {
		return err
	}
	return nil
}

func (m *Manager) StopAll() {
	for _, s := range m.Discover() {
		_ = m.Stop(s.Name)
	}
}

func (m *Manager) Logs(name string, lines int) []string {
	m.mu.Lock()
	defer m.mu.Unlock()
	b := append([]string{}, m.buffers[name]...)
	if lines > 0 && len(b) > lines {
		b = b[len(b)-lines:]
	}
	return b
}

func (m *Manager) Summary() map[string]int {
	items := m.Discover()
	running := 0
	for _, s := range items {
		if s.Running {
			running++
		}
	}
	return map[string]int{"total": len(items), "running": running, "stopped": len(items) - running}
}
