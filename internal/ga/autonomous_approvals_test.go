package ga

import (
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
	if updated.Approved != 1 || updated.Pending != 1 || updated.Items[0].Note != "reviewed" {
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
