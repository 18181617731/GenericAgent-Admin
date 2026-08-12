import test from 'node:test'
import assert from 'node:assert/strict'
import { buildScheduleCreateRequest, normalizeScheduleTasksPayload } from './schedule.js'
import { frontendSource } from './frontendSources.mjs'

test('normalizeScheduleTasksPayload gives stable empty and row states', () => {
  assert.deepEqual(normalizeScheduleTasksPayload(null).tasks, [])
  const state = normalizeScheduleTasksPayload({ tasks: [{ id: 'daily', enabled: true }, null] })
  assert.equal(state.tasks.length, 1)
  assert.equal(state.tasks[0].status, 'enabled')
  assert.deepEqual(state.tasks[0].recent_reports, [])
})

test('buildScheduleCreateRequest trims id and includes default task body', () => {
  const req = buildScheduleCreateRequest(' demo ', { prompt: 'hello' })
  assert.deepEqual(req, { id: 'demo', task: { schedule: '09:00', repeat: 'daily', enabled: false, prompt: 'hello' } })
})

test('schedule UI refreshes /api/schedule/tasks and confirms dangerous create', () => {
  const source = frontendSource()
  assert.match(source, /api\('\/api\/schedule\/tasks'\)/)
  assert.match(source, /const loadScheduleTasks = async/)
  assert.match(source, /setError\(e\.message\)/)
  assert.match(source, /confirmDanger\('schedule-create'/)
  assert.match(source, /api\('\/api\/schedule\/create', \{ dangerous:true, method:'POST'/)
  assert.match(source, /<RefreshCw size=\{14\}\/>\{t\.refresh\}/)
  assert.match(source, /\{t\.hints\.noTasks\}/)
})


test('normalizeScheduleTasksPayload fills missing task identity and disabled state', () => {
  const normalized = normalizeScheduleTasksPayload({ tasks: [{ name: '', enabled: false }, { name: 'nightly', enabled: true, recent_reports: null }] })
  assert.equal(normalized.version, 'unknown')
  assert.equal(normalized.tasks[0].id, 'task-1')
  assert.equal(normalized.tasks[0].status, 'disabled')
  assert.equal(normalized.tasks[0].schedule, 'unscheduled')
  assert.equal(normalized.tasks[0].repeat, 'manual')
  assert.deepEqual(normalized.tasks[1].recent_reports, [])
  assert.equal(normalized.tasks[1].status, 'enabled')
})

test('normalizeScheduleTasksPayload preserves error states without stale enabled success', () => {
  const state = normalizeScheduleTasksPayload({ enabled: true, version: '', error: 'schedule endpoint failed', tasks: 'stale' })
  assert.equal(state.enabled, false)
  assert.equal(state.error, 'schedule endpoint failed')
  assert.equal(state.version, 'unknown')
  assert.deepEqual(state.tasks, [])
})

test('normalizeScheduleTasksPayload handles null and missing task fields as disabled rows', () => {
  const state = normalizeScheduleTasksPayload({ enabled: false, tasks: [{ id: 0, name: '', status: '', schedule: '', repeat: '', prompt: null, recent_reports: 'stale' }] })
  assert.equal(state.tasks.length, 1)
  assert.equal(state.tasks[0].id, 'task-1')
  assert.equal(state.tasks[0].enabled, false)
  assert.equal(state.tasks[0].status, 'disabled')
  assert.equal(state.tasks[0].schedule, 'unscheduled')
  assert.equal(state.tasks[0].repeat, 'manual')
  assert.equal(state.tasks[0].prompt, '')
  assert.deepEqual(state.tasks[0].recent_reports, [])
})
