package api

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func localCmdRequest(t *testing.T, method, body string, confirmed bool) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(method, "/api/local-cmd/open", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	if confirmed {
		markDangerous(req)
	}
	rr := httptest.NewRecorder()
	newConfigTestServer(t).Routes().ServeHTTP(rr, req)
	return rr
}

func TestLocalCmdOpenRejectsWrongMethodAndMissingConfirmation(t *testing.T) {
	wrongMethod := localCmdRequest(t, http.MethodGet, `{}`, true)
	if wrongMethod.Code != http.StatusMethodNotAllowed {
		t.Fatalf("GET status=%d want=%d body=%s", wrongMethod.Code, http.StatusMethodNotAllowed, wrongMethod.Body.String())
	}
	missingConfirm := localCmdRequest(t, http.MethodPost, `{}`, false)
	if missingConfirm.Code != http.StatusPreconditionRequired {
		t.Fatalf("POST without confirmation status=%d want=%d body=%s", missingConfirm.Code, http.StatusPreconditionRequired, missingConfirm.Body.String())
	}
	if !strings.Contains(missingConfirm.Body.String(), "X-GA-Confirm") {
		t.Fatalf("missing confirmation guidance: %s", missingConfirm.Body.String())
	}
}

func TestLocalCmdOpenRejectsMalformedJSON(t *testing.T) {
	rr := localCmdRequest(t, http.MethodPost, `not-json`, true)
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("status=%d want=%d body=%s", rr.Code, http.StatusBadRequest, rr.Body.String())
	}
}

func TestLocalCmdOpenRejectsInvalidDirectories(t *testing.T) {
	filePath := filepath.Join(t.TempDir(), "not-a-directory.txt")
	if err := os.WriteFile(filePath, []byte("test"), 0600); err != nil {
		t.Fatal(err)
	}
	cases := []struct {
		name string
		path string
	}{
		{name: "empty", path: ""},
		{name: "relative", path: filepath.Join("relative", "directory")},
		{name: "missing", path: filepath.Join(t.TempDir(), "missing")},
		{name: "file", path: filePath},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rr := localCmdOpenWithPath(t, tc.path, true)
			if rr.Code != http.StatusBadRequest {
				t.Fatalf("path=%q status=%d want=%d body=%s", tc.path, rr.Code, http.StatusBadRequest, rr.Body.String())
			}
		})
	}
}

func TestLocalCmdOpenPassesValidatedDirectoryToLauncher(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "中文 local cmd")
	if err := os.Mkdir(dir, 0700); err != nil {
		t.Fatal(err)
	}
	var got string
	oldLauncher := launchLocalCmdFunc
	launchLocalCmdFunc = func(path string) error {
		got = path
		return nil
	}
	t.Cleanup(func() { launchLocalCmdFunc = oldLauncher })

	rr := localCmdOpenWithPath(t, "  "+dir+"  ", true)
	if rr.Code != http.StatusOK {
		t.Fatalf("status=%d want=%d body=%s", rr.Code, http.StatusOK, rr.Body.String())
	}
	if got != filepath.Clean(dir) {
		t.Fatalf("launcher path=%q want=%q", got, filepath.Clean(dir))
	}
	if !strings.Contains(rr.Body.String(), `"ok":true`) {
		t.Fatalf("unexpected success response: %s", rr.Body.String())
	}
}

func TestLocalCmdOpenReturnsUnsupportedOnNonWindows(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("non-Windows behavior is covered by the other build-tag implementation")
	}
	if err := startLocalCmd(t.TempDir()); !errors.Is(err, errLocalCmdUnsupported) {
		t.Fatalf("startLocalCmd() error=%v, want %v", err, errLocalCmdUnsupported)
	}
}

func localCmdOpenWithPath(t *testing.T, path string, confirmed bool) *httptest.ResponseRecorder {
	t.Helper()
	body, err := json.Marshal(localCmdOpenRequest{Path: path})
	if err != nil {
		t.Fatal(err)
	}
	return localCmdRequest(t, http.MethodPost, string(body), confirmed)
}
