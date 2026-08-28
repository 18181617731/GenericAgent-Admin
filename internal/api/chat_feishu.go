package api

import (
	_ "embed"
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

func newChatFeishuBridgeCommand(python, script string) *exec.Cmd {
	cmd := exec.Command(python, script)
	hideChildWindow(cmd)
	return cmd
}

// StartChatFeishuBridge starts the optional Admin-specific Feishu session bridge.
// The child reads Feishu credentials itself; Admin never puts secrets in argv or logs.
func (s *Server) StartChatFeishuBridge() {
	if s == nil {
		return
	}
	s.chatFeishuBridgeMu.Lock()
	defer s.chatFeishuBridgeMu.Unlock()
	if s.chatFeishuBridgeCmd != nil {
		return
	}
	cfg := s.CfgStore.Snapshot()
	configPath := s.channelConfigPath()
	if unsafeChannelGARoot(cfg.GARoot) {
		return
	}
	if _, err := os.Stat(configPath); err != nil {
		return
	}
	script, err := writeChatFeishuBridgeScript()
	if err != nil {
		log.Printf("chat feishu bridge: write script failed: %v", err)
		return
	}
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		_ = os.Remove(script)
		log.Printf("chat feishu bridge: listen failed: %v", err)
		return
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
		"GA_ADMIN_FEISHU_STATE="+filepath.Join(s.CfgStore.Root, "feishu_chat_bindings.json"),
		"PYTHONUNBUFFERED=1",
	)
	cmd.Stdout = os.Stderr
	cmd.Stderr = os.Stderr
	if err := cmd.Start(); err != nil {
		log.Printf("chat feishu bridge: start %q failed: %v", python, err)
		_ = listener.Close()
		_ = os.Remove(script)
		return
	}
	s.chatFeishuBridgeCmd = cmd
	s.chatFeishuBridgeServer = httpServer
	go func() { _ = httpServer.Serve(listener) }()
	go func() {
		err := cmd.Wait()
		_ = httpServer.Close()
		_ = os.Remove(script)
		s.chatFeishuBridgeMu.Lock()
		unexpected := s.chatFeishuBridgeCmd == cmd
		if unexpected {
			s.chatFeishuBridgeCmd = nil
			s.chatFeishuBridgeServer = nil
		}
		s.chatFeishuBridgeMu.Unlock()
		if unexpected {
			log.Printf("chat feishu bridge: process exited: %v", err)
		}
	}()
}

// RestartChatFeishuBridge applies channel credential changes without restarting Admin.
func (s *Server) RestartChatFeishuBridge() {
	s.StopChatFeishuBridge()
	s.StartChatFeishuBridge()
}

func (s *Server) StopChatFeishuBridge() {
	if s == nil {
		return
	}
	s.chatFeishuBridgeMu.Lock()
	cmd, server := s.chatFeishuBridgeCmd, s.chatFeishuBridgeServer
	s.chatFeishuBridgeCmd, s.chatFeishuBridgeServer = nil, nil
	s.chatFeishuBridgeMu.Unlock()
	if server != nil {
		_ = server.Close()
	}
	if cmd != nil && cmd.Process != nil {
		_ = cmd.Process.Kill()
	}
}
