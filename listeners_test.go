package main

import (
	"context"
	"net"
	"net/http"
	"net/netip"
	"testing"
	"time"
)

func TestTailscaleIPv4FromInterfaces(t *testing.T) {
	interfaces := []networkInterface{
		{name: "down", addrs: testAddrs("100.64.0.1/32")},
		{name: "loopback", flags: net.FlagUp | net.FlagLoopback, addrs: testAddrs("100.64.0.2/32")},
		{name: "tailscale0", flags: net.FlagUp, addrs: testAddrs("100.127.255.254/32", "100.92.41.120/32", "100.92.41.120/32", "fd7a:115c:a1e0::1/128")},
		{name: "wlan", flags: net.FlagUp, addrs: testAddrs("10.168.5.233/24", "100.128.0.1/16")},
	}

	got := tailscaleIPv4FromInterfaces(interfaces)
	want := []netip.Addr{netip.MustParseAddr("100.92.41.120"), netip.MustParseAddr("100.127.255.254")}
	if len(got) != len(want) {
		t.Fatalf("addresses = %v, want %v", got, want)
	}
	for index := range want {
		if got[index] != want[index] {
			t.Fatalf("addresses[%d] = %v, want %v", index, got[index], want[index])
		}
	}
}

func TestAdminListenAddresses(t *testing.T) {
	tailscaleIPs := []netip.Addr{
		netip.MustParseAddr("100.92.41.120"),
		netip.MustParseAddr("100.92.41.120"),
	}
	got := adminListenAddresses("127.0.0.1", 8787, tailscaleIPs)
	want := []string{"127.0.0.1:8787", "100.92.41.120:8787"}
	if len(got) != len(want) {
		t.Fatalf("addresses = %v, want %v", got, want)
	}
	for index := range want {
		if got[index] != want[index] {
			t.Fatalf("addresses[%d] = %q, want %q", index, got[index], want[index])
		}
	}
}

func TestAdminListenAddressesSkipsExtrasForWildcard(t *testing.T) {
	tailscaleIPs := []netip.Addr{netip.MustParseAddr("100.92.41.120")}
	got := adminListenAddresses("0.0.0.0", 8787, tailscaleIPs)
	if len(got) != 1 || got[0] != "0.0.0.0:8787" {
		t.Fatalf("addresses = %v, want [0.0.0.0:8787]", got)
	}
}

func TestStartHTTPListenersServesPrimaryWhenOptionalAddressFails(t *testing.T) {
	server := newHTTPServer("127.0.0.1:0", http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	active, err := startHTTPListeners(server, []string{"127.0.0.1:0", "192.0.2.1:0"})
	if err != nil {
		t.Fatalf("startHTTPListeners() error = %v", err)
	}
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		_ = server.Shutdown(ctx)
	})
	if len(active) != 1 {
		t.Fatalf("active addresses = %v, want one primary listener", active)
	}

	response, err := http.Get("http://" + active[0])
	if err != nil {
		t.Fatalf("GET primary listener: %v", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusNoContent {
		t.Fatalf("status = %d, want %d", response.StatusCode, http.StatusNoContent)
	}
}

func TestStartHTTPListenersRequiresPrimaryAddress(t *testing.T) {
	server := newHTTPServer("192.0.2.1:0", http.NewServeMux())
	active, err := startHTTPListeners(server, []string{"192.0.2.1:0"})
	if err == nil {
		t.Fatal("startHTTPListeners() error = nil, want primary bind error")
	}
	if len(active) != 0 {
		t.Fatalf("active addresses = %v, want none", active)
	}
}

func testAddrs(cidr ...string) []net.Addr {
	result := make([]net.Addr, 0, len(cidr))
	for _, value := range cidr {
		ip, network, err := net.ParseCIDR(value)
		if err != nil {
			panic(err)
		}
		network.IP = ip
		result = append(result, network)
	}
	return result
}
