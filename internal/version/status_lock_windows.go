//go:build windows

package version

import (
	"os"
	"path/filepath"

	"golang.org/x/sys/windows"
)

func withStatusFileLock(fn func() error) error {
	path := statusPath() + ".lock"
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return err
	}
	file, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		return err
	}
	defer file.Close()
	var overlapped windows.Overlapped
	if err := windows.LockFileEx(windows.Handle(file.Fd()), windows.LOCKFILE_EXCLUSIVE_LOCK, 0, 1, 0, &overlapped); err != nil {
		return err
	}
	defer windows.UnlockFileEx(windows.Handle(file.Fd()), 0, 1, 0, &overlapped)
	return fn()
}
