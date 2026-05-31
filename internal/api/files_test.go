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
			if tc.name == "write" {
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
