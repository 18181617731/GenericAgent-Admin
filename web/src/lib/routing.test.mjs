import test from 'node:test'
import assert from 'node:assert/strict'
import { NAV_ITEMS, buildRoute, parseRoute } from './routing.js'

const setLocation = (url) => {
  globalThis.window = { location: new URL(url) }
}

test('parseRoute keeps Goal Mode independent from scheduled tasks', () => {
  setLocation('http://localhost/goals')
  assert.deepEqual(parseRoute(), { tab: 'goals', taskSubTab: 'scheduled' })
  setLocation('http://localhost/tasks/runs')
  assert.deepEqual(parseRoute(), { tab: 'goals', taskSubTab: 'scheduled' })
  setLocation('http://localhost/tasks/services')
  assert.deepEqual(parseRoute(), { tab: 'tasks', taskSubTab: 'scheduled' })
  setLocation('http://localhost/tmwd')
  assert.deepEqual(parseRoute(), { tab: 'overview', taskSubTab: 'scheduled' })
})

test('parseRoute prefers hash routes', () => {
  setLocation('http://localhost/settings#/tasks/reports')
  assert.deepEqual(parseRoute(), { tab: 'tasks', taskSubTab: 'reports' })
})

test('buildRoute normalizes invalid tabs and task sub tabs', () => {
  assert.equal(buildRoute('missing'), '/overview')
  assert.equal(buildRoute('tasks', 'missing'), '/tasks/scheduled')
})

test('scheduled tasks are immediately above autonomous navigation', () => {
  assert.equal(NAV_ITEMS[NAV_ITEMS.indexOf('autonomous') - 1], 'tasks')
})

test('usage overview has a stable refreshable route', () => {
  setLocation('http://localhost/usage')
  assert.deepEqual(parseRoute(), { tab: 'usage', taskSubTab: 'scheduled' })
  assert.equal(buildRoute('usage'), '/usage')
})

test('GA instances has a stable refreshable route', () => {
  setLocation('http://localhost/instances')
  assert.deepEqual(parseRoute(), { tab: 'instances', taskSubTab: 'services' })
  assert.equal(buildRoute('instances'), '/instances')
})
