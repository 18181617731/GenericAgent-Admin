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

func TestScheduleArtifactRouteSafeRead(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "sche_tasks", "done"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "sche_tasks", "done", "task.txt"), []byte("ok artifact"), 0644); err != nil {
		t.Fatal(err)
	}
	h := newGoalTestServer(t, root).Routes()

	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/schedule/artifact?path=sche_tasks/done/task.txt", nil)
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK || !strings.Contains(rr.Body.String(), "ok artifact") || !strings.Contains(rr.Body.String(), "schedule") {
		t.Fatalf("artifact status/body = %d %s", rr.Code, rr.Body.String())
	}

	rr = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, "/api/schedule/artifact?path=memory/global_mem.txt", nil)
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusBadRequest || !strings.Contains(rr.Body.String(), "only schedule reports") {
		t.Fatalf("bad artifact status/body = %d %s", rr.Code, rr.Body.String())
	}
}

func TestScheduleToggleAndDeleteRequireDangerousConfirm(t *testing.T) {
	h := newGoalTestServer(t, t.TempDir()).Routes()
	for _, tc := range []struct {
		path string
		body string
	}{
		{"/api/schedule/toggle", `{"id":"task","enabled":false}`},
		{"/api/schedule/delete", `{"id":"task"}`},
	} {
		rr := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, tc.path, strings.NewReader(tc.body))
		req.Header.Set("Content-Type", "application/json")
		h.ServeHTTP(rr, req)
		if rr.Code != http.StatusPreconditionRequired || !strings.Contains(rr.Body.String(), "X-GA-Confirm") {
			t.Fatalf("%s without confirm status/body = %d %s", tc.path, rr.Code, rr.Body.String())
		}
	}
}

func TestScheduleReadRoutesRejectNonGETMethods(t *testing.T) {
	h := newGoalTestServer(t, t.TempDir()).Routes()
	for _, tc := range []struct {
		method string
		path   string
	}{
		{http.MethodPost, "/api/schedule/tasks"},
		{http.MethodPut, "/api/schedule/tasks"},
		{http.MethodPost, "/api/schedule/artifact?path=sche_tasks/done/task.txt"},
	} {
		rr := httptest.NewRecorder()
		req := httptest.NewRequest(tc.method, tc.path, strings.NewReader(`{}`))
		req.Header.Set("Content-Type", "application/json")
		h.ServeHTTP(rr, req)
		if rr.Code != http.StatusMethodNotAllowed || !strings.Contains(rr.Body.String(), "method not allowed") {
			t.Fatalf("%s %s status/body = %d %s", tc.method, tc.path, rr.Code, rr.Body.String())
		}
	}
}

func TestScheduleTaskPutRequiresDangerousConfirm(t *testing.T) {
	h := newGoalTestServer(t, t.TempDir()).Routes()
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPut, "/api/schedule/task", strings.NewReader(`{"id":"task","task":{}}`))
	req.Header.Set("Content-Type", "application/json")
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusPreconditionRequired || !strings.Contains(rr.Body.String(), "X-GA-Confirm") {
		t.Fatalf("PUT /api/schedule/task without confirm status/body = %d %s", rr.Code, rr.Body.String())
	}
}

func TestScheduleTaskRawContractRoundTrip(t *testing.T) {
	root := t.TempDir()
	h := newGoalTestServer(t, root).Routes()
	body := `{"id":"daily","raw":{"schedule":"daily","repeat":"daily","enabled":true,"prompt":"from raw"}}`

	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPut, "/api/schedule/task", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-GA-Confirm", "dangerous")
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("PUT /api/schedule/task raw status/body = %d %s", rr.Code, rr.Body.String())
	}
	var putResp map[string]any
	if err := json.Unmarshal(rr.Body.Bytes(), &putResp); err != nil {
		t.Fatal(err)
	}
	if raw, ok := putResp["raw"].(map[string]any); !ok || raw["prompt"] != "from raw" {
		t.Fatalf("PUT /api/schedule/task raw response = %#v", putResp)
	}

	rr = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, "/api/schedule/task?id=daily", nil)
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("GET /api/schedule/task status/body = %d %s", rr.Code, rr.Body.String())
	}
	var getResp map[string]any
	if err := json.Unmarshal(rr.Body.Bytes(), &getResp); err != nil {
		t.Fatal(err)
	}
	if raw, ok := getResp["raw"].(map[string]any); !ok || raw["prompt"] != "from raw" {
		t.Fatalf("GET /api/schedule/task raw response = %#v", getResp)
	}
	if task, ok := getResp["task"].(map[string]any); !ok || task["prompt"] != "from raw" {
		t.Fatalf("GET /api/schedule/task task response = %#v", getResp)
	}
}

func TestScheduleCreateRawContractRoundTrip(t *testing.T) {
	root := t.TempDir()
	h := newGoalTestServer(t, root).Routes()
	body := `{"id":"created","raw":{"schedule":"10:00","repeat":"daily","enabled":false,"prompt":"from create raw"}}`

	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/schedule/create", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-GA-Confirm", "dangerous")
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("POST /api/schedule/create raw status/body = %d %s", rr.Code, rr.Body.String())
	}

	rr = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, "/api/schedule/task?id=created", nil)
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("GET created schedule task status/body = %d %s", rr.Code, rr.Body.String())
	}
	var getResp map[string]any
	if err := json.Unmarshal(rr.Body.Bytes(), &getResp); err != nil {
		t.Fatal(err)
	}
	if raw, ok := getResp["raw"].(map[string]any); !ok || raw["prompt"] != "from create raw" || raw["domain"] != "schedule_task" {
		t.Fatalf("GET created schedule task raw response = %#v", getResp)
	}
}

func TestScheduleCreateRejectsNestedTaskID(t *testing.T) {
	h := newGoalTestServer(t, t.TempDir()).Routes()
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/schedule/create", strings.NewReader(`{"id":"nested/task","task":{"schedule":"daily","repeat":"daily","enabled":true,"prompt":"x"}}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-GA-Confirm", "dangerous")
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusBadRequest || !strings.Contains(rr.Body.String(), "invalid task id") {
		t.Fatalf("POST /api/schedule/create nested id status/body = %d %s", rr.Code, rr.Body.String())
	}
}
