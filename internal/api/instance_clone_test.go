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
	"genericagent-admin-go/internal/service"
)

func TestInstanceCreateCanCloneProjectWithExplicitSensitiveDataChoices(t *testing.T) {
	appRoot := t.TempDir()
	sourceRoot := filepath.Join(t.TempDir(), "source")
	if err := os.MkdirAll(filepath.Join(sourceRoot, "assets"), 0755); err != nil {
		t.Fatal(err)
	}
	for rel, content := range map[string]string{
		"agentmain.py":                      "# source agent\n",
		"llmcore.py":                        "# source llm\n",
		"assets/tools_schema.json":          "{}\n",
		"mykey.py":                          "SECRET = 'must be opt-in'\n",
		"mykey.py.bak-20260901":             "SECRET = 'old backup'\n",
		".env.local":                        "SECRET_ENV=must-not-copy\n",
		"model_profiles.json":               "{\"profiles\":[] }\n",
		"reflect/worker.py":                 "# worker\n",
		"sche_tasks/done/source-report.md":  "source schedule report\n",
		"temp/TODO.txt":                     "[ ] source autonomous item | 只读检查\n",
		"temp/autonomous/tasks.json":        "{\"tasks\":[] }\n",
		"temp/autonomous/control/stop":      "volatile signal\n",
		"temp/autonomous_reports/source.md": "source autonomous report\n",
		"temp/volatile-runtime.txt":         "must not copy\n",
		"memory/global_mem.txt":             "source memory\n",
		"memory/sop.md":                     "source SOP\n",
		".git/config":                       "must not copy\n",
	} {
		path := filepath.Join(sourceRoot, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(content), 0644); err != nil {
			t.Fatal(err)
		}
	}

	store := config.NewStore(appRoot)
	cfg := store.Snapshot()
	cfg.GARoot = sourceRoot
	cfg.DefaultInstanceID = "source"
	cfg.Instances = []config.InstanceConfig{{ID: "source", Name: "Source", GARoot: sourceRoot}}
	if err := store.Save(cfg); err != nil {
		t.Fatal(err)
	}
	server := New(store, service.NewManager(sourceRoot, cfg.BufferLines), nil, nil)
	h := server.Routes()

	create := func(id string, copyMemory, copyMyKey bool, root string) config.InstanceConfig {
		t.Helper()
		payload := map[string]interface{}{
			"id": id, "name": id, "source_instance_id": "source",
			"copy_memory": copyMemory, "copy_mykey": copyMyKey,
		}
		if root != "" {
			payload["ga_root"] = root
		}
		body, err := json.Marshal(payload)
		if err != nil {
			t.Fatal(err)
		}
		rr := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/api/instances/create", strings.NewReader(string(body)))
		markDangerous(req)
		h.ServeHTTP(rr, req)
		if rr.Code != http.StatusOK {
			t.Fatalf("clone %s status=%d body=%s", id, rr.Code, rr.Body.String())
		}
		instance, ok := server.CfgStore.Snapshot().Instance(id)
		if !ok {
			t.Fatalf("clone %s missing from config", id)
		}
		return instance
	}

	withoutSecrets := create("clone", false, false, "")
	managedRoot := filepath.Join(appRoot, automaticInstanceBaseID, "instances", "clone")
	if withoutSecrets.GARoot != managedRoot || withoutSecrets.InitStatus != config.InstanceInitStatusReady || withoutSecrets.InitProgress != 100 {
		t.Fatalf("cloned instance=%+v want managed ready instance", withoutSecrets)
	}
	for _, rel := range []string{"agentmain.py", "llmcore.py", "assets/tools_schema.json", "reflect/worker.py", "sche_tasks/done/source-report.md", "temp/TODO.txt", "temp/autonomous/tasks.json", "temp/autonomous_reports/source.md"} {
		if _, err := os.Stat(filepath.Join(managedRoot, filepath.FromSlash(rel))); err != nil {
			t.Errorf("expected copied file %s: %v", rel, err)
		}
	}
	for _, rel := range []string{"memory/global_mem.txt", "memory/sop.md", "mykey.py", "mykey.py.bak-20260901", ".env.local", "model_profiles.json", "temp/autonomous/control/stop", "temp/volatile-runtime.txt", ".git/config"} {
		if _, err := os.Stat(filepath.Join(managedRoot, filepath.FromSlash(rel))); !os.IsNotExist(err) {
			t.Errorf("sensitive or volatile file %s was copied: err=%v", rel, err)
		}
	}

	withSecrets := create("clone-copy", true, true, filepath.Join(appRoot, "custom-clone-copy"))
	for _, rel := range []string{"memory/global_mem.txt", "memory/sop.md", "mykey.py"} {
		if _, err := os.Stat(filepath.Join(withSecrets.GARoot, filepath.FromSlash(rel))); err != nil {
			t.Errorf("opt-in file %s was not copied: %v", rel, err)
		}
	}
	if got, _ := os.ReadFile(filepath.Join(withSecrets.GARoot, "mykey.py")); string(got) != "SECRET = 'must be opt-in'\n" {
		t.Fatalf("copied mykey content=%q", got)
	}

	overlap := filepath.Join(sourceRoot, "nested")
	body, _ := json.Marshal(map[string]interface{}{"id": "overlap", "name": "overlap", "source_instance_id": "source", "ga_root": overlap})
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/instances/create", strings.NewReader(string(body)))
	markDangerous(req)
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusBadRequest || !strings.Contains(rr.Body.String(), "contain one another") {
		t.Fatalf("overlap status=%d body=%s", rr.Code, rr.Body.String())
	}
	if _, err := os.Stat(overlap); !os.IsNotExist(err) {
		t.Fatalf("overlap destination was created: %v", err)
	}
}
