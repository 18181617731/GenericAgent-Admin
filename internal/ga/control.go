package ga

import (
	"bufio"
	"errors"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

type Capability struct {
	Name  string `json:"name"`
	Kind  string `json:"kind"`
	Path  string `json:"path"`
	Ready bool   `json:"ready"`
}

type RiskItem struct {
	Level string `json:"level"`
	Area  string `json:"area"`
	Text  string `json:"text"`
}

type ReportItem struct {
	Name    string    `json:"name"`
	Path    string    `json:"path"`
	Size    int64     `json:"size,omitempty"`
	ModTime time.Time `json:"mod_time,omitempty"`
	Kind    string    `json:"kind"`
}

type WorkspaceSummary struct {
	Root      string     `json:"root"`
	Version   string     `json:"version,omitempty"`
	Python    FileStatus `json:"python"`
	Memory    FileStatus `json:"memory"`
	Plan      FileStatus `json:"plan"`
	AdminLink string     `json:"admin_link,omitempty"`
}

type ModelSummary struct {
	Configured bool         `json:"configured"`
	Files      []FileStatus `json:"files"`
	Hint       string       `json:"hint,omitempty"`
}

type LogSummary struct {
	Items []ReportItem `json:"items"`
}

type ControlPlane struct {
	OK           bool             `json:"ok"`
	Generated    time.Time        `json:"generated"`
	Workspace    WorkspaceSummary `json:"workspace"`
	Models       ModelSummary     `json:"models"`
	Logs         LogSummary       `json:"logs"`
	Capabilities []Capability     `json:"capabilities"`
	Risks        []RiskItem       `json:"risks"`
	Reports      []ReportItem     `json:"reports"`
	Readiness    []RiskItem       `json:"readiness"`
	Metrics      map[string]int   `json:"metrics"`
}

func BuildControlPlane(root string) ControlPlane {
	inv := BuildInventory(root)
	health := BuildHealth(root)
	sched := BuildSchedule(root)
	cp := ControlPlane{OK: health.OK, Generated: time.Now(), Metrics: map[string]int{}}
	cp.Workspace = buildWorkspaceSummary(root)
	cp.Models = buildModelSummary(root)
	cp.Logs = LogSummary{Items: discoverLogs(root, 20)}
	for _, f := range inv.CoreFiles {
		if f.Exists {
			cp.Capabilities = append(cp.Capabilities, Capability{Name: filepath.Base(f.Path), Kind: "core", Path: f.Path, Ready: true})
		}
	}
	for _, e := range inv.Frontends {
		cp.Capabilities = append(cp.Capabilities, Capability{Name: e.Name, Kind: e.Domain, Path: e.Path, Ready: true})
	}
	for _, e := range inv.Reflect {
		cp.Capabilities = append(cp.Capabilities, Capability{Name: e.Name, Kind: "reflect", Path: e.Path, Ready: true})
	}
	for _, e := range inv.Plugins {
		cp.Capabilities = append(cp.Capabilities, Capability{Name: e.Name, Kind: "plugin", Path: e.Path, Ready: true})
	}
	for _, t := range sched.Tasks {
		cp.Capabilities = append(cp.Capabilities, Capability{Name: t.ID, Kind: "schedule", Path: t.Path, Ready: t.Status == "OK" && t.Enabled})
	}
	for name, state := range health.Checks {
		if state == "optional_missing" {
			cp.Risks = append(cp.Risks, RiskItem{Level: "info", Area: "models", Text: name + ": optional; configure models to generate it"})
			continue
		}
		if state != "ok" {
			cp.Readiness = append(cp.Readiness, RiskItem{Level: "error", Area: "health", Text: name + ": " + state})
		}
	}
	if inv.Memory.Insight.Exists == false {
		cp.Risks = append(cp.Risks, RiskItem{Level: "warn", Area: "memory", Text: "global_mem_insight.txt missing"})
	}
	if sched.TaskCount == 0 {
		cp.Risks = append(cp.Risks, RiskItem{Level: "info", Area: "schedule", Text: "no scheduled tasks discovered"})
	}
	if sched.Disabled > sched.Enabled && sched.TaskCount > 0 {
		cp.Risks = append(cp.Risks, RiskItem{Level: "info", Area: "schedule", Text: "more disabled tasks than enabled tasks"})
	}
	cp.Reports = discoverReports(root, 40)
	cp.Metrics["capabilities"] = len(cp.Capabilities)
	cp.Metrics["risks"] = len(cp.Risks)
	cp.Metrics["readiness"] = len(cp.Readiness)
	cp.Metrics["reports"] = len(cp.Reports)
	cp.Metrics["logs"] = len(cp.Logs.Items)
	return cp
}

func buildWorkspaceSummary(root string) WorkspaceSummary {
	ws := WorkspaceSummary{Root: root}
	ws.Version = readProjectVersion(filepath.Join(root, "pyproject.toml"))
	ws.Python = status(root, "agentmain.py")
	ws.Memory = status(root, "memory/global_mem.txt")
	ws.Plan = status(root, "temp/plan_ga_admin_ga_integration/plan.md")
	ws.AdminLink = "GenericAgent-Admin-Go control plane"
	return ws
}

func buildModelSummary(root string) ModelSummary {
	files := []FileStatus{
		status(root, "model_profiles.json"),
		status(root, "model_config.json"),
		status(root, "config/models.json"),
		status(root, ".env"),
	}
	configured := false
	for _, f := range files {
		if f.Exists {
			configured = true
			break
		}
	}
	hint := "model settings are provided by GA memory/defaults unless a config file exists"
	if configured {
		hint = "model config file discovered"
	}
	return ModelSummary{Configured: configured, Files: files, Hint: hint}
}

const (
	maxProjectVersionScanBytes = 512 * 1024
	maxProjectVersionLineBytes = 64 * 1024
)

func readProjectVersion(path string) string {
	f, err := os.Open(path)
	if err != nil {
		return ""
	}
	defer f.Close()
	return readProjectVersionReader(f)
}

func readProjectVersionReader(r *os.File) string {
	reader := bufio.NewReaderSize(r, 32*1024)
	inProject := false
	var scanned int64
	for scanned < maxProjectVersionScanBytes {
		line, truncated, err := readBoundedProjectLine(reader, &scanned)
		if len(line) > 0 && !truncated {
			text := strings.TrimSpace(string(line))
			if strings.HasPrefix(text, "[") {
				inProject = text == "[project]"
			} else if inProject && strings.HasPrefix(text, "version") {
				parts := strings.SplitN(text, "=", 2)
				if len(parts) == 2 {
					return strings.Trim(strings.TrimSpace(parts[1]), "\"")
				}
			}
		}
		if err != nil {
			return ""
		}
	}
	return ""
}

func readBoundedProjectLine(reader *bufio.Reader, scanned *int64) ([]byte, bool, error) {
	var line []byte
	truncated := false
	for {
		chunk, err := reader.ReadSlice('\n')
		*scanned += int64(len(chunk))
		if !truncated {
			remaining := maxProjectVersionLineBytes - len(line)
			if len(chunk) <= remaining {
				line = append(line, chunk...)
			} else {
				line = append(line, chunk[:remaining]...)
				truncated = true
			}
		}
		if errors.Is(err, bufio.ErrBufferFull) {
			if *scanned >= maxProjectVersionScanBytes {
				return line, true, nil
			}
			continue
		}
		if err != nil {
			return line, truncated, err
		}
		return line, truncated, nil
	}
}

func discoverLogs(root string, limit int) []ReportItem {
	candidates := []string{"agentmain.log", "error.log", "sche_tasks/scheduler.log", "temp/model_responses", "logs"}
	out := []ReportItem{}
	for _, rel := range candidates {
		p := filepath.Join(root, rel)
		info, err := os.Stat(p)
		if err == nil && !info.IsDir() {
			out = append(out, ReportItem{Name: filepath.Base(rel), Path: filepath.ToSlash(rel), Size: info.Size(), ModTime: info.ModTime(), Kind: "log"})
			continue
		}
		_ = filepath.WalkDir(p, func(path string, d os.DirEntry, err error) error {
			if err != nil || d.IsDir() {
				return nil
			}
			name := d.Name()
			if !strings.HasSuffix(strings.ToLower(name), ".log") {
				return nil
			}
			info, err := d.Info()
			if err != nil {
				return nil
			}
			relpath, _ := filepath.Rel(root, path)
			out = append(out, ReportItem{Name: name, Path: filepath.ToSlash(relpath), Size: info.Size(), ModTime: info.ModTime(), Kind: "log"})
			return nil
		})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ModTime.After(out[j].ModTime) })
	if len(out) > limit {
		return out[:limit]
	}
	return out
}

func discoverReports(root string, limit int) []ReportItem {
	candidates := []string{"autonomous_reports", "sche_tasks/done", "temp/model_responses", "memory/L4_raw_sessions"}
	out := []ReportItem{}
	for _, rel := range candidates {
		base := filepath.Join(root, rel)
		_ = filepath.WalkDir(base, func(p string, d os.DirEntry, err error) error {
			if err != nil || d.IsDir() {
				return nil
			}
			name := d.Name()
			lower := strings.ToLower(name)
			if !(strings.HasSuffix(lower, ".md") || strings.HasSuffix(lower, ".txt") || strings.HasSuffix(lower, ".json") || strings.HasSuffix(lower, ".log")) {
				return nil
			}
			info, err := d.Info()
			if err != nil {
				return nil
			}
			relpath, _ := filepath.Rel(root, p)
			kind := "report"
			if strings.HasSuffix(lower, ".log") {
				kind = "log"
			}
			out = append(out, ReportItem{Name: name, Path: filepath.ToSlash(relpath), Size: info.Size(), ModTime: info.ModTime(), Kind: kind})
			return nil
		})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ModTime.After(out[j].ModTime) })
	if len(out) > limit {
		out = out[:limit]
	}
	return out
}
