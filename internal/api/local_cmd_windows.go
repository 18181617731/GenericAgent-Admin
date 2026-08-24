//go:build windows

package api

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"unsafe"

	"golang.org/x/sys/windows"
)

const localCmdExecutable = "cmd.exe"

const localCmdCreateNewConsole = windows.CREATE_NEW_CONSOLE

type localCmdLaunchSpec struct {
	applicationName  *uint16
	commandLine      []uint16
	currentDirectory *uint16
	startupInfo      windows.StartupInfo
	creationFlags    uint32
}

var localCmdCreateProcess = windows.CreateProcess
var localCmdCloseHandle = windows.CloseHandle

func validateLocalCmdExecutablePath(path string) error {
	if !filepath.IsAbs(path) {
		return fmt.Errorf("cmd.exe path must be absolute")
	}
	if !strings.EqualFold(filepath.Base(path), localCmdExecutable) {
		return fmt.Errorf("cmd.exe path must name %s", localCmdExecutable)
	}
	return nil
}

func resolveLocalCmdExecutable() (string, error) {
	systemDirectory, err := windows.GetSystemDirectory()
	if err != nil {
		return "", fmt.Errorf("could not resolve Windows system directory: %w", err)
	}
	executablePath := filepath.Clean(filepath.Join(systemDirectory, localCmdExecutable))
	if err := validateLocalCmdExecutablePath(executablePath); err != nil {
		return "", err
	}
	info, err := os.Stat(executablePath)
	if err != nil {
		return "", fmt.Errorf("could not access %s: %w", executablePath, err)
	}
	if info.IsDir() {
		return "", fmt.Errorf("cmd.exe path is a directory: %s", executablePath)
	}
	return executablePath, nil
}

func newLocalCmdLaunchSpec(dir string) (localCmdLaunchSpec, error) {
	executablePath, err := resolveLocalCmdExecutable()
	if err != nil {
		return localCmdLaunchSpec{}, err
	}
	return newLocalCmdLaunchSpecWithExecutable(dir, executablePath)
}

func newLocalCmdLaunchSpecWithExecutable(dir, executablePath string) (localCmdLaunchSpec, error) {
	if err := validateLocalCmdExecutablePath(executablePath); err != nil {
		return localCmdLaunchSpec{}, err
	}
	applicationName, err := windows.UTF16PtrFromString(executablePath)
	if err != nil {
		return localCmdLaunchSpec{}, fmt.Errorf("could not encode cmd.exe path: %w", err)
	}
	commandLine, err := windows.UTF16FromString(windows.EscapeArg(executablePath))
	if err != nil {
		return localCmdLaunchSpec{}, fmt.Errorf("could not encode cmd.exe command line: %w", err)
	}
	currentDirectory, err := windows.UTF16PtrFromString(dir)
	if err != nil {
		return localCmdLaunchSpec{}, fmt.Errorf("could not encode current directory: %w", err)
	}
	var startupInfo windows.StartupInfo
	startupInfo.Cb = uint32(unsafe.Sizeof(startupInfo))
	startupInfo.Flags = windows.STARTF_USESHOWWINDOW
	startupInfo.ShowWindow = windows.SW_SHOWNORMAL
	return localCmdLaunchSpec{
		applicationName:  applicationName,
		commandLine:      commandLine,
		currentDirectory: currentDirectory,
		startupInfo:      startupInfo,
		creationFlags:    localCmdCreateNewConsole,
	}, nil
}

func startLocalCmd(dir string) error {
	spec, err := newLocalCmdLaunchSpec(dir)
	if err != nil {
		return err
	}
	var processInfo windows.ProcessInformation
	err = localCmdCreateProcess(
		spec.applicationName,
		&spec.commandLine[0],
		nil,
		nil,
		false,
		spec.creationFlags,
		nil,
		spec.currentDirectory,
		&spec.startupInfo,
		&processInfo,
	)
	if err != nil {
		return fmt.Errorf("could not start cmd.exe: %w", err)
	}
	defer func() { _ = localCmdCloseHandle(processInfo.Process) }()
	defer func() { _ = localCmdCloseHandle(processInfo.Thread) }()
	return nil
}
