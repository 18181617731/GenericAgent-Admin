package api

import (
	"fmt"
	"net/http"
	"strings"

	"genericagent-admin-go/internal/ga"
)

func (s *Server) scheduleTasks(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		bad(w, 405, "method not allowed")
		return
	}
	writeJSON(w, ga.BuildSchedule(s.CfgStore.Snapshot().GARoot))
}

// scheduleFolders exposes the Admin-only grouping index. GET is read-only;
// mutations remain behind the existing dangerous-confirm middleware because
// they write metadata under the selected GA root.
func (s *Server) scheduleFolders(w http.ResponseWriter, r *http.Request) {
	root := s.CfgStore.Snapshot().GARoot
	switch r.Method {
	case http.MethodGet:
		state, err := ga.LoadScheduleFolders(root)
		if err != nil {
			bad(w, http.StatusInternalServerError, err.Error())
			return
		}
		writeJSON(w, state)
	case http.MethodPost:
		var req struct {
			Action   string `json:"action"`
			ID       string `json:"id"`
			Name     string `json:"name"`
			TaskID   string `json:"task_id"`
			FolderID string `json:"folder_id"`
		}
		if err := decode(r, &req); err != nil {
			bad(w, http.StatusBadRequest, "bad request")
			return
		}
		var (
			state ga.ScheduleFolders
			err   error
		)
		switch strings.ToLower(strings.TrimSpace(req.Action)) {
		case "create":
			state, err = ga.CreateScheduleFolder(root, req.Name)
		case "rename":
			state, err = ga.RenameScheduleFolder(root, req.ID, req.Name)
		case "delete":
			state, err = ga.DeleteScheduleFolder(root, req.ID)
		case "move":
			state, err = ga.MoveScheduleTask(root, req.TaskID, req.FolderID)
		default:
			bad(w, http.StatusBadRequest, "action must be create, rename, delete or move")
			return
		}
		if err != nil {
			bad(w, http.StatusBadRequest, err.Error())
			return
		}
		writeJSON(w, map[string]interface{}{"ok": true, "folders": state})
	default:
		bad(w, http.StatusMethodNotAllowed, "method not allowed")
	}
}

func (s *Server) scheduleTask(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		raw, id, err := ga.ReadTask(s.CfgStore.Snapshot().GARoot, r.URL.Query().Get("id"))
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
		t, err := ga.SaveTask(s.CfgStore.Snapshot().GARoot, req.ID, taskRaw)
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
	content, entry, err := ga.ReadScheduleArtifact(s.CfgStore.Snapshot().GARoot, r.URL.Query().Get("path"), 256*1024)
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
	t, err := ga.CreateTask(s.CfgStore.Snapshot().GARoot, req.ID, taskRaw)
	if err != nil {
		bad(w, 400, err.Error())
		return
	}
	writeJSON(w, map[string]interface{}{"ok": true, "task": t, "runtime_patch": patch, "scheduler_restarted": restarted})
}

func (s *Server) prepareScheduleTaskModel(raw map[string]any) (ga.ScheduleModelDispatchResult, bool, error) {
	llmNo, selected, err := ga.ScheduleTaskLLMNo(raw)
	if err != nil {
		return ga.ScheduleModelDispatchResult{}, false, err
	}
	modelKey, selectedKey, err := ga.ScheduleTaskModelKey(raw)
	if err != nil {
		return ga.ScheduleModelDispatchResult{}, false, err
	}
	if !selected && !selectedKey {
		return ga.ScheduleModelDispatchResult{}, false, nil
	}
	llms, err := s.listGARuntimeLLMs(s.CfgStore.Snapshot())
	if err != nil {
		return ga.ScheduleModelDispatchResult{}, false, fmt.Errorf("cannot verify scheduled task model: %w", err)
	}
	if selectedKey {
		resolved, ok := scheduleLLMNoByModelKey(llms, modelKey)
		if !ok {
			return ga.ScheduleModelDispatchResult{}, false, fmt.Errorf("scheduled task model is unavailable; choose it again from the current model list")
		}
		llmNo, selected = resolved, true
		// Keep the numeric field as a backwards-compatible hint for older GA
		// runtimes; the stable key remains authoritative.
		if raw != nil {
			raw["llm_no"] = llmNo
		}
	} else {
		if !containsScheduleLLMNo(llms, llmNo) {
			return ga.ScheduleModelDispatchResult{}, false, fmt.Errorf("scheduled task model #%d is unavailable", llmNo)
		}
		// Migrate old tasks when they are next saved. This locks the selected
		// backend before a later provider reorder can change its index.
		if raw != nil {
			if key := scheduleModelKeyByLLMNo(llms, llmNo); key != "" {
				raw["model_key"] = key
			}
		}
	}
	patch, err := ga.EnsureScheduleModelDispatch(s.CfgStore.Snapshot().GARoot)
	if err != nil || len(patch.Updated) == 0 {
		return patch, false, err
	}
	running, err := ga.SchedulerRunning(s.CfgStore.Snapshot().GARoot, s.CfgStore.Snapshot().EffectivePython)
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

func scheduleLLMNoByModelKey(llms []map[string]interface{}, want string) (int, bool) {
	want = strings.TrimSpace(want)
	if want == "" {
		return 0, false
	}
	for _, llm := range llms {
		if strings.TrimSpace(fmt.Sprint(llm["model_key"])) != want {
			continue
		}
		index, selected, err := ga.ScheduleTaskLLMNo(map[string]any{"llm_no": llm["index"]})
		if err == nil && selected {
			return index, true
		}
	}
	return 0, false
}

func scheduleModelKeyByLLMNo(llms []map[string]interface{}, want int) string {
	for _, llm := range llms {
		index, selected, err := ga.ScheduleTaskLLMNo(map[string]any{"llm_no": llm["index"]})
		if err == nil && selected && index == want {
			return strings.TrimSpace(fmt.Sprint(llm["model_key"]))
		}
	}
	return ""
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
	if err := ga.DeleteTask(s.CfgStore.Snapshot().GARoot, req.ID); err != nil {
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
	task, err := ga.ToggleTask(s.CfgStore.Snapshot().GARoot, req.ID, req.Enabled)
	if err != nil {
		bad(w, 400, err.Error())
		return
	}
	writeJSON(w, map[string]interface{}{"ok": true, "task": task})
}
