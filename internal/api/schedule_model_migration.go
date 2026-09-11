package api

import (
	"errors"
	"fmt"
	"net/http"
	"strings"

	"genericagent-admin-go/internal/ga"
)

// scheduleModelMigrationItem describes the one-time migration decision for a
// scheduled task. Legacy numeric model positions are only migrated when the
// current runtime exposes exactly one stable identity for that position.
type scheduleModelMigrationItem struct {
	ID         string `json:"id"`
	Status     string `json:"status"`
	LLMNo      *int   `json:"llm_no,omitempty"`
	ModelKey   string `json:"model_key,omitempty"`
	ModelLabel string `json:"model_label,omitempty"`
	Reason     string `json:"reason,omitempty"`
}

type scheduleModelMigrationResponse struct {
	Items            []scheduleModelMigrationItem `json:"items"`
	Total            int                          `json:"total"`
	Migratable       int                          `json:"migratable"`
	Stable           int                          `json:"stable"`
	Skipped          int                          `json:"skipped"`
	SchedulerDefault int                          `json:"scheduler_default"`
	Migrated         int                          `json:"migrated"`
}

type scheduleModelMigrationCandidate struct {
	key   string
	label string
}

// scheduleModelMigration exposes a read-only preview over GET and performs a
// confirmed, backup-producing migration over POST. The route is wrapped in
// requireDangerousConfirm by Routes, so adding model_key to existing task JSON
// can never happen from a background or accidental request.
func (s *Server) scheduleModelMigration(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodPost {
		bad(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	cfg := s.CfgStore.Snapshot()
	llms, err := s.listGARuntimeLLMs(cfg)
	if err != nil {
		// Do not send the Python subprocess output back through the remote API;
		// it can contain provider-specific diagnostics. The local log still has
		// enough context in the wrapped error for an operator to investigate.
		bad(w, http.StatusBadGateway, scheduleModelMigrationListError(err))
		return
	}
	plan := buildScheduleModelMigrationPlan(ga.BuildSchedule(cfg.GARoot).Tasks, llms)
	if r.Method == http.MethodPost {
		var req struct {
			TaskIDs []string `json:"task_ids"`
		}
		if err := decode(r, &req); err != nil {
			bad(w, http.StatusBadRequest, "bad request")
			return
		}
		migrated := 0
		selected := make(map[string]struct{}, len(req.TaskIDs))
		for _, id := range req.TaskIDs {
			id = strings.TrimSpace(id)
			if id != "" {
				selected[id] = struct{}{}
			}
		}
		all := len(selected) == 0
		for index := range plan.Items {
			item := &plan.Items[index]
			if item.Status != "migratable" || (!all && !hasScheduleMigrationID(selected, item.ID)) {
				continue
			}
			if err := migrateScheduleModelItem(cfg.GARoot, item); err != nil {
				item.Status = "migration_failed"
				item.Reason = err.Error()
				plan.Migratable--
				plan.Skipped++
				continue
			}
			item.Status = "migrated"
			migrated++
			plan.Migratable--
			plan.Stable++
		}
		plan.Migrated = migrated
	}
	writeJSON(w, plan)
}

func scheduleModelMigrationListError(err error) string {
	var listErr *chatLLMListError
	if errors.As(err, &listErr) && strings.TrimSpace(listErr.Stage) != "" {
		return "cannot inspect runtime models: " + strings.TrimSpace(listErr.Stage)
	}
	return "cannot inspect runtime models"
}

func hasScheduleMigrationID(selected map[string]struct{}, id string) bool {
	_, ok := selected[id]
	return ok
}

func buildScheduleModelMigrationPlan(tasks []ga.ScheduleTask, llms []map[string]interface{}) scheduleModelMigrationResponse {
	byNo := make(map[int][]scheduleModelMigrationCandidate)
	for _, llm := range llms {
		index, selected, err := ga.ScheduleTaskLLMNo(map[string]any{"llm_no": llm["index"]})
		if err != nil || !selected {
			continue
		}
		key, _ := llm["model_key"].(string)
		key = strings.TrimSpace(key)
		if key == "" {
			continue
		}
		candidate := scheduleModelMigrationCandidate{key: key, label: scheduleRuntimeModelLabel(llm, index)}
		alreadyListed := false
		for _, existing := range byNo[index] {
			if existing.key == candidate.key {
				alreadyListed = true
				break
			}
		}
		if !alreadyListed {
			byNo[index] = append(byNo[index], candidate)
		}
	}

	response := scheduleModelMigrationResponse{Items: make([]scheduleModelMigrationItem, 0, len(tasks)), Total: len(tasks)}
	for _, task := range tasks {
		item := scheduleModelMigrationItem{ID: task.ID, LLMNo: task.LLMNo}
		switch {
		case strings.TrimSpace(task.ModelKey) != "":
			item.Status = "stable"
			item.ModelKey = task.ModelKey
			response.Stable++
		case strings.TrimSpace(task.Error) != "":
			item.Status = "invalid"
			item.Reason = task.Error
			response.Skipped++
		case task.LLMNo == nil:
			item.Status = "scheduler_default"
			response.SchedulerDefault++
		default:
			candidates := byNo[*task.LLMNo]
			switch len(candidates) {
			case 0:
				item.Status = "unavailable"
				item.Reason = fmt.Sprintf("当前模型列表中不存在 #%d", *task.LLMNo)
				response.Skipped++
			case 1:
				item.Status = "migratable"
				item.ModelKey = candidates[0].key
				item.ModelLabel = candidates[0].label
				response.Migratable++
			default:
				item.Status = "ambiguous"
				item.Reason = fmt.Sprintf("模型序号 #%d 对应多个当前模型", *task.LLMNo)
				response.Skipped++
			}
		}
		response.Items = append(response.Items, item)
	}
	return response
}

func scheduleRuntimeModelLabel(llm map[string]interface{}, index int) string {
	for _, field := range []string{"label", "model", "name", "provider"} {
		if value := strings.TrimSpace(fmt.Sprint(llm[field])); value != "" && value != "<nil>" {
			return value
		}
	}
	return fmt.Sprintf("#%d", index)
}

func migrateScheduleModelItem(root string, item *scheduleModelMigrationItem) error {
	if item == nil || item.ID == "" || item.LLMNo == nil || item.ModelKey == "" {
		return fmt.Errorf("任务缺少可迁移的模型标识")
	}
	raw, id, err := ga.ReadTask(root, item.ID)
	if err != nil {
		return fmt.Errorf("读取任务失败: %w", err)
	}
	if id != item.ID {
		return fmt.Errorf("任务标识已变化")
	}
	if existing, selected, err := ga.ScheduleTaskModelKey(raw); err != nil {
		return err
	} else if selected {
		return fmt.Errorf("任务已绑定稳定模型 %q", existing)
	}
	current, selected, err := ga.ScheduleTaskLLMNo(raw)
	if err != nil {
		return err
	}
	if !selected || current != *item.LLMNo {
		return fmt.Errorf("任务的模型序号已变化，请重新预览")
	}
	// Keep the legacy numeric hint untouched. It remains useful to older GA
	// runtimes, while model_key becomes authoritative for current dispatch.
	raw["model_key"] = item.ModelKey
	if _, err := ga.SaveTask(root, id, raw); err != nil {
		return fmt.Errorf("写入任务失败: %w", err)
	}
	return nil
}
