package main

import (
	"fmt"
	"net"
	"strings"

	"genericagent-admin-go/internal/config"
)

const trayTooltipSeparator = " · "

// trayText is every word the tray can show, gathered per language so that a
// menu built for one locale cannot come out half-translated. Scope wording
// carries the security-relevant part: it always says whether a password stands
// between the network and a server that can run processes and edit files.
type trayText struct {
	AppName string

	OpenChat, OpenChatTip         string
	OpenSettings, OpenSettingsTip string
	Exit, ExitTip                 string

	CopyLocalTip, CopyLANTip, ScopeTip string
	Copied                             string

	// StopRunningFmt takes the number of live services. There is no wording for
	// none, because an idle entry leaves the menu rather than greying out.
	StopRunningFmt, StopTip string

	AddressLabel, LANLabel, ScopeLabel string

	ScopeLocalOnly, ScopeRemotePassword string
	ScopeRemoteAnonymous, ScopeUnknown  string

	PendingRestart, PendingNeedsPassword string
}

var trayZH = trayText{
	AppName: "GenericAgent Admin",

	OpenChat: "打开 Chat", OpenChatTip: "打开对话界面",
	OpenSettings: "打开设置", OpenSettingsTip: "打开管理台的设置页：远程访问与访问密码都在这里",
	Exit: "退出 Admin", ExitTip: "退出 GenericAgent Admin",

	CopyLocalTip: "点击复制本机地址",
	CopyLANTip:   "点击复制局域网地址",
	ScopeTip:     "在设置页修改远程访问",
	Copied:       "已复制 ",

	StopRunningFmt: "停止所有服务（%d 个运行中）",
	StopTip:        "停止所有由 Admin 托管的服务",

	AddressLabel: "地址：", LANLabel: "局域网：", ScopeLabel: "访问范围：",

	ScopeLocalOnly:       "仅本机",
	ScopeRemotePassword:  "远程已开放 · 需密码",
	ScopeRemoteAnonymous: "远程已开放 · 无需密码",
	ScopeUnknown:         "未知",

	PendingRestart:       "（设置已改，重启后生效）",
	PendingNeedsPassword: "（远程访问待设置密码）",
}

var trayEN = trayText{
	AppName: "GenericAgent Admin",

	OpenChat: "Open Chat", OpenChatTip: "Open the chat interface",
	OpenSettings: "Open Settings", OpenSettingsTip: "Open the admin settings page, where remote access and the access password live",
	Exit: "Quit Admin", ExitTip: "Quit GenericAgent Admin",

	CopyLocalTip: "Click to copy the local address",
	CopyLANTip:   "Click to copy the LAN address",
	ScopeTip:     "Change remote access on the settings page",
	Copied:       "Copied ",

	StopRunningFmt: "Stop all services (%d running)",
	StopTip:        "Stop every service the Admin manages",

	AddressLabel: "Address: ", LANLabel: "LAN: ", ScopeLabel: "Access: ",

	ScopeLocalOnly:       "this machine only",
	ScopeRemotePassword:  "remote open · password required",
	ScopeRemoteAnonymous: "remote open · no password",
	ScopeUnknown:         "unknown",

	PendingRestart:       " (setting changed, restart to apply)",
	PendingNeedsPassword: " (remote access still needs a password)",
}

// stopServicesLabel keeps the entry honest about what clicking it would end.
// The tray only shows the entry while something is running.
func stopServicesLabel(text trayText, running int) string {
	return fmt.Sprintf(text.StopRunningFmt, running)
}

// trayApp is what the tray can do on behalf of the running admin server.
type trayApp struct {
	OpenChat     func()
	OpenSettings func()
	StopServices func()
	Exit         func()
	// Status and RunningServices are polled while the process runs; both must
	// be cheap and safe to call from the tray goroutine.
	Status          func() trayStatus
	RunningServices func() int
}

func (a trayApp) status() trayStatus {
	if a.Status == nil {
		return trayStatus{}
	}
	return a.Status()
}

func (a trayApp) runningServices() int {
	if a.RunningServices == nil {
		return 0
	}
	return a.RunningServices()
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
	// ScopeAlert marks a scope the user should see in the menu: reachable from
	// the network, or out of step with the config. A plain local-only server is
	// the safe default and says so in the tooltip instead of taking a row.
	ScopeAlert bool
	Tooltip    string
}

// describeTrayStatus derives the menu text from the address the server really
// bound, not from the config: a launch that was downgraded to loopback must
// not advertise itself as reachable from the network.
func describeTrayStatus(listenAddr string, cfg config.AppConfig, passwordSet bool, lan func() string, text trayText) trayStatus {
	host, port, err := net.SplitHostPort(listenAddr)
	if err != nil {
		return trayStatus{
			LocalLabel: text.AddressLabel + listenAddr,
			ScopeLabel: text.ScopeLabel + text.ScopeUnknown,
			ScopeAlert: true,
			Tooltip:    text.AppName,
		}
	}

	published := !config.IsLoopbackHost(host)
	localAddr := net.JoinHostPort("127.0.0.1", port)
	status := trayStatus{
		LocalURL:   "http://" + localAddr,
		LocalLabel: text.AddressLabel + localAddr,
	}
	if published {
		if ip := strings.TrimSpace(lanAddress(lan)); ip != "" {
			lanAddr := net.JoinHostPort(ip, port)
			status.LANURL = "http://" + lanAddr
			status.LANLabel = text.LANLabel + lanAddr
		}
	}

	scope := text.ScopeLocalOnly
	if published {
		scope = text.ScopeRemotePassword
		if cfg.RemoteAllowAnonymous {
			scope = text.ScopeRemoteAnonymous
		}
	}
	pending := pendingRemoteChange(published, cfg, passwordSet, text)
	scope += pending
	status.ScopeLabel = text.ScopeLabel + scope
	status.ScopeAlert = published || pending != ""

	reachable := localAddr
	if status.LANURL != "" {
		reachable = strings.TrimPrefix(status.LANURL, "http://")
	}
	status.Tooltip = text.AppName + trayTooltipSeparator + reachable + trayTooltipSeparator + scope
	return status
}

// pendingRemoteChange explains a mismatch between the live config and the
// socket this process is stuck with until it restarts.
func pendingRemoteChange(published bool, cfg config.AppConfig, passwordSet bool, text trayText) string {
	if cfg.RemoteAccess && !cfg.RemoteAllowAnonymous && !passwordSet && !published {
		return text.PendingNeedsPassword
	}
	if wantsPublished := cfg.RemoteAccess; wantsPublished != published {
		return text.PendingRestart
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
