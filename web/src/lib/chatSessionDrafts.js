const STORAGE_KEY = 'ga-admin-chat-session-drafts-v1'
const DEFAULT_INSTANCE_KEY = '__default__'

function availableStorage(storage) {
  if (storage) return storage
  if (typeof window === 'undefined') return null
  try { return window.localStorage } catch { return null }
}

function isStorage(value) {
  return Boolean(value && typeof value.getItem === 'function' && typeof value.setItem === 'function' && typeof value.removeItem === 'function')
}

function instanceKey(instanceID) {
  const id = String(instanceID || '').trim()
  return !id || id === 'default' ? DEFAULT_INSTANCE_KEY : id
}

function normalizeDraftMap(value) {
  if (!value || typeof value !== 'object' || Array.isArray(value)) return {}
  return Object.fromEntries(Object.entries(value).filter(([id, draft]) => id && typeof draft === 'string' && draft))
}

function readDraftStore(storage) {
  const target = availableStorage(storage)
  if (!target) return {}
  try {
    const parsed = JSON.parse(target.getItem(STORAGE_KEY) || '{}')
    if (!parsed || typeof parsed !== 'object' || Array.isArray(parsed)) return {}
    const entries = Object.entries(parsed)
    if (entries.every(([, value]) => typeof value === 'string')) return { [DEFAULT_INSTANCE_KEY]: normalizeDraftMap(parsed) }
    return Object.fromEntries(entries.map(([scope, drafts]) => [scope, normalizeDraftMap(drafts)]).filter(([, drafts]) => Object.keys(drafts).length))
  } catch {
    return {}
  }
}

function readDraftMap(storage, instanceID) {
  return readDraftStore(storage)[instanceKey(instanceID)] || {}
}

function writeDraftMap(drafts, storage, instanceID) {
  const target = availableStorage(storage)
  if (!target) return
  try {
    const store = readDraftStore(target)
    const scope = instanceKey(instanceID)
    const next = normalizeDraftMap(drafts)
    if (Object.keys(next).length) store[scope] = next
    else delete store[scope]
    if (Object.keys(store).length) target.setItem(STORAGE_KEY, JSON.stringify(store))
    else target.removeItem(STORAGE_KEY)
  } catch {
    // Draft persistence is best-effort when storage is unavailable or full.
  }
}

function scopedStorage(first, second) {
  if (isStorage(first) || first == null) return { storage: first, instanceID: second }
  return { storage: second, instanceID: first }
}

export function listChatSessionDraftIds(storageOrInstanceID, instanceIDOrStorage) {
  const { storage, instanceID } = scopedStorage(storageOrInstanceID, instanceIDOrStorage)
  return Object.keys(readDraftMap(storage, instanceID))
}

export function loadChatSessionDraft(sessionOrInstanceID, storageOrSessionID, maybeStorage) {
  const scoped = isStorage(storageOrSessionID) || storageOrSessionID == null
    ? { sessionId: sessionOrInstanceID, storage: storageOrSessionID, instanceID: maybeStorage }
    : { sessionId: storageOrSessionID, storage: maybeStorage, instanceID: sessionOrInstanceID }
  const { sessionId, storage, instanceID } = scoped
  const id = String(sessionId || '').trim()
  if (!id) return ''
  return readDraftMap(storage, instanceID)[id] || ''
}

export function saveChatSessionDraft(...args) {
  let sessionId, value, storage, instanceID
  if (args.length >= 3 && (isStorage(args[2]) || args[2] == null)) {
    [sessionId, value, storage, instanceID] = args
  } else if (args.length >= 3) {
    [instanceID, sessionId, value, storage] = args
  } else {
    [sessionId, value] = args
  }
  const id = String(sessionId || '').trim()
  if (!id) return
  const drafts = readDraftMap(storage, instanceID)
  const draft = typeof value === 'string' ? value : String(value || '')
  if (draft) drafts[id] = draft
  else delete drafts[id]
  writeDraftMap(drafts, storage, instanceID)
}

export function mergeChatSessionDraftSessions(sessions, instanceID, storage) {
  const serverSessions = (Array.isArray(sessions) ? sessions : []).filter(session => !session?.local_draft)
  const serverIDs = new Set(serverSessions.map(session => String(session?.id || '').trim()).filter(Boolean))
  const draftSessions = listChatSessionDraftIds(instanceID, storage)
    .filter(id => !serverIDs.has(id))
    .map(id => ({ id, title: '', count: 0, updated_at: '', local_draft: true }))
  return draftSessions.concat(serverSessions)
}

export function clearChatSessionDrafts(instanceOrSessionIds, sessionIdsOrStorage, maybeStorage) {
  const scoped = isStorage(sessionIdsOrStorage) || sessionIdsOrStorage == null
    ? { ids: instanceOrSessionIds, storage: sessionIdsOrStorage, instanceID: maybeStorage }
    : { ids: sessionIdsOrStorage, storage: maybeStorage, instanceID: instanceOrSessionIds }
  const ids = Array.isArray(scoped.ids) ? scoped.ids : [scoped.ids]
  const drafts = readDraftMap(scoped.storage, scoped.instanceID)
  let changed = false
  for (const value of ids) {
    const id = String(value || '').trim()
    if (id && Object.prototype.hasOwnProperty.call(drafts, id)) {
      delete drafts[id]
      changed = true
    }
  }
  if (changed) writeDraftMap(drafts, scoped.storage, scoped.instanceID)
}

export { DEFAULT_INSTANCE_KEY as CHAT_SESSION_DRAFT_DEFAULT_INSTANCE_KEY, STORAGE_KEY as CHAT_SESSION_DRAFTS_STORAGE_KEY }
