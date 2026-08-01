package main

import (
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
)

func TestNewHTTPServerSetsTimeouts(t *testing.T) {
	handler := http.NewServeMux()
	server := newHTTPServer("127.0.0.1:0", handler)

	if server.Addr != "127.0.0.1:0" {
		t.Fatalf("Addr = %q, want %q", server.Addr, "127.0.0.1:0")
	}
	if server.Handler == handler {
		t.Fatal("Handler must be wrapped by external authentication")
	}
	if server.ReadHeaderTimeout != adminReadHeaderTimeout {
		t.Fatalf("ReadHeaderTimeout = %v, want %v", server.ReadHeaderTimeout, adminReadHeaderTimeout)
	}
	if server.IdleTimeout != adminIdleTimeout {
		t.Fatalf("IdleTimeout = %v, want %v", server.IdleTimeout, adminIdleTimeout)
	}
	if server.ReadHeaderTimeout <= 0 {
		t.Fatal("ReadHeaderTimeout must be positive")
	}
	if server.IdleTimeout <= server.ReadHeaderTimeout {
		t.Fatalf("IdleTimeout = %v must exceed ReadHeaderTimeout = %v", server.IdleTimeout, server.ReadHeaderTimeout)
	}
}

func TestRequireExternalAuth(t *testing.T) {
	tests := []struct {
		name        string
		remoteAddr  string
		user        string
		password    string
		forwarded   string
		wantStatus  int
		wantReached bool
	}{
		{name: "127 localhost bypasses auth", remoteAddr: "127.0.0.1:5010", wantStatus: http.StatusNoContent, wantReached: true},
		{name: "127 subnet bypasses auth", remoteAddr: "127.42.0.9:5010", wantStatus: http.StatusNoContent, wantReached: true},
		{name: "external without auth is denied", remoteAddr: "192.168.1.20:5010", wantStatus: http.StatusUnauthorized},
		{name: "external wrong auth is denied", remoteAddr: "10.0.0.8:5010", user: "admin", password: "wrong", wantStatus: http.StatusUnauthorized},
		{name: "external correct auth passes", remoteAddr: "10.0.0.8:5010", user: "admin", password: "secret", wantStatus: http.StatusNoContent, wantReached: true},
		{name: "forwarded loopback is not trusted", remoteAddr: "10.0.0.8:5010", forwarded: "127.0.0.1", wantStatus: http.StatusUnauthorized},
		{name: "ipv6 loopback is not 127", remoteAddr: "[::1]:5010", wantStatus: http.StatusUnauthorized},
		{name: "malformed remote address is denied", remoteAddr: "127.0.0.1", wantStatus: http.StatusUnauthorized},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reached := false
			handler := requireExternalAuth(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				reached = true
				w.WriteHeader(http.StatusNoContent)
			}), "admin", "secret")
			req := httptest.NewRequest(http.MethodGet, "http://ga-admin.test/api/health", nil)
			req.RemoteAddr = tt.remoteAddr
			if tt.forwarded != "" {
				req.Header.Set("X-Forwarded-For", tt.forwarded)
			}
			if tt.user != "" || tt.password != "" {
				req.SetBasicAuth(tt.user, tt.password)
			}
			recorder := httptest.NewRecorder()

			handler.ServeHTTP(recorder, req)

			if recorder.Code != tt.wantStatus {
				t.Fatalf("status = %d, want %d", recorder.Code, tt.wantStatus)
			}
			if reached != tt.wantReached {
				t.Fatalf("downstream reached = %v, want %v", reached, tt.wantReached)
			}
			if tt.wantStatus == http.StatusUnauthorized {
				if !strings.HasPrefix(recorder.Header().Get("WWW-Authenticate"), "Basic ") {
					t.Fatal("401 response must include a Basic challenge")
				}
				if recorder.Header().Get("Cache-Control") != "no-store" {
					t.Fatal("401 response must not be cached")
				}
			}
		})
	}
}

func TestRequireExternalAuthFailsClosedWithoutCredentials(t *testing.T) {
	handler := requireExternalAuth(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		t.Fatal("external request reached downstream without configured credentials")
	}), "", "")
	req := httptest.NewRequest(http.MethodGet, "http://ga-admin.test/", nil)
	req.RemoteAddr = "192.168.1.20:5010"
	req.SetBasicAuth("", "")
	recorder := httptest.NewRecorder()

	handler.ServeHTTP(recorder, req)

	if recorder.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusUnauthorized)
	}
}

func TestAppRootExplicitRootTakesPrecedence(t *testing.T) {
	explicit := t.TempDir()
	got, err := appRoot(explicit)
	if err != nil {
		t.Fatal(err)
	}
	want, err := filepath.Abs(explicit)
	if err != nil {
		t.Fatal(err)
	}
	if got != want {
		t.Fatalf("appRoot(%q) = %q, want %q", explicit, got, want)
	}
}
