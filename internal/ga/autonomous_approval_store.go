package ga

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

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
