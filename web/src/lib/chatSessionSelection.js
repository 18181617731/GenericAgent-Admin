const STORAGE_KEY = 'ga-admin-chat-session-selection-v1'
const DEFAULT_INSTANCE_KEY = '__default__'

function availableStorage(storage) {
  if (storage) return storage
  if (typeof window === 'undefined') return null
  try {
    return window.sessionStorage
  } catch {
    return null
  }
}

function instanceKey(instanceID) {
  return String(instanceID || '').trim() || DEFAULT_INSTANCE_KEY
}

function readSelections(storage) {
  const target = availableStorage(storage)
  if (!target) return {}
  try {
    const parsed = JSON.parse(target.getItem(STORAGE_KEY) || '{}')
    if (!parsed || typeof parsed !== 'object' || Array.isArray(parsed)) return {}
    return Object.fromEntries(Object.entries(parsed).filter(([key, value]) => key && typeof value === 'string' && value.trim()))
  } catch {
    return {}
  }
}

function writeSelections(selections, storage) {
  const target = availableStorage(storage)
  if (!target) return
  try {
    if (Object.keys(selections).length) target.setItem(STORAGE_KEY, JSON.stringify(selections))
    else target.removeItem(STORAGE_KEY)
  } catch {
    // Remembering the selected session is best-effort when storage is unavailable.
  }
}

export function loadSelectedChatSessionID(instanceID, storage) {
  return readSelections(storage)[instanceKey(instanceID)] || ''
}

export function persistSelectedChatSessionID(instanceID, sessionID, storage) {
  const selections = readSelections(storage)
  const key = instanceKey(instanceID)
  const id = String(sessionID || '').trim()
  if (id) selections[key] = id
  else delete selections[key]
  writeSelections(selections, storage)
}

export function chooseChatSessionID(sessions, preferredID = '', restoredID = '') {
  const list = Array.isArray(sessions) ? sessions : []
  const ids = new Set(list.map(session => String(session?.id || '').trim()).filter(Boolean))
  const preferred = String(preferredID || '').trim()
  const restored = String(restoredID || '').trim()
  if (preferred && ids.has(preferred)) return preferred
  if (restored && ids.has(restored)) return restored
  return String(list[0]?.id || '').trim()
}

export { STORAGE_KEY as CHAT_SESSION_SELECTION_STORAGE_KEY }
