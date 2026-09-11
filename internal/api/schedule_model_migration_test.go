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
	"genericagent-admin-go/internal/ga"
)

func TestScheduleModelMigrationPreviewsAndWritesLegacyTasks(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "sche_tasks"), 0755); err != nil {
		t.Fatal(err)
	}
	const agentmain = `
class Backend:
    name = "provider"
    model = "model"
    config = {}

class Client:
    backend = Backend()

class GenericAgent:
    def __init__(self):
        self.llmclients = [Client(), Client()]

    def list_llms(self):
        return [(0, "model zero", True), (1, "model one", True)]
`
	if err := os.WriteFile(filepath.Join(root, "agentmain.py"), []byte(agentmain), 0644); err != nil {
		t.Fatal(err)
	}
	writeTask := func(id, body string) {
		t.Helper()
		if err := os.WriteFile(filepath.Join(root, "sche_tasks", id+".json"), []byte(body), 0644); err != nil {
			t.Fatal(err)
		}
	}
	writeTask("legacy", `{"schedule":"09:00","repeat":"daily","enabled":true,"prompt":"legacy","llm_no":1}`)
	writeTask("missing", `{"schedule":"09:00","repeat":"daily","enabled":true,"prompt":"missing","llm_no":9}`)
	writeTask("default", `{"schedule":"09:00","repeat":"daily","enabled":true,"prompt":"default"}`)
	stableBody, err := json.Marshal(map[string]any{
		"schedule": "09:00", "repeat": "daily", "enabled": true, "prompt": "stable", "llm_no": 0,
		"model_key": ga.ScheduleModelKey("model", "provider", ""),
	})
	if err != nil {
		t.Fatal(err)
	}
	writeTask("stable", string(stableBody))

	s := newGoalTestServer(t, root)
	s.CfgStore.UpdateRuntime(func(cfg *config.AppConfig) { cfg.PythonPath = "python" })
	h := s.Routes()

	request := func(method, body string, confirmed bool) *httptest.ResponseRecorder {
		t.Helper()
		rr := httptest.NewRecorder()
		req := httptest.NewRequest(method, "/api/schedule/model-migration", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		if confirmed {
			req.Header.Set("X-GA-Confirm", "dangerous")
		}
		h.ServeHTTP(rr, req)
		return rr
	}

	preview := request(http.MethodGet, "", false)
	if preview.Code != http.StatusOK {
		t.Fatalf("preview status=%d body=%s", preview.Code, preview.Body.String())
	}
	var plan scheduleModelMigrationResponse
	if err := json.Unmarshal(preview.Body.Bytes(), &plan); err != nil {
		t.Fatal(err)
	}
	if plan.Total != 4 || plan.Migratable != 1 || plan.Stable != 1 || plan.Skipped != 1 || plan.SchedulerDefault != 1 {
		t.Fatalf("unexpected migration preview: %#v", plan)
	}
	var legacyPreview scheduleModelMigrationItem
	for _, item := range plan.Items {
		if item.ID == "legacy" {
			legacyPreview = item
		}
	}
	if legacyPreview.Status != "migratable" || legacyPreview.ModelKey != ga.ScheduleModelKey("model", "provider", "") || legacyPreview.ModelLabel != "model one" {
		t.Fatalf("legacy preview=%#v", legacyPreview)
	}

	if rr := request(http.MethodPost, `{}`, false); rr.Code != http.StatusPreconditionRequired {
		t.Fatalf("unconfirmed migration status=%d body=%s", rr.Code, rr.Body.String())
	}
	result := request(http.MethodPost, `{}`, true)
	if result.Code != http.StatusOK {
		t.Fatalf("migration status=%d body=%s", result.Code, result.Body.String())
	}
	var migrated scheduleModelMigrationResponse
	if err := json.Unmarshal(result.Body.Bytes(), &migrated); err != nil {
		t.Fatal(err)
	}
	if migrated.Migrated != 1 || migrated.Migratable != 0 || migrated.Stable != 2 {
		t.Fatalf("unexpected migration result: %#v", migrated)
	}
	raw, err := os.ReadFile(filepath.Join(root, "sche_tasks", "legacy.json"))
	if err != nil {
		t.Fatal(err)
	}
	var legacy map[string]any
	if err := json.Unmarshal(raw, &legacy); err != nil {
		t.Fatal(err)
	}
	if legacy["prompt"] != "legacy" || legacy["llm_no"] != float64(1) || legacy["model_key"] != ga.ScheduleModelKey("model", "provider", "") {
		t.Fatalf("legacy task was not preserved and migrated: %#v", legacy)
	}
	backups, err := filepath.Glob(filepath.Join(root, "sche_tasks", "legacy.json.bak.*"))
	if err != nil || len(backups) != 1 {
		t.Fatalf("expected one legacy backup, matches=%v err=%v", backups, err)
	}

	second := request(http.MethodGet, "", false)
	if second.Code != http.StatusOK || !strings.Contains(second.Body.String(), `"migratable":0`) || !strings.Contains(second.Body.String(), `"status":"stable"`) {
		t.Fatalf("post-migration preview status/body=%d %s", second.Code, second.Body.String())
	}
}
