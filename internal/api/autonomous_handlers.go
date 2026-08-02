package api

import (
	"net/http"
	"strings"

	"genericagent-admin-go/internal/ga"
)

func (s *Server) autonomousApprovals(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		overview, err := ga.BuildAutonomousApprovals(s.CfgStore.Cfg.GARoot)
		if err != nil {
			bad(w, http.StatusInternalServerError, err.Error())
			return
		}
		s.reviewAutonomousApprovals(&overview)
		writeJSON(w, overview)
	case http.MethodPost:
		s.autonomousApprovalDecision(w, r)
	default:
		bad(w, http.StatusMethodNotAllowed, "method not allowed")
	}
}

func (s *Server) autonomousApprovalDecision(w http.ResponseWriter, r *http.Request) {
	var req struct {
		ID       string `json:"id"`
		Decision string `json:"decision"`
		Note     string `json:"note"`
	}
	if err := decode(r, &req); err != nil || strings.TrimSpace(req.ID) == "" {
		bad(w, http.StatusBadRequest, "bad request")
		return
	}
	overview, queued, err := ga.DecideAutonomousApproval(s.CfgStore.Cfg.GARoot, req.ID, req.Decision, req.Note)
	if err != nil {
		bad(w, http.StatusBadRequest, err.Error())
		return
	}
	s.reviewAutonomousApprovals(&overview)
	writeJSON(w, map[string]interface{}{"ok": true, "queued": queued, "overview": overview})
}
