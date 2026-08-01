package ga

import (
	"crypto/sha256"
	"errors"
	"fmt"
	"os"
	"strings"
	"sync"
	"time"
)

const autonomousApprovalSchemaVersion = 1

const (
	autonomousApprovalSource = "temp/pending_drafts.md"
	autonomousDecisionPath   = "temp/autonomous_approval_decisions.json"
	autonomousTodoPath       = "temp/TODO.txt"
)

type AutonomousApproval struct {
	ID        string    `json:"id"`
	Title     string    `json:"title"`
	Source    string    `json:"source,omitempty"`
	DraftPath string    `json:"draft_path,omitempty"`
	Target    string    `json:"target,omitempty"`
	Status    string    `json:"status,omitempty"`
	Risk      string    `json:"risk,omitempty"`
	Evidence  string    `json:"evidence,omitempty"`
	NextStep  string    `json:"next_step,omitempty"`
	State     string    `json:"state"`
	Decision  string    `json:"decision,omitempty"`
	Note      string    `json:"note,omitempty"`
	DecidedAt time.Time `json:"decided_at,omitempty"`
}

type AutonomousApprovalOverview struct {
	SchemaVersion int                  `json:"schema_version"`
	SourcePath    string               `json:"source_path"`
	SourceExists  bool                 `json:"source_exists"`
	Items         []AutonomousApproval `json:"items"`
	Pending       int                  `json:"pending"`
	Approved      int                  `json:"approved"`
	Rejected      int                  `json:"rejected"`
	GeneratedAt   time.Time            `json:"generated_at"`
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
	if base.Target == "" {
		base.Target = addition.Target
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
		if !ok || body == "" || marker == 'x' || marker == 'X' {
			continue
		}
		title, parts := autonomousTodoTitle(body)
		if title == "" {
			continue
		}
		state, decision, status := autonomousTodoState(body)
		item := AutonomousApproval{
			ID:        autonomousApprovalID(title),
			Title:     title,
			Source:    autonomousTodoPath,
			DraftPath: autonomousTodoPath,
			Status:    status,
			Risk:      autonomousTodoRisk(body),
			Evidence:  "来自 TODO.txt 的未完成条目，采用保守审批判定",
			NextStep:  autonomousTodoNextStep(parts),
			State:     state,
			Decision:  decision,
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
			current = &AutonomousApproval{ID: autonomousApprovalID(title), Title: title}
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
	case "状态":
		item.Status = value
	case "风险":
		item.Risk = value
	case "核查证据":
		item.Evidence = value
	case "下一步":
		item.NextStep = value
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
	stateText := item.Status + " " + item.NextStep
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

func containsAny(value string, terms ...string) bool {
	for _, term := range terms {
		if strings.Contains(value, term) {
			return true
		}
	}
	return false
}

func autonomousApprovalID(title string) string {
	sum := sha256.Sum256([]byte(strings.ToLower(strings.TrimSpace(title))))
	return fmt.Sprintf("draft-%x", sum[:6])
}
