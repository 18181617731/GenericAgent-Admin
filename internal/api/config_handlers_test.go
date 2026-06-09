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
	payload := config.AppConfig{GARoot: root, PythonPath: py, ProxyMode: "custom", HTTPProxy: "http://127.0.0.1:7890", DesktopPetDisabled: true}
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
	if !got.DesktopPetDisabled {
		t.Fatalf("desktop_pet_disabled was not preserved")
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
	if !strings.Contains(rr.Body.String(), "refusing to install GenericAgent into filesystem root") {
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
	dest := filepath.Join(t.TempDir(), "GenericAgent")
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/setup/install", strings.NewReader(`{"path":"`+filepath.ToSlash(dest)+`"}`)).WithContext(ctx)
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
