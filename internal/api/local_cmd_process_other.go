//go:build !windows

package api

func localCmdSupported() bool { return false }

func newLocalCmdProcess(string, int, int) (localCmdProcess, error) {
	return nil, errLocalCmdRemoteUnsupported
}
