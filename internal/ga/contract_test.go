package ga

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
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

func TestSchedulePathRejectsNestedOrTraversalIDs(t *testing.T) {
	root := t.TempDir()
	badIDs := []string{
		`../outside`,
		`nested/task`,
		`nested\\task`,
		`..\\outside`,
	}
	for _, id := range badIDs {
		if _, _, err := SchedulePath(root, id); err == nil {
			t.Fatalf("SchedulePath(%q) succeeded; want error", id)
		}
		if _, err := CreateTask(root, id, map[string]any{"schedule": "daily", "repeat": "daily", "enabled": true, "prompt": "x"}); err == nil {
			t.Fatalf("CreateTask(%q) succeeded; want error", id)
		}
	}
}

func TestScheduleRepeatEveryRejectsZeroOrMalformedIntervals(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "sche_tasks"), 0755); err != nil {
		t.Fatal(err)
	}
	cases := map[string]string{
		"zero-hours": "every_0h",
		"zero-days":  "every_0d",
		"bare":       "every_h",
		"negative":   "every_-1h",
	}
	for id, repeat := range cases {
		if err := os.WriteFile(filepath.Join(root, "sche_tasks", id+".json"), []byte(`{"schedule":"ignored","repeat":"`+repeat+`","enabled":true,"prompt":"x"}`), 0644); err != nil {
			t.Fatal(err)
		}
	}
	ov := BuildSchedule(root)
	if len(ov.Tasks) != len(cases) {
		t.Fatalf("tasks = %d, want %d", len(ov.Tasks), len(cases))
	}
	for _, task := range ov.Tasks {
		if task.Status != "ERROR" || task.Error != "repeat must be daily/weekday/weekly/monthly/once/every_Nh/every_Nd" {
			t.Fatalf("task %s repeat %q status/error = %s/%q", task.ID, task.Repeat, task.Status, task.Error)
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

func TestReadScheduleArtifactSafePathAndContract(t *testing.T) {
	root := t.TempDir()
	done := filepath.Join(root, "sche_tasks", "done")
	if err := os.MkdirAll(done, 0755); err != nil {
		t.Fatal(err)
	}
	artifact := filepath.Join(done, "task.report.txt")
	if err := os.WriteFile(artifact, []byte("report body"), 0644); err != nil {
		t.Fatal(err)
	}
	content, entry, err := ReadScheduleArtifact(root, "sche_tasks/done/task.report.txt", 1024)
	if err != nil {
		t.Fatal(err)
	}
	if content != "report body" || entry.Path != "sche_tasks/done/task.report.txt" || entry.Domain != "schedule" {
		t.Fatalf("artifact content/entry = %q %#v", content, entry)
	}

	if content, _, err := ReadScheduleArtifact(root, "sche_tasks/done/task.report.txt", 4); err != nil || content != "body" {
		t.Fatalf("artifact tail content/err = %q %v", content, err)
	}

	logPath := filepath.Join(root, "sche_tasks", "scheduler.log")
	if err := os.WriteFile(logPath, []byte("scheduler ok"), 0644); err != nil {
		t.Fatal(err)
	}
	if content, entry, err := ReadScheduleArtifact(root, "sche_tasks/scheduler.log", 1024); err != nil || content != "scheduler ok" || entry.Path != "sche_tasks/scheduler.log" {
		t.Fatalf("scheduler log content/entry/err = %q %#v %v", content, entry, err)
	}

	badPaths := []string{
		"",
		".",
		"../secret.txt",
		"sche_tasks/../secret.txt",
		"memory/global_mem.txt",
		"sche_tasks/done/../../memory/global_mem.txt",
	}
	for _, rel := range badPaths {
		if _, _, err := ReadScheduleArtifact(root, rel, 1024); err == nil {
			t.Fatalf("ReadScheduleArtifact(%q) succeeded; want error", rel)
		}
	}
}

func TestReadScheduleArtifactRejectsSymlinkEscape(t *testing.T) {
	root := t.TempDir()
	done := filepath.Join(root, "sche_tasks", "done")
	if err := os.MkdirAll(done, 0755); err != nil {
		t.Fatal(err)
	}
	outside := filepath.Join(t.TempDir(), "outside.txt")
	if err := os.WriteFile(outside, []byte("secret"), 0644); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(done, "outside.txt")
	if err := os.Symlink(outside, link); err != nil {
		t.Skipf("symlink unavailable: %v", err)
	}
	if _, _, err := ReadScheduleArtifact(root, "sche_tasks/done/outside.txt", 1024); err == nil {
		t.Fatal("symlink escape artifact read succeeded; want error")
	}
}

func TestBackupScheduleTaskFileReportsWriteError(t *testing.T) {
	root := t.TempDir()
	p := filepath.Join(root, "task.json")
	if err := os.WriteFile(p, []byte(`{"enabled":true}`), 0644); err != nil {
		t.Fatal(err)
	}
	fixed := time.Date(2026, 5, 31, 18, 50, 0, 123456789, time.UTC)
	oldNow := scheduleBackupNow
	scheduleBackupNow = func() time.Time { return fixed }
	defer func() { scheduleBackupNow = oldNow }()
	blocker := p + ".bak." + fixed.Format(scheduleBackupTimestampLayout)
	if err := os.Mkdir(blocker, 0755); err != nil {
		t.Fatal(err)
	}
	if err := backupScheduleTaskFile(p, []byte("old")); err == nil {
		t.Fatal("expected backup write error")
	}
}

func TestToggleTaskReportsBackupWriteError(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "sche_tasks"), 0755); err != nil {
		t.Fatal(err)
	}
	raw := map[string]any{"schedule": "daily", "repeat": "daily", "enabled": true, "prompt": "x"}
	if _, err := SaveTask(root, "toggle-backup", raw); err != nil {
		t.Fatal(err)
	}
	p := filepath.Join(root, "sche_tasks", "toggle-backup.json")
	fixed := time.Date(2026, 5, 31, 18, 50, 1, 987654321, time.UTC)
	oldNow := scheduleBackupNow
	scheduleBackupNow = func() time.Time { return fixed }
	defer func() { scheduleBackupNow = oldNow }()
	blocker := p + ".bak." + fixed.Format(scheduleBackupTimestampLayout)
	if err := os.Mkdir(blocker, 0755); err != nil {
		t.Fatal(err)
	}
	if _, err := ToggleTask(root, "toggle-backup", false); err == nil {
		t.Fatal("expected toggle backup write error")
	}
}

func TestReadProjectVersionSkipsOversizedLinesAndCapsScan(t *testing.T) {
	root := t.TempDir()
	pyproject := filepath.Join(root, "pyproject.toml")
	content := "[project]\n" + strings.Repeat("x", maxProjectVersionLineBytes+4096) + "\nversion = \"1.2.3\"\n"
	if err := os.WriteFile(pyproject, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
	if got := readProjectVersion(pyproject); got != "1.2.3" {
		t.Fatalf("version after oversized line = %q, want 1.2.3", got)
	}

	huge := "[project]\n" + strings.Repeat("x", maxProjectVersionScanBytes+1) + "\nversion = \"9.9.9\"\n"
	if err := os.WriteFile(pyproject, []byte(huge), 0644); err != nil {
		t.Fatal(err)
	}
	if got := readProjectVersion(pyproject); got != "" {
		t.Fatalf("version beyond scan cap = %q, want empty", got)
	}
}
