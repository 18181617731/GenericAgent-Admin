import test from 'node:test'
import assert from 'node:assert/strict'
import {
  UPDATE_NOTIFICATION_KEY, UPDATE_CHECK_INTERVAL_MS, UPDATE_CHECK_RETRY_MS,
  normalizeNotificationPreferences, readNotificationPreferences, saveNotificationPreferences,
  notificationRelease, readUpdateCheckCache, saveUpdateCheckCache, updateCheckCacheRemaining,
} from './updateNotifications.js'
const storage = () => {
  const data = new Map()
  return { getItem: key => data.get(key) ?? null, setItem: (key, value) => data.set(key, value) }
}
const candidate = { update: true, latest: { tag_name: 'v1.2.3' } }

test('reminders default on and malformed preferences are safe', () => {
  const s = storage()
  assert.deepEqual(readNotificationPreferences(s), { enabled: true, dismissedVersion: '' })
  s.setItem(UPDATE_NOTIFICATION_KEY, '{broken')
  assert.equal(readNotificationPreferences(s).enabled, true)
  assert.deepEqual(normalizeNotificationPreferences({ enabled: false, dismissedVersion: 42 }), { enabled: false, dismissedVersion: '' })
})
test('disable and dismissal persist independently and repeated writes are idempotent', () => {
  const s = storage()
  saveNotificationPreferences({ dismissedVersion: 'v1.2.3' }, s)
  saveNotificationPreferences({ enabled: false }, s)
  const before = s.getItem(UPDATE_NOTIFICATION_KEY)
  saveNotificationPreferences({ enabled: false }, s)
  assert.equal(s.getItem(UPDATE_NOTIFICATION_KEY), before)
  assert.deepEqual(readNotificationPreferences(s), { enabled: false, dismissedVersion: 'v1.2.3' })
  saveNotificationPreferences({ enabled: true }, s)
  assert.equal(readNotificationPreferences(s).dismissedVersion, 'v1.2.3')
})
test('only a new, enabled, non-dismissed release is announced', () => {
  const enabled = { enabled: true, dismissedVersion: '' }
  assert.equal(notificationRelease(candidate, enabled, 'v1.2.2'), 'v1.2.3')
  for (const check of [null, {}, { update: false, latest: candidate.latest }, { update: true }, { update: true, latest: { tag_name: ' ' } }]) assert.equal(notificationRelease(check, enabled, 'v1.2.2'), '')
  assert.equal(notificationRelease(candidate, { ...enabled, enabled: false }, 'v1.2.2'), '')
  assert.equal(notificationRelease(candidate, { ...enabled, dismissedVersion: '1.2.3' }, 'v1.2.2'), '')
  assert.equal(notificationRelease(candidate, enabled, '1.2.3'), '')
  assert.equal(notificationRelease({ ...candidate, latest: { tag_name: 'v1.2.4' } }, { ...enabled, dismissedVersion: 'v1.2.3' }, 'v1.2.2'), 'v1.2.4')
})
test('check cache survives reloads but expires, rejects future timestamps, and invalidates after upgrade', () => {
  const s = storage(), now = 10000
  saveUpdateCheckCache('v1.2.2', candidate, now, s)
  const cached = readUpdateCheckCache(s)
  assert.equal(updateCheckCacheRemaining(cached, 'v1.2.2', now + 1000), UPDATE_CHECK_INTERVAL_MS - 1000)
  assert.equal(updateCheckCacheRemaining(cached, 'v1.2.2', now + UPDATE_CHECK_INTERVAL_MS), 0)
  assert.equal(updateCheckCacheRemaining(cached, 'v1.2.3', now), 0)
  assert.equal(updateCheckCacheRemaining(cached, 'v1.2.2', now - 1), 0)
  assert.equal(updateCheckCacheRemaining({ ...cached, check: {} }, 'v1.2.2', now), 0)
})
test('storage denial preserves controls and cache in memory', () => {
  const denied = { getItem: () => null, setItem: () => { throw new Error('denied') } }
  saveNotificationPreferences({ enabled: false, dismissedVersion: 'v1.2.3' }, denied)
  assert.deepEqual(readNotificationPreferences(denied), { enabled: false, dismissedVersion: 'v1.2.3' })
  const cached = saveUpdateCheckCache('v1.2.2', candidate, 10000, denied)
  assert.deepEqual(readUpdateCheckCache(denied), cached)
  saveNotificationPreferences({ enabled: true }, null)
  assert.equal(readNotificationPreferences(null).enabled, true)
  assert.deepEqual(readUpdateCheckCache(null), cached)
})
test('failed checks use a bounded retry delay', () => {
  const s = storage()
  const cached = saveUpdateCheckCache('v1.2.2', null, 10000, s)
  assert.equal(updateCheckCacheRemaining(cached, 'v1.2.2', 10000), UPDATE_CHECK_RETRY_MS)
  assert.equal(updateCheckCacheRemaining(cached, 'v1.2.2', 10000 + UPDATE_CHECK_RETRY_MS), 0)
})
