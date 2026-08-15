//go:build !linux

package main

import (
	_ "embed"
	"runtime"
	"sync"

	"github.com/getlantern/systray"
)

//go:embed assets/tray_windows.ico
var trayIconWindows []byte

//go:embed assets/tray.png
var trayIconPNG []byte

func runTray(appURL string, onOpen func(), onOpenChat func(), onStopServices func(), onExit func()) {
	var exitOnce sync.Once

	systray.Run(func() {
		if runtime.GOOS == "windows" {
			systray.SetIcon(trayIconWindows)
		} else {
			systray.SetIcon(trayIconPNG)
		}
		systray.SetTitle("GA")
		systray.SetTooltip("GenericAgent Admin")

		openItem := systray.AddMenuItem("打开 Admin", "Open GenericAgent Admin")
		chatItem := systray.AddMenuItem("打开 Chat", "Open GenericAgent Chat")
		stopItem := systray.AddMenuItem("停止所有服务", "Stop all managed services")
		systray.AddSeparator()
		exitItem := systray.AddMenuItem("退出 Admin", "Quit GenericAgent Admin")

		go func() {
			for {
				select {
				case <-openItem.ClickedCh:
					if onOpen != nil {
						go onOpen()
					}
				case <-chatItem.ClickedCh:
					if onOpenChat != nil {
						go onOpenChat()
					}
				case <-stopItem.ClickedCh:
					if onStopServices != nil {
						go onStopServices()
					}
				case <-exitItem.ClickedCh:
					exitOnce.Do(func() {
						if onExit != nil {
							onExit()
						}
						systray.Quit()
					})
					return
				}
			}
		}()
	}, func() {
		exitOnce.Do(func() {
			if onExit != nil {
				onExit()
			}
		})
	})
}
