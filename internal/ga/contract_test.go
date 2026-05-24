package ga

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestScheduleTaskContractLegacyAndSave(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "sche_tasks"), 0755); err != nil {
		t.Fatal(err)
	}
	legacyPath := filepath.Join(root, "sche_tasks", "legacy.json")
	legacy := []byte(`{"schedule":"09:30","repeat":"daily","enabled":true,"prompt":"legacy prompt"}`)
	if err := os.WriteFile(legacyPath, legacy, 0644); err != nil {
		t.Fatal(err)
	}

	ov := BuildSchedule(root)
	if len(ov.Tasks) != 1 {
		t.Fatalf("tasks = %d, want 1", len(ov.Tasks))
	}
	got := ov.Tasks[0]
	if got.Contract.SchemaVersion != adminContractSchemaVersion || got.Contract.Domain != contractDomainScheduleTask || !got.Contract.Legacy || !got.Contract.Compatible {
		t.Fatalf("legacy contract meta = %#v", got.Contract)
	}
	if got.Schedule != "09:30" || got.Repeat != "daily" || got.Prompt != "legacy prompt" || !got.Enabled {
		t.Fatalf("legacy task parsed incorrectly: %#v", got)
	}

	saved, err := SaveTask(root, "saved", map[string]any{"schema_version": 99, "domain": "wrong", "schedule": "every_1h", "repeat": "every_1h", "enabled": true, "prompt": "saved prompt"})
	if err != nil {
		t.Fatal(err)
	}
	if saved.Contract.SchemaVersion != adminContractSchemaVersion || saved.Contract.Domain != contractDomainScheduleTask || saved.Contract.Legacy {
		t.Fatalf("saved contract meta = %#v", saved.Contract)
	}
	b, err := os.ReadFile(filepath.Join(root, "sche_tasks", "saved.json"))
	if err != nil {
		t.Fatal(err)
	}
	var raw map[string]any
	if err := json.Unmarshal(b, &raw); err != nil {
		t.Fatal(err)
	}
	if raw["schema_version"] != float64(adminContractSchemaVersion) || raw["domain"] != contractDomainScheduleTask {
		t.Fatalf("saved raw contract = %#v", raw)
	}
}

func TestScheduleTaskContractRejectsFutureSchemaAndWrongDomain(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "sche_tasks"), 0755); err != nil {
		t.Fatal(err)
	}
	cases := map[string]string{
		"future.json": `{"schema_version":999,"domain":"schedule_task","schedule":"09:00","repeat":"daily","enabled":true,"prompt":"x"}`,
		"wrong.json":  `{"schema_version":1,"domain":"goal_state","schedule":"09:00","repeat":"daily","enabled":true,"prompt":"x"}`,
	}
	for name, body := range cases {
		if err := os.WriteFile(filepath.Join(root, "sche_tasks", name), []byte(body), 0644); err != nil {
			t.Fatal(err)
		}
	}
	ov := BuildSchedule(root)
	if len(ov.Tasks) != len(cases) {
		t.Fatalf("tasks = %d, want %d", len(ov.Tasks), len(cases))
	}
	for _, task := range ov.Tasks {
		if task.Status != "ERROR" || task.Error == "" {
			t.Fatalf("task %s status/error = %s/%q", task.ID, task.Status, task.Error)
		}
	}
}

func TestGoalStateContractLegacyAndWrite(t *testing.T) {
	root := t.TempDir()
	temp := filepath.Join(root, "temp")
	if err := os.MkdirAll(temp, 0755); err != nil {
		t.Fatal(err)
	}
	legacyPath := filepath.Join(temp, goalStatePrefix+"legacy.json")
	legacy := []byte(`{"objective":"legacy","budget_seconds":60,"start_time":1000,"turns_used":1,"max_turns":5,"status":"done","done_prompt":"ok"}`)
	if err := os.WriteFile(legacyPath, legacy, 0644); err != nil {
		t.Fatal(err)
	}
	state, meta, err := readGoalState(legacyPath)
	if err != nil {
		t.Fatal(err)
	}
	if state.SchemaVersion != adminContractSchemaVersion || state.Objective != "legacy" || state.Status != "done" {
		t.Fatalf("legacy state = %#v", state)
	}
	if meta.SchemaVersion != adminContractSchemaVersion || meta.Domain != contractDomainGoalState || !meta.Legacy || !meta.Compatible {
		t.Fatalf("legacy goal meta = %#v", meta)
	}

	writtenPath := filepath.Join(temp, goalStatePrefix+"written.json")
	if err := writeGoalState(writtenPath, GoalState{Objective: "written", BudgetSeconds: 60, StartTime: float64(time.Now().Unix()), MaxTurns: 1, Status: "running"}); err != nil {
		t.Fatal(err)
	}
	written, writtenMeta, err := readGoalState(writtenPath)
	if err != nil {
		t.Fatal(err)
	}
	if written.SchemaVersion != adminContractSchemaVersion || written.Domain != contractDomainGoalState || writtenMeta.Legacy || writtenMeta.Domain != contractDomainGoalState {
		t.Fatalf("written contract = state %#v meta %#v", written, writtenMeta)
	}
	writtenBytes, err := os.ReadFile(writtenPath)
	if err != nil {
		t.Fatal(err)
	}
	var raw map[string]any
	if err := json.Unmarshal(writtenBytes, &raw); err != nil {
		t.Fatal(err)
	}
	if raw["schema_version"] != float64(adminContractSchemaVersion) || raw["domain"] != contractDomainGoalState {
		t.Fatalf("written raw contract = %#v", raw)
	}
}

func TestReportIndexContract(t *testing.T) {
	reports := []Entry{{Name: "a.md", Path: "autonomous_reports/a.md", Kind: "file"}}
	idx := buildReportIndex(t.TempDir(), reports)
	if idx.SchemaVersion != adminContractSchemaVersion || idx.Domain != contractDomainReportIndex || idx.Count != len(reports) {
		t.Fatalf("report index = %#v", idx)
	}
	if len(idx.Roots) != 2 || idx.Roots[0] != "autonomous_reports" || idx.Roots[1] != filepath.ToSlash(filepath.Join("temp", "autonomous_reports")) {
		t.Fatalf("report roots = %#v", idx.Roots)
	}
	if len(idx.Reports) != 1 || idx.Reports[0].Path != reports[0].Path {
		t.Fatalf("reports = %#v", idx.Reports)
	}
}
