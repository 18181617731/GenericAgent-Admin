package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"genericagent-admin-go/internal/config"
	"genericagent-admin-go/internal/ga"
)

const manualScheduleRunRoot = "temp/ga-admin-schedule-runs"

type manualScheduleRunStatus struct {
	RunID      string `json:"run_id"`
	TaskID     string `json:"task_id"`
	Status     string `json:"status"`
	ReportPath string `json:"report_path"`
	LogPath    string `json:"log_path"`
	PID        int    `json:"pid,omitempty"`
	ReturnCode *int   `json:"return_code,omitempty"`
	Error      string `json:"error,omitempty"`
	StartedAt  string `json:"started_at"`
	FinishedAt string `json:"finished_at,omitempty"`
}

func (s *Server) scheduleRun(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		bad(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	cfg := s.CfgStore.Snapshot()
	raw, taskID, err := ga.ReadTask(cfg.GARoot, readScheduleRunID(r))
	if err != nil {
		bad(w, http.StatusBadRequest, err.Error())
		return
	}
	prompt, _ := raw["prompt"].(string)
	if strings.TrimSpace(prompt) == "" {
		bad(w, http.StatusBadRequest, "scheduled task prompt is required")
		return
	}
	llmNo, err := s.manualScheduleModelNo(cfg, raw)
	if err != nil {
		bad(w, http.StatusBadRequest, err.Error())
		return
	}
	runID := fmt.Sprintf("manual-%d", time.Now().UnixNano())
	runDir := filepath.Join(cfg.GARoot, filepath.FromSlash(manualScheduleRunRoot), runID)
	if err := os.MkdirAll(runDir, 0755); err != nil {
		bad(w, http.StatusInternalServerError, err.Error())
		return
	}
	reportPath := filepath.ToSlash(filepath.Join("sche_tasks", "done", time.Now().Format("2006-01-02_150405")+"_"+taskID+".md"))
	status := manualScheduleRunStatus{
		RunID: runID, TaskID: taskID, Status: "starting", ReportPath: reportPath,
		LogPath: filepath.ToSlash(filepath.Join(manualScheduleRunRoot, runID, "stdout.log")), StartedAt: time.Now().Format(time.RFC3339),
	}
	if err := writeManualScheduleRunStatus(runDir, status); err != nil {
		bad(w, http.StatusInternalServerError, err.Error())
		return
	}
	input := buildManualSchedulePrompt(taskID, prompt, reportPath)
	if err := os.WriteFile(filepath.Join(runDir, "input.txt"), []byte(input), 0644); err != nil {
		bad(w, http.StatusInternalServerError, err.Error())
		return
	}
	logFile, err := os.OpenFile(filepath.Join(runDir, "stdout.log"), os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0644)
	if err != nil {
		bad(w, http.StatusInternalServerError, err.Error())
		return
	}
	py := resolvePythonForRoot(cfg.GARoot, cfg.EffectivePython)
	cmd := newManualScheduleRunCommand(py, runID, llmNo)
	cmd.Dir = cfg.GARoot
	cmd.Stdout = logFile
	cmd.Stderr = logFile
	cmd.Env = pythonEnvWithAdminProxy(cfg, "PYTHONUNBUFFERED=1", "PYTHONUTF8=1", "PYTHONIOENCODING=utf-8", "GA_ADMIN_USAGE_DIR="+usageEventDir(cfg), "GA_ADMIN_USAGE_CHANNEL=scheduled_task", "GA_ADMIN_USAGE_SOURCE=manual")
	if err := cmd.Start(); err != nil {
		_ = logFile.Close()
		status.Status, status.Error, status.FinishedAt = "failed", err.Error(), time.Now().Format(time.RFC3339)
		_ = writeManualScheduleRunStatus(runDir, status)
		bad(w, http.StatusInternalServerError, err.Error())
		return
	}
	_ = logFile.Close()
	status.Status, status.PID = "running", cmd.Process.Pid
	if err := writeManualScheduleRunStatus(runDir, status); err != nil {
		_ = cmd.Process.Kill()
		bad(w, http.StatusInternalServerError, err.Error())
		return
	}
	go waitManualScheduleRun(cmd, runDir, status)
	writeJSON(w, status)
}

func newManualScheduleRunCommand(py, runID string, llmNo int) *exec.Cmd {
	cmd := exec.Command(py, "agentmain.py", "--task", filepath.ToSlash(filepath.Join("ga-admin-schedule-runs", runID)), "--nobg", "--llm_no", strconv.Itoa(llmNo))
	hideChildWindow(cmd)
	return cmd
}

func (s *Server) scheduleRunStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		bad(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	runID := strings.TrimSpace(r.URL.Query().Get("run_id"))
	if !validManualScheduleRunID(runID) {
		bad(w, http.StatusBadRequest, "invalid run_id")
		return
	}
	root := s.CfgStore.Snapshot().GARoot
	path := filepath.Join(root, filepath.FromSlash(manualScheduleRunRoot), runID, "status.json")
	data, err := os.ReadFile(path)
	if err != nil {
		bad(w, http.StatusNotFound, "schedule run not found")
		return
	}
	var status manualScheduleRunStatus
	if err := json.Unmarshal(data, &status); err != nil {
		bad(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, status)
}

func readScheduleRunID(r *http.Request) string {
	if r.Method == http.MethodPost {
		var req struct {
			ID string `json:"id"`
		}
		if err := decode(r, &req); err == nil {
			return strings.TrimSpace(req.ID)
		}
	}
	return strings.TrimSpace(r.URL.Query().Get("id"))
}

func (s *Server) manualScheduleModelNo(cfg config.AppConfig, raw map[string]any) (int, error) {
	llmNo, selected, err := ga.ScheduleTaskLLMNo(raw)
	if err != nil {
		return 0, err
	}
	if !selected {
		llmNo = cfg.ServiceModels["reflect/scheduler.py"]
	}
	if llmNo < 0 {
		return 0, fmt.Errorf("scheduled task model #%d is invalid", llmNo)
	}
	llms, err := s.listGARuntimeLLMs(cfg)
	if err == nil && len(llms) > 0 && !containsScheduleLLMNo(llms, llmNo) {
		return 0, fmt.Errorf("scheduled task model #%d is unavailable", llmNo)
	}
	return llmNo, nil
}

func buildManualSchedulePrompt(taskID, prompt, reportPath string) string {
	return fmt.Sprintf("[定时任务手动执行] %s\n[报告路径] %s\n\n先读 scheduled_task_sop 了解执行流程，然后执行以下任务：\n\n%s\n\n完成后将执行报告写入 %s。", taskID, reportPath, prompt, reportPath)
}

func waitManualScheduleRun(cmd *exec.Cmd, runDir string, status manualScheduleRunStatus) {
	err := cmd.Wait()
	code := 0
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			code = exitErr.ExitCode()
		} else {
			code = -1
		}
	}
	status.ReturnCode = &code
	status.Status = "completed"
	if code != 0 {
		status.Status = "failed"
		status.Error = fmt.Sprintf("manual schedule process exited with code %d", code)
	}
	status.FinishedAt = time.Now().Format(time.RFC3339)
	_ = writeManualScheduleRunStatus(runDir, status)
}

func writeManualScheduleRunStatus(runDir string, status manualScheduleRunStatus) error {
	data, err := json.MarshalIndent(status, "", "  ")
	if err != nil {
		return err
	}
	tmp := filepath.Join(runDir, ".status.json.tmp")
	if err := os.WriteFile(tmp, data, 0644); err != nil {
		return err
	}
	return os.Rename(tmp, filepath.Join(runDir, "status.json"))
}

func validManualScheduleRunID(value string) bool {
	if value == "" || len(value) > 80 {
		return false
	}
	for _, ch := range value {
		if (ch < 'a' || ch > 'z') && (ch < '0' || ch > '9') && ch != '-' && ch != '_' {
			return false
		}
	}
	return true
}
