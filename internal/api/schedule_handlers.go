package api

import (
	"fmt"
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
		patch, restarted, err := s.prepareScheduleTaskModel(taskRaw)
		if err != nil {
			bad(w, 400, err.Error())
			return
		}
		t, err := ga.SaveTask(s.CfgStore.Cfg.GARoot, req.ID, taskRaw)
		if err != nil {
			bad(w, 400, err.Error())
			return
		}
		writeJSON(w, map[string]interface{}{"ok": true, "task": t, "raw": taskRaw, "runtime_patch": patch, "scheduler_restarted": restarted})
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
	patch, restarted, err := s.prepareScheduleTaskModel(taskRaw)
	if err != nil {
		bad(w, 400, err.Error())
		return
	}
	t, err := ga.CreateTask(s.CfgStore.Cfg.GARoot, req.ID, taskRaw)
	if err != nil {
		bad(w, 400, err.Error())
		return
	}
	writeJSON(w, map[string]interface{}{"ok": true, "task": t, "runtime_patch": patch, "scheduler_restarted": restarted})
}

func (s *Server) prepareScheduleTaskModel(raw map[string]any) (ga.ScheduleModelDispatchResult, bool, error) {
	llmNo, selected, err := ga.ScheduleTaskLLMNo(raw)
	if err != nil || !selected {
		return ga.ScheduleModelDispatchResult{}, false, err
	}
	llms, err := s.listGARuntimeLLMs(s.CfgStore.Cfg)
	if err != nil {
		return ga.ScheduleModelDispatchResult{}, false, fmt.Errorf("cannot verify scheduled task model: %w", err)
	}
	if !containsScheduleLLMNo(llms, llmNo) {
		return ga.ScheduleModelDispatchResult{}, false, fmt.Errorf("scheduled task model #%d is unavailable", llmNo)
	}
	patch, err := ga.EnsureScheduleModelDispatch(s.CfgStore.Cfg.GARoot)
	if err != nil || len(patch.Updated) == 0 {
		return patch, false, err
	}
	running, err := ga.SchedulerRunning(s.CfgStore.Cfg.GARoot, s.CfgStore.Cfg.EffectivePython)
	if err != nil {
		return patch, false, err
	}
	if !running {
		return patch, false, nil
	}
	if _, err := s.startServiceByName("reflect/scheduler.py", nil); err != nil {
		return patch, false, fmt.Errorf("restart scheduler for model dispatch: %w", err)
	}
	return patch, true, nil
}

func containsScheduleLLMNo(llms []map[string]interface{}, want int) bool {
	for _, llm := range llms {
		index, selected, err := ga.ScheduleTaskLLMNo(map[string]any{"llm_no": llm["index"]})
		if err == nil && selected && index == want {
			return true
		}
	}
	return false
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
