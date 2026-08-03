package ga

import (
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

type autonomousReviewLedger struct {
	SchemaVersion int                      `json:"schema_version"`
	Reviews       []AutonomousReviewRecord `json:"reviews"`
}

var autonomousReviewMu sync.Mutex

func loadAutonomousDecisions(root string) ([]autonomousDecision, error) {
	path, _, err := SafeResolve(root, autonomousDecisionPath)
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
	var ledger autonomousDecisionLedger
	if err := json.Unmarshal(b, &ledger); err != nil {
		return nil, fmt.Errorf("read autonomous approval decisions: %w", err)
	}
	if ledger.SchemaVersion > autonomousApprovalSchemaVersion {
		return nil, fmt.Errorf("unsupported autonomous approval schema_version %d", ledger.SchemaVersion)
	}
	return ledger.Decisions, nil
}

func applyAutonomousDecisions(overview *AutonomousApprovalOverview, decisions []autonomousDecision) {
	latest := map[string]autonomousDecision{}
	for _, decision := range decisions {
		latest[decision.ID] = decision
	}
	for i := range overview.Items {
		if decision, ok := latest[overview.Items[i].ID]; ok {
			overview.Items[i].Decision = decision.Decision
			overview.Items[i].Note = decision.Note
			overview.Items[i].DecidedAt = decision.DecidedAt
			overview.Items[i].State = decision.Decision
		}
		countAutonomousDecision(overview, overview.Items[i].State)
	}
}

func countAutonomousDecision(overview *AutonomousApprovalOverview, state string) {
	switch state {
	case "pending":
		overview.Pending++
	case "approved":
		overview.Approved++
	case "rejected":
		overview.Rejected++
	}
}

func approvalByID(items []AutonomousApproval, id string) (AutonomousApproval, error) {
	for _, item := range items {
		if item.ID == id {
			return item, nil
		}
	}
	return AutonomousApproval{}, errors.New("approval item not found")
}

func appendAutonomousDecision(root string, decision autonomousDecision) error {
	decisions, err := loadAutonomousDecisions(root)
	if err != nil {
		return err
	}
	ledger := autonomousDecisionLedger{SchemaVersion: autonomousApprovalSchemaVersion, Decisions: append(decisions, decision)}
	b, err := json.MarshalIndent(ledger, "", "  ")
	if err != nil {
		return err
	}
	path, _, err := SafeResolve(root, autonomousDecisionPath)
	if err != nil {
		return err
	}
	if err := ensureWriteParentWithinRoot(root, path); err != nil {
		return err
	}
	return writeFileAtomic(path, append(b, '\n'), 0644)
}

func AutonomousApprovalFingerprint(item AutonomousApproval) string {
	value := strings.Join([]string{item.Title, item.Source, item.DraftPath, item.Target, item.Status, item.Risk, item.Evidence, item.NextStep, item.ExpectedOutcome}, "\n")
	sum := sha256.Sum256([]byte(value))
	return fmt.Sprintf("%x", sum[:])
}

func loadAutonomousReviews(root string) ([]AutonomousReviewRecord, error) {
	autonomousReviewMu.Lock()
	defer autonomousReviewMu.Unlock()
	return loadAutonomousReviewsUnlocked(root)
}

func loadAutonomousReviewsUnlocked(root string) ([]AutonomousReviewRecord, error) {
	path, _, err := SafeResolve(root, autonomousReviewPath)
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
	var ledger autonomousReviewLedger
	if err := json.Unmarshal(b, &ledger); err != nil {
		return nil, fmt.Errorf("read autonomous approval reviews: %w", err)
	}
	if ledger.SchemaVersion > autonomousApprovalSchemaVersion {
		return nil, fmt.Errorf("unsupported autonomous review schema_version %d", ledger.SchemaVersion)
	}
	return ledger.Reviews, nil
}

func LoadAutonomousReviews(root string) ([]AutonomousReviewRecord, error) {
	return loadAutonomousReviews(root)
}

func SaveAutonomousReview(root string, record AutonomousReviewRecord) error {
	autonomousReviewMu.Lock()
	defer autonomousReviewMu.Unlock()
	reviews, err := loadAutonomousReviewsUnlocked(root)
	if err != nil {
		return err
	}
	replaced := false
	for index := range reviews {
		if reviews[index].ID == record.ID {
			reviews[index] = record
			replaced = true
			break
		}
	}
	if !replaced {
		reviews = append(reviews, record)
	}
	b, err := json.MarshalIndent(autonomousReviewLedger{SchemaVersion: autonomousApprovalSchemaVersion, Reviews: reviews}, "", "  ")
	if err != nil {
		return err
	}
	path, _, err := SafeResolve(root, autonomousReviewPath)
	if err != nil {
		return err
	}
	if err := ensureWriteParentWithinRoot(root, path); err != nil {
		return err
	}
	return writeFileAtomic(path, append(b, '\n'), 0644)
}

func applyAutonomousReviews(overview *AutonomousApprovalOverview, reviews []AutonomousReviewRecord) {
	byID := make(map[string]AutonomousReviewRecord, len(reviews))
	for _, review := range reviews {
		byID[review.ID] = review
	}
	for index := range overview.Items {
		review, ok := byID[overview.Items[index].ID]
		if !ok || (review.Fingerprint != "" && review.Fingerprint != AutonomousApprovalFingerprint(overview.Items[index])) {
			continue
		}
		item := &overview.Items[index]
		item.ReviewStatus = review.ReviewStatus
		item.ReviewDecision = review.ReviewDecision
		item.ReviewConfidence = review.ReviewConfidence
		item.ReviewReason = review.ReviewReason
		item.ReviewModelNo = review.ReviewModelNo
		item.ReviewModel = review.ReviewModel
		item.ReviewProvider = review.ReviewProvider
		item.ReviewAttempts = review.Attempts
		item.ReviewNextRetryAt = review.NextRetryAt
	}
}

func queueApprovedAutonomousTask(root string, item AutonomousApproval, note string) (bool, error) {
	path, _, err := SafeResolve(root, autonomousTodoPath)
	if err != nil {
		return false, err
	}
	b, err := os.ReadFile(path)
	if err != nil && !os.IsNotExist(err) {
		return false, err
	}
	marker := "ga-admin-approval:" + item.ID
	if strings.Contains(string(b), marker) {
		return false, nil
	}
	line := fmt.Sprintf("[ ] 用户已批准 | %s | 按 %s 中的下一步执行并生成报告", item.Title, filepath.ToSlash(autonomousApprovalSource))
	if reply := autonomousApprovalReply(note); reply != "" {
		line += " | 用户补充：" + reply
	}
	line += fmt.Sprintf(" <!-- %s -->", marker)
	content := strings.TrimRight(string(b), "\r\n")
	if content != "" {
		content += "\n"
	}
	content += line + "\n"
	if err := ensureWriteParentWithinRoot(root, path); err != nil {
		return false, err
	}
	return true, writeFileAtomic(path, []byte(content), 0644)
}

func autonomousApprovalReply(note string) string {
	reply := strings.Join(strings.Fields(note), " ")
	reply = strings.ReplaceAll(reply, "<!--", "&lt;!--")
	return strings.ReplaceAll(reply, "-->", "--&gt;")
}
