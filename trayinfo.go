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

	OpenAdmin, OpenAdminTip       string
	OpenChat, OpenChatTip         string
	OpenSettings, OpenSettingsTip string
	Exit, ExitTip                 string

	CopyLocalTip, CopyLANTip, ScopeTip string
	Copied                             string

	// StopRunningFmt takes the number of live services; StopIdle covers none.
	StopIdle, StopRunningFmt, StopTip string

	AddressLabel, LANLabel, ScopeLabel string

	ScopeLocalOnly, ScopeRemotePassword string
	ScopeRemoteAnonymous, ScopeUnknown  string

	PendingRestart, PendingNeedsPassword string
}

var trayZH = trayText{
	AppName: "GenericAgent Admin",

	OpenAdmin: "打开 Admin", OpenAdminTip: "打开管理界面",
	OpenChat: "打开 Chat", OpenChatTip: "打开对话界面",
	OpenSettings: "打开设置", OpenSettingsTip: "远程访问与访问密码都在设置页",
	Exit: "退出 Admin", ExitTip: "退出 GenericAgent Admin",

	CopyLocalTip: "点击复制本机地址",
	CopyLANTip:   "点击复制局域网地址",
	ScopeTip:     "在设置页修改远程访问",
	Copied:       "已复制 ",

	StopIdle:       "停止所有服务（无运行中）",
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

	OpenAdmin: "Open Admin", OpenAdminTip: "Open the admin interface",
	OpenChat: "Open Chat", OpenChatTip: "Open the chat interface",
	OpenSettings: "Open Settings", OpenSettingsTip: "Remote access and the access password live on the settings page",
	Exit: "Quit Admin", ExitTip: "Quit GenericAgent Admin",

	CopyLocalTip: "Click to copy the local address",
	CopyLANTip:   "Click to copy the LAN address",
	ScopeTip:     "Change remote access on the settings page",
	Copied:       "Copied ",

	StopIdle:       "Stop all services (none running)",
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
func stopServicesLabel(text trayText, running int) string {
	if running <= 0 {
		return text.StopIdle
	}
	return fmt.Sprintf(text.StopRunningFmt, running)
}

// trayApp is what the tray can do on behalf of the running admin server.
type trayApp struct {
	OpenAdmin    func()
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
	if pending := pendingRemoteChange(published, cfg, passwordSet, text); pending != "" {
		scope += pending
	}
	status.ScopeLabel = text.ScopeLabel + scope

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
