package api

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"genericagent-admin-go/internal/config"
	"genericagent-admin-go/internal/modelconfig"
	"genericagent-admin-go/internal/service"
)

func newConfigTestServer(t *testing.T) *Server {
	t.Helper()
	cfg := config.NewStore(t.TempDir())
	models := modelconfig.NewStore(t.TempDir())
	return New(cfg, service.NewManager(cfg.Cfg.GARoot, cfg.Cfg.BufferLines), models, nil)
}

func TestConfigSaveValidationAndDefaults(t *testing.T) {
	s := newConfigTestServer(t)
	root := t.TempDir()
	py := filepath.Join(root, "python.exe")
	if err := os.WriteFile(py, []byte("stub"), 0755); err != nil {
		t.Fatal(err)
	}
	payload := config.AppConfig{GARoot: root, PythonPath: py, ProxyMode: "custom", HTTPProxy: "http://127.0.0.1:7890"}
	body, _ := json.Marshal(payload)
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPut, "/api/config", bytes.NewReader(body))
	markDangerous(req)
	s.Routes().ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
	}
	var got config.AppConfig
	if err := json.Unmarshal(rr.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if got.ChatDataDir == "" || !strings.Contains(got.ChatDataDir, "GenericAgent-Admin") {
		t.Fatalf("chat_data_dir default not applied: %q", got.ChatDataDir)
	}
}

func TestConfigSaveRejectsInvalidPathsAndProxy(t *testing.T) {
	cases := []struct {
		name    string
		cfg     config.AppConfig
		wantErr string
	}{
		{"missing root", config.AppConfig{GARoot: filepath.Join(t.TempDir(), "missing")}, "ga_root does not exist"},
		{"bad port", config.AppConfig{GARoot: t.TempDir(), Port: 70000}, "port must be between 0 and 65535"},
		{"bad python", config.AppConfig{GARoot: t.TempDir(), PythonPath: filepath.Join(t.TempDir(), "python.exe")}, "python_path does not exist"},
		{"bad proxy mode", config.AppConfig{GARoot: t.TempDir(), ProxyMode: "pac"}, "proxy_mode"},
		{"bad proxy url", config.AppConfig{GARoot: t.TempDir(), ProxyMode: "custom", HTTPProxy: "127.0.0.1:7890"}, "http_proxy"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			s := newConfigTestServer(t)
			body, _ := json.Marshal(tc.cfg)
			rr := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodPut, "/api/config", bytes.NewReader(body))
			markDangerous(req)
			s.Routes().ServeHTTP(rr, req)
			if rr.Code != http.StatusBadRequest {
				t.Fatalf("status=%d want=400 body=%s", rr.Code, rr.Body.String())
			}
			if !strings.Contains(rr.Body.String(), tc.wantErr) {
				t.Fatalf("body %q does not contain %q", rr.Body.String(), tc.wantErr)
			}
		})
	}
}

func TestSetupValidateDryRunDoesNotPersistInvalidRoot(t *testing.T) {
	s := newConfigTestServer(t)
	before := s.CfgStore.Cfg.GARoot
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/setup/validate", strings.NewReader(`{"path":"`+filepath.ToSlash(t.TempDir())+`"}`))
	markDangerous(req)
	s.Routes().ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
	}
	if !strings.Contains(rr.Body.String(), `"ok":false`) {
		t.Fatalf("expected unhealthy response: %s", rr.Body.String())
	}
	if s.CfgStore.Cfg.GARoot != before {
		t.Fatalf("invalid dry-run persisted root: %q -> %q", before, s.CfgStore.Cfg.GARoot)
	}
}

func TestSetupEnvReportsOptionalUvAndNpm(t *testing.T) {
	s := newConfigTestServer(t)
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/setup/env", nil)
	s.Routes().ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
	}
	body := rr.Body.String()
	for _, name := range []string{`"name":"git"`, `"name":"python"`, `"name":"uv"`, `"name":"npm"`} {
		if !strings.Contains(body, name) {
			t.Fatalf("setup env missing %s in %s", name, body)
		}
	}
}

func TestSetupBrowseRejectsMalformedJSON(t *testing.T) {
	s := newConfigTestServer(t)
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/setup/browse", strings.NewReader(`not-json`))

	s.Routes().ServeHTTP(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("status=%d want=400 body=%s", rr.Code, rr.Body.String())
	}
}

func TestUnsafeSetupPath(t *testing.T) {
	for _, p := range []string{"", " ", ".", string(filepath.Separator)} {
		if !unsafeSetupPath(p) {
			t.Fatalf("unsafeSetupPath(%q)=false, want true", p)
		}
	}
	if runtime.GOOS == "windows" {
		for _, p := range []string{"C:", `C:\`, "C:/"} {
			if !unsafeSetupPath(p) {
				t.Fatalf("unsafeSetupPath(%q)=false, want true", p)
			}
		}
	}
	if unsafeSetupPath(filepath.Join(t.TempDir(), "GenericAgent")) {
		t.Fatalf("expected nested install path to be safe")
	}
}

func TestSetupInstallRejectsFilesystemRoot(t *testing.T) {
	s := newConfigTestServer(t)
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/setup/install", strings.NewReader(`{"path":"`+filepath.ToSlash(string(filepath.Separator))+`"}`))
	markDangerous(req)

	s.Routes().ServeHTTP(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("status=%d want=400 body=%s", rr.Code, rr.Body.String())
	}
	if !strings.Contains(rr.Body.String(), "refusing to install GenericAgent under filesystem root") {
		t.Fatalf("unexpected body: %s", rr.Body.String())
	}
}

func TestSetupInstallUsesCancelableCloneContext(t *testing.T) {
	s := newConfigTestServer(t)
	oldRunClone := runSetupCloneFunc
	t.Cleanup(func() { runSetupCloneFunc = oldRunClone })
	called := false
	runSetupCloneFunc = func(ctx context.Context, dest string) (string, error) {
		called = true
		if _, ok := ctx.Deadline(); !ok {
			t.Fatalf("clone context has no timeout deadline")
		}
		<-ctx.Done()
		return "clone stopped", ctx.Err()
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	installDir := t.TempDir()
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/setup/install", strings.NewReader(`{"path":"`+filepath.ToSlash(installDir)+`"}`)).WithContext(ctx)
	markDangerous(req)

	s.Routes().ServeHTTP(rr, req)

	if !called {
		t.Fatalf("clone hook was not called")
	}
	if rr.Code != http.StatusInternalServerError {
		t.Fatalf("status=%d want=500 body=%s", rr.Code, rr.Body.String())
	}
	body := rr.Body.String()
	if !strings.Contains(body, "clone stopped") || !strings.Contains(body, "context canceled") {
		t.Fatalf("unexpected body: %s", body)
	}
}

func TestSetupInstallClonesGenericAgentUnderInstallDirectory(t *testing.T) {
	s := newConfigTestServer(t)
	oldRunClone := runSetupCloneFunc
	oldZip := downloadAndExtractGenericAgentArchive
	t.Cleanup(func() {
		runSetupCloneFunc = oldRunClone
		downloadAndExtractGenericAgentArchive = oldZip
	})

	installDir := t.TempDir()
	wantRoot := filepath.Join(installDir, "GenericAgent")
	downloadAndExtractGenericAgentArchive = func(ctx context.Context, gotDest string) (string, error) {
		t.Fatalf("zip fallback should not be called for successful clone")
		return "", nil
	}
	runSetupCloneFunc = func(ctx context.Context, gotDest string) (string, error) {
		if gotDest != wantRoot {
			t.Fatalf("clone dest=%q want %q", gotDest, wantRoot)
		}
		if err := os.MkdirAll(filepath.Join(gotDest, "assets"), 0755); err != nil {
			return "", err
		}
		if err := os.WriteFile(filepath.Join(gotDest, "agentmain.py"), []byte("print('ok')\n"), 0644); err != nil {
			return "", err
		}
		if err := os.WriteFile(filepath.Join(gotDest, "llmcore.py"), []byte("# ok\n"), 0644); err != nil {
			return "", err
		}
		if err := os.WriteFile(filepath.Join(gotDest, "assets", "tools_schema.json"), []byte("[]\n"), 0644); err != nil {
			return "", err
		}
		return "clone completed", nil
	}

	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/setup/install", strings.NewReader(`{"path":"`+filepath.ToSlash(installDir)+`"}`))
	markDangerous(req)
	s.Routes().ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
	}
	if s.CfgStore.Cfg.GARoot != wantRoot {
		t.Fatalf("ga_root=%q want %q", s.CfgStore.Cfg.GARoot, wantRoot)
	}
	var got map[string]interface{}
	if err := json.Unmarshal(rr.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if got["root"] != wantRoot || got["install_dir"] != installDir {
		t.Fatalf("unexpected response: %#v", got)
	}
}

func TestGaGitStatusRejectsNonGET(t *testing.T) {
	s := newConfigTestServer(t)
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/ga/git-status", nil)
	s.Routes().ServeHTTP(rr, req)
	if rr.Code != http.StatusMethodNotAllowed {
		t.Fatalf("status=%d want=405 body=%s", rr.Code, rr.Body.String())
	}
}

func TestGaGitStatusRejectsMalformedAheadBehind(t *testing.T) {
	root := t.TempDir()
	if err := os.Mkdir(filepath.Join(root, ".git"), 0755); err != nil {
		t.Fatal(err)
	}
	oldRunGit := runGitCommandFunc
	t.Cleanup(func() { runGitCommandFunc = oldRunGit })
	runGitCommandFunc = func(ctx context.Context, root string, args ...string) (string, error) {
		joined := strings.Join(args, " ")
		switch joined {
		case "branch --show-current":
			return "main", nil
		case "rev-parse --short HEAD":
			return "abc1234", nil
		case "status --short":
			return "", nil
		case "rev-parse --abbrev-ref --symbolic-full-name @{u}":
			return "origin/main", nil
		case "rev-list --left-right --count HEAD...@{u}":
			return "not-a-number 2", nil
		default:
			t.Fatalf("unexpected git command: %s", joined)
			return "", nil
		}
	}

	_, err := gaGitStatusForRoot(context.Background(), root)
	if err == nil || !strings.Contains(err.Error(), "invalid git ahead count") {
		t.Fatalf("expected invalid ahead count error, got %v", err)
	}
}

func TestInstallGenericAgentSourceFallsBackToZipWhenGitCloneFails(t *testing.T) {
	oldClone := runSetupCloneFunc
	oldZip := downloadAndExtractGenericAgentArchive
	t.Cleanup(func() {
		runSetupCloneFunc = oldClone
		downloadAndExtractGenericAgentArchive = oldZip
	})

	dest := filepath.Join(t.TempDir(), "GenericAgent")
	zipCalled := false
	runSetupCloneFunc = func(ctx context.Context, gotDest string) (string, error) {
		if gotDest != dest {
			t.Fatalf("clone dest=%q want=%q", gotDest, dest)
		}
		return "git is not installed", os.ErrNotExist
	}
	downloadAndExtractGenericAgentArchive = func(ctx context.Context, gotDest string) (string, error) {
		zipCalled = true
		if gotDest != dest {
			t.Fatalf("zip dest=%q want=%q", gotDest, dest)
		}
		if err := os.MkdirAll(gotDest, 0755); err != nil {
			return "", err
		}
		if err := os.WriteFile(filepath.Join(gotDest, "agentmain.py"), []byte("print('ok')\n"), 0644); err != nil {
			return "", err
		}
		return "zip archive extracted", nil
	}

	method, out, err := installGenericAgentSource(context.Background(), dest)
	if err != nil {
		t.Fatalf("installGenericAgentSource error: %v", err)
	}
	if !zipCalled {
		t.Fatal("expected archive fallback to be called")
	}
	if method != "zip" {
		t.Fatalf("method=%q want zip", method)
	}
	if !strings.Contains(out, "git is not installed") || !strings.Contains(out, "zip archive extracted") {
		t.Fatalf("expected combined fallback output, got %q", out)
	}
}

func TestSetupPythonInstallPersistsDiscoveredPython(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("python installer endpoint is Windows-only")
	}
	s := newConfigTestServer(t)
	installerPython := filepath.Join(t.TempDir(), "Python312", "python.exe")
	if err := os.MkdirAll(filepath.Dir(installerPython), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(installerPython, []byte("stub"), 0755); err != nil {
		t.Fatal(err)
	}

	oldRunInstaller := runPythonInstallerFunc
	t.Cleanup(func() {
		runPythonInstallerFunc = oldRunInstaller
	})
	runPythonInstallerFunc = func(ctx context.Context) (string, string, error) {
		return installerPython, "installer completed", nil
	}

	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/setup/python/install", strings.NewReader(`{}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-GA-Confirm", "dangerous")
	s.Routes().ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
	}
	if s.CfgStore.Cfg.PythonPath != installerPython {
		t.Fatalf("python_path=%q want %q", s.CfgStore.Cfg.PythonPath, installerPython)
	}
	var got map[string]interface{}
	if err := json.Unmarshal(rr.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if got["python"] != installerPython || got["output"] != "installer completed" {
		t.Fatalf("unexpected response: %#v", got)
	}
}

func TestSetupStateIsReadOnlyAndDoesNotRequireDangerousConfirm(t *testing.T) {
	s := newConfigTestServer(t)
	s.CfgStore.Cfg.GARoot = t.TempDir()
	s.CfgStore.Cfg.EffectivePython = filepath.Join(s.CfgStore.Cfg.GARoot, ".venv", "Scripts", "python.exe")

	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/setup/state", nil)
	s.Routes().ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
	}
	var got map[string]interface{}
	if err := json.Unmarshal(rr.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if got["ok"] != true || got["ga_root"] != s.CfgStore.Cfg.GARoot || got["python"] != s.CfgStore.Cfg.EffectivePython {
		t.Fatalf("unexpected setup state: %#v", got)
	}
}

func TestSetupMutationsRequireDangerousConfirm(t *testing.T) {
	s := newConfigTestServer(t)
	for _, tc := range []struct {
		path string
		body string
	}{
		{"/api/setup/validate", `{"path":"` + strings.ReplaceAll(t.TempDir(), `\\`, `\\\\`) + `"}`},
		{"/api/setup/install", `{}`},
		{"/api/setup/python/install", `{}`},
		{"/api/setup/complete", `{}`},
		{"/api/setup/venv/create", `{}`},
		{"/api/setup/deps/install", `{}`},
		{"/api/setup/smoke", `{}`},
	} {
		t.Run(tc.path, func(t *testing.T) {
			rr := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodPost, tc.path, strings.NewReader(tc.body))
			req.Header.Set("Content-Type", "application/json")
			s.Routes().ServeHTTP(rr, req)
			if rr.Code != http.StatusPreconditionRequired {
				t.Fatalf("status=%d want=%d body=%s", rr.Code, http.StatusPreconditionRequired, rr.Body.String())
			}
		})
	}
}
