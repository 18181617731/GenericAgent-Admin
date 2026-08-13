package adminhttp

import (
	"net/http"
	"testing"
)

func TestNewServerSetsTimeouts(t *testing.T) {
	handler := http.NewServeMux()
	server := NewServer("127.0.0.1:0", handler)

	if server.Addr != "127.0.0.1:0" {
		t.Fatalf("Addr = %q, want %q", server.Addr, "127.0.0.1:0")
	}
	if server.Handler != handler {
		t.Fatal("Handler must be installed unchanged")
	}
	if server.ReadHeaderTimeout != readHeaderTimeout {
		t.Fatalf("ReadHeaderTimeout = %v, want %v", server.ReadHeaderTimeout, readHeaderTimeout)
	}
	if server.IdleTimeout != idleTimeout {
		t.Fatalf("IdleTimeout = %v, want %v", server.IdleTimeout, idleTimeout)
	}
	if server.ReadHeaderTimeout <= 0 {
		t.Fatal("ReadHeaderTimeout must be positive")
	}
	if server.IdleTimeout <= server.ReadHeaderTimeout {
		t.Fatalf("IdleTimeout = %v must exceed ReadHeaderTimeout = %v", server.IdleTimeout, server.ReadHeaderTimeout)
	}
}
