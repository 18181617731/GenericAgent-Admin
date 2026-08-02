package ga

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

const autonomousApprovalFixture = `# pending_drafts

## 状态图例
- 待批未落地：等待用户批准

## 1. daily_git_conflict_sop_DRAFT
- 来源：R37
- 草案位置：temp/daily_git_conflict_sop_DRAFT.md
- 落地目标：memory/daily_git_conflict_sop.md
- **状态：待批未落地**
- 核查证据：目标不存在
- 风险：低。纯文档变更
- 下一步：用户批准后写入 memory

## 2. github_push_sop 引用工具
- 来源：R66
- **状态：工具已固化，但 SOP 尚未引用（遗留缺口）**
- 风险：低
- 下一步：用户批准后补充引用

## 3. 已完成项目
- 状态：已落地
- 下一步：无

## 汇总
- 待批未落地：2
`

func writeAutonomousApprovalFixture(t *testing.T, root string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Join(root, "temp"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, filepath.FromSlash(autonomousApprovalSource)), []byte(autonomousApprovalFixture), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, filepath.FromSlash(autonomousTodoPath)), []byte("# TODO\n"), 0644); err != nil {
		t.Fatal(err)
	}
}

func TestBuildAutonomousApprovalsParsesTrackedDrafts(t *testing.T) {
	root := t.TempDir()
	writeAutonomousApprovalFixture(t, root)
	overview, err := BuildAutonomousApprovals(root)
	if err != nil {
		t.Fatal(err)
	}
	if !overview.SourceExists || overview.Pending != 2 || len(overview.Items) != 3 {
		t.Fatalf("overview = %+v", overview)
	}
	first := overview.Items[0]
	if first.Title != "daily_git_conflict_sop_DRAFT" || first.State != "pending" || first.Target != "memory/daily_git_conflict_sop.md" {
		t.Fatalf("first approval = %+v", first)
	}
	if overview.Items[2].State != "closed" {
		t.Fatalf("closed item = %+v", overview.Items[2])
	}
}

func TestBuildAutonomousApprovalsFallsBackToTodoQueue(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "temp"), 0755); err != nil {
		t.Fatal(err)
	}
	todo := `# TODO
[ ] 自身演进 | complete_task 待用户确认的源码变更 | 经用户批准后执行
[ ] 未标注的候选任务 | 需要人工复核后再决定
[x] 已完成但曾提到待用户批准 | 已归档
[ ] 用户已批准 | 已批准任务 | 按 TODO 执行
`
	if err := os.WriteFile(filepath.Join(root, filepath.FromSlash(autonomousTodoPath)), []byte(todo), 0644); err != nil {
		t.Fatal(err)
	}
	overview, err := BuildAutonomousApprovals(root)
	if err != nil {
		t.Fatal(err)
	}
	if !overview.SourceExists || overview.SourcePath != autonomousTodoPath {
		t.Fatalf("todo fallback source = %+v", overview)
	}
	if overview.Pending != 2 || overview.Approved != 1 || len(overview.Items) != 3 {
		t.Fatalf("todo fallback overview = %+v", overview)
	}
	if overview.Items[0].Title != "complete_task 待用户确认的源码变更" || overview.Items[0].State != "pending" {
		t.Fatalf("todo pending item = %+v", overview.Items[0])
	}
}

func TestApproveAutonomousTodoItemMergesQueueMarker(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "temp"), 0755); err != nil {
		t.Fatal(err)
	}
	todo := "[ ] 自身演进 | complete_task 需要用户确认的变更 | 用户批准后执行\n"
	if err := os.WriteFile(filepath.Join(root, filepath.FromSlash(autonomousTodoPath)), []byte(todo), 0644); err != nil {
		t.Fatal(err)
	}
	overview, err := BuildAutonomousApprovals(root)
	if err != nil || len(overview.Items) != 1 {
		t.Fatalf("initial todo overview = %+v err=%v", overview, err)
	}
	updated, queued, err := DecideAutonomousApproval(root, overview.Items[0].ID, "approved", "先执行自检")
	if err != nil || !queued || updated.Pending != 0 || updated.Approved != 1 || len(updated.Items) != 1 {
		t.Fatalf("approved todo overview = %+v queued=%v err=%v", updated, queued, err)
	}
	content, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(autonomousTodoPath)))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Count(string(content), "ga-admin-approval:"+overview.Items[0].ID) != 1 {
		t.Fatalf("approval marker count = %d, content=%s", strings.Count(string(content), "ga-admin-approval:"+overview.Items[0].ID), content)
	}
}

func TestBuildAutonomousApprovalsKeepsSupersededDraftsOutOfPending(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "temp"), 0755); err != nil {
		t.Fatal(err)
	}
	source := "## 1. obsolete draft\n- 状态：被替代覆盖\n- 下一步：新方案尚未接入调度\n"
	if err := os.WriteFile(filepath.Join(root, filepath.FromSlash(autonomousApprovalSource)), []byte(source), 0644); err != nil {
		t.Fatal(err)
	}
	overview, err := BuildAutonomousApprovals(root)
	if err != nil {
		t.Fatal(err)
	}
	if overview.Pending != 0 || len(overview.Items) != 1 || overview.Items[0].State != "closed" {
		t.Fatalf("superseded overview = %+v", overview)
	}
}

func TestApproveAutonomousDraftQueuesExactlyOnce(t *testing.T) {
	root := t.TempDir()
	writeAutonomousApprovalFixture(t, root)
	overview, err := BuildAutonomousApprovals(root)
	if err != nil {
		t.Fatal(err)
	}
	id := overview.Items[0].ID
	updated, queued, err := DecideAutonomousApproval(root, id, "approved", "reviewed")
	if err != nil || !queued {
		t.Fatalf("first approve queued=%v err=%v", queued, err)
	}
	if updated.Approved != 1 || updated.Pending != 1 || updated.Items[0].Note != "reviewed" || updated.Items[0].ExecutionState != autonomousExecutionQueued {
		t.Fatalf("updated overview = %+v", updated)
	}
	_, queued, err = DecideAutonomousApproval(root, id, "approved", "ignored repeat")
	if err != nil || queued {
		t.Fatalf("repeat approve queued=%v err=%v", queued, err)
	}
	todo, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(autonomousTodoPath)))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Count(string(todo), "ga-admin-approval:"+id) != 1 || !strings.Contains(string(todo), "[ ] 用户已批准") || !strings.Contains(string(todo), "用户补充：reviewed") {
		t.Fatalf("TODO was not queued exactly once: %s", todo)
	}
}

func TestApprovedAutonomousTaskExposesExecutionReport(t *testing.T) {
	root := t.TempDir()
	writeAutonomousApprovalFixture(t, root)
	overview, err := BuildAutonomousApprovals(root)
	if err != nil {
		t.Fatal(err)
	}
	id := overview.Items[0].ID
	if _, _, err := DecideAutonomousApproval(root, id, "approved", "reviewed"); err != nil {
		t.Fatal(err)
	}
	reportsRoot := filepath.Join(root, "temp", "autonomous_reports")
	if err := os.MkdirAll(reportsRoot, 0755); err != nil {
		t.Fatal(err)
	}
	reportPath := filepath.Join(reportsRoot, "R99_execution.md")
	report := "# 执行报告\n\n- 审批标记：ga-admin-approval:" + id + "\n- 结论：已完成并通过验证\n"
	if err := os.WriteFile(reportPath, []byte(report), 0644); err != nil {
		t.Fatal(err)
	}
	updated, err := BuildAutonomousApprovals(root)
	if err != nil {
		t.Fatal(err)
	}
	item := updated.Items[0]
	if item.ExecutionState != autonomousExecutionCompleted || item.ExecutionReport == nil || item.ExecutionReport.Path != "temp/autonomous_reports/R99_execution.md" {
		t.Fatalf("execution result = %+v", item)
	}
	if !strings.Contains(item.ExecutionSummary, "已完成并通过验证") {
		t.Fatalf("execution summary = %q", item.ExecutionSummary)
	}
}

func TestApprovedAutonomousTaskReportsFailure(t *testing.T) {
	root := t.TempDir()
	writeAutonomousApprovalFixture(t, root)
	overview, _ := BuildAutonomousApprovals(root)
	if _, _, err := DecideAutonomousApproval(root, overview.Items[0].ID, "approved", ""); err != nil {
		t.Fatal(err)
	}
	reportsRoot := filepath.Join(root, "temp", "autonomous_reports")
	if err := os.MkdirAll(reportsRoot, 0755); err != nil {
		t.Fatal(err)
	}
	content := "# 执行报告\n\n审批标记：ga-admin-approval:" + overview.Items[0].ID + "\nVERDICT: FAIL\n执行结果：失败，校验未通过\n"
	if err := os.WriteFile(filepath.Join(reportsRoot, "R100_execution.md"), []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
	updated, err := BuildAutonomousApprovals(root)
	if err != nil {
		t.Fatal(err)
	}
	item := updated.Items[0]
	if item.ExecutionState != autonomousExecutionFailed || item.ExecutionError == "" {
		t.Fatalf("failed execution = %+v", item)
	}
}

func TestApprovalAuditReferenceIsNotExecutionReport(t *testing.T) {
	root := t.TempDir()
	writeAutonomousApprovalFixture(t, root)
	overview, _ := BuildAutonomousApprovals(root)
	if _, _, err := DecideAutonomousApproval(root, overview.Items[0].ID, "approved", ""); err != nil {
		t.Fatal(err)
	}
	reportsRoot := filepath.Join(root, "temp", "autonomous_reports")
	if err := os.MkdirAll(reportsRoot, 0755); err != nil {
		t.Fatal(err)
	}
	audit := "# 审批门控复核\n\n本轮启动时 TODO 中的条目为：\n\n[ ] 用户已批准 | " + overview.Items[0].Title + " | 待执行\n\n结论：未修改源码，原实施 TODO 保持 [ ]\n"
	if err := os.WriteFile(filepath.Join(reportsRoot, "R101_audit.md"), []byte(audit), 0644); err != nil {
		t.Fatal(err)
	}
	updated, err := BuildAutonomousApprovals(root)
	if err != nil {
		t.Fatal(err)
	}
	if updated.Items[0].ExecutionState != autonomousExecutionQueued || updated.Items[0].ExecutionReport != nil {
		t.Fatalf("audit was incorrectly linked: %+v", updated.Items[0])
	}
}

func TestCompletedAutonomousQueueWithoutReportIsVisible(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "temp"), 0755); err != nil {
		t.Fatal(err)
	}
	title := "已批准但报告缺失"
	id := autonomousApprovalID(title)
	todo := "[x] 用户已批准 | " + title + " | 已执行 <!-- ga-admin-approval:" + id + " -->\n"
	if err := os.WriteFile(filepath.Join(root, filepath.FromSlash(autonomousTodoPath)), []byte(todo), 0644); err != nil {
		t.Fatal(err)
	}
	overview, err := BuildAutonomousApprovals(root)
	if err != nil || len(overview.Items) != 1 {
		t.Fatalf("overview = %+v err=%v", overview, err)
	}
	if overview.Items[0].ExecutionState != autonomousExecutionReportMissing {
		t.Fatalf("missing report state = %+v", overview.Items[0])
	}
}

func TestApprovedReplyStaysOnOneSafeTodoLine(t *testing.T) {
	root := t.TempDir()
	writeAutonomousApprovalFixture(t, root)
	overview, _ := BuildAutonomousApprovals(root)
	_, _, err := DecideAutonomousApproval(root, overview.Items[0].ID, "approved", "先验证\n再执行 <!-- hidden -->")
	if err != nil {
		t.Fatal(err)
	}
	todo, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(autonomousTodoPath)))
	if err != nil {
		t.Fatal(err)
	}
	content := string(todo)
	if !strings.Contains(content, "用户补充：先验证 再执行 &lt;!-- hidden --&gt;") || strings.Count(content, "ga-admin-approval:") != 1 {
		t.Fatalf("unsafe approval reply: %s", content)
	}
}

func TestRejectAutonomousDraftOnlyRecordsDecision(t *testing.T) {
	root := t.TempDir()
	writeAutonomousApprovalFixture(t, root)
	overview, err := BuildAutonomousApprovals(root)
	if err != nil {
		t.Fatal(err)
	}
	id := overview.Items[1].ID
	updated, queued, err := DecideAutonomousApproval(root, id, "rejected", "not needed")
	if err != nil || queued || updated.Rejected != 1 || updated.Pending != 1 {
		t.Fatalf("reject overview=%+v queued=%v err=%v", updated, queued, err)
	}
	todo, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(autonomousTodoPath)))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(todo), id) {
		t.Fatalf("rejected approval was queued: %s", todo)
	}
	if _, _, err := DecideAutonomousApproval(root, id, "approved", ""); err == nil {
		t.Fatal("conflicting decision should fail")
	}
}

func TestAutonomousApprovalValidationAndMissingSource(t *testing.T) {
	root := t.TempDir()
	overview, err := BuildAutonomousApprovals(root)
	if err != nil || overview.SourceExists || len(overview.Items) != 0 {
		t.Fatalf("missing source overview=%+v err=%v", overview, err)
	}
	writeAutonomousApprovalFixture(t, root)
	if _, _, err := DecideAutonomousApproval(root, "missing", "approved", ""); err == nil {
		t.Fatal("missing approval should fail")
	}
	if _, _, err := DecideAutonomousApproval(root, "missing", "later", ""); err == nil {
		t.Fatal("invalid decision should fail")
	}
	if _, _, err := DecideAutonomousApproval(root, "missing", "approved", strings.Repeat("x", 1001)); err == nil {
		t.Fatal("oversized note should fail")
	}
}

func TestAutonomousDecisionTimestampIsReturned(t *testing.T) {
	root := t.TempDir()
	writeAutonomousApprovalFixture(t, root)
	fixed := time.Date(2026, 7, 28, 15, 0, 0, 0, time.Local)
	oldNow := autonomousApprovalNow
	autonomousApprovalNow = func() time.Time { return fixed }
	t.Cleanup(func() { autonomousApprovalNow = oldNow })
	overview, _ := BuildAutonomousApprovals(root)
	updated, _, err := DecideAutonomousApproval(root, overview.Items[0].ID, "approved", "")
	if err != nil {
		t.Fatal(err)
	}
	if !updated.Items[0].DecidedAt.Equal(fixed) {
		t.Fatalf("decided_at=%v want %v", updated.Items[0].DecidedAt, fixed)
	}
}

func TestBuildAutonomousApprovalsFindsBlockedReportWithoutPendingDraft(t *testing.T) {
	root := t.TempDir()
	reportsRoot := filepath.Join(root, "temp", "autonomous_reports")
	if err := os.MkdirAll(reportsRoot, 0755); err != nil {
		t.Fatal(err)
	}
	content := `# complete_task 双前缀最小修复实施：审批阻塞复核

- 结论：BLOCKED（审批证据不可核验，未实施 memory 源码修改）
- 原 TODO 继续待审，仍需等待用户批准
- [ ] complete_task 双前缀最小修复实施
`
	if err := os.WriteFile(filepath.Join(reportsRoot, "R49_complete_task 双前缀实施审批阻塞复核.md"), []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
	overview, err := BuildAutonomousApprovals(root)
	if err != nil {
		t.Fatal(err)
	}
	if overview.Pending != 1 || len(overview.Items) != 1 {
		t.Fatalf("report-only overview = %+v", overview)
	}
	item := overview.Items[0]
	if item.State != "pending" || item.Decision != "" || item.CandidateSource != autonomousCandidateSourceReport || item.ReviewStatus != autonomousReviewNeedsApproval {
		t.Fatalf("report candidate = %+v", item)
	}
	if item.ReviewReport == nil || item.ReviewReport.Name != "R49_complete_task 双前缀实施审批阻塞复核.md" {
		t.Fatalf("report evidence = %+v", item.ReviewReport)
	}
}

func TestBlockedReportInvalidatesUnverifiedTodoApproval(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "temp", "autonomous_reports"), 0755); err != nil {
		t.Fatal(err)
	}
	title := "complete_task 双前缀最小修复实施（承接R19·待审批后执行）"
	id := autonomousApprovalID(title)
	todo := "[ ] 用户已批准 | " + title + " | 执行 <!-- ga-admin-approval:" + id + " -->\n"
	if err := os.WriteFile(filepath.Join(root, filepath.FromSlash(autonomousTodoPath)), []byte(todo), 0644); err != nil {
		t.Fatal(err)
	}
	ledger := `{"schema_version":1,"decisions":[{"id":"` + id + `","title":"` + title + `","decision":"approved","decided_at":"2026-08-02T00:31:03+08:00"}]}`
	if err := os.WriteFile(filepath.Join(root, filepath.FromSlash(autonomousDecisionPath)), []byte(ledger), 0644); err != nil {
		t.Fatal(err)
	}
	report := "# complete_task 双前缀最小修复实施：审批阻塞复核\n\n结论：BLOCKED，审批证据不可核验，未实施 memory 源码修改。\n"
	if err := os.WriteFile(filepath.Join(root, "temp", "autonomous_reports", "R49_complete_task.md"), []byte(report), 0644); err != nil {
		t.Fatal(err)
	}
	overview, err := BuildAutonomousApprovals(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(overview.Items) != 1 || overview.Pending != 1 || overview.Approved != 0 || overview.Items[0].State != "pending" {
		t.Fatalf("invalidated approval overview = %+v", overview)
	}
}

func TestBuildAutonomousApprovalsScansReportsBeyondInventoryWindow(t *testing.T) {
	root := t.TempDir()
	reportsRoot := filepath.Join(root, "temp", "autonomous_reports")
	if err := os.MkdirAll(reportsRoot, 0755); err != nil {
		t.Fatal(err)
	}
	for index := 0; index < 81; index++ {
		path := filepath.Join(reportsRoot, fmt.Sprintf("R%03d_filler.md", index))
		if err := os.WriteFile(path, []byte("# completed report\nVERDICT: PASS\n"), 0644); err != nil {
			t.Fatal(err)
		}
	}
	blocked := filepath.Join(reportsRoot, "R49_complete_task_blocked.md")
	if err := os.WriteFile(blocked, []byte("# complete_task blocked approval\nBLOCKED\napproval evidence is missing\n[ ] pending approval\n"), 0644); err != nil {
		t.Fatal(err)
	}
	old := time.Now().Add(-2 * time.Hour)
	if err := os.Chtimes(blocked, old, old); err != nil {
		t.Fatal(err)
	}
	overview, err := BuildAutonomousApprovals(root)
	if err != nil {
		t.Fatal(err)
	}
	if overview.Pending != 1 || len(overview.Items) != 1 || overview.Items[0].ReviewReport == nil || overview.Items[0].ReviewReport.Path != "temp/autonomous_reports/R49_complete_task_blocked.md" {
		t.Fatalf("full report scan missed blocked report: %+v", overview)
	}
}
