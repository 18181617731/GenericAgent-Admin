package api

import (
	"bufio"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"genericagent-admin-go/internal/config"
	"genericagent-admin-go/internal/modelconfig"
	"genericagent-admin-go/internal/service"
)

func newServiceHandlerTestServer(t *testing.T, gaRoot string) *Server {
	t.Helper()
	cfg := newTestConfigStore(t, t.TempDir(), config.Default())
	updateTestConfig(t, cfg, func(cfg *config.AppConfig) {
		cfg.GARoot = gaRoot
	})
	updateTestConfig(t, cfg, func(cfg *config.AppConfig) {
		cfg.LogTailLines = 1
	})
	models := modelconfig.NewStore(t.TempDir())
	return New(cfg, service.NewManager(cfg.Snapshot().GARoot, cfg.Snapshot().BufferLines), models, nil)
}

func TestLogsRouteRejectsUnknownAndEmptyService(t *testing.T) {
	h := newServiceHandlerTestServer(t, t.TempDir()).Routes()
	for _, tc := range []struct {
		path string
		want int
		body string
	}{
		{path: "/api/logs/", want: http.StatusBadRequest, body: "service name required"},
		{path: "/api/logs/missing.py", want: http.StatusNotFound, body: "service not found"},
	} {
		t.Run(tc.path, func(t *testing.T) {
			rr := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodGet, tc.path, nil)
			h.ServeHTTP(rr, req)
			if rr.Code != tc.want || !strings.Contains(rr.Body.String(), tc.body) {
				t.Fatalf("status=%d want=%d body=%s", rr.Code, tc.want, rr.Body.String())
			}
		})
	}
}

func TestLogsRouteReturnsKnownServiceTail(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "reflect"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "reflect", "custom_reflect.py"), []byte("# test\n"), 0644); err != nil {
		t.Fatal(err)
	}
	h := newServiceHandlerTestServer(t, root).Routes()

	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/logs/reflect%2Fcustom_reflect.py", nil)
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("status=%d want=%d body=%s", rr.Code, http.StatusOK, rr.Body.String())
	}
	if !strings.Contains(rr.Body.String(), `"name":"reflect/custom_reflect.py"`) || !strings.Contains(rr.Body.String(), `"lines":[]`) {
		t.Fatalf("unexpected body=%s", rr.Body.String())
	}
}

func TestLogsRouteRejectsNonGET(t *testing.T) {
	h := newServiceHandlerTestServer(t, t.TempDir()).Routes()
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/logs/app.py", strings.NewReader(`{}`))
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusMethodNotAllowed {
		t.Fatalf("status=%d want=%d body=%s", rr.Code, http.StatusMethodNotAllowed, rr.Body.String())
	}
}

func TestStopRouteRejectsUnknownService(t *testing.T) {
	h := newServiceHandlerTestServer(t, t.TempDir()).Routes()
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/services/stop", strings.NewReader(`{"name":"missing.py"}`))
	req.Header.Set("X-GA-Confirm", "dangerous")
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusBadRequest || !strings.Contains(rr.Body.String(), "service not found") {
		t.Fatalf("status=%d want=%d body=%s", rr.Code, http.StatusBadRequest, rr.Body.String())
	}
}

func TestServiceDangerousRoutesRequireConfirm(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "launch.py"), []byte("# test\n"), 0644); err != nil {
		t.Fatal(err)
	}
	h := newServiceHandlerTestServer(t, root).Routes()
	for _, tc := range []struct {
		path string
		body string
	}{
		{path: "/api/services/start", body: `{"name":"launch.py"}`},
		{path: "/api/services/stop", body: `{"name":"launch.py"}`},
		{path: "/api/services/stop-all", body: `{}`},
		{path: "/api/services/autostart", body: `{"name":"launch.py","enabled":true}`},
	} {
		t.Run(tc.path, func(t *testing.T) {
			rr := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodPost, tc.path, strings.NewReader(tc.body))
			h.ServeHTTP(rr, req)
			if rr.Code != http.StatusPreconditionRequired || !strings.Contains(rr.Body.String(), "X-GA-Confirm") {
				t.Fatalf("status=%d want=%d body=%s", rr.Code, http.StatusPreconditionRequired, rr.Body.String())
			}
		})
	}
}

func TestServicesSummaryRejectsNonGET(t *testing.T) {
	h := newServiceHandlerTestServer(t, t.TempDir()).Routes()
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/services/summary", nil)
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusMethodNotAllowed || !strings.Contains(rr.Body.String(), "method not allowed") {
		t.Fatalf("status=%d want=%d body=%s", rr.Code, http.StatusMethodNotAllowed, rr.Body.String())
	}
}

func TestServicesSummaryReturnsCountsOnGET(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "launch.py"), []byte("# test\n"), 0644); err != nil {
		t.Fatal(err)
	}
	h := newServiceHandlerTestServer(t, root).Routes()
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/services/summary", nil)
	h.ServeHTTP(rr, req)
	body := rr.Body.String()
	for _, want := range []string{`"total":1`, `"running":0`, `"stopped":1`} {
		if rr.Code != http.StatusOK || !strings.Contains(body, want) {
			t.Fatalf("status=%d missing %s body=%s", rr.Code, want, body)
		}
	}
}

func TestLogLinesQueryIsBounded(t *testing.T) {
	s := newServiceHandlerTestServer(t, t.TempDir())
	for _, tc := range []struct {
		raw  string
		want int
		ok   bool
	}{
		{raw: "", want: 1, ok: true},
		{raw: "37", want: 37, ok: true},
		{raw: "0"},
		{raw: "5001"},
		{raw: "nope"},
	} {
		req := httptest.NewRequest(http.MethodGet, "/api/logs/launch.py?lines="+tc.raw, nil)
		got, err := s.requestedLogLines(req)
		if tc.ok && (err != nil || got != tc.want) {
			t.Fatalf("lines=%q got=%d err=%v want=%d", tc.raw, got, err, tc.want)
		}
		if !tc.ok && err == nil {
			t.Fatalf("lines=%q unexpectedly accepted as %d", tc.raw, got)
		}
	}
}

func TestAdminFeishuServiceRejectsOriginalFeishuCredentials(t *testing.T) {
	root := t.TempDir()
	content := "fs_app_id = \"original-app\"\nfs_app_secret = \"original-secret\"\n"
	if err := os.WriteFile(filepath.Join(root, "mykey.py"), []byte(content), 0600); err != nil {
		t.Fatal(err)
	}
	s := newServiceHandlerTestServer(t, root)

	err := s.StartChatFeishuBridge()
	if err == nil || !strings.Contains(err.Error(), "configure the Feishu Admin sync channel") {
		t.Fatalf("StartChatFeishuBridge error=%v, want dedicated credential guidance", err)
	}
	if s.IsChatFeishuBridgeRunning() {
		t.Fatal("original fs_* credentials unexpectedly started the Admin sync service")
	}
}

func TestSavingAdminFeishuChannelDoesNotStartService(t *testing.T) {
	root := t.TempDir()
	s := newServiceHandlerTestServer(t, root)
	h := s.Routes()
	body := `{"profiles":[{"id":"feishu_admin","fields":[
		{"name":"feishu_admin_app_id","value":"admin-app"},
		{"name":"feishu_admin_app_secret","value":"admin-secret"},
		{"name":"feishu_admin_allowed_users","value":""},
		{"name":"feishu_admin_public_access","value":"false"}
	]}]}`
	req := httptest.NewRequest(http.MethodPut, "/api/channels", strings.NewReader(body))
	markDangerous(req)
	rec := httptest.NewRecorder()

	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("PUT /api/channels status=%d body=%s", rec.Code, rec.Body.String())
	}
	if s.IsChatFeishuBridgeRunning() || s.Svc.HasRunningProcesses() {
		t.Fatal("saving Feishu Admin credentials unexpectedly started a service")
	}
}

func TestRestartingStoppedAdminFeishuServiceDoesNotStartIt(t *testing.T) {
	root := t.TempDir()
	content := "feishu_admin_app_id = \"admin-app\"\nfeishu_admin_app_secret = \"admin-secret\"\n"
	if err := os.WriteFile(filepath.Join(root, "mykey.py"), []byte(content), 0600); err != nil {
		t.Fatal(err)
	}
	s := newServiceHandlerTestServer(t, root)

	if err := s.RestartChatFeishuBridgeIfRunning(); err != nil {
		t.Fatalf("RestartChatFeishuBridgeIfRunning: %v", err)
	}
	if s.IsChatFeishuBridgeRunning() {
		t.Fatal("restarting a stopped Feishu Admin service unexpectedly started it")
	}
}

func TestAdminFeishuServiceDiscoveryAndAutostartAreIndependent(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "frontends"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "frontends", "fsapp.py"), []byte("# original Feishu channel\n"), 0644); err != nil {
		t.Fatal(err)
	}
	s := newServiceHandlerTestServer(t, root)

	find := func(items []service.ServiceInfo, name string) (service.ServiceInfo, bool) {
		for _, item := range items {
			if item.Name == name {
				return item, true
			}
		}
		return service.ServiceInfo{}, false
	}
	items := s.servicesWithAutostart(s.Svc)
	original, ok := find(items, "frontends/fsapp.py")
	if !ok {
		t.Fatal("original frontends/fsapp.py service was not discovered")
	}
	adminSync, ok := find(items, adminFeishuServiceName)
	if !ok {
		t.Fatalf("dedicated %s service was not discovered", adminFeishuServiceName)
	}
	if original.Autostart || adminSync.Autostart || adminSync.Running {
		t.Fatalf("unexpected initial state: original=%+v admin=%+v", original, adminSync)
	}

	req := httptest.NewRequest(http.MethodPost, "/api/services/autostart", strings.NewReader(
		`{"name":"admin/feishuapp.py","enabled":true}`,
	))
	markDangerous(req)
	rec := httptest.NewRecorder()
	s.Routes().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("enable Admin Feishu autostart status=%d body=%s", rec.Code, rec.Body.String())
	}

	items = s.servicesWithAutostart(s.Svc)
	original, _ = find(items, "frontends/fsapp.py")
	adminSync, _ = find(items, adminFeishuServiceName)
	if original.Autostart || !adminSync.Autostart {
		t.Fatalf("autostart settings were coupled: original=%+v admin=%+v", original, adminSync)
	}
	if adminSync.Running || s.Svc.HasRunningProcesses() {
		t.Fatal("changing autostart unexpectedly started a service immediately")
	}
}

func TestLogStreamStartsWithSnapshot(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "launch.py"), []byte("# test\n"), 0644); err != nil {
		t.Fatal(err)
	}
	s := newServiceHandlerTestServer(t, root)
	ts := httptest.NewServer(s.Routes())
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/api/logs/launch.py/stream?lines=37")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status=%d", resp.StatusCode)
	}
	if got := resp.Header.Get("Content-Type"); !strings.HasPrefix(got, "text/event-stream") {
		t.Fatalf("content-type=%q", got)
	}
	scanner := bufio.NewScanner(resp.Body)
	for _, want := range []string{"event: snapshot", `data: {"lines":[]}`} {
		if !scanner.Scan() {
			t.Fatalf("missing %q: %v", want, scanner.Err())
		}
		if got := scanner.Text(); got != want {
			t.Fatalf("line=%q want=%q", got, want)
		}
	}
}

func TestServicesListRejectsNonGET(t *testing.T) {
	h := newServiceHandlerTestServer(t, t.TempDir()).Routes()
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/services", nil)
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusMethodNotAllowed || !strings.Contains(rr.Body.String(), "method not allowed") {
		t.Fatalf("status=%d want=%d body=%s", rr.Code, http.StatusMethodNotAllowed, rr.Body.String())
	}
}

func TestServicesListExposesProcessContext(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "launch.py"), []byte("# test\n"), 0644); err != nil {
		t.Fatal(err)
	}
	h := newServiceHandlerTestServer(t, root).Routes()
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/services", nil)
	h.ServeHTTP(rr, req)
	body := rr.Body.String()
	for _, want := range []string{`"name":"launch.py"`, `"command":`, `"workdir":"` + strings.ReplaceAll(root, `\`, `\\`) + `"`, `"running":false`} {
		if rr.Code != http.StatusOK || !strings.Contains(body, want) {
			t.Fatalf("status=%d missing %s body=%s", rr.Code, want, body)
		}
	}
}

func TestServiceRoutesEnforceWorkflowAndModelBoundaries(t *testing.T) {
	root := t.TempDir()
	reflectDir := filepath.Join(root, "reflect")
	if err := os.MkdirAll(reflectDir, 0755); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"watchdog.py", "agent_team_worker.py", "checklist_master.py"} {
		if err := os.WriteFile(filepath.Join(reflectDir, name), []byte("# test\n"), 0644); err != nil {
			t.Fatal(err)
		}
	}
	h := newServiceHandlerTestServer(t, root).Routes()
	cases := []struct {
		name string
		path string
		body string
		want int
		text string
	}{
		{name: "watchdog model", path: "/api/services/model", body: `{"name":"reflect/watchdog.py","llm_no":1}`, want: http.StatusBadRequest, text: "does not support model"},
		{name: "worker start", path: "/api/services/start", body: `{"name":"reflect/agent_team_worker.py"}`, want: http.StatusBadRequest, text: "managed by its Goal"},
		{name: "checklist autostart", path: "/api/services/autostart", body: `{"name":"reflect/checklist_master.py","enabled":true}`, want: http.StatusBadRequest, text: "autostart is managed"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rr := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodPost, tc.path, strings.NewReader(tc.body))
			req.Header.Set("X-GA-Confirm", "dangerous")
			h.ServeHTTP(rr, req)
			if rr.Code != tc.want || !strings.Contains(rr.Body.String(), tc.text) {
				t.Fatalf("status=%d want=%d body=%s", rr.Code, tc.want, rr.Body.String())
			}
		})
	}
}

func TestStartAutostartServicesUsesConfiguredServiceStartup(t *testing.T) {
	s := newServiceHandlerTestServer(t, t.TempDir())
	s.CfgStore.UpdateRuntime(func(cfg *config.AppConfig) { cfg.ServiceAutostart = []string{"reflect/scheduler.py", " ", "reflect/autonomous.py"} })
	previous := startAutostartService
	started := []string{}
	startAutostartService = func(_ *Server, name string) error {
		started = append(started, name)
		return nil
	}
	t.Cleanup(func() { startAutostartService = previous })

	s.StartAutostartServices()

	if strings.Join(started, ",") != "reflect/scheduler.py,reflect/autonomous.py" {
		t.Fatalf("autostart services = %v", started)
	}
}
