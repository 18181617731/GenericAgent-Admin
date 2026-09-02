package api

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"genericagent-admin-go/internal/config"
	"genericagent-admin-go/internal/service"
)

func TestInstanceCreateCloneReturnsInitializingAndCompletesInBackground(t *testing.T) {
	appRoot := t.TempDir()
	sourceRoot := filepath.Join(t.TempDir(), "source")
	if err := writeValidGenericAgentFixture(sourceRoot); err != nil {
		t.Fatal(err)
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
	t.Cleanup(server.stopInstanceInstalls)
	oldClone := cloneProjectForInstance
	started := make(chan struct{})
	progressed := make(chan struct{})
	release := make(chan struct{})
	cloneProjectForInstance = func(ctx context.Context, src, dest string, copyMemory, copyMyKey bool, onProgress cloneProgressFunc) error {
		close(started)
		if onProgress != nil {
			onProgress(1, 2, 1, 2)
		}
		close(progressed)
		select {
		case <-release:
		case <-ctx.Done():
			return ctx.Err()
		}
		if err := writeValidGenericAgentFixture(dest); err != nil {
			return err
		}
		if onProgress != nil {
			onProgress(1, 1, 1, 1)
		}
		return nil
	}
	t.Cleanup(func() { cloneProjectForInstance = oldClone })

	body, err := json.Marshal(map[string]interface{}{"id": "clone", "name": "Clone", "source_instance_id": "source"})
	if err != nil {
		t.Fatal(err)
	}
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/instances/create", strings.NewReader(string(body)))
	markDangerous(req)
	server.Routes().ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("clone status=%d body=%s", rr.Code, rr.Body.String())
	}
	var response instanceInstallResponse
	if err := json.Unmarshal(rr.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode clone response: %v body=%s", err, rr.Body.String())
	}
	if response.Instance.InitStatus != config.InstanceInitStatusInitializing || response.Instance.InitStage != "queued" || response.Instance.InitProgress != 5 {
		t.Fatalf("clone response state=%+v want queued initialization", response.Instance)
	}
	select {
	case <-started:
	case <-time.After(3 * time.Second):
		t.Fatal("timed out waiting for clone worker")
	}
	select {
	case <-progressed:
	case <-time.After(3 * time.Second):
		t.Fatal("timed out waiting for clone progress")
	}
	queued, ok := store.Snapshot().Instance("clone")
	if !ok || queued.InitStatus != config.InstanceInitStatusInitializing || queued.InitStage != "cloning" || queued.InitProgress != 52 {
		t.Fatalf("queued clone state=%+v exists=%v", queued, ok)
	}
	close(release)
	waitInstanceInstallTasksForTest(t, server)
	completed, ok := store.Snapshot().Instance("clone")
	if !ok || completed.InitStatus != config.InstanceInitStatusReady || completed.InitProgress != 100 || completed.InitError != "" {
		t.Fatalf("completed clone state=%+v exists=%v", completed, ok)
	}
}

func TestInstanceCreateCloneFailureIsPersistedForUserRecovery(t *testing.T) {
	appRoot := t.TempDir()
	sourceRoot := filepath.Join(t.TempDir(), "source")
	if err := writeValidGenericAgentFixture(sourceRoot); err != nil {
		t.Fatal(err)
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
	t.Cleanup(server.stopInstanceInstalls)
	oldClone := cloneProjectForInstance
	cloneProjectForInstance = func(_ context.Context, _ string, dest string, _ bool, _ bool, _ cloneProgressFunc) error {
		_ = os.RemoveAll(dest)
		return errors.New("source copy interrupted")
	}
	t.Cleanup(func() { cloneProjectForInstance = oldClone })
	body, err := json.Marshal(map[string]interface{}{"id": "failed-clone", "name": "Failed clone", "source_instance_id": "source"})
	if err != nil {
		t.Fatal(err)
	}
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/instances/create", strings.NewReader(string(body)))
	markDangerous(req)
	server.Routes().ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("clone status=%d body=%s", rr.Code, rr.Body.String())
	}
	waitInstanceInstallTasksForTest(t, server)
	failed, ok := store.Snapshot().Instance("failed-clone")
	if !ok || failed.InitStatus != config.InstanceInitStatusFailed || !strings.Contains(failed.InitError, "source copy interrupted") {
		t.Fatalf("failed clone state=%+v exists=%v", failed, ok)
	}
	if info, err := os.Stat(failed.GARoot); err != nil || !info.IsDir() {
		t.Fatalf("failed clone root was not restored for config validation: info=%v err=%v", info, err)
	}
}

func TestInstanceCloneResumesAfterServerRestart(t *testing.T) {
	oldClone := cloneProjectForInstance
	var calls atomic.Int32
	started := make(chan string, 2)
	cloneProjectForInstance = func(ctx context.Context, _, dest string, _, _ bool, _ cloneProgressFunc) error {
		call := calls.Add(1)
		started <- dest
		if call == 1 {
			<-ctx.Done()
			return ctx.Err()
		}
		if err := os.Remove(filepath.Join(dest, instanceCloneReservationFile)); err != nil && !os.IsNotExist(err) {
			return err
		}
		return writeValidGenericAgentFixture(dest)
	}
	t.Cleanup(func() { cloneProjectForInstance = oldClone })

	appRoot := t.TempDir()
	sourceRoot := filepath.Join(t.TempDir(), "source")
	if err := writeValidGenericAgentFixture(sourceRoot); err != nil {
		t.Fatal(err)
	}
	store := config.NewStore(appRoot)
	cfg := store.Snapshot()
	cfg.GARoot = sourceRoot
	cfg.DefaultInstanceID = "source"
	cfg.Instances = []config.InstanceConfig{{ID: "source", Name: "Source", GARoot: sourceRoot}}
	if err := store.Save(cfg); err != nil {
		t.Fatal(err)
	}

	firstServer := New(store, service.NewManager(sourceRoot, cfg.BufferLines), nil, nil)
	responseBody := `{"id":"resumed-clone","name":"Resumed clone","source_instance_id":"source"}`
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/instances/create", strings.NewReader(responseBody))
	markDangerous(req)
	firstServer.Routes().ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("clone status=%d body=%s", rr.Code, rr.Body.String())
	}
	firstDest := waitTestSignal(t, started, "initial clone")
	firstServer.stopInstanceInstalls()
	stateAfterStop, ok := store.Snapshot().Instance("resumed-clone")
	if !ok || stateAfterStop.InitStatus != config.InstanceInitStatusInitializing || stateAfterStop.InitSourceInstanceID != "source" {
		t.Fatalf("state after shutdown=%+v exists=%v", stateAfterStop, ok)
	}

	reloaded := config.NewStore(appRoot)
	if err := reloaded.Load(); err != nil {
		t.Fatalf("reload config: %v", err)
	}
	resumedServer := New(reloaded, nil, nil, nil)
	t.Cleanup(resumedServer.stopInstanceInstalls)
	secondDest := waitTestSignal(t, started, "resumed clone")
	if secondDest != firstDest {
		t.Errorf("resumed destination=%q want %q", secondDest, firstDest)
	}
	waitInstanceInstallTasksForTest(t, resumedServer)
	completed, ok := reloaded.Snapshot().Instance("resumed-clone")
	if !ok || completed.InitStatus != config.InstanceInitStatusReady || completed.InitProgress != 100 || completed.InitSourceInstanceID != "" {
		t.Fatalf("resumed clone state=%+v exists=%v", completed, ok)
	}
	if calls.Load() != 2 {
		t.Fatalf("clone calls=%d want 2", calls.Load())
	}
}

func TestCloneSkipsNestedRustBuildOutput(t *testing.T) {
	sourceRoot := t.TempDir()
	destinationRoot := filepath.Join(t.TempDir(), "clone")
	if err := writeValidGenericAgentFixture(sourceRoot); err != nil {
		t.Fatal(err)
	}
	generated := filepath.Join(sourceRoot, "frontends", "desktop", "src-tauri", "target", "debug", "generated.bin")
	if err := os.MkdirAll(filepath.Dir(generated), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(generated, []byte("generated build output"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := cloneGenericAgentProject(sourceRoot, destinationRoot, false, false); err != nil {
		t.Fatalf("clone returned error: %v", err)
	}
	if _, err := os.Stat(filepath.Join(destinationRoot, "agentmain.py")); err != nil {
		t.Fatalf("source file missing from clone: %v", err)
	}
	if _, err := os.Stat(filepath.Join(destinationRoot, "frontends", "desktop", "src-tauri", "target")); !os.IsNotExist(err) {
		t.Fatalf("nested Rust target was copied, err=%v", err)
	}
}

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
		waitInstanceInstallTasksForTest(t, server)
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
