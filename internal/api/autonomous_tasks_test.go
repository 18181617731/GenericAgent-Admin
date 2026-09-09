package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestAutonomousTaskLifecycleAndSubresources(t *testing.T) {
	root := t.TempDir()
	h := newGoalTestServer(t, root).Routes()

	unconfirmed := httptest.NewRecorder()
	h.ServeHTTP(unconfirmed, httptest.NewRequest(http.MethodPost, "/api/autonomous/tasks", strings.NewReader(`{"title":"blocked"}`)))
	if unconfirmed.Code != http.StatusPreconditionRequired {
		t.Fatalf("unconfirmed create status=%d body=%s", unconfirmed.Code, unconfirmed.Body.String())
	}

	created := requestAutonomousTask(t, h, http.MethodPost, "/api/autonomous/tasks", `{"title":"验证任务","objective":"验证任务控制台"}`)
	task := created["task"].(map[string]interface{})
	id := task["id"].(string)
	if task["status"] != "pending_approval" {
		t.Fatalf("created task=%+v", task)
	}

	approved := requestAutonomousTask(t, h, http.MethodPost, "/api/autonomous/tasks/"+id+"/approve", `{}`)
	if approved["task"].(map[string]interface{})["status"] != "queued" {
		t.Fatalf("approved response=%+v", approved)
	}
	started := requestAutonomousTask(t, h, http.MethodPost, "/api/autonomous/tasks/"+id+"/start", `{}`)
	runs := started["runs"].([]interface{})
	if len(runs) != 1 {
		t.Fatalf("started runs=%+v", runs)
	}
	runID := runs[0].(map[string]interface{})["id"].(string)

	assertAutonomousGET(t, h, "/api/autonomous/tasks/"+id+"/runs", "\"task_id\":\""+id+"\"")
	assertAutonomousGET(t, h, "/api/autonomous/runs/"+runID, "\"run\"")
	assertAutonomousGET(t, h, "/api/autonomous/runs/"+runID+"/events", "\"run_id\":\""+runID+"\"")
	event := requestAutonomousTask(t, h, http.MethodPost, "/api/autonomous/runs/"+runID+"/events", `{"type":"run_started","message":"执行器已启动"}`)
	if event["ok"] != true || event["run"].(map[string]interface{})["status"] != "running" {
		t.Fatalf("event response=%+v", event)
	}
}

func TestAutonomousTaskRejectsIllegalTransition(t *testing.T) {
	h := newGoalTestServer(t, t.TempDir()).Routes()
	created := requestAutonomousTask(t, h, http.MethodPost, "/api/autonomous/tasks", `{"title":"非法转换"}`)
	id := created["task"].(map[string]interface{})["id"].(string)
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/autonomous/tasks/"+id+"/start", strings.NewReader(`{}`))
	markDangerous(req)
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusConflict {
		t.Fatalf("illegal transition status=%d body=%s", rr.Code, rr.Body.String())
	}
}

func TestAutonomousTaskAPIUsesTodoAsOnlySourceAndClosesInPlace(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "temp", "autonomous"), 0755); err != nil {
		t.Fatal(err)
	}
	todo := "# TODO\n" +
		"[ ] 待批准 | pending task | check the change\n" +
		"[ ] 用户已批准 | queued task | wait for the worker <!-- ga-admin-approval:queued-id -->\n" +
		"[x] closed task | already verified\n"
	if err := os.WriteFile(filepath.Join(root, "temp", "TODO.txt"), []byte(todo), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "temp", "pending_drafts.md"), []byte("## 1. ghost draft\n- 状态：待审批\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "temp", "autonomous", "tasks.json"), []byte(`{"schema_version":1,"tasks":[{"id":"ghost","title":"ghost task","status":"failed"}]}`), 0644); err != nil {
		t.Fatal(err)
	}
	h := newGoalTestServer(t, root).Routes()
	listed := getAutonomousTaskList(t, h)
	if len(listed) != 3 {
		t.Fatalf("TODO task list=%+v", listed)
	}
	statuses := map[string]bool{"pending_approval": true, "queued": true, "completed": true}
	var pending map[string]interface{}
	for _, task := range listed {
		if !statuses[task["status"].(string)] || task["source_path"] != "temp/TODO.txt" {
			t.Fatalf("unexpected task=%+v", task)
		}
		if task["title"] == "ghost task" || task["title"] == "ghost draft" {
			t.Fatalf("non-TODO task leaked=%+v", task)
		}
		if task["status"] == "pending_approval" {
			pending = task
		}
	}
	if pending == nil {
		t.Fatal("pending TODO task was not listed")
	}
	id := pending["id"].(string)
	approved := requestAutonomousTask(t, h, http.MethodPost, "/api/autonomous/tasks/"+id+"/approve", `{}`)
	if approved["task"].(map[string]interface{})["status"] != "queued" {
		t.Fatalf("approved response=%+v", approved)
	}
	listed = getAutonomousTaskList(t, h)
	for _, task := range listed {
		if task["id"] == id && task["status"] != "queued" {
			t.Fatalf("approved TODO task changed id/status=%+v", task)
		}
	}
	started := requestAutonomousTask(t, h, http.MethodPost, "/api/autonomous/tasks/"+id+"/start", `{}`)
	runID := started["runs"].([]interface{})[0].(map[string]interface{})["id"].(string)
	listed = getAutonomousTaskList(t, h)
	for _, task := range listed {
		if task["id"] == id && task["last_run_id"] != runID {
			t.Fatalf("run metadata was not retained for TODO task=%+v", task)
		}
	}
	requestAutonomousTask(t, h, http.MethodPost, "/api/autonomous/runs/"+runID+"/events", `{"type":"run_started"}`)
	requestAutonomousTask(t, h, http.MethodPost, "/api/autonomous/runs/"+runID+"/events", `{"type":"run_completed","message":"报告已生成"}`)
	listed = getAutonomousTaskList(t, h)
	for _, task := range listed {
		if task["id"] == id && task["status"] != "completed" {
			t.Fatalf("completed TODO task status=%+v", task)
		}
	}
	content, err := os.ReadFile(filepath.Join(root, "temp", "TODO.txt"))
	if err != nil || !strings.Contains(string(content), "[x] 排队中 | pending task") {
		t.Fatalf("completed TODO row=%q err=%v", content, err)
	}
}

func TestAutonomousTaskAPIRejectKeepsTodoPending(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "temp"), 0755); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(root, "temp", "TODO.txt")
	if err := os.WriteFile(path, []byte("[ ] 待批准 | reject task | keep waiting\n"), 0644); err != nil {
		t.Fatal(err)
	}
	h := newGoalTestServer(t, root).Routes()
	listed := getAutonomousTaskList(t, h)
	if len(listed) != 1 {
		t.Fatalf("tasks=%+v", listed)
	}
	id := listed[0]["id"].(string)
	rejected := requestAutonomousTask(t, h, http.MethodPost, "/api/autonomous/tasks/"+id+"/reject", `{"note":"补充说明"}`)
	if task := rejected["task"].(map[string]interface{}); task["id"] != id || task["status"] != "pending_approval" {
		t.Fatalf("rejected response=%+v", rejected)
	}
	content, err := os.ReadFile(path)
	if err != nil || !strings.Contains(string(content), "[ ] 待批准 | reject task") || strings.Contains(string(content), "用户已拒绝") {
		t.Fatalf("rejected TODO row=%q err=%v", content, err)
	}
}

func TestAutonomousTaskPutUpdatesTheCanonicalTodoRow(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "temp"), 0755); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(root, "temp", "TODO.txt")
	if err := os.WriteFile(path, []byte("[ ] 待批准 | editable task | old objective\n"), 0644); err != nil {
		t.Fatal(err)
	}
	h := newGoalTestServer(t, root).Routes()
	listed := getAutonomousTaskList(t, h)
	if len(listed) != 1 {
		t.Fatalf("tasks=%+v", listed)
	}
	id := listed[0]["id"].(string)
	updated := requestAutonomousTask(t, h, http.MethodPut, "/api/autonomous/tasks/"+id, `{"title":"editable task","objective":"new objective","next_step":"执行 run checks"}`)
	if updated["ok"] != true {
		t.Fatalf("updated response=%+v", updated)
	}
	content, err := os.ReadFile(path)
	if err != nil || !strings.Contains(string(content), "[ ] 待批准 | editable task | new objective | 执行 run checks") || strings.Contains(string(content), "old objective") {
		t.Fatalf("PUT did not update TODO row=%q err=%v", content, err)
	}
	listed = getAutonomousTaskList(t, h)
	if len(listed) != 1 || listed[0]["id"] != id || listed[0]["title"] != "editable task" || listed[0]["objective"] != "new objective" {
		t.Fatalf("reloaded task=%+v", listed)
	}
}

func TestAutonomousTaskPutRejectsTodoTitleChangeWithoutChangingRow(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "temp"), 0755); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(root, "temp", "TODO.txt")
	original := "[ ] 待批准 | immutable task | keep identity\n"
	if err := os.WriteFile(path, []byte(original), 0644); err != nil {
		t.Fatal(err)
	}
	h := newGoalTestServer(t, root).Routes()
	listed := getAutonomousTaskList(t, h)
	id := listed[0]["id"].(string)
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPut, "/api/autonomous/tasks/"+id, strings.NewReader(`{"title":"renamed task"}`))
	markDangerous(req)
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusConflict || !strings.Contains(rr.Body.String(), "titles are immutable") {
		t.Fatalf("title change status=%d body=%s", rr.Code, rr.Body.String())
	}
	content, err := os.ReadFile(path)
	if err != nil || string(content) != original {
		t.Fatalf("title change mutated TODO row=%q err=%v", content, err)
	}
	listed = getAutonomousTaskList(t, h)
	if len(listed) != 1 || listed[0]["id"] != id || listed[0]["title"] != "immutable task" {
		t.Fatalf("task identity changed after rejected title edit=%+v", listed)
	}
}

func TestAutonomousTaskAPIIgnoresMalformedTaskLedger(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, filepath.FromSlash("temp/autonomous/tasks.json"))
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "temp", "TODO.txt"), []byte("[ ] visible TODO task | keep serving\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(`{"schema_version":1,"tasks":[`), 0644); err != nil {
		t.Fatal(err)
	}
	listed := getAutonomousTaskList(t, newGoalTestServer(t, root).Routes())
	if len(listed) != 1 || listed[0]["title"] != "visible TODO task" {
		t.Fatalf("malformed task ledger blocked API=%+v", listed)
	}
}

func TestAutonomousTaskAPIIgnoresMalformedAndFutureRunMetadata(t *testing.T) {
	root := t.TempDir()
	base := filepath.Join(root, "temp", "autonomous")
	if err := os.MkdirAll(base, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "temp", "TODO.txt"), []byte("[ ] visible TODO task | keep serving\n"), 0644); err != nil {
		t.Fatal(err)
	}
	// The first run decodes before the second array element fails. The API
	// must not expose that partially populated metadata slice.
	if err := os.WriteFile(filepath.Join(base, "runs.json"), []byte(`{"schema_version":1,"runs":[{"id":"ghost"},7]}`), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(base, "events.json"), []byte(`{"schema_version":999999,"events":[]}`), 0644); err != nil {
		t.Fatal(err)
	}
	rr := httptest.NewRecorder()
	newGoalTestServer(t, root).Routes().ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/api/autonomous/tasks", nil))
	if rr.Code != http.StatusOK {
		t.Fatalf("run metadata blocked TODO-only API status=%d body=%s", rr.Code, rr.Body.String())
	}
	var response struct {
		Tasks  []map[string]interface{} `json:"tasks"`
		Runs   []map[string]interface{} `json:"runs"`
		Events []map[string]interface{} `json:"events"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &response); err != nil {
		t.Fatal(err)
	}
	if len(response.Tasks) != 1 || response.Tasks[0]["title"] != "visible TODO task" || response.Tasks[0]["source_path"] != "temp/TODO.txt" {
		t.Fatalf("run metadata changed TODO-only API=%+v", response)
	}
	if len(response.Runs) != 0 || len(response.Events) != 0 {
		t.Fatalf("invalid run metadata leaked into API: runs=%+v events=%+v", response.Runs, response.Events)
	}
}

func getAutonomousTaskList(t *testing.T, h http.Handler) []map[string]interface{} {
	t.Helper()
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/api/autonomous/tasks", nil))
	if rr.Code != http.StatusOK {
		t.Fatalf("GET autonomous tasks status=%d body=%s", rr.Code, rr.Body.String())
	}
	var response struct {
		Tasks []map[string]interface{} `json:"tasks"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &response); err != nil {
		t.Fatal(err)
	}
	return response.Tasks
}

func requestAutonomousTask(t *testing.T, h http.Handler, method, path, body string) map[string]interface{} {
	t.Helper()
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	markDangerous(req)
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("%s %s status=%d body=%s", method, path, rr.Code, rr.Body.String())
	}
	var result map[string]interface{}
	if err := json.Unmarshal(rr.Body.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	return result
}

func assertAutonomousGET(t *testing.T, h http.Handler, path, want string) {
	t.Helper()
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, path, nil))
	if rr.Code != http.StatusOK || !strings.Contains(rr.Body.String(), want) {
		t.Fatalf("GET %s status=%d body=%s", path, rr.Code, rr.Body.String())
	}
}
