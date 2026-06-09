package api

import (
	"net/http"

	"genericagent-admin-go/internal/ga"
)

func (s *Server) scheduleTasks(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		bad(w, 405, "method not allowed")
		return
	}
	writeJSON(w, ga.BuildSchedule(s.CfgStore.Cfg.GARoot))
}

func (s *Server) scheduleTask(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		raw, id, err := ga.ReadTask(s.CfgStore.Cfg.GARoot, r.URL.Query().Get("id"))
		if err != nil {
			bad(w, 400, err.Error())
			return
		}
		writeJSON(w, map[string]interface{}{"id": id, "task": raw, "raw": raw})
	case http.MethodPut:
		var req struct {
			ID   string         `json:"id"`
			Task map[string]any `json:"task"`
			Raw  map[string]any `json:"raw"`
		}
		if err := decode(r, &req); err != nil || req.ID == "" {
			bad(w, 400, "bad request")
			return
		}
		taskRaw := req.Task
		if taskRaw == nil {
			taskRaw = req.Raw
		}
		t, err := ga.SaveTask(s.CfgStore.Cfg.GARoot, req.ID, taskRaw)
		if err != nil {
			bad(w, 400, err.Error())
			return
		}
		writeJSON(w, map[string]interface{}{"ok": true, "task": t, "raw": taskRaw})
	default:
		bad(w, 405, "method not allowed")
	}
}

func (s *Server) scheduleArtifact(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		bad(w, 405, "method not allowed")
		return
	}
	content, entry, err := ga.ReadScheduleArtifact(s.CfgStore.Cfg.GARoot, r.URL.Query().Get("path"), 256*1024)
	if err != nil {
		bad(w, 400, err.Error())
		return
	}
	writeJSON(w, map[string]interface{}{"entry": entry, "content": content})
}

func (s *Server) scheduleCreate(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		bad(w, 405, "method not allowed")
		return
	}
	var req struct {
		ID   string         `json:"id"`
		Task map[string]any `json:"task"`
		Raw  map[string]any `json:"raw"`
	}
	if err := decode(r, &req); err != nil || req.ID == "" {
		bad(w, 400, "bad request")
		return
	}
	taskRaw := req.Task
	if taskRaw == nil {
		taskRaw = req.Raw
	}
	t, err := ga.CreateTask(s.CfgStore.Cfg.GARoot, req.ID, taskRaw)
	if err != nil {
		bad(w, 400, err.Error())
		return
	}
	writeJSON(w, map[string]interface{}{"ok": true, "task": t})
}

func (s *Server) scheduleDelete(w http.ResponseWriter, r *http.Request) {
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
		bad(w, 400, "bad request")
		return
	}
	if req.ID == "" {
		bad(w, 400, "empty id")
		return
	}
	if err := ga.DeleteTask(s.CfgStore.Cfg.GARoot, req.ID); err != nil {
		bad(w, 400, err.Error())
		return
	}
	writeJSON(w, map[string]bool{"ok": true})
}

func (s *Server) scheduleToggle(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		bad(w, 405, "method not allowed")
		return
	}
	var req struct {
		ID      string `json:"id"`
		Enabled bool   `json:"enabled"`
	}
	if err := decode(r, &req); err != nil || req.ID == "" {
		bad(w, 400, "bad request")
		return
	}
	task, err := ga.ToggleTask(s.CfgStore.Cfg.GARoot, req.ID, req.Enabled)
	if err != nil {
		bad(w, 400, err.Error())
		return
	}
	writeJSON(w, map[string]interface{}{"ok": true, "task": task})
}
