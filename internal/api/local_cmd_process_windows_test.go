//go:build windows

package api

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"golang.org/x/sys/windows"
)

func TestRemoteCmdUsesConPTYWithoutVisibleConsole(t *testing.T) {
	if localCmdProcessCreateFlags&windows.CREATE_NEW_CONSOLE != 0 {
		t.Fatalf("ConPTY flags=%#x unexpectedly request CREATE_NEW_CONSOLE", localCmdProcessCreateFlags)
	}
	if localCmdProcessCreateFlags&windows.EXTENDED_STARTUPINFO_PRESENT == 0 {
		t.Fatal("ConPTY flags do not request extended startup info")
	}
	var startup windows.StartupInfoEx
	startup.StartupInfo.Flags = localCmdProcessStartupFlags
	if startup.StartupInfo.Flags&windows.STARTF_USESTDHANDLES == 0 {
		t.Fatal("ConPTY startup info must mark std handles as explicitly empty")
	}
	if startup.StartupInfo.StdInput != 0 || startup.StartupInfo.StdOutput != 0 || startup.StartupInfo.StdErr != 0 {
		t.Fatal("ConPTY startup info must not provide parent standard handles")
	}
}

func TestRemoteCmdConPTYIntegration(t *testing.T) {
	if testing.Short() {
		t.Skip("ConPTY integration test disabled in short mode")
	}
	dir := filepath.Join(t.TempDir(), "中文 local cmd")
	if err := ensureLocalCmdDirectory(dir); err != nil {
		t.Fatal(err)
	}
	process, err := newLocalCmdProcess(dir, 100, 30)
	if err != nil {
		t.Fatal(err)
	}
	defer process.Close()
	output := collectLocalCmdOutput(process)
	if _, err := waitLocalCmdOutput(output, ">", localCmdIntegrationStageTimeout); err != nil {
		t.Fatalf("initial prompt: %v", err)
	}
	t.Log("initial prompt received")
	if err := writeLocalCmdInput(process, "echo REMOTE_CMD_OK\r\n"); err != nil {
		t.Fatal(err)
	}
	if text, err := waitLocalCmdOutput(output, "REMOTE_CMD_OK", localCmdIntegrationStageTimeout); err != nil {
		t.Fatalf("first command: %v output=%q", err, text)
	}
	t.Log("echo marker received")
	if err := writeLocalCmdInput(process, "cd\r\n"); err != nil {
		t.Fatal(err)
	}
	text, err := waitLocalCmdOutput(output, filepath.Base(dir), localCmdIntegrationStageTimeout)
	if err != nil {
		t.Fatalf("cd output did not include %q: %v", filepath.Base(dir), err)
	}
	if !strings.Contains(strings.ToLower(text), strings.ToLower(filepath.Base(dir))) {
		t.Fatalf("cd output=%q", text)
	}
	t.Logf("working directory marker received: %s", filepath.Base(dir))
	if err := writeLocalCmdInput(process, "echo 中文_CMD_OK\r\nexit\r\n"); err != nil {
		t.Fatal(err)
	}
	if _, err := waitLocalCmdOutput(output, "中文_CMD_OK", localCmdIntegrationStageTimeout); err != nil {
		t.Fatal(err)
	}
	exitCode, err := waitLocalCmdExit(process, localCmdIntegrationStageTimeout)
	if err != nil {
		t.Fatal(err)
	}
	if exitCode != 0 {
		t.Fatalf("cmd exit code=%d, want 0", exitCode)
	}
}

const localCmdIntegrationStageTimeout = 5 * time.Second

func waitLocalCmdExit(process localCmdProcess, timeout time.Duration) (int, error) {
	native, ok := process.(*windowsLocalCmdProcess)
	if !ok {
		return process.Wait()
	}
	wait, err := windows.WaitForSingleObject(native.process, uint32(timeout.Milliseconds()))
	if err != nil {
		return -1, err
	}
	if wait != windows.WAIT_OBJECT_0 {
		return -1, errors.New("timed out waiting for cmd.exe exit")
	}
	return process.Wait()
}

func ensureLocalCmdDirectory(path string) error {
	return os.MkdirAll(path, 0700)
}

func writeLocalCmdInput(process localCmdProcess, command string) error {
	data := []byte(command)
	count, err := process.Write(data)
	if err == nil && count != len(data) {
		return io.ErrShortWrite
	}
	return err
}

func collectLocalCmdOutput(process localCmdProcess) <-chan []byte {
	output := make(chan []byte, 16)
	go func() {
		defer close(output)
		buffer := make([]byte, 4096)
		for {
			count, err := process.Read(buffer)
			if count > 0 {
				output <- append([]byte(nil), buffer[:count]...)
			}
			if err != nil {
				return
			}
		}
	}()
	return output
}

func waitLocalCmdOutput(output <-chan []byte, marker string, timeout time.Duration) (string, error) {
	deadline := time.NewTimer(timeout)
	defer deadline.Stop()
	var buffer bytes.Buffer
	for {
		select {
		case chunk, ok := <-output:
			if !ok {
				return buffer.String(), errors.New("ConPTY output closed before marker")
			}
			buffer.Write(chunk)
			if strings.Contains(buffer.String(), marker) {
				return buffer.String(), nil
			}
		case <-deadline.C:
			return buffer.String(), fmt.Errorf("timed out waiting for ConPTY marker output=%q", buffer.String())
		}
	}
}
