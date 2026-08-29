//go:build windows

package api

import (
	"fmt"
	"path/filepath"

	"golang.org/x/sys/windows"
)

func localCmdRootDirectories() ([]string, error) {
	mask, err := windows.GetLogicalDrives()
	if err != nil {
		return nil, fmt.Errorf("list Windows drives: %w", err)
	}
	roots := make([]string, 0, 26)
	for bit := 0; bit < 26; bit++ {
		if mask&(1<<bit) == 0 {
			continue
		}
		root := fmt.Sprintf("%c:\\", 'A'+bit)
		if filepath.VolumeName(root) != "" {
			roots = append(roots, root)
		}
	}
	return roots, nil
}
