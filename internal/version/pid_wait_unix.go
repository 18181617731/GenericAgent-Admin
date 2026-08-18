//go:build !windows

package version

import (
	"errors"
	"fmt"
	"syscall"
	"time"
)

func waitForPIDExit(pid int, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	for {
		err := syscall.Kill(pid, 0)
		if errors.Is(err, syscall.ESRCH) {
			return nil
		}
		if err != nil {
			return fmt.Errorf("probe process %d: %w", pid, err)
		}
		if !time.Now().Before(deadline) {
			return fmt.Errorf("timeout after %s", timeout)
		}
		time.Sleep(25 * time.Millisecond)
	}
}
