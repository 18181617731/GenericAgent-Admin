package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func authRequest(handler http.Handler, method, path, remote, user, password string, body any) *httptest.ResponseRecorder {
	var payload *bytes.Reader
	if body == nil {
		payload = bytes.NewReader(nil)
	} else {
		data, _ := json.Marshal(body)
		payload = bytes.NewReader(data)
	}
	req := httptest.NewRequest(method, path, payload)
	req.RemoteAddr = remote
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if user != "" || password != "" {
		req.SetBasicAuth(user, password)
	}
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, req)
	return recorder
}

func TestAuthManagerRequiresEnvironmentPair(t *testing.T) {
	if _, err := newAuthManager(t.TempDir(), "admin", ""); err == nil {
		t.Fatal("expected incomplete environment credentials to fail")
	}
	manager, err := newAuthManager(t.TempDir(), "operator", "configured-secret")
	if err != nil {
		t.Fatal(err)
	}
	if manager.mustChange || !manager.external {
		t.Fatalf("environment manager state = mustChange %v, external %v", manager.mustChange, manager.external)
	}
}

func TestAuthMiddlewareRequiresFirstRunSetup(t *testing.T) {
	manager, err := newAuthManager(t.TempDir(), "", "")
	if err != nil {
		t.Fatal(err)
	}
	if manager.initialized || manager.authenticatePassword("admin", "admin") {
		t.Fatal("fresh app root must not expose a default credential")
	}

	reached := false
	handler := manager.middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		reached = true
		w.WriteHeader(http.StatusNoContent)
	}))

	for _, tc := range []struct {
		name     string
		remote   string
		user     string
		password string
	}{
		{name: "remote anonymous", remote: "192.168.1.2:5000"},
		{name: "remote guessed default", remote: "192.168.1.2:5000", user: "admin", password: "admin"},
		{name: "loopback anonymous", remote: "127.0.0.1:5000"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := authRequest(handler, http.MethodGet, "/api/config", tc.remote, tc.user, tc.password, nil)
			if got.Code != http.StatusPreconditionRequired {
				t.Fatalf("request before setup = %d, want 428", got.Code)
			}
		})
	}
	if reached {
		t.Fatal("protected API reached the application before setup")
	}

	if got := authRequest(handler, http.MethodGet, "/", "192.168.1.2:5000", "", "", nil); got.Code != http.StatusNoContent || !reached {
		t.Fatalf("anonymous first-run SPA request = %d, reached %v", got.Code, reached)
	}
	status := authRequest(handler, http.MethodGet, "/api/auth/status", "192.168.1.2:5000", "", "", nil)
	if status.Code != http.StatusOK ||
		!bytes.Contains(status.Body.Bytes(), []byte(`"mustChangePassword":true`)) ||
		!bytes.Contains(status.Body.Bytes(), []byte(`"initialized":false`)) {
		t.Fatalf("status = %d %s", status.Code, status.Body.String())
	}
}

func TestChangePasswordPersistsAndSwitchesCredentials(t *testing.T) {
	root := t.TempDir()
	manager, err := newAuthManager(root, "", "")
	if err != nil {
		t.Fatal(err)
	}
	handler := manager.middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusNoContent) }))
	weakPassword := "Abc1234"
	weakBody := changePasswordRequest{CurrentPassword: "admin", NewPassword: weakPassword, ConfirmPassword: weakPassword}
	weak := authRequest(handler, http.MethodPost, "/api/auth/change-password", "127.0.0.1:5000", "", "", weakBody)
	if weak.Code != http.StatusBadRequest || !bytes.Contains(weak.Body.Bytes(), []byte(`"minimumLength":8`)) {
		t.Fatalf("seven-character password = %d %s, want 400 with minimumLength 8", weak.Code, weak.Body.String())
	}
	newPassword := "Abc12345"
	body := changePasswordRequest{CurrentPassword: "admin", NewPassword: newPassword, ConfirmPassword: newPassword}
	changed := authRequest(handler, http.MethodPost, "/api/auth/change-password", "127.0.0.1:5000", "", "", body)
	if changed.Code != http.StatusOK {
		t.Fatalf("change password = %d %s", changed.Code, changed.Body.String())
	}
	path := filepath.Join(root, authStateFilename)
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if runtime.GOOS != "windows" && info.Mode().Perm() != 0600 {
		t.Fatalf("auth file mode = %o, want 600", info.Mode().Perm())
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(data, []byte(newPassword)) || bytes.Contains(data, []byte(`"password"`)) {
		t.Fatal("auth file contains a plaintext password field or value")
	}
	if got := authRequest(handler, http.MethodGet, "/api/config", "192.168.1.2:5000", "admin", "admin", nil); got.Code != http.StatusUnauthorized {
		t.Fatalf("old credential after change = %d, want 401", got.Code)
	}
	if got := authRequest(handler, http.MethodGet, "/api/config", "192.168.1.2:5000", "admin", newPassword, nil); got.Code != http.StatusNoContent {
		t.Fatalf("new credential after change = %d, want 204", got.Code)
	}

	reloaded, err := newAuthManager(root, "", "")
	if err != nil {
		t.Fatal(err)
	}
	if reloaded.passwordChangeRequired() || !reloaded.authenticatePassword("admin", newPassword) {
		t.Fatal("persisted credential was not restored")
	}
}

func (a *authManager) authenticatePassword(user, password string) bool {
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.SetBasicAuth(user, password)
	return a.authenticateRequest(req)
}
