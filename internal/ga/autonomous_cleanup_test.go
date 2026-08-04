package ga

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestCleanupAutonomousReportsKeepsRecentAndPendingEvidence(t *testing.T) {
	root := t.TempDir()
	reportsRoot := filepath.Join(root, "temp", "autonomous_reports")
	if err := os.MkdirAll(reportsRoot, 0755); err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	files := map[string]string{
		"R001_latest.md":    "# latest\nVERDICT: PASS\ncompleted successfully\n",
		"R002_completed.md": "# old completed\nVERDICT: PASS\ncompleted successfully\n",
		"R003_pending.md":   "# pending proposal\nBLOCKED\napproval evidence is missing\n[ ] pending approval\n",
	}
	for name, content := range files {
		path := filepath.Join(reportsRoot, name)
		if err := os.WriteFile(path, []byte(content), 0644); err != nil {
			t.Fatal(err)
		}
	}
	old := now.Add(-48 * time.Hour)
	if err := os.Chtimes(filepath.Join(reportsRoot, "R002_completed.md"), old, old); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(filepath.Join(reportsRoot, "R003_pending.md"), old, old); err != nil {
		t.Fatal(err)
	}
	result, err := CleanupAutonomousReports(root, 1)
	if err != nil {
		t.Fatal(err)
	}
	if result.Scanned != 3 || result.Kept != 1 || result.Deleted != 1 || result.Skipped != 1 {
		t.Fatalf("cleanup result = %+v", result)
	}
	if _, err := os.Stat(filepath.Join(reportsRoot, "R001_latest.md")); err != nil {
		t.Fatalf("latest report should remain: %v", err)
	}
	if _, err := os.Stat(filepath.Join(reportsRoot, "R002_completed.md")); !os.IsNotExist(err) {
		t.Fatalf("old completed report should be deleted, err=%v", err)
	}
	if _, err := os.Stat(filepath.Join(reportsRoot, "R003_pending.md")); err != nil {
		t.Fatalf("pending evidence should remain: %v", err)
	}
}
