import test from 'node:test'
import assert from 'node:assert/strict'
import { groupRecentSessions, sessionAge } from './chatSessionGroups.js'

const seconds = (year, month, day, hour = 12) => Math.floor(new Date(year, month, day, hour).getTime() / 1000)

test('groupRecentSessions prioritizes pinned chats and uses local calendar boundaries', () => {
  const now = new Date(2026, 7, 20, 15, 30)
  const sessions = [
    { id:'pinned', pinned:true, updated_at:seconds(2026, 4, 1) },
    { id:'today', updated_at:seconds(2026, 7, 20) },
    { id:'yesterday', updated_at:seconds(2026, 7, 19) },
    { id:'this-week', updated_at:seconds(2026, 7, 17) },
    { id:'last-week', updated_at:seconds(2026, 7, 12) },
    { id:'this-month', updated_at:seconds(2026, 7, 2) },
    { id:'older', updated_at:seconds(2026, 6, 31) },
  ]

  const groups = groupRecentSessions(sessions, now)
  assert.deepEqual(groups.map(group => group.key), ['pinned', 'today', 'yesterday', 'this_week', 'last_week', 'this_month', 'older'])
  assert.deepEqual(groups.map(group => group.sessions.map(session => session.id)), sessions.map(session => [session.id]))
})

test('groupRecentSessions keeps invalid timestamps in older instead of dropping chats', () => {
  const groups = groupRecentSessions([{ id:'invalid', updated_at:'' }], new Date(2026, 7, 20))
  assert.deepEqual(groups.map(group => [group.key, group.sessions[0].id]), [['older', 'invalid']])
})

test('sessionAge reports the largest unit that still counts', () => {
  const now = new Date(2026, 7, 20, 15, 30).getTime()
  const ago = (ms) => sessionAge(Math.floor((now - ms) / 1000), now)
  assert.deepEqual(ago(0), { unit:'now', value:0 })
  assert.deepEqual(ago(59000), { unit:'now', value:0 })
  assert.deepEqual(ago(60000), { unit:'minute', value:1 })
  assert.deepEqual(ago(59 * 60000), { unit:'minute', value:59 })
  assert.deepEqual(ago(3600000), { unit:'hour', value:1 })
  assert.deepEqual(ago(23 * 3600000), { unit:'hour', value:23 })
  assert.deepEqual(ago(86400000), { unit:'day', value:1 })
  assert.deepEqual(ago(6 * 86400000), { unit:'day', value:6 })
  assert.deepEqual(ago(7 * 86400000), { unit:'week', value:1 })
  assert.deepEqual(ago(29 * 86400000), { unit:'week', value:4 })
  assert.deepEqual(ago(30 * 86400000), { unit:'month', value:1 })
  assert.deepEqual(ago(364 * 86400000), { unit:'month', value:12 })
  assert.deepEqual(ago(365 * 86400000), { unit:'year', value:1 })
  assert.deepEqual(ago(900 * 86400000), { unit:'year', value:2 })
})

test('sessionAge reads a clock that runs ahead as just now, and no timestamp as nothing', () => {
  const now = new Date(2026, 7, 20, 15, 30).getTime()
  assert.deepEqual(sessionAge(Math.floor(now / 1000) + 600, now), { unit:'now', value:0 })
  assert.equal(sessionAge('', now), null)
  assert.equal(sessionAge(undefined, now), null)
})
