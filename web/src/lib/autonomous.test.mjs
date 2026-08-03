import test from 'node:test'
import assert from 'node:assert/strict'
import { autonomousExecutionState, autonomousHandledProgress, autonomousReviewView, autonomousServiceView, autonomousSummary, filterAutonomousReports, latestAutonomousReport, readableAutonomousDate, splitAutonomousApprovals, summarizeAutonomousApproval, summarizeAutonomousReport, summarizeAutonomousReviewNeed } from './autonomous.js'

test('autonomous summary reports running services approvals and latest record', () => {
  const latest = { name: 'latest.md', mod_time: '2026-07-28T10:00:00Z' }
  assert.deepEqual(autonomousSummary({
    services: [{ running: true }, { running: false }],
    approvals: { pending: 2 },
    reports: [latest, { name: 'older.md' }],
  }), { running: 1, total: 2, pending: 2, reports: 2, latestReport: latest })
})

test('autonomous report search is case insensitive and includes paths', () => {
  const reports = [
    { name: 'Daily Review.md', path: 'temp/autonomous_reports/daily.md' },
    { name: 'result.md', path: 'autonomous_reports/DEPLOYMENT/result.md' },
  ]
  assert.deepEqual(filterAutonomousReports(reports, 'review'), [reports[0]])
  assert.deepEqual(filterAutonomousReports(reports, 'deployment'), [reports[1]])
  assert.deepEqual(filterAutonomousReports(reports, '  '), reports)
})

test('autonomous approvals keep every non-pending ledger item visible as handled', () => {
  const items = [{ state: 'pending' }, { state: 'approved' }, { state: 'rejected' }, { state: 'closed' }, { state: 'tracked' }]
  const result = splitAutonomousApprovals(items)
  assert.equal(result.pending.length, 1)
  assert.deepEqual(result.handled, items.slice(1))
})

test('autonomous handled approvals are grouped by execution progress', () => {
  const queued = { id: 'queued', state: 'approved', decision: 'approved', execution_state: 'queued' }
  const archived = { id: 'archived', state: 'approved', decision: 'approved', execution_state: 'completed', execution_report: { path: 'temp/autonomous_reports/R1.md' } }
  const missing = { id: 'missing', state: 'approved', decision: 'approved', execution_state: 'report_missing' }
  const failed = { id: 'failed', state: 'approved', decision: 'approved', execution_state: 'failed' }
  const rejected = { id: 'rejected', state: 'rejected', decision: 'rejected' }
  const result = splitAutonomousApprovals([queued, archived, missing, failed, rejected])
  assert.equal(autonomousHandledProgress(queued), 'queued')
  assert.deepEqual(result.handledGroups.map(group => [group.key, group.items]), [
    ['queued', [queued]],
    ['completed', [archived]],
    ['report_missing', [missing]],
    ['failed', [failed]],
    ['not_applicable', [rejected]],
  ])
})

test('autonomous execution state defaults approved work to queued', () => {
  assert.equal(autonomousExecutionState({ decision: 'approved' }), 'queued')
  assert.equal(autonomousExecutionState({ decision: 'approved', execution_state: 'completed' }), 'completed')
  assert.equal(autonomousExecutionState({ decision: 'rejected' }), 'not_applicable')
})

test('approval outcome summaries prefer explicit text and explain fallback outcomes plainly', () => {
  assert.equal(summarizeAutonomousApproval({ expected_outcome: '以后可以直接照着执行' }), '以后可以直接照着执行')
  assert.equal(summarizeAutonomousApproval({ target: 'memory/example.md' }), '批准后会把相关方案整理到 memory/example.md，以后遇到同类问题时可以直接参考。')
  assert.match(summarizeAutonomousApproval({ next_step: 'Add the reference and generate a report' }, 'en'), /After approval, the autonomous workflow will add/)
})

test('autonomous handled approvals expose progress groups in stable order', () => {
  const result = splitAutonomousApprovals([
    { id: 'unknown', state: 'closed' },
    { id: 'rejected', state: 'rejected', decision: 'rejected' },
    { id: 'queued', state: 'approved', decision: 'approved' },
  ])
  assert.deepEqual(result.handledGroups.map(group => ({ key: group.key, ids: group.items.map(item => item.id) })), [
    { key: 'queued', ids: ['queued'] },
    { key: 'not_applicable', ids: ['rejected'] },
    { key: 'unknown', ids: ['unknown'] },
  ])
})

test('autonomous review view makes model-unavailable fallback explicit', () => {
  const review = autonomousReviewView({
    review_status: 'fallback',
    review_decision: 'needs_approval',
    review_confidence: 'high',
    review_reason: 'report contains an explicit approval gate; model review unavailable: model review unavailable; retry scheduled; conservative rule retained',
    review_model_no: 12,
    review_model: 'gpt-5.6-luna',
    review_provider: '自费帅API gpt',
  })
  assert.equal(review.kind, 'unavailable')
  assert.equal(review.method, '仅规则筛选（未经过模型判断）')
  assert.equal(review.badge, '模型审核不可用')
  assert.equal(review.decision, '未自动批准')
  assert.match(review.summary, /不是模型审核通过/)
  assert.deepEqual(review.basis, ['报告明确要求人工审批', '重新审核时会再次尝试', '暂时按保守规则保留为待审批'])
  assert.match(summarizeAutonomousReviewNeed({ target: 'memory/autonomous_sop.md' }, review), /脚本规则筛选/)
  assert.match(summarizeAutonomousReviewNeed({ target: 'memory/autonomous_sop.md' }, review), /没有做模型判断/)
  assert.match(summarizeAutonomousReviewNeed({ target: 'memory/autonomous_sop.md' }, review), /人工决定批准还是拒绝/)
})

test('autonomous labels provide friendly service names and safe date fallback', () => {
  const service = autonomousServiceView({ name: 'reflect/autonomous.py' }, 'zh')
  assert.equal(service.title, '主自主引擎')
  assert.match(service.help, /自主任务队列/)
  assert.equal(autonomousServiceView({ name: 'reflect/custom.py' }, 'en').title, 'custom')
  assert.equal(readableAutonomousDate('not-a-date', 'zh'), '时间未知')
})

test('latest autonomous result prefers a report and summarizes its final section', () => {
  const history = { name: 'history.txt', path: 'temp/autonomous_reports/history.txt' }
  const report = { name: 'R216.md', path: 'temp/autonomous_reports/R216.md' }
  assert.equal(latestAutonomousReport([history, report]), report)
  assert.equal(summarizeAutonomousReport('# 标题\n前文\n\n## 最终结果\n- 已完成验证\n- 无待处理异常'), '已完成验证 - 无待处理异常')
})
