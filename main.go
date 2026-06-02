package main

import (
	"context"
	"embed"
	"flag"
	"fmt"
	"io/fs"
	"log"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"syscall"
	"time"

	"genericagent-admin-go/internal/api"
	"genericagent-admin-go/internal/config"
	"genericagent-admin-go/internal/modelconfig"
	"genericagent-admin-go/internal/service"
)

//go:embed web/dist
var webFS embed.FS

func main() {
	launch := parseLaunchOptions()
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
	petState := newPetActivityState()
	srv.PetEvent = petState.handle
	srv.PetSwitch = switchDesktopPet
	addr := fmt.Sprintf("%s:%d", cfgStore.Cfg.Host, cfgStore.Cfg.Port)
	url := "http://" + addr
	server := &http.Server{Addr: addr, Handler: srv.Routes()}
	go srv.StartAutostartServices()
	go func() {
		log.Printf("GenericAgent Admin Go listening on %s", url)
		if launch.Headless {
			log.Printf("headless/server-only mode enabled; open %s from another browser if needed", url)
		}
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("listen %s failed: %v; if the port is occupied, edit config.local.json and change port", addr, err)
		}
	}()

	if launch.Headless {
		waitForShutdownSignal(server, srv.ShutdownCleanup)
		return
	}

	if !launch.NoBrowser {
		go func() { time.Sleep(500 * time.Millisecond); openBrowser(url) }()
	}
	if activePet := srv.ActivePetID(); activePet != "" {
		if err := switchDesktopPet(activePet); err != nil {
			log.Printf("load active desktop pet %q failed: %v", activePet, err)
		}
	}
	stopPet := startDesktopPet(func() { openBrowser(url + "/chat") })
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

type launchOptions struct {
	Headless  bool
	NoBrowser bool
}

func parseLaunchOptions() launchOptions {
	headlessFlag := flag.Bool("headless", false, "run without browser, tray, or desktop pet; intended for Linux servers")
	serverOnlyFlag := flag.Bool("server-only", false, "alias for --headless")
	noBrowserFlag := flag.Bool("no-browser", false, "do not open the web UI automatically")
	flag.Parse()

	headless := *headlessFlag || *serverOnlyFlag || envBool("GA_ADMIN_HEADLESS") || envBool("GA_ADMIN_SERVER_ONLY")
	if !headless && runtime.GOOS == "linux" && !hasGraphicalSession() {
		headless = true
		log.Printf("no Linux graphical session detected; enabling headless/server-only mode")
	}
	return launchOptions{
		Headless:  headless,
		NoBrowser: *noBrowserFlag || envBool("GA_ADMIN_NO_BROWSER"),
	}
}

func envBool(name string) bool {
	value := strings.TrimSpace(strings.ToLower(os.Getenv(name)))
	switch value {
	case "1", "true", "yes", "y", "on":
		return true
	default:
		return false
	}
}

func hasGraphicalSession() bool {
	for _, name := range []string{"DISPLAY", "WAYLAND_DISPLAY", "MIR_SOCKET"} {
		if strings.TrimSpace(os.Getenv(name)) != "" {
			return true
		}
	}
	return false
}

func waitForShutdownSignal(server *http.Server, cleanup func()) {
	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)
	<-stop
	log.Printf("shutdown signal received; stopping GenericAgent Admin Go")
	if cleanup != nil {
		cleanup()
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	_ = server.Shutdown(ctx)
}

var (
	desktopPetAction         = setDesktopPetAction
	desktopPetActionForTicks = setDesktopPetActionForTicks
)

type petActivityState struct {
	mu      sync.Mutex
	running map[string]bool
}

func newPetActivityState() *petActivityState {
	return &petActivityState{running: map[string]bool{}}
}

func (p *petActivityState) handle(event string) {
	p.mu.Lock()
	defer p.mu.Unlock()

	switch event {
	case "chat:start":
		p.markRunning("chat", true)
		desktopPetAction(petActionRunning)
	case "goal:start", "goal:active":
		p.markRunning("goal", true)
		desktopPetAction(petActionRunning)
	case "react:start":
		p.markRunning("react", true)
		desktopPetAction(petActionRunning)
	case "chat:done", "goal:review", "react:review":
		p.markDomainDone(event)
		p.restoreBaseLocked()
		desktopPetActionForTicks(petActionReview, 30)
	case "chat:cancel", "goal:stop", "react:stop":
		p.markDomainDone(event)
		p.restoreBaseLocked()
		desktopPetActionForTicks(petActionWaiting, 20)
	case "goal:idle":
		p.markRunning("goal", false)
		p.restoreBaseLocked()
	case "service:start", "tmwebdriver:start":
		desktopPetActionForTicks(petActionRunningRight, 18)
	case "service:stop", "service:stop_all":
		desktopPetActionForTicks(petActionRunningLeft, 18)
	case "service:autostart", "tmwebdriver:ready":
		desktopPetActionForTicks(petActionWaving, 20)
	case "service:error", "react:error", "goal:error", "chat:error", "tmwebdriver:error":
		p.markDomainDone(event)
		p.restoreBaseLocked()
		desktopPetActionForTicks(petActionFailed, 24)
	}
}

func (p *petActivityState) markRunning(domain string, running bool) {
	if running {
		p.running[domain] = true
	} else {
		delete(p.running, domain)
	}
}

func (p *petActivityState) markDomainDone(event string) {
	for _, domain := range []string{"chat", "goal", "react"} {
		if len(event) >= len(domain) && event[:len(domain)] == domain {
			p.markRunning(domain, false)
			return
		}
	}
}

func (p *petActivityState) restoreBaseLocked() {
	if len(p.running) > 0 {
		desktopPetAction(petActionRunning)
		return
	}
	desktopPetAction(petActionIdle)
}

func appRoot() (string, error) {
	wd, wdErr := os.Getwd()
	exe, err := os.Executable()
	if err != nil {
		if wdErr == nil {
			return wd, nil
		}
		return "", err
	}
	if exe != "" {
		exeDir := filepath.Dir(exe)
		// `go run` executes from a temporary go-build directory. Keep runtime
		// state such as config.local.json and pets.local.json anchored to the
		// caller's working tree instead of the ephemeral compiled exe path.
		if wdErr == nil && wd != "" && strings.Contains(strings.ToLower(exeDir), string(filepath.Separator)+"go-build") {
			return wd, nil
		}
		return exeDir, nil
	}
	return wd, wdErr
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
	hideChildWindow(cmd)
	_ = cmd.Start()
}
