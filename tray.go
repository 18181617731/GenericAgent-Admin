//go:build !linux

package main

import (
	_ "embed"
	"log"
	"runtime"
	"sync"
	"time"

	"github.com/getlantern/systray"
)

// One icon serves the tray, the taskbar, and every desktop window.
//
//go:embed assets/tray_windows.ico
var appIconICO []byte

//go:embed assets/tray.png
var appIconPNG []byte

// The bound address is fixed for the process lifetime, but remote access and
// the password can be changed from the settings page while it runs, so the
// menu re-reads its status instead of rendering once at startup.
const trayRefreshInterval = 3 * time.Second

// copiedFeedbackTitle briefly replaces an address entry so that a copy, which
// otherwise changes nothing on screen, is visibly acknowledged.
const copiedFeedback = "已复制 "

func runTray(app trayApp) {
	var exitOnce sync.Once
	quit := func() {
		exitOnce.Do(func() {
			if app.Exit != nil {
				app.Exit()
			}
		})
	}

	systray.Run(func() {
		if runtime.GOOS == "windows" {
			systray.SetIcon(appIconICO)
		} else {
			systray.SetIcon(appIconPNG)
		}
		systray.SetTitle("GA")
		systray.SetTooltip(trayTooltipAppName)

		openItem := systray.AddMenuItem("打开 Admin", "打开管理界面")
		chatItem := systray.AddMenuItem("打开 Chat", "打开对话界面")
		settingsItem := systray.AddMenuItem("打开设置", "远程访问与访问密码都在设置页")
		systray.AddSeparator()
		// The default listen port is random, so the tray is the only place a
		// user can read, and take, the address of the running server.
		localItem := systray.AddMenuItem("", "点击复制本机地址")
		lanItem := systray.AddMenuItem("", "点击复制局域网地址")
		scopeItem := systray.AddMenuItem("", "在设置页修改远程访问")
		scopeItem.Disable()
		systray.AddSeparator()
		stopItem := systray.AddMenuItem("停止所有服务", "停止所有由 Admin 托管的服务")
		systray.AddSeparator()
		exitItem := systray.AddMenuItem("退出 Admin", "退出 GenericAgent Admin")

		status := app.status()
		render := func() {
			status = app.status()
			localItem.SetTitle(status.LocalLabel)
			scopeItem.SetTitle(status.ScopeLabel)
			systray.SetTooltip(status.Tooltip)
			if status.LANLabel == "" {
				lanItem.Hide()
				return
			}
			lanItem.SetTitle(status.LANLabel)
			lanItem.Show()
		}
		render()

		copyAddress := func(item *systray.MenuItem, url string) {
			if url == "" {
				return
			}
			if err := copyToClipboard(url); err != nil {
				log.Printf("tray: %v", err)
				return
			}
			item.SetTitle(copiedFeedback + url)
		}

		go func() {
			// One goroutine owns every menu item, so the periodic refresh
			// cannot race the click handlers that also rewrite titles.
			refresh := time.NewTicker(trayRefreshInterval)
			defer refresh.Stop()
			for {
				select {
				case <-refresh.C:
					render()
				case <-openItem.ClickedCh:
					run(app.OpenAdmin)
				case <-chatItem.ClickedCh:
					run(app.OpenChat)
				case <-settingsItem.ClickedCh:
					run(app.OpenSettings)
				case <-localItem.ClickedCh:
					copyAddress(localItem, status.LocalURL)
				case <-lanItem.ClickedCh:
					copyAddress(lanItem, status.LANURL)
				case <-stopItem.ClickedCh:
					run(app.StopServices)
				case <-exitItem.ClickedCh:
					quit()
					systray.Quit()
					return
				}
			}
		}()
	}, quit)
}

// run keeps a click from blocking the menu on work that may open a window or
// stop a process tree.
func run(action func()) {
	if action != nil {
		go action()
	}
}
