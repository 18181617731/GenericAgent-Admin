package ga

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"syscall"
	"time"
	"unicode/utf8"
)

const (
	goalStatePrefix        = "goal_admin_"
	goalStandardDir        = "goals"
	goalStandardStateFile  = "state.json"
	goalStandardOutputFile = "output.log"
	goalStandardEventsFile = "events.jsonl"
	maxGoalObjectiveBytes  = 16 * 1024
	maxGoalBudgetSeconds   = 30 * 24 * 60 * 60
	maxGoalTurns           = 10000
	defaultGoalOutputBytes = 64 * 1024
	maxGoalOutputBytes     = 1024 * 1024
)

// GoalState mirrors memory/goal_mode_sop.md and reflect/goal_mode.py state.
type GoalState struct {
	SchemaVersion int      `json:"schema_version,omitempty"`
	Domain        string   `json:"domain,omitempty"`
	Objective     string   `json:"objective"`
	BudgetSeconds int      `json:"budget_seconds"`
	StartTime     float64  `json:"start_time"`
	EndTime       *float64 `json:"end_time,omitempty"`
	TurnsUsed     int      `json:"turns_used"`
	MaxTurns      int      `json:"max_turns"`
	Status        string   `json:"status"`
	DonePrompt    string   `json:"done_prompt"`
	PID           int      `json:"pid,omitempty"`
	ID            string   `json:"id,omitempty"`
	StateFile     string   `json:"state_file,omitempty"`
	LogFile       string   `json:"log_file,omitempty"`
	ManagedBy     string   `json:"managed_by,omitempty"`
	OutputSource  string   `json:"output_source,omitempty"`
	LLMNo         *int     `json:"llm_no,omitempty"`
	PythonPath    string   `json:"python_path,omitempty"`
}

// GoalMeta is the Admin-Go console view of one Goal Mode run.
type GoalMeta struct {
	SchemaVersion    int          `json:"schema_version"`
	Contract         ContractMeta `json:"contract"`
	ID               string       `json:"id"`
	Objective        string       `json:"objective"`
	BudgetSeconds    int          `json:"budget_seconds"`
	StartTime        float64      `json:"start_time"`
	ElapsedSeconds   int          `json:"elapsed_seconds"`
	RemainingSeconds int          `json:"remaining_seconds"`
	BudgetPercent    int          `json:"budget_percent"`
	TurnsUsed        int          `json:"turns_used"`
	MaxTurns         int          `json:"max_turns"`
	TurnPercent      int          `json:"turn_percent"`
	Status           string       `json:"status"`
	PID              int          `json:"pid,omitempty"`
	Running          bool         `json:"running"`
	Origin           string       `json:"origin"`
	Managed          bool         `json:"managed"`
	PIDTrusted       bool         `json:"pid_trusted"`
	StopLevel        string       `json:"stop_level"`
	Actions          []string     `json:"actions"`
	RawStatus        string       `json:"raw_status,omitempty"`
	LastEvent        string       `json:"last_event,omitempty"`
	ErrorClass       string       `json:"error_class,omitempty"`
	StateFile        string       `json:"state_file"`
	LogFile          string       `json:"log_file"`
	LogExists        bool         `json:"log_exists"`
	MissingLog       bool         `json:"missing_log"`
	StateReadable    bool         `json:"state_readable"`
	LLMNo            *int         `json:"llm_no,omitempty"`
	PythonPath       string       `json:"python_path,omitempty"`
	ModTime          time.Time    `json:"mod_time"`
	EndTime          *float64     `json:"end_time,omitempty"`
}

type GoalStartOptions struct {
	Objective     string
	BudgetSeconds int
	MaxTurns      int
	LLMNo         *int
	PythonPath    string
}

func StartGoal(root string, opt GoalStartOptions) (GoalMeta, error) {
	root = strings.TrimSpace(root)
	if root == "" {
		return GoalMeta{}, errors.New("GA root is not configured")
	}
	if opt.BudgetSeconds <= 0 {
		return GoalMeta{}, errors.New("budget_seconds must be positive")
	}
	objective := strings.TrimSpace(opt.Objective)
	if objective == "" {
		return GoalMeta{}, errors.New("objective is required")
	}
	if len([]byte(objective)) > maxGoalObjectiveBytes {
		return GoalMeta{}, fmt.Errorf("objective exceeds %d bytes", maxGoalObjectiveBytes)
	}
	if opt.BudgetSeconds > maxGoalBudgetSeconds {
		return GoalMeta{}, fmt.Errorf("budget_seconds exceeds %d", maxGoalBudgetSeconds)
	}
	if opt.MaxTurns < 0 {
		return GoalMeta{}, errors.New("max_turns must be non-negative")
	}
	if opt.MaxTurns == 0 {
		opt.MaxTurns = 200
	}
	if opt.MaxTurns > maxGoalTurns {
		return GoalMeta{}, fmt.Errorf("max_turns exceeds %d", maxGoalTurns)
	}
	if opt.LLMNo != nil && *opt.LLMNo < 0 {
		return GoalMeta{}, errors.New("llm_no must be non-negative")
	}
	if !existsFile(filepath.Join(root, "agentmain.py")) {
		return GoalMeta{}, errors.New("agentmain.py not found under GA root")
	}
	if !existsFile(filepath.Join(root, "reflect", "goal_mode.py")) {
		return GoalMeta{}, errors.New("reflect/goal_mode.py not found under GA root")
	}
	id := newGoalID()
	goalDir := standardGoalDir(root, id)
	if err := os.MkdirAll(goalDir, 0755); err != nil {
		return GoalMeta{}, err
	}
	statePath := standardGoalStatePath(root, id)
	logPath := standardGoalOutputPath(root, id)
	pythonPath, err := goalPython(root, opt.PythonPath)
	if err != nil {
		return GoalMeta{}, err
	}
	state := GoalState{SchemaVersion: 1, Objective: strings.TrimSpace(opt.Objective), BudgetSeconds: opt.BudgetSeconds, StartTime: float64(time.Now().UnixNano()) / 1e9, TurnsUsed: 0, MaxTurns: opt.MaxTurns, Status: "running", DonePrompt: "", ID: id, StateFile: relGoalPath(root, statePath), LogFile: relGoalPath(root, logPath), LLMNo: opt.LLMNo, PythonPath: pythonPath}
	if err := writeGoalState(statePath, state); err != nil {
		return GoalMeta{}, err
	}
	logFile, err := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0644)
	if err != nil {
		_ = os.Remove(statePath)
		return GoalMeta{}, err
	}
	defer logFile.Close()

	args := []string{"agentmain.py", "--reflect", filepath.ToSlash(filepath.Join("reflect", "goal_mode.py"))}
	if opt.LLMNo != nil && *opt.LLMNo >= 0 {
		args = append(args, "--llm_no", strconv.Itoa(*opt.LLMNo))
	}
	cmd := exec.Command(pythonPath, args...)
	hideChildWindow(cmd)
	cmd.Dir = root
	cmd.Env = goalCommandEnv(os.Environ(), statePath)
	cmd.Stdout = logFile
	cmd.Stderr = logFile
	if err := cmd.Start(); err != nil {
		state.Status = "start_failed"
		now := float64(time.Now().UnixNano()) / 1e9
		state.EndTime = &now
		writeErr := writeGoalState(statePath, state)
		_, _ = fmt.Fprintf(logFile, "[goal start failed] %v\n", err)
		return GoalMeta{}, errors.Join(err, writeErr)
	}
	state.PID = cmd.Process.Pid
	if err := writeGoalState(statePath, state); err != nil {
		_ = killExactPID(state.PID)
		_ = cmd.Process.Release()
		return GoalMeta{}, err
	}
	_ = cmd.Process.Release()
	return metaFromState(root, id, statePath, logPath, state, time.Now()), nil
}

func ListGoals(root string) ([]GoalMeta, error) {
	files, err := goalStateFiles(root)
	if err != nil {
		return nil, err
	}
	items := make([]GoalMeta, 0, len(files))
	for _, f := range files {
		state, meta, err := readGoalState(f)
		if err != nil {
			items = append(items, corruptGoalMeta(root, f, err))
			continue
		}
		st, _ := os.Stat(f)
		id, origin, managed := goalIdentity(f, state)
		logPath := goalLogPath(root, f, id, state, managed)
		mod := time.Now()
		if st != nil {
			mod = st.ModTime()
		}
		goalMeta := metaFromState(root, id, f, logPath, state, mod)
		goalMeta.Contract = meta
		goalMeta.Origin = origin
		goalMeta.Managed = managed
		goalMeta.PIDTrusted = managed && state.PID > 0
		goalMeta.StopLevel = goalStopLevel(goalMeta, managed)
		goalMeta.Actions = goalActions(goalMeta)
		items = append(items, goalMeta)
	}
	sort.Slice(items, func(i, j int) bool { return items[i].ModTime.After(items[j].ModTime) })
	return items, nil
}

func standardGoalDir(root, id string) string {
	return filepath.Join(root, "temp", goalStandardDir, sanitizeGoalID(id))
}

func standardGoalStatePath(root, id string) string {
	return filepath.Join(standardGoalDir(root, id), goalStandardStateFile)
}

func standardGoalOutputPath(root, id string) string {
	return filepath.Join(standardGoalDir(root, id), goalStandardOutputFile)
}

func standardGoalEventsPath(root, id string) string {
	return filepath.Join(standardGoalDir(root, id), goalStandardEventsFile)
}

func goalStateFiles(root string) ([]string, error) {
	patterns := []string{
		filepath.Join(root, "temp", goalStandardDir, "*", goalStandardStateFile),
		filepath.Join(root, "temp", goalStatePrefix+"*.json"),
		filepath.Join(root, "temp", "goal_state.json"),
		filepath.Join(root, "temp", "goal_*.json"),
	}
	seen := map[string]bool{}
	files := []string{}
	for _, pattern := range patterns {
		matches, err := filepath.Glob(pattern)
		if err != nil {
			return nil, err
		}
		for _, f := range matches {
			clean := filepath.Clean(f)
			if seen[clean] {
				continue
			}
			seen[clean] = true
			if existsFile(clean) {
				files = append(files, clean)
			}
		}
	}
	sort.Strings(files)
	return files, nil
}

func resolveGoalState(root, id string) (string, GoalState, ContractMeta, bool, error) {
	id, err := normalizeGoalLookupID(id)
	if err != nil {
		return "", GoalState{}, ContractMeta{}, false, err
	}
	candidates := []string{standardGoalStatePath(root, id), filepath.Join(root, "temp", goalStatePrefix+id+".json")}
	if id == "goal_state" {
		candidates = append(candidates, filepath.Join(root, "temp", "goal_state.json"))
	}
	for _, f := range candidates {
		state, meta, err := readGoalState(f)
		if err == nil {
			_, _, managed := goalIdentity(f, state)
			return f, state, meta, managed, nil
		}
		if !errors.Is(err, os.ErrNotExist) {
			return "", GoalState{}, ContractMeta{}, false, err
		}
	}
	// Final read keeps the historical error surface for callers/tests.
	state, meta, err := readGoalState(candidates[0])
	return candidates[0], state, meta, true, err
}

func goalIdentity(path string, state GoalState) (id, origin string, managed bool) {
	id = sanitizeGoalID(state.ID)
	managed = isAdminGoalStatePath(path)
	if id == "" {
		if managed && filepath.Base(path) == goalStandardStateFile {
			id = sanitizeGoalID(filepath.Base(filepath.Dir(path)))
		} else {
			base := strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))
			id = sanitizeGoalID(strings.TrimPrefix(base, goalStatePrefix))
		}
	}
	if id == "" {
		id = "goal"
	}
	origin = "external"
	if managed {
		origin = "admin"
	}
	return id, origin, managed
}

func isAdminGoalStatePath(path string) bool {
	clean := filepath.Clean(path)
	if strings.HasPrefix(filepath.Base(clean), goalStatePrefix) {
		return true
	}
	return filepath.Base(clean) == goalStandardStateFile && filepath.Base(filepath.Dir(filepath.Dir(clean))) == goalStandardDir && filepath.Base(filepath.Dir(filepath.Dir(filepath.Dir(clean)))) == "temp"
}

func goalLogPath(root, statePath, id string, state GoalState, managed bool) string {
	if strings.TrimSpace(state.LogFile) != "" {
		if p := cleanGoalPath(root, filepath.Dir(statePath), state.LogFile); p != "" {
			return p
		}
	}
	if managed && filepath.Base(statePath) == goalStandardStateFile {
		return filepath.Join(filepath.Dir(statePath), goalStandardOutputFile)
	}
	if managed {
		return filepath.Join(filepath.Dir(statePath), goalStatePrefix+id+".log")
	}
	return filepath.Join(filepath.Dir(statePath), strings.TrimSuffix(filepath.Base(statePath), ".json")+".log")
}

func cleanGoalPath(root, baseDir, p string) string {
	p = strings.TrimSpace(p)
	if p == "" {
		return ""
	}
	root = filepath.Clean(root)
	tempDir := filepath.Join(root, "temp")
	var cand string
	if filepath.IsAbs(p) {
		cand = filepath.Clean(p)
	} else {
		rel := filepath.Clean(filepath.FromSlash(p))
		if rel == "." {
			return ""
		}
		if strings.HasPrefix(rel, ".."+string(os.PathSeparator)) || rel == ".." {
			return ""
		}
		parts := strings.Split(rel, string(os.PathSeparator))
		if len(parts) > 0 && parts[0] == "temp" {
			cand = filepath.Join(root, rel)
		} else {
			cand = filepath.Join(baseDir, rel)
		}
	}
	if !isPathWithin(tempDir, cand) {
		return ""
	}
	if _, err := os.Lstat(cand); err == nil {
		resolved, err := filepath.EvalSymlinks(cand)
		if err != nil {
			return ""
		}
		if !isPathWithin(tempDir, resolved) {
			return ""
		}
	}
	return cand
}

func isPathWithin(parent, child string) bool {
	parent = filepath.Clean(parent)
	child = filepath.Clean(child)
	if parent == child {
		return true
	}
	rel, err := filepath.Rel(parent, child)
	if err != nil {
		return false
	}
	return rel != ".." && !strings.HasPrefix(rel, ".."+string(os.PathSeparator)) && !filepath.IsAbs(rel)
}

func goalStopLevel(meta GoalMeta, managed bool) string {
	if !meta.Running {
		return "unsupported"
	}
	if managed && meta.PID > 0 {
		return "safe"
	}
	if !managed {
		return "state_only"
	}
	return "unsupported"
}

func latestModelResponseFile(dir string) string {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return ""
	}
	var best string
	var bestMod time.Time
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		info, err := e.Info()
		if err != nil {
			continue
		}
		path := filepath.Join(dir, e.Name())
		if best == "" || info.ModTime().After(bestMod) {
			best = path
			bestMod = info.ModTime()
		}
	}
	return best
}

func DeleteGoal(root, id string) error {
	id = sanitizeGoalID(id)
	if id == "" {
		return errors.New("id is required")
	}
	statePath, state, _, managed, err := resolveGoalState(root, id)
	if err != nil {
		return err
	}
	goalID, _, managedByFile := goalIdentity(statePath, state)
	if managedByFile {
		managed = true
	}
	if state.EndTime == nil && state.Status == "running" && isPIDRunning(state.PID) {
		return errors.New("goal is running; stop it before deleting")
	}
	logPath := goalLogPath(root, statePath, goalID, state, managed)
	if managed && filepath.Base(statePath) == goalStandardStateFile {
		goalDir := filepath.Dir(statePath)
		if err := os.RemoveAll(goalDir); err != nil && !errors.Is(err, os.ErrNotExist) {
			return err
		}
		return nil
	}
	if err := os.Remove(statePath); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	if err := os.Remove(logPath); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return nil
}

func StopGoal(root, id string, pid int) (GoalMeta, error) {
	statePath, state, meta, managed, err := resolveGoalState(root, id)
	if err != nil {
		return GoalMeta{}, err
	}
	goalID, origin, managedByFile := goalIdentity(statePath, state)
	if managedByFile {
		managed = true
	}
	logPath := goalLogPath(root, statePath, goalID, state, managed)
	if state.EndTime != nil || state.Status != "running" {
		return GoalMeta{}, errors.New("goal is not running")
	}
	if managed {
		if pid <= 0 {
			return GoalMeta{}, errors.New("pid is required for exact stop")
		}
		if state.PID != pid {
			return GoalMeta{}, errors.New("exact PID mismatch; refusing to stop")
		}
		if err := killGoalPID(pid); err != nil {
			return GoalMeta{}, err
		}
		state.PID = 0
	} else if state.PID > 0 && pid > 0 && state.PID != pid {
		return GoalMeta{}, errors.New("PID mismatch; refusing external soft stop")
	}
	now := float64(time.Now().UnixNano()) / 1e9
	state.Status = "stopped_by_admin"
	state.EndTime = &now
	if !managed && strings.TrimSpace(state.ManagedBy) == "" {
		state.ManagedBy = "ga-admin-soft-stop"
	}
	goalMeta := metaFromState(root, goalID, statePath, logPath, state, time.Now())
	goalMeta.Contract = meta
	goalMeta.Origin = origin
	goalMeta.Managed = managed
	goalMeta.PIDTrusted = managed && state.PID > 0
	goalMeta.StopLevel = goalStopLevel(goalMeta, managed)
	if err := writeGoalState(statePath, state); err != nil {
		return goalMeta, err
	}
	return goalMeta, nil
}

type GoalOutputResult struct {
	Output           string   `json:"output"`
	Goal             GoalMeta `json:"goal"`
	Truncated        bool     `json:"truncated"`
	BytesReturned    int64    `json:"bytes_returned"`
	TotalBytes       int64    `json:"total_bytes"`
	LinesReturned    int64    `json:"lines_returned"`
	TotalLines       int64    `json:"total_lines"`
	RequestedBytes   int64    `json:"requested_bytes"`
	MaxBytes         int64    `json:"max_bytes"`
	DefaultBytes     int64    `json:"default_bytes"`
	DefaultBytesUsed bool     `json:"default_bytes_used"`
	MaxBytesCapped   bool     `json:"max_bytes_capped"`
	OutputStatus     string   `json:"output_status"`
}

func GoalOutput(root, id string, maxBytes int64) (GoalOutputResult, error) {
	statePath, state, meta, managed, err := resolveGoalState(root, id)
	if err != nil {
		return GoalOutputResult{}, err
	}
	goalID, origin, managedByFile := goalIdentity(statePath, state)
	if managedByFile {
		managed = true
	}
	logPath := goalLogPath(root, statePath, goalID, state, managed)
	requestedBytes := maxBytes
	defaultBytesUsed := false
	if maxBytes <= 0 {
		maxBytes = defaultGoalOutputBytes
		defaultBytesUsed = true
	}
	maxBytesCapped := false
	if maxBytes > maxGoalOutputBytes {
		maxBytes = maxGoalOutputBytes
		maxBytesCapped = true
	}
	goalMeta := metaFromState(root, goalID, statePath, logPath, state, time.Now())
	goalMeta.Contract = meta
	goalMeta.Origin = origin
	goalMeta.Managed = managed
	goalMeta.PIDTrusted = managed && state.PID > 0
	goalMeta.StopLevel = goalStopLevel(goalMeta, managed)
	goalMeta.Actions = goalActions(goalMeta)
	out, totalBytes, truncated, err := tailFile(logPath, maxBytes)
	outputPath := logPath
	outputStatusPrefix := ""
	if err != nil && errors.Is(err, os.ErrNotExist) && !managed {
		if fallback := latestModelResponseFile(filepath.Join(root, "temp", "model_responses")); fallback != "" {
			outputPath = fallback
			out, totalBytes, truncated, err = tailFile(outputPath, maxBytes)
			outputStatusPrefix = "model_responses_"
			goalMeta.LogFile = relGoalPath(root, outputPath)
			goalMeta.LogExists = true
			goalMeta.MissingLog = false
		}
	}
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return GoalOutputResult{Goal: goalMeta, RequestedBytes: requestedBytes, MaxBytes: maxBytes, DefaultBytes: defaultGoalOutputBytes, DefaultBytesUsed: defaultBytesUsed, MaxBytesCapped: maxBytesCapped, OutputStatus: "missing_log"}, nil
		}
		return GoalOutputResult{}, err
	}
	outputStatus := outputStatusPrefix + "full"
	if totalBytes == 0 {
		outputStatus = outputStatusPrefix + "empty_log"
	} else if truncated {
		outputStatus = outputStatusPrefix + "tail_truncated"
	}
	totalLines, err := countDisplayLinesFile(outputPath)
	if err != nil {
		return GoalOutputResult{}, err
	}
	return GoalOutputResult{Output: out, Goal: goalMeta, Truncated: truncated, BytesReturned: int64(len([]byte(out))), TotalBytes: totalBytes, LinesReturned: countDisplayLinesString(out), TotalLines: totalLines, RequestedBytes: requestedBytes, MaxBytes: maxBytes, DefaultBytes: defaultGoalOutputBytes, DefaultBytesUsed: defaultBytesUsed, MaxBytesCapped: maxBytesCapped, OutputStatus: outputStatus}, nil
}

func readGoalState(path string) (GoalState, ContractMeta, error) {
	var state GoalState
	meta, err := readContractJSON(path, contractDomainGoalState, &state)
	if err != nil {
		return GoalState{}, meta, err
	}
	if state.SchemaVersion == 0 {
		state.SchemaVersion = adminContractSchemaVersion
	}
	if strings.TrimSpace(state.Domain) == "" {
		state.Domain = contractDomainGoalState
	}
	return state, meta, nil
}

func writeGoalState(path string, state GoalState) error {
	if state.SchemaVersion == 0 {
		state.SchemaVersion = adminContractSchemaVersion
	}
	state.Domain = contractDomainGoalState
	b, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return err
	}
	return writeFileAtomic(path, b, 0644)
}

func writeFileAtomic(path string, data []byte, perm os.FileMode) (err error) {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(dir, filepath.Base(path)+".tmp-*")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	defer func() {
		if err != nil {
			_ = os.Remove(tmpName)
		}
	}()
	if _, err = tmp.Write(data); err != nil {
		_ = tmp.Close()
		return err
	}
	if err = tmp.Chmod(perm); err != nil {
		_ = tmp.Close()
		return err
	}
	if err = tmp.Sync(); err != nil {
		_ = tmp.Close()
		return err
	}
	if err = tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmpName, path)
}

func metaFromState(root, id, statePath, logPath string, state GoalState, mod time.Time) GoalMeta {
	now := time.Now()
	managed := isAdminGoalStatePath(statePath)
	processRunning := state.PID > 0 && isPIDRunning(state.PID)
	running := state.EndTime == nil && ((managed && processRunning) || (!managed && state.Status == "running"))
	rawStatus := strings.TrimSpace(state.Status)
	status := rawStatus
	end := now
	if state.EndTime != nil && *state.EndTime > 0 {
		end = time.Unix(0, int64(*state.EndTime*1e9))
	} else if status == "running" && !running && !mod.IsZero() {
		// A stale running state means the detached Goal runner exited without writing
		// end_time. Freeze elapsed/remaining at the state file mtime instead of making
		// old crashed/completed runs appear to consume time forever in the Admin UI.
		end = mod
	}
	elapsed := int(end.Sub(time.Unix(0, int64(state.StartTime*1e9))).Seconds())
	if elapsed < 0 {
		elapsed = 0
	}
	remaining := state.BudgetSeconds - elapsed
	if remaining < 0 {
		remaining = 0
	}
	turnsUsed := state.TurnsUsed
	if turnsUsed < 0 {
		turnsUsed = 0
	}
	if state.MaxTurns > 0 && turnsUsed > state.MaxTurns {
		turnsUsed = state.MaxTurns
	}
	budgetPercent := percentOf(elapsed, state.BudgetSeconds)
	turnPercent := percentOf(turnsUsed, state.MaxTurns)
	normalizedStatus, errorClass := normalizeGoalStatus(status, running, state.EndTime)
	lastEvent := status
	if rawStatus == "running" && !running && state.EndTime == nil {
		lastEvent = "stale_running"
	}
	logExists := existsFile(logPath)
	goalMeta := GoalMeta{SchemaVersion: 1, ID: id, Objective: state.Objective, BudgetSeconds: state.BudgetSeconds, StartTime: state.StartTime, ElapsedSeconds: elapsed, RemainingSeconds: remaining, BudgetPercent: budgetPercent, TurnsUsed: turnsUsed, MaxTurns: state.MaxTurns, TurnPercent: turnPercent, Status: normalizedStatus, RawStatus: rawStatus, ErrorClass: errorClass, LastEvent: lastEvent, PID: state.PID, Running: running, StateFile: relGoalPath(root, statePath), LogFile: relGoalPath(root, logPath), LogExists: logExists, MissingLog: !logExists, StateReadable: true, LLMNo: state.LLMNo, ModTime: mod, EndTime: state.EndTime}
	goalMeta.Managed = managed
	if managed {
		goalMeta.Origin = "admin"
		goalMeta.PIDTrusted = state.PID > 0
	} else {
		goalMeta.Origin = "external"
	}
	goalMeta.Actions = goalActions(goalMeta)
	goalMeta.StopLevel = goalStopLevel(goalMeta, managed)
	return goalMeta
}

func normalizeGoalStatus(raw string, running bool, endTime *float64) (status, errorClass string) {
	r := strings.ToLower(strings.TrimSpace(raw))
	switch r {
	case "", "unknown":
		if running {
			return "running", ""
		}
		return "unknown", "state_missing"
	case "pending", "created", "queued":
		return "pending", ""
	case "running", "active", "in_progress":
		if running {
			return "running", ""
		}
		if endTime == nil {
			return "unknown", "stale_running"
		}
		return "done", ""
	case "blocked", "waiting", "need_input", "needs_input":
		return "blocked", ""
	case "done", "completed", "success", "succeeded":
		return "done", ""
	case "failed", "error", "start_failed", "crashed", "exited":
		return "failed", r
	case "cancelled", "canceled", "stopped", "stopped_by_admin", "terminated":
		return "cancelled", ""
	default:
		if strings.Contains(r, "fail") || strings.Contains(r, "error") {
			return "failed", r
		}
		if strings.Contains(r, "block") || strings.Contains(r, "wait") {
			return "blocked", ""
		}
		if endTime != nil {
			return "done", ""
		}
		return "unknown", "unrecognized_status"
	}
}

func goalActions(meta GoalMeta) []string {
	actions := []string{"refresh", "view_output"}
	if meta.Running && meta.StopLevel != "unsupported" {
		actions = append(actions, "stop")
	}
	if !meta.Running && meta.StateReadable {
		actions = append(actions, "delete")
	}
	if meta.Status == "blocked" {
		actions = append(actions, "resume")
	}
	return actions
}

func corruptGoalMeta(root, statePath string, cause error) GoalMeta {
	st, _ := os.Stat(statePath)
	mod := time.Now()
	if st != nil {
		mod = st.ModTime()
	}
	id := sanitizeGoalID(strings.TrimSuffix(filepath.Base(statePath), ".json"))
	if isAdminGoalStatePath(statePath) {
		id = sanitizeGoalID(strings.TrimPrefix(strings.TrimSuffix(filepath.Base(statePath), ".json"), goalStatePrefix))
	}
	if id == "" {
		id = "unknown"
	}
	meta := GoalMeta{SchemaVersion: 1, ID: id, Status: "unknown", RawStatus: "unreadable", ErrorClass: "state_corrupt", LastEvent: cause.Error(), Origin: "external", Managed: isAdminGoalStatePath(statePath), StateFile: relGoalPath(root, statePath), ModTime: mod, StateReadable: false}
	if meta.Managed {
		meta.Origin = "admin"
	}
	meta.StopLevel = goalStopLevel(meta, meta.Managed)
	meta.Actions = goalActions(meta)
	return meta
}

func percentOf(used, total int) int {
	if total <= 0 || used <= 0 {
		return 0
	}
	pct := int(float64(used)/float64(total)*100 + 0.5)
	if pct > 100 {
		return 100
	}
	return pct
}

var newGoalID = goalID

func goalID() string {
	return time.Now().Format("20060102_150405") + "_" + strconv.FormatInt(time.Now().UnixNano()%1000000, 10)
}

func sanitizeGoalID(id string) string {
	id = strings.TrimSpace(id)
	var b strings.Builder
	for _, r := range id {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '_' || r == '-' {
			b.WriteRune(r)
		}
	}
	return b.String()
}

func normalizeGoalLookupID(id string) (string, error) {
	id = strings.TrimSpace(id)
	if id == "" {
		return "", errors.New("id is required")
	}
	clean := sanitizeGoalID(id)
	if clean == "" || clean != id {
		return "", errors.New("invalid goal id")
	}
	return clean, nil
}

func goalCommandEnv(base []string, statePath string) []string {
	env := append([]string{}, base...)
	env = upsertEnv(env, "GOAL_STATE", statePath)
	// Goal logs are transported through JSON and rendered by the browser as UTF-8.
	// On Windows a detached Python process may otherwise inherit an ANSI/OEM code
	// page and write non-UTF-8 bytes, which appears as mojibake in the Goal panel.
	env = upsertEnv(env, "PYTHONIOENCODING", "utf-8")
	env = upsertEnv(env, "PYTHONUTF8", "1")
	return env
}

func upsertEnv(env []string, key, value string) []string {
	prefix := key + "="
	for i, item := range env {
		if strings.HasPrefix(item, prefix) {
			env[i] = prefix + value
			return env
		}
	}
	return append(env, prefix+value)
}

func goalPython(root, requested string) (string, error) {
	requested = strings.TrimSpace(requested)
	if requested != "" {
		if !filepath.IsAbs(requested) {
			requested = filepath.Join(root, requested)
		}
		if !existsFile(requested) {
			return "", fmt.Errorf("python_path does not exist: %s", requested)
		}
		return requested, nil
	}
	if runtime.GOOS == "windows" {
		for _, p := range []string{filepath.Join(root, ".venv", "Scripts", "python.exe"), filepath.Join(root, "venv", "Scripts", "python.exe")} {
			if existsFile(p) {
				return p, nil
			}
		}
		if p := latestUVWindowsPython(); p != "" {
			return p, nil
		}
		if p, err := exec.LookPath("python"); err == nil && !isWindowsAppsPythonAlias(p) {
			return p, nil
		}
	}
	for _, p := range []string{filepath.Join(root, ".venv", "bin", "python"), filepath.Join(root, "venv", "bin", "python")} {
		if existsFile(p) {
			return p, nil
		}
	}
	if p, err := exec.LookPath("python3"); err == nil {
		return p, nil
	}
	return "python", nil
}

func latestUVWindowsPython() string {
	base := filepath.Join(os.Getenv("APPDATA"), "uv", "python")
	matches, err := filepath.Glob(filepath.Join(base, "cpython-*", "python.exe"))
	if err != nil || len(matches) == 0 {
		return ""
	}
	sort.Strings(matches)
	for i := len(matches) - 1; i >= 0; i-- {
		if existsFile(matches[i]) {
			return matches[i]
		}
	}
	return ""
}

func isWindowsAppsPythonAlias(p string) bool {
	p = strings.ToLower(filepath.Clean(p))
	return strings.Contains(p, strings.ToLower(filepath.Join("Microsoft", "WindowsApps", "python.exe")))
}

var killGoalPID = killExactPID

func killExactPID(pid int) error {
	if pid <= 0 {
		return errors.New("invalid PID")
	}
	if runtime.GOOS == "windows" {
		// Stop only the requested goal runner process. Do not use /T here: the Admin UI
		// must never kill unrelated child Python/tool processes that may belong to
		// another foreground GenericAgent session. Treat an already-exited PID as a
		// successful idempotent stop so stale UI state can still be cleaned up.
		cmd := exec.Command("taskkill", "/PID", strconv.Itoa(pid), "/F")
		hideChildWindow(cmd)
		if err := cmd.Run(); err != nil {
			if !isPIDRunning(pid) {
				return nil
			}
			return err
		}
		return nil
	}
	p, err := os.FindProcess(pid)
	if err != nil {
		return err
	}
	return p.Kill()
}

func isPIDRunning(pid int) bool {
	if pid <= 0 {
		return false
	}
	if runtime.GOOS == "windows" {
		cmd := exec.Command("tasklist", "/FI", fmt.Sprintf("PID eq %d", pid), "/FO", "CSV", "/NH")
		hideChildWindow(cmd)
		out, err := cmd.Output()
		if err != nil {
			return false
		}
		return tasklistHasExactPID(out, pid)
	}
	p, err := os.FindProcess(pid)
	if err != nil || p == nil {
		return false
	}
	return p.Signal(syscall.Signal(0)) == nil
}

func tasklistHasExactPID(out []byte, pid int) bool {
	want := strconv.Itoa(pid)
	for _, line := range strings.Split(string(out), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.Contains(line, "INFO:") {
			continue
		}
		fields := strings.Split(line, ",")
		if len(fields) < 2 {
			continue
		}
		got := strings.Trim(fields[1], " \t\r\n\"")
		if got == want {
			return true
		}
	}
	return false
}

func countDisplayLinesString(s string) int64 {
	s = strings.TrimSuffix(s, "\n")
	s = strings.TrimSuffix(s, "\r")
	if s == "" {
		return 0
	}
	return int64(strings.Count(s, "\n") + 1)
}

func countDisplayLinesFile(path string) (int64, error) {
	f, err := os.Open(path)
	if err != nil {
		return 0, err
	}
	defer f.Close()
	r := bufio.NewReader(f)
	var lineFeeds int64
	var totalBytes int64
	var prev, last byte
	buf := make([]byte, 32*1024)
	for {
		n, readErr := r.Read(buf)
		if n > 0 {
			for _, b := range buf[:n] {
				if b == '\n' {
					lineFeeds++
				}
				prev, last = last, b
			}
			totalBytes += int64(n)
		}
		if readErr != nil {
			if errors.Is(readErr, io.EOF) {
				break
			}
			return 0, readErr
		}
	}
	if totalBytes == 0 {
		return 0, nil
	}
	if last == '\n' {
		lineFeeds--
		if totalBytes == 1 || (totalBytes == 2 && prev == '\r') {
			return 0, nil
		}
	}
	return lineFeeds + 1, nil
}

func tailFile(path string, maxBytes int64) (string, int64, bool, error) {
	if maxBytes < 0 {
		return "", 0, false, errors.New("max_bytes must be >= 0")
	}
	f, err := os.Open(path)
	if err != nil {
		return "", 0, false, err
	}
	defer f.Close()
	st, err := f.Stat()
	if err != nil {
		return "", 0, false, err
	}
	start := st.Size() - maxBytes
	if start < 0 {
		start = 0
	}
	if start > 0 {
		start = alignUTF8TailStart(f, start, st.Size())
	}
	buf := make([]byte, st.Size()-start)
	_, err = f.ReadAt(buf, start)
	if err != nil && !errors.Is(err, io.EOF) {
		return "", 0, false, err
	}
	return string(buf), st.Size(), start > 0, nil
}

func alignUTF8TailStart(f *os.File, start, size int64) int64 {
	if start <= 0 || start >= size {
		return start
	}
	probeLen := int64(utf8.UTFMax)
	if remaining := size - start; remaining < probeLen {
		probeLen = remaining
	}
	probe := make([]byte, probeLen)
	_, err := f.ReadAt(probe, start)
	if err != nil && !errors.Is(err, io.EOF) {
		return start
	}
	for len(probe) > 0 && !utf8.RuneStart(probe[0]) {
		probe = probe[1:]
		start++
	}
	if len(probe) == 0 {
		return start
	}
	_, width := utf8.DecodeRune(probe)
	if width == 1 && probe[0] >= utf8.RuneSelf {
		start++
	}
	return start
}

func existsFile(p string) bool {
	st, err := os.Stat(p)
	return err == nil && !st.IsDir()
}

func relGoalPath(root, p string) string {
	if rel, err := filepath.Rel(root, p); err == nil && !strings.HasPrefix(rel, "..") {
		return filepath.ToSlash(rel)
	}
	return filepath.ToSlash(p)
}
