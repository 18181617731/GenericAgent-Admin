//go:build !windows

package api

func localCmdRootDirectories() ([]string, error) {
	return []string{"/"}, nil
}
