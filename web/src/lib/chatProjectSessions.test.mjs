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
    { name: 'Beta', sessions: [sessions[1]] },
    { name: 'Empty', sessions: [] },
    { name: 'Alpha', sessions: [sessions[2], sessions[4]] },
  ])
})

test('groupProjectSessions ignores duplicate and blank project names', () => {
  assert.deepEqual(groupProjectSessions(['Alpha', '', 'Alpha', '  ', null], []), [
    { name: 'Alpha', sessions: [] },
  ])
})

test('groupProjectSessions tolerates malformed API collections', () => {
  assert.deepEqual(groupProjectSessions(null, null), [])
})
