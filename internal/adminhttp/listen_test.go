package adminhttp

import (
	"encoding/json"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"genericagent-admin-go/internal/config"
)

func TestOpenAdminListenerDefaultsToLoopbackEphemeral(t *testing.T) {
	cfg := config.Default()
	listener, err := OpenListener(cfg, 0, false)
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()

	addr := listener.Addr().String()
	if !strings.HasPrefix(addr, "127.0.0.1:") {
		t.Fatalf("listener bound %s, want loopback", addr)
	}
	if port := listenerPort(listener); port == 0 || port == 8787 {
		t.Fatalf("listener port = %d, want an OS-assigned ephemeral port", port)
	}
	if url := LocalURL(listener); !strings.HasPrefix(url, "http://127.0.0.1:") {
		t.Fatalf("local URL = %q", url)
	}
}

func TestOpenAdminListenerDowngradesRemoteAccessWithoutPassword(t *testing.T) {
	cfg := config.Default()
	cfg.RemoteAccess = true
	cfg.Port = 8787

	listener, err := OpenListener(cfg, 0, false)
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()

	if addr := listener.Addr().String(); !strings.HasPrefix(addr, "127.0.0.1:") {
		t.Fatalf("passwordless remote config bound %s, want loopback downgrade", addr)
	}
	if listenerIsRemote(listener) {
		t.Fatal("a downgraded launch must not report itself as remote")
	}
}

func TestOpenAdminListenerPublishesRemoteAccessWithPassword(t *testing.T) {
	cfg := config.Default()
	cfg.RemoteAccess = true
	cfg.Port = 0 // any free port: the wildcard bind is what matters here

	listener, err := OpenListener(cfg, freePort(t), true)
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()

	if !listenerIsRemote(listener) {
		t.Fatalf("listener bound %s, want a non-loopback interface", listener.Addr())
	}
	// The desktop window still talks to the server as a local client.
	if url := LocalURL(listener); !strings.HasPrefix(url, "http://127.0.0.1:") {
		t.Fatalf("local URL = %q", url)
	}
}

func freePort(t *testing.T) int {
	t.Helper()
	probe, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	port := listenerPort(probe)
	if err := probe.Close(); err != nil {
		t.Fatal(err)
	}
	return port
}

func TestRuntimeInfoRoundTrip(t *testing.T) {
	root := t.TempDir()
	cfg := config.Default()
	listener, err := OpenListener(cfg, 0, false)
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()

	if err := WriteRuntimeInfo(root, listener); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(root, runtimeInfoFilename))
	if err != nil {
		t.Fatal(err)
	}
	var info runtimeInfo
	if err := json.Unmarshal(data, &info); err != nil {
		t.Fatal(err)
	}
	if info.Port != listenerPort(listener) || info.URL != LocalURL(listener) || info.PID != os.Getpid() {
		t.Fatalf("runtime info mismatch: %+v", info)
	}
	if info.RemoteAccess {
		t.Fatal("a loopback listener must not be recorded as remote")
	}

	RemoveRuntimeInfo(root)
	if _, err := os.Stat(filepath.Join(root, runtimeInfoFilename)); !os.IsNotExist(err) {
		t.Fatalf("runtime info file still present: %v", err)
	}
}
