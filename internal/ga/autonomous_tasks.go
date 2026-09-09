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
	ID          string                `json:"id"`
	TaskID      string                `json:"task_id"`
	Status      string                `json:"status"`
	Stage       string                `json:"stage,omitempty"`
	Progress    int                   `json:"progress,omitempty"`
	Service     string                `json:"service,omitempty"`
	PID         int                   `json:"pid,omitempty"`
	RetryCount  int                   `json:"retry_count,omitempty"`
	Error       string                `json:"error,omitempty"`
	ReportPath  string                `json:"report_path,omitempty"`
	StartedAt   time.Time             `json:"started_at,omitempty"`
	FinishedAt  time.Time             `json:"finished_at,omitempty"`
	UpdatedAt   time.Time             `json:"updated_at"`
	PauseReason string                `json:"pause_reason,omitempty"`
	ControlPath string                `json:"control_path,omitempty"`
	Steps       []AutonomousStep      `json:"steps,omitempty"`
	Checkpoint  *AutonomousCheckpoint `json:"checkpoint,omitempty"`
	LastEventAt time.Time             `json:"last_event_at,omitempty"`
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

type AutonomousStep struct {
	Name          string    `json:"name"`
	Order         int       `json:"order"`
	Status        string    `json:"status"`
	Progress      int       `json:"progress,omitempty"`
	StartedAt     time.Time `json:"started_at,omitempty"`
	FinishedAt    time.Time `json:"finished_at,omitempty"`
	OutputSummary string    `json:"output_summary,omitempty"`
	Evidence      []string  `json:"evidence,omitempty"`
	ReportPath    string    `json:"report_path,omitempty"`
	BlockReason   string    `json:"block_reason,omitempty"`
}

type AutonomousCheckpoint struct {
	StepOrder int                    `json:"step_order"`
	StepName  string                 `json:"step_name"`
	State     map[string]interface{} `json:"state,omitempty"`
	SavedAt   time.Time              `json:"saved_at"`
	Resumable bool                   `json:"resumable"`
}

const (
	EventRunStarted       = "run_started"
	EventStepStarted      = "step_started"
	EventStepProgress     = "step_progress"
	EventApprovalRequired = "approval_required"
	EventStepBlocked      = "step_blocked"
	EventStepFailed       = "step_failed"
	EventCheckpointSaved  = "checkpoint_saved"
	EventRunPaused        = "run_paused"
	EventRunResumed       = "run_resumed"
	EventRunCompleted     = "run_completed"
	EventRunCancelled     = "run_cancelled"
)

var autonomousRunEventTypes = map[string]bool{
	EventRunStarted: true, EventStepStarted: true, EventStepProgress: true,
	EventApprovalRequired: true, EventStepBlocked: true, EventStepFailed: true,
	EventCheckpointSaved: true, EventRunPaused: true, EventRunResumed: true,
	EventRunCompleted: true, EventRunCancelled: true,
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
	tasks, err := readAutonomousTaskLedger(root)
	if err != nil {
		return board, err
	}
	board.MigrationVersion = tasks.MigrationVersion
	runBytes, err := readOptionalJSONLedger(root, autonomousRunStorePath)
	if err != nil {
		return board, err
	}
	if len(runBytes) > 0 {
		var runs autonomousRunLedger
		if json.Unmarshal(runBytes, &runs) == nil && runs.SchemaVersion <= autonomousTaskSchemaVersion {
			board.Runs = append(board.Runs, runs.Runs...)
		}
	}
	eventBytes, err := readOptionalJSONLedger(root, autonomousEventStorePath)
	if err != nil {
		return board, err
	}
	if len(eventBytes) > 0 {
		var events autonomousEventLedger
		if json.Unmarshal(eventBytes, &events) == nil && events.SchemaVersion <= autonomousTaskSchemaVersion {
			board.Events = append(board.Events, events.Events...)
		}
	}
	// TODO.txt is the sole source of autonomous task rows. The JSON task
	// ledger remains only as a compatibility store for run metadata; its rows
	// must not reappear when a task list is rebuilt.
	todoTasks, err := loadAutonomousTodoTasks(root, tasks.Tasks)
	if err != nil {
		return board, err
	}
	board.Tasks = todoTasks
	if board.MigrationVersion < autonomousTaskMigrationVersion {
		board.MigrationVersion = autonomousTaskMigrationVersion
	}
	filterAutonomousTaskHistory(&board)
	sort.SliceStable(board.Tasks, func(i, j int) bool { return board.Tasks[i].UpdatedAt.After(board.Tasks[j].UpdatedAt) })
	sort.SliceStable(board.Runs, func(i, j int) bool { return board.Runs[i].UpdatedAt.After(board.Runs[j].UpdatedAt) })
	sort.SliceStable(board.Events, func(i, j int) bool { return board.Events[i].CreatedAt.After(board.Events[j].CreatedAt) })
	return board, nil
}

// loadAutonomousTodoTasks projects TODO.txt into the small public task model.
// Stored rows are consulted only for execution metadata; source fields and
// status are recalculated from TODO on every read.
func loadAutonomousTodoTasks(root string, stored []AutonomousTask) ([]AutonomousTask, error) {
	overview, err := BuildProjectTodos(root)
	if err != nil {
		return nil, err
	}
	storedByID := make(map[string]AutonomousTask, len(stored))
	for _, task := range stored {
		storedByID[task.ID] = task
		if task.SourcePath == autonomousTodoPath && task.SourceLine > 0 {
			storedByID[autonomousTodoLineKey(task.SourceLine)] = task
		}
	}
	tasks := make([]AutonomousTask, 0, len(overview.Items))
	now := time.Now()
	lines := autonomousTodoSourceLines(root)
	for _, item := range overview.Items {
		marker, body, _ := autonomousTodoChecklistAtLine(lines, item.Line)
		status := autonomousTaskStatusFromTodo(item, marker, body)
		title, objective, nextStep := autonomousTodoTaskFields(lines, item)
		task := AutonomousTask{
			ID: projectTodoID(title, item.Line), Title: title, Objective: objective, Status: status,
			SourceType: "todo", SourcePath: item.SourcePath, SourceLine: item.Line,
			Priority: item.Priority, CurrentStage: autonomousTaskStage(status),
			NextStep: nextStep, Progress: autonomousTaskProgress(status), CreatedAt: now, UpdatedAt: now,
			Imported: true,
		}
		previous, ok := storedByID[item.ID]
		if !ok {
			previous, ok = storedByID[autonomousTodoLineKey(item.Line)]
		}
		if ok {
			task.CreatedAt, task.UpdatedAt = previous.CreatedAt, previous.UpdatedAt
			task.Risk, task.Project = previous.Risk, previous.Project
			task.ScheduleAt, task.DueAt = previous.ScheduleAt, previous.DueAt
			task.Progress, task.BlockReason = previous.Progress, previous.BlockReason
			task.Owner, task.ApprovalNote = previous.Owner, previous.ApprovalNote
			task.ReportPath, task.LastRunID = previous.ReportPath, previous.LastRunID
		}
		if task.CreatedAt.IsZero() {
			task.CreatedAt = now
		}
		if task.UpdatedAt.IsZero() {
			task.UpdatedAt = now
		}
		if status == TaskCompleted {
			task.Progress = 100
		}
		tasks = append(tasks, task)
	}
	return tasks, nil
}

func autonomousTodoChecklistAtLine(lines []string, line int) (rune, string, bool) {
	if line < 1 || line > len(lines) {
		return 0, "", false
	}
	return projectTodoChecklist(lines[line-1])
}

func autonomousTodoLineKey(line int) string {
	return fmt.Sprintf("todo-line:%d", line)
}

func autonomousTodoSourceLines(root string) []string {
	detail, err := ReadSafe(root, autonomousTodoPath)
	if err != nil {
		return nil
	}
	return strings.Split(strings.ReplaceAll(detail.Content, "\r\n", "\n"), "\n")
}

func autonomousTodoTaskFields(lines []string, item ProjectTodoItem) (string, string, string) {
	if item.Line < 1 || item.Line > len(lines) {
		return item.Title, item.Summary, ""
	}
	_, body, ok := projectTodoChecklist(lines[item.Line-1])
	if !ok {
		return item.Title, item.Summary, ""
	}
	title, objectiveParts := autonomousTodoCanonicalFields(body)
	if title == "" {
		title = item.Title
	}
	nextStep := ""
	if len(objectiveParts) > 0 && (projectTodoNextStep(objectiveParts[len(objectiveParts)-1]) || containsAny(strings.ToLower(objectiveParts[len(objectiveParts)-1]), "批准后", "批准并")) {
		nextStep = objectiveParts[len(objectiveParts)-1]
	}
	if nextStep != "" && len(objectiveParts) > 0 {
		objectiveParts = objectiveParts[:len(objectiveParts)-1]
	}
	if autonomousTodoRoundTitle(title) {
		// BuildProjectTodos treats a round token such as R11 as a label and
		// keeps the following pipe-delimited description in the title. Build
		// the same canonical title before and after adding a decision prefix,
		// otherwise a status transition changes the derived task ID.
		title = strings.Join(append([]string{title}, objectiveParts...), " · ")
		objectiveParts = nil
	}
	objective := strings.Join(objectiveParts, " | ")
	if objective == "" && nextStep == "" {
		objective = item.Summary
	}
	return title, objective, nextStep
}

func autonomousTodoRoundTitle(value string) bool {
	value = strings.TrimSpace(value)
	if len(value) < 2 || (value[0] != 'R' && value[0] != 'r') {
		return false
	}
	for _, char := range value[1:] {
		if char < '0' || char > '9' {
			return false
		}
	}
	return true
}

func autonomousTodoCanonicalIdentityTitle(body string) string {
	title, parts := autonomousTodoCanonicalFields(body)
	if !autonomousTodoRoundTitle(title) {
		return title
	}
	if len(parts) > 0 && (projectTodoNextStep(parts[len(parts)-1]) || containsAny(strings.ToLower(parts[len(parts)-1]), "批准后", "批准并")) {
		parts = parts[:len(parts)-1]
	}
	return strings.Join(append([]string{title}, parts...), " · ")
}

// autonomousTodoCanonicalFields parses the stable, user-facing TODO shape:
// an optional decision prefix followed by title, objective, and next step.
// Plain checklist rows use their first pipe-delimited field as the title.
func autonomousTodoCanonicalFields(body string) (string, []string) {
	clean := strings.TrimSpace(projectTodoCommentPattern.ReplaceAllString(body, ""))
	parts := strings.FieldsFunc(clean, func(r rune) bool { return r == '|' || r == '｜' })
	parts = slicesWithoutEmptyStrings(parts)
	parts = autonomousTodoCanonicalParts(parts)
	if len(parts) == 0 {
		return "", nil
	}
	return parts[0], parts[1:]
}

func autonomousTaskStatusFromTodo(item ProjectTodoItem, marker rune, body string) string {
	// The checklist marker is authoritative for closure, even if the text
	// still contains an old approval phrase.
	if marker == 'x' || marker == 'X' || (marker == 0 && item.Status == "completed") {
		return TaskCompleted
	}
	if autonomousTodoHasExplicitPendingMarker(body) {
		return TaskPendingApproval
	}
	// An unchecked TODO row is executable by default. Approval is opt-in and
	// must be expressed by a narrow, unambiguous marker on that same row.
	return TaskQueued
}

func autonomousTodoHasExplicitPendingMarker(body string) bool {
	clean := strings.TrimSpace(projectTodoCommentPattern.ReplaceAllString(body, ""))
	parts := strings.FieldsFunc(clean, func(r rune) bool { return r == '|' || r == '｜' })
	parts = slicesWithoutEmptyStrings(parts)
	return len(parts) > 0 && (autonomousTodoPendingPrefix(parts[0]) || autonomousTodoPendingTokenPrefix(parts[0]))
}

func autonomousTodoPendingPrefix(value string) bool {
	clean := strings.TrimSpace(value)
	for _, label := range []string{"待批准", "待审批", "待人工批准", "待人工审批"} {
		if autonomousTodoTokenPrefix(clean, label) {
			return true
		}
	}
	return false
}

func autonomousTodoPendingTokenPrefix(value string) bool {
	clean := strings.TrimSpace(value)
	for _, marker := range []string{"【待批准】", "【待审批】", "[待批准]", "[待审批]"} {
		if autonomousTodoTokenPrefix(clean, marker) {
			return true
		}
	}
	return false
}

func autonomousTodoTokenPrefix(value, token string) bool {
	if value == token {
		return true
	}
	if !strings.HasPrefix(value, token) {
		return false
	}
	rest := value[len(token):]
	return strings.HasPrefix(rest, ":") || strings.HasPrefix(rest, "：") || strings.HasPrefix(rest, " ")
}

func autonomousTodoDecisionPrefix(value string) bool {
	clean := strings.TrimSpace(value)
	if autonomousTodoPendingPrefix(clean) || autonomousTodoPendingTokenPrefix(clean) {
		return true
	}
	for _, marker := range []string{"【待批准】", "【待审批】", "[待批准]", "[待审批]", "排队中", "用户已批准", "已批准", "已审批", "用户已拒绝", "已拒绝", "已驳回"} {
		if autonomousTodoTokenPrefix(clean, marker) {
			return true
		}
	}
	return false
}

// autonomousTodoDecisionRemainder splits a decision prefix from an inline
// title, for example "待批准: task" -> "task". A prefix without a title is
// represented by an empty remainder. The delimiter requirement keeps prose
// such as "待批准任务" from being mistaken for a decision marker.
func autonomousTodoDecisionRemainder(value string) (string, bool) {
	clean := strings.TrimSpace(value)
	for _, token := range []string{
		"【待人工批准】", "【待人工审批】", "【待批准】", "【待审批】",
		"[待人工批准]", "[待人工审批]", "[待批准]", "[待审批]",
		"待人工批准", "待人工审批", "待批准", "待审批",
		"用户已拒绝", "已拒绝", "已驳回", "用户已批准", "已批准", "已审批", "排队中",
	} {
		if clean == token {
			return "", true
		}
		if !strings.HasPrefix(clean, token) {
			continue
		}
		rest := clean[len(token):]
		if strings.HasPrefix(rest, ":") {
			return strings.TrimSpace(strings.TrimPrefix(rest, ":")), true
		}
		if strings.HasPrefix(rest, "：") {
			return strings.TrimSpace(strings.TrimPrefix(rest, "：")), true
		}
		if strings.HasPrefix(rest, " ") || strings.HasPrefix(rest, "\t") {
			return strings.TrimSpace(rest), true
		}
	}
	return "", false
}

func autonomousTodoCanonicalParts(parts []string) []string {
	if len(parts) == 0 || !autonomousTodoDecisionPrefix(parts[0]) {
		return parts
	}
	remainder, ok := autonomousTodoDecisionRemainder(parts[0])
	if !ok || remainder == "" {
		return parts[1:]
	}
	return append([]string{remainder}, parts[1:]...)
}

func autonomousTaskProgress(status string) int {
	if status == TaskCompleted {
		return 100
	}
	return 0
}

func autonomousTaskStage(status string) string {
	switch status {
	case TaskCompleted:
		return "已闭环"
	case TaskQueued:
		return "排队中"
	default:
		return "待批准"
	}
}

// UpdateAutonomousTodoTask changes the canonical TODO row for a task. The
// operation intentionally updates one existing row instead of appending a
// second task, so TODO.txt remains the only task source.
func UpdateAutonomousTodoTask(root, id, state, note string) (bool, error) {
	if state != TaskPendingApproval && state != TaskQueued && state != TaskCompleted {
		return false, fmt.Errorf("unsupported autonomous TODO state %q", state)
	}
	if len([]rune(note)) > 1000 {
		return false, errors.New("TODO note is too long")
	}
	path, _, err := SafeResolve(root, autonomousTodoPath)
	if err != nil {
		return false, err
	}
	content, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return false, fmt.Errorf("autonomous TODO source not found")
		}
		return false, err
	}
	lines := strings.Split(strings.ReplaceAll(string(content), "\r\n", "\n"), "\n")
	updated := false
	for index, line := range lines {
		marker, body, ok := projectTodoChecklist(line)
		if !ok || !autonomousTodoLineMatches(body, id, index+1) {
			continue
		}
		if state == TaskQueued && marker != 'x' && marker != 'X' && strings.Contains(body, "ga-admin-approval:"+id) && containsAny(strings.ToLower(body), "排队中", "用户已批准", "已批准", "已审批") {
			return false, nil
		}
		if state == TaskCompleted && (marker == 'x' || marker == 'X') {
			return false, nil
		}
		if state == TaskPendingApproval && (marker == 'x' || marker == 'X') {
			return false, errors.New("closed autonomous TODO cannot be moved back to approval")
		}
		lines[index] = rewriteAutonomousTodoLine(line, body, state, id, note)
		updated = true
		break
	}
	if !updated {
		return false, fmt.Errorf("autonomous TODO task %q not found", strings.TrimSpace(id))
	}
	return true, writeAutonomousTodoContent(root, path, strings.Join(lines, "\n"))
}

// UpdateAutonomousTodoTaskDetails keeps the compatibility PUT route anchored
// to the same TODO row instead of persisting edits only in tasks.json.
func UpdateAutonomousTodoTaskDetails(root, id, title, objective, nextStep string) (bool, error) {
	path, _, err := SafeResolve(root, autonomousTodoPath)
	if err != nil {
		return false, err
	}
	content, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return false, fmt.Errorf("autonomous TODO source not found")
		}
		return false, err
	}
	lines := strings.Split(strings.ReplaceAll(string(content), "\r\n", "\n"), "\n")
	for index, line := range lines {
		marker, body, ok := projectTodoChecklist(line)
		if !ok || !autonomousTodoLineMatches(body, id, index+1) {
			continue
		}
		if marker == 'x' || marker == 'X' {
			return false, errors.New("closed autonomous TODO cannot be edited")
		}
		canonicalTitle, _ := autonomousTodoCanonicalFields(body)
		if strings.TrimSpace(title) == "" {
			title = canonicalTitle
		}
		if strings.TrimSpace(title) == "" {
			return false, errors.New("autonomous TODO title cannot be empty")
		}
		state := autonomousTodoDecisionState(body)
		lines[index] = rewriteAutonomousTodoFieldsLine(line, state, id, title, objective, nextStep)
		return true, writeAutonomousTodoContent(root, path, strings.Join(lines, "\n"))
	}
	return false, fmt.Errorf("autonomous TODO task %q not found", strings.TrimSpace(id))
}

func autonomousTodoDecisionState(body string) string {
	if autonomousTodoHasExplicitPendingMarker(body) {
		return TaskPendingApproval
	}
	return TaskQueued
}

func rewriteAutonomousTodoFieldsLine(line, state, id, title, objective, nextStep string) string {
	position := strings.Index(line, "[")
	prefix := ""
	if position >= 0 {
		prefix = line[:position]
	}
	decision := "待批准"
	if state == TaskQueued {
		decision = "排队中"
	}
	parts := []string{decision, sanitizeAutonomousTodoText(title)}
	for _, value := range []string{objective, nextStep} {
		if clean := sanitizeAutonomousTodoText(value); clean != "" {
			parts = append(parts, clean)
		}
	}
	body := strings.Join(parts, " | ")
	if state == TaskQueued {
		body += " <!-- ga-admin-approval:" + id + " -->"
	}
	marker := " "
	return prefix + "[" + marker + "] " + body
}

func autonomousTodoLineMatches(body, id string, line int) bool {
	id = strings.TrimSpace(id)
	if id == "" {
		return false
	}
	if strings.Contains(body, "ga-admin-approval:"+id) || strings.Contains(body, "ga-admin-task:"+id) {
		return true
	}
	title := autonomousTodoCanonicalIdentityTitle(body)
	legacyTitle, _ := autonomousTodoTitle(body)
	projectTitle, _ := projectTodoTitle(body)
	canonicalTitle, _ := autonomousTodoCanonicalFields(body)
	return autonomousApprovalID(title) == id || autonomousApprovalID(legacyTitle) == id || projectTodoID(title, line) == id || projectTodoID(canonicalTitle, line) == id || projectTodoID(projectTitle, line) == id
}

func rewriteAutonomousTodoLine(line, body, state, id, note string) string {
	position := strings.Index(line, "[")
	prefix := ""
	if position >= 0 {
		prefix = line[:position]
	}
	clean := strings.TrimSpace(projectTodoCommentPattern.ReplaceAllString(body, ""))
	parts := strings.FieldsFunc(clean, func(r rune) bool { return r == '|' || r == '｜' })
	parts = slicesWithoutEmptyStrings(parts)
	if len(parts) == 0 {
		parts = []string{"待批准"}
	}
	if state == TaskQueued || state == TaskPendingApproval {
		parts = autonomousTodoCanonicalParts(parts)
		if len(parts) == 0 {
			parts = []string{"待批准"}
		}
	}
	switch state {
	case TaskQueued:
		if autonomousTodoDecisionPrefix(parts[0]) {
			parts[0] = "排队中"
		} else {
			parts = append([]string{"排队中"}, parts...)
		}
		if reply := autonomousApprovalReply(note); reply != "" && !strings.Contains(clean, "用户补充：") {
			parts = append(parts, "用户补充："+reply)
		}
	case TaskPendingApproval:
		if autonomousTodoDecisionPrefix(parts[0]) {
			parts[0] = "待批准"
		} else {
			parts = append([]string{"待批准"}, parts...)
		}
	case TaskCompleted:
		// The checked marker is the completion record; keep the task text intact.
	}
	newMarker := ' '
	if state == TaskCompleted {
		newMarker = 'x'
	}
	newBody := strings.Join(parts, " | ")
	if state == TaskQueued && !strings.Contains(newBody, "ga-admin-approval:"+id) {
		newBody += " <!-- ga-admin-approval:" + id + " -->"
	}
	return prefix + "[" + string(newMarker) + "] " + newBody
}

func writeAutonomousTodoContent(root, path, content string) error {
	if err := ensureWriteParentWithinRoot(root, path); err != nil {
		return err
	}
	return writeFileAtomic(path, []byte(content), 0644)
}

// AppendAutonomousTodoTask adds a newly created task in the same canonical
// three-state format used by the list parser. It returns the source line.
func AppendAutonomousTodoTask(root string, task AutonomousTask) (int, error) {
	path, _, err := SafeResolve(root, autonomousTodoPath)
	if err != nil {
		return 0, err
	}
	content, err := os.ReadFile(path)
	if err != nil && !os.IsNotExist(err) {
		return 0, err
	}
	parts := []string{"待批准", sanitizeAutonomousTodoText(task.Title)}
	if objective := sanitizeAutonomousTodoText(task.Objective); objective != "" {
		parts = append(parts, objective)
	}
	if next := sanitizeAutonomousTodoText(task.NextStep); next != "" {
		parts = append(parts, next)
	}
	text := strings.TrimRight(strings.ReplaceAll(string(content), "\r\n", "\n"), "\n")
	if text != "" {
		text += "\n"
	}
	line := "[ ] " + strings.Join(parts, " | ")
	text += line + "\n"
	if err := ensureWriteParentWithinRoot(root, path); err != nil {
		return 0, err
	}
	if err := writeFileAtomic(path, []byte(text), 0644); err != nil {
		return 0, err
	}
	return strings.Count(text, "\n"), nil
}

func sanitizeAutonomousTodoText(value string) string {
	value = strings.Join(strings.Fields(value), " ")
	value = strings.ReplaceAll(value, "|", "／")
	value = strings.ReplaceAll(value, "｜", "／")
	value = strings.ReplaceAll(value, "<!--", "&lt;!--")
	value = strings.ReplaceAll(value, "-->", "--&gt;")
	return strings.TrimSpace(value)
}

func filterAutonomousTaskHistory(board *AutonomousTaskBoard) {
	ids := make(map[string]bool, len(board.Tasks))
	for _, task := range board.Tasks {
		ids[task.ID] = true
	}
	runs := board.Runs[:0]
	for _, run := range board.Runs {
		if ids[run.TaskID] {
			runs = append(runs, run)
		}
	}
	board.Runs = runs
	events := board.Events[:0]
	for _, event := range board.Events {
		if ids[event.TaskID] {
			events = append(events, event)
		}
	}
	board.Events = events
}

func readOptionalJSONLedger(root, rel string) ([]byte, error) {
	path, _, err := SafeResolve(root, rel)
	if err != nil {
		return nil, err
	}
	b, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	// Run and event ledgers are derived metadata. Return raw bytes so callers
	// can decode into a temporary ledger and discard malformed or future data
	// without exposing a partially populated destination.
	return b, nil
}

func readAutonomousTaskLedger(root string) (autonomousTaskLedger, error) {
	path, _, err := SafeResolve(root, autonomousTaskStorePath)
	if err != nil {
		return autonomousTaskLedger{}, err
	}
	b, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return autonomousTaskLedger{}, nil
	}
	if err != nil {
		return autonomousTaskLedger{}, err
	}
	// tasks.json is compatibility metadata only. Decode into a temporary value
	// so malformed or future data can never block the TODO-backed task list or
	// leak a partially populated ledger into the public board.
	var candidate autonomousTaskLedger
	if err := json.Unmarshal(b, &candidate); err != nil || candidate.SchemaVersion > autonomousTaskSchemaVersion {
		return autonomousTaskLedger{}, nil
	}
	return candidate, nil
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

func firstNonEmptyTaskText(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
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
