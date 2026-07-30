package ga

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"
)

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
