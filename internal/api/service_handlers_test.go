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
	cfg := &config.Store{Root: t.TempDir(), Cfg: config.Default()}
	cfg.Cfg.GARoot = gaRoot
	cfg.Cfg.LogTailLines = 1
	models := modelconfig.NewStore(t.TempDir())
	return New(cfg, service.NewManager(cfg.Cfg.GARoot, cfg.Cfg.BufferLines), models, nil)
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
