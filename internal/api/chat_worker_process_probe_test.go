package api

import (
	"errors"
	"os"
	"syscall"
	"testing"
)

func assertUnixChatTestProcessReaped(t *testing.T, process *os.Process) {
	t.Helper()
	// Go 1.23 的 Linux pidfd 二次 Wait 返回 ECHILD，表示子进程已经回收。
	// 同时检查 Kill 不能成功，保留“进程确已结束”的独立断言。
	if _, err := process.Wait(); !errors.Is(err, os.ErrProcessDone) && !errors.Is(err, syscall.ECHILD) {
		t.Fatalf("retirement did not reap subprocess: %v", err)
	}
	if err := process.Kill(); !errors.Is(err, os.ErrProcessDone) {
		t.Fatalf("retired subprocess remains live: %v", err)
	}
}
