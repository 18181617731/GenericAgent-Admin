//go:build windows

package api

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"golang.org/x/sys/windows"
)

func TestLocalCmdLaunchSpecUsesInteractiveConsole(t *testing.T) {
	dir := `C:\Users\Public\中文 local cmd`
	executablePath := `C:\Program Files\Windows\System32\cmd.exe`
	spec, err := newLocalCmdLaunchSpecWithExecutable(dir, executablePath)
	if err != nil {
		t.Fatalf("newLocalCmdLaunchSpecWithExecutable() error = %v", err)
	}
	if got := windows.UTF16PtrToString(spec.applicationName); got != executablePath {
		t.Fatalf("application name = %q, want %q", got, executablePath)
	}
	if got := windows.UTF16PtrToString(&spec.commandLine[0]); got != windows.EscapeArg(executablePath) {
		t.Fatalf("command line = %q, want %q", got, windows.EscapeArg(executablePath))
	}
	if got := windows.UTF16PtrToString(spec.currentDirectory); got != dir {
		t.Fatalf("current directory = %q, want %q", got, dir)
	}
	if spec.creationFlags&windows.CREATE_NEW_CONSOLE == 0 {
		t.Fatalf("creation flags = %#x, want CREATE_NEW_CONSOLE", spec.creationFlags)
	}
	if spec.startupInfo.Flags&windows.STARTF_USESHOWWINDOW == 0 {
		t.Fatal("startup info does not request a visible normal window")
	}
	if spec.startupInfo.ShowWindow != windows.SW_SHOWNORMAL {
		t.Fatalf("show window = %d, want SW_SHOWNORMAL", spec.startupInfo.ShowWindow)
	}
	if spec.startupInfo.Flags&windows.STARTF_USESTDHANDLES != 0 {
		t.Fatal("startup info unexpectedly opts into parent standard handles")
	}
	if spec.startupInfo.StdInput != 0 || spec.startupInfo.StdOutput != 0 || spec.startupInfo.StdErr != 0 {
		t.Fatal("startup info unexpectedly supplies standard handles")
	}
}

func TestResolveLocalCmdExecutableReturnsSystemCmd(t *testing.T) {
	path, err := resolveLocalCmdExecutable()
	if err != nil {
		t.Fatalf("resolveLocalCmdExecutable() error = %v", err)
	}
	if !filepath.IsAbs(path) {
		t.Fatalf("resolved path = %q, want absolute path", path)
	}
	if !strings.EqualFold(filepath.Base(path), localCmdExecutable) {
		t.Fatalf("resolved path = %q, want cmd.exe basename", path)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat resolved path: %v", err)
	}
	if info.IsDir() {
		t.Fatalf("resolved path = %q, want a file", path)
	}
}

type localCmdLaunchCapture struct {
	applicationName  *uint16
	commandLine      *uint16
	currentDirectory *uint16
	creationFlags    uint32
	startupInfo      *windows.StartupInfo
}

func installLocalCmdTestHooks(t *testing.T) (*localCmdLaunchCapture, *[]windows.Handle) {
	t.Helper()
	oldCreateProcess := localCmdCreateProcess
	oldCloseHandle := localCmdCloseHandle
	t.Cleanup(func() {
		localCmdCreateProcess = oldCreateProcess
		localCmdCloseHandle = oldCloseHandle
	})

	captured := &localCmdLaunchCapture{}
	localCmdCreateProcess = func(
		applicationName *uint16,
		commandLine *uint16,
		procSecurity *windows.SecurityAttributes,
		threadSecurity *windows.SecurityAttributes,
		inheritHandles bool,
		creationFlags uint32,
		env *uint16,
		currentDirectory *uint16,
		startupInfo *windows.StartupInfo,
		processInfo *windows.ProcessInformation,
	) error {
		captured.applicationName = applicationName
		captured.commandLine = commandLine
		captured.currentDirectory = currentDirectory
		captured.creationFlags = creationFlags
		captured.startupInfo = startupInfo
		processInfo.Process = windows.Handle(101)
		processInfo.Thread = windows.Handle(102)
		return nil
	}
	var closed []windows.Handle
	localCmdCloseHandle = func(handle windows.Handle) error {
		closed = append(closed, handle)
		return nil
	}
	return captured, &closed
}

func assertLocalCmdProcessArguments(t *testing.T, captured *localCmdLaunchCapture, dir string) {
	t.Helper()
	executablePath, err := resolveLocalCmdExecutable()
	if err != nil {
		t.Fatalf("resolveLocalCmdExecutable() error = %v", err)
	}
	if got := windows.UTF16PtrToString(captured.applicationName); got != executablePath {
		t.Fatalf("application name = %q, want %q", got, executablePath)
	}
	if got := windows.UTF16PtrToString(captured.commandLine); got != windows.EscapeArg(executablePath) {
		t.Fatalf("command line = %q, want %q", got, windows.EscapeArg(executablePath))
	}
	if got := windows.UTF16PtrToString(captured.currentDirectory); got != dir {
		t.Fatalf("current directory = %q, want %q", got, dir)
	}
	if captured.creationFlags&windows.CREATE_NEW_CONSOLE == 0 {
		t.Fatalf("creation flags = %#x, want CREATE_NEW_CONSOLE", captured.creationFlags)
	}
	if captured.startupInfo.Flags&windows.STARTF_USESHOWWINDOW == 0 {
		t.Fatal("startup info does not request a visible normal window")
	}
	if captured.startupInfo.ShowWindow != windows.SW_SHOWNORMAL {
		t.Fatalf("show window = %d, want SW_SHOWNORMAL", captured.startupInfo.ShowWindow)
	}
	if captured.startupInfo.Flags&windows.STARTF_USESTDHANDLES != 0 {
		t.Fatal("startup info unexpectedly opts into parent standard handles")
	}
}

func TestStartLocalCmdPassesLaunchArgumentsAndClosesHandles(t *testing.T) {
	captured, closed := installLocalCmdTestHooks(t)
	dir := `C:\Program Files\Generic Agent 中文`
	if err := startLocalCmd(dir); err != nil {
		t.Fatalf("startLocalCmd() error = %v", err)
	}
	assertLocalCmdProcessArguments(t, captured, dir)
	if !reflect.DeepEqual(*closed, []windows.Handle{102, 101}) {
		t.Fatalf("closed handles = %v, want thread then process handles", *closed)
	}
}
