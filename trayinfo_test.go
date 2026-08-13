package main

import (
	"strings"
	"testing"

	"genericagent-admin-go/internal/config"
)

const testLAN = "192.168.1.20"

func lanIP(ip string) func() string { return func() string { return ip } }

func TestTrayStatusLoopbackLaunch(t *testing.T) {
	status := describeTrayStatus("127.0.0.1:50451", config.Default(), false, lanIP(testLAN), trayZH)

	if status.LocalURL != "http://127.0.0.1:50451" {
		t.Fatalf("local URL = %q", status.LocalURL)
	}
	if status.LocalLabel != "地址：127.0.0.1:50451" {
		t.Fatalf("local label = %q", status.LocalLabel)
	}
	// A local-only server has no address worth handing to another device.
	if status.LANLabel != "" || status.LANURL != "" {
		t.Fatalf("loopback launch advertised a LAN address: %q %q", status.LANLabel, status.LANURL)
	}
	if status.ScopeLabel != trayZH.ScopeLabel+trayZH.ScopeLocalOnly {
		t.Fatalf("scope = %q", status.ScopeLabel)
	}
	if !strings.Contains(status.Tooltip, "127.0.0.1:50451") || !strings.Contains(status.Tooltip, trayZH.ScopeLocalOnly) {
		t.Fatalf("tooltip = %q", status.Tooltip)
	}
}

func TestTrayStatusPublishedWithPassword(t *testing.T) {
	cfg := config.Default()
	cfg.RemoteAccess = true
	cfg.Port = 8787

	status := describeTrayStatus("[::]:8787", cfg, true, lanIP(testLAN), trayZH)

	if status.LANURL != "http://192.168.1.20:8787" {
		t.Fatalf("LAN URL = %q", status.LANURL)
	}
	if status.LANLabel != "局域网：192.168.1.20:8787" {
		t.Fatalf("LAN label = %q", status.LANLabel)
	}
	// The window still reaches the server as a local client.
	if status.LocalURL != "http://127.0.0.1:8787" {
		t.Fatalf("local URL = %q", status.LocalURL)
	}
	if status.ScopeLabel != trayZH.ScopeLabel+trayZH.ScopeRemotePassword {
		t.Fatalf("scope = %q", status.ScopeLabel)
	}
}

func TestTrayStatusPublishedAnonymously(t *testing.T) {
	cfg := config.Default()
	cfg.RemoteAccess = true
	cfg.RemoteAllowAnonymous = true
	cfg.Port = 8787

	status := describeTrayStatus("0.0.0.0:8787", cfg, false, lanIP(testLAN), trayZH)

	if status.ScopeLabel != trayZH.ScopeLabel+trayZH.ScopeRemoteAnonymous {
		t.Fatalf("scope = %q, want the anonymous warning", status.ScopeLabel)
	}
}

func TestTrayStatusPublishedWithoutARoutableAddress(t *testing.T) {
	cfg := config.Default()
	cfg.RemoteAccess = true
	cfg.Port = 8787

	status := describeTrayStatus("0.0.0.0:8787", cfg, true, lanIP(""), trayZH)

	if status.LANLabel != "" {
		t.Fatalf("LAN label = %q, want empty when no interface answers", status.LANLabel)
	}
	if !strings.Contains(status.Tooltip, "127.0.0.1:8787") {
		t.Fatalf("tooltip = %q, want the local address as a fallback", status.Tooltip)
	}
}

// The socket cannot change until the process restarts, so a config edited from
// the settings page must be reported as pending rather than as fact.
func TestTrayStatusReportsPendingRemoteChanges(t *testing.T) {
	cases := []struct {
		name        string
		listen      string
		remote      bool
		anonymous   bool
		passwordSet bool
		wantScope   string
	}{
		{
			name:   "remote turned on after launch waits for a restart",
			listen: "127.0.0.1:50451", remote: true, passwordSet: true,
			wantScope: trayZH.ScopeLocalOnly + trayZH.PendingRestart,
		},
		{
			name:   "remote turned off after launch still serves the network",
			listen: "0.0.0.0:8787", remote: false, passwordSet: true,
			wantScope: trayZH.ScopeRemotePassword + trayZH.PendingRestart,
		},
		{
			name:   "remote without a password explains the blocker",
			listen: "127.0.0.1:50451", remote: true, passwordSet: false,
			wantScope: trayZH.ScopeLocalOnly + trayZH.PendingNeedsPassword,
		},
		{
			name:   "anonymous remote needs no password to be pending",
			listen: "127.0.0.1:50451", remote: true, anonymous: true, passwordSet: false,
			wantScope: trayZH.ScopeLocalOnly + trayZH.PendingRestart,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cfg := config.Default()
			cfg.RemoteAccess = tc.remote
			cfg.RemoteAllowAnonymous = tc.anonymous
			cfg.Port = 8787
			status := describeTrayStatus(tc.listen, cfg, tc.passwordSet, lanIP(""), trayZH)
			if status.ScopeLabel != trayZH.ScopeLabel+tc.wantScope {
				t.Fatalf("scope = %q, want %q", status.ScopeLabel, trayZH.ScopeLabel+tc.wantScope)
			}
		})
	}
}

// The scope row costs a line of menu, so only a server that is reachable from
// the network, or out of step with its config, is allowed to take one.
func TestTrayStatusFlagsOnlyAScopeWorthShowing(t *testing.T) {
	if describeTrayStatus("127.0.0.1:50451", config.Default(), false, lanIP(testLAN), trayZH).ScopeAlert {
		t.Fatal("a local-only server in step with its config asked for a scope row")
	}

	cfg := config.Default()
	cfg.RemoteAccess = true
	cfg.Port = 8787
	if !describeTrayStatus("0.0.0.0:8787", cfg, true, lanIP(testLAN), trayZH).ScopeAlert {
		t.Fatal("a published server did not ask for a scope row")
	}
	// Remote access was switched on but this process is still on loopback.
	if !describeTrayStatus("127.0.0.1:50451", cfg, true, lanIP(""), trayZH).ScopeAlert {
		t.Fatal("a pending remote change did not ask for a scope row")
	}
}

func TestTrayStatusSurvivesAnUnparsableAddress(t *testing.T) {
	status := describeTrayStatus("not-an-address", config.Default(), false, nil, trayZH)

	if status.LocalURL != "" {
		t.Fatalf("local URL = %q, want empty for an address we cannot read", status.LocalURL)
	}
	if !strings.Contains(status.LocalLabel, "not-an-address") {
		t.Fatalf("local label = %q", status.LocalLabel)
	}
	if status.ScopeLabel != trayZH.ScopeLabel+trayZH.ScopeUnknown {
		t.Fatalf("scope = %q", status.ScopeLabel)
	}
}

func TestTrayStatusToleratesAMissingLANLookup(t *testing.T) {
	cfg := config.Default()
	cfg.RemoteAccess = true
	cfg.Port = 8787

	status := describeTrayStatus("0.0.0.0:8787", cfg, true, nil, trayZH)

	if status.LANLabel != "" {
		t.Fatalf("LAN label = %q", status.LANLabel)
	}
}

func TestTrayAppStatusDefaultsToEmpty(t *testing.T) {
	if got := (trayApp{}).status(); got != (trayStatus{}) {
		t.Fatalf("status without a provider = %+v, want the zero value", got)
	}
}
