package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func serveJSONAnyContract(t *testing.T, h http.Handler, method, target, body string) (int, any, string) {
	t.Helper()
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(method, target, strings.NewReader(body))
	h.ServeHTTP(rr, req)
	raw := rr.Body.String()
	var decoded any
	if strings.TrimSpace(raw) != "" {
		if err := json.Unmarshal(rr.Body.Bytes(), &decoded); err != nil {
			t.Fatalf("%s %s returned non-JSON body: status=%d body=%s err=%v", method, target, rr.Code, raw, err)
		}
	}
	return rr.Code, decoded, raw
}

func requireJSONObjectContract(t *testing.T, v any, raw string) map[string]any {
	t.Helper()
	obj, ok := v.(map[string]any)
	if !ok || obj == nil {
		t.Fatalf("response is %T, want non-null JSON object body=%s", v, raw)
	}
	return obj
}

func requireJSONArrayContract(t *testing.T, v any, raw string) []any {
	t.Helper()
	arr, ok := v.([]any)
	if !ok || arr == nil {
		t.Fatalf("response is %T, want non-null JSON array body=%s", v, raw)
	}
	return arr
}

func TestNonChatStatusEndpointsReturnStableJSONObjects(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "launch.py"), []byte("# launch\n"), 0644); err != nil {
		t.Fatal(err)
	}
	h := newServiceHandlerTestServer(t, root).Routes()

	for _, tc := range []struct {
		name string
		path string
		keys []string
	}{
		{name: "health", path: "/api/health", keys: []string{"ok", "config", "services", "health"}},
		{name: "ga_health", path: "/api/ga/health", keys: []string{"ok"}},
		{name: "version", path: "/api/version", keys: []string{"version"}},
		{name: "version_status", path: "/api/version/status", keys: []string{"running", "stage", "progress", "message"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			code, v, raw := serveJSONAnyContract(t, h, http.MethodGet, tc.path, "")
			if code != http.StatusOK {
				t.Fatalf("status=%d want=200 body=%s", code, raw)
			}
			body := requireJSONObjectContract(t, v, raw)
			for _, key := range tc.keys {
				if _, ok := body[key]; !ok {
					t.Fatalf("missing stable key %q in body=%s", key, raw)
				}
			}
		})
	}
}

func TestNonChatCollectionEndpointsReturnNonNullArrays(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "launch.py"), []byte("# launch\n"), 0644); err != nil {
		t.Fatal(err)
	}
	h := newServiceHandlerTestServer(t, root).Routes()

	for _, tc := range []struct {
		name     string
		path     string
		arrayKey string
	}{
		{name: "services", path: "/api/services", arrayKey: ""},
		{name: "ga_processes", path: "/api/ga/processes", arrayKey: "items"},
		{name: "models", path: "/api/models", arrayKey: "profiles"},
		{name: "files_list", path: "/api/files/list?path=.", arrayKey: "items"},
		{name: "files_search_no_hits", path: "/api/files/search?path=.&q=definitely-not-present", arrayKey: "hits"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			code, v, raw := serveJSONAnyContract(t, h, http.MethodGet, tc.path, "")
			if code != http.StatusOK {
				t.Fatalf("status=%d want=200 body=%s", code, raw)
			}
			if tc.arrayKey == "" {
				requireJSONArrayContract(t, v, raw)
				return
			}
			body := requireJSONObjectContract(t, v, raw)
			arrValue, ok := body[tc.arrayKey]
			if !ok {
				t.Fatalf("missing array key %q body=%s", tc.arrayKey, raw)
			}
			requireJSONArrayContract(t, arrValue, raw)
		})
	}
}

func TestNonChatReadEndpointsRejectUnsafeOrMalformedInputs(t *testing.T) {
	h := newServiceHandlerTestServer(t, t.TempDir()).Routes()

	for _, tc := range []struct {
		name string
		path string
		want string
	}{
		{name: "files_read_traversal", path: "/api/files/read?path=..%2Fsecret.txt", want: "path"},
		{name: "files_tail_bad_lines", path: "/api/files/tail?path=missing.log&lines=0", want: "lines"},
		{name: "log_unknown_service", path: "/api/logs/missing.py", want: "service"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			code, _, raw := serveJSONAnyContract(t, h, http.MethodGet, tc.path, "")
			if code < 400 || code >= 500 {
				t.Fatalf("status=%d want 4xx body=%s", code, raw)
			}
			if !strings.Contains(strings.ToLower(raw), tc.want) {
				t.Fatalf("body=%s does not mention %q", raw, tc.want)
			}
		})
	}
}

func TestNonChatDangerousRoutesRequireExplicitConfirm(t *testing.T) {
	h := newServiceHandlerTestServer(t, t.TempDir()).Routes()

	for _, tc := range []struct {
		name   string
		method string
		path   string
		body   string
	}{
		{name: "models_save", method: http.MethodPut, path: "/api/models", body: `{"profiles":[]}`},
		{name: "models_raw", method: http.MethodGet, path: "/api/models/raw", body: ""},
		{name: "files_write", method: http.MethodPost, path: "/api/files/write", body: `{"path":"x.txt","content":"x"}`},
		{name: "files_delete", method: http.MethodPost, path: "/api/files/delete", body: `{"path":"x.txt"}`},
		{name: "ga_process_kill", method: http.MethodPost, path: "/api/ga/processes/kill", body: `{"pid":1}`},
		{name: "ga_process_adopt", method: http.MethodPost, path: "/api/ga/processes/adopt", body: `{"pid":1}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			code, _, raw := serveJSONAnyContract(t, h, tc.method, tc.path, tc.body)
			if code != http.StatusPreconditionRequired {
				t.Fatalf("status=%d want=428 body=%s", code, raw)
			}
			if !strings.Contains(strings.ToLower(raw), "confirm") && !strings.Contains(strings.ToLower(raw), "danger") {
				t.Fatalf("dangerous rejection should mention confirm/danger body=%s", raw)
			}
		})
	}
}
