//go:build linux

package tray

import (
	"log"
	"os"
	"os/signal"
	"syscall"
)

// Run has no menu to put on screen here, so it reports the address to the log
// and blocks until the process is signalled.
func Run(app App) {
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
