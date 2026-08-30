package api

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

const chatFeishuBridgeHelperExit = "chat-feishu-helper-exit"
const chatFeishuBridgeHelperLive = "chat-feishu-helper-live"

func TestChatFeishuBridgeHelperProcess(t *testing.T) {
	mode := ""
	for _, arg := range os.Args {
		if arg == chatFeishuBridgeHelperExit || arg == chatFeishuBridgeHelperLive {
			mode = arg
			break
		}
	}
	switch mode {
	case chatFeishuBridgeHelperExit:
		os.Exit(23)
	case chatFeishuBridgeHelperLive:
		time.Sleep(10 * time.Second)
	}
}

func configuredChatFeishuTestServer(t *testing.T) *Server {
	t.Helper()
	root := t.TempDir()
	content := "feishu_admin_app_id = \"admin-app\"\nfeishu_admin_app_secret = \"admin-secret\"\n"
	if err := os.WriteFile(filepath.Join(root, "mykey.py"), []byte(content), 0600); err != nil {
		t.Fatal(err)
	}
	return newServiceHandlerTestServer(t, root)
}

func withChatFeishuBridgeHelper(t *testing.T, mode string, grace time.Duration, scriptPath *string) {
	t.Helper()
	previousFactory := newChatFeishuBridgeCommand
	previousGrace := chatFeishuBridgeStartupGrace
	newChatFeishuBridgeCommand = func(_, script string) *exec.Cmd {
		if scriptPath != nil {
			*scriptPath = script
		}
		return exec.Command(os.Args[0], "-test.run=^TestChatFeishuBridgeHelperProcess$", "--", mode)
	}
	chatFeishuBridgeStartupGrace = grace
	t.Cleanup(func() {
		newChatFeishuBridgeCommand = previousFactory
		chatFeishuBridgeStartupGrace = previousGrace
	})
}

func TestStartChatFeishuBridgeRejectsImmediateChildExitAndCleansUp(t *testing.T) {
	s := configuredChatFeishuTestServer(t)
	var script string
	withChatFeishuBridgeHelper(t, chatFeishuBridgeHelperExit, 200*time.Millisecond, &script)

	err := s.StartChatFeishuBridge()
	if err == nil || !strings.Contains(err.Error(), "exited during startup") {
		t.Fatalf("StartChatFeishuBridge error=%v, want startup-exit error", err)
	}
	if strings.Contains(err.Error(), "admin-secret") {
		t.Fatalf("startup error leaked credential: %v", err)
	}
	if s.IsChatFeishuBridgeRunning() || s.chatFeishuBridgeServer != nil {
		t.Fatal("failed bridge remained visible as running")
	}
	if script == "" {
		t.Fatal("bridge script path was not captured")
	}
	if _, statErr := os.Stat(script); !os.IsNotExist(statErr) {
		t.Fatalf("temporary bridge script was not removed: %v", statErr)
	}
}

func TestStartChatFeishuBridgeReportsRunningOnlyAfterStartupGrace(t *testing.T) {
	s := configuredChatFeishuTestServer(t)
	withChatFeishuBridgeHelper(t, chatFeishuBridgeHelperLive, 30*time.Millisecond, nil)
	t.Cleanup(s.StopChatFeishuBridge)

	if err := s.StartChatFeishuBridge(); err != nil {
		t.Fatalf("StartChatFeishuBridge: %v", err)
	}
	if !s.IsChatFeishuBridgeRunning() {
		t.Fatal("surviving bridge was not reported running after startup grace")
	}
	s.StopChatFeishuBridge()
	if s.IsChatFeishuBridgeRunning() {
		t.Fatal("stopped bridge remained visible as running")
	}
}
