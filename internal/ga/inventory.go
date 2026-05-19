package ga

import (
	"encoding/json"
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
	Root      string           `json:"root"`
	CoreFiles []FileStatus     `json:"core_files"`
	Tools     []FileStatus     `json:"tools"`
	Frontends []Entry          `json:"frontends"`
	Reflect   []Entry          `json:"reflect"`
	Plugins   []Entry          `json:"plugins"`
	Memory    MemorySummary    `json:"memory"`
	Schedule  ScheduleOverview `json:"schedule"`
	Generated time.Time        `json:"generated_at"`
}
type ScheduleTask struct {
	ID            string    `json:"id"`
	Path          string    `json:"path"`
	Schedule      string    `json:"schedule"`
	Repeat        string    `json:"repeat"`
	Enabled       bool      `json:"enabled"`
	Prompt        string    `json:"prompt"`
	MaxDelayHours any       `json:"max_delay_hours,omitempty"`
	ModTime       time.Time `json:"mod_time,omitempty"`
	Status        string    `json:"status"`
	Error         string    `json:"error,omitempty"`
	RecentReports []Entry   `json:"recent_reports,omitempty"`
}
type ScheduleOverview struct {
	Tasks      []ScheduleTask `json:"tasks"`
	TaskCount  int            `json:"task_count"`
	Enabled    int            `json:"enabled"`
	Disabled   int            `json:"disabled"`
	DoneCount  int            `json:"done_count"`
	Log        FileStatus     `json:"log"`
	DoneRecent []Entry        `json:"done_recent"`
}
type Health struct {
	OK        bool              `json:"ok"`
	Root      string            `json:"root"`
	Checks    map[string]string `json:"checks"`
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
	return inv
}

func BuildHealth(root string) Health {
	inv := BuildInventory(root)
	checks := map[string]string{}
	ok := true
	for _, f := range inv.CoreFiles {
		if f.Path == "agentmain.py" || f.Path == "llmcore.py" || f.Path == "mykey.py" {
			if f.Exists {
				checks[f.Path] = "ok"
			} else {
				checks[f.Path] = "missing"
				ok = false
			}
		}
	}
	if len(inv.Tools) > 0 && inv.Tools[0].Exists {
		checks["tools_schema"] = "ok"
	} else {
		checks["tools_schema"] = "missing"
	}
	if len(inv.Reflect) > 0 {
		checks["reflect"] = "ok"
	} else {
		checks["reflect"] = "empty"
	}
	if len(inv.Memory.SOPs) > 0 {
		checks["memory_sops"] = "ok"
	} else {
		checks["memory_sops"] = "empty"
	}
	checks["schedule_tasks"] = "ok"
	return Health{OK: ok, Root: root, Checks: checks, Inventory: inv, Generated: time.Now()}
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
	ov := ScheduleOverview{Log: status(root, "sche_tasks/scheduler.log")}
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
		t := ScheduleTask{ID: id, Path: filepath.ToSlash(filepath.Join("sche_tasks", de.Name())), Status: "OK"}
		if info, err := de.Info(); err == nil {
			t.ModTime = info.ModTime()
		}
		data, err := os.ReadFile(p)
		if err != nil {
			t.Status = "ERROR"
			t.Error = err.Error()
			ov.Tasks = append(ov.Tasks, t)
			continue
		}
		var raw map[string]any
		if err := json.Unmarshal(data, &raw); err != nil {
			t.Status = "ERROR"
			t.Error = err.Error()
			ov.Tasks = append(ov.Tasks, t)
			continue
		}
		t.Schedule, _ = raw["schedule"].(string)
		t.Repeat, _ = raw["repeat"].(string)
		t.Prompt, _ = raw["prompt"].(string)
		if v, ok := raw["enabled"].(bool); ok {
			t.Enabled = v
		}
		t.MaxDelayHours = raw["max_delay_hours"]
		t.RecentReports = reportsFor(ov.DoneRecent, id)
		if t.Enabled {
			ov.Enabled++
		} else {
			ov.Disabled++
		}
		ov.Tasks = append(ov.Tasks, t)
	}
	sort.Slice(ov.Tasks, func(i, j int) bool { return ov.Tasks[i].ID < ov.Tasks[j].ID })
	ov.TaskCount = len(ov.Tasks)
	return ov
}

func ToggleTask(root, id string, enabled bool) (ScheduleTask, error) {
	base := filepath.Base(id)
	if !strings.HasSuffix(base, ".json") {
		base += ".json"
	}
	p := filepath.Join(root, "sche_tasks", base)
	data, err := os.ReadFile(p)
	if err != nil {
		return ScheduleTask{}, err
	}
	var raw map[string]any
	if err := json.Unmarshal(data, &raw); err != nil {
		return ScheduleTask{}, err
	}
	_ = os.WriteFile(p+".bak."+time.Now().Format("20060102_150405"), data, 0644)
	raw["enabled"] = enabled
	out, err := json.MarshalIndent(raw, "", "  ")
	if err != nil {
		return ScheduleTask{}, err
	}
	if err := os.WriteFile(p, out, 0644); err != nil {
		return ScheduleTask{}, err
	}
	want := strings.TrimSuffix(base, ".json")
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
