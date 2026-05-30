package api

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
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
