package api

import (
	_ "embed"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"time"
)

//go:embed chat_feishu_bridge.py
var embeddedChatFeishuBridge []byte

func writeChatFeishuBridgeScript() (string, error) {
	file, err := os.CreateTemp("", "ga-admin-chat-feishu-*.py")
	if err != nil {
		return "", err
	}
	path := file.Name()
	if _, err = file.Write(embeddedChatFeishuBridge); err == nil {
		err = file.Close()
	} else {
		_ = file.Close()
	}
	if err != nil {
		_ = os.Remove(path)
		return "", err
	}
	return path, nil
}

var newChatFeishuBridgeCommand = func(python, script string) *exec.Cmd {
	cmd := exec.Command(python, script)
	hideChildWindow(cmd)
	return cmd
}

var chatFeishuBridgeStartupGrace = 300 * time.Millisecond

const adminFeishuServiceName = "admin/feishuapp.py"

// StartChatFeishuBridge starts the optional Admin-specific Feishu session bridge.
// The child reads its dedicated credentials itself; Admin never puts secrets in argv or logs.
func (s *Server) StartChatFeishuBridge() error {
	if s == nil || s.CfgStore == nil {
		return fmt.Errorf("Feishu Admin sync service is unavailable")
	}
	s.chatFeishuBridgeMu.Lock()
	defer s.chatFeishuBridgeMu.Unlock()
	return s.startChatFeishuBridgeLocked()
}

// startChatFeishuBridgeLocked starts the bridge while chatFeishuBridgeMu is held.
func (s *Server) startChatFeishuBridgeLocked() error {
	if s.chatFeishuBridgeCmd != nil {
		return nil
	}
	cfg := s.CfgStore.Snapshot()
	configPath := s.channelConfigPath()
	if unsafeChannelGARoot(cfg.GARoot) {
		return fmt.Errorf("unsafe GenericAgent root for Feishu Admin sync")
	}
	content, err := os.ReadFile(configPath)
	if err != nil {
		return fmt.Errorf("configure the Feishu Admin sync channel before starting it: %w", err)
	}
	values := parseChannelAssignments(string(content))
	if values["feishu_admin_app_id"] == "" || values["feishu_admin_app_secret"] == "" {
		return fmt.Errorf("configure the Feishu Admin sync channel before starting it")
	}
	script, err := writeChatFeishuBridgeScript()
	if err != nil {
		return fmt.Errorf("write Feishu Admin sync bridge: %w", err)
	}
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		_ = os.Remove(script)
		return fmt.Errorf("listen for Feishu Admin sync bridge: %w", err)
	}
	token := randomChatHubToken()
	httpServer := &http.Server{Handler: s.chatSessionBridgeAPI(token, true), ReadHeaderTimeout: 5 * time.Second}
	python := resolveUsablePythonForRoot(cfg.GARoot, cfg.EffectivePython, cfg.PythonFallbackRoots)
	cmd := newChatFeishuBridgeCommand(python, script)
	cmd.Env = append(
		pythonEnvWithAdminProxy(cfg),
		"GA_ROOT="+cfg.GARoot,
		"GA_ADMIN_FEISHU_API=http://"+listener.Addr().String(),
		"GA_ADMIN_FEISHU_TOKEN="+token,
		"GA_ADMIN_FEISHU_CONFIG="+configPath,
		"GA_ADMIN_FEISHU_STATE="+filepath.Join(s.CfgStore.Root, "feishu_admin_chat_bindings.json"),
		"PYTHONUNBUFFERED=1",
	)
	cmd.Stdout = os.Stderr
	cmd.Stderr = os.Stderr
	if err := cmd.Start(); err != nil {
		_ = listener.Close()
		_ = os.Remove(script)
		return fmt.Errorf("start Feishu Admin sync bridge with %q: %w", python, err)
	}
	go func() { _ = httpServer.Serve(listener) }()
	exited := make(chan error, 1)
	go func() { exited <- cmd.Wait() }()
	timer := time.NewTimer(chatFeishuBridgeStartupGrace)
	defer timer.Stop()
	select {
	case err := <-exited:
		_ = httpServer.Close()
		_ = os.Remove(script)
		if err == nil {
			return fmt.Errorf("Feishu Admin sync bridge exited during startup")
		}
		return fmt.Errorf("Feishu Admin sync bridge exited during startup: %w", err)
	case <-timer.C:
	}
	s.chatFeishuBridgeCmd = cmd
	s.chatFeishuBridgeServer = httpServer
	s.chatFeishuBridgeStartedAt = time.Now().UTC()
	go func() {
		err := <-exited
		_ = httpServer.Close()
		_ = os.Remove(script)
		s.chatFeishuBridgeMu.Lock()
		unexpected := s.chatFeishuBridgeCmd == cmd
		if unexpected {
			s.chatFeishuBridgeCmd = nil
			s.chatFeishuBridgeServer = nil
			s.chatFeishuBridgeStartedAt = time.Time{}
		}
		s.chatFeishuBridgeMu.Unlock()
		if unexpected {
			log.Printf("chat feishu bridge: process exited: %v", err)
		}
	}()
	return nil
}

// RestartChatFeishuBridgeIfRunning applies credential changes without starting a stopped service.
func (s *Server) RestartChatFeishuBridgeIfRunning() error {
	if s == nil || s.CfgStore == nil {
		return fmt.Errorf("Feishu Admin sync service is unavailable")
	}
	s.chatFeishuBridgeMu.Lock()
	defer s.chatFeishuBridgeMu.Unlock()
	if s.chatFeishuBridgeCmd == nil {
		return nil
	}
	cmd, server := s.chatFeishuBridgeCmd, s.chatFeishuBridgeServer
	s.chatFeishuBridgeCmd, s.chatFeishuBridgeServer = nil, nil
	s.chatFeishuBridgeStartedAt = time.Time{}
	if server != nil {
		_ = server.Close()
	}
	if cmd.Process != nil {
		_ = cmd.Process.Kill()
	}
	return s.startChatFeishuBridgeLocked()
}

func (s *Server) chatFeishuBridgeStatus() (running bool, pid *int, startedAt string) {
	if s == nil {
		return false, nil, ""
	}
	s.chatFeishuBridgeMu.Lock()
	defer s.chatFeishuBridgeMu.Unlock()
	if s.chatFeishuBridgeCmd == nil || s.chatFeishuBridgeCmd.Process == nil {
		return false, nil, ""
	}
	n := s.chatFeishuBridgeCmd.Process.Pid
	if !s.chatFeishuBridgeStartedAt.IsZero() {
		startedAt = s.chatFeishuBridgeStartedAt.Format(time.RFC3339)
	}
	return true, &n, startedAt
}

func (s *Server) IsChatFeishuBridgeRunning() bool {
	running, _, _ := s.chatFeishuBridgeStatus()
	return running
}

func (s *Server) StopChatFeishuBridge() {
	if s == nil {
		return
	}
	s.chatFeishuBridgeMu.Lock()
	cmd, server := s.chatFeishuBridgeCmd, s.chatFeishuBridgeServer
	s.chatFeishuBridgeCmd, s.chatFeishuBridgeServer = nil, nil
	s.chatFeishuBridgeStartedAt = time.Time{}
	s.chatFeishuBridgeMu.Unlock()
	if server != nil {
		_ = server.Close()
	}
	if cmd != nil && cmd.Process != nil {
		_ = cmd.Process.Kill()
	}
}
