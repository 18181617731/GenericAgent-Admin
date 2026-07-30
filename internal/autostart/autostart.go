package autostart

import (
	"os"
	"path/filepath"
	"strings"
)

const appID = "com.genericagent.admin"
const runName = "GenericAgent Admin"

type Status struct {
	Supported bool   `json:"supported"`
	Enabled   bool   `json:"enabled"`
	Method    string `json:"method"`
	Path      string `json:"path"`
	Target    string `json:"target"`
	Detail    string `json:"detail,omitempty"`
}

func StatusForCurrent(appRoot string) Status {
	target, err := os.Executable()
	if err != nil {
		return Status{Supported: false, Detail: err.Error()}
	}
	return StatusFor(target, appRoot)
}

func EnableCurrent(appRoot string) (Status, error) {
	target, err := os.Executable()
	if err != nil {
		return Status{}, err
	}
	return Enable(target, appRoot)
}

func DisableCurrent(appRoot string) (Status, error) {
	target, err := os.Executable()
	if err != nil {
		return Status{}, err
	}
	return Disable(target, appRoot)
}

// writeFileAtomic writes data to path atomically using a temp file + rename
func writeFileAtomic(path string, data []byte, perm os.FileMode) (err error) {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}
	f, err := os.CreateTemp(dir, filepath.Base(path)+".tmp-*")
	if err != nil {
		return err
	}
	tmp := f.Name()
	defer func() {
		if err != nil {
			_ = os.Remove(tmp)
		}
	}()
	if _, err = f.Write(data); err != nil {
		_ = f.Close()
		return err
	}
	if err = f.Chmod(perm); err != nil {
		_ = f.Close()
		return err
	}
	if err = f.Sync(); err != nil {
		_ = f.Close()
		return err
	}
	if err = f.Close(); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

func xmlEscape(s string) string {
	s = strings.ReplaceAll(s, "&", "&amp;")
	s = strings.ReplaceAll(s, "<", "&lt;")
	s = strings.ReplaceAll(s, ">", "&gt;")
	s = strings.ReplaceAll(s, "\"", "&quot;")
	s = strings.ReplaceAll(s, "'", "&apos;")
	return s
}
