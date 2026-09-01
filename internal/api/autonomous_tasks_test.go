package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"genericagent-admin-go/internal/ga"
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
	if task["status"] != "draft" {
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

func TestAutonomousTaskListKeepsSummaryStableWhenStatusFilterChanges(t *testing.T) {
	root := t.TempDir()
	if err := ga.SaveAutonomousTaskBoard(root, ga.AutonomousTaskBoard{
		SchemaVersion:    1,
		MigrationVersion: 1,
		Tasks: []ga.AutonomousTask{
			{ID: "completed", Title: "已完成", Status: ga.TaskCompleted, Progress: 0},
			{ID: "pending", Title: "待审批", Status: ga.TaskPendingApproval},
			{ID: "running", Title: "执行中", Status: ga.TaskRunning, UpdatedAt: time.Now()},
		},
	}); err != nil {
		t.Fatal(err)
	}
	h := newGoalTestServer(t, root).Routes()
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/api/autonomous/tasks?status=completed", nil))
	if rr.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
	}
	var body struct {
		Tasks         []ga.AutonomousTask      `json:"tasks"`
		Total         int                      `json:"total"`
		FilteredTotal int                      `json:"filtered_total"`
		Summary       ga.AutonomousTaskSummary `json:"summary"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if len(body.Tasks) != 1 || body.Total != 1 || body.FilteredTotal != 1 || body.Summary.Total != 3 || body.Summary.Pending != 1 || body.Summary.Running != 1 || body.Summary.Completed != 1 {
		t.Fatalf("filtered response=%+v", body)
	}
	if body.Tasks[0].Progress != 100 {
		t.Fatalf("completed task progress=%d, want 100", body.Tasks[0].Progress)
	}
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
