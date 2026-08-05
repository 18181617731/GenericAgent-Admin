package main

import (
	"context"
	"embed"
	"flag"
	"fmt"
	"io/fs"
	"log"
	"net"
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
	cwd, err := appRoot(launch.AppRoot)
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
	// Portable bundle auto-init: detect and run bootstrap.py to populate config
	if !cfgStore.Cfg.BootstrapDone {
		if err := tryPortableAutoInit(cwd, cfgStore); err != nil {
			log.Printf("portable auto-init: %v", err)
		}
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
	srv.StartAutomaticChatTitleBackfill()
	auth, err := newAuthManager(cwd, os.Getenv("GA_ADMIN_AUTH_USER"), os.Getenv("GA_ADMIN_AUTH_PASSWORD"))
	if err != nil {
		log.Fatalf("initialize admin authentication: %v", err)
	}
	addr := fmt.Sprintf("%s:%d", cfgStore.Cfg.Host, cfgStore.Cfg.Port)
	url := "http://" + addr
	server := newHTTPServer(addr, auth.middleware(srv.Routes()))
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
	AppRoot   string
	Port      int
	PortSet   bool
}

const (
	adminReadHeaderTimeout = 10 * time.Second
	adminIdleTimeout       = 120 * time.Second
	authUserEnv            = "GA_ADMIN_AUTH_USER"
	authPasswordEnv        = "GA_ADMIN_AUTH_PASSWORD"
)

func newHTTPServer(addr string, handler http.Handler) *http.Server {
	return &http.Server{
		Addr:              addr,
		Handler:           handler,
		ReadHeaderTimeout: adminReadHeaderTimeout,
		IdleTimeout:       adminIdleTimeout,
	}
}

func isIPv4LoopbackRemote(remoteAddr string) bool {
	host, _, err := net.SplitHostPort(remoteAddr)
	if err != nil {
		return false
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.To4() != nil && ip.IsLoopback()
}

func parseLaunchOptions() launchOptions {
	headlessFlag := flag.Bool("headless", false, "run without browser or tray; intended for Linux servers")
	serverOnlyFlag := flag.Bool("server-only", false, "alias for --headless")
	noBrowserFlag := flag.Bool("no-browser", false, "do not open the web UI automatically")
	appRootFlag := flag.String("app-root", "", "override the directory containing config.local.json")
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
		AppRoot:   strings.TrimSpace(*appRootFlag),
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

func appRoot(explicitRoot string) (string, error) {
	if root := strings.TrimSpace(explicitRoot); root != "" {
		return filepath.Abs(root)
	}

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

// tryPortableAutoInit detects portable bundle environment and runs bootstrap.py
// to populate config with correct paths. Returns nil if not portable or on success.
func tryPortableAutoInit(cwd string, store *config.Store) error {
	// Check for portable bundle markers
	gaRoot := filepath.Join(cwd, "GenericAgent")
	bootstrapPy := filepath.Join(gaRoot, "bootstrap.py")
	
	if _, err := os.Stat(bootstrapPy); os.IsNotExist(err) {
		return nil // Not a portable bundle
	}
	if _, err := os.Stat(gaRoot); os.IsNotExist(err) {
		return nil // Not a portable bundle
	}
	
	log.Printf("portable bundle detected; running bootstrap auto-init")
	
	// Find bundled Python interpreter
	var pythonExe string
	if runtime.GOOS == "windows" {
		pythonExe = filepath.Join(gaRoot, "python", "python.exe")
	} else {
		pythonExe = filepath.Join(gaRoot, "python", "bin", "python3")
	}
	
	if _, err := os.Stat(pythonExe); os.IsNotExist(err) {
		return fmt.Errorf("bundled python not found at %s", pythonExe)
	}
	
	// Run bootstrap.py with bundled Python
	cmd := exec.Command(pythonExe, bootstrapPy)
	cmd.Dir = cwd
	output, err := cmd.CombinedOutput()
	if err != nil {
		log.Printf("bootstrap.py output:\n%s", string(output))
		return fmt.Errorf("bootstrap.py failed: %w", err)
	}
	
	log.Printf("bootstrap.py completed successfully")
	
	// Reload config to pick up bootstrap-populated paths
	if err := store.Load(); err != nil {
		return fmt.Errorf("reload config after bootstrap: %w", err)
	}
	
	// Mark bootstrap as done
	store.Cfg.BootstrapDone = true
	if err := store.Save(store.Cfg); err != nil {
		return fmt.Errorf("save bootstrap_done flag: %w", err)
	}
	
	log.Printf("portable auto-init completed: ga_root=%s", store.Cfg.GARoot)
	return nil
}
