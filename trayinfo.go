package main

import (
	"net"
	"strings"

	"genericagent-admin-go/internal/config"
)

// Scope wording for the tray. Remote access is the security-relevant state, so
// the label always says whether a password stands between the network and a
// server that can run processes and edit files.
const (
	scopeLocalOnly       = "仅本机"
	scopeRemotePassword  = "远程已开放 · 需密码"
	scopeRemoteAnonymous = "远程已开放 · 无需密码"
	scopeUnknown         = "未知"

	pendingRestart        = "（设置已改，重启后生效）"
	pendingNeedsPassword  = "（远程访问待设置密码）"
	trayAddressLabel      = "地址："
	trayLANLabel          = "局域网："
	trayScopeLabel        = "访问范围："
	trayTooltipAppName    = "GenericAgent Admin"
	trayTooltipSeparator  = " · "
	trayUnknownAddrPrefix = trayAddressLabel
)

// trayApp is what the tray can do on behalf of the running admin server.
type trayApp struct {
	OpenAdmin    func()
	OpenChat     func()
	OpenSettings func()
	StopServices func()
	Exit         func()
	// Status is polled while the menu is open; it must be cheap and safe to
	// call from the tray goroutine.
	Status func() trayStatus
}

func (a trayApp) status() trayStatus {
	if a.Status == nil {
		return trayStatus{}
	}
	return a.Status()
}

// trayStatus is everything the tray menu says about where this process can be
// reached. The bound address cannot change while the process runs, but the
// config behind it can, so the scope line also reports pending changes.
type trayStatus struct {
	LocalURL   string
	LocalLabel string
	// LANURL and LANLabel are empty unless the server is actually published
	// and a routable address was found.
	LANURL     string
	LANLabel   string
	ScopeLabel string
	Tooltip    string
}

// describeTrayStatus derives the menu text from the address the server really
// bound, not from the config: a launch that was downgraded to loopback must
// not advertise itself as reachable from the network.
func describeTrayStatus(listenAddr string, cfg config.AppConfig, passwordSet bool, lan func() string) trayStatus {
	host, port, err := net.SplitHostPort(listenAddr)
	if err != nil {
		return trayStatus{
			LocalLabel: trayUnknownAddrPrefix + listenAddr,
			ScopeLabel: trayScopeLabel + scopeUnknown,
			Tooltip:    trayTooltipAppName,
		}
	}

	published := !config.IsLoopbackHost(host)
	localAddr := net.JoinHostPort("127.0.0.1", port)
	status := trayStatus{
		LocalURL:   "http://" + localAddr,
		LocalLabel: trayAddressLabel + localAddr,
	}
	if published {
		if ip := strings.TrimSpace(lanAddress(lan)); ip != "" {
			lanAddr := net.JoinHostPort(ip, port)
			status.LANURL = "http://" + lanAddr
			status.LANLabel = trayLANLabel + lanAddr
		}
	}

	scope := scopeLocalOnly
	if published {
		scope = scopeRemotePassword
		if cfg.RemoteAllowAnonymous {
			scope = scopeRemoteAnonymous
		}
	}
	if pending := pendingRemoteChange(published, cfg, passwordSet); pending != "" {
		scope += pending
	}
	status.ScopeLabel = trayScopeLabel + scope

	reachable := localAddr
	if status.LANURL != "" {
		reachable = strings.TrimPrefix(status.LANURL, "http://")
	}
	status.Tooltip = trayTooltipAppName + trayTooltipSeparator + reachable + trayTooltipSeparator + scope
	return status
}

// pendingRemoteChange explains a mismatch between the live config and the
// socket this process is stuck with until it restarts.
func pendingRemoteChange(published bool, cfg config.AppConfig, passwordSet bool) string {
	if cfg.RemoteAccess && !cfg.RemoteAllowAnonymous && !passwordSet && !published {
		return pendingNeedsPassword
	}
	if wantsPublished := cfg.RemoteAccess; wantsPublished != published {
		return pendingRestart
	}
	return ""
}

func lanAddress(lan func() string) string {
	if lan == nil {
		return ""
	}
	return lan()
}

// primaryLANAddress picks the IPv4 address other devices would use to reach
// this machine. Interfaces that are down, loopback, or self-assigned cannot
// serve that purpose and are skipped.
func primaryLANAddress() string {
	interfaces, err := net.Interfaces()
	if err != nil {
		return ""
	}
	for _, iface := range interfaces {
		if iface.Flags&net.FlagUp == 0 || iface.Flags&net.FlagLoopback != 0 {
			continue
		}
		addrs, err := iface.Addrs()
		if err != nil {
			continue
		}
		for _, addr := range addrs {
			ipNet, ok := addr.(*net.IPNet)
			if !ok {
				continue
			}
			ip := ipNet.IP.To4()
			if ip == nil || !ip.IsGlobalUnicast() || ip.IsLinkLocalUnicast() {
				continue
			}
			return ip.String()
		}
	}
	return ""
}
