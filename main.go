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
	"genericagent-admin-go/internal/autostart"
	"genericagent-admin-go/internal/config"
	"genericagent-admin-go/internal/modelconfig"
	"genericagent-admin-go/internal/service"
	"genericagent-admin-go/internal/version"
)

//go:embed web/dist
var webFS embed.FS

// The Windows executable carries the app icon in a resource section so that
// Explorer, the taskbar, and pinned shortcuts show it. The .syso files are
// committed because every build path needs them, including a plain `go build`;
// regenerate them after changing the icon:
//
//go:generate go run github.com/akavel/rsrc@v0.10.2 -ico assets/tray_windows.ico -arch amd64 -o rsrc_windows_amd64.syso
//go:generate go run github.com/akavel/rsrc@v0.10.2 -ico assets/tray_windows.ico -arch arm64 -o rsrc_windows_arm64.syso

func main() {
	// Has to happen before anything can put a window on screen.
	enableHiDPI()
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
	if _, err := autostart.MigrateCurrent(cwd); err != nil {
		log.Printf("migrate autostart entry: %v", err)
	}
	version.SetRepoURL(cfgStore.Snapshot().UpdateRepoURL)
	svc := service.NewManagerWithPython(cfgStore.Snapshot().GARoot, cfgStore.Snapshot().EffectivePython, cfgStore.Snapshot().BufferLines)
	models := modelconfig.NewStore(cwd)
	static, err := fs.Sub(webFS, "web/dist")
	if err != nil {
		log.Fatal(err)
	}
	srv := api.New(cfgStore, svc, models, static)
	addrs := adminListenAddresses(cfgStore.Snapshot().Host, cfgStore.Snapshot().Port, discoverTailscaleIPv4())
	url := "http://" + addrs[0]
	server := newHTTPServer(addrs[0], authDisabledMiddleware(srv.Routes()))
	srv.StartAutostartServices()
	srv.StartAutonomousMaintenance()
	activeAddrs, err := startHTTPListeners(server, addrs)
	if err != nil {
		log.Fatalf("start HTTP service: %v; if the port is occupied, edit config.local.json and change port", err)
	}
	logListenURLs(activeAddrs)
	if launch.Headless {
		log.Printf("headless/server-only mode enabled; open %s from another browser if needed", url)
	}

	if launch.Headless {
		waitForShutdownSignal(server, func() {
			srv.ShutdownCleanup()
			removeRuntimeInfo(cwd)
		})
		return
	}

	ui := newAppUI(cwd, launch.NoWindow)
	if !launch.NoBrowser {
		go func() { time.Sleep(500 * time.Millisecond); ui.OpenChat(url) }()
	}
	runTray(trayApp{
		OpenChat:     func() { ui.OpenChat(url) },
		OpenSettings: func() { ui.OpenSettings(url) },
		StopServices: func() { srv.StopManagedServices() },
		Status: func() trayStatus {
			return describeTrayStatus(listener.Addr().String(), cfgStore.Snapshot(), auth.PasswordConfigured(), primaryLANAddress, trayLanguage())
		},
		RunningServices: func() int { return srv.RunningManagedServices() },
		Exit: func() {
			ui.CloseAll()
			srv.ShutdownCleanup()
			removeRuntimeInfo(cwd)
			ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
			defer cancel()
			_ = server.Shutdown(ctx)
		},
	})
}

type launchOptions struct {
	Headless  bool
	NoBrowser bool
	NoWindow  bool
	AppRoot   string
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

// isLoopbackRemote reports whether a request came from this machine. Remote
// access binds a dual-stack wildcard socket, so a local browser that resolves
// localhost to ::1 must be recognised as local just like one that picks
// 127.0.0.1; otherwise enabling remote access would start prompting the owner
// for a password on their own desktop.
func isLoopbackRemote(remoteAddr string) bool {
	host, _, err := net.SplitHostPort(remoteAddr)
	if err != nil {
		return false
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

func parseLaunchOptions() launchOptions {
	headlessFlag := flag.Bool("headless", false, "run without browser or tray; intended for Linux servers")
	serverOnlyFlag := flag.Bool("server-only", false, "alias for --headless")
	noBrowserFlag := flag.Bool("no-browser", false, "do not open the web UI automatically")
	noWindowFlag := flag.Bool("no-window", false, "open the web UI in the system browser instead of a native desktop window")
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
		NoWindow:  *noWindowFlag || envBool("GA_ADMIN_NO_WINDOW"),
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
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	_ = server.Shutdown(ctx)
	if cleanup != nil {
		cleanup()
	}
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

func resolvePortableBootstrap(cwd string) (bootstrapPy, pythonExe string, ok bool) {
	// The published/PowerShell bundle layout is canonical.  Retain support for
	// artifacts from a short-lived builder that nested bootstrap and Python in
	// GenericAgent, but never let that override a valid root marker.
	for _, root := range []string{cwd, filepath.Join(cwd, "GenericAgent")} {
		candidate := filepath.Join(root, "bootstrap.py")
		if st, err := os.Stat(candidate); err != nil || st.IsDir() {
			continue
		}
		pythonExe = filepath.Join(root, "python", "bin", "python3")
		if runtime.GOOS == "windows" {
			pythonExe = filepath.Join(root, "python", "python.exe")
		}
		return candidate, pythonExe, true
	}
	return "", "", false
}

// tryPortableAutoInit detects portable bundle environment and runs bootstrap.py
// to populate config with correct paths. Returns nil if not portable or on success.
func tryPortableAutoInit(cwd string, store *config.Store) error {
	gaRoot := filepath.Join(cwd, "GenericAgent")
	gaInfo, gaErr := os.Stat(gaRoot)
	if gaErr != nil || !gaInfo.IsDir() {
		return nil // Not a portable bundle
	}
	bootstrapPy, pythonExe, ok := resolvePortableBootstrap(cwd)
	if !ok {
		return nil // Not a portable bundle
	}
	if store.Snapshot().BootstrapDone && validatePortableConfig(cwd, store.Snapshot()) == nil {
		return nil // Already initialized for this exact bundle location
	}

	log.Printf("portable bundle detected; running bootstrap auto-init")

	if st, err := os.Stat(pythonExe); err != nil {
		return fmt.Errorf("bundled python not found at %s: %w", pythonExe, err)
	} else if st.IsDir() {
		return fmt.Errorf("bundled python path is a directory: %s", pythonExe)
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
	if err := validatePortableConfig(cwd, store.Snapshot()); err != nil {
		return fmt.Errorf("bootstrap produced incomplete config: %w", err)
	}

	// Mark bootstrap as done only after both required paths are present and usable.
	cfg := store.Snapshot()
	cfg.BootstrapDone = true
	if err := store.Save(cfg); err != nil {
		return fmt.Errorf("save bootstrap_done flag: %w", err)
	}

	log.Printf("portable auto-init completed: ga_root=%s", store.Snapshot().GARoot)
	return nil
}

func validatePortableConfig(cwd string, cfg config.AppConfig) error {
	wantGARoot := filepath.Clean(filepath.Join(cwd, "GenericAgent"))
	gaRoot := filepath.Clean(strings.TrimSpace(cfg.GARoot))
	if strings.TrimSpace(cfg.GARoot) == "" {
		return fmt.Errorf("ga_root is empty")
	}
	if !samePath(gaRoot, wantGARoot) {
		return fmt.Errorf("ga_root %q does not point at this bundle %q", gaRoot, wantGARoot)
	}
	st, err := os.Stat(gaRoot)
	if err != nil {
		return fmt.Errorf("ga_root is unusable: %w", err)
	}
	if !st.IsDir() {
		return fmt.Errorf("ga_root is not a directory")
	}

	pythonPath := strings.TrimSpace(cfg.EffectivePython)
	if pythonPath == "" {
		pythonPath = strings.TrimSpace(cfg.PythonPath)
	}
	if pythonPath == "" {
		return fmt.Errorf("python_path is empty")
	}
	pythonPath = filepath.Clean(pythonPath)
	wantPython := filepath.Join(wantGARoot, ".venv", "bin", "python")
	if runtime.GOOS == "windows" {
		wantPython = filepath.Join(wantGARoot, ".venv", "Scripts", "python.exe")
	}
	if !samePath(pythonPath, wantPython) {
		return fmt.Errorf("python_path %q does not point at this bundle %q", pythonPath, wantPython)
	}
	st, err = os.Stat(pythonPath)
	if err != nil {
		return fmt.Errorf("python_path is unusable: %w", err)
	}
	if st.IsDir() {
		return fmt.Errorf("python_path is a directory")
	}
	return nil
}

func samePath(left, right string) bool {
	if runtime.GOOS == "windows" {
		return strings.EqualFold(filepath.Clean(left), filepath.Clean(right))
	}
	return filepath.Clean(left) == filepath.Clean(right)
}
