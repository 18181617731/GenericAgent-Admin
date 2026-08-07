package api

import (
	"net/http"

	"genericagent-admin-go/internal/ga"
)

func (s *Server) projectTodos(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		bad(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	overview, err := ga.BuildProjectTodos(s.CfgStore.Snapshot().GARoot)
	if err != nil {
		bad(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, overview)
}
