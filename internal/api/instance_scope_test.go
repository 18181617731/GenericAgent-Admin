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
	"genericagent-admin-go/internal/service"
)

func TestInstanceBoundRoutesUseTheSelectedProjectRoot(t *testing.T) {
	appRoot := t.TempDir()
	defaultRoot := filepath.Join(t.TempDir(), "default")
	betaRoot := filepath.Join(t.TempDir(), "beta")
	for _, root := range []string{defaultRoot, betaRoot} {
		if err := os.MkdirAll(root, 0755); err != nil {
			t.Fatal(err)
		}
	}
	writeScopeFixture := func(root, marker string) {
		t.Helper()
		if err := os.WriteFile(filepath.Join(root, "agentmain.py"), []byte(marker), 0644); err != nil {
			t.Fatal(err)
		}
		if err := os.MkdirAll(filepath.Join(root, "sche_tasks"), 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(root, "sche_tasks", "marker.txt"), []byte(marker), 0644); err != nil {
			t.Fatal(err)
		}
		if err := os.MkdirAll(filepath.Join(root, "temp"), 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(root, "temp", "TODO.txt"), []byte("[ ] | "+marker+"\n"), 0644); err != nil {
			t.Fatal(err)
		}
	}
	writeScopeFixture(defaultRoot, "default-marker")
	writeScopeFixture(betaRoot, "beta-marker")
	for root, id := range map[string]string{defaultRoot: "default-task", betaRoot: "beta-task"} {
		if _, err := ga.CreateTask(root, id, map[string]any{
			"enabled": true, "schedule": "09:00", "repeat": "daily", "prompt": id,
		}); err != nil {
			t.Fatal(err)
		}
	}

	store := config.NewStore(appRoot)
	cfg := store.Snapshot()
	cfg.GARoot = defaultRoot
	cfg.DefaultInstanceID = "default"
	cfg.Instances = []config.InstanceConfig{
		{ID: "default", Name: "Default", GARoot: defaultRoot},
		{ID: "beta", Name: "Beta", GARoot: betaRoot},
	}
	if err := store.Save(cfg); err != nil {
		t.Fatal(err)
	}
	server := New(store, service.NewManager(defaultRoot, cfg.BufferLines), nil, nil)
	h := server.Routes()

	request := func(method, target, instanceID string) *httptest.ResponseRecorder {
		t.Helper()
		rr := httptest.NewRecorder()
		req := httptest.NewRequest(method, target, nil)
		if instanceID != "" {
			req.Header.Set("X-GA-Instance-ID", instanceID)
		}
		h.ServeHTTP(rr, req)
		if rr.Code != http.StatusOK {
			t.Fatalf("%s %s status=%d body=%s", method, target, rr.Code, rr.Body.String())
		}
		return rr
	}

	var files struct {
		Items []struct {
			Name string `json:"name"`
		} `json:"items"`
	}
	if err := json.Unmarshal(request(http.MethodGet, "/api/files/list", "beta").Body.Bytes(), &files); err != nil {
		t.Fatal(err)
	}
	if len(files.Items) != 3 || !containsScopeName(files.Items, "agentmain.py") || !containsScopeName(files.Items, "sche_tasks") || !containsScopeName(files.Items, "temp") {
		t.Fatalf("beta files=%+v", files.Items)
	}

	var schedule struct {
		Tasks []struct {
			ID string `json:"id"`
		} `json:"tasks"`
	}
	if err := json.Unmarshal(request(http.MethodGet, "/api/schedule/tasks", "beta").Body.Bytes(), &schedule); err != nil {
		t.Fatal(err)
	}
	if len(schedule.Tasks) != 1 || schedule.Tasks[0].ID != "beta-task" {
		t.Fatalf("beta schedule tasks=%+v", schedule.Tasks)
	}
	if body := request(http.MethodGet, "/api/ga/inventory?instance_id=default", "beta").Body.String(); !strings.Contains(body, "default-task") || strings.Contains(body, "beta-task") {
		t.Fatalf("query instance did not take precedence over header: %s", body)
	}
	if body := request(http.MethodGet, "/api/ga/inventory", "beta").Body.String(); !strings.Contains(body, "beta-task") {
		t.Fatalf("beta inventory root missing: %s", body)
	}
	if body := request(http.MethodGet, "/api/autonomous/approvals", "beta").Body.String(); !strings.Contains(body, "beta-marker") {
		t.Fatalf("beta autonomous data missing: %s", body)
	}

	rr := httptest.NewRecorder()
	unknown := httptest.NewRequest(http.MethodGet, "/api/files/list", nil)
	unknown.Header.Set("X-GA-Instance-ID", "missing")
	h.ServeHTTP(rr, unknown)
	if rr.Code != http.StatusNotFound {
		t.Fatalf("unknown instance status=%d body=%s", rr.Code, rr.Body.String())
	}
}

func containsScopeName(items []struct {
	Name string `json:"name"`
}, want string) bool {
	for _, item := range items {
		if item.Name == want {
			return true
		}
	}
	return false
}

func TestSelectedInstanceConfigAndSetupChangesStayInThatInstance(t *testing.T) {
	appRoot := t.TempDir()
	defaultRoot := filepath.Join(t.TempDir(), "default")
	betaRoot := filepath.Join(t.TempDir(), "beta")
	if err := os.MkdirAll(defaultRoot, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(betaRoot, 0755); err != nil {
		t.Fatal(err)
	}
	store := config.NewStore(appRoot)
	cfg := store.Snapshot()
	cfg.GARoot = defaultRoot
	cfg.DefaultInstanceID = "default"
	cfg.Instances = []config.InstanceConfig{
		{ID: "default", Name: "Default", GARoot: defaultRoot},
		{ID: "beta", Name: "Beta", GARoot: betaRoot},
	}
	if err := store.Save(cfg); err != nil {
		t.Fatal(err)
	}
	server := New(store, service.NewManager(defaultRoot, cfg.BufferLines), nil, nil)
	h := server.Routes()

	configRequest := httptest.NewRequest(http.MethodGet, "/api/config", nil)
	configRequest.Header.Set("X-GA-Instance-ID", "beta")
	configResponse := httptest.NewRecorder()
	h.ServeHTTP(configResponse, configRequest)
	if configResponse.Code != http.StatusOK {
		t.Fatalf("GET selected config status=%d body=%s", configResponse.Code, configResponse.Body.String())
	}
	var selected config.AppConfig
	if err := json.Unmarshal(configResponse.Body.Bytes(), &selected); err != nil {
		t.Fatal(err)
	}
	if selected.GARoot != betaRoot || len(selected.Instances) != 2 {
		t.Fatalf("selected config=%+v, want beta projection with registry", selected)
	}

	stateRequest := httptest.NewRequest(http.MethodGet, "/api/setup/state", nil)
	stateRequest.Header.Set("X-GA-Instance-ID", "beta")
	stateResponse := httptest.NewRecorder()
	h.ServeHTTP(stateResponse, stateRequest)
	var state struct {
		GARoot string `json:"ga_root"`
	}
	if err := json.Unmarshal(stateResponse.Body.Bytes(), &state); err != nil {
		t.Fatal(err)
	}
	if stateResponse.Code != http.StatusOK || state.GARoot != betaRoot {
		t.Fatalf("GET selected setup state status=%d body=%s", stateResponse.Code, stateResponse.Body.String())
	}

	currentExe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	body, err := json.Marshal(map[string]string{"path": currentExe})
	if err != nil {
		t.Fatal(err)
	}
	pythonRequest := httptest.NewRequest(http.MethodPost, "/api/setup/python/validate", strings.NewReader(string(body)))
	pythonRequest.Header.Set("X-GA-Instance-ID", "beta")
	markDangerous(pythonRequest)
	pythonResponse := httptest.NewRecorder()
	h.ServeHTTP(pythonResponse, pythonRequest)
	if pythonResponse.Code != http.StatusOK {
		t.Fatalf("POST selected Python validation status=%d body=%s", pythonResponse.Code, pythonResponse.Body.String())
	}
	saved := store.Snapshot()
	defaultInstance, _ := saved.Instance("default")
	betaInstance, _ := saved.Instance("beta")
	if betaInstance.PythonPath == "" || defaultInstance.PythonPath != "" {
		t.Fatalf("Python path crossed instance boundary: default=%q beta=%q", defaultInstance.PythonPath, betaInstance.PythonPath)
	}
}
