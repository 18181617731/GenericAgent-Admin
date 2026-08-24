//go:build !windows

package api

func startLocalCmd(string) error {
	return errLocalCmdUnsupported
}
