import test from 'node:test'
import assert from 'node:assert/strict'
import {
  NOTIFICATION_ITEMS_KEY,
  NOTIFICATION_SETTINGS_KEY,
  browserNotificationCapability,
  clearReadNotifications,
  isNotificationQuietHours,
  loadNotifications,
  loadNotificationSettings,
  markAllNotificationsRead,
  publishNotification,
  registerNotificationServiceWorker,
  requestBrowserNotificationPermission,
  saveNotificationSettings,
} from './notifications.js'

const storage = () => {
  const values = new Map()
  return { getItem: key => values.get(key) ?? null, setItem: (key, value) => values.set(key, String(value)), removeItem: key => values.delete(key) }
}

test('notification settings normalize nested channel and category defaults', () => {
  globalThis.localStorage = storage()
  const settings = saveNotificationSettings({ channels: { browser: true }, categories: { chat: false }, maxItems: 200 })
  assert.equal(settings.channels.inApp, true)
  assert.equal(settings.channels.browser, true)
  assert.equal(settings.categories.chat, false)
  assert.equal(settings.categories.goal, true)
  assert.equal(loadNotificationSettings().maxItems, 200)
})

test('publishing stores unread items and deduplicates by key', () => {
  globalThis.localStorage = storage()
  const first = publishNotification({ category: 'chat', title: '完成', message: '已完成', dedupeKey: 'chat:1' })
  const duplicate = publishNotification({ category: 'chat', title: '重复', message: '不应新增', dedupeKey: 'chat:1' })
  assert.equal(first.id, duplicate.id)
  assert.equal(loadNotifications().length, 1)
  assert.equal(loadNotifications()[0].read, false)
})

test('quiet hours mute delivery without hiding the in-app record', () => {
  globalThis.localStorage = storage()
  saveNotificationSettings({ quietHours: { enabled: true, start: '22:00', end: '08:00' } })
  assert.equal(isNotificationQuietHours(loadNotificationSettings(), new Date('2026-08-05T23:30:00')), true)
  publishNotification({ category: 'system', title: '静默测试', message: '仍应进入收件箱', dedupeKey: 'quiet-test' })
  assert.equal(loadNotifications().length, 1)
})

test('read operations update and remove only the intended records', () => {
  globalThis.localStorage = storage()
  publishNotification({ category: 'system', title: '一', message: '一', dedupeKey: 'one' })
  publishNotification({ category: 'system', title: '二', message: '二', dedupeKey: 'two' })
  markAllNotificationsRead()
  assert.equal(loadNotifications().every(item => item.read), true)
  publishNotification({ category: 'system', title: '三', message: '三', dedupeKey: 'three' })
  clearReadNotifications()
  assert.deepEqual(loadNotifications().map(item => item.dedupeKey), ['three'])
  assert.ok(NOTIFICATION_ITEMS_KEY)
  assert.ok(NOTIFICATION_SETTINGS_KEY)
})

test('browser notification capability explains insecure hosts before requesting permission', async () => {
  const originalNotification = globalThis.Notification
  const originalWindow = globalThis.window
  const originalNavigator = globalThis.navigator
  try {
    Object.defineProperty(globalThis, 'Notification', { configurable: true, value: { permission: 'default', requestPermission: async () => 'granted' } })
    Object.defineProperty(globalThis, 'window', { configurable: true, value: { isSecureContext: false, location: { hostname: '100.92.41.120' } } })
    Object.defineProperty(globalThis, 'navigator', { configurable: true, value: { userAgent: 'Chrome', platform: 'Win32', maxTouchPoints: 0 } })
    assert.equal(browserNotificationCapability().status, 'insecure')
    assert.equal(await requestBrowserNotificationPermission(), 'insecure')
    Object.defineProperty(globalThis, 'Notification', { configurable: true, value: { permission: 'default', requestPermission: async () => 'granted' } })
    Object.defineProperty(globalThis, 'window', { configurable: true, value: { isSecureContext: true, location: { hostname: 'example.com' }, matchMedia: () => ({ matches: false }) } })
    Object.defineProperty(globalThis, 'navigator', { configurable: true, value: { userAgent: 'Mozilla/5.0 (iPhone; CPU iPhone OS 18_0 like Mac OS X)', platform: 'iPhone', maxTouchPoints: 5 } })
    assert.equal(browserNotificationCapability().status, 'ios-browser')
    assert.equal(await requestBrowserNotificationPermission(), 'ios-browser')
  } finally {
    if (originalNotification === undefined) delete globalThis.Notification
    else Object.defineProperty(globalThis, 'Notification', { configurable: true, value: originalNotification })
    if (originalWindow === undefined) delete globalThis.window
    else Object.defineProperty(globalThis, 'window', { configurable: true, value: originalWindow })
    if (originalNavigator === undefined) delete globalThis.navigator
    else Object.defineProperty(globalThis, 'navigator', { configurable: true, value: originalNavigator })
  }
})

test('registers a notification service worker on secure browser pages', async () => {
  const originalWindow = globalThis.window
  const originalNavigator = globalThis.navigator
  const registration = { showNotification: async () => {} }
  try {
    Object.defineProperty(globalThis, 'window', { configurable: true, value: { isSecureContext: true } })
    Object.defineProperty(globalThis, 'navigator', { configurable: true, value: { serviceWorker: { register: async (path, options) => {
      assert.equal(path, '/sw.js')
      assert.deepEqual(options, { scope: '/' })
      return registration
    }, ready: Promise.resolve(registration) } } })
    assert.equal(await registerNotificationServiceWorker(), registration)
  } finally {
    if (originalWindow === undefined) delete globalThis.window
    else Object.defineProperty(globalThis, 'window', { configurable: true, value: originalWindow })
    if (originalNavigator === undefined) delete globalThis.navigator
    else Object.defineProperty(globalThis, 'navigator', { configurable: true, value: originalNavigator })
  }
})
