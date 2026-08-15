package main

import (
	"bytes"
	"net/http"
	"os"
	"path/filepath"
	"testing"
)

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

func TestConfiguredServicesStartBeforeHTTPListener(t *testing.T) {
	source, err := os.ReadFile("main.go")
	if err != nil {
		t.Fatal(err)
	}
	serviceStart := bytes.Index(source, []byte("srv.StartAutostartServices()"))
	listenerStart := bytes.Index(source, []byte("startHTTPListeners(server, addrs)"))
	if serviceStart < 0 || listenerStart < 0 {
		t.Fatal("main startup sequence is missing service or listener initialization")
	}
	if serviceStart > listenerStart {
		t.Fatal("configured services must start before the HTTP listener is exposed")
	}
	if bytes.Contains(source, []byte("go srv.StartAutostartServices()")) {
		t.Fatal("configured services must not start asynchronously")
	}
}
