package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"genericagent-admin-go/internal/ga"
)

func TestProjectTodosEndpointIsReadOnlyAndStructured(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "temp"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "temp", "TODO.txt"), []byte("[ ] 用户已批准 | R20 | scheduler 调度修复 | 执行\n"), 0644); err != nil {
		t.Fatal(err)
	}
	handler := newGoalTestServer(t, root).Routes()

	get := httptest.NewRecorder()
	handler.ServeHTTP(get, httptest.NewRequest(http.MethodGet, "/api/todos", nil))
	if get.Code != http.StatusOK {
		t.Fatalf("GET status=%d body=%s", get.Code, get.Body.String())
	}
	var overview ga.ProjectTodoOverview
	if err := json.Unmarshal(get.Body.Bytes(), &overview); err != nil {
		t.Fatal(err)
	}
	if overview.Total != 1 || overview.Open != 1 || overview.Items[0].Module != "tasks" || overview.Items[0].Status != "queued" {
		t.Fatalf("overview = %+v", overview)
	}

	post := httptest.NewRecorder()
	handler.ServeHTTP(post, httptest.NewRequest(http.MethodPost, "/api/todos", nil))
	if post.Code != http.StatusMethodNotAllowed {
		t.Fatalf("POST status=%d body=%s", post.Code, post.Body.String())
	}
}
