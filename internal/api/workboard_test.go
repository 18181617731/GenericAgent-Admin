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

func TestWorkboardDecisionWorkflowPersistsContractProposalAndEvents(t *testing.T) {
	dataDir := t.TempDir()
	s := newWorkboardTestServer(t, dataDir)
	created := performWorkboardJSON(t, s, http.MethodPost, "/api/workboard", `{"title":"Ship guarded deploy","outcome":"Deploy without errors","acceptance_criteria":["focused tests pass","rollback documented"],"owner":"agent-api","risk":"high"}`, http.StatusCreated)
	var item workboardItem
	decodeWorkboardResponse(t, created, &item)
	if item.Status != "backlog" || item.Revision != 1 || len(item.Events) != 1 || item.Events[0].Action != "created" {
		t.Fatalf("unexpected created item: %+v", item)
	}

	steps := []struct {
		body   string
		status string
		rev    int
	}{
		{`{"action":"start","expected_revision":1}`, "active", 2},
		{`{"action":"submit_proposal","expected_revision":2,"proposal":"Deploy canary then expand","evidence":[{"label":"tests","detail":"go test ./... passed"}]}`, "review", 3},
		{`{"action":"return","expected_revision":3,"note":"Add rollback evidence"}`, "active", 4},
		{`{"action":"submit_proposal","expected_revision":4,"proposal":"Deploy canary with rollback","evidence":[{"label":"tests","detail":"all passed"},{"label":"rollback","detail":"staging rollback completed"}]}`, "review", 5},
		{`{"action":"approve","expected_revision":5,"note":"Evidence meets acceptance criteria"}`, "done", 6},
	}
	for _, step := range steps {
		rr := performWorkboardJSON(t, s, http.MethodPost, "/api/workboard/"+item.ID+"/commands", step.body, http.StatusOK)
		decodeWorkboardResponse(t, rr, &item)
		if item.Status != step.status || item.Revision != step.rev {
			t.Fatalf("after %s got status=%s revision=%d", step.body, item.Status, item.Revision)
		}
	}
	if len(item.Events) != 6 || item.Proposal.Summary != "Deploy canary with rollback" || len(item.Proposal.Evidence) != 2 || item.Instructions != "" {
		t.Fatalf("decision record incomplete: %+v", item)
	}

	reloaded := newWorkboardTestServer(t, dataDir)
	rr := performWorkboardJSON(t, reloaded, http.MethodGet, "/api/workboard", "", http.StatusOK)
	var state workboardState
	decodeWorkboardResponse(t, rr, &state)
	if len(state.Items) != 1 || state.Items[0].Revision != 6 || len(state.Items[0].Events) != 6 {
		t.Fatalf("persisted state mismatch: %+v", state)
	}
}

func TestWorkboardRejectsMissingConfirmInvalidCommandsAndStaleRevision(t *testing.T) {
	s := newWorkboardTestServer(t, t.TempDir())
	unconfirmed := httptest.NewRequest(http.MethodPost, "/api/workboard", strings.NewReader(`{"title":"x"}`))
	rr := httptest.NewRecorder()
	s.requireDangerousConfirm(s.workboard)(rr, unconfirmed)
	if rr.Code != http.StatusPreconditionRequired {
		t.Fatalf("missing confirm status=%d body=%s", rr.Code, rr.Body.String())
	}

	badContract := performWorkboardJSON(t, s, http.MethodPost, "/api/workboard", `{"title":"x","owner":"agent","risk":"low"}`, http.StatusBadRequest)
	if !strings.Contains(badContract.Body.String(), "outcome") {
		t.Fatalf("unexpected contract error: %s", badContract.Body.String())
	}

	created := performWorkboardJSON(t, s, http.MethodPost, "/api/workboard", `{"title":"Task","outcome":"Result","acceptance_criteria":["verified"],"owner":"agent","risk":"low"}`, http.StatusCreated)
	var item workboardItem
	decodeWorkboardResponse(t, created, &item)
	performWorkboardJSON(t, s, http.MethodPost, "/api/workboard/"+item.ID+"/commands", `{"action":"start","expected_revision":1}`, http.StatusOK)

	stale := performWorkboardJSON(t, s, http.MethodPost, "/api/workboard/"+item.ID+"/commands", `{"action":"start","expected_revision":1}`, http.StatusConflict)
	if !strings.Contains(stale.Body.String(), "current revision is 2") {
		t.Fatalf("missing actionable stale error: %s", stale.Body.String())
	}
	performWorkboardJSON(t, s, http.MethodPost, "/api/workboard/"+item.ID+"/commands", `{"action":"submit_proposal","expected_revision":2,"proposal":"draft"}`, http.StatusBadRequest)
	performWorkboardJSON(t, s, http.MethodPost, "/api/workboard/"+item.ID+"/commands", `{"action":"approve","expected_revision":2}`, http.StatusConflict)
}

func TestWorkboardLoadsLegacyItemsWithUsableRevision(t *testing.T) {
	dataDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dataDir, workboardFileName), []byte(`{"items":[{"id":"legacy","title":"Old","owner":"agent","risk":"low","status":"backlog"}]}`), 0o600); err != nil {
		t.Fatal(err)
	}
	s := newWorkboardTestServer(t, dataDir)
	rr := performWorkboardJSON(t, s, http.MethodGet, "/api/workboard", "", http.StatusOK)
	var state workboardState
	decodeWorkboardResponse(t, rr, &state)
	if state.Items[0].Revision != 1 || state.Items[0].Events == nil || state.Items[0].Proposal.Evidence == nil {
		t.Fatalf("legacy item not normalized: %+v", state.Items[0])
	}
}

func performWorkboardJSON(t *testing.T, s *Server, method, path, body string, want int) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	if method != http.MethodGet {
		markDangerous(req)
	}
	rr := httptest.NewRecorder()
	if strings.HasPrefix(path, "/api/workboard/") {
		s.requireDangerousConfirm(s.workboardItem)(rr, req)
	} else {
		s.requireDangerousConfirm(s.workboard)(rr, req)
	}
	if rr.Code != want {
		t.Fatalf("%s %s status=%d want=%d body=%s", method, path, rr.Code, want, rr.Body.String())
	}
	return rr
}

func decodeWorkboardResponse(t *testing.T, rr *httptest.ResponseRecorder, dst any) {
	t.Helper()
	if err := json.NewDecoder(rr.Body).Decode(dst); err != nil {
		t.Fatal(err)
	}
}

func newWorkboardTestServer(t *testing.T, dataDir string) *Server {
	t.Helper()
	cfg := config.NewStore(t.TempDir())
	updateTestConfig(t, cfg, func(app *config.AppConfig) { app.GARoot = t.TempDir(); app.ChatDataDir = dataDir })
	return New(cfg, nil, modelconfig.NewStore(t.TempDir()), nil)
}
