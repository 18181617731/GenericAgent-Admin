import test from 'node:test'
import assert from 'node:assert/strict'
import { buildNotificationSnapshot, collectNotificationEvents } from './notificationMonitor.js'

const snapshot = input => buildNotificationSnapshot(input)

test('does not notify for the initial monitor baseline', () => {
  const current = snapshot({ schedule: { tasks: [{ id: 'daily', status: 'HEALTHY', last_report: { path: 'sche_tasks/done/daily.md' } }] } })
  assert.deepEqual(collectNotificationEvents(null, current), [])
})

test('notifies when a scheduled task produces a new report or fails', () => {
  const previous = snapshot({ schedule: { tasks: [{ id: 'daily', status: 'HEALTHY', last_report: { path: 'old.md', mod_time: '2026-08-05T10:00:00Z' } }] } })
  const success = snapshot({ schedule: { tasks: [{ id: 'daily', status: 'HEALTHY', last_report: { path: 'new.md', mod_time: '2026-08-05T11:00:00Z' } }] } })
  assert.equal(collectNotificationEvents(previous, success)[0].level, 'success')
  const failed = snapshot({ schedule: { tasks: [{ id: 'daily', status: 'ERROR', error: '模型不可用', last_report: { path: 'newer.md', mod_time: '2026-08-05T12:00:00Z' } }] } })
  assert.equal(collectNotificationEvents(success, failed)[0].level, 'error')
})

test('notifies Goal completion and autonomous approval transitions', () => {
  const previous = snapshot({ goals: [{ id: 'goal-1', status: 'running', running: true }], approvals: { items: [{ id: 'draft-1', title: '更新记忆', state: 'pending' }] } })
  const current = snapshot({ goals: [{ id: 'goal-1', status: 'completed', running: false }], approvals: { items: [{ id: 'draft-1', title: '更新记忆', state: 'approved', decision: 'approved', execution_state: 'completed' }] } })
  const events = collectNotificationEvents(previous, current)
  assert.ok(events.some(event => event.category === 'goal' && event.level === 'success'))
  assert.ok(events.some(event => event.category === 'autonomous' && event.level === 'success'))
})

test('notifies a newly discovered pending approval but ignores handled items', () => {
  const previous = snapshot({ approvals: { items: [] } })
  const current = snapshot({ approvals: { items: [{ id: 'draft-1', title: '需要确认', state: 'pending' }, { id: 'done-1', title: '已完成', state: 'closed' }] } })
  const events = collectNotificationEvents(previous, current)
  assert.deepEqual(events.map(event => event.dedupeKey), ['approval:draft-1:pending'])
})
