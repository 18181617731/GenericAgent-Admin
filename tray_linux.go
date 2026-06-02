//go:build linux

package main

import (
	"log"
	"os"
	"os/signal"
	"syscall"
)

func runTray(appURL string, onOpen func(), onOpenChat func(), onShowPet func(), onHidePet func(), onStopServices func(), onExit func()) {
	log.Printf("system tray is not available in the Linux build; server stays running until SIGINT/SIGTERM")
	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)
	<-stop
	if onExit != nil {
		onExit()
	}
}
