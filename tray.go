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

// The bound address is fixed for the process lifetime, but remote access, the
// password, and the set of running services all change while the process runs,
// so the menu re-reads them instead of rendering once at startup.
const trayRefreshInterval = 3 * time.Second

func runTray(app trayApp) {
	text := trayLanguage()
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
		systray.SetTooltip(text.AppName)

		openItem := systray.AddMenuItem(text.OpenAdmin, text.OpenAdminTip)
		chatItem := systray.AddMenuItem(text.OpenChat, text.OpenChatTip)
		settingsItem := systray.AddMenuItem(text.OpenSettings, text.OpenSettingsTip)
		systray.AddSeparator()
		// The default listen port is random, so the tray is the only place a
		// user can read, and take, the address of the running server.
		localItem := systray.AddMenuItem("", text.CopyLocalTip)
		lanItem := systray.AddMenuItem("", text.CopyLANTip)
		scopeItem := systray.AddMenuItem("", text.ScopeTip)
		scopeItem.Disable()
		systray.AddSeparator()
		stopItem := systray.AddMenuItem("", text.StopTip)
		systray.AddSeparator()
		exitItem := systray.AddMenuItem(text.Exit, text.ExitTip)

		status := app.status()
		render := func() {
			status = app.status()
			localItem.SetTitle(status.LocalLabel)
			scopeItem.SetTitle(status.ScopeLabel)
			systray.SetTooltip(status.Tooltip)

			running := app.runningServices()
			stopItem.SetTitle(stopServicesLabel(text, running))
			// Nothing to stop is worth saying with the entry itself rather
			// than with a click that would do nothing.
			if running > 0 {
				stopItem.Enable()
			} else {
				stopItem.Disable()
			}

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
			// A copy changes nothing on screen, so the entry acknowledges it
			// until the next refresh restores the address.
			item.SetTitle(text.Copied + url)
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
