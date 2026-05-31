package api

import (
	"net/http"
	"strconv"
	"strings"

	"genericagent-admin-go/internal/ga"
)

func (s *Server) goalsStart(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		bad(w, 405, "method not allowed")
		return
	}
	var req struct {
		Objective     string `json:"objective"`
		BudgetSeconds int    `json:"budget_seconds"`
		BudgetMinutes int    `json:"budget_minutes"`
		MaxTurns      int    `json:"max_turns"`
		LLMNo         *int   `json:"llm_no"`
	}
	if err := decode(r, &req); err != nil {
		bad(w, 400, err.Error())
		return
	}
	if req.BudgetSeconds < 0 {
		bad(w, 400, "budget_seconds must be >= 0")
		return
	}
	if req.BudgetMinutes < 0 {
		bad(w, 400, "budget_minutes must be >= 0")
		return
	}
	if req.BudgetSeconds > 0 && req.BudgetMinutes > 0 {
		bad(w, 400, "use either budget_seconds or budget_minutes, not both")
		return
	}
	if req.BudgetSeconds <= 0 && req.BudgetMinutes > 0 {
		if req.BudgetMinutes > 30*24*60 {
			bad(w, 400, "budget_minutes must be <= 43200")
			return
		}
		req.BudgetSeconds = req.BudgetMinutes * 60
	}
	meta, err := ga.StartGoal(s.CfgStore.Cfg.GARoot, ga.GoalStartOptions{Objective: req.Objective, BudgetSeconds: req.BudgetSeconds, MaxTurns: req.MaxTurns, LLMNo: req.LLMNo, PythonPath: s.CfgStore.Cfg.PythonPath})
	if err != nil {
		bad(w, 400, err.Error())
		return
	}
	writeJSON(w, map[string]interface{}{"ok": true, "goal": meta})
}

func (s *Server) goalsList(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		bad(w, 405, "method not allowed")
		return
	}
	items, err := ga.ListGoals(s.CfgStore.Cfg.GARoot)
	if err != nil {
		bad(w, 400, err.Error())
		return
	}
	writeJSON(w, map[string]interface{}{"goals": items})
}

func (s *Server) goalsStop(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		bad(w, 405, "method not allowed")
		return
	}
	var req struct {
		ID  string `json:"id"`
		PID int    `json:"pid"`
	}
	if err := decode(r, &req); err != nil {
		bad(w, 400, err.Error())
		return
	}
	meta, err := ga.StopGoal(s.CfgStore.Cfg.GARoot, req.ID, req.PID)
	if err != nil {
		bad(w, 400, err.Error())
		return
	}
	writeJSON(w, map[string]interface{}{"ok": true, "goal": meta})
}

func (s *Server) goalsDelete(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost && r.Method != http.MethodDelete {
		bad(w, 405, "method not allowed")
		return
	}
	var req struct {
		ID string `json:"id"`
	}
	if r.Method == http.MethodDelete {
		req.ID = r.URL.Query().Get("id")
	} else if err := decode(r, &req); err != nil {
		bad(w, 400, err.Error())
		return
	}
	if err := ga.DeleteGoal(s.CfgStore.Cfg.GARoot, req.ID); err != nil {
		bad(w, 400, err.Error())
		return
	}
	writeJSON(w, map[string]interface{}{"ok": true, "id": req.ID})
}

func (s *Server) goalsOutput(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		bad(w, 405, "method not allowed")
		return
	}
	maxBytes := int64(0)
	if raw := strings.TrimSpace(r.URL.Query().Get("max_bytes")); raw != "" {
		var err error
		maxBytes, err = strconv.ParseInt(raw, 10, 64)
		if err != nil {
			bad(w, 400, "invalid max_bytes")
			return
		}
		if maxBytes < 0 {
			bad(w, 400, "max_bytes must be >= 0")
			return
		}
	}
	result, err := ga.GoalOutput(s.CfgStore.Cfg.GARoot, r.URL.Query().Get("id"), maxBytes)
	if err != nil {
		bad(w, 400, err.Error())
		return
	}
	writeJSON(w, result)
}
