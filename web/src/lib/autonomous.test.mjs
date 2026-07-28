import test from 'node:test'
import assert from 'node:assert/strict'
import { autonomousServiceView, autonomousSummary, filterAutonomousReports, latestAutonomousReport, readableAutonomousDate, splitAutonomousApprovals, summarizeAutonomousReport } from './autonomous.js'

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
