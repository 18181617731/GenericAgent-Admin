package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"time"

	"genericagent-admin-go/internal/config"
)

const runtimeInfoFilename = "runtime.local.json"

// openAdminListener binds the admin HTTP listener for this launch.
//
// Remote access that cannot authenticate anybody is downgraded to loopback
// rather than published: a half-finished setup must never be the reason the
// machine ends up serving process execution and file access to the network.
func openAdminListener(cfg config.AppConfig, portOverride int, passwordConfigured bool) (net.Listener, error) {
	if cfg.RemoteAccess && !cfg.RemoteAllowAnonymous && !passwordConfigured {
		log.Printf("remote access is enabled but no password is set; staying on loopback until one is configured")
		cfg.RemoteAccess = false
	}
	addr := config.ListenAddress(cfg, portOverride)
	listener, err := net.Listen("tcp", addr)
	if err != nil {
		if cfg.RemoteAccess || portOverride > 0 {
			return nil, fmt.Errorf("listen %s failed: %w; the port is most likely already in use, pick another one in settings", addr, err)
		}
		return nil, fmt.Errorf("listen %s failed: %w", addr, err)
	}
	return listener, nil
}

// localURL is the address the desktop window and browser should open. It is
// always loopback: even when the server also answers the network, the local
// UI talks to it as a local client and skips authentication.
func localURL(listener net.Listener) string {
	_, port, err := net.SplitHostPort(listener.Addr().String())
	if err != nil {
		return "http://" + listener.Addr().String()
	}
	return "http://127.0.0.1:" + port
}

func listenerPort(listener net.Listener) int {
	_, port, err := net.SplitHostPort(listener.Addr().String())
	if err != nil {
		return 0
	}
	parsed, err := strconv.Atoi(port)
	if err != nil {
		return 0
	}
	return parsed
}

// listenerIsRemote reports whether the bound socket answers anything beyond
// this machine. It reads the listener rather than the config so a launch that
// was downgraded to loopback is reported as local, not as the remote access
// the config asked for.
func listenerIsRemote(listener net.Listener) bool {
	host, _, err := net.SplitHostPort(listener.Addr().String())
	if err != nil {
		return false
	}
	return !config.IsLoopbackHost(host)
}

// runtimeInfo records where this process actually ended up listening. The
// default ephemeral port is unknowable ahead of time, so tooling such as the
// Vite dev proxy reads it from here instead of guessing.
type runtimeInfo struct {
	URL          string `json:"url"`
	Address      string `json:"address"`
	Port         int    `json:"port"`
	RemoteAccess bool   `json:"remote_access"`
	PID          int    `json:"pid"`
	StartedAt    string `json:"started_at"`
}

func writeRuntimeInfo(appRoot string, listener net.Listener) error {
	info := runtimeInfo{
		URL:          localURL(listener),
		Address:      listener.Addr().String(),
		Port:         listenerPort(listener),
		RemoteAccess: listenerIsRemote(listener),
		PID:          os.Getpid(),
		StartedAt:    time.Now().UTC().Format(time.RFC3339),
	}
	data, err := json.MarshalIndent(info, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(appRoot, runtimeInfoFilename), append(data, '\n'), 0o644)
}

func removeRuntimeInfo(appRoot string) {
	if err := os.Remove(filepath.Join(appRoot, runtimeInfoFilename)); err != nil && !os.IsNotExist(err) {
		log.Printf("remove %s: %v", runtimeInfoFilename, err)
	}
}
