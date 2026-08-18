//go:build windows

package version

import (
	"errors"
	"fmt"
	"time"

	"golang.org/x/sys/windows"
)

func waitForPIDExit(pid int, timeout time.Duration) error {
	handle, err := windows.OpenProcess(windows.SYNCHRONIZE, false, uint32(pid))
	if err != nil {
		if errors.Is(err, windows.ERROR_INVALID_PARAMETER) {
			return nil
		}
		return fmt.Errorf("open process %d: %w", pid, err)
	}
	defer windows.CloseHandle(handle)

	millis := uint32(timeout / time.Millisecond)
	if millis == 0 {
		millis = 1
	}
	result, err := windows.WaitForSingleObject(handle, millis)
	if err != nil {
		return fmt.Errorf("wait for process %d: %w", pid, err)
	}
	switch result {
	case windows.WAIT_OBJECT_0:
		return nil
	case uint32(windows.WAIT_TIMEOUT):
		return fmt.Errorf("timeout after %s", timeout)
	default:
		return fmt.Errorf("wait for process %d returned %#x", pid, result)
	}
}
