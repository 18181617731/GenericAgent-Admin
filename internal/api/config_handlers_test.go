package api

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
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
	return New(cfg, service.NewManager(cfg.Snapshot().GARoot, cfg.Snapshot().BufferLines), models, nil)
}

// The setup endpoint must execute the candidate rather than trusting its name.
// Reusing the test binary keeps this contract test independent of whatever
// Python installation happens to be present on the test machine.
func TestMain(m *testing.M) {
	if len(os.Args) > 1 && os.Args[1] == "--version" {
		if os.Getenv("GA_TEST_PYTHON_VERSION_FAIL") == "1" {
			_, _ = os.Stderr.WriteString("synthetic Python version failure\n")
			os.Exit(7)
		}
		_, _ = os.Stdout.WriteString("Python 3.11.9\n")
		os.Exit(0)
	}
	os.Exit(m.Run())
}

func TestSetupPythonValidateRejectsInvalidExecutable(t *testing.T) {
	currentExe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	missing := filepath.Join(t.TempDir(), "missing-python")
	cases := []struct {
		name        string
		path        string
		failVersion bool
	}{
		{name: "empty path"},
		{name: "missing path", path: missing},
		{name: "directory", path: t.TempDir()},
		{name: "version command fails", path: currentExe, failVersion: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			s := newConfigTestServer(t)
			baseline, err := filepath.Abs(currentExe)
			if err != nil {
				t.Fatal(err)
			}
			baseline = filepath.Clean(baseline)
			cfg := s.CfgStore.Snapshot()
			cfg.PythonPath = baseline
			if err := s.CfgStore.Save(cfg); err != nil {
				t.Fatal(err)
			}
			if tc.failVersion {
				t.Setenv("GA_TEST_PYTHON_VERSION_FAIL", "1")
			}
			body, err := json.Marshal(map[string]string{"path": tc.path})
			if err != nil {
				t.Fatal(err)
			}
			rr := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodPost, "/api/setup/python/validate", bytes.NewReader(body))
			markDangerous(req)

			s.Routes().ServeHTTP(rr, req)

			if rr.Code != http.StatusBadRequest {
				t.Fatalf("status=%d want=400 body=%s", rr.Code, rr.Body.String())
			}
			if got := s.CfgStore.Snapshot().PythonPath; got != baseline {
				t.Fatalf("PythonPath changed after rejected candidate: %q", got)
			}
		})
	}
}

func TestSetupPythonValidateSavesWorkingExecutable(t *testing.T) {
	t.Setenv("GA_TEST_PYTHON_VERSION_FAIL", "")
	currentExe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	expected, err := filepath.Abs(currentExe)
	if err != nil {
		t.Fatal(err)
	}
	expected = filepath.Clean(expected)
	s := newConfigTestServer(t)
	body, err := json.Marshal(map[string]string{"path": currentExe})
	if err != nil {
		t.Fatal(err)
	}
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/setup/python/validate", bytes.NewReader(body))
	markDangerous(req)

	s.Routes().ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status=%d want=200 body=%s", rr.Code, rr.Body.String())
	}
	var response struct {
		OK      bool   `json:"ok"`
		Python  string `json:"python"`
		Version string `json:"version"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &response); err != nil {
		t.Fatal(err)
	}
	if !response.OK || response.Python != expected || response.Version != "Python 3.11.9" {
		t.Fatalf("unexpected response: %+v", response)
	}
	saved := s.CfgStore.Snapshot()
	if saved.PythonPath != expected {
		t.Fatalf("PythonPath=%q want=%q", saved.PythonPath, expected)
	}
	if saved.EffectivePython != expected {
		t.Fatalf("EffectivePython=%q want=%q", saved.EffectivePython, expected)
	}
}

func TestInstanceDeleteRejectsProtectedDefaultAfterDefaultSwitch(t *testing.T) {
	s := newConfigTestServer(t)
	defaultRoot := t.TempDir()
	secondaryRoot := t.TempDir()
	cfg := s.CfgStore.Snapshot()
	cfg.Instances = []config.InstanceConfig{
		{ID: protectedDefaultInstanceID, Name: "Default", GARoot: defaultRoot},
		{ID: "secondary", Name: "Secondary", GARoot: secondaryRoot},
	}
	cfg.DefaultInstanceID = "secondary"
	cfg.GARoot = secondaryRoot
	if err := s.CfgStore.Save(cfg); err != nil {
		t.Fatalf("save fixture: %v", err)
	}

	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodDelete, "/api/instances/delete", strings.NewReader(`{"id":"default"}`))
	markDangerous(req)
	s.Routes().ServeHTTP(rr, req)
	if rr.Code != http.StatusConflict {
		t.Fatalf("status=%d want=%d body=%s", rr.Code, http.StatusConflict, rr.Body.String())
	}
	if !strings.Contains(rr.Body.String(), "default instance cannot be deleted") {
		t.Fatalf("body=%q does not explain protected default", rr.Body.String())
	}
	got := s.CfgStore.Snapshot()
	if _, ok := got.Instance(protectedDefaultInstanceID); !ok {
		t.Fatal("protected default instance was deleted")
	}
	if got.DefaultInstanceID != "secondary" {
		t.Fatalf("default_instance_id=%q want secondary", got.DefaultInstanceID)
	}
}

func TestConfigSaveValidationAndDefaults(t *testing.T) {
	s := newConfigTestServer(t)
	root := t.TempDir()
	py := filepath.Join(root, "python.exe")
	if err := os.WriteFile(py, []byte("stub"), 0755); err != nil {
		t.Fatal(err)
	}
	payload := config.AppConfig{GARoot: root, PythonPath: py, ProxyMode: "custom", HTTPProxy: "http://127.0.0.1:7890", GitHubMirror: "https://mirror.example"}
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
	if got.GitHubMirror != "https://mirror.example" {
		t.Fatalf("github_mirror = %q, want persisted mirror", got.GitHubMirror)
	}
	wantChatDataDir := filepath.Join(s.CfgStore.Root, "data")
	if got.ChatDataDir != wantChatDataDir {
		t.Fatalf("chat_data_dir = %q, want %q", got.ChatDataDir, wantChatDataDir)
	}
}

func TestConfigSavePreservesUIThemeWhenOmitted(t *testing.T) {
	s := newConfigTestServer(t)
	root := t.TempDir()
	py := filepath.Join(root, "python.exe")
	if err := os.WriteFile(py, []byte("stub"), 0755); err != nil {
		t.Fatal(err)
	}
	cfg := s.CfgStore.Snapshot()
	cfg.UITheme = "dark"
	if err := s.CfgStore.Save(cfg); err != nil {
		t.Fatal(err)
	}

	payload := config.AppConfig{GARoot: root, PythonPath: py, ProxyMode: "off"}
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
	if got.UITheme != "dark" {
		t.Fatalf("ui_theme=%q want dark", got.UITheme)
	}
	if snap := s.CfgStore.Snapshot().UITheme; snap != "dark" {
		t.Fatalf("stored ui_theme=%q want dark", snap)
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
	before := s.CfgStore.Snapshot().GARoot
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
	if s.CfgStore.Snapshot().GARoot != before {
		t.Fatalf("invalid dry-run persisted root: %q -> %q", before, s.CfgStore.Snapshot().GARoot)
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
	if s.CfgStore.Snapshot().GARoot != wantRoot {
		t.Fatalf("ga_root=%q want %q", s.CfgStore.Snapshot().GARoot, wantRoot)
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

// The console hides its GA source card when git cannot answer, so these cases
// must be a normal 200 with available=false rather than an error the UI has to
// guess about.
func TestGaGitStatusReportsUnavailable(t *testing.T) {
	repo := t.TempDir()
	if err := os.Mkdir(filepath.Join(repo, ".git"), 0755); err != nil {
		t.Fatal(err)
	}
	cases := []struct {
		name    string
		root    string
		gitPath func() (string, error)
		reason  string
	}{
		{name: "no root", root: "", gitPath: func() (string, error) { return "git", nil }, reason: gitUnavailableNoRoot},
		{name: "git missing", root: repo, gitPath: func() (string, error) { return "", errors.New("not found") }, reason: gitUnavailableNoGit},
		{name: "not a repo", root: t.TempDir(), gitPath: func() (string, error) { return "git", nil }, reason: gitUnavailableNoRepo},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			s := newConfigTestServer(t)
			updateTestConfig(t, s.CfgStore, func(cfg *config.AppConfig) { cfg.GARoot = tc.root })
			oldLook := lookGitPathFunc
			t.Cleanup(func() { lookGitPathFunc = oldLook })
			lookGitPathFunc = tc.gitPath
			oldRunGit := runGitCommandFunc
			t.Cleanup(func() { runGitCommandFunc = oldRunGit })
			runGitCommandFunc = func(ctx context.Context, root string, args ...string) (string, error) {
				t.Fatalf("git must not run when unavailable: %s", strings.Join(args, " "))
				return "", nil
			}

			rr := httptest.NewRecorder()
			s.Routes().ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/api/ga/git-status", nil))
			if rr.Code != http.StatusOK {
				t.Fatalf("status=%d want=200 body=%s", rr.Code, rr.Body.String())
			}
			var got map[string]interface{}
			if err := json.Unmarshal(rr.Body.Bytes(), &got); err != nil {
				t.Fatal(err)
			}
			if got["available"] != false || got["reason"] != tc.reason {
				t.Fatalf("unexpected payload: %#v", got)
			}
		})
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

func TestGaGitStatusHandlesMissingUpstream(t *testing.T) {
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
			return "fatal: no upstream configured for branch 'main'", errors.New("exit status 128")
		case "rev-list --left-right --count HEAD...@{u}":
			t.Fatal("rev-list must not run without an upstream branch")
			return "", nil
		default:
			t.Fatalf("unexpected git command: %s", joined)
			return "", nil
		}
	}

	status, err := gaGitStatusForRoot(context.Background(), root)
	if err != nil {
		t.Fatalf("gaGitStatusForRoot error: %v", err)
	}
	if status["upstream"] != "" || status["upstream_configured"] != false {
		t.Fatalf("unexpected upstream status: %#v", status)
	}
	if status["latest"] != false {
		t.Fatalf("repository without upstream must not be reported as latest: %#v", status)
	}
}

// Updating the GA checkout is GA's own job (the /update chat command), so the
// Admin server must not expose a git pull of its own.
func TestGaGitUpdateEndpointIsNotServed(t *testing.T) {
	s := newConfigTestServer(t)
	root := t.TempDir()
	if err := os.Mkdir(filepath.Join(root, ".git"), 0755); err != nil {
		t.Fatal(err)
	}
	updateTestConfig(t, s.CfgStore, func(cfg *config.AppConfig) {
		cfg.GARoot = root
	})

	oldRunGit := runGitCommandFunc
	t.Cleanup(func() { runGitCommandFunc = oldRunGit })
	runGitCommandFunc = func(ctx context.Context, root string, args ...string) (string, error) {
		t.Fatalf("the admin server must not run git for GA updates: %s", strings.Join(args, " "))
		return "", nil
	}

	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/ga/git-update", strings.NewReader(`{}`))
	markDangerous(req)
	s.Routes().ServeHTTP(rr, req)

	// Unregistered API paths fall through to the static handler, which rejects
	// anything but GET, so an absent route answers 404 or 405 - never 200.
	if rr.Code != http.StatusNotFound && rr.Code != http.StatusMethodNotAllowed {
		t.Fatalf("status=%d want=404 or 405 body=%s", rr.Code, rr.Body.String())
	}
	for _, entry := range riskCatalogItems {
		if entry.Path == "/api/ga/git-update" {
			t.Fatal("risk catalog still advertises /api/ga/git-update")
		}
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
	if s.CfgStore.Snapshot().PythonPath != installerPython {
		t.Fatalf("python_path=%q want %q", s.CfgStore.Snapshot().PythonPath, installerPython)
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
	gaRoot := t.TempDir()
	python := filepath.Join(gaRoot, ".venv", "Scripts", "python.exe")
	if err := os.MkdirAll(filepath.Dir(python), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(python, []byte("stub"), 0755); err != nil {
		t.Fatal(err)
	}
	updateTestConfig(t, s.CfgStore, func(cfg *config.AppConfig) {
		cfg.GARoot = gaRoot
		cfg.EffectivePython = python
	})

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
	if got["ok"] != true || got["ga_root"] != s.CfgStore.Snapshot().GARoot || got["python"] != s.CfgStore.Snapshot().EffectivePython {
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

func TestSlashCommandsIncludesGAPaletteEntries(t *testing.T) {
	s := newConfigTestServer(t)
	root := t.TempDir()
	frontends := filepath.Join(root, "frontends")
	if err := os.MkdirAll(frontends, 0755); err != nil {
		t.Fatal(err)
	}
	slashFile := filepath.Join(frontends, "slash_cmds.py")
	content := `PALETTE_ENTRIES = [
    ("/scheduler", "", "多选启动/停止 reflect 任务"),
    ("/resume", "", "列出最近会话并恢复其中一个"),
    ("/goal", "[goal]", "进入 Goal 模式"),
]
`
	if err := os.WriteFile(slashFile, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
	projectsDir := filepath.Join(root, "temp", "projects")
	for _, name := range []string{"zeta", "alpha"} {
		if err := os.MkdirAll(filepath.Join(projectsDir, name), 0755); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(projectsDir, "not-a-project.txt"), []byte("ignored"), 0644); err != nil {
		t.Fatal(err)
	}
	cfg := s.CfgStore.Snapshot()
	cfg.GARoot = root
	cfg.EffectivePython = ""
	if err := s.CfgStore.Save(cfg); err != nil {
		t.Fatal(err)
	}
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/slash-commands", nil)
	s.Routes().ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
	}
	var got struct {
		Commands []slashCommandItem `json:"commands"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	seen := map[string]slashCommandItem{}
	semanticCounts := map[string]int{}
	for _, c := range got.Commands {
		seen[c.Cmd] = c
		semanticKey := strings.ToLower(strings.TrimSpace(c.Key)) + "\x00" + strings.ToLower(c.Insert)
		semanticCounts[semanticKey]++
	}
	for semanticKey, count := range semanticCounts {
		if count > 1 {
			t.Fatalf("duplicate slash command semantics %q: count=%d commands=%#v", semanticKey, count, got.Commands)
		}
	}
	if seen["/scheduler [status|run]"].Source != "admin" || seen["/resume [args]"].Source != "admin" {
		t.Fatalf("admin command variants should win semantic duplicates: %#v", seen)
	}
	if _, ok := seen["/scheduler"]; ok {
		t.Fatalf("GA scheduler duplicate should be removed: %#v", seen["/scheduler"])
	}
	if _, ok := seen["/resume"]; ok {
		t.Fatalf("GA resume duplicate should be removed: %#v", seen["/resume"])
	}
	if seen["/goal [goal]"].Insert != "/goal " || seen["/goal [goal]"].Key != "/goal" {
		t.Fatalf("goal metadata not normalized: %#v", seen["/goal [goal]"])
	}
	if _, ok := seen["/review <自然语言请求>"]; !ok {
		t.Fatalf("admin built-in review command missing")
	}
	if btw, ok := seen["/btw <问题>"]; !ok || btw.Key != "/btw" || btw.Insert != "/btw " {
		t.Fatalf("admin built-in btw command missing or malformed: %#v", btw)
	}
	if worldline, ok := seen["/worldline"]; !ok || worldline.Source != "admin" || worldline.Insert != "/worldline" {
		t.Fatalf("admin built-in worldline list command missing or malformed: %#v", worldline)
	}
	if restore, ok := seen["/worldline restore <节点> [both|conversation|code] [at|before]"]; !ok || restore.Key != "/worldline restore" || restore.Insert != "/worldline restore " {
		t.Fatalf("admin built-in worldline restore command missing or malformed: %#v", restore)
	}
	if _, ok := seen["/continue"]; !ok {
		t.Fatalf("bare continue variant missing")
	}
	if _, ok := seen["/continue <编号>"]; !ok {
		t.Fatalf("parameterized continue variant missing")
	}
	alpha, ok := seen["/project alpha"]
	if !ok || alpha.Insert != "/project alpha" || alpha.Key != "/project alpha" {
		t.Fatalf("project alpha candidate missing or malformed: %#v", alpha)
	}
	zeta, ok := seen["/project zeta"]
	if !ok || zeta.Insert != "/project zeta" || zeta.Key != "/project zeta" {
		t.Fatalf("project zeta candidate missing or malformed: %#v", zeta)
	}
	if _, ok := seen["/project not-a-project.txt"]; ok {
		t.Fatalf("regular files must not become project candidates")
	}
	alphaIndex, zetaIndex := -1, -1
	for i, c := range got.Commands {
		switch c.Cmd {
		case "/project alpha":
			alphaIndex = i
		case "/project zeta":
			zetaIndex = i
		}
	}
	if alphaIndex < 0 || zetaIndex < 0 || alphaIndex > zetaIndex {
		t.Fatalf("project candidates should be present in sorted source order: alpha=%d zeta=%d", alphaIndex, zetaIndex)
	}
}
