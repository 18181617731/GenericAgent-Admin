package ga

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
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

func TestAutonomousTaskBoardIgnoresFutureTaskMetadata(t *testing.T) {
	root := t.TempDir()
	todoPath := filepath.Join(root, filepath.FromSlash(autonomousTodoPath))
	if err := os.MkdirAll(filepath.Dir(todoPath), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(todoPath, []byte("[ ] TODO-backed task | keep loading\n"), 0644); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(root, filepath.FromSlash(autonomousTaskStorePath))
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		t.Fatal(err)
	}
	payload, _ := json.Marshal(map[string]interface{}{"schema_version": autonomousTaskSchemaVersion + 1, "tasks": []interface{}{}})
	if err := os.WriteFile(path, payload, 0644); err != nil {
		t.Fatal(err)
	}
	board, err := LoadAutonomousTaskBoard(root)
	if err != nil || len(board.Tasks) != 1 || board.Tasks[0].Title != "TODO-backed task" {
		t.Fatalf("future metadata blocked TODO list: board=%+v err=%v", board, err)
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

func TestAutonomousTaskBoardUsesOnlyTodoAndExposesThreeStates(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "temp", "autonomous"), 0755); err != nil {
		t.Fatal(err)
	}
	todo := "# TODO\n" +
		"[ ] 待批准 | pending task | check the change\n" +
		"[ ] 排队中 | queued task | wait for the worker <!-- ga-admin-approval:queued-id -->\n" +
		"[x] closed task | already verified\n"
	if err := os.WriteFile(filepath.Join(root, "temp", "TODO.txt"), []byte(todo), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "temp", "pending_drafts.md"), []byte("## 1. ghost draft\n- 状态：待审批\n"), 0644); err != nil {
		t.Fatal(err)
	}
	ghost := `{"schema_version":1,"tasks":[{"id":"ghost","title":"ghost task","status":"running"}]}`
	if err := os.WriteFile(filepath.Join(root, "temp", "autonomous", "tasks.json"), []byte(ghost), 0644); err != nil {
		t.Fatal(err)
	}
	board, err := LoadAutonomousTaskBoard(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(board.Tasks) != 3 {
		t.Fatalf("TODO-only tasks=%+v", board.Tasks)
	}
	want := map[string]bool{TaskPendingApproval: true, TaskQueued: true, TaskCompleted: true}
	wantStage := map[string]string{TaskPendingApproval: "待批准", TaskQueued: "排队中", TaskCompleted: "已闭环"}
	for _, task := range board.Tasks {
		if !want[task.Status] || task.CurrentStage != wantStage[task.Status] || task.SourcePath != autonomousTodoPath || task.SourceType != "todo" {
			t.Fatalf("unexpected public task=%+v", task)
		}
		if task.Title == "ghost task" || task.Title == "ghost draft" {
			t.Fatalf("non-TODO task leaked=%+v", task)
		}
	}
}

func TestAutonomousTodoStateChangesInPlace(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "temp"), 0755); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(root, filepath.FromSlash(autonomousTodoPath))
	if err := os.WriteFile(path, []byte("[ ] 待批准 | same task | verify once\n"), 0644); err != nil {
		t.Fatal(err)
	}
	initial, err := LoadAutonomousTaskBoard(root)
	if err != nil || len(initial.Tasks) != 1 || initial.Tasks[0].Status != TaskPendingApproval {
		t.Fatalf("initial=%+v err=%v", initial, err)
	}
	id := initial.Tasks[0].ID
	changed, err := UpdateAutonomousTodoTask(root, id, TaskQueued, "follow the checklist")
	if err != nil || !changed {
		t.Fatalf("queue changed=%v err=%v", changed, err)
	}
	queued, err := LoadAutonomousTaskBoard(root)
	if err != nil || len(queued.Tasks) != 1 || queued.Tasks[0].ID != id || queued.Tasks[0].Status != TaskQueued {
		t.Fatalf("queued=%+v err=%v", queued, err)
	}
	changed, err = UpdateAutonomousTodoTask(root, id, TaskQueued, "ignored repeat")
	if err != nil || changed {
		t.Fatalf("repeat queue changed=%v err=%v", changed, err)
	}
	if changed, err = UpdateAutonomousTodoTask(root, id, TaskCompleted, ""); err != nil || !changed {
		t.Fatalf("close changed=%v err=%v", changed, err)
	}
	closed, err := LoadAutonomousTaskBoard(root)
	if err != nil || len(closed.Tasks) != 1 || closed.Tasks[0].ID != id || closed.Tasks[0].Status != TaskCompleted || closed.Tasks[0].Progress != 100 {
		t.Fatalf("closed=%+v err=%v", closed, err)
	}
	content, err := os.ReadFile(path)
	if err != nil || strings.Count(string(content), "same task") != 1 || !strings.Contains(string(content), "[x]") || !strings.Contains(string(content), "排队中") {
		t.Fatalf("TODO content=%q err=%v", content, err)
	}
}

func TestAutonomousTodoTaskFieldsKeepPlainTitlesStable(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "temp"), 0755); err != nil {
		t.Fatal(err)
	}
	todo := "# TODO\n" +
		"[ ] plain title\n" +
		"- [x] closed task | closed summary\n" +
		"[ ] 待批准 | approval title | approval objective | 批准后执行\n" +
		"* [ ] 排队中 | queued title | queued objective\n"
	if err := os.WriteFile(filepath.Join(root, "temp", "TODO.txt"), []byte(todo), 0644); err != nil {
		t.Fatal(err)
	}
	board, err := LoadAutonomousTaskBoard(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(board.Tasks) != 4 {
		t.Fatalf("tasks=%+v", board.Tasks)
	}
	want := map[string]struct {
		status, objective string
	}{
		"plain title":    {TaskPendingApproval, ""},
		"closed task":    {TaskCompleted, "closed summary"},
		"approval title": {TaskPendingApproval, "approval objective"},
		"queued title":   {TaskQueued, "queued objective"},
	}
	for _, task := range board.Tasks {
		expected, ok := want[task.Title]
		if !ok || task.Status != expected.status || task.Objective != expected.objective {
			t.Fatalf("unexpected TODO projection=%+v", task)
		}
	}
}

func TestAutonomousTodoRoundTitleKeepsDescriptionAndIDAcrossTransitions(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "temp"), 0755); err != nil {
		t.Fatal(err)
	}
	todoPath := filepath.Join(root, filepath.FromSlash(autonomousTodoPath))
	line := "[ ] R11 | deep_search 模型链路修复 | 按草案执行\n"
	if err := os.WriteFile(todoPath, []byte(line), 0644); err != nil {
		t.Fatal(err)
	}
	pending, err := LoadAutonomousTaskBoard(root)
	if err != nil || len(pending.Tasks) != 1 {
		t.Fatalf("pending board=%+v err=%v", pending, err)
	}
	initial := pending.Tasks[0]
	if initial.Title != "R11 · deep_search 模型链路修复" || initial.Status != TaskPendingApproval {
		t.Fatalf("round pending projection=%+v", initial)
	}
	if changed, err := UpdateAutonomousTodoTask(root, initial.ID, TaskQueued, ""); err != nil || !changed {
		t.Fatalf("queue changed=%v err=%v", changed, err)
	}
	queued, err := LoadAutonomousTaskBoard(root)
	if err != nil || len(queued.Tasks) != 1 || queued.Tasks[0].ID != initial.ID || queued.Tasks[0].Title != initial.Title || queued.Tasks[0].Status != TaskQueued {
		t.Fatalf("round task changed on queue: before=%+v after=%+v err=%v", initial, queued.Tasks, err)
	}
	if changed, err := UpdateAutonomousTodoTask(root, initial.ID, TaskCompleted, ""); err != nil || !changed {
		t.Fatalf("close changed=%v err=%v", changed, err)
	}
	closed, err := LoadAutonomousTaskBoard(root)
	if err != nil || len(closed.Tasks) != 1 {
		t.Fatalf("closed board=%+v err=%v", closed, err)
	}
	if closed.Tasks[0].ID != initial.ID || closed.Tasks[0].Title != initial.Title || closed.Tasks[0].Status != TaskCompleted {
		t.Fatalf("round task changed across transition: before=%+v after=%+v", initial, closed.Tasks[0])
	}
}

func TestAutonomousTodoDetailsOnlyUsesDecisionPrefixForQueuedState(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "temp"), 0755); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(root, filepath.FromSlash(autonomousTodoPath))
	if err := os.WriteFile(path, []byte("[ ] 待批准 | objective mentions 用户已批准 but remains pending\n"), 0644); err != nil {
		t.Fatal(err)
	}
	board, err := LoadAutonomousTaskBoard(root)
	if err != nil || len(board.Tasks) != 1 || board.Tasks[0].Status != TaskPendingApproval {
		t.Fatalf("initial board=%+v err=%v", board, err)
	}
	if changed, err := UpdateAutonomousTodoTaskDetails(root, board.Tasks[0].ID, board.Tasks[0].Title, board.Tasks[0].Objective, board.Tasks[0].NextStep); err != nil || !changed {
		t.Fatalf("details update changed=%v err=%v", changed, err)
	}
	content, err := os.ReadFile(path)
	if err != nil || strings.Contains(string(content), "[ ] 用户已批准") || !strings.Contains(string(content), "[ ] 待批准") {
		t.Fatalf("objective text changed decision state=%q err=%v", content, err)
	}
}

func TestAutonomousTaskBoardIgnoresMalformedTaskMetadata(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, filepath.FromSlash(autonomousTaskStorePath))
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "temp", "TODO.txt"), []byte("[ ] TODO-backed task | verify\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(`{"schema_version":1,"tasks":[`), 0644); err != nil {
		t.Fatal(err)
	}
	board, err := LoadAutonomousTaskBoard(root)
	if err != nil || len(board.Tasks) != 1 || board.Tasks[0].Title != "TODO-backed task" {
		t.Fatalf("malformed metadata blocked TODO list: board=%+v err=%v", board, err)
	}
}

func TestAutonomousTodoTextEscapesHTMLCommentsBeforeParsing(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "temp"), 0755); err != nil {
		t.Fatal(err)
	}
	task := AutonomousTask{Title: "comment <!-- title -->", Objective: "objective <!-- detail -->"}
	line, err := AppendAutonomousTodoTask(root, task)
	if err != nil || line != 1 {
		t.Fatalf("append line=%d err=%v", line, err)
	}
	path := filepath.Join(root, filepath.FromSlash(autonomousTodoPath))
	content, err := os.ReadFile(path)
	if err != nil || strings.Contains(string(content), "<!--") || strings.Contains(string(content), "-->") {
		t.Fatalf("unsafe TODO comment content=%q err=%v", content, err)
	}
	board, err := LoadAutonomousTaskBoard(root)
	if err != nil || len(board.Tasks) != 1 {
		t.Fatalf("reloaded board=%+v err=%v", board, err)
	}
	if board.Tasks[0].Title != "comment &lt;!-- title --&gt;" || board.Tasks[0].Objective != "objective &lt;!-- detail --&gt;" {
		t.Fatalf("escaped TODO fields were not preserved=%+v", board.Tasks[0])
	}
	if changed, err := UpdateAutonomousTodoTask(root, board.Tasks[0].ID, TaskQueued, ""); err != nil || !changed {
		t.Fatalf("queue escaped TODO changed=%v err=%v", changed, err)
	}
	queued, err := LoadAutonomousTaskBoard(root)
	if err != nil || len(queued.Tasks) != 1 || queued.Tasks[0].ID != board.Tasks[0].ID || queued.Tasks[0].Title != board.Tasks[0].Title {
		t.Fatalf("escaped TODO identity changed after transition=%+v err=%v", queued, err)
	}
}
