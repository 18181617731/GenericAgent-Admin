package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"genericagent-admin-go/internal/ga"
)

func (s *Server) autonomousTasks(w http.ResponseWriter, r *http.Request) {
	root := strings.TrimSpace(s.CfgStore.Snapshot().GARoot)
	if root == "" {
		bad(w, http.StatusBadRequest, "ga_root is not configured")
		return
	}
	if r.URL.Path == "/api/autonomous/tasks" || r.URL.Path == "/api/autonomous/tasks/" {
		if r.Method == http.MethodGet {
			s.listAutonomousTasks(w, r, root)
			return
		}
		if r.Method == http.MethodPost {
			s.createAutonomousTask(w, r, root)
			return
		}
		bad(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	id := strings.Trim(strings.TrimPrefix(r.URL.Path, "/api/autonomous/tasks/"), "/")
	if id == "parse" {
		s.parseAutonomousTaskInput(w, r)
		return
	}
	parts := strings.Split(id, "/")
	if len(parts) == 2 && parts[1] == "runs" && r.Method == http.MethodGet {
		s.listAutonomousTaskRuns(w, root, parts[0])
		return
	}
	if len(parts) == 1 && r.Method == http.MethodGet {
		s.getAutonomousTask(w, root, parts[0])
		return
	}
	if len(parts) == 1 && r.Method == http.MethodPut {
		s.updateAutonomousTask(w, r, root, parts[0])
		return
	}
	if len(parts) == 2 && r.Method == http.MethodPost {
		s.autonomousTaskAction(w, r, root, parts[0], parts[1])
		return
	}
	bad(w, http.StatusNotFound, "autonomous task route not found")
}

func (s *Server) listAutonomousTasks(w http.ResponseWriter, r *http.Request, root string) {
	board, err := ga.LoadAutonomousTaskBoard(root)
	if err != nil {
		bad(w, 500, err.Error())
		return
	}
	q := r.URL.Query()
	status, source, project, risk, priority, search := strings.TrimSpace(q.Get("status")), strings.TrimSpace(q.Get("source")), strings.TrimSpace(q.Get("project")), strings.TrimSpace(q.Get("risk")), strings.TrimSpace(q.Get("priority")), strings.ToLower(strings.TrimSpace(q.Get("q")))
	filtered := make([]ga.AutonomousTask, 0, len(board.Tasks))
	for _, task := range board.Tasks {
		if status != "" && task.Status != status || source != "" && task.SourceType != source || project != "" && task.Project != project || risk != "" && task.Risk != risk || priority != "" && task.Priority != priority {
			continue
		}
		if search != "" && !strings.Contains(strings.ToLower(task.Title+" "+task.Objective+" "+task.Project), search) {
			continue
		}
		filtered = append(filtered, task)
	}
	writeJSON(w, map[string]interface{}{"schema_version": board.SchemaVersion, "tasks": filtered, "runs": board.Runs, "events": board.Events, "total": len(filtered)})
}

func (s *Server) getAutonomousTask(w http.ResponseWriter, root, id string) {
	board, err := ga.LoadAutonomousTaskBoard(root)
	if err != nil {
		bad(w, 500, err.Error())
		return
	}
	task, err := findTask(board, id)
	if err != nil {
		bad(w, 404, err.Error())
		return
	}
	runs := make([]ga.AutonomousRun, 0)
	events := make([]ga.AutonomousEvent, 0)
	for _, run := range board.Runs {
		if run.TaskID == id {
			runs = append(runs, run)
		}
	}
	for _, event := range board.Events {
		if event.TaskID == id {
			events = append(events, event)
		}
	}
	writeJSON(w, map[string]interface{}{"task": task, "runs": runs, "events": events})
}

func (s *Server) listAutonomousTaskRuns(w http.ResponseWriter, root, id string) {
	board, err := ga.LoadAutonomousTaskBoard(root)
	if err != nil {
		bad(w, http.StatusInternalServerError, err.Error())
		return
	}
	if _, err := findTask(board, id); err != nil {
		bad(w, http.StatusNotFound, err.Error())
		return
	}
	runs := make([]ga.AutonomousRun, 0)
	for _, run := range board.Runs {
		if run.TaskID == id {
			runs = append(runs, run)
		}
	}
	writeJSON(w, map[string]interface{}{"task_id": id, "runs": runs})
}

func (s *Server) createAutonomousTask(w http.ResponseWriter, r *http.Request, root string) {
	var input ga.AutonomousTask
	if err := decode(r, &input); err != nil {
		bad(w, 400, err.Error())
		return
	}
	now := time.Now()
	input.ID = ga.NewAutonomousTaskID(input.Title, now)
	input.Status = ga.TaskPendingApproval
	input.SourceType = "todo"
	input.SourcePath = "temp/TODO.txt"
	input.Imported = true
	input.CreatedAt = now
	input.UpdatedAt = now
	if err := ga.ValidateAutonomousTask(input); err != nil {
		bad(w, 400, err.Error())
		return
	}
	board, err := ga.LoadAutonomousTaskBoard(root)
	if err != nil {
		bad(w, 500, err.Error())
		return
	}
	line, err := ga.AppendAutonomousTodoTask(root, input)
	if err != nil {
		bad(w, 500, err.Error())
		return
	}
	board, err = ga.LoadAutonomousTaskBoard(root)
	if err != nil {
		bad(w, 500, err.Error())
		return
	}
	created := findAutonomousTaskBySourceLine(board, line)
	if created == nil {
		bad(w, 500, "created autonomous TODO task could not be reloaded")
		return
	}
	ga.AppendAutonomousTaskEvent(&board, created.ID, "", "created", "任务已创建，等待批准", nil)
	if err := ga.SaveAutonomousTaskBoard(root, board); err != nil {
		bad(w, 500, err.Error())
		return
	}
	writeJSON(w, map[string]interface{}{"ok": true, "task": created})
}

func findAutonomousTaskBySourceLine(board ga.AutonomousTaskBoard, line int) *ga.AutonomousTask {
	for index := range board.Tasks {
		if board.Tasks[index].SourceLine == line {
			return &board.Tasks[index]
		}
	}
	return nil
}

func (s *Server) updateAutonomousTask(w http.ResponseWriter, r *http.Request, root, id string) {
	var input ga.AutonomousTask
	if err := decode(r, &input); err != nil {
		bad(w, 400, err.Error())
		return
	}
	board, err := ga.LoadAutonomousTaskBoard(root)
	if err != nil {
		bad(w, 500, err.Error())
		return
	}
	task, err := findTask(board, id)
	if err != nil {
		bad(w, 404, err.Error())
		return
	}
	if task.Status != ga.TaskDraft && task.Status != ga.TaskPendingApproval {
		bad(w, http.StatusConflict, "only draft or pending approval tasks can be edited")
		return
	}
	if task.SourceType == "todo" && strings.TrimSpace(input.Title) != "" && strings.TrimSpace(input.Title) != strings.TrimSpace(task.Title) {
		bad(w, http.StatusConflict, "TODO-backed task titles are immutable; edit TODO.txt directly")
		return
	}
	if input.Title != "" {
		task.Title = input.Title
	}
	if input.Objective != "" {
		task.Objective = input.Objective
	}
	if input.Priority != "" {
		task.Priority = input.Priority
	}
	if input.Risk != "" {
		task.Risk = input.Risk
	}
	if input.Project != "" {
		task.Project = input.Project
	}
	if !input.DueAt.IsZero() {
		task.DueAt = input.DueAt
	}
	if !input.ScheduleAt.IsZero() {
		task.ScheduleAt = input.ScheduleAt
	}
	if input.Owner != "" {
		task.Owner = input.Owner
	}
	task.NextStep, task.BlockReason = input.NextStep, input.BlockReason
	task.UpdatedAt = time.Now()
	if err := ga.ValidateAutonomousTask(*task); err != nil {
		bad(w, 400, err.Error())
		return
	}
	if task.SourceType == "todo" {
		if _, err := ga.UpdateAutonomousTodoTaskDetails(root, id, task.Title, task.Objective, task.NextStep); err != nil {
			bad(w, http.StatusConflict, err.Error())
			return
		}
	}
	ga.AppendAutonomousTaskEvent(&board, id, task.LastRunID, "updated", "任务配置已更新", nil)
	if err := ga.SaveAutonomousTaskBoard(root, board); err != nil {
		bad(w, 500, err.Error())
		return
	}
	writeJSON(w, map[string]interface{}{"ok": true, "task": task})
}

func (s *Server) autonomousTaskAction(w http.ResponseWriter, r *http.Request, root, id, action string) {
	board, err := ga.LoadAutonomousTaskBoard(root)
	if err != nil {
		bad(w, 500, err.Error())
		return
	}
	task, err := findTask(board, id)
	if err != nil {
		bad(w, 404, err.Error())
		return
	}
	var note string
	var input map[string]interface{}
	_ = json.NewDecoder(r.Body).Decode(&input)
	if input != nil {
		if value, ok := input["note"].(string); ok {
			note = strings.TrimSpace(value)
		}
	}
	now := time.Now()
	eventType, message := action, ""
	switch action {
	case "duplicate":
		copy := *task
		copy.Title = task.Title + "（副本）"
		copy.Status = ga.TaskPendingApproval
		copy.LastRunID = ""
		copy.CreatedAt, copy.UpdatedAt = now, now
		copy.Imported = true
		line, appendErr := ga.AppendAutonomousTodoTask(root, copy)
		if appendErr != nil {
			bad(w, http.StatusInternalServerError, appendErr.Error())
			return
		}
		board, err = ga.LoadAutonomousTaskBoard(root)
		if err != nil {
			bad(w, http.StatusInternalServerError, err.Error())
			return
		}
		task = findAutonomousTaskBySourceLine(board, line)
		if task == nil {
			bad(w, http.StatusInternalServerError, "duplicated autonomous TODO task could not be reloaded")
			return
		}
		eventType, message = "duplicated", "任务已复制"
	case "approve":
		if task.Status != ga.TaskPendingApproval {
			bad(w, http.StatusConflict, "task cannot be approved from its current state")
			return
		}
		if _, err := ga.UpdateAutonomousTodoTask(root, id, ga.TaskQueued, note); err != nil {
			bad(w, http.StatusConflict, err.Error())
			return
		}
		task.Status, task.ApprovalNote, task.CurrentStage, eventType, message = ga.TaskQueued, note, "排队中", "approved", "任务已批准并排队"
	case "reject":
		if task.Status != ga.TaskPendingApproval {
			bad(w, http.StatusConflict, "task cannot be rejected from its current state")
			return
		}
		if _, err := ga.UpdateAutonomousTodoTask(root, id, ga.TaskPendingApproval, note); err != nil {
			bad(w, http.StatusConflict, err.Error())
			return
		}
		task.Status, task.ApprovalNote, task.CurrentStage, eventType, message = ga.TaskPendingApproval, note, "待批准", "rejected", "任务仍待批准"
	case "start", "retry":
		if action == "start" && task.Status != ga.TaskQueued {
			bad(w, http.StatusConflict, "only queued tasks can be started")
			return
		}
		if action == "retry" && task.Status != ga.TaskFailed {
			bad(w, http.StatusConflict, "only failed tasks can be retried")
			return
		}
		run := ga.AutonomousRun{ID: ga.NewAutonomousRunID(id, now), TaskID: id, Status: "queued", Stage: "排队中", Service: "reflect/autonomous.py", RetryCount: 0, UpdatedAt: now}
		if action == "retry" {
			run.RetryCount = countTaskRetries(board, id) + 1
		}
		board.Runs = append(board.Runs, run)
		task.LastRunID, task.Status, task.CurrentStage, task.Progress = run.ID, ga.TaskQueued, "排队中", 0
		eventType, message = action+"ed", "任务已加入执行队列"
	case "pause", "resume", "cancel":
		if action == "cancel" && task.LastRunID == "" {
			if task.Status == ga.TaskCompleted {
				bad(w, http.StatusConflict, "closed tasks cannot be cancelled")
				return
			}
			if _, err := ga.UpdateAutonomousTodoTask(root, id, ga.TaskPendingApproval, note); err != nil {
				bad(w, http.StatusConflict, err.Error())
				return
			}
			task.Status, task.CurrentStage, eventType, message = ga.TaskPendingApproval, "待批准", "cancelled", "任务已退回待批准"
			break
		}
		run, runErr := latestTaskRun(board, id)
		if runErr != nil {
			bad(w, 409, runErr.Error())
			return
		}
		if action == "pause" {
			if run.Status != "running" {
				bad(w, http.StatusConflict, "only running tasks can be paused")
				return
			}
			run.Status, run.PauseReason, task.Status, task.CurrentStage, eventType, message = "paused", note, ga.TaskQueued, "排队中", "paused", "已请求暂停，任务保持排队中"
		}
		if action == "resume" {
			if run.Status != "paused" && run.Status != "blocked" {
				bad(w, http.StatusConflict, "only paused or blocked tasks can be resumed")
				return
			}
			run.Status, run.PauseReason, task.Status, task.CurrentStage, eventType, message = "running", "", ga.TaskQueued, "排队中", "resumed", "任务已恢复排队"
		}
		if action == "cancel" {
			if run.Status == "completed" || run.Status == "cancelled" {
				bad(w, http.StatusConflict, "terminal tasks cannot be cancelled")
				return
			}
			run.Status, run.FinishedAt, task.Status, task.CurrentStage, eventType, message = "cancelled", now, ga.TaskPendingApproval, "待批准", "cancelled", "任务已退回待批准"
			if _, err := ga.UpdateAutonomousTodoTask(root, id, ga.TaskPendingApproval, note); err != nil {
				bad(w, http.StatusConflict, err.Error())
				return
			}
		}
		run.UpdatedAt = now
		if action == "pause" || action == "resume" || action == "cancel" {
			controlPath, controlErr := ga.WriteAutonomousControlSignal(root, run.ID, action, note)
			if controlErr != nil {
				bad(w, http.StatusInternalServerError, controlErr.Error())
				return
			}
			run.ControlPath = controlPath
		}
	case "nudge":
		eventType, message = "nudged", "已记录催办要求"
	default:
		bad(w, 404, "unknown autonomous task action")
		return
	}
	task.UpdatedAt = now
	if note != "" {
		task.NextStep = note
	}
	ga.AppendAutonomousTaskEvent(&board, id, task.LastRunID, eventType, message, map[string]interface{}{"note": note})
	if err := ga.SaveAutonomousTaskBoard(root, board); err != nil {
		bad(w, 500, err.Error())
		return
	}
	writeJSON(w, map[string]interface{}{"ok": true, "task": task, "runs": board.Runs})
}

func (s *Server) autonomousRuns(w http.ResponseWriter, r *http.Request) {
	root := strings.TrimSpace(s.CfgStore.Snapshot().GARoot)
	if root == "" {
		bad(w, 400, "ga_root is not configured")
		return
	}
	id := strings.Trim(strings.TrimPrefix(r.URL.Path, "/api/autonomous/runs/"), "/")
	parts := strings.Split(id, "/")
	if len(parts) == 2 && parts[1] == "events" && r.Method == http.MethodGet {
		s.listAutonomousRunEvents(w, root, parts[0])
		return
	}
	if len(parts) == 2 && parts[1] == "events" && r.Method == http.MethodPost {
		s.appendAutonomousRunEvent(w, r, root, parts[0])
		return
	}
	if len(parts) != 1 {
		bad(w, http.StatusNotFound, "autonomous run route not found")
		return
	}
	board, err := ga.LoadAutonomousTaskBoard(root)
	if err != nil {
		bad(w, 500, err.Error())
		return
	}
	run, err := findRun(board, id)
	if err != nil {
		bad(w, 404, err.Error())
		return
	}
	events := make([]ga.AutonomousEvent, 0)
	for _, event := range board.Events {
		if event.RunID == id {
			events = append(events, event)
		}
	}
	writeJSON(w, map[string]interface{}{"run": run, "events": events})
}

func (s *Server) appendAutonomousRunEvent(w http.ResponseWriter, r *http.Request, root, runID string) {
	var input ga.AutonomousRunEventInput
	if err := decode(r, &input); err != nil {
		bad(w, http.StatusBadRequest, err.Error())
		return
	}
	if input.ReportPath != "" {
		if _, _, err := ga.SafeResolve(root, input.ReportPath); err != nil {
			bad(w, http.StatusBadRequest, "report_path must stay within the configured GA root")
			return
		}
	}
	board, err := ga.LoadAutonomousTaskBoard(root)
	if err != nil {
		bad(w, http.StatusInternalServerError, err.Error())
		return
	}
	runBefore, err := findRun(board, runID)
	if err != nil {
		bad(w, http.StatusNotFound, err.Error())
		return
	}
	taskID := runBefore.TaskID
	if err := ga.ApplyAutonomousRunEvent(&board, runID, input); err != nil {
		code := http.StatusBadRequest
		if strings.Contains(err.Error(), "not found") || strings.Contains(err.Error(), "cannot become") {
			code = http.StatusConflict
		}
		bad(w, code, err.Error())
		return
	}
	if input.Type == ga.EventRunCompleted {
		if _, err := ga.UpdateAutonomousTodoTask(root, taskID, ga.TaskCompleted, ""); err != nil {
			bad(w, http.StatusInternalServerError, err.Error())
			return
		}
	}
	if input.Type == ga.EventApprovalRequired || input.Type == ga.EventRunCancelled {
		if _, err := ga.UpdateAutonomousTodoTask(root, taskID, ga.TaskPendingApproval, input.Message); err != nil {
			bad(w, http.StatusInternalServerError, err.Error())
			return
		}
	}
	if err := ga.SaveAutonomousTaskBoard(root, board); err != nil {
		bad(w, http.StatusInternalServerError, err.Error())
		return
	}
	run, _ := findRun(board, runID)
	writeJSON(w, map[string]interface{}{"ok": true, "run": run})
}

func (s *Server) listAutonomousRunEvents(w http.ResponseWriter, root, id string) {
	board, err := ga.LoadAutonomousTaskBoard(root)
	if err != nil {
		bad(w, http.StatusInternalServerError, err.Error())
		return
	}
	if _, err := findRun(board, id); err != nil {
		bad(w, http.StatusNotFound, err.Error())
		return
	}
	events := make([]ga.AutonomousEvent, 0)
	for _, event := range board.Events {
		if event.RunID == id {
			events = append(events, event)
		}
	}
	writeJSON(w, map[string]interface{}{"run_id": id, "events": events})
}

func findTask(board ga.AutonomousTaskBoard, id string) (*ga.AutonomousTask, error) {
	for i := range board.Tasks {
		if board.Tasks[i].ID == id {
			return &board.Tasks[i], nil
		}
	}
	return nil, fmt.Errorf("autonomous task %q not found", id)
}
func findRun(board ga.AutonomousTaskBoard, id string) (*ga.AutonomousRun, error) {
	for i := range board.Runs {
		if board.Runs[i].ID == id {
			return &board.Runs[i], nil
		}
	}
	return nil, fmt.Errorf("autonomous run %q not found", id)
}
func latestTaskRun(board ga.AutonomousTaskBoard, taskID string) (*ga.AutonomousRun, error) {
	var latest *ga.AutonomousRun
	for i := range board.Runs {
		if board.Runs[i].TaskID == taskID && (latest == nil || board.Runs[i].UpdatedAt.After(latest.UpdatedAt)) {
			latest = &board.Runs[i]
		}
	}
	if latest == nil {
		return nil, fmt.Errorf("task has no run")
	}
	return latest, nil
}
func countTaskRetries(board ga.AutonomousTaskBoard, taskID string) int {
	count := 0
	for _, run := range board.Runs {
		if run.TaskID == taskID {
			count += run.RetryCount
		}
	}
	return count
}
