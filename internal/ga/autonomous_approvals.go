package ga

import (
	"crypto/sha256"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode"
)

const autonomousApprovalSchemaVersion = 1

const (
	autonomousApprovalSource = "temp/pending_drafts.md"
	autonomousDecisionPath   = "temp/autonomous_approval_decisions.json"
	autonomousReviewPath     = "temp/autonomous_approval_reviews.json"
	autonomousTodoPath       = "temp/TODO.txt"
)

const (
	autonomousExecutionQueued        = "queued"
	autonomousExecutionCompleted     = "completed"
	autonomousExecutionFailed        = "failed"
	autonomousExecutionReportMissing = "report_missing"
	autonomousExecutionNotApplicable = "not_applicable"
)

const (
	autonomousCandidateSourceDraft  = "pending_draft"
	autonomousCandidateSourceTodo   = "todo"
	autonomousCandidateSourceReport = "autonomous_report"
	autonomousReviewNeedsApproval   = "needs_approval"
	autonomousApprovalNotRequired   = "not_required"
)

type AutonomousApproval struct {
	ID              string    `json:"id"`
	Title           string    `json:"title"`
	Source          string    `json:"source,omitempty"`
	DraftPath       string    `json:"draft_path,omitempty"`
	CandidateSource string    `json:"candidate_source,omitempty"`
	Target          string    `json:"target,omitempty"`
	Problem         string    `json:"problem,omitempty"`
	Status          string    `json:"status,omitempty"`
	Risk            string    `json:"risk,omitempty"`
	Evidence        string    `json:"evidence,omitempty"`
	NextStep        string    `json:"next_step,omitempty"`
	ExpectedOutcome string    `json:"expected_outcome,omitempty"`
	State           string    `json:"state"`
	Decision        string    `json:"decision,omitempty"`
	Note            string    `json:"note,omitempty"`
	DecidedAt       time.Time `json:"decided_at,omitempty"`

	ExecutionState     string     `json:"execution_state,omitempty"`
	ExecutionReport    *Entry     `json:"execution_report,omitempty"`
	ExecutionSummary   string     `json:"execution_summary,omitempty"`
	ExecutionError     string     `json:"execution_error,omitempty"`
	ExecutionUpdatedAt *time.Time `json:"execution_updated_at,omitempty"`

	ReviewStatus      string                   `json:"review_status,omitempty"`
	ReviewDecision    string                   `json:"review_decision,omitempty"`
	ReviewConfidence  string                   `json:"review_confidence,omitempty"`
	ReviewReason      string                   `json:"review_reason,omitempty"`
	ReviewModelNo     *int                     `json:"review_model_no,omitempty"`
	ReviewModel       string                   `json:"review_model,omitempty"`
	ReviewProvider    string                   `json:"review_provider,omitempty"`
	ReviewReport      *Entry                   `json:"review_report,omitempty"`
	ReviewReports     []Entry                  `json:"review_reports,omitempty"`
	ReviewAttempts    int                      `json:"review_attempts,omitempty"`
	ReviewNextRetryAt time.Time                `json:"review_next_retry_at,omitempty"`
	ReviewTags        []string                 `json:"review_tags,omitempty"`
	ReviewFocus       string                   `json:"review_focus,omitempty"`
	ReviewOptions     []AutonomousReviewOption `json:"review_options,omitempty"`
}

type AutonomousReviewOption struct {
	Key         string `json:"key"`
	Title       string `json:"title"`
	Summary     string `json:"summary,omitempty"`
	Recommended bool   `json:"recommended,omitempty"`
}

type AutonomousApprovalOverview struct {
	SchemaVersion  int                  `json:"schema_version"`
	SourcePath     string               `json:"source_path"`
	SourceExists   bool                 `json:"source_exists"`
	Items          []AutonomousApproval `json:"items"`
	Pending        int                  `json:"pending"`
	Approved       int                  `json:"approved"`
	Rejected       int                  `json:"rejected"`
	GeneratedAt    time.Time            `json:"generated_at"`
	ReviewModelNo  *int                 `json:"review_model_no,omitempty"`
	ReviewModel    string               `json:"review_model,omitempty"`
	ReviewProvider string               `json:"review_provider,omitempty"`
	ReviewStatus   string               `json:"review_status,omitempty"`
}

type autonomousDecision struct {
	ID        string    `json:"id"`
	Title     string    `json:"title"`
	Decision  string    `json:"decision"`
	Note      string    `json:"note,omitempty"`
	DecidedAt time.Time `json:"decided_at"`
}

type autonomousDecisionLedger struct {
	SchemaVersion int                  `json:"schema_version"`
	Decisions     []autonomousDecision `json:"decisions"`
}

var autonomousDecisionMu sync.Mutex
var autonomousApprovalNow = time.Now

func BuildAutonomousApprovals(root string) (AutonomousApprovalOverview, error) {
	overview := AutonomousApprovalOverview{SchemaVersion: autonomousApprovalSchemaVersion, SourcePath: autonomousApprovalSource, Items: make([]AutonomousApproval, 0), GeneratedAt: autonomousApprovalNow()}
	content, sourceExists, err := readAutonomousApprovalSource(root, autonomousApprovalSource)
	if err != nil {
		return overview, err
	}
	if sourceExists {
		overview.SourceExists = true
		overview.Items = parseAutonomousApprovals(content)
	}
	todoContent, todoExists, err := readAutonomousApprovalSource(root, autonomousTodoPath)
	if err != nil {
		return overview, err
	}
	if todoExists {
		overview.SourceExists = true
		if !sourceExists {
			overview.SourcePath = autonomousTodoPath
		}
		overview.Items = mergeAutonomousApprovals(overview.Items, parseAutonomousTodoApprovals(todoContent))
	}
	decisions, err := loadAutonomousDecisions(root)
	if err != nil {
		return overview, err
	}
	applyAutonomousDecisions(&overview, decisions)
	reports := buildAllAutonomousReports(root)
	contents := loadAutonomousReportContents(root, reports)
	attachAutonomousReportCandidates(root, &overview, reports, contents)
	reviews, err := loadAutonomousReviews(root)
	if err != nil {
		return overview, err
	}
	applyAutonomousReviews(&overview, reviews)
	attachAutonomousExecution(&overview, reports, contents)
	reconcileAutonomousApprovalStates(&overview, reports, contents)
	enrichAutonomousReviewContexts(&overview, contents)
	recountAutonomousDecisions(&overview)
	return overview, nil
}

func readAutonomousApprovalSource(root, rel string) (string, bool, error) {
	path, _, err := SafeResolve(root, rel)
	if err != nil {
		return "", false, err
	}
	content, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return "", false, nil
	}
	if err != nil {
		return "", false, err
	}
	return string(content), true, nil
}

func mergeAutonomousApprovals(existing, additions []AutonomousApproval) []AutonomousApproval {
	merged := append([]AutonomousApproval(nil), existing...)
	indexes := make(map[string]int, len(merged))
	for index, item := range merged {
		indexes[item.ID] = index
	}
	for _, addition := range additions {
		if index, ok := indexes[addition.ID]; ok {
			merged[index] = mergeAutonomousApproval(merged[index], addition)
			continue
		}
		indexes[addition.ID] = len(merged)
		merged = append(merged, addition)
	}
	return merged
}

func mergeAutonomousApproval(base, addition AutonomousApproval) AutonomousApproval {
	if base.Source == "" {
		base.Source = addition.Source
	}
	if base.DraftPath == "" {
		base.DraftPath = addition.DraftPath
	}
	if base.CandidateSource == "" {
		base.CandidateSource = addition.CandidateSource
	} else if addition.CandidateSource != "" && !strings.Contains(base.CandidateSource, addition.CandidateSource) {
		base.CandidateSource += "," + addition.CandidateSource
	}
	if base.Target == "" {
		base.Target = addition.Target
	}
	if base.Problem == "" {
		base.Problem = addition.Problem
	}
	if base.Status == "" {
		base.Status = addition.Status
	}
	if base.Risk == "" {
		base.Risk = addition.Risk
	}
	if base.Evidence == "" {
		base.Evidence = addition.Evidence
	}
	if base.NextStep == "" {
		base.NextStep = addition.NextStep
	}
	if base.ExpectedOutcome == "" {
		base.ExpectedOutcome = addition.ExpectedOutcome
	}
	if addition.ExecutionState != "" {
		base.ExecutionState = addition.ExecutionState
	}
	if base.ExecutionReport == nil && addition.ExecutionReport != nil {
		base.ExecutionReport = addition.ExecutionReport
	}
	if base.ExecutionSummary == "" {
		base.ExecutionSummary = addition.ExecutionSummary
	}
	if base.ExecutionError == "" {
		base.ExecutionError = addition.ExecutionError
	}
	if base.ExecutionUpdatedAt == nil && addition.ExecutionUpdatedAt != nil {
		base.ExecutionUpdatedAt = addition.ExecutionUpdatedAt
	}
	if base.ReviewStatus == "" {
		base.ReviewStatus = addition.ReviewStatus
	}
	if base.ReviewDecision == "" {
		base.ReviewDecision = addition.ReviewDecision
	}
	if base.ReviewConfidence == "" {
		base.ReviewConfidence = addition.ReviewConfidence
	}
	if base.ReviewReason == "" {
		base.ReviewReason = addition.ReviewReason
	}
	if base.ReviewModelNo == nil && addition.ReviewModelNo != nil {
		base.ReviewModelNo = addition.ReviewModelNo
	}
	if base.ReviewModel == "" {
		base.ReviewModel = addition.ReviewModel
	}
	if base.ReviewProvider == "" {
		base.ReviewProvider = addition.ReviewProvider
	}
	if base.ReviewReport == nil && addition.ReviewReport != nil {
		base.ReviewReport = addition.ReviewReport
	}
	if len(addition.ReviewReports) > 0 {
		base.ReviewReports = mergeAutonomousReportEntries(base.ReviewReports, addition.ReviewReports...)
	}
	if base.Decision == "" && addition.Decision != "" {
		base.Decision = addition.Decision
		base.State = addition.State
	}
	return base
}

func parseAutonomousTodoApprovals(content string) []AutonomousApproval {
	lines := strings.Split(strings.ReplaceAll(content, "\r\n", "\n"), "\n")
	items := make([]AutonomousApproval, 0)
	for _, line := range lines {
		marker, body, ok := autonomousTodoItem(line)
		if !ok || body == "" || ((marker == 'x' || marker == 'X') && !strings.Contains(body, "ga-admin-approval:")) {
			continue
		}
		title, parts := autonomousTodoTitle(body)
		if title == "" {
			continue
		}
		state, decision, status := autonomousTodoState(body)
		executionState := ""
		if strings.Contains(body, "ga-admin-approval:") {
			if marker == 'x' || marker == 'X' {
				executionState = autonomousExecutionReportMissing
			} else if decision == "approved" {
				if marker == 'x' || marker == 'X' {
					executionState = autonomousExecutionReportMissing
				} else {
					executionState = autonomousExecutionQueued
				}
			} else if decision == "rejected" {
				executionState = autonomousExecutionNotApplicable
			}
		}
		item := AutonomousApproval{
			ID:              autonomousApprovalID(title),
			Title:           title,
			Source:          autonomousTodoPath,
			DraftPath:       autonomousTodoPath,
			CandidateSource: autonomousCandidateSourceTodo,
			Status:          status,
			Risk:            autonomousTodoRisk(body),
			Evidence:        "来自 TODO.txt 的未完成条目，采用保守审批判定",
			NextStep:        autonomousTodoNextStep(parts),
			State:           state,
			Decision:        decision,
			ExecutionState:  executionState,
		}
		items = append(items, item)
	}
	return items
}

func autonomousTodoItem(line string) (rune, string, bool) {
	clean := strings.TrimSpace(line)
	if len(clean) < 4 || clean[0] != '[' || clean[2] != ']' {
		return 0, "", false
	}
	marker := rune(clean[1])
	if marker != ' ' && marker != 'x' && marker != 'X' {
		return 0, "", false
	}
	return marker, strings.TrimSpace(clean[3:]), true
}

func autonomousTodoTitle(body string) (string, []string) {
	parts := strings.FieldsFunc(body, func(r rune) bool { return r == '|' || r == '｜' })
	for index := range parts {
		parts[index] = strings.TrimSpace(parts[index])
	}
	parts = slicesWithoutEmptyStrings(parts)
	if len(parts) > 1 {
		return parts[1], parts
	}
	return strings.TrimSpace(body), parts
}

func slicesWithoutEmptyStrings(values []string) []string {
	filtered := make([]string, 0, len(values))
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			filtered = append(filtered, strings.TrimSpace(value))
		}
	}
	return filtered
}

func autonomousTodoState(body string) (string, string, string) {
	if containsAny(body, "用户已批准", "已批准", "已审批") {
		return "approved", "approved", "已批准"
	}
	if containsAny(body, "用户已拒绝", "已拒绝", "已驳回") {
		return "rejected", "rejected", "已拒绝"
	}
	if autonomousApprovalTextHasNoApproval(body) {
		return autonomousApprovalNotRequired, "", "无需审批"
	}
	if autonomousApprovalTextHasCompletion(body) {
		return "closed", "", "已完成"
	}
	return "pending", "", "待人工复核"
}

func autonomousTodoRisk(body string) string {
	if containsAny(body, "高风险", "删除", "源码", "写入", "修改", "覆盖") {
		return "需人工复核"
	}
	if containsAny(body, "低风险", "只读", "只读检查") {
		return "低风险，仍需确认"
	}
	return "未标注，需人工复核"
}

func autonomousTodoNextStep(parts []string) string {
	if len(parts) < 3 {
		return ""
	}
	return strings.Join(parts[2:], " | ")
}

func DecideAutonomousApproval(root, id, decision, note string) (AutonomousApprovalOverview, bool, error) {
	autonomousDecisionMu.Lock()
	defer autonomousDecisionMu.Unlock()
	decision = strings.ToLower(strings.TrimSpace(decision))
	if decision != "approved" && decision != "rejected" {
		return AutonomousApprovalOverview{}, false, errors.New("decision must be approved or rejected")
	}
	if len([]rune(note)) > 1000 {
		return AutonomousApprovalOverview{}, false, errors.New("decision note is too long")
	}
	overview, err := BuildAutonomousApprovals(root)
	if err != nil {
		return overview, false, err
	}
	item, err := approvalByID(overview.Items, strings.TrimSpace(id))
	if err != nil {
		return overview, false, err
	}
	if item.Decision != "" && item.Decision != decision {
		return overview, false, errors.New("approval already has a different decision")
	}
	queued := false
	if decision == "approved" {
		queued, err = queueApprovedAutonomousTask(root, item, note)
		if err != nil {
			return overview, false, err
		}
	}
	if item.Decision == "" {
		err = appendAutonomousDecision(root, autonomousDecision{ID: item.ID, Title: item.Title, Decision: decision, Note: strings.TrimSpace(note), DecidedAt: autonomousApprovalNow()})
		if err != nil {
			return overview, queued, err
		}
	}
	overview, err = BuildAutonomousApprovals(root)
	return overview, queued, err
}

func parseAutonomousApprovals(content string) []AutonomousApproval {
	lines := strings.Split(strings.ReplaceAll(content, "\r\n", "\n"), "\n")
	items := make([]AutonomousApproval, 0)
	var current *AutonomousApproval
	for _, line := range lines {
		if title, ok := autonomousApprovalHeading(line); ok {
			if current != nil {
				items = append(items, finishAutonomousApproval(*current))
			}
			current = &AutonomousApproval{ID: autonomousApprovalID(title), Title: title, CandidateSource: autonomousCandidateSourceDraft}
			continue
		}
		if current != nil {
			applyAutonomousApprovalField(current, line)
		}
	}
	if current != nil {
		items = append(items, finishAutonomousApproval(*current))
	}
	return items
}

func autonomousApprovalHeading(line string) (string, bool) {
	heading := strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(line), "##"))
	dot := strings.IndexByte(heading, '.')
	if dot < 1 {
		return "", false
	}
	for _, r := range heading[:dot] {
		if r < '0' || r > '9' {
			return "", false
		}
	}
	title := strings.TrimSpace(heading[dot+1:])
	return title, title != ""
}

func applyAutonomousApprovalField(item *AutonomousApproval, line string) {
	label, value, ok := autonomousMarkdownField(line)
	if !ok {
		return
	}
	switch label {
	case "来源":
		item.Source = value
	case "草案位置":
		item.DraftPath = value
	case "落地目标":
		item.Target = value
	case "要解决的问题", "解决的问题", "问题", "背景", "目的":
		item.Problem = value
	case "状态":
		item.Status = value
	case "风险":
		item.Risk = value
	case "核查证据":
		item.Evidence = value
	case "下一步":
		item.NextStep = value
	case "预期效果", "预期结果", "预期影响":
		item.ExpectedOutcome = value
	case "审核状态":
		item.ReviewStatus = value
	case "审核结论":
		item.ReviewDecision = value
	case "审核置信度":
		item.ReviewConfidence = value
	case "审核原因":
		item.ReviewReason = value
	case "审核模型编号":
		if modelNo, err := strconv.Atoi(value); err == nil && modelNo >= 0 {
			item.ReviewModelNo = &modelNo
		}
	case "审核模型":
		item.ReviewModel = value
	case "审核服务商":
		item.ReviewProvider = value
	}
}

func autonomousMarkdownField(line string) (string, string, bool) {
	clean := strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(line), "-"))
	clean = strings.Trim(clean, "* ")
	separator, separatorWidth := strings.Index(clean, "："), len("：")
	if ascii := strings.IndexByte(clean, ':'); ascii >= 0 && (separator < 0 || ascii < separator) {
		separator, separatorWidth = ascii, 1
	}
	if separator < 1 {
		return "", "", false
	}
	label := strings.Trim(strings.TrimSpace(clean[:separator]), "* ")
	value := strings.Trim(strings.TrimSpace(clean[separator+separatorWidth:]), "* ")
	return label, value, value != ""
}

func finishAutonomousApproval(item AutonomousApproval) AutonomousApproval {
	if autonomousApprovalNotRequiredForItem(item) {
		item.State = autonomousApprovalNotRequired
		return item
	}
	if autonomousApprovalStatusCompleted(item) {
		item.State = "closed"
		return item
	}
	stateText := strings.Join([]string{item.Status, item.NextStep, item.ReviewStatus, item.ReviewDecision, item.ReviewReason}, " ")
	switch {
	case containsAny(stateText, "已落地", "被替代", "废弃", "作废", "已归档"):
		item.State = "closed"
	case containsAny(stateText, "待批", "待审", "用户批准", "遗留缺口", "尚未"):
		item.State = "pending"
	default:
		item.State = "tracked"
	}
	return item
}

func autonomousApprovalTextHasNoApproval(values ...string) bool {
	text := strings.ToLower(strings.Join(values, " "))
	text = strings.ReplaceAll(text, "_", " ")
	return containsAny(text,
		"no approval required", "approval not required", "no human approval", "no human review", "no review needed", "not required", "not applicable",
		"无需审批", "无需人工审批", "不需要审批", "不需审批", "无须审批", "无需审核", "无需人工审核", "不需要审核", "不需审核", "无须审核",
		"无需人工复核", "不需要人工复核", "无需用户确认", "不需要用户确认", "无需批准", "不需要批准")
}

func autonomousApprovalTextHasCompletion(values ...string) bool {
	text := strings.ToLower(strings.Join(values, " "))
	text = strings.ReplaceAll(text, "complete_task", "")
	if containsAny(text, "未完成", "尚未完成", "未实施", "未落地", "未执行", "not completed", "not implemented", "unfinished", "pending") {
		return false
	}
	return containsAny(text,
		"已完成", "执行完成", "完成并通过", "已执行", "已落地", "已实施", "已归档", "已关闭", "已通过", "通过验证",
		"completed", "done", "finished", "implemented", "landed", "archived", "closed", "passed", "successful", "obsolete", "superseded")
}

func autonomousApprovalNotRequiredForItem(item AutonomousApproval) bool {
	if autonomousApprovalReviewRequiresDecision(item) {
		return false
	}
	return autonomousApprovalTextHasNoApproval(item.Title, item.Status, item.NextStep, item.ReviewStatus, item.ReviewDecision, item.ReviewReason)
}

func autonomousApprovalReviewRequiresDecision(item AutonomousApproval) bool {
	text := strings.ToLower(strings.Join([]string{item.ReviewStatus, item.ReviewDecision, item.ReviewReason}, " "))
	text = strings.ReplaceAll(text, "_", " ")
	return containsAny(text,
		"needs approval", "pending approval", "awaiting user approval", "approval gate", "approval evidence", "report is blocked", "model review unavailable",
		"需要审批", "待审批", "待批准", "等待用户审批", "审批门槛", "审批证据", "报告处于阻塞状态", "模型审核不可用")
}

func autonomousApprovalStatusCompleted(item AutonomousApproval) bool {
	return autonomousApprovalTextHasCompletion(item.Status)
}

func containsAny(value string, terms ...string) bool {
	for _, term := range terms {
		if strings.Contains(value, term) {
			return true
		}
	}
	return false
}

func loadAutonomousReportContents(root string, reports []Entry) map[string]string {
	contents := make(map[string]string, len(reports))
	for _, report := range reports {
		if report.Kind != "file" || strings.EqualFold(report.Name, "history.txt") {
			continue
		}
		content, ok := readListedAutonomousReport(root, report.Path)
		if ok {
			contents[report.Path] = content
		}
	}
	return contents
}

func readListedAutonomousReport(root, rel string) (string, bool) {
	clean := filepath.Clean(filepath.FromSlash(rel))
	if clean == "." || filepath.IsAbs(clean) || strings.HasPrefix(clean, ".."+string(filepath.Separator)) || clean == ".." {
		return "", false
	}
	path := filepath.Join(root, clean)
	info, err := os.Lstat(path)
	if err != nil || info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
		return "", false
	}
	data, err := os.ReadFile(path)
	return string(data), err == nil
}

func attachAutonomousReportCandidates(root string, overview *AutonomousApprovalOverview, reports []Entry, contents map[string]string) {
	for _, report := range reports {
		if report.Kind != "file" || strings.EqualFold(report.Name, "history.txt") {
			continue
		}
		content, ok := contents[report.Path]
		if !ok {
			continue
		}
		needsApproval, invalidEvidence, reason, confidence := autonomousReportReviewSignals(content)
		if !needsApproval {
			continue
		}
		title := autonomousReportCandidateTitle(report.Name, content)
		if title == "" {
			continue
		}
		matched := -1
		for index := range overview.Items {
			if autonomousApprovalMatchesReport(overview.Items[index], title, content) {
				matched = index
				break
			}
		}
		if matched >= 0 {
			mergeAutonomousReportReview(&overview.Items[matched], report, reason, confidence, invalidEvidence)
			continue
		}
		candidate := AutonomousApproval{
			ID:               autonomousApprovalID(title),
			Title:            title,
			Source:           report.Path,
			DraftPath:        report.Path,
			CandidateSource:  autonomousCandidateSourceReport,
			Status:           "report requires human approval",
			Risk:             "human review required",
			Evidence:         autonomousReportSummary(content),
			NextStep:         "Review the report evidence, then approve or reject explicitly",
			State:            "pending",
			ReviewStatus:     autonomousReviewNeedsApproval,
			ReviewDecision:   autonomousReviewNeedsApproval,
			ReviewConfidence: confidence,
			ReviewReason:     reason,
			ReviewReport:     entryPointer(report),
			ReviewReports:    []Entry{report},
		}
		overview.Items = mergeAutonomousApprovals(overview.Items, []AutonomousApproval{candidate})
	}
	if len(overview.Items) > 0 {
		overview.SourceExists = true
		if strings.TrimSpace(overview.SourcePath) == "" || overview.SourcePath == autonomousApprovalSource {
			if overview.SourcePath == "" || !fileExistsForApproval(root, overview.SourcePath) {
				overview.SourcePath = filepath.ToSlash(filepath.Join("temp", "autonomous_reports"))
			}
		}
	}
}

func fileExistsForApproval(root, rel string) bool {
	path, _, err := SafeResolve(root, rel)
	if err != nil {
		return false
	}
	_, err = os.Stat(path)
	return err == nil
}

func entryPointer(entry Entry) *Entry {
	copy := entry
	return &copy
}

func mergeAutonomousReportReview(item *AutonomousApproval, report Entry, reason, confidence string, invalidEvidence bool) {
	if item == nil {
		return
	}
	isNewer := item.ReviewReport == nil || report.ModTime.After(item.ReviewReport.ModTime)
	if isNewer {
		item.ReviewStatus = autonomousReviewNeedsApproval
		item.ReviewDecision = autonomousReviewNeedsApproval
		item.ReviewConfidence = confidence
		item.ReviewReason = reason
		item.ReviewReport = entryPointer(report)
	}
	item.ReviewReports = mergeAutonomousReportEntries(item.ReviewReports, report)
	if item.CandidateSource == "" {
		item.CandidateSource = autonomousCandidateSourceReport
	} else if !strings.Contains(item.CandidateSource, autonomousCandidateSourceReport) {
		item.CandidateSource += "," + autonomousCandidateSourceReport
	}
	if item.Source == "" {
		item.Source = report.Path
	}
	if item.DraftPath == "" {
		item.DraftPath = report.Path
	}
	// A newer report that explicitly says approval evidence is missing invalidates
	// an older, unverified approval. A fresh user decision after that report is
	// respected on the next build.
	if invalidEvidence && (item.Decision == "" || item.DecidedAt.IsZero() || report.ModTime.After(item.DecidedAt)) {
		item.Decision = ""
		item.DecidedAt = time.Time{}
		item.State = "pending"
		item.ExecutionState = ""
		item.ExecutionReport = nil
		item.ExecutionSummary = ""
		item.ExecutionError = ""
		item.ExecutionUpdatedAt = nil
	}
	if item.Decision == "" {
		item.State = "pending"
	}
}

func mergeAutonomousReportEntries(existing []Entry, additions ...Entry) []Entry {
	merged := append([]Entry(nil), existing...)
	seen := make(map[string]bool, len(merged)+len(additions))
	for _, entry := range merged {
		seen[entry.Path] = true
	}
	for _, entry := range additions {
		if entry.Path == "" || seen[entry.Path] {
			continue
		}
		seen[entry.Path] = true
		merged = append(merged, entry)
	}
	sort.SliceStable(merged, func(i, j int) bool { return merged[i].ModTime.After(merged[j].ModTime) })
	if len(merged) > 12 {
		merged = merged[:12]
	}
	return merged
}

func autonomousReportReviewSignals(content string) (needsApproval, invalidEvidence bool, reason, confidence string) {
	lower := strings.ToLower(content)
	approvalContext := containsAny(lower, "approval", "todo", "待审批", "待批准", "用户批准", "用户授权", "审批门控", "审批证据", "原实施")
	pendingLanguage := containsAny(lower, "pending approval", "awaiting user approval", "must remain pending", "continue waiting", "待审", "待批准", "待用户批准", "待用户决策", "保留待批准", "保留为 [ ]", "保留为[ ]", "未经明确批准")
	blocked := containsAny(lower, "blocked", "approval blocked", "审批阻塞", "阻塞复核")
	missingEvidence := containsAny(lower,
		"approval evidence is missing", "approval evidence cannot be verified", "approval evidence unverifiable", "not treated as approved", "must remain pending", "审批证据缺失", "审批证据不可核验", "不应视为已批准", "继续待审", "仍需等待用户批准")
	notImplemented := containsAny(lower, "not implemented", "not changed", "未实施", "未修改源码", "未改源码", "未执行")
	approvalContext = approvalContext || containsAny(lower, "审批", "待审批", "待批准", "用户批准", "用户授权", "审批门控", "审批证据", "原实施")
	pendingLanguage = pendingLanguage || containsAny(lower, "待审", "待审批", "待批准", "待用户批准", "待用户决策", "保留待批准", "保留为 [ ]", "未经明确批准")
	blocked = blocked || containsAny(lower, "阻塞", "审批阻塞", "阻塞复核")
	missingEvidence = missingEvidence || containsAny(lower, "审批证据缺失", "审批证据不可核验", "不应视为已批准", "继续待审", "仍需等待用户批准")
	notImplemented = notImplemented || containsAny(lower, "未实施", "未修改源码", "未改源码", "未执行")
	uncheckedTodo := strings.Contains(content, "[ ]") && pendingLanguage
	blockedApproval := blocked && approvalContext && (pendingLanguage || missingEvidence || notImplemented || strings.Contains(lower, "memory/"))
	if missingEvidence || pendingLanguage || blockedApproval || uncheckedTodo {
		parts := make([]string, 0, 3)
		if blocked {
			parts = append(parts, "report is blocked")
		}
		if missingEvidence {
			parts = append(parts, "approval evidence is missing or unverifiable")
		}
		if notImplemented || uncheckedTodo {
			parts = append(parts, "the proposed source change is not confirmed as implemented")
		}
		if len(parts) == 0 {
			parts = append(parts, "report contains an explicit approval gate")
		}
		return true, missingEvidence || blockedApproval && notImplemented, strings.Join(parts, "; "), "high"
	}
	return false, false, "", ""
}

var autonomousReviewOptionHeading = regexp.MustCompile(`^(?:#{2,6}\s*)?(?:\[(?:候选|方案|选项|建议|路径|选择)\s*([A-Za-z0-9一二三四五六七八九十]+)[^\]]*\]\s*(.+)|(?:建议|方案|选项|候选|路径|选择)\s*([A-Za-z0-9一二三四五六七八九十]+)\s*(?:[:：-]\s*|\s+)(.+))$`)

func enrichAutonomousReviewContexts(overview *AutonomousApprovalOverview, contents map[string]string) {
	if overview == nil {
		return
	}
	for index := range overview.Items {
		item := &overview.Items[index]
		content := autonomousReviewContent(*item, contents)
		options := extractAutonomousReviewOptions(content)
		tags := autonomousReviewTags(*item, content, options)
		item.ReviewOptions = options
		item.ReviewTags = tags
		item.ReviewFocus = autonomousReviewFocus(tags)
	}
}

func autonomousReviewContent(item AutonomousApproval, contents map[string]string) string {
	if item.ReviewReport != nil {
		if content := contents[item.ReviewReport.Path]; content != "" {
			return content
		}
	}
	for _, report := range item.ReviewReports {
		if content := contents[report.Path]; content != "" {
			return content
		}
	}
	return ""
}

func extractAutonomousReviewOptions(content string) []AutonomousReviewOption {
	lines := strings.Split(strings.ReplaceAll(content, "\r\n", "\n"), "\n")
	options := make([]AutonomousReviewOption, 0, 4)
	seen := map[string]bool{}
	for index, line := range lines {
		key, title, recommended, ok := parseAutonomousReviewOptionHeading(line)
		if !ok {
			continue
		}
		identity := strings.ToLower(key + "|" + title)
		if seen[identity] {
			continue
		}
		seen[identity] = true
		option := AutonomousReviewOption{Key: key, Title: title, Recommended: recommended}
		for next := index + 1; next < len(lines); next++ {
			if _, _, _, nextOption := parseAutonomousReviewOptionHeading(lines[next]); nextOption {
				break
			}
			candidate := cleanAutonomousReviewText(lines[next])
			if candidate == "" || strings.HasPrefix(candidate, "#") || strings.HasPrefix(candidate, "---") || strings.HasPrefix(candidate, "|") {
				continue
			}
			option.Summary = candidate
			break
		}
		options = append(options, option)
		if len(options) >= 5 {
			break
		}
	}
	return options
}

func parseAutonomousReviewOptionHeading(line string) (string, string, bool, bool) {
	matches := autonomousReviewOptionHeading.FindStringSubmatch(strings.TrimSpace(line))
	if len(matches) == 0 {
		return "", "", false, false
	}
	key, title := matches[1], matches[2]
	if key == "" {
		key, title = matches[3], matches[4]
	}
	title = cleanAutonomousReviewText(title)
	if key == "" || title == "" {
		return "", "", false, false
	}
	return strings.ToUpper(key), title, strings.Contains(strings.ToLower(matches[0]), "推荐") || strings.Contains(strings.ToLower(matches[0]), "recommend"), true
}

func cleanAutonomousReviewText(value string) string {
	value = strings.Join(strings.Fields(value), " ")
	value = strings.Trim(value, " -*_`")
	value = strings.ReplaceAll(value, "**", "")
	value = strings.ReplaceAll(value, "`", "")
	if len([]rune(value)) > 180 {
		value = string([]rune(value)[:180])
	}
	return strings.TrimSpace(value)
}

func autonomousReviewTags(item AutonomousApproval, content string, options []AutonomousReviewOption) []string {
	metadata := strings.ToLower(strings.Join([]string{item.Title, item.Target, item.Status, item.Risk, item.Evidence, item.NextStep, item.ReviewReason}, " "))
	allText := strings.ToLower(metadata + " " + content)
	tags := make([]string, 0, 5)
	if len(options) > 0 {
		tags = append(tags, "choice")
	}
	if containsAny(allText, "blocked", "approval blocked", "审批阻塞", "阻塞复核", "审批证据缺失", "审批证据不可核验", "无法核验", "仍需等待用户批准") {
		tags = append(tags, "blocked")
	}
	if containsAny(metadata, "源码", "代码", "开发", "实现", "修复", "补全", "新增", "修改", "删除", "写入", "部署", "工具", ".py", ".go", ".js", ".ts") {
		tags = append(tags, "file_change")
	}
	if containsAny(metadata, "配置变更", "配置切换", "切换", "调度", "参数", "环境变量", "上游", ".json", ".yaml", ".yml", ".toml", ".ini") {
		tags = append(tags, "config_change")
	}
	if containsAny(metadata, "验证", "实测", "验收", "诊断", "检查", "benchmark", "health", "健康") {
		tags = append(tags, "verification")
	}
	if containsAny(metadata, "sop", "文档", "记忆", "memory", "规范", "手册", ".md", ".txt") {
		tags = append(tags, "documentation")
	}
	if containsAny(metadata, "观察", "只读", "探测", "监测") && !containsAny(metadata, "修改", "写入", "部署") {
		tags = append(tags, "observation")
	}
	if autonomousApprovalTextHasCompletion(item.Status) {
		tags = append(tags, "completed")
	}
	if len(tags) == 0 {
		tags = append(tags, "manual")
	}
	return uniqueNonEmptyStrings(tags)
}

func autonomousReviewFocus(tags []string) string {
	if containsAny(strings.Join(tags, " "), "choice") {
		return "报告给出了多个可选方案，批准前需要确认采用哪一个方案。"
	}
	if containsAny(strings.Join(tags, " "), "blocked") {
		return "报告存在阻塞或审批证据不足，需先确认是否继续处理。"
	}
	if containsAny(strings.Join(tags, " "), "file_change") && containsAny(strings.Join(tags, " "), "config_change") {
		return "这项建议同时涉及文件或代码与运行配置变更，需确认变更范围。"
	}
	if containsAny(strings.Join(tags, " "), "file_change") {
		return "这项建议可能修改文件或代码，需确认是否允许落地。"
	}
	if containsAny(strings.Join(tags, " "), "config_change") {
		return "这项建议会改变运行配置或调度参数，需确认是否切换。"
	}
	if containsAny(strings.Join(tags, " "), "verification") {
		return "这项建议主要用于验证、实测或健康检查，需确认是否继续执行。"
	}
	if containsAny(strings.Join(tags, " "), "documentation") {
		return "这项建议主要完善 SOP、记忆或项目文档，需确认是否写入。"
	}
	if containsAny(strings.Join(tags, " "), "observation") {
		return "这项建议以只读观察或环境探测为主，不应直接修改文件。"
	}
	return "请确认是否允许自主服务按报告中的下一步继续处理。"
}

func autonomousReportCandidateTitle(name, content string) string {
	for _, line := range strings.Split(strings.ReplaceAll(content, "\r\n", "\n"), "\n") {
		clean := strings.TrimSpace(line)
		if !strings.HasPrefix(clean, "#") {
			continue
		}
		title := strings.TrimSpace(strings.TrimLeft(clean, "#"))
		if title == "" || containsAny(strings.ToLower(title), "execution report", "audit report", "history") {
			continue
		}
		return title
	}
	base := strings.TrimSpace(strings.TrimSuffix(name, filepath.Ext(name)))
	if underscore := strings.IndexByte(base, '_'); underscore > 1 && strings.HasPrefix(strings.ToLower(base[:underscore]), "r") {
		allDigits := true
		for _, r := range base[1:underscore] {
			if !unicode.IsDigit(r) {
				allDigits = false
				break
			}
		}
		if allDigits {
			base = strings.TrimSpace(base[underscore+1:])
		}
	}
	return base
}

func autonomousApprovalMatchesReport(item AutonomousApproval, reportTitle, content string) bool {
	if autonomousApprovalTitlesMatch(item.Title, reportTitle) {
		return true
	}
	normalizedContent := normalizeAutonomousMatch(content)
	for _, variant := range autonomousTitleVariants(item.Title) {
		if len([]rune(variant)) >= 10 && strings.Contains(normalizedContent, variant) {
			return true
		}
	}
	return false
}

func autonomousApprovalTitlesMatch(left, right string) bool {
	leftKey := autonomousApprovalTaskKey(left)
	rightKey := autonomousApprovalTaskKey(right)
	if leftKey == "" || rightKey == "" {
		return false
	}
	if strings.Contains(leftKey, rightKey) || strings.Contains(rightKey, leftKey) {
		return true
	}
	common := 0
	leftRunes := []rune(leftKey)
	rightRunes := []rune(rightKey)
	for common < len(leftRunes) && common < len(rightRunes) && leftRunes[common] == rightRunes[common] {
		common++
	}
	return common >= 12 && (strings.Contains(leftKey, "completetask") || strings.Contains(rightKey, "completetask"))
}

func autonomousApprovalTaskKey(title string) string {
	clean := strings.TrimSpace(title)
	for _, separator := range []string{"：", ":", "（", "(", "—", "- 审批", "- approval"} {
		if index := strings.Index(clean, separator); index > 0 {
			clean = clean[:index]
		}
	}
	return normalizeAutonomousMatch(clean)
}

func recountAutonomousDecisions(overview *AutonomousApprovalOverview) {
	if overview == nil {
		return
	}
	overview.Pending = 0
	overview.Approved = 0
	overview.Rejected = 0
	for _, item := range overview.Items {
		countAutonomousDecision(overview, item.State)
	}
}

func autonomousApprovalID(title string) string {
	sum := sha256.Sum256([]byte(strings.ToLower(strings.TrimSpace(title))))
	return fmt.Sprintf("draft-%x", sum[:6])
}

func attachAutonomousExecution(overview *AutonomousApprovalOverview, reports []Entry, contents map[string]string) {
	for index := range overview.Items {
		item := &overview.Items[index]
		switch item.Decision {
		case "approved":
			if item.ExecutionState == "" {
				item.ExecutionState = autonomousExecutionQueued
			}
		case "rejected":
			item.ExecutionState = autonomousExecutionNotApplicable
			continue
		default:
			continue
		}
		if report, content, ok := matchAutonomousExecutionReport(reports, contents, *item); ok {
			item.ExecutionReport = report
			reportTime := report.ModTime
			item.ExecutionUpdatedAt = &reportTime
			item.ExecutionSummary = autonomousReportSummary(content)
			if autonomousReportFailed(content) {
				item.ExecutionState = autonomousExecutionFailed
				item.ExecutionError = item.ExecutionSummary
			} else {
				item.ExecutionState = autonomousExecutionCompleted
				item.ExecutionError = ""
			}
		}
	}
}

func reconcileAutonomousApprovalStates(overview *AutonomousApprovalOverview, reports []Entry, contents map[string]string) {
	if overview == nil {
		return
	}
	for index := range overview.Items {
		item := &overview.Items[index]
		if item.State != "pending" || item.Decision != "" {
			continue
		}
		if item.ReviewReport != nil {
			continue
		}
		if autonomousApprovalNotRequiredForItem(*item) {
			item.State = autonomousApprovalNotRequired
			item.ExecutionState = autonomousExecutionNotApplicable
			continue
		}
		if item.ExecutionState == autonomousExecutionCompleted {
			item.State = "closed"
			continue
		}
		if report, content, ok := matchAutonomousExecutionReport(reports, contents, *item); ok && autonomousReportConfirmsCompletion(content) && reportIsCurrentForApproval(*item, report) {
			item.State = "closed"
			item.ExecutionState = autonomousExecutionCompleted
			item.ExecutionReport = report
			item.ExecutionSummary = autonomousReportSummary(content)
			item.ExecutionError = ""
			reportTime := report.ModTime
			item.ExecutionUpdatedAt = &reportTime
			continue
		}
		if autonomousApprovalStatusCompleted(*item) && !autonomousApprovalReviewRequiresDecision(*item) {
			item.State = "closed"
		}
	}
}

func reportIsCurrentForApproval(item AutonomousApproval, report *Entry) bool {
	return report != nil && (item.ReviewReport == nil || !report.ModTime.Before(item.ReviewReport.ModTime))
}

func autonomousReportConfirmsCompletion(content string) bool {
	if needsApproval, _, _, _ := autonomousReportReviewSignals(content); needsApproval {
		return false
	}
	lower := strings.ToLower(content)
	return containsAny(lower,
		"verdict: pass", "verdict：pass", "result: success", "result：success", "outcome: success", "outcome：success",
		"执行结果：成功", "执行结果:成功", "执行结果：完成", "执行结果:完成", "结论：已完成", "结论:已完成", "已完成并通过验证", "通过验证",
		"completed successfully", "successfully completed")
}

func matchAutonomousExecutionReport(reports []Entry, contents map[string]string, item AutonomousApproval) (*Entry, string, bool) {
	marker := "ga-admin-approval:" + item.ID
	variants := autonomousTitleVariants(item.Title)
	var best *Entry
	var bestContent string
	bestScore := 0
	for _, report := range reports {
		if report.Kind != "file" || strings.EqualFold(report.Name, "history.txt") {
			continue
		}
		content, ok := contents[report.Path]
		if !ok {
			continue
		}
		if needsApproval, _, _, _ := autonomousReportReviewSignals(content); needsApproval {
			continue
		}
		if !item.DecidedAt.IsZero() && report.ModTime.Before(item.DecidedAt) && !strings.Contains(content, marker) && !strings.Contains(strings.ToLower(report.Name), strings.ToLower(item.ID)) {
			continue
		}
		score := 0
		name := normalizeAutonomousMatch(report.Name)
		if strings.Contains(content, marker) {
			score = 100
		} else if strings.Contains(strings.ToLower(report.Name), strings.ToLower(item.ID)) {
			score = 90
		} else {
			for _, variant := range variants {
				if variant != "" && strings.Contains(name, variant) {
					score = maxInt(score, 80)
				}
				if variant != "" && autonomousReportHasTaskTitle(content, variant) {
					score = maxInt(score, 70)
				}
			}
		}
		if score > bestScore || (score == bestScore && best != nil && report.ModTime.After(best.ModTime)) {
			candidate := report
			best = &candidate
			bestContent = content
			bestScore = score
		}
	}
	return best, bestContent, best != nil && bestScore > 0
}

func autonomousReportHasTaskTitle(content, variant string) bool {
	for _, line := range strings.Split(strings.ReplaceAll(content, "\r\n", "\n"), "\n") {
		clean := strings.TrimSpace(line)
		lower := strings.ToLower(clean)
		if clean == "" || strings.Contains(lower, "todo") || strings.Contains(clean, "[ ]") || strings.Contains(clean, "[x]") || strings.Contains(clean, "[X]") {
			continue
		}
		if strings.HasPrefix(clean, "#") || strings.Contains(clean, "任务") || strings.Contains(lower, "task") || strings.Contains(lower, "target") {
			if strings.Contains(normalizeAutonomousMatch(clean), variant) {
				return true
			}
		}
	}
	return false
}

func autonomousTitleVariants(title string) []string {
	clean := strings.TrimSpace(title)
	variants := []string{normalizeAutonomousMatch(clean)}
	if open := strings.IndexAny(clean, "（("); open > 0 {
		variants = append(variants, normalizeAutonomousMatch(clean[:open]))
	}
	clean = strings.ReplaceAll(clean, "待审批后执行", "")
	variants = append(variants, normalizeAutonomousMatch(clean))
	return uniqueNonEmptyStrings(variants)
}

func normalizeAutonomousMatch(value string) string {
	var normalized strings.Builder
	for _, r := range strings.ToLower(value) {
		if unicode.IsLetter(r) || unicode.IsNumber(r) {
			normalized.WriteRune(r)
		}
	}
	return normalized.String()
}

func uniqueNonEmptyStrings(values []string) []string {
	seen := map[string]bool{}
	result := make([]string, 0, len(values))
	for _, value := range values {
		if value != "" && !seen[value] {
			seen[value] = true
			result = append(result, value)
		}
	}
	return result
}

func maxInt(left, right int) int {
	if left > right {
		return left
	}
	return right
}

func autonomousReportFailed(content string) bool {
	upper := strings.ToUpper(strings.ReplaceAll(content, " ", ""))
	for _, marker := range []string{"VERDICT:FAIL", "VERDICT：FAIL", "状态：失败", "执行结果：失败", "执行结果:失败", "RESULT:FAILED", "OUTCOME:FAILED"} {
		if strings.Contains(upper, marker) {
			return true
		}
	}
	return false
}

func autonomousReportSummary(content string) string {
	lines := strings.Split(strings.ReplaceAll(content, "\r\n", "\n"), "\n")
	for _, line := range lines {
		clean := strings.TrimSpace(strings.TrimLeft(strings.TrimSpace(line), "-*_# "))
		if clean == "" || strings.HasPrefix(clean, "```") || strings.HasPrefix(clean, "|") {
			continue
		}
		lower := strings.ToLower(clean)
		if strings.Contains(clean, "结论") || strings.Contains(clean, "结果") || strings.Contains(clean, "状态") || strings.Contains(lower, "verdict") || strings.Contains(lower, "summary") || strings.Contains(lower, "result") {
			return truncateAutonomousSummary(clean, 320)
		}
	}
	for _, line := range lines {
		clean := strings.TrimSpace(strings.TrimLeft(strings.TrimSpace(line), "-*_# "))
		if clean != "" && !strings.HasPrefix(clean, "```") && !strings.HasPrefix(clean, "|") {
			return truncateAutonomousSummary(clean, 320)
		}
	}
	return ""
}

func truncateAutonomousSummary(value string, limit int) string {
	value = strings.Join(strings.Fields(value), " ")
	runes := []rune(value)
	if len(runes) <= limit {
		return value
	}
	return string(runes[:limit]) + "…"
}
