import { playChatCompletionTone } from './chatCompletionTone.js'
import { chatNotificationForDisplay } from './chatPrivacy.js'

export const NOTIFICATION_SETTINGS_KEY = 'ga-admin-notification-settings'
export const NOTIFICATION_ITEMS_KEY = 'ga-admin-notifications'
export const NOTIFICATION_EVENT = 'ga-admin-notification-change'

export const NOTIFICATION_CATEGORIES = [
  { id: 'chat', label: '对话', description: '对话完成、失败或被中止' },
  { id: 'schedule', label: '定时任务', description: '定时任务完成或执行失败' },
  { id: 'autonomous', label: '自主进化', description: '自主执行完成、失败或出现待审批项' },
  { id: 'goal', label: 'Goal 模式', description: 'Goal 运行完成或失败' },
  { id: 'system', label: '系统', description: '通知设置测试和重要运行提示' },
]

const DEFAULT_SETTINGS = {
  enabled: true,
  channels: { inApp: true, browser: false, sound: true, backgroundOnly: true },
  quietHours: { enabled: false, start: '22:00', end: '08:00' },
  categories: Object.fromEntries(NOTIFICATION_CATEGORIES.map(item => [item.id, true])),
  maxItems: 100,
}

let notificationSequence = 0

const getStorage = () => typeof localStorage !== 'undefined' ? localStorage : null
const clone = value => JSON.parse(JSON.stringify(value))
const emitChange = detail => {
  if (typeof window !== 'undefined') window.dispatchEvent(new CustomEvent(NOTIFICATION_EVENT, { detail }))
}

export const normalizeNotificationSettings = (raw = {}) => {
  const source = raw && typeof raw === 'object' ? raw : {}
  const channels = source.channels && typeof source.channels === 'object' ? source.channels : {}
  const quietHours = source.quietHours && typeof source.quietHours === 'object' ? source.quietHours : {}
  const categories = source.categories && typeof source.categories === 'object' ? source.categories : {}
  const maxItems = Number(source.maxItems)
  return {
    enabled: source.enabled !== false,
    channels: {
      inApp: channels.inApp !== false,
      browser: channels.browser === true,
      sound: channels.sound !== false,
      backgroundOnly: channels.backgroundOnly !== false,
    },
    quietHours: {
      enabled: quietHours.enabled === true,
      start: /^\d{2}:\d{2}$/.test(String(quietHours.start || '')) ? quietHours.start : DEFAULT_SETTINGS.quietHours.start,
      end: /^\d{2}:\d{2}$/.test(String(quietHours.end || '')) ? quietHours.end : DEFAULT_SETTINGS.quietHours.end,
    },
    categories: Object.fromEntries(NOTIFICATION_CATEGORIES.map(item => [item.id, categories[item.id] !== false])),
    maxItems: [50, 100, 200].includes(maxItems) ? maxItems : DEFAULT_SETTINGS.maxItems,
  }
}

export const loadNotificationSettings = () => {
  const storage = getStorage()
  if (!storage) return clone(DEFAULT_SETTINGS)
  try { return normalizeNotificationSettings(JSON.parse(storage.getItem(NOTIFICATION_SETTINGS_KEY) || '{}')) } catch { return clone(DEFAULT_SETTINGS) }
}

export const saveNotificationSettings = next => {
  const normalized = normalizeNotificationSettings(next)
  const storage = getStorage()
  try { storage?.setItem(NOTIFICATION_SETTINGS_KEY, JSON.stringify(normalized)) } catch {}
  emitChange({ kind: 'settings', settings: normalized })
  return normalized
}

const normalizeItem = item => ({
  id: String(item?.id || ''),
  category: String(item?.category || 'system'),
  level: ['success', 'info', 'warning', 'error'].includes(item?.level) ? item.level : 'info',
  title: String(item?.title || '消息通知'),
  message: String(item?.message || ''),
  createdAt: String(item?.createdAt || new Date().toISOString()),
  read: item?.read === true,
  route: String(item?.route || 'notifications'),
  subtab: String(item?.subtab || ''),
  dedupeKey: String(item?.dedupeKey || ''),
})

export const loadNotifications = () => {
  const storage = getStorage()
  if (!storage) return []
  try {
    const parsed = JSON.parse(storage.getItem(NOTIFICATION_ITEMS_KEY) || '[]')
    return Array.isArray(parsed) ? parsed.map(normalizeItem).filter(item => item.id) : []
  } catch { return [] }
}

export const unreadNotificationCount = (items = loadNotifications()) => items.filter(item => !item.read).length

export const notificationCategoryLabel = (category, lang = 'zh') => {
  const item = NOTIFICATION_CATEGORIES.find(entry => entry.id === category)
  if (!item) return lang === 'en' ? 'System' : '系统'
  if (lang !== 'en') return item.label
  return { chat: 'Chat', schedule: 'Scheduled task', autonomous: 'Autonomous', goal: 'Goal Mode', system: 'System' }[item.id] || item.label
}

export const notificationLevelLabel = (level, lang = 'zh') => {
  if (lang === 'en') return { success: 'Completed', info: 'Information', warning: 'Needs attention', error: 'Failed' }[level] || 'Information'
  return { success: '已完成', info: '提示', warning: '需要关注', error: '失败' }[level] || '提示'
}

export const formatNotificationTime = (value, lang = 'zh') => {
  const date = new Date(value)
  if (Number.isNaN(date.getTime())) return lang === 'en' ? 'Unknown time' : '时间未知'
  return date.toLocaleString(lang === 'en' ? 'en-US' : 'zh-CN', { dateStyle: 'medium', timeStyle: 'short' })
}

const saveNotifications = items => {
  const settings = loadNotificationSettings()
  const next = items.slice(0, settings.maxItems)
  try { getStorage()?.setItem(NOTIFICATION_ITEMS_KEY, JSON.stringify(next)) } catch {}
  return next
}

const timeToMinutes = value => {
  const match = String(value || '').match(/^(\d{2}):(\d{2})$/)
  if (!match) return 0
  return Math.min(1439, Number(match[1]) * 60 + Number(match[2]))
}

export const isNotificationQuietHours = (settings = loadNotificationSettings(), date = new Date()) => {
  if (!settings.quietHours.enabled) return false
  const current = date.getHours() * 60 + date.getMinutes()
  const start = timeToMinutes(settings.quietHours.start)
  const end = timeToMinutes(settings.quietHours.end)
  if (start === end) return false
  return start < end ? current >= start && current < end : current >= start || current < end
}

const isLocalNotificationHost = hostname => ['localhost', '127.0.0.1', '::1', '[::1]'].includes(String(hostname || '').toLowerCase())

const isIosBrowserTab = () => {
  if (typeof navigator === 'undefined') return false
  const userAgent = String(navigator.userAgent || '')
  const isIos = /iPad|iPhone|iPod/.test(userAgent) || (navigator.platform === 'MacIntel' && navigator.maxTouchPoints > 1)
  if (!isIos || typeof window === 'undefined') return false
  const standalone = Boolean(navigator.standalone) || Boolean(window.matchMedia?.('(display-mode: standalone)').matches)
  return !standalone
}

export const browserNotificationCapability = () => {
  const iosBrowser = isIosBrowserTab()
  const hasApi = typeof Notification !== 'undefined' && typeof Notification.requestPermission === 'function'
  const permission = typeof Notification === 'undefined' ? 'unsupported' : Notification.permission || 'default'
  if (!hasApi) return { status: iosBrowser ? 'ios-browser' : 'unsupported', permission }
  if (typeof window !== 'undefined' && window.isSecureContext === false && !isLocalNotificationHost(window.location?.hostname)) {
    return { status: 'insecure', permission }
  }
  if (iosBrowser) return { status: 'ios-browser', permission }
  return { status: 'available', permission }
}

export const browserNotificationPermission = () => browserNotificationCapability().permission

export const requestBrowserNotificationPermission = async () => {
  const capability = browserNotificationCapability()
  if (capability.status !== 'available') return capability.status
  try { return await Notification.requestPermission() } catch { return 'denied' }
}

export const notificationPath = item => {
  const route = item.route.startsWith('/') ? item.route : `/${item.route}`
  return item.subtab ? `${route}/${item.subtab}` : route
}

const notificationOptions = item => ({
  body: item.message,
  tag: item.dedupeKey || item.id,
  data: { path: notificationPath(item) },
  renotify: true,
  silent: false,
})

const notificationLanguage = () => {
  try { return localStorage.getItem('ga-admin-lang') === 'en' ? 'en' : 'zh' } catch { return 'zh' }
}

const showWindowNotification = item => {
  try {
    const notification = new Notification(item.title, notificationOptions(item))
    notification.onclick = () => {
      window.focus?.()
      window.location.href = notificationPath(item)
    }
  } catch {}
}

export const registerNotificationServiceWorker = () => {
  if (typeof window === 'undefined' || typeof navigator === 'undefined' || window.isSecureContext === false || !navigator.serviceWorker?.register) return Promise.resolve(null)
  return navigator.serviceWorker.register('/sw.js', { scope: '/' })
    .then(() => navigator.serviceWorker.ready)
    .catch(() => null)
}

const deliverBrowserNotification = (item, settings) => {
  if (!settings.channels.browser || browserNotificationPermission() !== 'granted') return
  if (settings.channels.backgroundOnly && typeof document !== 'undefined' && document.visibilityState !== 'hidden') return
  const displayItem = chatNotificationForDisplay(item, undefined, notificationLanguage())
  const ready = typeof navigator === 'undefined' ? null : navigator.serviceWorker?.ready
  if (!ready?.then) {
    showWindowNotification(displayItem)
    return
  }
  ready.then(registration => {
    if (typeof registration.showNotification !== 'function') {
      showWindowNotification(displayItem)
      return null
    }
    return registration.showNotification(displayItem.title, notificationOptions(displayItem)).catch(() => showWindowNotification(displayItem))
  }).catch(() => showWindowNotification(displayItem))
}

export const publishNotification = (input = {}, options = {}) => {
  const settings = loadNotificationSettings()
  const category = NOTIFICATION_CATEGORIES.some(item => item.id === input.category) ? input.category : 'system'
  if (!settings.enabled || settings.categories[category] === false) return null
  const dedupeKey = String(input.dedupeKey || '')
  const items = loadNotifications()
  const existing = items.find(item => (input.id && item.id === input.id) || (dedupeKey && item.dedupeKey === dedupeKey))
  if (existing) return existing
  const item = normalizeItem({
    ...input,
    id: String(input.id || `notification-${Date.now()}-${notificationSequence++}`),
    category,
    dedupeKey,
  })
  if (settings.channels.inApp) saveNotifications([item, ...items])
  const quiet = isNotificationQuietHours(settings)
  if (!quiet && settings.channels.sound && options.playSound !== false) playChatCompletionTone()
  if (!quiet) deliverBrowserNotification(item, settings)
  if (settings.channels.inApp) emitChange({ kind: 'item', item })
  return item
}

const updateItems = updater => {
  const next = saveNotifications(updater(loadNotifications()))
  emitChange({ kind: 'items', items: next })
  return next
}

export const markNotificationRead = id => updateItems(items => items.map(item => item.id === id ? { ...item, read: true } : item))
export const markAllNotificationsRead = () => updateItems(items => items.map(item => ({ ...item, read: true })))
export const clearReadNotifications = () => updateItems(items => items.filter(item => !item.read))

export const subscribeNotifications = listener => {
  if (typeof window === 'undefined') return () => {}
  const onChange = event => listener(event.detail || { kind: 'changed' })
  const onStorage = event => { if (event.key === NOTIFICATION_ITEMS_KEY || event.key === NOTIFICATION_SETTINGS_KEY) listener({ kind: 'changed' }) }
  window.addEventListener(NOTIFICATION_EVENT, onChange)
  window.addEventListener('storage', onStorage)
  return () => { window.removeEventListener(NOTIFICATION_EVENT, onChange); window.removeEventListener('storage', onStorage) }
}
