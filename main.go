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
	// Portable bundle bootstrap is idempotent and also repairs bundles created
	// by older versions that may have persisted bootstrap_done=true too early.
	if err := tryPortableAutoInit(cwd, cfgStore); err != nil {
		log.Printf("portable auto-init: %v", err)
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
	if store.Cfg.BootstrapDone && validatePortableConfig(cwd, store.Cfg) == nil {
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
	if err := validatePortableConfig(cwd, store.Cfg); err != nil {
		return fmt.Errorf("bootstrap produced incomplete config: %w", err)
	}

	// Mark bootstrap as done only after both required paths are present and usable.
	store.Cfg.BootstrapDone = true
	if err := store.Save(store.Cfg); err != nil {
		return fmt.Errorf("save bootstrap_done flag: %w", err)
	}

	log.Printf("portable auto-init completed: ga_root=%s", store.Cfg.GARoot)
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
