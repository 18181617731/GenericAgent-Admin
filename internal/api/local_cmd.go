package api

import (
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
)

var errLocalCmdUnsupported = errors.New("local CMD is only supported on Windows")

// launchLocalCmdFunc is replaceable in tests so validation and route safety can
// be verified without opening a real console window.
var launchLocalCmdFunc = startLocalCmd

type localCmdOpenRequest struct {
	Path string `json:"path"`
}

func validateLocalCmdDirectory(raw string) (string, error) {
	dir := strings.TrimSpace(raw)
	if dir == "" {
		return "", errors.New("directory path is required")
	}
	if !filepath.IsAbs(dir) {
		return "", errors.New("directory path must be absolute")
	}
	info, err := os.Stat(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return "", fmt.Errorf("directory does not exist: %s", dir)
		}
		return "", fmt.Errorf("cannot access directory %q: %w", dir, err)
	}
	if !info.IsDir() {
		return "", fmt.Errorf("path is not a directory: %s", dir)
	}
	return filepath.Clean(dir), nil
}

func (s *Server) localCmdOpen(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		bad(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	var req localCmdOpenRequest
	if err := decode(r, &req); err != nil {
		bad(w, http.StatusBadRequest, err.Error())
		return
	}
	dir, err := validateLocalCmdDirectory(req.Path)
	if err != nil {
		bad(w, http.StatusBadRequest, err.Error())
		return
	}
	if err := launchLocalCmdFunc(dir); err != nil {
		if errors.Is(err, errLocalCmdUnsupported) {
			bad(w, http.StatusNotImplemented, err.Error())
			return
		}
		bad(w, http.StatusInternalServerError, fmt.Sprintf("could not open local CMD: %v", err))
		return
	}
	writeJSON(w, map[string]interface{}{"ok": true, "path": dir})
}
