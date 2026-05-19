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
	cwd, err := os.Getwd()
	if err != nil {
		log.Fatal(err)
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
	go func() { time.Sleep(500 * time.Millisecond); openBrowser(url) }()
	go func() {
		log.Printf("GenericAgent Admin Go listening on %s", url)
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("listen %s failed: %v; if the port is occupied, edit config.local.json and change port", addr, err)
		}
	}()
	runTray(url,
		func() { openBrowser(url) },
		func() { openBrowser(url + "/chat") },
		func() { srv.StopManagedServices() },
		func() {
			srv.ShutdownCleanup()
			ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
			defer cancel()
			_ = server.Shutdown(ctx)
		},
	)
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
