import test from 'node:test'
import assert from 'node:assert/strict'
import { hasSubagentLaunch, subagentCardGroups, subagentCardView, formatAgo } from './subagentCards.js'

test('hasSubagentLaunch detects --task launches in transcript', () => {
  assert.equal(hasSubagentLaunch([{ content: 'python agentmain.py --task sup_c_test2' }]), true)
  assert.equal(hasSubagentLaunch([{ content: 'plain chat' }, { content: 42 }]), false)
  assert.equal(hasSubagentLaunch(null), false)
})

test('subagentCardView derives state with priority intervene > stop > round end > stall', () => {
  const now = 10 * 60 * 1000
  const base = { name: 't', rounds: 2, updated_at: now - 1000, latest_summary: 's' }
  assert.deepEqual(
    [subagentCardView({ ...base }, now).label, subagentCardView({ ...base }, now).tone],
    ['运行中', 'run'])
  assert.equal(subagentCardView({ ...base, round_ended: true }, now).label, '本轮完成')
  assert.equal(subagentCardView({ ...base, updated_at: now - 4 * 60 * 1000 }, now).label, '疑似停滞')
  // round end wins over staleness (finished rounds are naturally old)
  assert.equal(subagentCardView({ ...base, round_ended: true, updated_at: now - 9 * 60 * 1000 }, now).tone, 'done')
  assert.equal(subagentCardView({ ...base, stop_requested: true }, now).tone, 'stop')
  assert.equal(subagentCardView({ ...base, stop_requested: true, intervened: true }, now).label, '已干预')
  assert.equal(subagentCardView(null), null)
})

test('formatAgo buckets', () => {
  const now = 100 * 86400000
  assert.equal(formatAgo(0, now), '')
  assert.equal(formatAgo(now - 3000, now), '刚刚')
  assert.equal(formatAgo(now - 30 * 1000, now), '30秒前')
  assert.equal(formatAgo(now - 5 * 60000, now), '5分钟前')
  assert.equal(formatAgo(now - 3 * 3600000, now), '3小时前')
  assert.equal(formatAgo(now - 2 * 86400000, now), '2天前')
})

test('subagentCardGroups keeps current work visible and folds settled history', () => {
  const now = 10 * 60 * 1000
  const groups = subagentCardGroups([
    { name: 'running', rounds: 1, updated_at: now - 1000 },
    { name: 'stalled', rounds: 1, updated_at: now - 4 * 60000 },
    { name: 'done', rounds: 2, round_ended: true, updated_at: now - 3600000 },
    { name: 'old-stop', rounds: 1, stop_requested: true, updated_at: now - 3600000 },
    { name: 'new-stop', rounds: 1, stop_requested: true, updated_at: now - 1000 },
  ], now)

  assert.deepEqual(groups.current.map(item => item.name), ['running', 'stalled', 'new-stop'])
  assert.deepEqual(groups.history.map(item => item.name), ['done', 'old-stop'])
})
