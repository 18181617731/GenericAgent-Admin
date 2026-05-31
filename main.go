package main

import (
	"context"
	"embed"
	"fmt"
	"io/fs"
	"log"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"time"

	"genericagent-admin-go/internal/api"
	"genericagent-admin-go/internal/config"
	"genericagent-admin-go/internal/modelconfig"
	"genericagent-admin-go/internal/service"
)

//go:embed web/dist
var webFS embed.FS

func main() {
	cwd, err := appRoot()
	if err != nil {
		log.Fatal(err)
	}
	if err := os.Chdir(cwd); err != nil {
		log.Fatalf("chdir %s failed: %v", cwd, err)
	}
	cfgStore := config.NewStore(cwd)
	if err := cfgStore.Load(); err != nil {
		log.Printf("load config: %v", err)
	}
	svc := service.NewManager(cfgStore.Cfg.GARoot, cfgStore.Cfg.BufferLines)
	models := modelconfig.NewStore(cwd)
	static, err := fs.Sub(webFS, "web/dist")
	if err != nil {
		log.Fatal(err)
	}
	srv := api.New(cfgStore, svc, models, static)
	addr := fmt.Sprintf("%s:%d", cfgStore.Cfg.Host, cfgStore.Cfg.Port)
	url := "http://" + addr
	server := &http.Server{Addr: addr, Handler: srv.Routes()}
	go srv.StartAutostartServices()
	go func() { time.Sleep(500 * time.Millisecond); openBrowser(url) }()
	go func() {
		log.Printf("GenericAgent Admin Go listening on %s", url)
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("listen %s failed: %v; if the port is occupied, edit config.local.json and change port", addr, err)
		}
	}()
	stopPet := startDesktopPet()
	runTray(url,
		func() { openBrowser(url) },
		func() { openBrowser(url + "/chat") },
		func() { showDesktopPet() },
		func() { hideDesktopPet() },
		func() { srv.StopManagedServices() },
		func() {
			stopPet()
			srv.ShutdownCleanup()
			ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
			defer cancel()
			_ = server.Shutdown(ctx)
		},
	)
}

func appRoot() (string, error) {
	exe, err := os.Executable()
	if err != nil {
		return "", err
	}
	if exe != "" {
		return filepath.Dir(exe), nil
	}
	return os.Getwd()
}

func openBrowser(url string) {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "windows":
		cmd = exec.Command("rundll32", "url.dll,FileProtocolHandler", url)
	case "darwin":
		cmd = exec.Command("open", url)
	default:
		cmd = exec.Command("xdg-open", url)
	}
	_ = cmd.Start()
}
