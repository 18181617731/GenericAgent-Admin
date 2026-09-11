package ga

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestScheduleFoldersCRUDAndAssignmentsAreRecoverable(t *testing.T) {
	root := t.TempDir()
	state, err := LoadScheduleFolders(root)
	if err != nil {
		t.Fatal(err)
	}
	if state.SchemaVersion != scheduleFoldersSchemaVersion || len(state.Folders) != 0 || len(state.Assignments) != 0 {
		t.Fatalf("missing metadata state = %#v", state)
	}

	state, err = CreateScheduleFolder(root, "  日常运营  ")
	if err != nil {
		t.Fatal(err)
	}
	if len(state.Folders) != 1 || state.Folders[0].Name != "日常运营" || state.Folders[0].Order != 0 {
		t.Fatalf("created state = %#v", state)
	}
	folderID := state.Folders[0].ID
	if _, err := CreateScheduleFolder(root, "日常运营"); err == nil || !strings.Contains(err.Error(), "already exists") {
		t.Fatalf("duplicate folder error = %v", err)
	}
	if _, err := MoveScheduleTask(root, "nightly", folderID); err != nil {
		t.Fatal(err)
	}
	state, err = RenameScheduleFolder(root, folderID, "夜间任务")
	if err != nil || state.Folders[0].Name != "夜间任务" || state.Assignments["nightly"] != folderID {
		t.Fatalf("renamed state/error = %#v %v", state, err)
	}

	bakFiles, err := filepath.Glob(filepath.Join(root, "temp", "ga-admin-schedule-folders.json.bak.*"))
	if err != nil || len(bakFiles) == 0 {
		t.Fatalf("expected recoverable folder backup, files=%v err=%v", bakFiles, err)
	}
	state, err = DeleteScheduleFolder(root, folderID)
	if err != nil || len(state.Folders) != 0 || len(state.Assignments) != 0 {
		t.Fatalf("deleted state/error = %#v %v", state, err)
	}
	if _, err := MoveScheduleTask(root, "nightly", ""); err != nil {
		t.Fatal(err)
	}
}

func TestScheduleFoldersRejectUnsafeOrMalformedState(t *testing.T) {
	root := t.TempDir()
	if _, err := CreateScheduleFolder(root, "\n"); err == nil {
		t.Fatal("control-only folder name must be rejected")
	}
	if _, err := MoveScheduleTask(root, "../escape", ""); err == nil {
		t.Fatal("path traversal task id must be rejected")
	}
	if _, err := MoveScheduleTask(root, "task", "missing-folder"); err == nil {
		t.Fatal("unknown folder assignment must be rejected")
	}

	path := filepath.Join(root, filepath.FromSlash(scheduleFoldersRelativePath))
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		t.Fatal(err)
	}
	bad := ScheduleFolders{SchemaVersion: scheduleFoldersSchemaVersion, Folders: []ScheduleFolder{{ID: "folder-a", Name: "A"}}, Assignments: map[string]string{"task": "folder-missing"}}
	data, err := json.Marshal(bad)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, data, 0644); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadScheduleFolders(root); err == nil || !strings.Contains(err.Error(), "references missing folder") {
		t.Fatalf("malformed assignment error = %v", err)
	}
}

func TestBuildScheduleIncludesOnlyExistingFolderAssignments(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "sche_tasks"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "sche_tasks", "daily.json"), []byte(`{"schedule":"09:00","repeat":"daily","enabled":true,"prompt":"run"}`), 0644); err != nil {
		t.Fatal(err)
	}
	state, err := CreateScheduleFolder(root, "Daily")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := MoveScheduleTask(root, "daily", state.Folders[0].ID); err != nil {
		t.Fatal(err)
	}
	overview := BuildSchedule(root)
	if len(overview.Folders) != 1 || overview.Folders[0].Name != "Daily" || len(overview.Tasks) != 1 || overview.Tasks[0].FolderID != state.Folders[0].ID {
		t.Fatalf("folder-aware overview = %#v", overview)
	}
	encoded, err := json.Marshal(overview)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(encoded), `"folders":[`) {
		t.Fatalf("folders must be an array in API JSON: %s", encoded)
	}
}

func TestBuildScheduleMarksMalformedModelIdentityAsConfigurationError(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "sche_tasks"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "sche_tasks", "bad-model.json"), []byte(`{"schedule":"09:00","repeat":"daily","enabled":true,"prompt":"run","model_key":3}`), 0644); err != nil {
		t.Fatal(err)
	}
	overview := BuildSchedule(root)
	if len(overview.Tasks) != 1 || overview.Tasks[0].Status != "ERROR" || !strings.Contains(overview.Tasks[0].Error, "model_key") {
		t.Fatalf("malformed model identity overview = %#v", overview)
	}
}
