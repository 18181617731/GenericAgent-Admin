import test from 'node:test'
import assert from 'node:assert/strict'
import { groupProjectSessions } from './chatProjectSessions.js'

test('groupProjectSessions keeps project order and includes projects without sessions', () => {
  const sessions = [
    { id: 'general', project_mode: '' },
    { id: 'beta-1', project_mode: 'Beta' },
    { id: 'alpha-1', project_mode: 'Alpha' },
    { id: 'removed', project_mode: 'Removed' },
    { id: 'alpha-2', project_mode: 'Alpha' },
  ]

  assert.deepEqual(groupProjectSessions(['Beta', 'Empty', 'Alpha'], sessions), [
    { name: 'Beta', pinned: false, sessions: [sessions[1]] },
    { name: 'Empty', pinned: false, sessions: [] },
    { name: 'Alpha', pinned: false, sessions: [sessions[2], sessions[4]] },
  ])
})

test('groupProjectSessions ignores duplicate and blank project names', () => {
  assert.deepEqual(groupProjectSessions(['Alpha', '', 'Alpha', '  ', null], []), [
    { name: 'Alpha', pinned: false, sessions: [] },
  ])
})

test('groupProjectSessions tolerates malformed API collections', () => {
  assert.deepEqual(groupProjectSessions(null, null), [])
  assert.deepEqual(groupProjectSessions(['Alpha'], [], null), [
    { name: 'Alpha', pinned: false, sessions: [] },
  ])
})

test('pinned projects are marked and moved to the top', () => {
  const groups = groupProjectSessions(['Alpha', 'Beta', 'Gamma'], [], ['Gamma', 'Beta'])
  assert.deepEqual(groups.map(g => [g.name, g.pinned]), [
    ['Beta', true],
    ['Gamma', true],
    ['Alpha', false],
  ])
})

// Within each half the server's order is preserved, so pinning one project does
// not shuffle the rest of the list.
test('pinning preserves the relative order inside the pinned and unpinned halves', () => {
  const groups = groupProjectSessions(['d', 'c', 'b', 'a'], [], ['c', 'a'])
  assert.deepEqual(groups.map(g => g.name), ['c', 'a', 'd', 'b'])
})

test('a pin for a project that no longer exists is ignored', () => {
  const groups = groupProjectSessions(['Alpha'], [], ['Removed'])
  assert.deepEqual(groups, [{ name: 'Alpha', pinned: false, sessions: [] }])
})
