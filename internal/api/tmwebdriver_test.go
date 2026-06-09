package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"
)

func TestTMWebDriverPythonDependencyPackageNames(t *testing.T) {
	wantModules := []string{"requests", "bottle", "simple_websocket_server"}
	if got := tmwebdriverPythonModules(); !reflect.DeepEqual(got, wantModules) {
		t.Fatalf("modules=%v want=%v", got, wantModules)
	}

	wantPkgs := []string{"requests", "bottle", "simple-websocket-server"}
	if got := tmwebdriverRequiredPipPackages(); !reflect.DeepEqual(got, wantPkgs) {
		t.Fatalf("packages=%v want=%v", got, wantPkgs)
	}
	if got := tmwebdriverPipPackages([]string{"simple_websocket_server"}); !reflect.DeepEqual(got, []string{"simple-websocket-server"}) {
		t.Fatalf("mapped package=%v", got)
	}
}

func TestBuildTMWebDriverInstallCommand(t *testing.T) {
	if got, want := buildTMWebDriverInstallCommand(""), "python -m pip install -i https://pypi.tuna.tsinghua.edu.cn/simple requests bottle simple-websocket-server"; got != want {
		t.Fatalf("command=%q want=%q", got, want)
	}
	if got, want := buildTMWebDriverInstallCommand("C:/GA/.venv/Scripts/python.exe"), "C:/GA/.venv/Scripts/python.exe -m pip install -i https://pypi.tuna.tsinghua.edu.cn/simple requests bottle simple-websocket-server"; got != want {
		t.Fatalf("command=%q want=%q", got, want)
	}
}

func TestBuildGitMirrorArgs(t *testing.T) {
	mirror := "https://gh-proxy.com/https://github.com/"
	if got, want := buildGitMirrorArgs(true, mirror), []string{"config", "--global", "url." + mirror + ".insteadOf", "https://github.com/"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("enable args=%v want=%v", got, want)
	}
	if got, want := buildGitMirrorArgs(false, mirror), []string{"config", "--global", "--unset-all", "url." + mirror + ".insteadOf"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("disable args=%v want=%v", got, want)
	}
}

func TestValidateGitMirrorPrefixRejectsUnsafeURLParts(t *testing.T) {
	valid := []string{
		"https://gh-proxy.com/https://github.com/",
		"http://mirror.example/github/",
	}
	for _, mirror := range valid {
		if err := validateGitMirrorPrefix(mirror); err != nil {
			t.Fatalf("valid mirror %q error=%v", mirror, err)
		}
	}

	cases := []struct {
		name   string
		mirror string
		want   string
	}{
		{"relative", "gh-proxy.com/https://github.com/", "absolute http(s) URL"},
		{"ftp", "ftp://mirror.example/github/", "scheme must be http or https"},
		{"empty host", "https:///github/", "host is required"},
		{"userinfo", "https://user:pass@mirror.example/github/", "must not include userinfo"},
		{"query", "https://mirror.example/github/?token=secret", "must not include userinfo"},
		{"fragment", "https://mirror.example/github/#frag", "must not include userinfo"},
		{"newline", "https://mirror.example/github/\n--bad", "control characters"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := validateGitMirrorPrefix(tc.mirror)
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("error=%v want contains %q", err, tc.want)
			}
		})
	}
}

func TestTMWebDriverStatusContract(t *testing.T) {
	h := newGoalTestServer(t, t.TempDir()).Routes()
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/tmwebdriver/status", nil)
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("status=%d want=%d body=%s", rr.Code, http.StatusOK, rr.Body.String())
	}

	var body tmwebdriverStatusResponse
	if err := json.Unmarshal(rr.Body.Bytes(), &body); err != nil {
		t.Fatalf("invalid json: %v body=%s", err, rr.Body.String())
	}
	if strings.TrimSpace(body.InstallCommand) == "" || !strings.Contains(body.InstallCommand, "pip install") {
		t.Fatalf("missing install command: %+v", body)
	}
	if strings.TrimSpace(body.CheckedAt) == "" {
		t.Fatalf("missing checked_at: %+v", body)
	}
	wantChecks := []string{"browser_process", "python_dependencies", "ws_master_port", "chrome_extension"}
	if len(body.Checks) != len(wantChecks) {
		t.Fatalf("checks=%v want names=%v", body.Checks, wantChecks)
	}
	for i, want := range wantChecks {
		if body.Checks[i].Name != want || strings.TrimSpace(body.Checks[i].Detail) == "" {
			t.Fatalf("check[%d]=%+v want name=%q with detail", i, body.Checks[i], want)
		}
	}
	if !body.OK && strings.TrimSpace(body.Recommendation) == "" {
		t.Fatalf("missing recommendation for not-ok status: %+v", body)
	}
}

func TestTMWebDriverDangerousRoutesRequireConfirm(t *testing.T) {
	h := newGoalTestServer(t, t.TempDir()).Routes()
	for _, path := range []string{"/api/tmwebdriver/repair", "/api/tmwebdriver/install-deps"} {
		t.Run(path, func(t *testing.T) {
			rr := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodPost, path, strings.NewReader(`{}`))
			req.Header.Set("Content-Type", "application/json")
			h.ServeHTTP(rr, req)
			if rr.Code != http.StatusPreconditionRequired || !strings.Contains(rr.Body.String(), "X-GA-Confirm") {
				t.Fatalf("status=%d want=%d body=%s", rr.Code, http.StatusPreconditionRequired, rr.Body.String())
			}
		})
	}
}
