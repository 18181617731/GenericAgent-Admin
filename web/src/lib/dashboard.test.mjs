import test from 'node:test'
import assert from 'node:assert/strict'
import { dashboardSummary } from './dashboard.js'

test('dashboard summary distinguishes available services from actively running services', () => {
  const summary = dashboardSummary([
    { name: 'scheduler', running: true },
    { name: 'worker', running: false },
  ], { task_count: 3, enabled: 2, overdue: 1, errors: 0 })

  assert.deepEqual(summary, {
    managedServices: 2,
    runningServices: 1,
    taskCount: 3,
    enabledTasks: 2,
    overdueTasks: 1,
    taskErrors: 0,
  })
})

test('dashboard summary keeps missing or invalid counts at zero', () => {
  assert.deepEqual(dashboardSummary(null, { enabled: 'invalid', overdue: -1 }), {
    managedServices: 0,
    runningServices: 0,
    taskCount: 0,
    enabledTasks: 0,
    overdueTasks: 0,
    taskErrors: 0,
  })
})
