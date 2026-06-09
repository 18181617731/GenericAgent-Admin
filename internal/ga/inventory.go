package ga

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

type FileStatus struct {
	Path    string    `json:"path"`
	Exists  bool      `json:"exists"`
	Size    int64     `json:"size,omitempty"`
	ModTime time.Time `json:"mod_time,omitempty"`
}
type Entry struct {
	Name    string    `json:"name"`
	Path    string    `json:"path"`
	Kind    string    `json:"kind"`
	Size    int64     `json:"size,omitempty"`
	ModTime time.Time `json:"mod_time,omitempty"`
	Domain  string    `json:"domain,omitempty"`
}
type MemorySummary struct {
	Insight FileStatus `json:"insight"`
	Facts   FileStatus `json:"facts"`
	SOPs    []Entry    `json:"sops"`
	Utils   []Entry    `json:"utils"`
	Raw     []Entry    `json:"raw_sessions"`
}
type Inventory struct {
	Root                  string           `json:"root"`
	CoreFiles             []FileStatus     `json:"core_files"`
	Tools                 []FileStatus     `json:"tools"`
	Frontends             []Entry          `json:"frontends"`
	Reflect               []Entry          `json:"reflect"`
	Plugins               []Entry          `json:"plugins"`
	Memory                MemorySummary    `json:"memory"`
	Schedule              ScheduleOverview `json:"schedule"`
	AutonomousReports     []Entry          `json:"autonomous_reports"`
	AutonomousReportIndex ReportIndex      `json:"autonomous_report_index"`
	Generated             time.Time        `json:"generated_at"`
}
type ScheduleTask struct {
	SchemaVersion int          `json:"schema_version"`
	Contract      ContractMeta `json:"contract"`
	ID            string       `json:"id"`
	Path          string       `json:"path"`
	Schedule      string       `json:"schedule"`
	Repeat        string       `json:"repeat"`
	Enabled       bool         `json:"enabled"`
	Prompt        string       `json:"prompt"`
	MaxDelayHours any          `json:"max_delay_hours,omitempty"`
	ModTime       time.Time    `json:"mod_time,omitempty"`
	Status        string       `json:"status"`
	NextHint      string       `json:"next_hint,omitempty"`
	LastReport    *Entry       `json:"last_report,omitempty"`
	Error         string       `json:"error,omitempty"`
	RecentReports []Entry      `json:"recent_reports,omitempty"`
}
type ScheduleOverview struct {
	SchemaVersion int            `json:"schema_version"`
	Tasks         []ScheduleTask `json:"tasks"`
	TaskCount     int            `json:"task_count"`
	Enabled       int            `json:"enabled"`
	Disabled      int            `json:"disabled"`
	Overdue       int            `json:"overdue"`
	Errors        int            `json:"errors"`
	NeverRun      int            `json:"never_run"`
	DoneCount     int            `json:"done_count"`
	Log           FileStatus     `json:"log"`
	DoneRecent    []Entry        `json:"done_recent"`
}
type Health struct {
	OK        bool              `json:"ok"`
	Root      string            `json:"root"`
	Checks    map[string]string `json:"checks"`
	Errors    []string          `json:"errors,omitempty"`
	Warnings  []string          `json:"warnings,omitempty"`
	Inventory Inventory         `json:"inventory"`
	Generated time.Time         `json:"generated_at"`
}

func BuildInventory(root string) Inventory {
	inv := Inventory{Root: root, Generated: time.Now()}
	for _, rel := range []string{"agentmain.py", "agent_loop.py", "ga.py", "llmcore.py", "mykey.py", "hub.pyw", "launch.pyw"} {
		inv.CoreFiles = append(inv.CoreFiles, status(root, rel))
	}
	for _, rel := range []string{"assets/tools_schema.json", "assets/tools_schema_claude.json", "assets/tools_schema_gemini.json"} {
		inv.Tools = append(inv.Tools, status(root, rel))
	}
	inv.Frontends = listDir(root, "frontends", classifyFrontend)
	inv.Reflect = listDir(root, "reflect", func(name string, isDir bool) string { return strings.TrimSuffix(name, filepath.Ext(name)) })
	inv.Plugins = listDir(root, "plugins", func(name string, isDir bool) string { return "plugin" })
	inv.Memory = buildMemory(root)
	inv.Schedule = BuildSchedule(root)
	inv.AutonomousReports = buildAutonomousReports(root)
	inv.AutonomousReportIndex = buildReportIndex(root, inv.AutonomousReports)
	return inv
}

func BuildHealth(root string) Health {
	root = strings.TrimSpace(root)
	generated := time.Now()
	if root == "" {
		checks := map[string]string{"ga_root": "empty"}
		return Health{OK: false, Root: root, Checks: checks, Errors: []string{"ga_root: empty"}, Inventory: Inventory{Root: root, Generated: generated}, Generated: generated}
	}
	inv := BuildInventory(root)
	checks := map[string]string{}
	errors := []string{}
	warnings := []string{}
	set := func(name, state string) {
		checks[name] = state
		switch state {
		case "ok", "optional_missing":
			return
		case "empty":
			warnings = append(warnings, name+": "+state)
		default:
			errors = append(errors, name+": "+state)
		}
	}
	for _, f := range inv.CoreFiles {
		switch f.Path {
		case "agentmain.py", "llmcore.py":
			if f.Exists {
				set(f.Path, "ok")
			} else {
				set(f.Path, "missing")
			}
		case "mykey.py":
			// Official GA source does not ship private credentials. Treat mykey.py as
			// an optional runtime model configuration file so first-time installs can
			// open the admin UI and create/export it from the Models page.
			if f.Exists {
				set(f.Path, "ok")
			} else {
				set(f.Path, "optional_missing")
			}
		}
	}
	if len(inv.Tools) > 0 && inv.Tools[0].Exists {
		set("tools_schema", "ok")
	} else {
		set("tools_schema", "missing")
	}
	if len(inv.Reflect) > 0 {
		set("reflect", "ok")
	} else {
		set("reflect", "empty")
	}
	if len(inv.Memory.SOPs) > 0 {
		set("memory_sops", "ok")
	} else {
		set("memory_sops", "empty")
	}
	set("schedule_tasks", "ok")
	return Health{OK: len(errors) == 0, Root: root, Checks: checks, Errors: errors, Warnings: warnings, Inventory: inv, Generated: generated}
}

func buildMemory(root string) MemorySummary {
	m := MemorySummary{Insight: status(root, "memory/global_mem_insight.txt"), Facts: status(root, "memory/global_mem.txt")}
	for _, e := range listDir(root, "memory", func(name string, isDir bool) string { return "memory" }) {
		lower := strings.ToLower(e.Name)
		if e.Kind == "file" && strings.HasSuffix(lower, ".md") {
			m.SOPs = append(m.SOPs, e)
		}
		if e.Kind == "file" && strings.HasSuffix(lower, ".py") {
			m.Utils = append(m.Utils, e)
		}
	}
	m.Raw = listDir(root, "memory/L4_raw_sessions", func(name string, isDir bool) string { return "raw" })
	return m
}

func BuildSchedule(root string) ScheduleOverview {
	ov := ScheduleOverview{SchemaVersion: 1, Log: status(root, "sche_tasks/scheduler.log")}
	ov.DoneRecent = listDir(root, "sche_tasks/done", func(name string, isDir bool) string { return "report" })
	ov.DoneCount = len(ov.DoneRecent)
	if len(ov.DoneRecent) > 20 {
		ov.DoneRecent = ov.DoneRecent[:20]
	}
	entries, err := os.ReadDir(filepath.Join(root, "sche_tasks"))
	if err != nil {
		return ov
	}
	for _, de := range entries {
		if de.IsDir() || !strings.HasSuffix(strings.ToLower(de.Name()), ".json") {
			continue
		}
		p := filepath.Join(root, "sche_tasks", de.Name())
		id := strings.TrimSuffix(de.Name(), filepath.Ext(de.Name()))
		t := ScheduleTask{SchemaVersion: 1, ID: id, Path: filepath.ToSlash(filepath.Join("sche_tasks", de.Name())), Status: "OK"}
		if info, err := de.Info(); err == nil {
			t.ModTime = info.ModTime()
		}
		var raw map[string]any
		meta, err := readContractJSON(p, contractDomainScheduleTask, &raw)
		if err != nil {
			t.Status = "ERROR"
			t.Error = err.Error()
			ov.Tasks = append(ov.Tasks, t)
			continue
		}
		t.Contract = meta
		t.Schedule, _ = raw["schedule"].(string)
		t.Repeat, _ = raw["repeat"].(string)
		t.Prompt, _ = raw["prompt"].(string)
		if v, ok := raw["enabled"].(bool); ok {
			t.Enabled = v
		}
		t.MaxDelayHours = raw["max_delay_hours"]
		t.RecentReports = reportsFor(ov.DoneRecent, id)
		if len(t.RecentReports) > 0 {
			last := t.RecentReports[0]
			t.LastReport = &last
		}
		applyScheduleHealth(&t)
		switch t.Status {
		case "DISABLED":
			ov.Disabled++
		case "OVERDUE":
			ov.Overdue++
		case "NEVER_RUN":
			ov.NeverRun++
		case "ERROR":
			ov.Errors++
		}
		if t.Enabled {
			ov.Enabled++
		}
		ov.Tasks = append(ov.Tasks, t)
	}
	sort.Slice(ov.Tasks, func(i, j int) bool { return ov.Tasks[i].ID < ov.Tasks[j].ID })
	ov.TaskCount = len(ov.Tasks)
	return ov
}

func applyScheduleHealth(t *ScheduleTask) {
	if t.Error != "" {
		t.Status = "ERROR"
		return
	}
	if !t.Enabled {
		t.Status = "DISABLED"
		return
	}
	if strings.TrimSpace(t.Schedule) == "" || strings.TrimSpace(t.Repeat) == "" || strings.TrimSpace(t.Prompt) == "" {
		t.Status = "ERROR"
		t.Error = "schedule, repeat and prompt are required"
		return
	}
	if _, err := time.Parse("15:04", t.Schedule); err != nil && !strings.HasPrefix(t.Repeat, "every_") {
		t.Status = "ERROR"
		t.Error = "schedule must be HH:MM unless repeat is every_Nh/every_Nd"
		return
	}
	if !validRepeat(t.Repeat) {
		t.Status = "ERROR"
		t.Error = "repeat must be daily/weekday/weekly/monthly/once/every_Nh/every_Nd"
		return
	}
	if t.LastReport == nil {
		t.Status = "NEVER_RUN"
		t.NextHint = "waiting for scheduler first run; report will appear in sche_tasks/done"
		return
	}
	age := time.Since(t.LastReport.ModTime)
	if maxAge, ok := repeatMaxAge(t.Repeat); ok && age > maxAge {
		t.Status = "OVERDUE"
		t.NextHint = fmt.Sprintf("last report %.1f hours ago", age.Hours())
		return
	}
	t.Status = "HEALTHY"
}

func validRepeat(rep string) bool {
	switch rep {
	case "daily", "weekday", "weekly", "monthly", "once":
		return true
	}
	if strings.HasPrefix(rep, "every_") && (strings.HasSuffix(rep, "h") || strings.HasSuffix(rep, "d")) {
		mid := strings.TrimSuffix(strings.TrimPrefix(rep, "every_"), rep[len(rep)-1:])
		if mid == "" {
			return false
		}
		n := 0
		for _, r := range mid {
			if r < '0' || r > '9' {
				return false
			}
			n = n*10 + int(r-'0')
		}
		return n > 0
	}
	return false
}

func repeatMaxAge(rep string) (time.Duration, bool) {
	switch rep {
	case "daily", "weekday":
		return 36 * time.Hour, true
	case "weekly":
		return 8 * 24 * time.Hour, true
	case "monthly":
		return 35 * 24 * time.Hour, true
	case "once":
		return 0, false
	}
	if strings.HasPrefix(rep, "every_") {
		unit := rep[len(rep)-1]
		mid := strings.TrimSuffix(strings.TrimPrefix(rep, "every_"), string(unit))
		n := 0
		for _, r := range mid {
			n = n*10 + int(r-'0')
		}
		if n <= 0 {
			return 0, false
		}
		if unit == 'h' {
			return time.Duration(n+1) * time.Hour, true
		}
		if unit == 'd' {
			return time.Duration(n+1) * 24 * time.Hour, true
		}
	}
	return 0, false
}

func ReadScheduleArtifact(root, rel string, maxBytes int64) (string, Entry, error) {
	clean := filepath.ToSlash(filepath.Clean(rel))
	if strings.HasPrefix(clean, "../") || clean == ".." || filepath.IsAbs(clean) {
		return "", Entry{}, errors.New("invalid path")
	}
	allowed := clean == "sche_tasks/scheduler.log" ||
		strings.HasPrefix(clean, "sche_tasks/done/") ||
		strings.HasPrefix(clean, "autonomous_reports/") ||
		strings.HasPrefix(clean, "temp/autonomous_reports/")
	if !allowed {
		return "", Entry{}, errors.New("only schedule reports, autonomous reports and scheduler.log can be read here")
	}
	p := filepath.Join(root, filepath.FromSlash(clean))
	rootEval, err := filepath.EvalSymlinks(root)
	if err != nil {
		return "", Entry{}, err
	}
	targetEval, err := filepath.EvalSymlinks(p)
	if err != nil {
		return "", Entry{}, err
	}
	relToRoot, err := filepath.Rel(rootEval, targetEval)
	if err != nil || relToRoot == ".." || strings.HasPrefix(relToRoot, ".."+string(filepath.Separator)) || filepath.IsAbs(relToRoot) {
		return "", Entry{}, errors.New("artifact path escapes GA root")
	}
	info, err := os.Stat(targetEval)
	if err != nil {
		return "", Entry{}, err
	}
	if !info.Mode().IsRegular() {
		return "", Entry{}, errors.New("artifact path is not a regular file")
	}
	start := int64(0)
	if maxBytes > 0 && info.Size() > maxBytes {
		start = info.Size() - maxBytes
	}
	f, err := os.Open(targetEval)
	if err != nil {
		return "", Entry{}, err
	}
	defer f.Close()
	if start > 0 {
		_, _ = f.Seek(start, 0)
	}
	buf := make([]byte, info.Size()-start)
	n, _ := f.Read(buf)
	e := Entry{Name: filepath.Base(clean), Path: clean, Kind: "file", Size: info.Size(), ModTime: info.ModTime(), Domain: "schedule"}
	return string(buf[:n]), e, nil
}

func SchedulePath(root, id string) (string, string, error) {
	raw := strings.TrimSpace(id)
	if raw == "." || raw == "" {
		return "", "", errors.New("empty task id")
	}
	if strings.Contains(raw, "..") || strings.ContainsAny(raw, `/\`) || filepath.IsAbs(raw) {
		return "", "", errors.New("invalid task id")
	}
	base := raw
	if !strings.HasSuffix(strings.ToLower(base), ".json") {
		base += ".json"
	}
	if base == ".json" || strings.Contains(base, "..") || strings.ContainsAny(base, `/\`) || filepath.Base(base) != base {
		return "", "", errors.New("invalid task id")
	}
	return filepath.Join(root, "sche_tasks", base), strings.TrimSuffix(base, filepath.Ext(base)), nil
}

func ReadTask(root, id string) (map[string]any, string, error) {
	p, cleanID, err := SchedulePath(root, id)
	if err != nil {
		return nil, "", err
	}
	data, err := os.ReadFile(p)
	if err != nil {
		return nil, cleanID, err
	}
	var raw map[string]any
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, cleanID, err
	}
	return raw, cleanID, nil
}

func SaveTask(root, id string, raw map[string]any) (ScheduleTask, error) {
	p, cleanID, err := SchedulePath(root, id)
	if err != nil {
		return ScheduleTask{}, err
	}
	if raw == nil {
		return ScheduleTask{}, errors.New("empty task")
	}
	if old, err := os.ReadFile(p); err == nil {
		if err := backupScheduleTaskFile(p, old); err != nil {
			return ScheduleTask{}, err
		}
	}
	out, err := marshalContractJSON(raw, contractDomainScheduleTask)
	if err != nil {
		return ScheduleTask{}, err
	}
	if err := os.MkdirAll(filepath.Dir(p), 0755); err != nil {
		return ScheduleTask{}, err
	}
	if err := writeFileAtomic(p, out, 0644); err != nil {
		return ScheduleTask{}, err
	}
	for _, t := range BuildSchedule(root).Tasks {
		if t.ID == cleanID {
			return t, nil
		}
	}
	return ScheduleTask{ID: cleanID, Path: filepath.ToSlash(filepath.Join("sche_tasks", cleanID+".json")), Status: "OK"}, nil
}

func CreateTask(root, id string, raw map[string]any) (ScheduleTask, error) {
	p, _, err := SchedulePath(root, id)
	if err != nil {
		return ScheduleTask{}, err
	}
	if _, err := os.Stat(p); err == nil {
		return ScheduleTask{}, errors.New("task already exists")
	}
	return SaveTask(root, id, raw)
}

const scheduleBackupTimestampLayout = "20060102_150405.000000000"

var scheduleBackupNow = time.Now

func backupScheduleTaskFile(p string, data []byte) error {
	return writeFileAtomic(p+".bak."+scheduleBackupNow().Format(scheduleBackupTimestampLayout), data, 0644)
}

func DeleteTask(root, id string) error {
	p, _, err := SchedulePath(root, id)
	if err != nil {
		return err
	}
	old, err := os.ReadFile(p)
	if err != nil {
		return err
	}
	bak := p + ".bak." + scheduleBackupNow().Format(scheduleBackupTimestampLayout)
	if err := writeFileAtomic(bak, old, 0644); err != nil {
		return err
	}
	return os.Remove(p)
}

func ToggleTask(root, id string, enabled bool) (ScheduleTask, error) {
	p, want, err := SchedulePath(root, id)
	if err != nil {
		return ScheduleTask{}, err
	}
	data, err := os.ReadFile(p)
	if err != nil {
		return ScheduleTask{}, err
	}
	var raw map[string]any
	if _, err := readContractJSON(p, contractDomainScheduleTask, &raw); err != nil {
		return ScheduleTask{}, err
	}
	if err := backupScheduleTaskFile(p, data); err != nil {
		return ScheduleTask{}, err
	}
	raw["enabled"] = enabled
	out, err := marshalContractJSON(raw, contractDomainScheduleTask)
	if err != nil {
		return ScheduleTask{}, err
	}
	if err := writeFileAtomic(p, out, 0644); err != nil {
		return ScheduleTask{}, err
	}
	for _, t := range BuildSchedule(root).Tasks {
		if t.ID == want {
			return t, nil
		}
	}
	return ScheduleTask{}, nil
}

func status(root, rel string) FileStatus {
	fs := FileStatus{Path: filepath.ToSlash(rel)}
	if info, err := os.Stat(filepath.Join(root, rel)); err == nil {
		fs.Exists = true
		fs.Size = info.Size()
		fs.ModTime = info.ModTime()
	}
	return fs
}
func listDir(root, rel string, domain func(string, bool) string) []Entry {
	des, err := os.ReadDir(filepath.Join(root, rel))
	if err != nil {
		return nil
	}
	out := []Entry{}
	for _, de := range des {
		if strings.HasPrefix(de.Name(), ".") || de.Name() == "__pycache__" {
			continue
		}
		kind := "file"
		if de.IsDir() {
			kind = "dir"
		}
		e := Entry{Name: de.Name(), Path: filepath.ToSlash(filepath.Join(rel, de.Name())), Kind: kind, Domain: domain(de.Name(), de.IsDir())}
		if info, err := de.Info(); err == nil {
			e.Size = info.Size()
			e.ModTime = info.ModTime()
		}
		out = append(out, e)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ModTime.After(out[j].ModTime) })
	return out
}
func buildAutonomousReports(root string) []Entry {
	out := []Entry{}
	for _, rel := range []string{"autonomous_reports", filepath.ToSlash(filepath.Join("temp", "autonomous_reports"))} {
		for _, e := range listDir(root, rel, func(string, bool) string { return "autonomous-report" }) {
			if e.Kind == "file" {
				out = append(out, e)
			}
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ModTime.After(out[j].ModTime) })
	if len(out) > 80 {
		return out[:80]
	}
	return out
}

func classifyFrontend(name string, isDir bool) string {
	n := strings.ToLower(name)
	switch {
	case strings.Contains(n, "telegram") || strings.Contains(n, "wechat") || strings.Contains(n, "qq") || strings.Contains(n, "feishu") || strings.Contains(n, "dingtalk") || strings.Contains(n, "wecom"):
		return "im-bot"
	case strings.Contains(n, "desktop") || strings.Contains(n, "pet"):
		return "desktop"
	case strings.Contains(n, "streamlit") || strings.Contains(n, "conductor"):
		return "web"
	case strings.Contains(n, "cmd") || strings.Contains(n, "tui"):
		return "terminal"
	default:
		return "frontend"
	}
}
func reportsFor(reports []Entry, id string) []Entry {
	out := []Entry{}
	for _, r := range reports {
		if strings.Contains(r.Name, id) {
			out = append(out, r)
		}
	}
	if len(out) > 5 {
		return out[:5]
	}
	return out
}
