package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"log"
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

	"genericagent-admin-go/internal/autostart"
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
	mux.HandleFunc("/api/ga/inventory", s.gaInventory)
	mux.HandleFunc("/api/ga/health", s.gaHealth)
	mux.HandleFunc("/api/ga/control", s.gaControl)
	mux.HandleFunc("/api/files/list", s.filesList)
	mux.HandleFunc("/api/files/read", s.filesRead)
	mux.HandleFunc("/api/files/write", s.filesWrite)
	mux.HandleFunc("/api/files/tail", s.filesTail)
	mux.HandleFunc("/api/files/search", s.filesSearch)
	mux.HandleFunc("/api/files/open", s.filesOpen)
	mux.HandleFunc("/api/schedule/tasks", s.scheduleTasks)
	mux.HandleFunc("/api/schedule/task", s.scheduleTask)
	mux.HandleFunc("/api/schedule/create", s.scheduleCreate)
	mux.HandleFunc("/api/schedule/delete", s.scheduleDelete)
	mux.HandleFunc("/api/schedule/toggle", s.scheduleToggle)
	mux.HandleFunc("/api/schedule/artifact", s.scheduleArtifact)
	mux.HandleFunc("/api/goals/start", s.goalsStart)
	mux.HandleFunc("/api/goals/list", s.goalsList)
	mux.HandleFunc("/api/goals/stop", s.goalsStop)
	mux.HandleFunc("/api/goals/delete", s.goalsDelete)
	mux.HandleFunc("/api/goals/output", s.goalsOutput)
	mux.HandleFunc("/api/config", s.configHandler)
	mux.HandleFunc("/api/setup/env", s.setupEnv)
	mux.HandleFunc("/api/setup/browse", s.setupBrowse)
	mux.HandleFunc("/api/setup/validate", s.setupValidate)
	mux.HandleFunc("/api/setup/install", s.setupInstall)
	mux.HandleFunc("/api/autostart/status", s.autostartStatus)
	mux.HandleFunc("/api/autostart/enable", s.autostartEnable)
	mux.HandleFunc("/api/autostart/disable", s.autostartDisable)
	mux.HandleFunc("/api/services", s.services)
	mux.HandleFunc("/api/services/summary", s.summary)
	mux.HandleFunc("/api/services/start", s.start)
	mux.HandleFunc("/api/services/stop", s.stop)
	mux.HandleFunc("/api/services/stop-all", s.stopAll)
	mux.HandleFunc("/api/services/autostart", s.serviceAutostart)
	mux.HandleFunc("/api/logs/", s.logs)
	mux.HandleFunc("/api/models", s.models)
	mux.HandleFunc("/api/models/raw", s.modelsRaw)
	mux.HandleFunc("/api/models/preview", s.modelsPreview)
	mux.HandleFunc("/api/models/import-mykey", s.modelsImportMyKey)
	mux.HandleFunc("/api/models/export", s.modelsExport)
	mux.HandleFunc("/api/chat/sessions", s.chatSessions)
	mux.HandleFunc("/api/chat/", s.chatHandler)
	// Legacy reactapp bridge is intentionally not routed; Chat is now native Admin API.
	mux.HandleFunc("/", s.static)
	return cors(mux)
}

func cors(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
		w.Header().Set("Access-Control-Allow-Methods", "GET,POST,PUT,DELETE,OPTIONS")
		if r.Method == "OPTIONS" {
			w.WriteHeader(204)
			return
		}
		next.ServeHTTP(w, r)
	})
}
func writeJSON(w http.ResponseWriter, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(v)
}
func bad(w http.ResponseWriter, code int, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(map[string]string{"detail": msg})
}
func decode(r *http.Request, v interface{}) error {
	defer r.Body.Close()
	dec := json.NewDecoder(r.Body)
	if err := dec.Decode(v); err != nil {
		return err
	}
	var extra any
	if err := dec.Decode(&extra); err != io.EOF {
		if err == nil {
			return errors.New("request body must contain a single JSON value")
		}
		return err
	}
	return nil
}

func (s *Server) health(w http.ResponseWriter, r *http.Request) {
	h := ga.BuildHealth(s.CfgStore.Cfg.GARoot)
	writeJSON(w, map[string]interface{}{"ok": h.OK, "config": s.CfgStore.Cfg, "services": s.Svc.Summary(), "health": h})
}

func (s *Server) gaInventory(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, ga.BuildInventory(s.CfgStore.Cfg.GARoot))
}

func (s *Server) gaHealth(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, ga.BuildHealth(s.CfgStore.Cfg.GARoot))
}

func (s *Server) gaControl(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, ga.BuildControlPlane(s.CfgStore.Cfg.GARoot))
}

func (s *Server) scheduleTasks(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, ga.BuildSchedule(s.CfgStore.Cfg.GARoot))
}

func (s *Server) scheduleTask(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		raw, id, err := ga.ReadTask(s.CfgStore.Cfg.GARoot, r.URL.Query().Get("id"))
		if err != nil {
			bad(w, 400, err.Error())
			return
		}
		writeJSON(w, map[string]interface{}{"id": id, "task": raw})
	case http.MethodPut:
		var req struct {
			ID   string         `json:"id"`
			Task map[string]any `json:"task"`
		}
		if err := decode(r, &req); err != nil || req.ID == "" {
			bad(w, 400, "bad request")
			return
		}
		t, err := ga.SaveTask(s.CfgStore.Cfg.GARoot, req.ID, req.Task)
		if err != nil {
			bad(w, 400, err.Error())
			return
		}
		writeJSON(w, map[string]interface{}{"ok": true, "task": t})
	default:
		bad(w, 405, "method not allowed")
	}
}

func (s *Server) goalsStart(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		bad(w, 405, "method not allowed")
		return
	}
	var req struct {
		Objective     string `json:"objective"`
		BudgetSeconds int    `json:"budget_seconds"`
		BudgetMinutes int    `json:"budget_minutes"`
		MaxTurns      int    `json:"max_turns"`
		LLMNo         *int   `json:"llm_no"`
	}
	if err := decode(r, &req); err != nil {
		bad(w, 400, err.Error())
		return
	}
	if req.BudgetSeconds > 0 && req.BudgetMinutes > 0 {
		bad(w, 400, "use either budget_seconds or budget_minutes, not both")
		return
	}
	if req.BudgetSeconds <= 0 && req.BudgetMinutes > 0 {
		if req.BudgetMinutes > 30*24*60 {
			bad(w, 400, "budget_minutes must be <= 43200")
			return
		}
		req.BudgetSeconds = req.BudgetMinutes * 60
	}
	meta, err := ga.StartGoal(s.CfgStore.Cfg.GARoot, ga.GoalStartOptions{Objective: req.Objective, BudgetSeconds: req.BudgetSeconds, MaxTurns: req.MaxTurns, LLMNo: req.LLMNo, PythonPath: s.CfgStore.Cfg.PythonPath})
	if err != nil {
		bad(w, 400, err.Error())
		return
	}
	writeJSON(w, map[string]interface{}{"ok": true, "goal": meta})
}

func (s *Server) goalsList(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		bad(w, 405, "method not allowed")
		return
	}
	items, err := ga.ListGoals(s.CfgStore.Cfg.GARoot)
	if err != nil {
		bad(w, 400, err.Error())
		return
	}
	writeJSON(w, map[string]interface{}{"goals": items})
}

func (s *Server) goalsStop(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		bad(w, 405, "method not allowed")
		return
	}
	var req struct {
		ID  string `json:"id"`
		PID int    `json:"pid"`
	}
	if err := decode(r, &req); err != nil {
		bad(w, 400, err.Error())
		return
	}
	meta, err := ga.StopGoal(s.CfgStore.Cfg.GARoot, req.ID, req.PID)
	if err != nil {
		bad(w, 400, err.Error())
		return
	}
	writeJSON(w, map[string]interface{}{"ok": true, "goal": meta})
}

func (s *Server) goalsDelete(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost && r.Method != http.MethodDelete {
		bad(w, 405, "method not allowed")
		return
	}
	var req struct {
		ID string `json:"id"`
	}
	if r.Method == http.MethodDelete {
		req.ID = r.URL.Query().Get("id")
	} else if err := decode(r, &req); err != nil {
		bad(w, 400, err.Error())
		return
	}
	if err := ga.DeleteGoal(s.CfgStore.Cfg.GARoot, req.ID); err != nil {
		bad(w, 400, err.Error())
		return
	}
	writeJSON(w, map[string]interface{}{"ok": true, "id": req.ID})
}

func (s *Server) goalsOutput(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		bad(w, 405, "method not allowed")
		return
	}
	maxBytes := int64(0)
	if raw := strings.TrimSpace(r.URL.Query().Get("max_bytes")); raw != "" {
		var err error
		maxBytes, err = strconv.ParseInt(raw, 10, 64)
		if err != nil {
			bad(w, 400, "invalid max_bytes")
			return
		}
		if maxBytes < 0 {
			bad(w, 400, "max_bytes must be >= 0")
			return
		}
	}
	result, err := ga.GoalOutput(s.CfgStore.Cfg.GARoot, r.URL.Query().Get("id"), maxBytes)
	if err != nil {
		bad(w, 400, err.Error())
		return
	}
	writeJSON(w, result)
}

func (s *Server) scheduleArtifact(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		bad(w, 405, "method not allowed")
		return
	}
	content, entry, err := ga.ReadScheduleArtifact(s.CfgStore.Cfg.GARoot, r.URL.Query().Get("path"), 256*1024)
	if err != nil {
		bad(w, 400, err.Error())
		return
	}
	writeJSON(w, map[string]interface{}{"entry": entry, "content": content})
}

func (s *Server) scheduleCreate(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		bad(w, 405, "method not allowed")
		return
	}
	var req struct {
		ID   string         `json:"id"`
		Task map[string]any `json:"task"`
	}
	if err := decode(r, &req); err != nil || req.ID == "" {
		bad(w, 400, "bad request")
		return
	}
	t, err := ga.CreateTask(s.CfgStore.Cfg.GARoot, req.ID, req.Task)
	if err != nil {
		bad(w, 400, err.Error())
		return
	}
	writeJSON(w, map[string]interface{}{"ok": true, "task": t})
}

func (s *Server) scheduleDelete(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost && r.Method != http.MethodDelete {
		bad(w, 405, "method not allowed")
		return
	}
	var req struct {
		ID string `json:"id"`
	}
	if r.Method == http.MethodDelete {
		req.ID = r.URL.Query().Get("id")
	} else if err := decode(r, &req); err != nil {
		bad(w, 400, "bad request")
		return
	}
	if req.ID == "" {
		bad(w, 400, "empty id")
		return
	}
	if err := ga.DeleteTask(s.CfgStore.Cfg.GARoot, req.ID); err != nil {
		bad(w, 400, err.Error())
		return
	}
	writeJSON(w, map[string]bool{"ok": true})
}

func (s *Server) scheduleToggle(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		bad(w, 405, "method not allowed")
		return
	}
	var req struct {
		ID      string `json:"id"`
		Enabled bool   `json:"enabled"`
	}
	if err := decode(r, &req); err != nil || req.ID == "" {
		bad(w, 400, "bad request")
		return
	}
	task, err := ga.ToggleTask(s.CfgStore.Cfg.GARoot, req.ID, req.Enabled)
	if err != nil {
		bad(w, 400, err.Error())
		return
	}
	writeJSON(w, map[string]interface{}{"ok": true, "task": task})
}

func (s *Server) filesList(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		bad(w, 405, "method not allowed")
		return
	}
	items, err := ga.ListSafe(s.CfgStore.Cfg.GARoot, r.URL.Query().Get("path"))
	if err != nil {
		bad(w, 400, err.Error())
		return
	}
	writeJSON(w, map[string]interface{}{"items": items})
}

func (s *Server) filesRead(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		bad(w, 405, "method not allowed")
		return
	}
	d, err := ga.ReadSafe(s.CfgStore.Cfg.GARoot, r.URL.Query().Get("path"))
	if err != nil {
		bad(w, 400, err.Error())
		return
	}
	writeJSON(w, d)
}

func (s *Server) filesWrite(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost && r.Method != http.MethodPut {
		bad(w, 405, "method not allowed")
		return
	}
	var req struct {
		Path    string `json:"path"`
		Content string `json:"content"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		bad(w, 400, err.Error())
		return
	}
	d, err := ga.WriteSafe(s.CfgStore.Cfg.GARoot, req.Path, req.Content)
	if err != nil {
		bad(w, 400, err.Error())
		return
	}
	writeJSON(w, d)
}

func (s *Server) filesTail(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		bad(w, 405, "method not allowed")
		return
	}
	lines, _ := strconv.Atoi(r.URL.Query().Get("lines"))
	d, err := ga.TailSafe(s.CfgStore.Cfg.GARoot, r.URL.Query().Get("path"), lines)
	if err != nil {
		bad(w, 400, err.Error())
		return
	}
	writeJSON(w, d)
}

func (s *Server) filesSearch(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		bad(w, 405, "method not allowed")
		return
	}
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	hits, err := ga.SearchSafe(s.CfgStore.Cfg.GARoot, r.URL.Query().Get("path"), r.URL.Query().Get("q"), limit)
	if err != nil {
		bad(w, 400, err.Error())
		return
	}
	writeJSON(w, map[string]interface{}{"hits": hits})
}

func (s *Server) configHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method == "GET" {
		writeJSON(w, s.CfgStore.Cfg)
		return
	}
	if r.Method == "PUT" {
		var c config.AppConfig
		if err := decode(r, &c); err != nil {
			bad(w, 400, err.Error())
			return
		}
		if err := s.CfgStore.Save(c); err != nil {
			bad(w, 400, err.Error())
			return
		}
		s.Svc.SetRoot(c.GARoot, c.BufferLines)
		writeJSON(w, c)
		return
	}
	bad(w, 405, "method not allowed")
}

type setupPathReq struct {
	Path string `json:"path"`
}

type setupToolStatus struct {
	Name    string `json:"name"`
	OK      bool   `json:"ok"`
	Path    string `json:"path,omitempty"`
	Version string `json:"version,omitempty"`
	Error   string `json:"error,omitempty"`
}

func (s *Server) setupEnv(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		bad(w, 405, "method not allowed")
		return
	}
	writeJSON(w, map[string]interface{}{
		"ok":      toolOK("git") && toolOK("python"),
		"tools":   []setupToolStatus{checkTool("git", "--version"), checkTool("python", "--version")},
		"checked": time.Now().Format(time.RFC3339),
	})
}

func (s *Server) setupBrowse(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		bad(w, 405, "method not allowed")
		return
	}
	var req setupPathReq
	_ = decode(r, &req)
	start := strings.TrimSpace(req.Path)
	if start == "" {
		if home, err := os.UserHomeDir(); err == nil {
			start = home
		}
	}
	selected, err := chooseDirectory(start)
	if err != nil {
		bad(w, 500, err.Error())
		return
	}
	if selected == "" {
		writeJSON(w, map[string]interface{}{"ok": false, "cancelled": true})
		return
	}
	writeJSON(w, map[string]interface{}{"ok": true, "path": selected})
}

func toolOK(name string) bool {
	_, err := exec.LookPath(name)
	return err == nil
}

func checkTool(name string, args ...string) setupToolStatus {
	st := setupToolStatus{Name: name}
	path, err := exec.LookPath(name)
	if err != nil {
		st.Error = err.Error()
		return st
	}
	st.OK = true
	st.Path = path
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	out, err := exec.CommandContext(ctx, name, args...).CombinedOutput()
	if err != nil && strings.TrimSpace(string(out)) == "" {
		st.Error = err.Error()
	}
	st.Version = strings.TrimSpace(string(out))
	return st
}

func chooseDirectory(start string) (string, error) {
	if runtime.GOOS == "windows" {
		ps := `$ErrorActionPreference='Stop'; Add-Type -AssemblyName System.Windows.Forms; $d = New-Object System.Windows.Forms.FolderBrowserDialog; $d.Description = 'Select GenericAgent directory'; $d.ShowNewFolderButton = $true; if ($env:GA_ADMIN_BROWSE_START -and (Test-Path -LiteralPath $env:GA_ADMIN_BROWSE_START)) { $d.SelectedPath = $env:GA_ADMIN_BROWSE_START }; if ($d.ShowDialog() -eq [System.Windows.Forms.DialogResult]::OK) { [Console]::OutputEncoding=[System.Text.Encoding]::UTF8; Write-Output $d.SelectedPath }`
		cmd := exec.Command("powershell", "-NoProfile", "-STA", "-Command", ps)
		cmd.Env = append(os.Environ(), "GA_ADMIN_BROWSE_START="+start)
		out, err := cmd.CombinedOutput()
		if err != nil {
			return "", fmt.Errorf("directory picker failed: %s", strings.TrimSpace(string(out)))
		}
		return strings.TrimSpace(string(out)), nil
	}
	return "", fmt.Errorf("directory picker is only supported on Windows in this build; please paste the path manually")
}

func (s *Server) setupValidate(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		bad(w, 405, "method not allowed")
		return
	}
	var req setupPathReq
	if err := decode(r, &req); err != nil {
		bad(w, 400, err.Error())
		return
	}
	root := strings.TrimSpace(req.Path)
	if root == "" {
		bad(w, 400, "path is required")
		return
	}
	abs, err := filepath.Abs(root)
	if err != nil {
		bad(w, 400, err.Error())
		return
	}
	h := ga.BuildHealth(abs)
	if h.OK {
		cfg := s.CfgStore.Cfg
		cfg.GARoot = abs
		if err := s.CfgStore.Save(cfg); err != nil {
			bad(w, 500, err.Error())
			return
		}
	}
	writeJSON(w, map[string]interface{}{"ok": h.OK, "root": abs, "health": h})
}

func (s *Server) setupInstall(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		bad(w, 405, "method not allowed")
		return
	}
	var req setupPathReq
	if err := decode(r, &req); err != nil {
		bad(w, 400, err.Error())
		return
	}
	root := strings.TrimSpace(req.Path)
	if root == "" {
		bad(w, 400, "install path is required")
		return
	}
	abs, err := filepath.Abs(root)
	if err != nil {
		bad(w, 400, err.Error())
		return
	}
	if _, err := os.Stat(filepath.Join(abs, "agentmain.py")); err == nil {
		bad(w, 409, "target already looks like a GenericAgent directory")
		return
	}
	if err := os.MkdirAll(filepath.Dir(abs), 0755); err != nil {
		bad(w, 500, err.Error())
		return
	}
	cmd := exec.Command("git", "clone", "https://github.com/lsdefine/GenericAgent", abs)
	out, err := cmd.CombinedOutput()
	if err != nil {
		bad(w, 500, strings.TrimSpace(string(out))+": "+err.Error())
		return
	}
	h := ga.BuildHealth(abs)
	if !h.OK {
		bad(w, 500, "clone completed but GenericAgent health check failed")
		return
	}
	cfg := s.CfgStore.Cfg
	cfg.GARoot = abs
	if err := s.CfgStore.Save(cfg); err != nil {
		bad(w, 500, err.Error())
		return
	}
	writeJSON(w, map[string]interface{}{"ok": true, "root": abs, "health": h})
}

func (s *Server) autostartStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		bad(w, 405, "method not allowed")
		return
	}
	writeJSON(w, autostart.StatusForCurrent())
}

func (s *Server) autostartEnable(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		bad(w, 405, "method not allowed")
		return
	}
	st, err := autostart.EnableCurrent()
	if err != nil {
		bad(w, 500, err.Error())
		return
	}
	writeJSON(w, st)
}

func (s *Server) autostartDisable(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		bad(w, 405, "method not allowed")
		return
	}
	st, err := autostart.DisableCurrent()
	if err != nil {
		bad(w, 500, err.Error())
		return
	}
	writeJSON(w, st)
}

func (s *Server) StartAutostartServices() {
	for _, name := range s.CfgStore.Cfg.ServiceAutostart {
		name = strings.TrimSpace(name)
		if name == "" {
			continue
		}
		if _, err := s.Svc.Start(name); err != nil {
			log.Printf("service autostart %s failed: %v", name, err)
		}
	}
}

func (s *Server) services(w http.ResponseWriter, r *http.Request) {
	if r.Method != "GET" {
		bad(w, 405, "method not allowed")
		return
	}
	writeJSON(w, s.servicesWithAutostart())
}

func (s *Server) servicesWithAutostart() []service.ServiceInfo {
	items := s.Svc.Discover()
	auto := map[string]bool{}
	for _, name := range s.CfgStore.Cfg.ServiceAutostart {
		auto[name] = true
	}
	for i := range items {
		items[i].Autostart = auto[items[i].Name]
	}
	return items
}
func (s *Server) summary(w http.ResponseWriter, r *http.Request) { writeJSON(w, s.Svc.Summary()) }

type nameReq struct {
	Name string `json:"name"`
}

func (s *Server) start(w http.ResponseWriter, r *http.Request) {
	var q nameReq
	if err := decode(r, &q); err != nil {
		bad(w, 400, err.Error())
		return
	}
	svc, err := s.Svc.Start(q.Name)
	if err != nil {
		bad(w, 404, err.Error())
		return
	}
	writeJSON(w, svc)
}
func (s *Server) stop(w http.ResponseWriter, r *http.Request) {
	var q nameReq
	if err := decode(r, &q); err != nil {
		bad(w, 400, err.Error())
		return
	}
	if err := s.Svc.Stop(q.Name); err != nil {
		bad(w, 400, err.Error())
		return
	}
	svc, _ := s.Svc.Find(q.Name)
	writeJSON(w, svc)
}
func (s *Server) stopAll(w http.ResponseWriter, r *http.Request) {
	s.Svc.StopAll()
	writeJSON(w, map[string]bool{"ok": true})
}
func (s *Server) serviceAutostart(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		bad(w, 405, "method not allowed")
		return
	}
	var q struct {
		Name    string `json:"name"`
		Enabled bool   `json:"enabled"`
	}
	if err := decode(r, &q); err != nil || strings.TrimSpace(q.Name) == "" {
		bad(w, 400, "bad request")
		return
	}
	if _, ok := s.Svc.Find(q.Name); !ok {
		bad(w, 404, "service not found")
		return
	}
	cfg := s.CfgStore.Cfg
	seen := map[string]bool{}
	next := []string{}
	for _, name := range cfg.ServiceAutostart {
		if name == q.Name || seen[name] {
			continue
		}
		seen[name] = true
		next = append(next, name)
	}
	if q.Enabled {
		next = append(next, q.Name)
	}
	cfg.ServiceAutostart = next
	if err := s.CfgStore.Save(cfg); err != nil {
		bad(w, 500, err.Error())
		return
	}
	writeJSON(w, map[string]interface{}{"ok": true, "services": s.servicesWithAutostart()})
}
func (s *Server) logs(w http.ResponseWriter, r *http.Request) {
	name := strings.TrimPrefix(r.URL.Path, "/api/logs/")
	lines := s.CfgStore.Cfg.LogTailLines
	writeJSON(w, map[string]interface{}{"name": name, "lines": s.Svc.Logs(name, lines)})
}

func (s *Server) models(w http.ResponseWriter, r *http.Request) {
	if r.Method == "GET" {
		d, err := s.Models.Load(false)
		if err != nil {
			bad(w, 500, err.Error())
			return
		}
		writeJSON(w, map[string]interface{}{"profiles": d.Profiles, "updated_at": d.UpdatedAt, "source": modelconfig.SourceStatus(s.CfgStore.Cfg.GARoot)})
		return
	}
	if r.Method == "PUT" {
		var p struct {
			Profiles []modelconfig.Profile `json:"profiles"`
		}
		if err := decode(r, &p); err != nil {
			bad(w, 400, err.Error())
			return
		}
		d, err := s.Models.Save(p.Profiles)
		if err != nil {
			bad(w, 400, err.Error())
			return
		}
		writeJSON(w, d)
		return
	}
	bad(w, 405, "method not allowed")
}
func (s *Server) modelsRaw(w http.ResponseWriter, r *http.Request) {
	d, err := s.Models.Load(true)
	if err != nil {
		bad(w, 500, err.Error())
		return
	}
	writeJSON(w, d)
}
func (s *Server) modelsPreview(w http.ResponseWriter, r *http.Request) {
	var p struct {
		Profiles []modelconfig.Profile `json:"profiles"`
	}
	if err := decode(r, &p); err != nil {
		bad(w, 400, err.Error())
		return
	}
	txt, err := modelconfig.Render(p.Profiles)
	if err != nil {
		bad(w, 400, err.Error())
		return
	}
	writeJSON(w, map[string]string{"python": txt})
}
func (s *Server) modelsImportMyKey(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		bad(w, 405, "method not allowed")
		return
	}
	var p struct {
		Reveal bool `json:"reveal"`
		Save   bool `json:"save"`
	}
	if r.Body != nil && r.ContentLength != 0 {
		if err := decode(r, &p); err != nil {
			bad(w, 400, err.Error())
			return
		}
	}
	d, err := modelconfig.ImportMyKey(s.CfgStore.Cfg.GARoot, p.Reveal)
	if err != nil {
		bad(w, 400, err.Error())
		return
	}
	if p.Save {
		if saved, err := s.Models.Save(d.Profiles); err == nil {
			d = saved
		} else {
			bad(w, 400, err.Error())
			return
		}
	}
	writeJSON(w, map[string]interface{}{"profiles": d.Profiles, "updated_at": d.UpdatedAt, "saved": p.Save, "masked": !p.Reveal})
}
func (s *Server) modelsExport(w http.ResponseWriter, r *http.Request) {
	var p struct {
		Profiles        []modelconfig.Profile `json:"profiles"`
		OverwriteActive bool                  `json:"overwrite_active"`
	}
	if err := decode(r, &p); err != nil {
		bad(w, 400, err.Error())
		return
	}
	_, _ = s.Models.Save(p.Profiles)
	res, err := modelconfig.Export(s.CfgStore.Cfg.GARoot, p.Profiles, p.OverwriteActive)
	if err != nil {
		bad(w, 400, err.Error())
		return
	}
	writeJSON(w, res)
}

func (s *Server) static(w http.ResponseWriter, r *http.Request) {
	if s.Static == nil {
		bad(w, 404, "web dist not embedded")
		return
	}
	p := strings.TrimPrefix(r.URL.Path, "/")
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
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if conn, err := net.DialTimeout("tcp", fmt.Sprintf("127.0.0.1:%d", port), 200*time.Millisecond); err == nil {
			_ = conn.Close()
			return nil
		}
		time.Sleep(150 * time.Millisecond)
	}
	return nil
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

func (s *Server) filesOpen(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		bad(w, 405, "method not allowed")
		return
	}
	var req struct {
		Path string `json:"path"`
		Mode string `json:"mode"`
	}
	if err := decode(r, &req); err != nil {
		bad(w, 400, "bad request")
		return
	}
	p := strings.TrimSpace(req.Path)
	if p == "" || !filepath.IsAbs(p) {
		bad(w, 400, "absolute path required")
		return
	}
	info, err := os.Stat(p)
	if err != nil {
		bad(w, 404, err.Error())
		return
	}
	mode := strings.ToLower(strings.TrimSpace(req.Mode))
	if mode == "" {
		mode = "file"
	}
	if err := openLocalPath(p, info.IsDir(), mode); err != nil {
		bad(w, 500, err.Error())
		return
	}
	writeJSON(w, map[string]bool{"ok": true})
}

func openLocalPath(p string, isDir bool, mode string) error {
	switch runtime.GOOS {
	case "windows":
		if mode == "folder" {
			if isDir {
				return exec.Command("explorer", p).Start()
			}
			return exec.Command("explorer", "/select,"+p).Start()
		}
		return exec.Command("rundll32", "url.dll,FileProtocolHandler", p).Start()
	case "darwin":
		if mode == "folder" {
			if isDir {
				return exec.Command("open", p).Start()
			}
			return exec.Command("open", "-R", p).Start()
		}
		return exec.Command("open", p).Start()
	default:
		if mode == "folder" && !isDir {
			p = filepath.Dir(p)
		}
		return exec.Command("xdg-open", p).Start()
	}
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
