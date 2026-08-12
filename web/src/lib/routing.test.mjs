import test from 'node:test'
import assert from 'node:assert/strict'
import { buildRoute, parseRoute } from './routing.js'

const setLocation = (url) => {
  globalThis.window = { location: new URL(url) }
}

test('parseRoute maps aliases and task sub tabs', () => {
  setLocation('http://localhost/admin/tasks/goals')
  assert.deepEqual(parseRoute(), { tab: 'tasks', taskSubTab: 'runs' })
  setLocation('http://localhost/admin/tmwd')
  assert.deepEqual(parseRoute(), { tab: 'overview', taskSubTab: 'services' })
})

test('goal mode round-trips its own route instead of collapsing into tasks', () => {
  setLocation('http://localhost/admin/goals')
  assert.deepEqual(parseRoute(), { tab: 'goals', taskSubTab: 'services' })
  assert.equal(buildRoute('goals'), '/admin/goals')
})

test('parseRoute strips the /admin mount prefix', () => {
  setLocation('http://localhost/admin')
  assert.deepEqual(parseRoute(), { tab: 'overview', taskSubTab: 'services' })
  setLocation('http://localhost/admin/tasks/reports')
  assert.deepEqual(parseRoute(), { tab: 'tasks', taskSubTab: 'reports' })
})

test('/admin/chat is the chat settings page (the chat itself lives at /)', () => {
  setLocation('http://localhost/admin/chat')
  assert.deepEqual(parseRoute(), { tab: 'chat', taskSubTab: 'services' })
})

test('legacy config and about paths resolve to their settings pages', () => {
  setLocation('http://localhost/admin/config')
  assert.deepEqual(parseRoute(), { tab: 'settings', taskSubTab: 'services' })
  setLocation('http://localhost/admin/about')
  assert.deepEqual(parseRoute(), { tab: 'overview', taskSubTab: 'services' })
})

test('parseRoute prefers hash routes', () => {
  setLocation('http://localhost/admin/settings#/tasks/reports')
  assert.deepEqual(parseRoute(), { tab: 'tasks', taskSubTab: 'reports' })
})

test('buildRoute normalizes invalid tabs and task sub tabs', () => {
  assert.equal(buildRoute('missing'), '/admin/overview')
  assert.equal(buildRoute('tasks', 'missing'), '/admin/tasks/services')
})

test('usage overview has a stable refreshable route', () => {
  setLocation('http://localhost/admin/usage')
  assert.deepEqual(parseRoute(), { tab: 'usage', taskSubTab: 'services' })
  assert.equal(buildRoute('usage'), '/admin/usage')
})

test('GA instances has a stable refreshable route', () => {
  setLocation('http://localhost/admin/instances')
  assert.deepEqual(parseRoute(), { tab: 'instances', taskSubTab: 'services' })
  assert.equal(buildRoute('instances'), '/admin/instances')
})
