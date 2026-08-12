//go:build linux

package main

import (
	"log"
	"os"
	"os/signal"
	"syscall"
)

func runTray(app trayApp) {
	log.Printf("system tray is not available in the Linux build; server stays running until SIGINT/SIGTERM")
	if status := app.status(); status.LocalLabel != "" {
		log.Printf("%s | %s", status.LocalLabel, status.ScopeLabel)
	}
	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)
	<-stop
	if app.Exit != nil {
		app.Exit()
	}
}
