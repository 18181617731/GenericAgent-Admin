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
	"time"
)

var (
	ErrServiceNotFound = errors.New("service not found")
	ErrWorkflowManaged = errors.New("service lifecycle is managed by its Goal or checklist workflow")
)

type ServiceInfo struct {
	Name       string   `json:"name"`
	Kind       string   `json:"kind"`
	Command    []string `json:"command"`
	WorkDir    string   `json:"workdir"`
	Running    bool     `json:"running"`
	PID        *int     `json:"pid"`
	ReturnCode *int     `json:"returncode"`
	StartedAt  string   `json:"started_at,omitempty"`
	Autostart  bool     `json:"autostart,omitempty"`
	ModelNo    *int     `json:"model_no,omitempty"`
}

type runningProc struct {
	cmd       *exec.Cmd
	ret       *int
	startedAt time.Time
	stopping  bool
}

type LogEvent struct {
	Reset bool
	Lines []string
	Line  string
}

type Manager struct {
	GARoot          string
	EffectivePython string
	UsageDir        string
	BufferLines     int
	mu              sync.Mutex
	procs           map[string]*runningProc
	buffers         map[string][]string
	subscribers     map[string]map[chan LogEvent]struct{}
	externalPIDs    map[string]int
	externalPIDsAt  time.Time
}

const (
	serviceRestartGrace     = 500 * time.Millisecond
	externalProcessStateTTL = 2 * time.Second
)

// HasRunningProcesses reports whether this manager currently owns at least one
// live process. It deliberately only considers processes started or adopted by
// this manager; unrelated GenericAgent processes are outside its ownership.
func (m *Manager) HasRunningProcesses() bool {
	if m == nil {
		return false
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, p := range m.procs {
		if p == nil || p.cmd == nil || p.cmd.Process == nil || p.ret != nil {
			continue
		}
		if processAlive(p.cmd.Process.Pid) {
			return true
		}
	}
	return false
}

func NewManager(gaRoot string, bufferLines int) *Manager {
	return NewManagerWithPython(gaRoot, "", bufferLines)
}

func NewManagerWithPython(gaRoot string, effectivePython string, bufferLines int) *Manager {
	if bufferLines <= 0 {
		bufferLines = 1000
	}
	return &Manager{
		GARoot:          gaRoot,
		EffectivePython: strings.TrimSpace(effectivePython),
		BufferLines:     bufferLines,
		procs:           map[string]*runningProc{},
		buffers:         map[string][]string{},
		subscribers:     map[string]map[chan LogEvent]struct{}{},
		externalPIDs:    map[string]int{},
	}
}

func (m *Manager) SetRoot(root string, effectivePython string, bufferLines int) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.GARoot = root
	m.EffectivePython = strings.TrimSpace(effectivePython)
	m.externalPIDs = map[string]int{}
	m.externalPIDsAt = time.Time{}
	if bufferLines > 0 {
		m.BufferLines = bufferLines
	}
}

func (m *Manager) SetUsageDir(directory string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.UsageDir = strings.TrimSpace(directory)
}

func (m *Manager) python() string {
	if py := strings.TrimSpace(m.EffectivePython); py != "" {
		return py
	}
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

func serviceExecutable(executable string) string {
	if runtime.GOOS != "windows" {
		return executable
	}
	base := strings.ToLower(filepath.Base(executable))
	if base == "python.exe" || base == "python3.exe" {
		windowless := filepath.Join(filepath.Dir(executable), "pythonw.exe")
		if existsFile(windowless) {
			return windowless
		}
		return executable
	}
	if base == "python" || base == "python3" {
		if windowless, err := exec.LookPath(base + "w"); err == nil {
			return windowless
		}
	}
	return executable
}

func excluded(name string) bool {
	switch name {
	case "chatapp_common.py", "goal_mode.py", "watchdog.py":
		return true
	}
	return false
}

func workflowManagedService(name string) bool {
	switch filepath.ToSlash(name) {
	case "reflect/agent_team_worker.py", "reflect/checklist_master.py", "reflect/goal_mode.py":
		return true
	}
	return false
}

func SupportsManualLifecycle(s ServiceInfo) bool {
	return !workflowManagedService(s.Name)
}

func SupportsModelConfiguration(s ServiceInfo) bool {
	return s.Kind == "reflect" && !workflowManagedService(s.Name)
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
		*out = append(*out, ServiceInfo{Name: filepath.ToSlash(name), Kind: kind, Command: command, WorkDir: m.GARoot})
	}
}

func (m *Manager) Discover() []ServiceInfo {
	py := m.python()
	var out []ServiceInfo

	// GA native lifecycle entries. These are not under frontends/reflect but are essential to taking over GA.
	m.addIfExists(&out, "hub.pyw", "core", []string{py, "hub.pyw"})
	m.addIfExists(&out, "launch.py", "core", []string{py, "launch.py"})
	m.addIfExists(&out, "agent_loop.py", "core", []string{py, "agent_loop.py"})
	m.addIfExists(&out, filepath.Join("reflect", "scheduler.py"), "reflect", []string{py, "agentmain.py", "--reflect", filepath.ToSlash(filepath.Join("reflect", "scheduler.py"))})
	m.addIfExists(&out, filepath.Join("reflect", "autonomous.py"), "reflect", []string{py, "agentmain.py", "--reflect", filepath.ToSlash(filepath.Join("reflect", "autonomous.py"))})
	m.addIfExists(&out, filepath.Join("reflect", "goal_mode.py"), "reflect", []string{py, "agentmain.py", "--reflect", filepath.ToSlash(filepath.Join("reflect", "goal_mode.py"))})
	m.addIfExists(&out, filepath.Join("reflect", "watchdog.py"), "guardian", []string{py, filepath.ToSlash(filepath.Join("reflect", "watchdog.py"))})

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
			out = append(out, ServiceInfo{Name: rel, Kind: "reflect", Command: []string{py, "agentmain.py", "--reflect", rel}, WorkDir: m.GARoot})
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
			out = append(out, ServiceInfo{Name: rel, Kind: "frontend", Command: cmd, WorkDir: m.GARoot})
		}
	}
	externalPIDs := m.externalServicePIDs(out)
	for i := range out {
		out[i] = m.withState(out[i])
		if !out[i].Running {
			if pid := externalPIDs[out[i].Name]; pid > 0 {
				out[i].Running = true
				out[i].PID = &pid
				out[i].ReturnCode = nil
			}
		}
	}
	return out
}

func (m *Manager) withState(s ServiceInfo) ServiceInfo {
	m.mu.Lock()
	defer m.mu.Unlock()
	if p, ok := m.procs[s.Name]; ok {
		if p.cmd.Process != nil && p.ret == nil {
			pid := p.cmd.Process.Pid
			if processAlive(pid) {
				s.PID = &pid
				s.Running = true
				if !p.startedAt.IsZero() {
					s.StartedAt = p.startedAt.Format(time.RFC3339)
				}
			} else {
				code := -1
				p.ret = &code
				m.appendLocked(s.Name, fmt.Sprintf("[process exited: pid %d is no longer alive]", pid))
			}
		}
		if p.ret != nil {
			s.ReturnCode = p.ret
		}
	}
	return s
}

func (m *Manager) externalServicePIDs(services []ServiceInfo) map[string]int {
	now := time.Now()
	m.mu.Lock()
	if !m.externalPIDsAt.IsZero() && now.Sub(m.externalPIDsAt) < externalProcessStateTTL {
		result := cloneServicePIDs(m.externalPIDs)
		m.mu.Unlock()
		return result
	}
	m.mu.Unlock()

	scanned := m.scanExternalServicePIDs(services)
	m.mu.Lock()
	m.externalPIDs = scanned
	m.externalPIDsAt = now
	result := cloneServicePIDs(scanned)
	m.mu.Unlock()
	return result
}

func cloneServicePIDs(source map[string]int) map[string]int {
	result := make(map[string]int, len(source))
	for name, pid := range source {
		result[name] = pid
	}
	return result
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
	return m.StartWithParams(name, nil)
}

func buildServiceArgs(s ServiceInfo, params map[string]string) ([]string, error) {
	cmdArgs := append([]string{}, s.Command[1:]...)
	if len(params) == 0 {
		return cmdArgs, nil
	}
	if s.Kind != "reflect" {
		return cmdArgs, nil
	}
	llmNo := strings.TrimSpace(params["llm_no"])
	if llmNo == "" {
		return cmdArgs, nil
	}
	for _, ch := range llmNo {
		if ch < '0' || ch > '9' {
			return nil, fmt.Errorf("invalid llm_no %q: must be a non-negative integer", llmNo)
		}
	}
	return append(cmdArgs, "--llm_no", llmNo), nil
}

func (m *Manager) StartWithParams(name string, params map[string]string) (ServiceInfo, error) {
	s, ok := m.Find(name)
	if !ok {
		return s, ErrServiceNotFound
	}
	if !SupportsManualLifecycle(s) {
		return s, ErrWorkflowManaged
	}
	s.Running = false
	s.PID = nil
	s.ReturnCode = nil
	s.StartedAt = ""
	m.mu.Lock()
	if p, ok := m.procs[name]; ok && p.cmd.Process != nil && p.ret == nil {
		pid := p.cmd.Process.Pid
		if processAlive(pid) {
			m.mu.Unlock()
			return m.withState(s), nil
		}
		code := -1
		p.ret = &code
		m.appendLocked(name, fmt.Sprintf("[process exited: pid %d is no longer alive]", pid))
	}

	cmdArgs, err := buildServiceArgs(s, params)
	if err != nil {
		m.mu.Unlock()
		return s, err
	}

	m.resetLocked(name, []string{fmt.Sprintf("$ %s %s", s.Command[0], strings.Join(cmdArgs, " "))})
	m.mu.Unlock()
	if killed, err := m.stopConflictingService(s); err != nil {
		return s, err
	} else if len(killed) > 0 {
		m.mu.Lock()
		for _, pid := range killed {
			m.appendLocked(name, fmt.Sprintf("[force restart] stopped existing instance pid=%d", pid))
		}
		m.mu.Unlock()
		// Give singleton locks/ports a short moment to be released before starting the managed instance.
		time.Sleep(serviceRestartGrace)
	}
	cmd := exec.Command(serviceExecutable(s.Command[0]), cmdArgs...)
	cmd.Dir = m.GARoot
	hideChildWindow(cmd)
	cmd.Env = m.serviceEnv(s.Name, params)
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
	m.procs[name] = &runningProc{cmd: cmd, startedAt: time.Now()}
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
			if p.stopping {
				code = 0
			}
			p.ret = &code
		}
		m.appendLocked(name, fmt.Sprintf("[process exited rc=%d]", code))
		m.mu.Unlock()
	}()
	return m.withState(s), nil
}

func (m *Manager) serviceEnv(name string, params map[string]string) []string {
	env := append([]string{}, os.Environ()...)
	env = append(env, "PYTHONUNBUFFERED=1", "PYTHONIOENCODING=utf-8", "PYTHONUTF8=1")
	usageDir := strings.TrimSpace(m.UsageDir)
	if usageDir == "" {
		return env
	}
	channel := "service"
	switch filepath.ToSlash(name) {
	case "reflect/scheduler.py":
		channel = "scheduled_task"
	case "reflect/autonomous.py":
		channel = "autonomous"
	case "reflect/goal_mode.py":
		channel = "goal"
	}
	env = replaceEnv(env, "GA_ADMIN_USAGE_DIR", usageDir)
	env = replaceEnv(env, "GA_ADMIN_USAGE_CHANNEL", channel)
	env = replaceEnv(env, "GA_ADMIN_USAGE_SOURCE", filepath.ToSlash(name))
	env = replaceEnv(env, "GA_ADMIN_USAGE_SESSION_ID", "service-"+sanitizeEnvValue(name))
	env = replaceEnv(env, "GA_ADMIN_USAGE_SESSION_NAME", filepath.ToSlash(name))
	if params != nil {
		if llmNo := strings.TrimSpace(params["llm_no"]); llmNo != "" {
			env = replaceEnv(env, "GA_ADMIN_LLM_NO", llmNo)
		}
	}
	return env
}

func replaceEnv(env []string, key, value string) []string {
	prefix := key + "="
	for index, item := range env {
		if strings.HasPrefix(item, prefix) {
			env[index] = prefix + value
			return env
		}
	}
	return append(env, prefix+value)
}

func sanitizeEnvValue(value string) string {
	value = filepath.ToSlash(strings.TrimSpace(value))
	var builder strings.Builder
	for _, char := range value {
		if char >= 'a' && char <= 'z' || char >= 'A' && char <= 'Z' || char >= '0' && char <= '9' || char == '-' || char == '_' {
			builder.WriteRune(char)
		}
	}
	if builder.Len() == 0 {
		return "service"
	}
	return builder.String()
}

const maxLogLineBytes = 1024 * 1024

func (m *Manager) readPipe(name string, r io.Reader) {
	reader := bufio.NewReaderSize(r, 64*1024)
	line := make([]byte, 0, 64*1024)
	truncated := false
	for {
		chunk, err := reader.ReadSlice('\n')
		if len(chunk) > 0 {
			if len(line) < maxLogLineBytes {
				remaining := maxLogLineBytes - len(line)
				if len(chunk) > remaining {
					line = append(line, chunk[:remaining]...)
					truncated = true
				} else {
					line = append(line, chunk...)
				}
			} else {
				truncated = true
			}
		}
		if err == nil {
			m.appendLogLine(name, line, truncated)
			line = line[:0]
			truncated = false
			continue
		}
		if errors.Is(err, bufio.ErrBufferFull) {
			continue
		}
		if len(line) > 0 || truncated {
			m.appendLogLine(name, line, truncated)
		}
		return
	}
}

func (m *Manager) appendLogLine(name string, line []byte, truncated bool) {
	text := strings.TrimRight(string(line), "\r\n")
	if truncated {
		text += " [truncated]"
	}
	m.mu.Lock()
	m.appendLocked(name, text)
	m.mu.Unlock()
}

func (m *Manager) appendLocked(name, line string) {
	b := append(m.buffers[name], line)
	if len(b) > m.BufferLines {
		b = b[len(b)-m.BufferLines:]
	}
	m.buffers[name] = b
	m.publishLocked(name, LogEvent{Line: line})
}

func (m *Manager) resetLocked(name string, lines []string) {
	m.buffers[name] = append([]string{}, lines...)
	m.publishLocked(name, LogEvent{Reset: true, Lines: append([]string{}, lines...)})
}

func (m *Manager) publishLocked(name string, event LogEvent) {
	for subscriber := range m.subscribers[name] {
		select {
		case subscriber <- event:
		default:
			// A slow client must not block the process pipe. Replace its stale
			// backlog with a fresh snapshot so it catches up without losing state.
			for len(subscriber) > 0 {
				<-subscriber
			}
			snapshot := append([]string{}, m.buffers[name]...)
			subscriber <- LogEvent{Reset: true, Lines: snapshot}
		}
	}
}

func (m *Manager) Subscribe(name string, lines int) ([]string, <-chan LogEvent, func()) {
	m.mu.Lock()
	snapshot := append([]string{}, m.buffers[name]...)
	if lines > 0 && len(snapshot) > lines {
		snapshot = snapshot[len(snapshot)-lines:]
	}
	subscriber := make(chan LogEvent, 256)
	if m.subscribers[name] == nil {
		m.subscribers[name] = map[chan LogEvent]struct{}{}
	}
	m.subscribers[name][subscriber] = struct{}{}
	m.mu.Unlock()

	var once sync.Once
	cancel := func() {
		once.Do(func() {
			m.mu.Lock()
			delete(m.subscribers[name], subscriber)
			if len(m.subscribers[name]) == 0 {
				delete(m.subscribers, name)
			}
			close(subscriber)
			m.mu.Unlock()
		})
	}
	return snapshot, subscriber, cancel
}

func (m *Manager) Stop(name string) error {
	s, ok := m.Find(name)
	if !ok {
		return errors.New("service not found")
	}
	m.mu.Lock()
	p := m.procs[name]
	managedPID := 0
	if p != nil && p.cmd.Process != nil && p.ret == nil {
		managedPID = p.cmd.Process.Pid
	}
	if managedPID <= 0 || !processAlive(managedPID) {
		m.mu.Unlock()
		if s.Running {
			_, err := m.stopConflictingService(s)
			return err
		}
		return nil
	}
	pid := managedPID
	p.stopping = true
	m.appendLocked(name, fmt.Sprintf("[admin] stopping pid %d", pid))
	m.mu.Unlock()

	if pid <= 0 {
		m.mu.Lock()
		p.stopping = false
		m.mu.Unlock()
		return errors.New("invalid process pid")
	}
	if !processAlive(pid) {
		m.mu.Lock()
		if current := m.procs[name]; current == p {
			code := 0
			p.ret = &code
		}
		m.appendLocked(name, fmt.Sprintf("[admin] pid %d is already stopped", pid))
		m.mu.Unlock()
		return nil
	}
	if err := stopManagedProcess(s.Kind, pid); err != nil {
		if !processAlive(pid) {
			m.mu.Lock()
			if current := m.procs[name]; current == p {
				code := 0
				p.ret = &code
			}
			m.mu.Unlock()
			return nil
		}
		m.mu.Lock()
		p.stopping = false
		m.mu.Unlock()
		return err
	}
	deadline := time.Now().Add(5 * time.Second)
	for processAlive(pid) && time.Now().Before(deadline) {
		time.Sleep(100 * time.Millisecond)
	}
	if processAlive(pid) {
		m.mu.Lock()
		p.stopping = false
		m.mu.Unlock()
		return fmt.Errorf("process %d still alive after stop", pid)
	}
	m.mu.Lock()
	if current := m.procs[name]; current == p {
		code := 0
		p.ret = &code
	}
	m.appendLocked(name, fmt.Sprintf("[admin] stopped pid %d", pid))
	m.mu.Unlock()
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
