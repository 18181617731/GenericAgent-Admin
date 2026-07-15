package main

import (
	"fmt"
	"log"
	"net"
	"net/http"
	"net/netip"
	"sort"
	"strconv"
	"strings"
)

var tailscaleIPv4Prefix = netip.MustParsePrefix("100.64.0.0/10")

type networkInterface struct {
	name  string
	flags net.Flags
	addrs []net.Addr
}

func discoverTailscaleIPv4() []netip.Addr {
	interfaces, err := net.Interfaces()
	if err != nil {
		log.Printf("discover Tailscale addresses: %v", err)
		return nil
	}

	candidates := make([]networkInterface, 0, len(interfaces))
	for _, iface := range interfaces {
		addrs, addrErr := iface.Addrs()
		if addrErr != nil {
			log.Printf("inspect network interface %s: %v", iface.Name, addrErr)
			continue
		}
		candidates = append(candidates, networkInterface{iface.Name, iface.Flags, addrs})
	}
	return tailscaleIPv4FromInterfaces(candidates)
}

func tailscaleIPv4FromInterfaces(interfaces []networkInterface) []netip.Addr {
	unique := make(map[netip.Addr]struct{})
	for _, iface := range interfaces {
		if iface.flags&net.FlagUp == 0 || iface.flags&net.FlagLoopback != 0 {
			continue
		}
		for _, address := range iface.addrs {
			if ip, ok := networkIPv4(address); ok && tailscaleIPv4Prefix.Contains(ip) {
				unique[ip] = struct{}{}
			}
		}
	}

	result := make([]netip.Addr, 0, len(unique))
	for ip := range unique {
		result = append(result, ip)
	}
	sort.Slice(result, func(i, j int) bool { return result[i].Less(result[j]) })
	return result
}

func networkIPv4(address net.Addr) (netip.Addr, bool) {
	var raw string
	switch value := address.(type) {
	case *net.IPNet:
		raw = value.IP.String()
	case *net.IPAddr:
		raw = value.IP.String()
	default:
		raw = address.String()
		if host, _, err := net.SplitHostPort(raw); err == nil {
			raw = host
		} else if prefix, prefixErr := netip.ParsePrefix(raw); prefixErr == nil {
			raw = prefix.Addr().String()
		}
	}
	ip, err := netip.ParseAddr(strings.TrimSpace(raw))
	return ip.Unmap(), err == nil && ip.Is4()
}

func adminListenAddresses(host string, port int, tailscaleIPs []netip.Addr) []string {
	portText := strconv.Itoa(port)
	primary := net.JoinHostPort(strings.TrimSpace(host), portText)
	result := []string{primary}
	if ip, err := netip.ParseAddr(strings.TrimSpace(host)); err == nil && ip.IsUnspecified() {
		return result
	}

	seen := map[string]struct{}{primary: {}}
	for _, ip := range tailscaleIPs {
		addr := net.JoinHostPort(ip.String(), portText)
		if _, exists := seen[addr]; exists {
			continue
		}
		seen[addr] = struct{}{}
		result = append(result, addr)
	}
	return result
}

func startHTTPListeners(server *http.Server, addrs []string) ([]string, error) {
	if len(addrs) == 0 {
		return nil, fmt.Errorf("no HTTP listen addresses configured")
	}
	active := make([]string, 0, len(addrs))
	for index, addr := range addrs {
		listener, err := net.Listen("tcp", addr)
		if err != nil {
			if index == 0 {
				return nil, fmt.Errorf("listen %s: %w", addr, err)
			}
			log.Printf("Tailscale listener %s unavailable: %v", addr, err)
			continue
		}
		active = append(active, listener.Addr().String())
		go serveHTTP(server, listener)
	}
	return active, nil
}

func serveHTTP(server *http.Server, listener net.Listener) {
	if err := server.Serve(listener); err != nil && err != http.ErrServerClosed {
		log.Printf("HTTP listener %s stopped: %v", listener.Addr(), err)
	}
}

func logListenURLs(addrs []string) {
	for _, addr := range addrs {
		log.Printf("GenericAgent Admin Go listening on http://%s", addr)
	}
}
