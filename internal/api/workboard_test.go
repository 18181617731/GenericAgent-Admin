package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"genericagent-admin-go/internal/config"
	"genericagent-admin-go/internal/modelconfig"
)

func TestWorkboardPersistsAndTransitionsThroughReviewedWorkflow(t *testing.T) {
	dataDir := t.TempDir()
	h := newWorkboardTestServer(t, dataDir).Routes()

	create := httptest.NewRequest(http.MethodPost, "/api/workboard", strings.NewReader(`{"title":"Ship approval gate","owner":"ops","risk":"high"}`))
	markDangerous(create)
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, create)
	if rr.Code != http.StatusCreated {
		t.Fatalf("create status=%d body=%s", rr.Code, rr.Body.String())
	}
	var created workboardItem
	if err := json.Unmarshal(rr.Body.Bytes(), &created); err != nil {
		t.Fatal(err)
	}
	if created.Status != "backlog" || created.ID == "" {
		t.Fatalf("unexpected item: %+v", created)
	}

	for _, status := range []string{"active", "review", "done", "review"} {
		req := httptest.NewRequest(http.MethodPatch, "/api/workboard/"+created.ID, strings.NewReader(`{"status":"`+status+`"}`))
		markDangerous(req)
		rr = httptest.NewRecorder()
		h.ServeHTTP(rr, req)
		if rr.Code != http.StatusOK {
			t.Fatalf("transition to %s status=%d body=%s", status, rr.Code, rr.Body.String())
		}
	}

	persisted, err := os.ReadFile(filepath.Join(dataDir, workboardFileName))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(persisted), `"status": "review"`) {
		t.Fatalf("unexpected persisted data: %s", persisted)
	}

	h = newWorkboardTestServer(t, dataDir).Routes()
	rr = httptest.NewRecorder()
	h.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/api/workboard", nil))
	if rr.Code != http.StatusOK || !strings.Contains(rr.Body.String(), created.ID) {
		t.Fatalf("reload status=%d body=%s", rr.Code, rr.Body.String())
	}
}

func TestWorkboardRejectsUnsafeMutationAndInvalidTransition(t *testing.T) {
	h := newWorkboardTestServer(t, t.TempDir()).Routes()

	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, httptest.NewRequest(http.MethodPost, "/api/workboard", strings.NewReader(`{"title":"x","owner":"ops","risk":"low"}`)))
	if rr.Code != http.StatusPreconditionRequired {
		t.Fatalf("missing confirm status=%d", rr.Code)
	}

	create := httptest.NewRequest(http.MethodPost, "/api/workboard", strings.NewReader(`{"title":"Review release","owner":"ops","risk":"low"}`))
	markDangerous(create)
	rr = httptest.NewRecorder()
	h.ServeHTTP(rr, create)
	var created workboardItem
	if err := json.Unmarshal(rr.Body.Bytes(), &created); err != nil {
		t.Fatal(err)
	}

	jump := httptest.NewRequest(http.MethodPatch, "/api/workboard/"+created.ID, strings.NewReader(`{"status":"done"}`))
	markDangerous(jump)
	rr = httptest.NewRecorder()
	h.ServeHTTP(rr, jump)
	if rr.Code != http.StatusConflict {
		t.Fatalf("jump status=%d body=%s", rr.Code, rr.Body.String())
	}

	invalid := httptest.NewRequest(http.MethodPost, "/api/workboard", strings.NewReader(`{"title":"","owner":"ops","risk":"critical"}`))
	markDangerous(invalid)
	rr = httptest.NewRecorder()
	h.ServeHTTP(rr, invalid)
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("invalid status=%d body=%s", rr.Code, rr.Body.String())
	}
}

func newWorkboardTestServer(t *testing.T, dataDir string) *Server {
	t.Helper()
	cfg := config.NewStore(t.TempDir())
	updateTestConfig(t, cfg, func(app *config.AppConfig) {
		app.GARoot = t.TempDir()
		app.ChatDataDir = dataDir
	})
	return New(cfg, nil, modelconfig.NewStore(t.TempDir()), nil)
}
