package api

import (
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

type localCmdDirectoryEntry struct {
	Name string `json:"name"`
	Path string `json:"path"`
}

func (s *Server) localCmdDirectories(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		bad(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	if !localCmdSupported() {
		bad(w, http.StatusNotImplemented, errLocalCmdRemoteUnsupported.Error())
		return
	}
	path := strings.TrimSpace(r.URL.Query().Get("path"))
	if path == "" {
		s.localCmdDirectoryRoots(w)
		return
	}
	dir, err := validateLocalCmdDirectory(path)
	if err != nil {
		bad(w, http.StatusBadRequest, err.Error())
		return
	}
	entries, err := localCmdChildDirectories(dir)
	if err != nil {
		bad(w, http.StatusBadRequest, fmt.Sprintf("cannot list directory %q: %v", dir, err))
		return
	}
	parent := filepath.Dir(dir)
	if parent == dir {
		parent = ""
	}
	writeJSON(w, map[string]any{"ok": true, "current": dir, "parent": parent, "roots": []string{}, "entries": entries})
}

func (s *Server) localCmdDirectoryRoots(w http.ResponseWriter) {
	roots, err := localCmdRootDirectories()
	if err != nil {
		bad(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, map[string]any{"ok": true, "current": "", "parent": "", "roots": roots, "entries": []localCmdDirectoryEntry{}})
}

func localCmdChildDirectories(dir string) ([]localCmdDirectoryEntry, error) {
	items, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	entries := make([]localCmdDirectoryEntry, 0, len(items))
	for _, item := range items {
		if !item.IsDir() {
			continue
		}
		entries = append(entries, localCmdDirectoryEntry{Name: item.Name(), Path: filepath.Join(dir, item.Name())})
	}
	sort.Slice(entries, func(i, j int) bool { return strings.ToLower(entries[i].Name) < strings.ToLower(entries[j].Name) })
	return entries, nil
}
