package ga

import (
	"testing"
	"time"
)

func TestApplyAutonomousRunEventsTracksStepsAndCheckpoint(t *testing.T) {
	now := time.Now()
	board := AutonomousTaskBoard{SchemaVersion: 1, Tasks: []AutonomousTask{{ID: "task-1", Title: "task", Status: TaskQueued, CreatedAt: now, UpdatedAt: now}}, Runs: []AutonomousRun{{ID: "run-1", TaskID: "task-1", Status: TaskQueued, UpdatedAt: now}}}
	events := []AutonomousRunEventInput{{Type: EventRunStarted, Message: "开始"}, {Type: EventStepStarted, StepName: "检查", StepOrder: 1, StepStatus: "running"}, {Type: EventStepProgress, StepName: "检查", StepOrder: 1, StepStatus: "completed", Progress: 50, OutputSummary: "已检查一半"}, {Type: EventCheckpointSaved, StepName: "检查", StepOrder: 1, Checkpoint: map[string]interface{}{"cursor": 3}, Resumable: true}}
	for _, event := range events {
		if err := ApplyAutonomousRunEvent(&board, "run-1", event); err != nil {
			t.Fatal(err)
		}
	}
	if board.Tasks[0].Status != TaskRunning || board.Runs[0].Status != TaskRunning || board.Runs[0].Progress != 50 {
		t.Fatalf("unexpected running state: task=%+v run=%+v", board.Tasks[0], board.Runs[0])
	}
	if len(board.Runs[0].Steps) != 1 || board.Runs[0].Steps[0].Progress != 50 || board.Runs[0].Checkpoint == nil || !board.Runs[0].Checkpoint.Resumable {
		t.Fatalf("step/checkpoint not recorded: %+v", board.Runs[0])
	}
	if len(board.Events) != len(events) {
		t.Fatalf("events = %d, want %d", len(board.Events), len(events))
	}
}

func TestApplyAutonomousRunEventRejectsInvalidTransitionAndInput(t *testing.T) {
	now := time.Now()
	board := AutonomousTaskBoard{Tasks: []AutonomousTask{{ID: "task-1", Title: "task", Status: TaskCompleted, CreatedAt: now, UpdatedAt: now}}, Runs: []AutonomousRun{{ID: "run-1", TaskID: "task-1", Status: TaskCompleted, UpdatedAt: now}}}
	if err := ApplyAutonomousRunEvent(&board, "run-1", AutonomousRunEventInput{Type: EventRunResumed}); err == nil {
		t.Fatal("expected terminal transition error")
	}
	if err := ApplyAutonomousRunEvent(&board, "run-1", AutonomousRunEventInput{Type: "unknown"}); err == nil {
		t.Fatal("expected unknown event error")
	}
	if err := ApplyAutonomousRunEvent(&board, "run-1", AutonomousRunEventInput{Type: EventStepProgress, Progress: 101}); err == nil {
		t.Fatal("expected progress validation error")
	}
}
