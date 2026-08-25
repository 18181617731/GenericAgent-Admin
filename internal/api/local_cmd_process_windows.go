//go:build windows

package api

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sync"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"
)

const (
	localCmdProcessExecutable   = "cmd.exe"
	localCmdProcessInitCommand  = "set TERM=xterm-256color && set COLORTERM=truecolor && chcp 65001>nul"
	localCmdProcessExitCode     = 1
	localCmdProcessCreateFlags  = windows.EXTENDED_STARTUPINFO_PRESENT
	localCmdProcessStartupFlags = windows.STARTF_USESTDHANDLES
)

func localCmdSupported() bool { return true }

type windowsLocalCmdProcess struct {
	mu      sync.Mutex
	process windows.Handle
	input   windows.Handle
	output  windows.Handle
	console windows.Handle
	closed  bool
}

func newLocalCmdProcess(dir string, cols, rows int) (localCmdProcess, error) {
	executable, err := resolveRemoteCmdExecutable()
	if err != nil {
		return nil, err
	}
	return createWindowsLocalCmdProcess(executable, dir, cols, rows)
}

func resolveRemoteCmdExecutable() (string, error) {
	systemDir, err := windows.GetSystemDirectory()
	if err != nil {
		return "", fmt.Errorf("resolve Windows system directory: %w", err)
	}
	path := filepath.Clean(filepath.Join(systemDir, localCmdProcessExecutable))
	if !filepath.IsAbs(path) || !stringsEqualFold(filepath.Base(path), localCmdProcessExecutable) {
		return "", fmt.Errorf("invalid cmd.exe system path %q", path)
	}
	info, err := os.Stat(path)
	if err != nil || info.IsDir() {
		if err == nil {
			err = fmt.Errorf("path is a directory")
		}
		return "", fmt.Errorf("invalid cmd.exe path %q: %w", path, err)
	}
	return path, nil
}

func stringsEqualFold(a, b string) bool {
	return len(a) == len(b) && equalFoldASCII(a, b)
}

func equalFoldASCII(a, b string) bool {
	for i := range a {
		x, y := a[i], b[i]
		if x >= 'A' && x <= 'Z' {
			x += 'a' - 'A'
		}
		if y >= 'A' && y <= 'Z' {
			y += 'a' - 'A'
		}
		if x != y {
			return false
		}
	}
	return true
}

func createWindowsLocalCmdProcess(executable, dir string, cols, rows int) (*windowsLocalCmdProcess, error) {
	inputRead, inputWrite, outputRead, outputWrite, err := createLocalCmdPipes()
	if err != nil {
		return nil, err
	}
	console := windows.Handle(0)
	if err = windows.CreatePseudoConsole(windows.Coord{X: int16(cols), Y: int16(rows)}, inputRead, outputWrite, 0, &console); err != nil {
		closeHandles(inputRead, inputWrite, outputRead, outputWrite)
		return nil, fmt.Errorf("create ConPTY: %w", err)
	}
	process, err := startConPTYProcess(executable, dir, console)
	_ = windows.CloseHandle(inputRead)
	_ = windows.CloseHandle(outputWrite)
	if err != nil {
		windows.ClosePseudoConsole(console)
		closeHandles(inputWrite, outputRead)
		return nil, err
	}
	return &windowsLocalCmdProcess{
		process: process,
		input:   inputWrite,
		output:  outputRead,
		console: console,
	}, nil
}

func createLocalCmdPipes() (windows.Handle, windows.Handle, windows.Handle, windows.Handle, error) {
	input := make([]windows.Handle, 2)
	if err := windows.CreatePipe(&input[0], &input[1], nil, 0); err != nil {
		return 0, 0, 0, 0, fmt.Errorf("create ConPTY input pipe: %w", err)
	}
	output := make([]windows.Handle, 2)
	if err := windows.CreatePipe(&output[0], &output[1], nil, 0); err != nil {
		closeHandles(input...)
		return 0, 0, 0, 0, fmt.Errorf("create ConPTY output pipe: %w", err)
	}
	return input[0], input[1], output[0], output[1], nil
}

func localCmdProcessCommandLine(executable string) string {
	return windows.ComposeCommandLine([]string{executable, "/D", "/Q", "/K", localCmdProcessInitCommand})
}

func startConPTYProcess(executable, dir string, console windows.Handle) (windows.Handle, error) {
	attributes, err := windows.NewProcThreadAttributeList(1)
	if err != nil {
		return 0, fmt.Errorf("create process attribute list: %w", err)
	}
	defer attributes.Delete()
	consolePointer := *(*unsafe.Pointer)(unsafe.Pointer(&console))
	if err := attributes.Update(windows.PROC_THREAD_ATTRIBUTE_PSEUDOCONSOLE, consolePointer, unsafe.Sizeof(console)); err != nil {
		return 0, fmt.Errorf("attach ConPTY attribute: %w", err)
	}
	applicationName, err := windows.UTF16PtrFromString(executable)
	if err != nil {
		return 0, fmt.Errorf("encode cmd.exe path: %w", err)
	}
	currentDirectory, err := windows.UTF16PtrFromString(dir)
	if err != nil {
		return 0, fmt.Errorf("encode working directory: %w", err)
	}
	commandLine, err := windows.UTF16FromString(localCmdProcessCommandLine(executable))
	if err != nil {
		return 0, fmt.Errorf("encode cmd.exe command line: %w", err)
	}
	startup := windows.StartupInfoEx{}
	startup.Cb = uint32(unsafe.Sizeof(startup))
	startup.Flags = localCmdProcessStartupFlags
	startup.ProcThreadAttributeList = attributes.List()
	var processInfo windows.ProcessInformation
	err = windows.CreateProcess(applicationName, &commandLine[0], nil, nil, false, localCmdProcessCreateFlags, nil, currentDirectory, &startup.StartupInfo, &processInfo)
	if err != nil {
		return 0, fmt.Errorf("start ConPTY cmd.exe: %w", err)
	}
	_ = windows.CloseHandle(processInfo.Thread)
	return processInfo.Process, nil
}

func closeHandles(handles ...windows.Handle) {
	for _, handle := range handles {
		if handle != 0 && handle != windows.InvalidHandle {
			_ = windows.CloseHandle(handle)
		}
	}
}

func (p *windowsLocalCmdProcess) Read(data []byte) (int, error) {
	var count uint32
	err := windows.ReadFile(p.output, data, &count, nil)
	if err == syscall.ERROR_BROKEN_PIPE {
		return int(count), io.EOF
	}
	return int(count), err
}

func (p *windowsLocalCmdProcess) Write(data []byte) (int, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.closed || p.input == 0 {
		return 0, io.ErrClosedPipe
	}
	var count uint32
	err := windows.WriteFile(p.input, data, &count, nil)
	return int(count), err
}

func (p *windowsLocalCmdProcess) Resize(cols, rows int) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.closed {
		return io.ErrClosedPipe
	}
	return windows.ResizePseudoConsole(p.console, windows.Coord{X: int16(cols), Y: int16(rows)})
}

func (p *windowsLocalCmdProcess) Kill() error {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.closed || p.process == 0 {
		return nil
	}
	err := windows.TerminateProcess(p.process, localCmdProcessExitCode)
	if err == syscall.ERROR_ACCESS_DENIED {
		return nil
	}
	return err
}

func (p *windowsLocalCmdProcess) Wait() (int, error) {
	if p.process == 0 {
		return -1, io.ErrClosedPipe
	}
	if _, err := windows.WaitForSingleObject(p.process, windows.INFINITE); err != nil {
		return -1, err
	}
	var code uint32
	if err := windows.GetExitCodeProcess(p.process, &code); err != nil {
		return -1, err
	}
	return int(code), nil
}

func (p *windowsLocalCmdProcess) Close() error {
	p.mu.Lock()
	if p.closed {
		p.mu.Unlock()
		return nil
	}
	if p.process != 0 {
		_ = windows.TerminateProcess(p.process, localCmdProcessExitCode)
		_, _ = windows.WaitForSingleObject(p.process, 1000)
	}
	p.closed = true
	input, output, process, console := p.input, p.output, p.process, p.console
	p.input, p.output, p.process, p.console = 0, 0, 0, 0
	p.mu.Unlock()
	if console != 0 {
		windows.ClosePseudoConsole(console)
	}
	if input != 0 {
		_ = windows.CloseHandle(input)
	}
	if output != 0 {
		_ = windows.CloseHandle(output)
	}
	if process != 0 {
		_ = windows.CloseHandle(process)
	}
	return nil
}
