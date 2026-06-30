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
	"syscall"
	"time"

	"genericagent-admin-go/internal/api"
	"genericagent-admin-go/internal/config"
	"genericagent-admin-go/internal/modelconfig"
	"genericagent-admin-go/internal/service"
	"genericagent-admin-go/internal/version"
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
	if launch.PortSet {
		cfgStore.Cfg.Port = launch.Port
	}
	version.SetRepoURL(cfgStore.Cfg.UpdateRepoURL)
	svc := service.NewManagerWithPython(cfgStore.Cfg.GARoot, cfgStore.Cfg.EffectivePython, cfgStore.Cfg.BufferLines)
	models := modelconfig.NewStore(cwd)
	static, err := fs.Sub(webFS, "web/dist")
	if err != nil {
		log.Fatal(err)
	}
	srv := api.New(cfgStore, svc, models, static)
	addr := fmt.Sprintf("%s:%d", cfgStore.Cfg.Host, cfgStore.Cfg.Port)
	url := "http://" + addr
	server := newHTTPServer(addr, srv.Routes())
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

type launchOptions struct {
	Headless  bool
	NoBrowser bool
	Port      int
	PortSet   bool
}

const (
	adminReadHeaderTimeout = 10 * time.Second
	adminIdleTimeout       = 120 * time.Second
)

func newHTTPServer(addr string, handler http.Handler) *http.Server {
	return &http.Server{
		Addr:              addr,
		Handler:           handler,
		ReadHeaderTimeout: adminReadHeaderTimeout,
		IdleTimeout:       adminIdleTimeout,
	}
}

func parseLaunchOptions() launchOptions {
	headlessFlag := flag.Bool("headless", false, "run without browser or tray; intended for Linux servers")
	serverOnlyFlag := flag.Bool("server-only", false, "alias for --headless")
	noBrowserFlag := flag.Bool("no-browser", false, "do not open the web UI automatically")
	portFlag := flag.Int("port", 0, "override HTTP listen port for this launch (1-65535)")
	flag.Parse()

	portSet := false
	flag.Visit(func(f *flag.Flag) {
		if f.Name == "port" {
			portSet = true
		}
	})
	if portSet && (*portFlag < 1 || *portFlag > 65535) {
		log.Fatalf("invalid --port %d: port must be between 1 and 65535", *portFlag)
	}

	headless := *headlessFlag || *serverOnlyFlag || envBool("GA_ADMIN_HEADLESS") || envBool("GA_ADMIN_SERVER_ONLY")
	if !headless && runtime.GOOS == "linux" && !hasGraphicalSession() {
		headless = true
		log.Printf("no Linux graphical session detected; enabling headless/server-only mode")
	}
	return launchOptions{
		Headless:  headless,
		NoBrowser: *noBrowserFlag || envBool("GA_ADMIN_NO_BROWSER"),
		Port:      *portFlag,
		PortSet:   portSet,
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
		// state such as config.local.json anchored to the caller's working tree
		// instead of the ephemeral compiled exe path.
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
