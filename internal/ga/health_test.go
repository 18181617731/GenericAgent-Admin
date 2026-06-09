package ga

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestBuildHealthSeparatesBlockingErrorsFromAdvisoryWarnings(t *testing.T) {
	root := t.TempDir()
	for _, rel := range []string{"agentmain.py", "llmcore.py", "assets/tools_schema.json"} {
		p := filepath.Join(root, rel)
		if err := os.MkdirAll(filepath.Dir(p), 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte("stub"), 0644); err != nil {
			t.Fatal(err)
		}
	}

	h := BuildHealth(root)
	if !h.OK {
		t.Fatalf("expected optional/advisory gaps to keep GA usable: errors=%v warnings=%v checks=%v", h.Errors, h.Warnings, h.Checks)
	}
	if len(h.Errors) != 0 {
		t.Fatalf("expected no blocking errors, got %v", h.Errors)
	}
	if h.Checks["mykey.py"] != "optional_missing" {
		t.Fatalf("mykey.py should be optional and never read as a secret file: %v", h.Checks)
	}
	if h.Checks["reflect"] != "empty" || h.Checks["memory_sops"] != "empty" {
		t.Fatalf("expected empty reflect/memory to be advisory checks: %v", h.Checks)
	}
	if !containsHealthItem(h.Warnings, "reflect: empty") || !containsHealthItem(h.Warnings, "memory_sops: empty") {
		t.Fatalf("expected user-facing advisory warnings, got %v", h.Warnings)
	}
}

func TestBuildHealthReportsBlockingErrors(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "agentmain.py"), []byte("stub"), 0644); err != nil {
		t.Fatal(err)
	}

	h := BuildHealth(root)
	if h.OK {
		t.Fatalf("expected missing runtime/tooling files to fail health: %v", h.Checks)
	}
	for _, want := range []string{"llmcore.py: missing", "tools_schema: missing"} {
		if !containsHealthItem(h.Errors, want) {
			t.Fatalf("errors %v missing %q", h.Errors, want)
		}
	}
}

func TestBuildHealthEmptyRootHasActionableError(t *testing.T) {
	h := BuildHealth("  ")
	if h.OK || h.Checks["ga_root"] != "empty" || !containsHealthItem(h.Errors, "ga_root: empty") {
		t.Fatalf("empty root should be a blocking actionable error: %#v", h)
	}
}

func containsHealthItem(items []string, want string) bool {
	for _, item := range items {
		if strings.Contains(item, want) {
			return true
		}
	}
	return false
}

func TestBuildInventoryMemorySummaryIncludesOnlyVisibleMemoryFiles(t *testing.T) {
	root := t.TempDir()
	files := map[string]string{
		"memory/global_mem_insight.txt": "insight",
		"memory/global_mem.txt":         "facts",
		"memory/agent_sop.md":           "sop",
		"memory/helper.py":              "util",
		"memory/L4_raw_sessions/raw.md": "raw",
		"memory/.hidden.md":             "hidden",
		"memory/__pycache__/helper.pyc": "cache",
	}
	for rel, content := range files {
		p := filepath.Join(root, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(p), 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(content), 0644); err != nil {
			t.Fatal(err)
		}
	}

	inv := BuildInventory(root)

	if !inv.Memory.Insight.Exists || inv.Memory.Insight.Path != "memory/global_mem_insight.txt" {
		t.Fatalf("insight status = %#v", inv.Memory.Insight)
	}
	if !inv.Memory.Facts.Exists || inv.Memory.Facts.Path != "memory/global_mem.txt" {
		t.Fatalf("facts status = %#v", inv.Memory.Facts)
	}
	if got := entryNames(inv.Memory.SOPs); len(got) != 1 || got[0] != "agent_sop.md" {
		t.Fatalf("memory SOPs = %v, want only visible .md SOP", got)
	}
	if got := entryNames(inv.Memory.Utils); len(got) != 1 || got[0] != "helper.py" {
		t.Fatalf("memory utils = %v, want only visible .py util", got)
	}
	if got := entryNames(inv.Memory.Raw); len(got) != 1 || got[0] != "raw.md" {
		t.Fatalf("raw sessions = %v, want raw.md", got)
	}
}

func entryNames(entries []Entry) []string {
	names := make([]string, 0, len(entries))
	for _, e := range entries {
		names = append(names, e.Name)
	}
	return names
}
