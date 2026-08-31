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

const instanceKey = value => String(value || '').trim()

function normalizeDraftEntry(value) {
  if (typeof value === 'string') {
    return value ? { text: value, updated_at: 0, instance_id: '' } : null
  }
  if (!value || typeof value !== 'object' || Array.isArray(value)) return null
  const text = typeof value.text === 'string' ? value.text : ''
  if (!text) return null
  const updatedAt = Number(value.updated_at)
  return {
    text,
    updated_at: Number.isFinite(updatedAt) && updatedAt > 0 ? updatedAt : 0,
    instance_id: instanceKey(value.instance_id),
  }
}

function readDraftMap(storage) {
  const target = availableStorage(storage)
  if (!target) return {}
  try {
    const parsed = JSON.parse(target.getItem(STORAGE_KEY) || '{}')
    if (!parsed || typeof parsed !== 'object' || Array.isArray(parsed)) return {}
    return Object.fromEntries(Object.entries(parsed).flatMap(([id, value]) => {
      const draft = id ? normalizeDraftEntry(value) : null
      return draft ? [[id, draft]] : []
    }))
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

export function listChatSessionDraftIds(storage, instanceId) {
  const requestedInstance = instanceId === undefined ? null : instanceKey(instanceId)
  return Object.entries(readDraftMap(storage))
    .filter(([, draft]) => requestedInstance === null || !draft.instance_id || draft.instance_id === requestedInstance)
    .map(([id]) => id)
}

export function loadChatSessionDraft(sessionId, storage, instanceId) {
  const id = String(sessionId || '').trim()
  if (!id) return ''
  const draft = readDraftMap(storage)[id]
  if (!draft) return ''
  const requestedInstance = instanceId === undefined ? null : instanceKey(instanceId)
  if (requestedInstance !== null && draft.instance_id && draft.instance_id !== requestedInstance) return ''
  return draft.text
}

export function saveChatSessionDraft(sessionId, value, storage, instanceId = '') {
  const id = String(sessionId || '').trim()
  if (!id) return
  const drafts = readDraftMap(storage)
  const text = typeof value === 'string' ? value : String(value || '')
  if (text) {
    const previous = drafts[id]
    drafts[id] = {
      text,
      updated_at: previous?.text === text && previous.updated_at > 0 ? previous.updated_at : Date.now(),
      instance_id: instanceKey(instanceId),
    }
  } else delete drafts[id]
  writeDraftMap(drafts, storage)
}

export function mergeChatSessionDraftSessions(sessions, instanceId, storage) {
  const sourceSessions = Array.isArray(sessions) ? sessions : []
  const serverSessions = sourceSessions.filter(session => !session?.local_draft)
  const serverIDs = new Set(serverSessions.map(session => String(session?.id || '').trim()).filter(Boolean))
  const requestedInstance = instanceKey(instanceId)
  const draftSessions = Object.entries(readDraftMap(storage))
    .filter(([id, draft]) => id && !serverIDs.has(id) && (!draft.instance_id || draft.instance_id === requestedInstance))
    .sort((a, b) => b[1].updated_at - a[1].updated_at)
    .map(([id, draft]) => ({
      id,
      title: '',
      count: 0,
      updated_at: draft.updated_at > 0 ? new Date(draft.updated_at).toISOString() : '',
      local_draft: true,
    }))
  return draftSessions.concat(serverSessions)
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
