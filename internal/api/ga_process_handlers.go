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
	manager, instanceID, ok := s.serviceManagerForHTTP(w, r)
	if !ok {
		return
	}
	snap, err := manager.ScanGAProcesses()
	if err != nil {
		bad(w, http.StatusInternalServerError, err.Error())
		return
	}
	setResolvedInstanceHeader(w, instanceID)
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
	manager, instanceID, ok := s.serviceManagerForHTTP(w, r)
	if !ok {
		return
	}
	var req gaProcessReq
	if err := decode(r, &req); err != nil {
		bad(w, http.StatusBadRequest, err.Error())
		return
	}
	res, err := manager.KillGAProcess(req.PID)
	if err != nil {
		bad(w, http.StatusBadRequest, err.Error())
		return
	}
	setResolvedInstanceHeader(w, instanceID)
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
	manager, instanceID, ok := s.serviceManagerForHTTP(w, r)
	if !ok {
		return
	}
	var req gaProcessReq
	if err := decode(r, &req); err != nil {
		bad(w, http.StatusBadRequest, err.Error())
		return
	}
	res, err := manager.AdoptGAProcess(req.PID)
	if err != nil {
		bad(w, http.StatusBadRequest, err.Error())
		return
	}
	setResolvedInstanceHeader(w, instanceID)
	writeJSON(w, res)
}
