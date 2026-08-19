package ga

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"sort"
	"strings"
	"sync"
	"time"
)

const autonomousTaskSchemaVersion = 1

const autonomousTaskMigrationVersion = 1

const (
	TaskDraft           = "draft"
	TaskPendingApproval = "pending_approval"
	TaskQueued          = "queued"
	TaskRunning         = "running"
	TaskPaused          = "paused"
	TaskBlocked         = "blocked"
	TaskFailed          = "failed"
	TaskCompleted       = "completed"
	TaskCancelled       = "cancelled"
)

const (
	autonomousTaskStorePath  = "temp/autonomous/tasks.json"
	autonomousRunStorePath   = "temp/autonomous/runs.json"
	autonomousEventStorePath = "temp/autonomous/events.json"
	autonomousControlDir     = "temp/autonomous/control"
)

var autonomousTaskMu sync.Mutex

type AutonomousTask struct {
	ID           string    `json:"id"`
	Title        string    `json:"title"`
	Objective    string    `json:"objective"`
	Status       string    `json:"status"`
	SourceType   string    `json:"source_type,omitempty"`
	SourcePath   string    `json:"source_path,omitempty"`
	SourceLine   int       `json:"source_line,omitempty"`
	Priority     string    `json:"priority,omitempty"`
	Risk         string    `json:"risk,omitempty"`
	Project      string    `json:"project,omitempty"`
	ScheduleAt   time.Time `json:"schedule_at,omitempty"`
	DueAt        time.Time `json:"due_at,omitempty"`
	CurrentStage string    `json:"current_stage,omitempty"`
	Progress     int       `json:"progress,omitempty"`
	BlockReason  string    `json:"block_reason,omitempty"`
	NextStep     string    `json:"next_step,omitempty"`
	Owner        string    `json:"owner,omitempty"`
	ApprovalNote string    `json:"approval_note,omitempty"`
	ReportPath   string    `json:"report_path,omitempty"`
	LastRunID    string    `json:"last_run_id,omitempty"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
	Imported     bool      `json:"imported,omitempty"`
}

type AutonomousRun struct {
	ID          string    `json:"id"`
	TaskID      string    `json:"task_id"`
	Status      string    `json:"status"`
	Stage       string    `json:"stage,omitempty"`
	Progress    int       `json:"progress,omitempty"`
	Service     string    `json:"service,omitempty"`
	PID         int       `json:"pid,omitempty"`
	RetryCount  int       `json:"retry_count,omitempty"`
	Error       string    `json:"error,omitempty"`
	ReportPath  string    `json:"report_path,omitempty"`
	StartedAt   time.Time `json:"started_at,omitempty"`
	FinishedAt  time.Time `json:"finished_at,omitempty"`
	UpdatedAt   time.Time `json:"updated_at"`
	PauseReason string    `json:"pause_reason,omitempty"`
	ControlPath string    `json:"control_path,omitempty"`
}

type AutonomousEvent struct {
	ID        string                 `json:"id"`
	TaskID    string                 `json:"task_id"`
	RunID     string                 `json:"run_id,omitempty"`
	Type      string                 `json:"type"`
	Message   string                 `json:"message,omitempty"`
	CreatedAt time.Time              `json:"created_at"`
	Data      map[string]interface{} `json:"data,omitempty"`
}

type autonomousTaskLedger struct {
	SchemaVersion    int              `json:"schema_version"`
	MigrationVersion int              `json:"migration_version,omitempty"`
	Tasks            []AutonomousTask `json:"tasks"`
}

type autonomousRunLedger struct {
	SchemaVersion int             `json:"schema_version"`
	Runs          []AutonomousRun `json:"runs"`
}

type autonomousEventLedger struct {
	SchemaVersion int               `json:"schema_version"`
	Events        []AutonomousEvent `json:"events"`
}

type AutonomousTaskBoard struct {
	SchemaVersion    int               `json:"schema_version"`
	MigrationVersion int               `json:"migration_version"`
	Tasks            []AutonomousTask  `json:"tasks"`
	Runs             []AutonomousRun   `json:"runs"`
	Events           []AutonomousEvent `json:"events"`
}

func LoadAutonomousTaskBoard(root string) (AutonomousTaskBoard, error) {
	autonomousTaskMu.Lock()
	defer autonomousTaskMu.Unlock()
	return loadAutonomousTaskBoardUnlocked(root)
}

func loadAutonomousTaskBoardUnlocked(root string) (AutonomousTaskBoard, error) {
	board := AutonomousTaskBoard{SchemaVersion: autonomousTaskSchemaVersion, Tasks: []AutonomousTask{}, Runs: []AutonomousRun{}, Events: []AutonomousEvent{}}
	var tasks autonomousTaskLedger
	if err := readJSONLedger(root, autonomousTaskStorePath, &tasks); err == nil {
		if tasks.SchemaVersion > autonomousTaskSchemaVersion {
			return board, fmt.Errorf("unsupported autonomous task schema_version %d", tasks.SchemaVersion)
		}
		board.MigrationVersion = tasks.MigrationVersion
		board.Tasks = append(board.Tasks, tasks.Tasks...)
	} else if !os.IsNotExist(err) {
		return board, err
	}
	var runs autonomousRunLedger
	if err := readJSONLedger(root, autonomousRunStorePath, &runs); err == nil {
		if runs.SchemaVersion > autonomousTaskSchemaVersion {
			return board, fmt.Errorf("unsupported autonomous run schema_version %d", runs.SchemaVersion)
		}
		board.Runs = append(board.Runs, runs.Runs...)
	} else if !os.IsNotExist(err) {
		return board, err
	}
	var events autonomousEventLedger
	if err := readJSONLedger(root, autonomousEventStorePath, &events); err == nil {
		if events.SchemaVersion > autonomousTaskSchemaVersion {
			return board, fmt.Errorf("unsupported autonomous event schema_version %d", events.SchemaVersion)
		}
		board.Events = append(board.Events, events.Events...)
	} else if !os.IsNotExist(err) {
		return board, err
	}
	if board.MigrationVersion < autonomousTaskMigrationVersion {
		if err := migrateAutonomousApprovals(root, &board); err != nil {
			return board, err
		}
		board.MigrationVersion = autonomousTaskMigrationVersion
		if err := saveAutonomousTaskBoardUnlocked(root, board); err != nil {
			return board, err
		}
	}
	sort.SliceStable(board.Tasks, func(i, j int) bool { return board.Tasks[i].UpdatedAt.After(board.Tasks[j].UpdatedAt) })
	sort.SliceStable(board.Runs, func(i, j int) bool { return board.Runs[i].UpdatedAt.After(board.Runs[j].UpdatedAt) })
	sort.SliceStable(board.Events, func(i, j int) bool { return board.Events[i].CreatedAt.After(board.Events[j].CreatedAt) })
	return board, nil
}

func readJSONLedger(root, rel string, dst interface{}) error {
	path, _, err := SafeResolve(root, rel)
	if err != nil {
		return err
	}
	b, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	return json.Unmarshal(b, dst)
}

func saveAutonomousTaskBoardUnlocked(root string, board AutonomousTaskBoard) error {
	if err := writeAutonomousLedger(root, autonomousTaskStorePath, autonomousTaskLedger{SchemaVersion: autonomousTaskSchemaVersion, MigrationVersion: autonomousTaskMigrationVersion, Tasks: board.Tasks}); err != nil {
		return err
	}
	if err := writeAutonomousLedger(root, autonomousRunStorePath, autonomousRunLedger{SchemaVersion: autonomousTaskSchemaVersion, Runs: board.Runs}); err != nil {
		return err
	}
	return writeAutonomousLedger(root, autonomousEventStorePath, autonomousEventLedger{SchemaVersion: autonomousTaskSchemaVersion, Events: board.Events})
}

func writeAutonomousLedger(root, rel string, value interface{}) error {
	path, _, err := SafeResolve(root, rel)
	if err != nil {
		return err
	}
	b, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	if err := ensureWriteParentWithinRoot(root, path); err != nil {
		return err
	}
	return writeFileAtomic(path, append(b, '\n'), 0644)
}

func migrateAutonomousApprovals(root string, board *AutonomousTaskBoard) error {
	overview, err := BuildAutonomousApprovals(root)
	if err != nil {
		return err
	}
	now := time.Now()
	for _, item := range overview.Items {
		status := "pending_approval"
		switch {
		case item.State == "closed" || item.ExecutionState == autonomousExecutionCompleted:
			status = "completed"
		case item.Decision == "rejected":
			status = "cancelled"
		case item.Decision == "approved":
			status = "queued"
		case item.ExecutionState == autonomousExecutionFailed:
			status = "failed"
		}
		board.Tasks = append(board.Tasks, AutonomousTask{ID: "task-" + item.ID, Title: item.Title, Objective: firstNonEmptyTaskText(item.Problem, item.Title), Status: status, SourceType: item.CandidateSource, SourcePath: firstNonEmptyTaskText(item.Source, item.DraftPath), Priority: "normal", Risk: item.Risk, CurrentStage: taskStageForStatus(status), NextStep: item.NextStep, ApprovalNote: item.Note, ReportPath: autonomousTaskReportPath(item), CreatedAt: now, UpdatedAt: now, Imported: true})
	}
	return nil
}

func firstNonEmptyTaskText(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}
func taskStageForStatus(status string) string {
	switch status {
	case "completed":
		return "已完成"
	case "failed":
		return "失败待重试"
	case "queued":
		return "等待执行"
	case "running":
		return "执行中"
	default:
		return "等待审核"
	}
}
func autonomousTaskReportPath(item AutonomousApproval) string {
	if item.ExecutionReport != nil {
		return item.ExecutionReport.Path
	}
	if item.ReviewReport != nil {
		return item.ReviewReport.Path
	}
	return ""
}

func makeAutonomousTaskID(title string, now time.Time) string {
	sum := sha256.Sum256([]byte(strings.TrimSpace(title) + "|" + now.UTC().Format(time.RFC3339Nano)))
	return "task-" + hex.EncodeToString(sum[:8])
}
func makeAutonomousRunID(taskID string, now time.Time) string {
	sum := sha256.Sum256([]byte(taskID + "|" + now.UTC().Format(time.RFC3339Nano)))
	return "run-" + hex.EncodeToString(sum[:8])
}
func makeAutonomousEventID(taskID, eventType string, now time.Time) string {
	sum := sha256.Sum256([]byte(taskID + "|" + eventType + "|" + now.UTC().Format(time.RFC3339Nano)))
	return "evt-" + hex.EncodeToString(sum[:8])
}

func NewAutonomousTaskID(title string, now time.Time) string { return makeAutonomousTaskID(title, now) }
func NewAutonomousRunID(taskID string, now time.Time) string { return makeAutonomousRunID(taskID, now) }
func AppendAutonomousTaskEvent(board *AutonomousTaskBoard, taskID, runID, eventType, message string, data map[string]interface{}) {
	appendAutonomousTaskEvent(board, taskID, runID, eventType, message, data)
}

func ValidateAutonomousTask(task AutonomousTask) error {
	if strings.TrimSpace(task.Title) == "" || len([]rune(task.Title)) > 200 {
		return errors.New("title must be between 1 and 200 characters")
	}
	if len([]rune(task.Objective)) > 4000 {
		return errors.New("objective is too long")
	}
	if task.Progress < 0 || task.Progress > 100 {
		return errors.New("progress must be between 0 and 100")
	}
	if !isAutonomousTaskStatus(task.Status) {
		return errors.New("status is required")
	}
	return nil
}

func isAutonomousTaskStatus(status string) bool {
	switch status {
	case TaskDraft, TaskPendingApproval, TaskQueued, TaskRunning, TaskPaused, TaskBlocked, TaskFailed, TaskCompleted, TaskCancelled:
		return true
	default:
		return false
	}
}

func CanTransitionAutonomousTask(from, to string) bool {
	if from == to {
		return true
	}
	switch from {
	case TaskDraft:
		return to == TaskPendingApproval || to == TaskCancelled
	case TaskPendingApproval:
		return to == TaskQueued || to == TaskCancelled || to == TaskDraft
	case TaskQueued:
		return to == TaskRunning || to == TaskCancelled || to == TaskFailed
	case TaskRunning:
		return to == TaskPaused || to == TaskBlocked || to == TaskFailed || to == TaskCompleted || to == TaskCancelled
	case TaskPaused:
		return to == TaskRunning || to == TaskCancelled || to == TaskFailed
	case TaskBlocked:
		return to == TaskRunning || to == TaskCancelled || to == TaskFailed
	case TaskFailed:
		return to == TaskQueued || to == TaskCancelled
	default:
		return false
	}
}

func appendAutonomousTaskEvent(board *AutonomousTaskBoard, taskID, runID, eventType, message string, data map[string]interface{}) {
	now := time.Now()
	board.Events = append(board.Events, AutonomousEvent{ID: makeAutonomousEventID(taskID, eventType, now), TaskID: taskID, RunID: runID, Type: eventType, Message: strings.TrimSpace(message), CreatedAt: now, Data: data})
}

func findAutonomousTask(board *AutonomousTaskBoard, id string) (*AutonomousTask, error) {
	for i := range board.Tasks {
		if board.Tasks[i].ID == id {
			return &board.Tasks[i], nil
		}
	}
	return nil, fmt.Errorf("autonomous task %q not found", id)
}
func findAutonomousRun(board *AutonomousTaskBoard, id string) (*AutonomousRun, error) {
	for i := range board.Runs {
		if board.Runs[i].ID == id {
			return &board.Runs[i], nil
		}
	}
	return nil, fmt.Errorf("autonomous run %q not found", id)
}

func SaveAutonomousTaskBoard(root string, board AutonomousTaskBoard) error {
	autonomousTaskMu.Lock()
	defer autonomousTaskMu.Unlock()
	return saveAutonomousTaskBoardUnlocked(root, board)
}

func WriteAutonomousControlSignal(root, runID, action, note string) (string, error) {
	if strings.TrimSpace(runID) == "" || strings.TrimSpace(action) == "" {
		return "", errors.New("run_id and action are required")
	}
	if !isAutonomousControlAction(action) {
		return "", fmt.Errorf("unsupported autonomous control action %q", action)
	}
	rel := autonomousControlDir + "/" + runID + ".json"
	path, _, err := SafeResolve(root, rel)
	if err != nil {
		return "", err
	}
	if err := ensureWriteParentWithinRoot(root, path); err != nil {
		return "", err
	}
	payload, err := json.MarshalIndent(map[string]interface{}{"run_id": runID, "action": action, "note": strings.TrimSpace(note), "created_at": time.Now()}, "", "  ")
	if err != nil {
		return "", err
	}
	if err := writeFileAtomic(path, append(payload, '\n'), 0644); err != nil {
		return "", err
	}
	return rel, nil
}

func isAutonomousControlAction(action string) bool {
	switch action {
	case "pause", "resume", "cancel":
		return true
	default:
		return false
	}
}
