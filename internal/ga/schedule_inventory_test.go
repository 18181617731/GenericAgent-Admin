package ga

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestBuildScheduleParsesLatestRunStatusTimeAndReason(t *testing.T) {
	root := t.TempDir()
	tasks := filepath.Join(root, "sche_tasks")
	done := filepath.Join(tasks, "done")
	if err := os.MkdirAll(done, 0755); err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{"success", "legacy_success", "blocked", "failed", "partial", "waiting", "unknown", "never"} {
		if err := os.WriteFile(filepath.Join(tasks, id+".json"), []byte(`{"schedule":"09:00","repeat":"daily","enabled":true,"prompt":"run"}`), 0644); err != nil {
			t.Fatal(err)
		}
	}
	fallback := time.Date(2026, 8, 29, 9, 0, 0, 0, time.UTC)
	reports := map[string]string{
		"success":        "# Run\n- status: **SUCCESS**\nCompleted cleanly.\n## Detail\n- BLOCKED: a local optional check\n",
		"legacy_success": "# Legacy run\nA nested step was BLOCKED but recovered.\n## 结论\n\n**SUCCESS**：origin push and remote verification completed.\n",
		"blocked":        "# Run\n- **status**：`BLOCKED`\n- 执行时间：2026-08-29 10:00:00\n- 阻塞原因：缺少上游凭据 **token**\n",
		"failed":         "- 执行结果：ERROR\n- 失败原因：browser login timed out\n",
		"partial":        "- status: PARTIAL\nOnly two of three checks passed.\n",
		"waiting":        "- status: WAITING\n- task: xianyu daily ops\n## 执行摘要\n因为浏览器会话不可用，等待用户登录。\n",
		"unknown":        "# Legacy run\nNo explicit overall result.\nA nested step was BLOCKED.\n",
	}
	for id, body := range reports {
		name := fmt.Sprintf("2026-08-29_1000_%s.md", id)
		path := filepath.Join(done, name)
		if err := os.WriteFile(path, []byte(body), 0644); err != nil {
			t.Fatal(err)
		}
		if err := os.Chtimes(path, fallback, fallback); err != nil {
			t.Fatal(err)
		}
	}
	byID := map[string]ScheduleTask{}
	for _, task := range BuildSchedule(root).Tasks {
		byID[task.ID] = task
	}
	if got := byID["success"].LatestRun.Status; got != "success" {
		t.Fatalf("success status = %q; local BLOCKED text must not override overall SUCCESS", got)
	}
	if got := byID["legacy_success"].LatestRun.Status; got != "success" {
		t.Fatalf("legacy conclusion status = %q; conclusion section must override nested BLOCKED text", got)
	}
	blocked := byID["blocked"].LatestRun
	if blocked.Status != "blocked" || blocked.Reason != "缺少上游凭据 token" {
		t.Fatalf("blocked latest run = %#v", blocked)
	}
	if want := time.Date(2026, 8, 29, 10, 0, 0, 0, time.Local); blocked.ExecutedAt == nil || !blocked.ExecutedAt.Equal(want) {
		t.Fatalf("metadata executed_at = %v, want %v", blocked.ExecutedAt, want)
	}
	if got := byID["failed"].LatestRun; got.Status != "failed" || got.Reason != "browser login timed out" {
		t.Fatalf("failed latest run = %#v", got)
	}
	if got := byID["partial"].LatestRun; got.Status != "partial" || got.Summary != "Only two of three checks passed." {
		t.Fatalf("partial latest run = %#v", got)
	}
	if got := byID["waiting"].LatestRun; got.Status != "waiting" || got.Reason != "因为浏览器会话不可用，等待用户登录。" {
		t.Fatalf("waiting latest run = %#v", got)
	}
	if got := byID["unknown"].LatestRun; got.Status != "unknown" || got.ExecutedAt == nil || !got.ExecutedAt.Equal(fallback) {
		t.Fatalf("unknown latest run = %#v", got)
	}
	if got := byID["never"].LatestRun; got.Status != "never_run" || got.ExecutedAt != nil {
		t.Fatalf("never latest run = %#v", got)
	}
}

func TestScheduleReportReadIsBoundedAndStatusFieldWins(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "sche_tasks", "done")
	if err := os.MkdirAll(path, 0755); err != nil {
		t.Fatal(err)
	}
	body := "- status: OK\nsummary\n" + strings.Repeat("BLOCKED failure noise\n", int(maxScheduleReportBytes))
	file := filepath.Join(path, "2026-08-29_1000_big.md")
	if err := os.WriteFile(file, []byte(body), 0644); err != nil {
		t.Fatal(err)
	}
	run := readScheduleRun(root, Entry{Path: "sche_tasks/done/2026-08-29_1000_big.md", ModTime: time.Now()})
	if run.Status != "success" {
		t.Fatalf("large report status = %q", run.Status)
	}
}

func TestBuildScheduleGroupsExactTaskReportsAndKeepsThirty(t *testing.T) {
	root := t.TempDir()
	tasks := filepath.Join(root, "sche_tasks")
	done := filepath.Join(tasks, "done")
	if err := os.MkdirAll(done, 0755); err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{"daily", "daily_backup"} {
		body := []byte(`{"schedule":"09:00","repeat":"daily","enabled":true,"prompt":"run"}`)
		if err := os.WriteFile(filepath.Join(tasks, id+".json"), body, 0644); err != nil {
			t.Fatal(err)
		}
	}
	base := time.Date(2026, 7, 30, 10, 0, 0, 0, time.UTC)
	for index := 0; index < 35; index++ {
		writeScheduleReport(t, done, fmt.Sprintf("2026-07-30_%04d_daily.md", index), base.Add(time.Duration(index)*time.Minute))
	}
	writeScheduleReport(t, done, "2026-07-30_9999_daily_backup.md", base.Add(36*time.Minute))

	overview := BuildSchedule(root)
	if len(overview.DoneRecent) != 20 || overview.DoneCount != 36 {
		t.Fatalf("done summary = %d/%d, want 20/36", len(overview.DoneRecent), overview.DoneCount)
	}
	byID := map[string]ScheduleTask{}
	for _, task := range overview.Tasks {
		byID[task.ID] = task
	}
	if len(byID["daily"].RecentReports) != 30 {
		t.Fatalf("daily reports = %d, want 30", len(byID["daily"].RecentReports))
	}
	if got := byID["daily"].RecentReports[0].Name; got != "2026-07-30_0034_daily.md" {
		t.Fatalf("latest daily report = %q", got)
	}
	if got := byID["daily_backup"].RecentReports; len(got) != 1 || got[0].Name != "2026-07-30_9999_daily_backup.md" {
		t.Fatalf("daily_backup reports = %#v", got)
	}
}

func TestBuildScheduleReadsTaskModelNumber(t *testing.T) {
	root := t.TempDir()
	tasks := filepath.Join(root, "sche_tasks")
	if err := os.MkdirAll(tasks, 0755); err != nil {
		t.Fatal(err)
	}
	body := []byte(`{"schedule":"09:00","repeat":"daily","enabled":false,"prompt":"run","llm_no":4}`)
	if err := os.WriteFile(filepath.Join(tasks, "model.json"), body, 0644); err != nil {
		t.Fatal(err)
	}
	task := BuildSchedule(root).Tasks[0]
	if task.LLMNo == nil || *task.LLMNo != 4 {
		t.Fatalf("task model = %#v", task.LLMNo)
	}
}

func writeScheduleReport(t *testing.T, directory, name string, modTime time.Time) {
	t.Helper()
	path := filepath.Join(directory, name)
	if err := os.WriteFile(path, []byte("report"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(path, modTime, modTime); err != nil {
		t.Fatal(err)
	}
}
