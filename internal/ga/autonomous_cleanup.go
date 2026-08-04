package ga

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
)

const DefaultAutonomousReportRetention = 30

type AutonomousReportCleanupResult struct {
	Scanned int `json:"scanned"`
	Kept    int `json:"kept"`
	Deleted int `json:"deleted"`
	Skipped int `json:"skipped"`
}

func CleanupAutonomousReports(root string, keep int) (AutonomousReportCleanupResult, error) {
	if strings.TrimSpace(root) == "" {
		return AutonomousReportCleanupResult{}, errors.New("GA root is empty")
	}
	if keep < 1 {
		keep = DefaultAutonomousReportRetention
	}
	overview, err := BuildAutonomousApprovals(root)
	if err != nil {
		return AutonomousReportCleanupResult{}, err
	}
	protected := protectedAutonomousReports(overview)
	reports := buildAllAutonomousReports(root)
	result := AutonomousReportCleanupResult{}
	seen := 0
	for _, report := range reports {
		if !isTemporaryAutonomousReport(report) {
			continue
		}
		result.Scanned++
		if seen < keep {
			seen++
			result.Kept++
			continue
		}
		if protected[report.Path] {
			result.Skipped++
			continue
		}
		content, ok := readListedAutonomousReport(root, report.Path)
		if !ok || autonomousReportNeedsApproval(content) {
			result.Skipped++
			continue
		}
		path := filepath.Join(root, filepath.FromSlash(report.Path))
		if err := os.Remove(path); err != nil {
			return result, err
		}
		result.Deleted++
	}
	return result, nil
}

func autonomousReportNeedsApproval(content string) bool {
	needsApproval, _, _, _ := autonomousReportReviewSignals(content)
	return needsApproval
}

func isTemporaryAutonomousReport(report Entry) bool {
	return report.Kind == "file" && strings.HasPrefix(filepath.ToSlash(report.Path), "temp/autonomous_reports/") && !strings.EqualFold(report.Name, "history.txt")
}

func protectedAutonomousReports(overview AutonomousApprovalOverview) map[string]bool {
	protected := map[string]bool{}
	for _, item := range overview.Items {
		if item.State != "pending" && item.Decision != "" {
			continue
		}
		if item.ReviewReport != nil {
			protected[item.ReviewReport.Path] = true
		}
		for _, report := range item.ReviewReports {
			protected[report.Path] = true
		}
	}
	return protected
}
