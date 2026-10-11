export const UPDATE_NOTIFICATION_KEY = 'ga-admin-update-notifications'
export const UPDATE_NOTIFICATION_EVENT = 'ga-admin-update-notifications-change'
export const UPDATE_CHECK_CACHE_KEY = 'ga-admin-update-check-cache'
export const UPDATE_CHECK_INTERVAL_MS = 6 * 60 * 60 * 1000
export const UPDATE_CHECK_RETRY_MS = 30 * 60 * 1000
export const UPDATE_CHECK_TIMEOUT_MS = 30 * 1000

let memoryPreferences = { enabled: true, dismissedVersion: '' }
let memoryCache = null
const preferenceFallback = new WeakMap()
const cacheFallback = new WeakMap()
const browserStorage = () => { try { return window.localStorage } catch { return null } }
export const normalizeNotificationPreferences = value => ({
  enabled: value?.enabled !== false,
  dismissedVersion: typeof value?.dismissedVersion === 'string' ? value.dismissedVersion : '',
})

export function readNotificationPreferences(storage = browserStorage()) {
  if (!storage) return memoryPreferences
  if (preferenceFallback.has(storage)) return preferenceFallback.get(storage)
  try { return normalizeNotificationPreferences(JSON.parse(storage.getItem(UPDATE_NOTIFICATION_KEY) || 'null')) }
  catch { return memoryPreferences }
}

export function saveNotificationPreferences(patch, storage = browserStorage()) {
  memoryPreferences = normalizeNotificationPreferences({ ...readNotificationPreferences(storage), ...patch })
  if (storage) {
    try { storage.setItem(UPDATE_NOTIFICATION_KEY, JSON.stringify(memoryPreferences)); preferenceFallback.delete(storage) }
    catch { preferenceFallback.set(storage, memoryPreferences) }
  }
  if (typeof window !== 'undefined') window.dispatchEvent(new Event(UPDATE_NOTIFICATION_EVENT))
  return memoryPreferences
}

const normalizeVersion = value => String(value || '').trim().replace(/^v/i, '')
export const notificationRelease = (check, preferences, currentVersion) => {
  const tag = check?.latest?.tag_name
  if (!preferences.enabled || check?.update !== true || typeof tag !== 'string' || !tag.trim()) return ''
  if (normalizeVersion(tag) === normalizeVersion(currentVersion) || normalizeVersion(tag) === normalizeVersion(preferences.dismissedVersion)) return ''
  return tag
}

export function readUpdateCheckCache(storage = browserStorage()) {
  if (!storage) return memoryCache
  if (cacheFallback.has(storage)) return cacheFallback.get(storage)
  try { return JSON.parse(storage.getItem(UPDATE_CHECK_CACHE_KEY) || 'null') }
  catch { return memoryCache }
}

export function saveUpdateCheckCache(currentVersion, check, now = Date.now(), storage = browserStorage()) {
  memoryCache = { currentVersion, check, checkedAt: now }
  if (storage) {
    try { storage.setItem(UPDATE_CHECK_CACHE_KEY, JSON.stringify(memoryCache)); cacheFallback.delete(storage) }
    catch { cacheFallback.set(storage, memoryCache) }
  }
  return memoryCache
}

export const updateCheckCacheRemaining = (cache, currentVersion, now = Date.now()) => {
  if (!cache || cache.currentVersion !== currentVersion || !Number.isFinite(cache.checkedAt) || cache.checkedAt > now) return 0
  if (cache.check !== null && typeof cache.check?.update !== 'boolean') return 0
  const interval = cache.check ? UPDATE_CHECK_INTERVAL_MS : UPDATE_CHECK_RETRY_MS
  return Math.max(0, cache.checkedAt + interval - now)
}
