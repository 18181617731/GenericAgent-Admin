package ga

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestAutonomousTaskBoardMigratesOnce(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "temp"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "temp", "TODO.txt"), []byte("# TODO\n[ ] 自身演进 | 编写回归测试 | 用户批准后执行\n"), 0644); err != nil {
		t.Fatal(err)
	}
	first, err := LoadAutonomousTaskBoard(root)
	if err != nil {
		t.Fatal(err)
	}
	if first.MigrationVersion != autonomousTaskMigrationVersion || len(first.Tasks) != 1 {
		t.Fatalf("unexpected migrated board: %+v", first)
	}
	second, err := LoadAutonomousTaskBoard(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(second.Tasks) != 1 || second.Tasks[0].ID != first.Tasks[0].ID {
		t.Fatalf("migration was not idempotent: %+v", second.Tasks)
	}
}

func TestAutonomousTaskBoardRejectsFutureSchema(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, filepath.FromSlash(autonomousTaskStorePath))
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		t.Fatal(err)
	}
	payload, _ := json.Marshal(map[string]interface{}{"schema_version": autonomousTaskSchemaVersion + 1, "tasks": []interface{}{}})
	if err := os.WriteFile(path, payload, 0644); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadAutonomousTaskBoard(root); err == nil {
		t.Fatal("expected unsupported schema error")
	}
}

func TestAutonomousTaskValidationAndTransitions(t *testing.T) {
	valid := AutonomousTask{Title: "task", Status: TaskDraft, CreatedAt: time.Now(), UpdatedAt: time.Now()}
	if err := ValidateAutonomousTask(valid); err != nil {
		t.Fatal(err)
	}
	invalid := valid
	invalid.Status = "unknown"
	if err := ValidateAutonomousTask(invalid); err == nil {
		t.Fatal("expected invalid status error")
	}
	allowed := [][2]string{{TaskDraft, TaskPendingApproval}, {TaskPendingApproval, TaskQueued}, {TaskQueued, TaskRunning}, {TaskRunning, TaskPaused}, {TaskPaused, TaskRunning}, {TaskFailed, TaskQueued}}
	for _, transition := range allowed {
		if !CanTransitionAutonomousTask(transition[0], transition[1]) {
			t.Fatalf("expected transition %s -> %s", transition[0], transition[1])
		}
	}
	if CanTransitionAutonomousTask(TaskCompleted, TaskRunning) || CanTransitionAutonomousTask(TaskDraft, TaskRunning) {
		t.Fatal("illegal transition was accepted")
	}
}

func TestWriteAutonomousControlSignal(t *testing.T) {
	root := t.TempDir()
	rel, err := WriteAutonomousControlSignal(root, "run-1", "pause", "finish current step")
	if err != nil {
		t.Fatal(err)
	}
	payload, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(rel)))
	if err != nil {
		t.Fatal(err)
	}
	var signal map[string]interface{}
	if err := json.Unmarshal(payload, &signal); err != nil {
		t.Fatal(err)
	}
	if signal["action"] != "pause" || signal["run_id"] != "run-1" {
		t.Fatalf("unexpected signal: %+v", signal)
	}
	if _, err := WriteAutonomousControlSignal(root, "run-1", "delete", ""); err == nil {
		t.Fatal("expected unsupported control action error")
	}
}
