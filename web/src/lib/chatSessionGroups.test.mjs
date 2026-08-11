import test from 'node:test'
import assert from 'node:assert/strict'
import { groupRecentSessions } from './chatSessionGroups.js'

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
