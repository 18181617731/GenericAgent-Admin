import test from 'node:test'
import assert from 'node:assert/strict'
import { hubSessions } from './chatHubSessions.js'

test('hubSessions keeps only joined Hub sessions and supports title search', () => {
  const sessions = [
    { id:'a', title:'Alpha', hub_enabled:true },
    { id:'b', title:'Beta', hub_enabled:false },
    { id:'c', title:'Hub Notes', hub_enabled:true },
  ]
  assert.deepEqual(hubSessions(sessions).map((session) => session.id), ['a', 'c'])
  assert.deepEqual(hubSessions(sessions, ' notes ').map((session) => session.id), ['c'])
})

test('hubSessions tolerates malformed inputs', () => {
  assert.deepEqual(hubSessions(null), [])
  assert.deepEqual(hubSessions([null, { id:'a', hub_enabled:true }], 'missing'), [])
})
