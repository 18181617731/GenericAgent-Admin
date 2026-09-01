import test from 'node:test'
import assert from 'node:assert/strict'
import { autonomousExecutionState, autonomousHandledProgress, autonomousReviewView, autonomousServiceView, autonomousSummary, filterAutonomousReports, latestAutonomousReport, readableAutonomousDate, splitAutonomousApprovals, summarizeAutonomousProblem, summarizeAutonomousReport, summarizeAutonomousReviewNeed } from './autonomous.js'

test('autonomous summary reports running services approvals and latest record', () => {
  const latest = { name: 'latest.md', mod_time: '2026-07-28T10:00:00Z' }
  assert.deepEqual(autonomousSummary({
    services: [{ running: true }, { running: false }],
    approvals: { pending: 2 },
    reports: [latest, { name: 'older.md' }],
  }), { running: 1, total: 2, pending: 2, reports: 2, latestReport: latest })
})

test('autonomous summary derives pending count from normalized items', () => {
  assert.equal(autonomousSummary({
    approvals: {
      pending: 3,
      items: [
        { state: 'pending', status: '已完成' },
        { state: 'pending', status: '无需审批' },
        { state: 'pending', status: '待批未落地', next_step: '用户批准后执行' },
      ],
    },
  }).pending, 1)
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

test('approval problem summaries prefer the model-generated plain-language text', () => {
  assert.equal(summarizeAutonomousProblem({ problem: '修复台账与真实文件状态不一致' }), '修复台账与真实文件状态不一致')
  assert.match(summarizeAutonomousProblem({ title: '示例任务' }), /这项任务是为了解决“示例任务”/)
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
    { key: 'closed', ids: ['unknown'] },
  ])
})

test('autonomous pending view removes stale completed and no-approval items', () => {
  const completed = { id: 'completed', state: 'pending', status: '已完成并通过验证' }
  const noApproval = { id: 'no-approval', state: 'pending', status: '无需审批' }
  const queued = { id: 'queued-stale', state: 'pending', execution_state: 'queued' }
  const pending = { id: 'pending', state: 'pending', status: '待批未落地', next_step: '用户批准后执行' }
  const result = splitAutonomousApprovals([completed, noApproval, queued, pending])
  assert.deepEqual(result.pending.map(item => item.id), ['pending'])
  assert.deepEqual(result.handledGroups.map(group => ({ key: group.key, ids: group.items.map(item => item.id) })), [
    { key: 'queued', ids: ['queued-stale'] },
    { key: 'not_required', ids: ['no-approval'] },
    { key: 'closed', ids: ['completed'] },
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

test('autonomous review view explains model auto-approval and its boundary', () => {
  const item = {
    decision: 'approved',
    decision_source: 'model_auto',
    review_status: 'model',
    review_decision: 'approved',
    review_risk: 'low',
    review_confidence: 'high',
    review_reason: '证据充分且无需人工复核',
  }
  const review = autonomousReviewView(item, 'zh')
  assert.equal(review.kind, 'model_auto')
  assert.equal(review.badge, '模型自动批准')
  assert.equal(review.risk, '低')
  assert.match(review.summary, /已自动批准并加入自主任务队列/)
  assert.match(summarizeAutonomousReviewNeed({ target: 'memory/read_only_check.md' }, review), /不会立即修改文件/)
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
