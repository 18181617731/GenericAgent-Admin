package ga

import (
	"errors"
	"fmt"
	"strings"
	"time"
)

type AutonomousRunEventInput struct {
	Type          string                 `json:"type"`
	Message       string                 `json:"message,omitempty"`
	StepName      string                 `json:"step_name,omitempty"`
	StepOrder     int                    `json:"step_order,omitempty"`
	StepStatus    string                 `json:"step_status,omitempty"`
	Progress      int                    `json:"progress,omitempty"`
	OutputSummary string                 `json:"output_summary,omitempty"`
	Evidence      []string               `json:"evidence,omitempty"`
	ReportPath    string                 `json:"report_path,omitempty"`
	BlockReason   string                 `json:"block_reason,omitempty"`
	Checkpoint    map[string]interface{} `json:"checkpoint,omitempty"`
	Resumable     bool                   `json:"resumable,omitempty"`
}

func ApplyAutonomousRunEvent(board *AutonomousTaskBoard, runID string, input AutonomousRunEventInput) error {
	if board == nil {
		return errors.New("autonomous task board is required")
	}
	if !autonomousRunEventTypes[input.Type] {
		return fmt.Errorf("unsupported autonomous run event %q", input.Type)
	}
	run, err := findAutonomousRun(board, strings.TrimSpace(runID))
	if err != nil {
		return err
	}
	task, err := findAutonomousTask(board, run.TaskID)
	if err != nil {
		return err
	}
	if input.Progress < 0 || input.Progress > 100 {
		return errors.New("progress must be between 0 and 100")
	}
	if len([]rune(input.StepName)) > 200 || len([]rune(input.OutputSummary)) > 4000 || len([]rune(input.BlockReason)) > 2000 {
		return errors.New("run event text is too long")
	}
	now := time.Now()
	run.LastEventAt = now
	run.UpdatedAt = now
	if input.ReportPath != "" {
		run.ReportPath = input.ReportPath
		task.ReportPath = input.ReportPath
	}
	if input.Progress > 0 || input.Type == EventStepProgress {
		run.Progress = input.Progress
		task.Progress = input.Progress
	}
	if input.StepName != "" {
		updateAutonomousStep(run, input, now)
		task.CurrentStage = input.StepName
	}
	switch input.Type {
	case EventRunStarted:
		if !CanTransitionAutonomousTask(task.Status, TaskRunning) {
			return fmt.Errorf("task cannot become running from %q", task.Status)
		}
		run.Status, task.Status = TaskRunning, TaskRunning
		if run.StartedAt.IsZero() {
			run.StartedAt = now
		}
	case EventApprovalRequired:
		run.Status, task.Status = TaskBlocked, TaskBlocked
		task.BlockReason = firstNonEmptyTaskText(input.BlockReason, input.Message, "需要人工审批")
	case EventStepBlocked:
		run.Status, task.Status = TaskBlocked, TaskBlocked
		task.BlockReason = firstNonEmptyTaskText(input.BlockReason, input.Message, "步骤被阻塞")
	case EventStepFailed:
		run.Status, task.Status = TaskFailed, TaskFailed
		run.Error, task.BlockReason = firstNonEmptyTaskText(input.Message, input.BlockReason, "步骤执行失败"), input.BlockReason
		run.FinishedAt = now
	case EventCheckpointSaved:
		run.Checkpoint = &AutonomousCheckpoint{StepOrder: input.StepOrder, StepName: input.StepName, State: input.Checkpoint, SavedAt: now, Resumable: input.Resumable}
	case EventRunPaused:
		if !CanTransitionAutonomousTask(task.Status, TaskPaused) {
			return fmt.Errorf("task cannot become paused from %q", task.Status)
		}
		run.Status, task.Status, run.PauseReason = TaskPaused, TaskPaused, firstNonEmptyTaskText(input.Message, "等待当前步骤结束")
	case EventRunResumed:
		if !CanTransitionAutonomousTask(task.Status, TaskRunning) {
			return fmt.Errorf("task cannot become running from %q", task.Status)
		}
		run.Status, task.Status, run.PauseReason = TaskRunning, TaskRunning, ""
	case EventRunCompleted:
		if !CanTransitionAutonomousTask(task.Status, TaskCompleted) {
			return fmt.Errorf("task cannot become completed from %q", task.Status)
		}
		run.Status, task.Status, run.Progress = TaskCompleted, TaskCompleted, 100
		task.Progress, run.FinishedAt = 100, now
	case EventRunCancelled:
		if !CanTransitionAutonomousTask(task.Status, TaskCancelled) {
			return fmt.Errorf("task cannot become cancelled from %q", task.Status)
		}
		run.Status, task.Status, run.FinishedAt = TaskCancelled, TaskCancelled, now
	case EventStepStarted, EventStepProgress:
		if task.Status == TaskQueued && CanTransitionAutonomousTask(task.Status, TaskRunning) {
			task.Status, run.Status = TaskRunning, TaskRunning
			if run.StartedAt.IsZero() {
				run.StartedAt = now
			}
		}
	}
	if input.Message != "" {
		task.NextStep = input.Message
	}
	task.UpdatedAt = now
	AppendAutonomousTaskEvent(board, task.ID, run.ID, input.Type, input.Message, map[string]interface{}{
		"step_name": input.StepName, "step_order": input.StepOrder, "progress": input.Progress,
		"step_status": input.StepStatus, "output_summary": input.OutputSummary,
		"evidence": input.Evidence, "report_path": input.ReportPath,
	})
	return nil
}

func updateAutonomousStep(run *AutonomousRun, input AutonomousRunEventInput, now time.Time) {
	for index := range run.Steps {
		if run.Steps[index].Order == input.StepOrder && run.Steps[index].Name == input.StepName {
			applyAutonomousStep(&run.Steps[index], input, now)
			return
		}
	}
	step := AutonomousStep{Name: input.StepName, Order: input.StepOrder, Status: input.StepStatus}
	applyAutonomousStep(&step, input, now)
	run.Steps = append(run.Steps, step)
}

func applyAutonomousStep(step *AutonomousStep, input AutonomousRunEventInput, now time.Time) {
	if input.StepStatus != "" {
		step.Status = input.StepStatus
	}
	if input.Progress > 0 || input.Type == EventStepProgress {
		step.Progress = input.Progress
	}
	if input.OutputSummary != "" {
		step.OutputSummary = input.OutputSummary
	}
	if len(input.Evidence) > 0 {
		step.Evidence = append([]string(nil), input.Evidence...)
	}
	if input.ReportPath != "" {
		step.ReportPath = input.ReportPath
	}
	if input.BlockReason != "" {
		step.BlockReason = input.BlockReason
	}
	if input.Type == EventStepStarted && step.StartedAt.IsZero() {
		step.StartedAt = now
	}
	if input.Type == EventStepFailed || input.Type == EventStepBlocked {
		step.FinishedAt = now
	}
}
