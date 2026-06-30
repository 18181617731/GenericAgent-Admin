package api

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"

	"genericagent-admin-go/internal/config"
	"genericagent-admin-go/internal/ga"
	"genericagent-admin-go/internal/modelconfig"
	"genericagent-admin-go/internal/service"
)

type Server struct {
	CfgStore    *config.Store
	Svc         *service.Manager
	Models      *modelconfig.Store
	Static      fs.FS
	ReactApp    *reactAppBridge
	ChatMu      sync.Mutex
	ChatRuns    map[string]*chatRun
	ChatWorkers map[string]*chatWorker
}

func New(cfg *config.Store, svc *service.Manager, models *modelconfig.Store, static fs.FS) *Server {
	return &Server{CfgStore: cfg, Svc: svc, Models: models, Static: static, ReactApp: newReactAppBridge(), ChatRuns: map[string]*chatRun{}, ChatWorkers: map[string]*chatWorker{}}
}

func (s *Server) Routes() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/health", s.health)
	mux.HandleFunc("/api/version", s.versionInfo)
	mux.HandleFunc("/api/version/info", s.versionInfo)
	mux.HandleFunc("/api/version/check", s.versionCheck)
	mux.HandleFunc("/api/version/status", s.versionStatus)
	mux.HandleFunc("/api/risk/catalog", s.riskCatalog)
	mux.HandleFunc("/api/version/update", s.requireDangerousConfirm(s.versionUpdate))
	mux.HandleFunc("/api/ga/inventory", s.gaInventory)
	mux.HandleFunc("/api/ga/health", s.gaHealth)
	mux.HandleFunc("/api/ga/control", s.gaControl)
	mux.HandleFunc("/api/ga/git-update", s.requireDangerousConfirm(s.gaGitUpdate))
	mux.HandleFunc("/api/ga/git-status", s.gaGitStatus)
	mux.HandleFunc("/api/ga/git-mirror", s.requireDangerousConfirm(s.gitMirrorConfig))
	mux.HandleFunc("/api/tmwebdriver/status", s.tmwebdriverStatus)
	mux.HandleFunc("/api/tmwebdriver/repair", s.requireDangerousConfirm(s.tmwebdriverRepair))
	mux.HandleFunc("/api/tmwebdriver/install-deps", s.requireDangerousConfirm(s.tmwebdriverInstallDeps))
	mux.HandleFunc("/api/files/list", s.filesList)
	mux.HandleFunc("/api/files/read", s.filesRead)
	mux.HandleFunc("/api/files/write", s.requireDangerousConfirm(s.filesWrite))
	mux.HandleFunc("/api/files/delete", s.requireDangerousConfirm(s.filesDelete))
	mux.HandleFunc("/api/files/download", s.filesDownload)
	mux.HandleFunc("/api/files/tail", s.filesTail)
	mux.HandleFunc("/api/files/search", s.filesSearch)
	mux.HandleFunc("/api/files/open", s.requireDangerousConfirm(s.filesOpen))
	mux.HandleFunc("/api/files/image", s.filesImage)
	mux.HandleFunc("/api/schedule/tasks", s.scheduleTasks)
	mux.HandleFunc("/api/schedule/task", s.requireDangerousConfirm(s.scheduleTask))
	mux.HandleFunc("/api/schedule/create", s.requireDangerousConfirm(s.scheduleCreate))
	mux.HandleFunc("/api/schedule/delete", s.requireDangerousConfirm(s.scheduleDelete))
	mux.HandleFunc("/api/schedule/toggle", s.requireDangerousConfirm(s.scheduleToggle))
	mux.HandleFunc("/api/schedule/artifact", s.scheduleArtifact)
	mux.HandleFunc("/api/goals/start", s.requireDangerousConfirm(s.goalsStart))
	mux.HandleFunc("/api/goals/list", s.goalsList)
	mux.HandleFunc("/api/goals/stop", s.requireDangerousConfirm(s.goalsStop))
	mux.HandleFunc("/api/goals/delete", s.requireDangerousConfirm(s.goalsDelete))
	mux.HandleFunc("/api/goals/output", s.goalsOutput)
	mux.HandleFunc("/api/config", s.requireDangerousConfirm(s.configHandler))
	mux.HandleFunc("/api/setup/state", s.setupState)
	mux.HandleFunc("/api/setup/env", s.setupEnv)
	mux.HandleFunc("/api/setup/browse", s.setupBrowse)
	mux.HandleFunc("/api/setup/validate", s.requireDangerousConfirm(s.setupValidate))
	mux.HandleFunc("/api/setup/install", s.requireDangerousConfirm(s.setupInstall))
	mux.HandleFunc("/api/setup/python/install", s.requireDangerousConfirm(s.setupPythonInstall))
	mux.HandleFunc("/api/setup/venv/create", s.requireDangerousConfirm(s.setupVenvCreate))
	mux.HandleFunc("/api/setup/deps/install", s.requireDangerousConfirm(s.setupDepsInstall))
	mux.HandleFunc("/api/setup/smoke", s.requireDangerousConfirm(s.setupSmoke))
	mux.HandleFunc("/api/setup/complete", s.requireDangerousConfirm(s.setupComplete))
	mux.HandleFunc("/api/autostart/status", s.autostartStatus)
	mux.HandleFunc("/api/autostart/enable", s.requireDangerousConfirm(s.autostartEnable))
	mux.HandleFunc("/api/autostart/disable", s.requireDangerousConfirm(s.autostartDisable))
	mux.HandleFunc("/api/services", s.services)
	mux.HandleFunc("/api/services/summary", s.summary)
	mux.HandleFunc("/api/services/start", s.requireDangerousConfirm(s.start))
	mux.HandleFunc("/api/services/stop", s.requireDangerousConfirm(s.stop))
	mux.HandleFunc("/api/services/stop-all", s.requireDangerousConfirm(s.stopAll))
	mux.HandleFunc("/api/services/autostart", s.requireDangerousConfirm(s.serviceAutostart))
	mux.HandleFunc("/api/services/model", s.requireDangerousConfirm(s.serviceModel))
	mux.HandleFunc("/api/logs/", s.logs)
	mux.HandleFunc("/api/ga/processes", s.gaProcesses)
	mux.HandleFunc("/api/ga/processes/kill", s.requireDangerousConfirm(s.killGAProcess))
	mux.HandleFunc("/api/ga/processes/adopt", s.requireDangerousConfirm(s.adoptGAProcess))
	mux.HandleFunc("/api/models", s.models)
	mux.HandleFunc("/api/models/raw", s.modelsRaw)
	mux.HandleFunc("/api/models/preview", s.modelsPreview)
	mux.HandleFunc("/api/models/discover", s.modelsDiscover)
	mux.HandleFunc("/api/models/import-mykey", s.modelsImportMyKey)
	mux.HandleFunc("/api/models/export", s.requireDangerousConfirm(s.modelsExport))
	mux.HandleFunc("/api/channels/test", s.channelTest)
	mux.HandleFunc("/api/channels", s.requireDangerousConfirm(s.channels))
	mux.HandleFunc("/api/chat/sessions", s.chatSessions)
	mux.HandleFunc("/api/chat/", s.chatHandler)
	// Legacy reactapp bridge is intentionally not routed; Chat is now native Admin API.
	mux.HandleFunc("/", s.static)
	return recoverPanics(cors(mux))
}

func recoverPanics(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if recovered := recover(); recovered != nil {
				bad(w, http.StatusInternalServerError, "internal server error")
			}
		}()
		next.ServeHTTP(w, r)
	})
}

func cors(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, X-GA-Confirm")
		w.Header().Set("Access-Control-Allow-Methods", "GET,POST,PUT,DELETE,OPTIONS")
		if r.Method == "OPTIONS" {
			w.WriteHeader(204)
			return
		}
		next.ServeHTTP(w, r)
	})
}

type riskCatalogItem struct {
	Path   string `json:"path"`
	Level  string `json:"level"`
	Action string `json:"action"`
	Reason string `json:"reason"`
}

var riskCatalogItems = []riskCatalogItem{
	{Path: "/api/files/write", Level: "dangerous", Action: "write_file", Reason: "writes into GA workspace; handler creates backup before overwrite"},
	{Path: "/api/files/delete", Level: "dangerous", Action: "delete_file", Reason: "deletes a file or directory under the configured GA root"},
	{Path: "/api/files/open", Level: "reversible", Action: "open_file_shell", Reason: "spawns the OS desktop shell to open a GA file or its containing folder"},
	{Path: "/api/config", Level: "reversible", Action: "save_config", Reason: "updates Admin-Go local config"},
	{Path: "/api/setup/validate", Level: "reversible", Action: "save_ga_root", Reason: "persists configured GA root after successful health validation"},
	{Path: "/api/setup/install", Level: "dangerous", Action: "install_ga", Reason: "runs git clone or downloads the GenericAgent source archive and changes configured GA root"},
	{Path: "/api/setup/python/install", Level: "dangerous", Action: "install_python", Reason: "downloads and runs the official Windows Python installer and persists the Python path"},
	{Path: "/api/setup/venv/create", Level: "dangerous", Action: "create_venv", Reason: "creates or updates a Python virtual environment under the configured GA root"},
	{Path: "/api/setup/deps/install", Level: "dangerous", Action: "install_dependencies", Reason: "executes pip install in the configured GA root and streams process output"},
	{Path: "/api/setup/smoke", Level: "dangerous", Action: "run_setup_smoke", Reason: "executes Python in the configured GA root to verify bootstrap readiness"},
	{Path: "/api/setup/complete", Level: "reversible", Action: "complete_bootstrap", Reason: "marks first-run bootstrap complete and persists GA root/Python settings"},
	{Path: "/api/ga/git-update", Level: "dangerous", Action: "git_pull", Reason: "executes git pull --ff-only in GA root"},
	{Path: "/api/version/update", Level: "dangerous", Action: "self_update", Reason: "downloads and applies Admin-Go release"},
	{Path: "/api/services/start", Level: "dangerous", Action: "start_process", Reason: "starts GA Python service process"},
	{Path: "/api/services/stop", Level: "dangerous", Action: "stop_process", Reason: "stops a managed GA service process"},
	{Path: "/api/services/stop-all", Level: "dangerous", Action: "stop_all_processes", Reason: "stops all managed GA services"},
	{Path: "/api/services/autostart", Level: "reversible", Action: "toggle_service_autostart", Reason: "changes Admin-Go service autostart list"},
	{Path: "/api/services/model", Level: "reversible", Action: "set_service_model", Reason: "changes the persisted model used to launch a reflect/autonomous service"},
	{Path: "/api/tmwebdriver/repair", Level: "reversible", Action: "start_tmwebdriver_master", Reason: "starts a persistent TMWebDriver master process on localhost:18766"},
	{Path: "/api/tmwebdriver/install-deps", Level: "dangerous", Action: "install_tmwebdriver_deps", Reason: "runs pip install with Tsinghua PyPI mirror for TMWebDriver dependencies"},
	{Path: "/api/ga/git-mirror", Level: "reversible", Action: "configure_git_mirror", Reason: "updates global git insteadOf mirror for github.com URLs"},
	{Path: "/api/autostart/enable", Level: "dangerous", Action: "enable_os_autostart", Reason: "writes OS autostart entry"},
	{Path: "/api/autostart/disable", Level: "reversible", Action: "disable_os_autostart", Reason: "removes OS autostart entry"},
	{Path: "/api/schedule/task", Level: "dangerous", Action: "edit_schedule_task", Reason: "changes scheduled task JSON"},
	{Path: "/api/schedule/create", Level: "dangerous", Action: "create_schedule_task", Reason: "creates scheduled task JSON"},
	{Path: "/api/schedule/delete", Level: "dangerous", Action: "delete_schedule_task", Reason: "deletes scheduled task JSON"},
	{Path: "/api/schedule/toggle", Level: "reversible", Action: "toggle_schedule_task", Reason: "enables or disables scheduled task"},
	{Path: "/api/goals/start", Level: "dangerous", Action: "start_goal", Reason: "starts autonomous GA goal process"},
	{Path: "/api/goals/stop", Level: "dangerous", Action: "stop_goal", Reason: "stops autonomous GA goal process by recorded PID"},
	{Path: "/api/goals/delete", Level: "dangerous", Action: "delete_goal", Reason: "deletes goal state/output files"},
	{Path: "/api/models", Level: "dangerous", Action: "save_model_draft", Reason: "writes GA Admin model draft profiles, including provider endpoints and credentials when supplied"},
	{Path: "/api/models/raw", Level: "dangerous", Action: "reveal_model_secrets", Reason: "returns unmasked model provider credentials after explicit dangerous authorization"},
	{Path: "/api/models/import-mykey", Level: "dangerous", Action: "import_mykey_models", Reason: "can execute mykey import and reveal or persist provider credentials when explicitly authorized"},
	{Path: "/api/models/discover", Level: "reversible", Action: "discover_provider_models", Reason: "queries the selected provider models endpoint without saving configuration"},
	{Path: "/api/models/export", Level: "dangerous", Action: "export_models", Reason: "writes active GA model configuration"},
	{Path: "/api/ga/processes/kill", Level: "dangerous", Action: "kill_ga_process", Reason: "terminates a GA-related process by PID after explicit dangerous authorization"},
	{Path: "/api/ga/processes/adopt", Level: "dangerous", Action: "adopt_ga_process", Reason: "marks an external GA process as managed by Admin-Go for subsequent supervision"},
	{Path: "/api/channels", Level: "dangerous", Action: "edit_channel_secrets", Reason: "writes GA Admin channel credentials to GA root mykey.py"},
}

func (s *Server) riskCatalog(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		bad(w, 405, "method not allowed")
		return
	}
	writeJSON(w, map[string]interface{}{"items": riskCatalogItems})
}

func hasDangerousConfirm(r *http.Request) bool {
	return r.Header.Get("X-GA-Confirm") == "dangerous"
}

func requireDangerousHeader(w http.ResponseWriter, r *http.Request) bool {
	if !hasDangerousConfirm(r) {
		bad(w, http.StatusPreconditionRequired, "dangerous operation requires X-GA-Confirm: dangerous")
		return false
	}
	return true
}

func (s *Server) requireDangerousConfirm(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet && !hasDangerousConfirm(r) {
			bad(w, 428, "dangerous operation requires X-GA-Confirm: dangerous")
			return
		}
		next(w, r)
	}
}

func (s *Server) health(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		bad(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	h := ga.BuildHealth(s.CfgStore.Cfg.GARoot)
	writeJSON(w, map[string]interface{}{"ok": h.OK, "config": s.CfgStore.Cfg, "services": s.Svc.Summary(), "health": h})
}

func (s *Server) gaInventory(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		bad(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	writeJSON(w, ga.BuildInventory(s.CfgStore.Cfg.GARoot))
}

func (s *Server) gaHealth(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		bad(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	writeJSON(w, ga.BuildHealth(s.CfgStore.Cfg.GARoot))
}

func (s *Server) gaControl(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		bad(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	writeJSON(w, ga.BuildControlPlane(s.CfgStore.Cfg.GARoot))
}

type tmwebdriverCheck struct {
	Name   string `json:"name"`
	OK     bool   `json:"ok"`
	Detail string `json:"detail,omitempty"`
}

type tmwebdriverStatusResponse struct {
	OK             bool               `json:"ok"`
	BrowserRunning bool               `json:"browser_running"`
	PortListening  bool               `json:"port_listening"`
	ExtensionFound bool               `json:"extension_found"`
	PythonOK       bool               `json:"python_ok"`
	PythonPath     string             `json:"python_path,omitempty"`
	PythonMissing  []string           `json:"python_missing,omitempty"`
	InstallCommand string             `json:"install_command,omitempty"`
	Port           int                `json:"port"`
	ExtensionPaths []string           `json:"extension_paths,omitempty"`
	Checks         []tmwebdriverCheck `json:"checks"`
	Recommendation string             `json:"recommendation,omitempty"`
	CheckedAt      string             `json:"checked_at"`
}

func (s *Server) tmwebdriverStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		bad(w, 405, "method not allowed")
		return
	}
	st := s.buildTMWebDriverStatus()
	writeJSON(w, st)
}

const defaultPipIndexURL = "https://pypi.tuna.tsinghua.edu.cn/simple"

func (s *Server) tmwebdriverInstallDeps(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		bad(w, 405, "method not allowed")
		return
	}
	gaRoot := strings.TrimSpace(s.CfgStore.Cfg.GARoot)
	if gaRoot == "" {
		bad(w, 400, "ga_root is empty")
		return
	}
	python := resolvePythonForRoot(gaRoot, s.CfgStore.Cfg.PythonPath)
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Minute)
	defer cancel()
	args := buildTMWebDriverInstallArgs(defaultPipIndexURL)
	cmd := exec.CommandContext(ctx, python, args...)
	cmd.Dir = gaRoot
	hideChildWindow(cmd)
	out, err := cmd.CombinedOutput()
	if ctx.Err() == context.DeadlineExceeded {
		bad(w, 504, "pip install timed out")
		return
	}
	status := s.buildTMWebDriverStatus()
	resp := map[string]interface{}{
		"ok":      err == nil && status.PythonOK,
		"python":  python,
		"command": append([]string{python}, args...),
		"output":  strings.TrimSpace(string(out)),
		"status":  status,
	}
	if err != nil {
		resp["error"] = err.Error()
	}
	writeJSON(w, resp)
}

const defaultGitHubMirrorPrefix = "https://gh-proxy.com/https://github.com/"

type gitMirrorRequest struct {
	Enabled bool   `json:"enabled"`
	Mirror  string `json:"mirror"`
}

func (s *Server) gitMirrorConfig(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		bad(w, 405, "method not allowed")
		return
	}
	var req gitMirrorRequest
	if r.Body != nil {
		if err := decode(r, &req); err != nil {
			bad(w, http.StatusBadRequest, err.Error())
			return
		}
	}
	mirror := strings.TrimSpace(req.Mirror)
	if mirror == "" {
		mirror = defaultGitHubMirrorPrefix
	}
	if err := validateGitMirrorPrefix(mirror); err != nil {
		bad(w, 400, err.Error())
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()
	args := buildGitMirrorArgs(req.Enabled, mirror)
	cmd := exec.CommandContext(ctx, "git", args...)
	hideChildWindow(cmd)
	out, err := cmd.CombinedOutput()
	if ctx.Err() == context.DeadlineExceeded {
		bad(w, 504, "git config timed out")
		return
	}
	resp := map[string]interface{}{
		"ok":      err == nil,
		"enabled": req.Enabled,
		"mirror":  mirror,
		"command": append([]string{"git"}, args...),
		"output":  strings.TrimSpace(string(out)),
	}
	if err != nil {
		resp["error"] = err.Error()
	}
	writeJSON(w, resp)
}

func validateGitMirrorPrefix(mirror string) error {
	if mirror == "" {
		return fmt.Errorf("mirror is required")
	}
	if strings.ContainsAny(mirror, "\x00\r\n\t") {
		return fmt.Errorf("mirror must not contain control characters")
	}
	u, err := url.Parse(mirror)
	if err != nil || !u.IsAbs() {
		return fmt.Errorf("mirror must be an absolute http(s) URL")
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return fmt.Errorf("mirror scheme must be http or https")
	}
	if u.Host == "" {
		return fmt.Errorf("mirror host is required")
	}
	if u.User != nil || u.RawQuery != "" || u.Fragment != "" {
		return fmt.Errorf("mirror must not include userinfo, query, or fragment")
	}
	return nil
}

func buildGitMirrorArgs(enabled bool, mirror string) []string {
	key := "url." + mirror + ".insteadOf"
	if enabled {
		return []string{"config", "--global", key, "https://github.com/"}
	}
	return []string{"config", "--global", "--unset-all", key}
}

type tmwebdriverRepairResponse struct {
	Started bool                      `json:"started"`
	PID     int                       `json:"pid,omitempty"`
	Command []string                  `json:"command,omitempty"`
	Status  tmwebdriverStatusResponse `json:"status"`
	Message string                    `json:"message,omitempty"`
}

func (s *Server) tmwebdriverRepair(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		bad(w, 405, "method not allowed")
		return
	}
	before := s.buildTMWebDriverStatus()
	if before.PortListening {
		writeJSON(w, tmwebdriverRepairResponse{Started: false, Status: before, Message: "18766 master 已在监听，无需重复启动。"})
		return
	}
	if !before.PythonOK {
		writeJSON(w, tmwebdriverRepairResponse{Started: false, Status: before, Message: "TMWebDriver Python 依赖缺失，请先执行：" + before.InstallCommand})
		return
	}
	pid, cmdline, err := s.startTMWebDriverMaster()
	if err != nil {
		bad(w, 500, err.Error())
		return
	}
	var after tmwebdriverStatusResponse
	for i := 0; i < 20; i++ {
		time.Sleep(250 * time.Millisecond)
		after = s.buildTMWebDriverStatus()
		if after.PortListening {
			break
		}
	}
	if after.PortListening {
	} else {
	}
	writeJSON(w, tmwebdriverRepairResponse{Started: true, PID: pid, Command: cmdline, Status: after, Message: "已启动 TMWebDriver master；若仍未 OK，请确认浏览器已打开且扩展已安装。"})
}

func (s *Server) startTMWebDriverMaster() (int, []string, error) {
	gaRoot := strings.TrimSpace(s.CfgStore.Cfg.GARoot)
	if gaRoot == "" {
		return 0, nil, errors.New("ga_root is empty")
	}
	python := resolvePythonForRoot(gaRoot, s.CfgStore.Cfg.PythonPath)
	code := "from TMWebDriver import TMWebDriver; TMWebDriver()"
	cmd := exec.Command(python, "-c", code)
	cmd.Dir = gaRoot
	cmd.Stdout = nil
	cmd.Stderr = nil
	hideChildWindow(cmd)
	if err := cmd.Start(); err != nil {
		return 0, []string{python, "-c", code}, err
	}
	return cmd.Process.Pid, []string{python, "-c", code}, nil
}

func resolvePythonForRoot(gaRoot, configured string) string {
	if configured = strings.TrimSpace(configured); configured != "" {
		return configured
	}
	var candidates []string
	if runtime.GOOS == "windows" {
		candidates = append(candidates, filepath.Join(gaRoot, ".venv", "Scripts", "python.exe"), filepath.Join(gaRoot, "venv", "Scripts", "python.exe"), "python")
	} else {
		candidates = append(candidates, filepath.Join(gaRoot, ".venv", "bin", "python"), filepath.Join(gaRoot, "venv", "bin", "python"), "python3", "python")
	}
	for _, c := range candidates {
		if strings.ContainsRune(c, filepath.Separator) {
			if st, err := os.Stat(c); err == nil && !st.IsDir() {
				return c
			}
			continue
		}
		if p, err := exec.LookPath(c); err == nil {
			return p
		}
	}
	return "python"
}

func (s *Server) buildTMWebDriverStatus() tmwebdriverStatusResponse {
	return buildTMWebDriverStatusForConfig(s.CfgStore.Cfg.GARoot, s.CfgStore.Cfg.PythonPath)
}

func buildTMWebDriverStatusForConfig(gaRoot, configuredPython string) tmwebdriverStatusResponse {
	const port = 18766
	browserRunning, browserDetail := detectChromeRunning()
	portListening, portDetail := detectTCPListening("127.0.0.1", port, 700*time.Millisecond)
	extFound, extPaths, extDetail := detectTMWebDriverExtension()
	pythonPath := resolvePythonForRoot(gaRoot, configuredPython)
	pythonOK, pythonMissing, pythonDetail := detectTMWebDriverPythonDeps(gaRoot, pythonPath)
	installCommand := buildTMWebDriverInstallCommand(pythonPath)
	st := tmwebdriverStatusResponse{
		OK:             browserRunning && portListening && extFound && pythonOK,
		BrowserRunning: browserRunning,
		PortListening:  portListening,
		ExtensionFound: extFound,
		PythonOK:       pythonOK,
		PythonPath:     pythonPath,
		PythonMissing:  pythonMissing,
		InstallCommand: installCommand,
		Port:           port,
		ExtensionPaths: extPaths,
		CheckedAt:      time.Now().Format(time.RFC3339),
		Checks: []tmwebdriverCheck{
			{Name: "browser_process", OK: browserRunning, Detail: browserDetail},
			{Name: "python_dependencies", OK: pythonOK, Detail: pythonDetail},
			{Name: "ws_master_port", OK: portListening, Detail: portDetail},
			{Name: "chrome_extension", OK: extFound, Detail: extDetail},
		},
	}
	if st.OK {
		st.Recommendation = "TMWebDriver 基础监控正常：Python 依赖、浏览器进程、18766 master 端口和扩展均已检测到。"
	} else if !pythonOK {
		st.Recommendation = fmt.Sprintf("TMWebDriver Python 依赖缺失：%s。请在 GA 环境执行：%s", strings.Join(pythonMissing, ", "), installCommand)
	} else if !browserRunning {
		st.Recommendation = "未检测到 Chrome/Edge 浏览器进程；请先打开已安装 TMWebDriver 扩展的浏览器。"
	} else if !portListening {
		st.Recommendation = "未检测到 18766 端口监听；请重启 ljq_driver/TMWebDriver master。"
	} else if !extFound {
		st.Recommendation = "未在 Chrome Secure Preferences 中检测到 tmwd_cdp_bridge 扩展；请按 web_setup_sop 安装或修复扩展。"
	}
	return st
}

func tmwebdriverPythonModules() []string {
	return []string{"requests", "bottle", "simple_websocket_server"}
}

func tmwebdriverModuleToPipPackage(module string) string {
	if module == "simple_websocket_server" {
		return "simple-websocket-server"
	}
	return module
}

func tmwebdriverPipPackages(modules []string) []string {
	pkgs := make([]string, 0, len(modules))
	for _, m := range modules {
		pkgs = append(pkgs, tmwebdriverModuleToPipPackage(m))
	}
	return pkgs
}

func tmwebdriverRequiredPipPackages() []string {
	return tmwebdriverPipPackages(tmwebdriverPythonModules())
}

func detectTMWebDriverPythonDeps(gaRoot, python string) (bool, []string, string) {
	modules := tmwebdriverPythonModules()
	code := "import importlib.util, json; mods=" + strconv.Quote(strings.Join(modules, ",")) + ".split(','); missing=[m for m in mods if importlib.util.find_spec(m) is None]; print(json.dumps(missing))"
	cmd := exec.Command(python, "-c", code)
	if strings.TrimSpace(gaRoot) != "" {
		cmd.Dir = gaRoot
	}
	hideChildWindow(cmd)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return false, tmwebdriverRequiredPipPackages(), strings.TrimSpace(string(out) + " " + err.Error())
	}
	var missingModules []string
	if err := json.Unmarshal(bytes.TrimSpace(out), &missingModules); err != nil {
		return false, tmwebdriverRequiredPipPackages(), "cannot parse python dependency probe: " + strings.TrimSpace(string(out))
	}
	missingPkgs := tmwebdriverPipPackages(missingModules)
	if len(missingPkgs) > 0 {
		return false, missingPkgs, "missing: " + strings.Join(missingPkgs, ", ")
	}
	return true, nil, strings.Join(tmwebdriverRequiredPipPackages(), ", ") + " installed for " + python
}

func buildTMWebDriverInstallArgs(indexURL string) []string {
	args := []string{"-m", "pip", "install"}
	if strings.TrimSpace(indexURL) != "" {
		args = append(args, "-i", strings.TrimSpace(indexURL))
	}
	return append(args, tmwebdriverRequiredPipPackages()...)
}

func buildTMWebDriverInstallCommand(python string) string {
	if strings.TrimSpace(python) == "" {
		python = "python"
	}
	return python + " " + strings.Join(buildTMWebDriverInstallArgs(defaultPipIndexURL), " ")
}

func detectTCPListening(host string, port int, timeout time.Duration) (bool, string) {
	addr := net.JoinHostPort(host, strconv.Itoa(port))
	conn, err := net.DialTimeout("tcp", addr, timeout)
	if err != nil {
		return false, err.Error()
	}
	_ = conn.Close()
	return true, "listening on " + addr
}

func detectChromeRunning() (bool, string) {
	if runtime.GOOS == "windows" {
		cmd := exec.Command("tasklist", "/FO", "CSV", "/NH")
		hideChildWindow(cmd)
		out, err := cmd.Output()
		if err != nil {
			return false, err.Error()
		}
		lower := bytes.ToLower(out)
		if bytes.Contains(lower, []byte("chrome.exe")) || bytes.Contains(lower, []byte("msedge.exe")) {
			return true, "chrome.exe/msedge.exe process found"
		}
		return false, "chrome.exe/msedge.exe process not found"
	}
	cmd := exec.Command("ps", "-A", "-o", "comm=")
	hideChildWindow(cmd)
	out, err := cmd.Output()
	if err != nil {
		return false, err.Error()
	}
	lower := bytes.ToLower(out)
	if bytes.Contains(lower, []byte("chrome")) || bytes.Contains(lower, []byte("chromium")) || bytes.Contains(lower, []byte("msedge")) {
		return true, "chrome/chromium/msedge process found"
	}
	return false, "chrome/chromium/msedge process not found"
}

func detectTMWebDriverExtension() (bool, []string, string) {
	candidates := chromeSecurePreferencePaths()
	var paths []string
	var checked []string
	for _, p := range candidates {
		checked = append(checked, p)
		b, err := os.ReadFile(p)
		if err != nil {
			continue
		}
		lower := strings.ToLower(string(b))
		if strings.Contains(lower, "tmwd_cdp_bridge") {
			paths = append(paths, p)
		}
	}
	if len(paths) > 0 {
		return true, paths, fmt.Sprintf("found in %d profile(s)", len(paths))
	}
	if len(checked) == 0 {
		return false, nil, "no known Chrome profile paths"
	}
	return false, nil, "not found in checked Secure Preferences"
}

func chromeSecurePreferencePaths() []string {
	var roots []string
	if runtime.GOOS == "windows" {
		local := os.Getenv("LOCALAPPDATA")
		if local != "" {
			roots = append(roots, filepath.Join(local, "Google", "Chrome", "User Data"), filepath.Join(local, "Microsoft", "Edge", "User Data"))
		}
	} else {
		home, _ := os.UserHomeDir()
		if home != "" {
			roots = append(roots, filepath.Join(home, ".config", "google-chrome"), filepath.Join(home, ".config", "chromium"), filepath.Join(home, ".config", "microsoft-edge"), filepath.Join(home, "Library", "Application Support", "Google", "Chrome"))
		}
	}
	var out []string
	for _, root := range roots {
		entries, err := os.ReadDir(root)
		if err != nil {
			continue
		}
		for _, e := range entries {
			if !e.IsDir() {
				continue
			}
			name := e.Name()
			if name == "Default" || strings.HasPrefix(name, "Profile ") {
				out = append(out, filepath.Join(root, name, "Secure Preferences"))
			}
		}
	}
	return out
}

func (s *Server) static(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		bad(w, 405, "method not allowed")
		return
	}
	if s.Static == nil {
		bad(w, 404, "web dist not embedded")
		return
	}
	rawPath := strings.TrimPrefix(r.URL.Path, "/")
	for _, seg := range strings.Split(rawPath, "/") {
		if seg == ".." || strings.Contains(seg, `\\`) {
			bad(w, http.StatusBadRequest, "invalid static asset path")
			return
		}
	}
	p := rawPath
	if p == "" {
		p = "index.html"
	}
	p = path.Clean(p)
	data, err := fs.ReadFile(s.Static, p)
	if err != nil {
		data, err = fs.ReadFile(s.Static, "index.html")
		if err != nil {
			bad(w, 404, fmt.Sprintf("not found: %s", p))
			return
		}
		p = "index.html"
	}
	if strings.HasSuffix(p, ".js") {
		w.Header().Set("Content-Type", "application/javascript")
	} else if strings.HasSuffix(p, ".css") {
		w.Header().Set("Content-Type", "text/css")
	} else if strings.HasSuffix(p, ".html") {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
	}
	_, _ = w.Write(data)
}

type reactAppBridge struct {
	mu     sync.Mutex
	cmd    *exec.Cmd
	port   int
	base   *url.URL
	proxy  *httputil.ReverseProxy
	logs   []string
	status string
}

var (
	reactAppReadyTimeout      = 5 * time.Second
	reactAppReadyPollInterval = 150 * time.Millisecond
)

func newReactAppBridge() *reactAppBridge { return &reactAppBridge{status: "stopped"} }

func (b *reactAppBridge) snapshot() map[string]interface{} {
	b.mu.Lock()
	defer b.mu.Unlock()
	running := b.cmd != nil && b.cmd.Process != nil && b.status == "running"
	pid := 0
	if running {
		pid = b.cmd.Process.Pid
		if !processAlive(pid) {
			b.status = "stopped"
			b.logs = append(b.logs, fmt.Sprintf("[process exited: pid %d is no longer alive]", pid))
			if len(b.logs) > 300 {
				b.logs = b.logs[len(b.logs)-300:]
			}
			running = false
			pid = 0
		}
	}
	logs := append([]string(nil), b.logs...)
	return map[string]interface{}{"running": running, "pid": pid, "port": b.port, "url": "/reactapp/", "status": b.status, "logs": logs}
}

func (b *reactAppBridge) appendLog(line string) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.logs = append(b.logs, line)
	if len(b.logs) > 300 {
		b.logs = b.logs[len(b.logs)-300:]
	}
}

func (b *reactAppBridge) stop() error {
	b.mu.Lock()
	cmd := b.cmd
	b.status = "stopped"
	b.proxy = nil
	b.base = nil
	b.mu.Unlock()
	if cmd != nil && cmd.Process != nil {
		return cmd.Process.Kill()
	}
	return nil
}

func (b *reactAppBridge) start(gaRoot string) error {
	b.mu.Lock()
	if b.cmd != nil && b.cmd.Process != nil && b.status == "running" {
		pid := b.cmd.Process.Pid
		if processAlive(pid) {
			b.mu.Unlock()
			return nil
		}
		b.status = "stopped"
		b.logs = append(b.logs, fmt.Sprintf("[process exited: pid %d is no longer alive]", pid))
		if len(b.logs) > 300 {
			b.logs = b.logs[len(b.logs)-300:]
		}
	}
	b.mu.Unlock()
	port, err := freePort()
	if err != nil {
		return err
	}
	py := pythonForRoot(gaRoot)
	script := filepath.Join(gaRoot, "frontends", "reactapp.py")
	if st, err := os.Stat(script); err != nil || st.IsDir() {
		return fmt.Errorf("reactapp.py not found: %s", script)
	}
	cmd := exec.Command(py, script)
	cmd.Dir = gaRoot
	cmd.Env = append(os.Environ(), "PYTHONUNBUFFERED=1", fmt.Sprintf("GA_REACT_PORT=%d", port))
	hideChildWindow(cmd)
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return err
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return err
	}
	u, _ := url.Parse(fmt.Sprintf("http://127.0.0.1:%d/", port))
	proxy := httputil.NewSingleHostReverseProxy(u)
	orig := proxy.Director
	proxy.Director = func(r *http.Request) {
		orig(r)
		r.Host = u.Host
		r.URL.Path = strings.TrimPrefix(r.URL.Path, "/reactapp")
		if r.URL.Path == "" {
			r.URL.Path = "/"
		}
	}
	if err := cmd.Start(); err != nil {
		return err
	}
	b.mu.Lock()
	b.cmd = cmd
	b.port = port
	b.base = u
	b.proxy = proxy
	b.status = "running"
	b.logs = []string{fmt.Sprintf("$ %s %s (GA_REACT_PORT=%d)", py, script, port)}
	b.mu.Unlock()
	go b.copyPipe(stdout)
	go b.copyPipe(stderr)
	go func() {
		err := cmd.Wait()
		b.appendLog(fmt.Sprintf("[process exited: %v]", err))
		b.mu.Lock()
		if b.cmd == cmd {
			b.status = "stopped"
		}
		b.mu.Unlock()
	}()
	return b.waitUntilReady(port, cmd)
}

func (b *reactAppBridge) waitUntilReady(port int, cmd *exec.Cmd) error {
	deadline := time.Now().Add(reactAppReadyTimeout)
	for time.Now().Before(deadline) {
		if conn, err := net.DialTimeout("tcp", fmt.Sprintf("127.0.0.1:%d", port), 200*time.Millisecond); err == nil {
			_ = conn.Close()
			return nil
		}
		time.Sleep(reactAppReadyPollInterval)
	}
	err := fmt.Errorf("reactapp did not become ready on 127.0.0.1:%d within %s", port, reactAppReadyTimeout)
	b.appendLog("[readiness timeout] " + err.Error())
	if cmd != nil && cmd.Process != nil {
		_ = cmd.Process.Kill()
	}
	b.mu.Lock()
	if b.cmd == cmd {
		b.status = "stopped"
		b.proxy = nil
		b.base = nil
	}
	b.mu.Unlock()
	return err
}

func (b *reactAppBridge) copyPipe(r io.Reader) {
	buf := make([]byte, 4096)
	acc := ""
	for {
		n, err := r.Read(buf)
		if n > 0 {
			acc += string(buf[:n])
			for {
				i := strings.IndexByte(acc, '\n')
				if i < 0 {
					break
				}
				b.appendLog(strings.TrimRight(acc[:i], "\r"))
				acc = acc[i+1:]
			}
		}
		if err != nil {
			if acc != "" {
				b.appendLog(acc)
			}
			return
		}
	}
}

func freePort() (int, error) {
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return 0, err
	}
	defer l.Close()
	return l.Addr().(*net.TCPAddr).Port, nil
}

func pythonForRoot(root string) string {
	cands := []string{}
	if runtime.GOOS == "windows" {
		cands = append(cands, filepath.Join(root, ".venv", "Scripts", "python.exe"), filepath.Join(root, "venv", "Scripts", "python.exe"))
	} else {
		cands = append(cands, filepath.Join(root, ".venv", "bin", "python"), filepath.Join(root, "venv", "bin", "python"))
	}
	for _, c := range cands {
		if st, err := os.Stat(c); err == nil && !st.IsDir() {
			return c
		}
	}
	return "python"
}

func (s *Server) reactAppStatus(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, s.ReactApp.snapshot())
}
func (s *Server) reactAppStart(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		bad(w, 405, "method not allowed")
		return
	}
	if err := s.ReactApp.start(s.CfgStore.Cfg.GARoot); err != nil {
		bad(w, 500, err.Error())
		return
	}
	writeJSON(w, s.ReactApp.snapshot())
}
func (s *Server) reactAppStop(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		bad(w, 405, "method not allowed")
		return
	}
	if err := s.ReactApp.stop(); err != nil {
		bad(w, 500, err.Error())
		return
	}
	writeJSON(w, s.ReactApp.snapshot())
}
func (s *Server) reactAppProxy(w http.ResponseWriter, r *http.Request) {
	if err := s.ReactApp.start(s.CfgStore.Cfg.GARoot); err != nil {
		bad(w, 500, err.Error())
		return
	}
	s.ReactApp.mu.Lock()
	proxy := s.ReactApp.proxy
	s.ReactApp.mu.Unlock()
	if proxy == nil {
		bad(w, 503, "reactapp proxy not ready")
		return
	}
	proxy.ServeHTTP(w, r)
}

// StopManagedServices stops GenericAgent child services managed by the Admin UI.
func (s *Server) StopManagedServices() {
	if s == nil || s.Svc == nil {
		return
	}
	s.Svc.StopAll()
}

// ShutdownCleanup stops child processes before the Admin process exits.
func (s *Server) ShutdownCleanup() {
	if s == nil {
		return
	}
	s.StopManagedServices()
	if s.ReactApp != nil {
		_ = s.ReactApp.stop()
	}
	s.CloseChatWorkers()
}
