package api

import (
	"context"
	"net/http"
	"time"

	"genericagent-admin-go/internal/version"
)

func (s *Server) versionInfo(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		bad(w, 405, "method not allowed")
		return
	}
	writeJSON(w, version.Current())
}

func (s *Server) versionCheck(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodPost {
		bad(w, 405, "method not allowed")
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
	defer cancel()
	res, err := version.Check(ctx)
	if err != nil {
		bad(w, 502, err.Error())
		return
	}
	writeJSON(w, res)
}

func (s *Server) versionStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		bad(w, 405, "method not allowed")
		return
	}
	writeJSON(w, version.CurrentUpdateStatus())
}

func (s *Server) versionUpdate(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		bad(w, 405, "method not allowed")
		return
	}
	st, err := version.StartApplyLatest()
	if err != nil {
		bad(w, 400, err.Error())
		return
	}
	writeJSON(w, st)
}
