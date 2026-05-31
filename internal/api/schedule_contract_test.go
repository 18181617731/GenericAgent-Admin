package api

import (
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
