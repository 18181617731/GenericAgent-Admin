import test from 'node:test'
import assert from 'node:assert/strict'
import { buildRoute, parseRoute } from './routing.js'

const setLocation = (url) => {
  globalThis.window = { location: new URL(url) }
}

test('parseRoute maps aliases and task sub tabs', () => {
  setLocation('http://localhost/goals')
  assert.deepEqual(parseRoute(), { tab: 'tasks', taskSubTab: 'runs' })
  setLocation('http://localhost/tmwd')
  assert.deepEqual(parseRoute(), { tab: 'control', taskSubTab: 'services' })
})

test('parseRoute prefers hash routes', () => {
  setLocation('http://localhost/settings#/tasks/reports')
  assert.deepEqual(parseRoute(), { tab: 'tasks', taskSubTab: 'reports' })
})

test('buildRoute normalizes invalid tabs and task sub tabs', () => {
  assert.equal(buildRoute('missing'), '/overview')
  assert.equal(buildRoute('tasks', 'missing'), '/tasks/services')
})
