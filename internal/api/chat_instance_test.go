package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"genericagent-admin-go/internal/config"
	"genericagent-admin-go/internal/service"
)

func mustChatServerForRequest(t *testing.T, s *Server, target string) (*Server, string) {
	t.Helper()
	resolved, instanceID, err := s.chatServerForRequest(httptest.NewRequest("GET", target, nil))
	if err != nil {
		t.Fatalf("chatServerForRequest(%q) error = %v", target, err)
	}
	return resolved, instanceID
}

func TestChatInstanceLegacyRuntimeAndDataDirSurviveDefaultMigration(t *testing.T) {
	appRoot := t.TempDir()
	gaRoot := filepath.Join(t.TempDir(), "ga")
	if err := os.MkdirAll(gaRoot, 0755); err != nil {
		t.Fatal(err)
	}

	store := config.NewStore(appRoot)
	server := New(store, nil, nil, nil)
	before, beforeID := mustChatServerForRequest(t, server, "/api/chat/sessions")
	if beforeID != "" {
		t.Fatalf("initial instance ID = %q, want legacy empty ID", beforeID)
	}
	wantDataDir := filepath.Join(appRoot, "data")
	if got := before.CfgStore.Snapshot().ChatDataDir; got != wantDataDir {
		t.Fatalf("initial chat data dir = %q, want %q", got, wantDataDir)
	}
	before.ChatTitleJobs["legacy-job"] = true
	legacyRuntime := before.ChatMu

	cfg := store.Snapshot()
	cfg.GARoot = gaRoot
	if err := store.Save(cfg); err != nil {
		t.Fatalf("Save() migration error = %v", err)
	}
	if store.Snapshot().DefaultInstanceID != "default" || len(store.Snapshot().Instances) != 1 {
		t.Fatalf("Save() did not synthesize default instance: %#v", store.Snapshot())
	}

	after, afterID := mustChatServerForRequest(t, server, "/api/chat/sessions")
	if afterID != "default" {
		t.Fatalf("migrated instance ID = %q, want default", afterID)
	}
	if got := after.CfgStore.Snapshot().ChatDataDir; got != wantDataDir {
		t.Fatalf("migrated chat data dir = %q, want stable %q", got, wantDataDir)
	}
	if after.ChatMu != legacyRuntime {
		t.Fatal("default migration replaced the legacy in-memory chat runtime")
	}
	if !after.ChatTitleJobs["legacy-job"] {
		t.Fatal("default migration lost state from the legacy in-memory chat runtime")
	}
}

func TestChatInstanceDataAndRuntimeOwnershipAreStableAcrossDefaultSwitch(t *testing.T) {
	appRoot := t.TempDir()
	defaultRoot := filepath.Join(t.TempDir(), "ga-default")
	betaRoot := filepath.Join(t.TempDir(), "ga-beta")
	for _, root := range []string{defaultRoot, betaRoot} {
		if err := os.MkdirAll(root, 0755); err != nil {
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
		t.Fatalf("Save() error = %v", err)
	}

	server := New(store, nil, nil, nil)
	defaultServer, defaultID := mustChatServerForRequest(t, server, "/api/chat/sessions?instance_id=default")
	betaServer, betaID := mustChatServerForRequest(t, server, "/api/chat/sessions?instance_id=beta")
	if defaultID != "default" || betaID != "beta" {
		t.Fatalf("resolved IDs = %q, %q; want default, beta", defaultID, betaID)
	}
	baseDataDir := filepath.Join(appRoot, "data")
	betaDataDir := filepath.Join(baseDataDir, "instances", "beta")
	if got := defaultServer.CfgStore.Snapshot().ChatDataDir; got != baseDataDir {
		t.Fatalf("default data dir = %q, want %q", got, baseDataDir)
	}
	if got := betaServer.CfgStore.Snapshot().ChatDataDir; got != betaDataDir {
		t.Fatalf("beta data dir = %q, want %q", got, betaDataDir)
	}
	betaCfg := betaServer.CfgStore.Snapshot()
	if betaCfg.GARoot != betaRoot {
		t.Fatalf("beta GA root = %q, want %q", betaCfg.GARoot, betaRoot)
	}
	if betaCfg.DefaultInstanceID != "beta" || len(betaCfg.Instances) != 1 || betaCfg.Instances[0].ID != "beta" {
		t.Fatalf("beta runtime instance scope = default %q, instances %#v; want beta only", betaCfg.DefaultInstanceID, betaCfg.Instances)
	}
	if defaultServer.ChatMu == betaServer.ChatMu {
		t.Fatal("default and beta unexpectedly share a chat runtime")
	}
	defaultServer.ChatTitleJobs["default-only"] = true
	if betaServer.ChatTitleJobs["default-only"] {
		t.Fatal("default runtime state leaked into beta")
	}
	defaultRuntime := defaultServer.ChatMu
	betaRuntime := betaServer.ChatMu

	next := store.Snapshot()
	next.DefaultInstanceID = "beta"
	if err := store.Save(next); err != nil {
		t.Fatalf("Save(default=beta) error = %v", err)
	}

	defaultAgain, _ := mustChatServerForRequest(t, server, "/api/chat/sessions?instance_id=default")
	betaAgain, _ := mustChatServerForRequest(t, server, "/api/chat/sessions?instance_id=beta")
	selected, selectedID := mustChatServerForRequest(t, server, "/api/chat/sessions")
	if selectedID != "beta" {
		t.Fatalf("implicit instance ID = %q, want beta", selectedID)
	}
	if defaultAgain.CfgStore.Snapshot().ChatDataDir != baseDataDir || defaultAgain.ChatMu != defaultRuntime {
		t.Fatalf("literal default ownership changed after default switch: dir=%q", defaultAgain.CfgStore.Snapshot().ChatDataDir)
	}
	if betaAgain.CfgStore.Snapshot().ChatDataDir != betaDataDir || betaAgain.ChatMu != betaRuntime {
		t.Fatalf("literal beta ownership changed after default switch: dir=%q", betaAgain.CfgStore.Snapshot().ChatDataDir)
	}
	if selected.CfgStore.Snapshot().ChatDataDir != betaDataDir || selected.ChatMu != betaRuntime {
		t.Fatalf("implicit selection did not use beta ownership: dir=%q", selected.CfgStore.Snapshot().ChatDataDir)
	}
	if !defaultAgain.ChatTitleJobs["default-only"] || betaAgain.ChatTitleJobs["default-only"] {
		t.Fatal("runtime state isolation changed after switching the default instance")
	}
}

func TestChatInstanceProjectsFollowSelectedInstance(t *testing.T) {
	appRoot := t.TempDir()
	defaultRoot := filepath.Join(t.TempDir(), "ga-default")
	betaRoot := filepath.Join(t.TempDir(), "ga-beta")
	for root, project := range map[string]string{defaultRoot: "default-project", betaRoot: "beta-project"} {
		if err := os.MkdirAll(filepath.Join(root, "temp", "projects", project), 0755); err != nil {
			t.Fatal(err)
		}
	}

	store := config.NewStore(appRoot)
	cfg := store.Snapshot()
	cfg.DefaultInstanceID = "default"
	cfg.Instances = []config.InstanceConfig{
		{ID: "default", Name: "Default", GARoot: defaultRoot},
		{ID: "beta", Name: "Beta", GARoot: betaRoot},
	}
	if err := store.Save(cfg); err != nil {
		t.Fatalf("Save() error = %v", err)
	}

	server := New(store, nil, nil, nil)
	for instanceID, want := range map[string]string{"default": "default-project", "beta": "beta-project"} {
		rr := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/api/chat/sessions?instance_id="+instanceID, nil)
		server.Routes().ServeHTTP(rr, req)
		if rr.Code != http.StatusOK {
			t.Fatalf("instance %q status = %d, body = %s", instanceID, rr.Code, rr.Body.String())
		}
		var payload struct {
			Projects []string `json:"projects"`
		}
		if err := json.Unmarshal(rr.Body.Bytes(), &payload); err != nil {
			t.Fatalf("instance %q decode error = %v", instanceID, err)
		}
		if len(payload.Projects) != 1 || payload.Projects[0] != want {
			t.Fatalf("instance %q projects = %#v, want [%q]", instanceID, payload.Projects, want)
		}
	}
}

func TestChatInstanceRouteReturnsNotFoundForUnknownInstance(t *testing.T) {
	server := New(config.NewStore(t.TempDir()), nil, nil, nil)
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/chat/sessions?instance_id=missing", nil)

	server.Routes().ServeHTTP(rr, req)

	if rr.Code != http.StatusNotFound {
		t.Fatalf("status=%d want=%d body=%s", rr.Code, http.StatusNotFound, rr.Body.String())
	}
}

func TestInstanceManagerRegistryClearsFallbackRootWhenLastInstanceIsRemoved(t *testing.T) {
	oldRoot := t.TempDir()
	fallback := service.NewManagerWithPython(oldRoot, "legacy-python", 100)
	registry := newInstanceManagerRegistry(config.AppConfig{
		GARoot:            oldRoot,
		EffectivePython:   "legacy-python",
		BufferLines:       100,
		DefaultInstanceID: "default",
		Instances: []config.InstanceConfig{{
			ID:              "default",
			GARoot:          oldRoot,
			EffectivePython: "legacy-python",
		}},
	}, fallback)

	got, err := registry.reconcile(config.AppConfig{BufferLines: 321})
	if err != nil {
		t.Fatalf("reconcile(empty) error = %v", err)
	}
	if got != fallback {
		t.Fatal("reconcile(empty) replaced the compatibility fallback manager")
	}
	if fallback.GARoot != "" || fallback.EffectivePython != "" {
		t.Fatalf("fallback retained removed instance paths: root=%q python=%q", fallback.GARoot, fallback.EffectivePython)
	}
	if fallback.BufferLines != 321 {
		t.Fatalf("fallback buffer lines = %d, want 321", fallback.BufferLines)
	}
	resolved, instanceID, ok := registry.manager("")
	if !ok || resolved != fallback || instanceID != "" {
		t.Fatalf("legacy fallback resolution = (%p, %q, %v), want (%p, empty, true)", resolved, instanceID, ok, fallback)
	}
}
