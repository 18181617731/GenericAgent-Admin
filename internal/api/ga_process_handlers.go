package api

import "net/http"

type gaProcessReq struct {
	PID int `json:"pid"`
}

func (s *Server) gaProcesses(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		bad(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	snap, err := s.Svc.ScanGAProcesses()
	if err != nil {
		bad(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, snap)
}

func (s *Server) killGAProcess(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		bad(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	if !requireDangerousHeader(w, r) {
		return
	}
	var req gaProcessReq
	if err := decode(r, &req); err != nil {
		bad(w, http.StatusBadRequest, err.Error())
		return
	}
	res, err := s.Svc.KillGAProcess(req.PID)
	if err != nil {
		bad(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, res)
}

func (s *Server) adoptGAProcess(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		bad(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	if !requireDangerousHeader(w, r) {
		return
	}
	var req gaProcessReq
	if err := decode(r, &req); err != nil {
		bad(w, http.StatusBadRequest, err.Error())
		return
	}
	res, err := s.Svc.AdoptGAProcess(req.PID)
	if err != nil {
		bad(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, res)
}
