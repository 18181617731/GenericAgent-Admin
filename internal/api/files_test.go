package api

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

func TestFilesTailRejectsInvalidLinesQuery(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "sample.log"), []byte("one\ntwo\n"), 0644); err != nil {
		t.Fatal(err)
	}
	h := newGoalTestServer(t, root).Routes()

	for _, raw := range []string{"abc", "0", "-3"} {
		rr := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/api/files/tail?path=sample.log&lines="+raw, nil)
		h.ServeHTTP(rr, req)
		if rr.Code != http.StatusBadRequest {
			t.Fatalf("lines=%q status=%d want=400 body=%s", raw, rr.Code, rr.Body.String())
		}
		if !strings.Contains(rr.Body.String(), "lines") {
			t.Fatalf("lines=%q unexpected error body: %s", raw, rr.Body.String())
		}
	}
}

func TestFilesSearchRejectsInvalidLimitQuery(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "sample.log"), []byte("alpha\nbeta\n"), 0644); err != nil {
		t.Fatal(err)
	}
	h := newGoalTestServer(t, root).Routes()

	for _, raw := range []string{"abc", "0", "-3"} {
		rr := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/api/files/search?path=.&q=alpha&limit="+raw, nil)
		h.ServeHTTP(rr, req)
		if rr.Code != http.StatusBadRequest {
			t.Fatalf("limit=%q status=%d want=400 body=%s", raw, rr.Code, rr.Body.String())
		}
		if !strings.Contains(rr.Body.String(), "limit") {
			t.Fatalf("limit=%q unexpected error body: %s", raw, rr.Body.String())
		}
	}
}

func TestFilesImageServesSVGWithIsolationHeaders(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "x.svg"), []byte(`<svg xmlns="http://www.w3.org/2000/svg"><script>alert(1)</script></svg>`), 0644); err != nil {
		t.Fatal(err)
	}
	h := newGoalTestServer(t, root).Routes()

	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/files/image?path=x.svg", nil)
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("status=%d want=200 body=%s", rr.Code, rr.Body.String())
	}
	if got := rr.Header().Get("Content-Type"); !strings.Contains(got, "image/svg+xml") {
		t.Fatalf("Content-Type=%q want image/svg+xml", got)
	}
	if got := rr.Header().Get("X-Content-Type-Options"); got != "nosniff" {
		t.Fatalf("X-Content-Type-Options=%q want nosniff", got)
	}
	if got := rr.Header().Get("Content-Security-Policy"); !strings.Contains(got, "sandbox") || !strings.Contains(got, "default-src 'none'") {
		t.Fatalf("Content-Security-Policy=%q missing SVG sandbox", got)
	}
}

func TestFilesOpenRequiresDangerousConfirm(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "sample.txt"), []byte("visible"), 0644); err != nil {
		t.Fatal(err)
	}
	h := newGoalTestServer(t, root).Routes()

	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/files/open", strings.NewReader(`{"path":"sample.txt","mode":"file"}`))
	req.Header.Set("Content-Type", "application/json")
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusPreconditionRequired {
		t.Fatalf("status=%d want=428 body=%s", rr.Code, rr.Body.String())
	}
	if !strings.Contains(rr.Body.String(), "X-GA-Confirm") {
		t.Fatalf("unexpected error body: %s", rr.Body.String())
	}
}

func TestFilesOpenRejectsInvalidMode(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "sample.txt"), []byte("visible"), 0644); err != nil {
		t.Fatal(err)
	}
	h := newGoalTestServer(t, root).Routes()

	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/files/open", strings.NewReader(`{"path":"sample.txt","mode":"bogus"}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-GA-Confirm", "dangerous")
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("status=%d want=400 body=%s", rr.Code, rr.Body.String())
	}
	if !strings.Contains(rr.Body.String(), "mode") {
		t.Fatalf("unexpected error body: %s", rr.Body.String())
	}
}

func TestFilesEndpointsRejectPathTraversal(t *testing.T) {
	root := t.TempDir()
	outside := filepath.Join(t.TempDir(), "outside.txt")
	if err := os.WriteFile(outside, []byte("secret"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "inside.txt"), []byte("visible"), 0644); err != nil {
		t.Fatal(err)
	}
	h := newGoalTestServer(t, root).Routes()

	cases := []struct {
		name   string
		method string
		path   string
		body   string
	}{
		{name: "list", method: http.MethodGet, path: "/api/files/list?path=.."},
		{name: "read", method: http.MethodGet, path: "/api/files/read?path=../outside.txt"},
		{name: "image", method: http.MethodGet, path: "/api/files/image?path=../outside.txt"},
		{name: "image absolute", method: http.MethodGet, path: "/api/files/image?path=" + outside},
		{name: "open absolute", method: http.MethodPost, path: "/api/files/open", body: `{"path":` + strconv.Quote(outside) + `,"mode":"file"}`},
		{name: "tail", method: http.MethodGet, path: "/api/files/tail?path=../outside.txt"},
		{name: "search", method: http.MethodGet, path: "/api/files/search?path=..&q=secret"},
		{name: "write", method: http.MethodPost, path: "/api/files/write", body: `{"path":"../outside.txt","content":"pwned"}`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var body *bytes.Reader
			if tc.body != "" {
				body = bytes.NewReader([]byte(tc.body))
			} else {
				body = bytes.NewReader(nil)
			}
			rr := httptest.NewRecorder()
			req := httptest.NewRequest(tc.method, tc.path, body)
			if tc.name == "write" || tc.name == "open absolute" {
				req.Header.Set("X-GA-Confirm", "dangerous")
				req.Header.Set("Content-Type", "application/json")
			}
			h.ServeHTTP(rr, req)
			if rr.Code != http.StatusBadRequest {
				t.Fatalf("status=%d want=400 body=%s", rr.Code, rr.Body.String())
			}
			if !strings.Contains(rr.Body.String(), "escapes GA root") {
				t.Fatalf("unexpected error body: %s", rr.Body.String())
			}
		})
	}
	got, err := os.ReadFile(outside)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "secret" {
		t.Fatalf("outside file was modified: %q", got)
	}
}

func TestFilesDownloadServesFile(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "download.txt"), []byte("payload"), 0644); err != nil {
		t.Fatal(err)
	}
	h := newGoalTestServer(t, root).Routes()

	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/files/download?path=download.txt", nil)
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("status=%d want=200 body=%s", rr.Code, rr.Body.String())
	}
	if rr.Body.String() != "payload" {
		t.Fatalf("body=%q want payload", rr.Body.String())
	}
	if got := rr.Header().Get("Content-Disposition"); !strings.Contains(got, "download.txt") {
		t.Fatalf("Content-Disposition=%q", got)
	}
}

func TestFilesDeleteRequiresDangerousConfirmAndDeletes(t *testing.T) {
	root := t.TempDir()
	target := filepath.Join(root, "delete-me.txt")
	if err := os.WriteFile(target, []byte("gone"), 0644); err != nil {
		t.Fatal(err)
	}
	h := newGoalTestServer(t, root).Routes()

	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/files/delete", strings.NewReader(`{"path":"delete-me.txt"}`))
	req.Header.Set("Content-Type", "application/json")
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusPreconditionRequired {
		t.Fatalf("status=%d want=428 body=%s", rr.Code, rr.Body.String())
	}
	if _, err := os.Stat(target); err != nil {
		t.Fatalf("file removed without dangerous confirm: %v", err)
	}

	rr = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/api/files/delete", strings.NewReader(`{"path":"delete-me.txt"}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-GA-Confirm", "dangerous")
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("status=%d want=200 body=%s", rr.Code, rr.Body.String())
	}
	if _, err := os.Stat(target); !os.IsNotExist(err) {
		t.Fatalf("file still exists or stat failed unexpectedly: %v", err)
	}
}
