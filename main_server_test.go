package main

import (
	"bytes"
	"net/http"
	"os"
	"path/filepath"
	"testing"
)

func TestNewHTTPServerSetsTimeouts(t *testing.T) {
	handler := http.NewServeMux()
	server := newHTTPServer("127.0.0.1:0", handler)

	if server.Addr != "127.0.0.1:0" {
		t.Fatalf("Addr = %q, want %q", server.Addr, "127.0.0.1:0")
	}
	if server.Handler != handler {
		t.Fatal("Handler must be installed unchanged")
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

func TestParseInternalLaunchArgs(t *testing.T) {
	tests := []struct {
		name string
		args []string
		want internalLaunchOptions
	}{
		{
			name: "public arguments only",
			args: []string{"--headless", "--port", "9000"},
			want: internalLaunchOptions{PublicArgs: []string{"--headless", "--port", "9000"}},
		},
		{
			name: "helper split form",
			args: []string{"--headless", "--update-helper", `C:\\temp\\update.json`, "--no-browser"},
			want: internalLaunchOptions{
				HelperManifest: `C:\\temp\\update.json`,
				PublicArgs:     []string{"--headless", "--no-browser"},
			},
		},
		{
			name: "confirmation equal form",
			args: []string{"--port=9001", "--update-confirm=operation-7"},
			want: internalLaunchOptions{
				ConfirmOperation: "operation-7",
				PublicArgs:       []string{"--port=9001"},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseInternalLaunchArgs(tt.args)
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("parseInternalLaunchArgs(%#v) = %#v, want %#v", tt.args, got, tt.want)
			}
		})
	}
}

func TestParseInternalLaunchArgsRejectsUnsafeForms(t *testing.T) {
	for _, args := range [][]string{
		{"--update-helper"},
		{"--update-helper="},
		{"--update-confirm", ""},
		{"--update-confirm=operation-1", "--update-confirm", "operation-2"},
		{"--update-helper", "manifest.json", "--update-confirm", "operation-1"},
	} {
		t.Run(strings.Join(args, "_"), func(t *testing.T) {
			if _, err := parseInternalLaunchArgs(args); err == nil {
				t.Fatalf("parseInternalLaunchArgs(%#v) error = nil, want rejection", args)
			}
		})
	}
}

func TestOpenListenerAndConfirmBindsBeforeConfirmation(t *testing.T) {
	var listener net.Listener
	var events []string
	got, err := openListenerAndConfirm(func() (net.Listener, error) {
		var openErr error
		listener, openErr = net.Listen("tcp", "127.0.0.1:0")
		events = append(events, "bound")
		return listener, openErr
	}, "operation-1", func(operationID string) error {
		if listener == nil {
			t.Fatal("confirmation ran before listener bind")
		}
		if operationID != "operation-1" {
			t.Fatalf("confirmation operation = %q", operationID)
		}
		events = append(events, "confirmed")
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	defer got.Close()
	if !reflect.DeepEqual(events, []string{"bound", "confirmed"}) {
		t.Fatalf("events = %#v", events)
	}
}

func TestOpenListenerAndConfirmClosesListenerOnConfirmationFailure(t *testing.T) {
	var listener net.Listener
	confirmErr := errors.New("confirmation failed")
	got, err := openListenerAndConfirm(func() (net.Listener, error) {
		var openErr error
		listener, openErr = net.Listen("tcp", "127.0.0.1:0")
		return listener, openErr
	}, "operation-2", func(string) error {
		return confirmErr
	})
	if !errors.Is(err, confirmErr) {
		t.Fatalf("openListenerAndConfirm() error = %v, want %v", err, confirmErr)
	}
	if got != nil {
		t.Fatalf("openListenerAndConfirm() listener = %v, want nil", got)
	}
	if listener == nil {
		t.Fatal("listener was never opened")
	}
	if closeErr := listener.Close(); closeErr == nil {
		t.Fatal("listener remained open after confirmation failure")
	}
}
