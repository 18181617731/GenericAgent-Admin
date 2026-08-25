package api

import (
	"errors"
	"io"
)

var errLocalCmdRemoteUnsupported = errors.New("remote CMD sessions are only supported on Windows")

// localCmdProcess is the small contract shared by the session registry and the
// platform-specific ConPTY implementation. Keeping the process behind this
// interface also lets handler tests run without spawning a real shell.
type localCmdProcess interface {
	io.Reader
	io.Writer
	Resize(cols, rows int) error
	Kill() error
	Wait() (int, error)
	Close() error
}

var newLocalCmdProcessFunc = newLocalCmdProcess
