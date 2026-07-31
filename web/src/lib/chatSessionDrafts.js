const STORAGE_KEY = 'ga-admin-chat-session-drafts-v1'

function availableStorage(storage) {
  if (storage) return storage
  if (typeof window === 'undefined') return null
  try {
    return window.localStorage
  } catch {
    return null
  }
}

function readDraftMap(storage) {
  const target = availableStorage(storage)
  if (!target) return {}
  try {
    const parsed = JSON.parse(target.getItem(STORAGE_KEY) || '{}')
    if (!parsed || typeof parsed !== 'object' || Array.isArray(parsed)) return {}
    return Object.fromEntries(Object.entries(parsed).filter(([id, value]) => id && typeof value === 'string' && value))
  } catch {
    return {}
  }
}

function writeDraftMap(drafts, storage) {
  const target = availableStorage(storage)
  if (!target) return
  try {
    if (Object.keys(drafts).length) target.setItem(STORAGE_KEY, JSON.stringify(drafts))
    else target.removeItem(STORAGE_KEY)
  } catch {
    // Draft persistence is best-effort when storage is unavailable or full.
  }
}

export function listChatSessionDraftIds(storage) {
  return Object.keys(readDraftMap(storage))
}

export function loadChatSessionDraft(sessionId, storage) {
  const id = String(sessionId || '').trim()
  if (!id) return ''
  return readDraftMap(storage)[id] || ''
}

export function saveChatSessionDraft(sessionId, value, storage) {
  const id = String(sessionId || '').trim()
  if (!id) return
  const drafts = readDraftMap(storage)
  const draft = typeof value === 'string' ? value : String(value || '')
  if (draft) drafts[id] = draft
  else delete drafts[id]
  writeDraftMap(drafts, storage)
}

export function clearChatSessionDrafts(sessionIds, storage) {
  const ids = Array.isArray(sessionIds) ? sessionIds : [sessionIds]
  const drafts = readDraftMap(storage)
  let changed = false
  for (const value of ids) {
    const id = String(value || '').trim()
    if (id && Object.prototype.hasOwnProperty.call(drafts, id)) {
      delete drafts[id]
      changed = true
    }
  }
  if (changed) writeDraftMap(drafts, storage)
}

export { STORAGE_KEY as CHAT_SESSION_DRAFTS_STORAGE_KEY }
