package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"genericagent-admin-go/internal/config"
	"genericagent-admin-go/internal/modelconfig"
	"genericagent-admin-go/internal/service"
)

func makeProjectScopeRoot(t *testing.T, root string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Join(root, "reflect"), 0755); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"agentmain.py", "reflect/scheduler.py"} {
		path := filepath.Join(root, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte("# test\n"), 0644); err != nil {
			t.Fatal(err)
		}
	}
}

func projectScopeRequest(t *testing.T, method, path, instanceID string, body interface{}) *http.Request {
	t.Helper()
	var payload *bytes.Reader
	if body == nil {
		payload = bytes.NewReader(nil)
	} else {
		data, err := json.Marshal(body)
		if err != nil {
			t.Fatal(err)
		}
		payload = bytes.NewReader(data)
	}
	req := httptest.NewRequest(method, path, payload)
	if instanceID != "" {
		req.Header.Set("X-GA-Instance-ID", instanceID)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	return req
}

func serveProjectScope(t *testing.T, handler http.Handler, req *http.Request, want int) *httptest.ResponseRecorder {
	t.Helper()
	if req.Method != http.MethodGet && req.Method != http.MethodHead {
		markDangerous(req)
	}
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != want {
		t.Fatalf("%s %s status=%d want=%d body=%s", req.Method, req.URL.Path, rec.Code, want, rec.Body.String())
	}
	return rec
}

func TestInstanceProjectStateStaysIsolatedAcrossConfigServiceChatAndModelRoutes(t *testing.T) {
	appRoot := t.TempDir()
	defaultRoot := filepath.Join(t.TempDir(), "default")
	betaRoot := filepath.Join(t.TempDir(), "beta")
	makeProjectScopeRoot(t, defaultRoot)
	makeProjectScopeRoot(t, betaRoot)
	baseChatDir := filepath.Join(appRoot, "chat-data")
	betaChatDir := filepath.Join(baseChatDir, "instances", "beta")

	store := config.NewStore(appRoot)
	defaultTitle := &config.ChatTitleModelRef{Enable: true, ProviderVarName: "native_oai_config1", Model: "default-title", LLMNo: 0}
	cfg := store.Snapshot()
	cfg.GARoot = defaultRoot
	cfg.ChatDataDir = baseChatDir
	cfg.DefaultInstanceID = "default"
	cfg.BootstrapDone = true
	cfg.ServiceAutostart = []string{"reflect/scheduler.py"}
	cfg.ServiceModels = map[string]int{"reflect/scheduler.py": 1}
	cfg.ModelProbeProviders = []string{"native_default"}
	cfg.ChatTitleModel = defaultTitle
	cfg.ChatDefaultLLMNo = 2
	cfg.SlashCommands = []config.SlashCommandItem{{Cmd: "/default", Desc: "default"}}
	cfg.ExtraSystemPromptPresets = []config.ExtraSystemPromptPreset{{ID: "default", Name: "Default", Content: "default"}}
	cfg.Instances = []config.InstanceConfig{
		{
			ID: "default", Name: "Default", GARoot: defaultRoot, ChatDataDir: baseChatDir,
			BootstrapDone: true, ServiceAutostart: []string{"reflect/scheduler.py"},
			ServiceModels: map[string]int{"reflect/scheduler.py": 1}, ModelProbeProviders: []string{"native_default"},
			ChatTitleModel: defaultTitle, ChatDefaultLLMNo: 2,
			SlashCommands:            []config.SlashCommandItem{{Cmd: "/default", Desc: "default"}},
			ExtraSystemPromptPresets: []config.ExtraSystemPromptPreset{{ID: "default", Name: "Default", Content: "default"}},
		},
		{
			ID: "beta", Name: "Beta", GARoot: betaRoot, ChatDataDir: betaChatDir,
			BootstrapDone: false, ServiceAutostart: nil,
			ServiceModels: map[string]int{"reflect/scheduler.py": 3}, ModelProbeProviders: []string{"native_beta"},
			ChatTitleModel: &config.ChatTitleModelRef{Enable: true, ProviderVarName: "native_oai_config2", Model: "beta-title", LLMNo: 0}, ChatDefaultLLMNo: 4,
			SlashCommands:            []config.SlashCommandItem{{Cmd: "/beta", Desc: "beta"}},
			ExtraSystemPromptPresets: []config.ExtraSystemPromptPreset{{ID: "beta", Name: "Beta", Content: "beta"}},
		},
	}
	if err := store.Save(cfg); err != nil {
		t.Fatal(err)
	}
	server := New(store, service.NewManager(defaultRoot, cfg.BufferLines), nil, nil)
	h := server.Routes()

	readConfig := func(instanceID string) config.AppConfig {
		rec := serveProjectScope(t, h, projectScopeRequest(t, http.MethodGet, "/api/config", instanceID, nil), http.StatusOK)
		var got config.AppConfig
		if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
			t.Fatal(err)
		}
		return got
	}
	defaultView := readConfig("default")
	betaView := readConfig("beta")
	if defaultView.GARoot != defaultRoot || betaView.GARoot != betaRoot || defaultView.ChatDataDir != baseChatDir || betaView.ChatDataDir != betaChatDir {
		t.Fatalf("config projections crossed roots or chat data: default=%+v beta=%+v", defaultView, betaView)
	}
	if defaultView.ChatDefaultLLMNo != 2 || betaView.ChatDefaultLLMNo != 4 || defaultView.SlashCommands[0].Cmd != "/default" || betaView.SlashCommands[0].Cmd != "/beta" {
		t.Fatalf("config project settings were not projected independently: default=%+v beta=%+v", defaultView, betaView)
	}
	defaultManager, _, _ := server.InstanceManagers.manager("default")
	betaManager, _, _ := server.InstanceManagers.manager("beta")
	if defaultManager.UsageDir != filepath.Join(baseChatDir, "usage_events") || betaManager.UsageDir != filepath.Join(betaChatDir, "usage_events") {
		t.Fatalf("usage directories crossed instances: default=%q beta=%q", defaultManager.UsageDir, betaManager.UsageDir)
	}

	betaView.ChatDefaultLLMNo = 9
	betaView.BootstrapDone = true
	betaView.ServiceAutostart = []string{"reflect/scheduler.py"}
	betaView.ServiceModels = map[string]int{"reflect/scheduler.py": 8}
	betaView.ModelProbeProviders = []string{"native_beta_changed"}
	betaView.SlashCommands = []config.SlashCommandItem{{Cmd: "/beta-changed", Desc: "beta changed"}}
	betaView.ExtraSystemPromptPresets = []config.ExtraSystemPromptPreset{{ID: "beta-changed", Name: "Beta changed", Content: "beta changed"}}
	betaView.ChatDataDir = filepath.Join(baseChatDir, "custom-beta-data")
	serveProjectScope(t, h, projectScopeRequest(t, http.MethodPut, "/api/config", "beta", betaView), http.StatusOK)
	saved := store.Snapshot()
	defaultSettings, _ := saved.Instance("default")
	betaSettings, _ := saved.Instance("beta")
	if defaultSettings.ChatDefaultLLMNo != 2 || betaSettings.ChatDefaultLLMNo != 9 || !defaultSettings.BootstrapDone || !betaSettings.BootstrapDone {
		t.Fatalf("config PUT crossed instance state: default=%+v beta=%+v", defaultSettings, betaSettings)
	}
	if defaultSettings.ChatDataDir != baseChatDir || betaSettings.ChatDataDir != betaView.ChatDataDir || !reflect.DeepEqual(defaultSettings.ServiceModels, map[string]int{"reflect/scheduler.py": 1}) || !reflect.DeepEqual(betaSettings.ServiceModels, betaView.ServiceModels) {
		t.Fatalf("config PUT did not persist independent project settings: default=%+v beta=%+v", defaultSettings, betaSettings)
	}

	serveProjectScope(t, h, projectScopeRequest(t, http.MethodPost, "/api/services/autostart", "beta", map[string]interface{}{"name": "reflect/scheduler.py", "enabled": false}), http.StatusOK)
	saved = store.Snapshot()
	defaultSettings, _ = saved.Instance("default")
	betaSettings, _ = saved.Instance("beta")
	if len(defaultSettings.ServiceAutostart) != 1 || len(betaSettings.ServiceAutostart) != 0 {
		t.Fatalf("service autostart crossed instances: default=%v beta=%v", defaultSettings.ServiceAutostart, betaSettings.ServiceAutostart)
	}
	serveProjectScope(t, h, projectScopeRequest(t, http.MethodPost, "/api/services/model", "beta", map[string]interface{}{"name": "reflect/scheduler.py", "llm_no": 7}), http.StatusOK)
	saved = store.Snapshot()
	defaultSettings, _ = saved.Instance("default")
	betaSettings, _ = saved.Instance("beta")
	if defaultSettings.ServiceModels["reflect/scheduler.py"] != 1 || betaSettings.ServiceModels["reflect/scheduler.py"] != 7 {
		t.Fatalf("service model crossed instances: default=%v beta=%v", defaultSettings.ServiceModels, betaSettings.ServiceModels)
	}

	serveProjectScope(t, h, projectScopeRequest(t, http.MethodPut, "/api/extra-system-prompt-presets", "beta", map[string]interface{}{"presets": []config.ExtraSystemPromptPreset{{ID: "beta-only", Name: "Beta only", Content: "beta only"}}}), http.StatusOK)
	saved = store.Snapshot()
	defaultSettings, _ = saved.Instance("default")
	betaSettings, _ = saved.Instance("beta")
	if defaultSettings.ExtraSystemPromptPresets[0].ID != "default" || betaSettings.ExtraSystemPromptPresets[0].ID != "beta-only" {
		t.Fatalf("prompt presets crossed instances: default=%v beta=%v", defaultSettings.ExtraSystemPromptPresets, betaSettings.ExtraSystemPromptPresets)
	}

	chatServer, _, err := server.chatServerForRequest(projectScopeRequest(t, http.MethodGet, "/api/chat/sessions", "beta", nil))
	if err != nil {
		t.Fatal(err)
	}
	chatServer.rememberDefaultChatLLMNo(11)
	saved = store.Snapshot()
	defaultSettings, _ = saved.Instance("default")
	betaSettings, _ = saved.Instance("beta")
	if defaultSettings.ChatDefaultLLMNo != 2 || betaSettings.ChatDefaultLLMNo != 11 {
		t.Fatalf("chat default model crossed instances: default=%d beta=%d", defaultSettings.ChatDefaultLLMNo, betaSettings.ChatDefaultLLMNo)
	}

	defaultProfiles := []modelconfig.Profile{{VarName: "native_oai_config1", Type: "native_oai", Name: "Default", Model: "default-title", APIBase: "https://default.example/v1", APIKey: "sk-default"}}
	betaProfiles := []modelconfig.Profile{{VarName: "native_oai_config2", Type: "native_oai", Name: "Beta", Model: "beta-title", APIBase: "https://beta.example/v1", APIKey: "sk-beta"}}
	if _, err := modelconfig.Export(defaultRoot, defaultProfiles, true); err != nil {
		t.Fatal(err)
	}
	if _, err := modelconfig.Export(betaRoot, betaProfiles, true); err != nil {
		t.Fatal(err)
	}
	readTitle := func(instanceID string) []chatTitleModelOption {
		rec := serveProjectScope(t, h, projectScopeRequest(t, http.MethodGet, "/api/models/title-model", instanceID, nil), http.StatusOK)
		var payload struct {
			Options []chatTitleModelOption `json:"options"`
		}
		if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
			t.Fatal(err)
		}
		return payload.Options
	}
	defaultOptions := readTitle("default")
	betaOptions := readTitle("beta")
	if len(defaultOptions) != 1 || defaultOptions[0].Model != "default-title" || len(betaOptions) != 1 || betaOptions[0].Model != "beta-title" {
		t.Fatalf("title model options crossed instances: default=%v beta=%v", defaultOptions, betaOptions)
	}
	serveProjectScope(t, h, projectScopeRequest(t, http.MethodPut, "/api/models/title-model", "beta", map[string]interface{}{"model": &config.ChatTitleModelRef{Enable: true, ProviderVarName: "native_oai_config2", Model: "beta-title", LLMNo: 99}}), http.StatusOK)
	saved = store.Snapshot()
	defaultSettings, _ = saved.Instance("default")
	betaSettings, _ = saved.Instance("beta")
	if defaultSettings.ChatTitleModel == nil || defaultSettings.ChatTitleModel.Model != "default-title" || betaSettings.ChatTitleModel == nil || betaSettings.ChatTitleModel.Model != "beta-title" {
		t.Fatalf("title model crossed instances: default=%v beta=%v", defaultSettings.ChatTitleModel, betaSettings.ChatTitleModel)
	}
}

func TestInstanceCloneCopiesProjectPreferencesButKeepsRuntimeAutostartOff(t *testing.T) {
	appRoot := t.TempDir()
	sourceRoot := filepath.Join(t.TempDir(), "source")
	destinationRoot := filepath.Join(t.TempDir(), "destination")
	makeProjectScopeRoot(t, sourceRoot)
	if err := os.MkdirAll(filepath.Join(sourceRoot, "memory"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(sourceRoot, "memory", "private.md"), []byte("private"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(sourceRoot, "mykey.py"), []byte("api_key = 'secret'\n"), 0600); err != nil {
		t.Fatal(err)
	}
	store := config.NewStore(appRoot)
	cfg := store.Snapshot()
	cfg.GARoot = sourceRoot
	cfg.DefaultInstanceID = "default"
	cfg.ServiceAutostart = []string{"reflect/scheduler.py"}
	cfg.ServiceModels = map[string]int{"reflect/scheduler.py": 3}
	cfg.ModelProbeProviders = []string{"native_source"}
	cfg.ChatDefaultLLMNo = 4
	cfg.SlashCommands = []config.SlashCommandItem{{Cmd: "/source", Desc: "source"}}
	cfg.ExtraSystemPromptPresets = []config.ExtraSystemPromptPreset{{ID: "source", Name: "Source", Content: "source"}}
	cfg.Instances = []config.InstanceConfig{{
		ID: "default", Name: "Default", GARoot: sourceRoot, ChatDataDir: filepath.Join(appRoot, "chat-data"),
		ServiceAutostart: []string{"reflect/scheduler.py"}, ServiceModels: map[string]int{"reflect/scheduler.py": 3},
		ModelProbeProviders: []string{"native_source"}, ChatDefaultLLMNo: 4,
		SlashCommands:            []config.SlashCommandItem{{Cmd: "/source", Desc: "source"}},
		ExtraSystemPromptPresets: []config.ExtraSystemPromptPreset{{ID: "source", Name: "Source", Content: "source"}},
	}}
	if err := store.Save(cfg); err != nil {
		t.Fatal(err)
	}
	server := New(store, service.NewManager(sourceRoot, cfg.BufferLines), nil, nil)
	request := projectScopeRequest(t, http.MethodPost, "/api/instances/create", "", map[string]interface{}{
		"id": "clone", "name": "Clone", "ga_root": destinationRoot, "source_instance_id": "default",
		"copy_memory": false, "copy_mykey": false,
	})
	markDangerous(request)
	serveProjectScope(t, server.Routes(), request, http.StatusOK)
	waitInstanceInstallTasksForTest(t, server)
	saved := store.Snapshot()
	clone, ok := saved.Instance("clone")
	if !ok {
		t.Fatal("clone instance was not registered")
	}
	if len(clone.ServiceAutostart) != 0 || clone.ServiceModels["reflect/scheduler.py"] != 3 || clone.ChatDefaultLLMNo != 4 || clone.SlashCommands[0].Cmd != "/source" {
		t.Fatalf("clone project preferences=%+v", clone)
	}
	if clone.ChatDataDir == filepath.Join(appRoot, "chat-data") || clone.ChatDataDir == "" {
		t.Fatalf("clone chat data directory was not isolated: %q", clone.ChatDataDir)
	}
	if _, err := os.Stat(filepath.Join(destinationRoot, "agentmain.py")); err != nil {
		t.Fatalf("clone did not copy project file: %v", err)
	}
	if _, err := os.Stat(filepath.Join(destinationRoot, "memory", "private.md")); !os.IsNotExist(err) {
		t.Fatalf("memory copied without opt-in, err=%v", err)
	}
	if _, err := os.Stat(filepath.Join(destinationRoot, "mykey.py")); !os.IsNotExist(err) {
		t.Fatalf("mykey copied without opt-in, err=%v", err)
	}
}
