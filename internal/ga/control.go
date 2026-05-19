package ga

import (
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

type ControlPlane struct {
	OK           bool           `json:"ok"`
	Generated    time.Time      `json:"generated"`
	Capabilities []Capability   `json:"capabilities"`
	Risks        []RiskItem     `json:"risks"`
	Reports      []ReportItem   `json:"reports"`
	Readiness    []RiskItem     `json:"readiness"`
	Metrics      map[string]int `json:"metrics"`
}

func BuildControlPlane(root string) ControlPlane {
	inv := BuildInventory(root)
	health := BuildHealth(root)
	sched := BuildSchedule(root)
	cp := ControlPlane{OK: health.OK, Generated: time.Now(), Metrics: map[string]int{}}
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
	return cp
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
