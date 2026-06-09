package api

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"

	"genericagent-admin-go/internal/autostart"
	"genericagent-admin-go/internal/config"
	"genericagent-admin-go/internal/ga"
)

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
		c = s.CfgStore.Cfg
		s.Svc.SetRoot(c.GARoot, c.BufferLines)
		writeJSON(w, c)
		return
	}
	bad(w, 405, "method not allowed")
}

type setupPathReq struct {
	Path string `json:"path"`
}

func unsafeSetupPath(p string) bool {
	clean := filepath.Clean(strings.TrimSpace(p))
	if clean == "" || clean == "." {
		return true
	}
	vol := filepath.VolumeName(clean)
	rest := strings.TrimPrefix(clean, vol)
	rest = filepath.Clean(rest)
	return rest == "" || rest == "." || rest == string(filepath.Separator)
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
		"tools":   []setupToolStatus{checkTool("git", "--version"), checkTool("python", "--version"), checkTool("uv", "--version"), checkTool("npm", "--version")},
		"checked": time.Now().Format(time.RFC3339),
	})
}

func (s *Server) setupBrowse(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		bad(w, 405, "method not allowed")
		return
	}
	var req setupPathReq
	if err := decode(r, &req); err != nil {
		bad(w, 400, err.Error())
		return
	}
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
	cmd := exec.CommandContext(ctx, name, args...)
	hideChildWindow(cmd)
	out, err := cmd.CombinedOutput()
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
		hideChildWindow(cmd)
		cmd.Env = append(os.Environ(), "GA_ADMIN_BROWSE_START="+start)
		out, err := cmd.CombinedOutput()
		if err != nil {
			return "", fmt.Errorf("directory picker failed: %s", strings.TrimSpace(string(out)))
		}
		return strings.TrimSpace(string(out)), nil
	}
	return "", fmt.Errorf("directory picker is only supported on Windows in this build; please paste the path manually")
}

func runGitCommand(ctx context.Context, root string, args ...string) (string, error) {
	return runGitCommandFunc(ctx, root, args...)
}

const setupInstallCloneTimeout = 5 * time.Minute

func runSetupClone(ctx context.Context, dest string) (string, error) {
	return runSetupCloneFunc(ctx, dest)
}

var runSetupCloneFunc = func(ctx context.Context, dest string) (string, error) {
	cmd := exec.CommandContext(ctx, "git", "clone", "https://github.com/lsdefine/GenericAgent", dest)
	hideChildWindow(cmd)
	out, err := cmd.CombinedOutput()
	return strings.TrimSpace(string(out)), err
}

var runGitCommandFunc = func(ctx context.Context, root string, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, "git", args...)
	cmd.Dir = root
	hideChildWindow(cmd)
	out, err := cmd.CombinedOutput()
	text := strings.TrimSpace(string(out))
	if err != nil {
		if text == "" {
			text = err.Error()
		}
		return text, fmt.Errorf("git %s failed: %w", strings.Join(args, " "), err)
	}
	return text, nil
}

func gaGitStatusForRoot(ctx context.Context, abs string) (map[string]interface{}, error) {
	if st, err := os.Stat(filepath.Join(abs, ".git")); err != nil || !st.IsDir() {
		return nil, errors.New("GA root is not a git repository")
	}
	branch, _ := runGitCommand(ctx, abs, "branch", "--show-current")
	if strings.TrimSpace(branch) == "" {
		branch, _ = runGitCommand(ctx, abs, "rev-parse", "--short", "HEAD")
	}
	commit, _ := runGitCommand(ctx, abs, "rev-parse", "--short", "HEAD")
	status, _ := runGitCommand(ctx, abs, "status", "--short")
	upstream, _ := runGitCommand(ctx, abs, "rev-parse", "--abbrev-ref", "--symbolic-full-name", "@{u}")
	ahead := 0
	behind := 0
	if strings.TrimSpace(upstream) != "" {
		aheadBehind, err := runGitCommand(ctx, abs, "rev-list", "--left-right", "--count", "HEAD...@{u}")
		if err != nil {
			return nil, err
		}
		parts := strings.Fields(aheadBehind)
		if len(parts) < 2 {
			return nil, fmt.Errorf("unexpected git ahead/behind output: %q", aheadBehind)
		}
		ahead, err = strconv.Atoi(parts[0])
		if err != nil {
			return nil, fmt.Errorf("invalid git ahead count %q: %w", parts[0], err)
		}
		behind, err = strconv.Atoi(parts[1])
		if err != nil {
			return nil, fmt.Errorf("invalid git behind count %q: %w", parts[1], err)
		}
	}
	return map[string]interface{}{
		"ok": true, "root": abs, "branch": strings.TrimSpace(branch), "commit": strings.TrimSpace(commit),
		"upstream": strings.TrimSpace(upstream), "ahead": ahead, "behind": behind,
		"latest": behind == 0, "dirty": strings.TrimSpace(status) != "", "status": status,
	}, nil
}

func (s *Server) gaGitStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		bad(w, 405, "method not allowed")
		return
	}
	root := strings.TrimSpace(s.CfgStore.Cfg.GARoot)
	if root == "" {
		bad(w, 400, "ga_root is not configured")
		return
	}
	abs, err := filepath.Abs(root)
	if err != nil {
		bad(w, 400, err.Error())
		return
	}

	// Keep page load fast: local git status by default. Network fetch can block
	// on slow remotes or credential prompts, so run it only when explicitly asked.
	remote := r.URL.Query().Get("remote") == "1" || strings.EqualFold(r.URL.Query().Get("fetch"), "true")
	timeout := 5 * time.Second
	if remote {
		timeout = 30 * time.Second
	}
	ctx, cancel := context.WithTimeout(r.Context(), timeout)
	defer cancel()

	var fetchOut string
	var fetchErr error
	if remote {
		fetchOut, fetchErr = runGitCommand(ctx, abs, "fetch", "--all", "--prune")
	}
	st, err := gaGitStatusForRoot(ctx, abs)
	if err != nil {
		bad(w, 400, err.Error())
		return
	}
	st["remote_checked"] = remote
	if fetchErr != nil {
		st["fetch_error"] = strings.TrimSpace(fetchOut + "\n" + fetchErr.Error())
	}
	writeJSON(w, st)
}

func (s *Server) gaGitUpdate(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		bad(w, 405, "method not allowed")
		return
	}
	root := strings.TrimSpace(s.CfgStore.Cfg.GARoot)
	if root == "" {
		bad(w, 400, "ga_root is not configured")
		return
	}
	abs, err := filepath.Abs(root)
	if err != nil {
		bad(w, 400, err.Error())
		return
	}
	if st, err := os.Stat(filepath.Join(abs, ".git")); err != nil || !st.IsDir() {
		bad(w, 400, "GA root is not a git repository")
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 120*time.Second)
	defer cancel()
	before, _ := runGitCommand(ctx, abs, "rev-parse", "--short", "HEAD")
	branch, _ := runGitCommand(ctx, abs, "branch", "--show-current")
	statusBefore, _ := runGitCommand(ctx, abs, "status", "--short")
	fetchOut, err := runGitCommand(ctx, abs, "fetch", "--all", "--prune")
	if err != nil {
		bad(w, 500, strings.TrimSpace(fetchOut+"\n"+err.Error()))
		return
	}
	pullOut, err := runGitCommand(ctx, abs, "pull", "--ff-only")
	if err != nil {
		bad(w, 500, strings.TrimSpace(pullOut+"\n"+err.Error()))
		return
	}
	after, _ := runGitCommand(ctx, abs, "rev-parse", "--short", "HEAD")
	statusAfter, _ := runGitCommand(ctx, abs, "status", "--short")
	writeJSON(w, map[string]interface{}{
		"ok": true, "root": abs, "branch": branch, "before": before, "after": after,
		"changed": before != after, "status_before": statusBefore, "status_after": statusAfter,
		"fetch": fetchOut, "pull": pullOut,
	})
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
	if unsafeSetupPath(abs) {
		bad(w, 400, "refusing to install GenericAgent into filesystem root")
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
	ctx, cancel := context.WithTimeout(r.Context(), setupInstallCloneTimeout)
	defer cancel()
	out, err := runSetupClone(ctx, abs)
	if err != nil {
		bad(w, 500, strings.TrimSpace(out)+": "+err.Error())
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
