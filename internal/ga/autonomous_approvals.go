package ga

import (
	"crypto/sha256"
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"
)

const autonomousApprovalSchemaVersion = 1

const (
	autonomousApprovalSource = "temp/pending_drafts.md"
	autonomousDecisionPath   = "temp/autonomous_approval_decisions.json"
	autonomousReviewPath     = "temp/autonomous_approval_reviews.json"
	autonomousTodoPath       = "temp/TODO.txt"
)

type AutonomousApproval struct {
	ID                string    `json:"id"`
	Title             string    `json:"title"`
	Source            string    `json:"source,omitempty"`
	DraftPath         string    `json:"draft_path,omitempty"`
	Target            string    `json:"target,omitempty"`
	Status            string    `json:"status,omitempty"`
	Risk              string    `json:"risk,omitempty"`
	Evidence          string    `json:"evidence,omitempty"`
	NextStep          string    `json:"next_step,omitempty"`
	ExpectedOutcome   string    `json:"expected_outcome,omitempty"`
	CandidateSource   string    `json:"candidate_source,omitempty"`
	ReviewStatus      string    `json:"review_status,omitempty"`
	ReviewDecision    string    `json:"review_decision,omitempty"`
	ReviewConfidence  string    `json:"review_confidence,omitempty"`
	ReviewReason      string    `json:"review_reason,omitempty"`
	ReviewModelNo     int       `json:"review_model_no,omitempty"`
	ReviewModel       string    `json:"review_model,omitempty"`
	ReviewProvider    string    `json:"review_provider,omitempty"`
	ReviewAttempts    int       `json:"review_attempts,omitempty"`
	ReviewNextRetryAt time.Time `json:"review_next_retry_at,omitempty"`
	State             string    `json:"state"`
	Decision          string    `json:"decision,omitempty"`
	Note              string    `json:"note,omitempty"`
	DecidedAt         time.Time `json:"decided_at,omitempty"`
}

type AutonomousReviewRecord struct {
	ID               string    `json:"id"`
	Fingerprint      string    `json:"fingerprint,omitempty"`
	ReviewStatus     string    `json:"review_status,omitempty"`
	ReviewDecision   string    `json:"review_decision,omitempty"`
	ReviewConfidence string    `json:"review_confidence,omitempty"`
	ReviewReason     string    `json:"review_reason,omitempty"`
	ReviewModelNo    int       `json:"review_model_no,omitempty"`
	ReviewModel      string    `json:"review_model,omitempty"`
	ReviewProvider   string    `json:"review_provider,omitempty"`
	Attempts         int       `json:"attempts,omitempty"`
	NextRetryAt      time.Time `json:"next_retry_at,omitempty"`
	UpdatedAt        time.Time `json:"updated_at"`
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
	overview := AutonomousApprovalOverview{SchemaVersion: autonomousApprovalSchemaVersion, SourcePath: autonomousApprovalSource, GeneratedAt: autonomousApprovalNow()}
	path, _, err := SafeResolve(root, autonomousApprovalSource)
	if err != nil {
		return overview, err
	}
	content, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return overview, nil
	}
	if err != nil {
		return overview, err
	}
	overview.SourceExists = true
	overview.Items = parseAutonomousApprovals(string(content))
	decisions, err := loadAutonomousDecisions(root)
	if err != nil {
		return overview, err
	}
	applyAutonomousDecisions(&overview, decisions)
	reviews, err := loadAutonomousReviews(root)
	if err != nil {
		return overview, err
	}
	applyAutonomousReviews(&overview, reviews)
	return overview, nil
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
	switch strings.ToLower(label) {
	case "来源":
		item.Source = value
	case "候选来源", "candidate_source":
		item.CandidateSource = value
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
	case "预期效果", "预期结果", "完成后效果", "完成后会怎样", "预期收益", "expected outcome", "expected result", "expected_effect":
		item.ExpectedOutcome = value
	case "审核状态", "review_status":
		item.ReviewStatus = value
	case "审核结论", "review_decision":
		item.ReviewDecision = value
	case "审核置信度", "review_confidence":
		item.ReviewConfidence = value
	case "审核原因", "review_reason":
		item.ReviewReason = value
	case "审核模型", "review_model":
		item.ReviewModel = value
	case "审核服务商", "review_provider":
		item.ReviewProvider = value
	case "审核模型编号", "review_model_no":
		if number, err := strconv.Atoi(value); err == nil && number > 0 {
			item.ReviewModelNo = number
		}
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
