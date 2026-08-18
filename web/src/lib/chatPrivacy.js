export const CHAT_PRIVACY_STORAGE_KEY = 'ga-admin-chat-privacy-mode'
export const CHAT_PRIVACY_EVENT = 'ga-admin-chat-privacy-change'

const availableStorage = storage => {
  if (storage) return storage
  if (typeof window === 'undefined') return null
  try { return window.localStorage } catch { return null }
}

export const normalizeChatPrivacyMode = value => value === true || value === 'true'

export const loadChatPrivacyMode = (storage = null) => {
  const target = availableStorage(storage)
  if (!target) return false
  try { return normalizeChatPrivacyMode(target.getItem(CHAT_PRIVACY_STORAGE_KEY)) } catch { return false }
}

export const saveChatPrivacyMode = (enabled, { storage = null, windowRef = typeof window !== 'undefined' ? window : null } = {}) => {
  const value = Boolean(enabled)
  try { availableStorage(storage)?.setItem(CHAT_PRIVACY_STORAGE_KEY, String(value)) } catch {}
  try { windowRef?.dispatchEvent(new CustomEvent(CHAT_PRIVACY_EVENT, { detail: { enabled:value } })) } catch {}
  return value
}

export const subscribeChatPrivacyMode = (listener, { windowRef = typeof window !== 'undefined' ? window : null } = {}) => {
  if (!windowRef?.addEventListener) return () => {}
  const onPrivacyChange = event => listener(Boolean(event?.detail?.enabled))
  const onStorage = event => {
    if (event.key === CHAT_PRIVACY_STORAGE_KEY) listener(normalizeChatPrivacyMode(event.newValue))
  }
  windowRef.addEventListener(CHAT_PRIVACY_EVENT, onPrivacyChange)
  windowRef.addEventListener('storage', onStorage)
  return () => {
    windowRef.removeEventListener(CHAT_PRIVACY_EVENT, onPrivacyChange)
    windowRef.removeEventListener('storage', onStorage)
  }
}

export const privateSessionTitle = (index = 0, lang = 'zh') => {
  const number = String(Math.max(0, Number(index) || 0) + 1).padStart(2, '0')
  return lang === 'en' ? `Private chat ${number}` : `隐私会话 ${number}`
}

export const privateProjectTitle = (index = 0, lang = 'zh') => {
  const number = String(Math.max(0, Number(index) || 0) + 1).padStart(2, '0')
  return lang === 'en' ? `Private project ${number}` : `隐私项目 ${number}`
}

export const chatNotificationForDisplay = (item, privacyMode = loadChatPrivacyMode(), lang = 'zh') => {
  if (!privacyMode || item?.category !== 'chat') return item
  const level = String(item?.level || 'info')
  const title = lang === 'en'
    ? ({ success:'Chat task completed', error:'Chat task failed', warning:'Chat task stopped' }[level] || 'Chat task updated')
    : ({ success:'对话任务已完成', error:'对话任务失败', warning:'对话任务已停止' }[level] || '对话任务状态更新')
  return {
    ...item,
    title,
    message: lang === 'en' ? 'Privacy mode has hidden the chat title and content.' : '隐私模式已隐藏会话标题和内容。',
  }
}
