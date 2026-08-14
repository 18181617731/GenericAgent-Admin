const STORAGE_KEY = 'ga-admin-chat-search-history-v1'
const DEFAULT_INSTANCE_KEY = '__default__'
const MAX_HISTORY = 8
const VALID_SCOPES = new Set(['all', 'title', 'content', 'project'])

const resolveStorage = storage => {
  if (storage) return storage
  if (typeof window === 'undefined') return null
  try { return window.localStorage } catch { return null }
}

const isStorage = value => Boolean(value && typeof value.getItem === 'function' && typeof value.setItem === 'function' && typeof value.removeItem === 'function')

const instanceKey = instanceID => {
  const id = String(instanceID || '').trim()
  return !id || id === 'default' ? DEFAULT_INSTANCE_KEY : id
}

const normalizeScope = scope => VALID_SCOPES.has(scope) ? scope : 'all'

export const normalizeSessionSearchEntry = entry => {
  const query = typeof entry === 'string' ? entry.trim() : String(entry?.query || '').trim()
  if (!query) return null
  return { query, scope: normalizeScope(typeof entry === 'string' ? 'all' : entry?.scope) }
}

export const normalizeSessionSearchHistory = entries => {
  const seen = new Set()
  return (Array.isArray(entries) ? entries : [])
    .map(normalizeSessionSearchEntry)
    .filter(entry => {
      if (!entry) return false
      const key = `${entry.scope}:${entry.query.toLocaleLowerCase()}`
      if (seen.has(key)) return false
      seen.add(key)
      return true
    })
    .slice(0, MAX_HISTORY)
}

const readHistoryStore = storage => {
  const target = resolveStorage(storage)
  if (!target) return {}
  try {
    const parsed = JSON.parse(target.getItem(STORAGE_KEY) || '[]')
    if (Array.isArray(parsed)) return { [DEFAULT_INSTANCE_KEY]: normalizeSessionSearchHistory(parsed) }
    if (!parsed || typeof parsed !== 'object') return {}
    return Object.fromEntries(Object.entries(parsed)
      .map(([scope, entries]) => [scope, normalizeSessionSearchHistory(entries)])
      .filter(([, entries]) => entries.length))
  } catch {
    return {}
  }
}

const writeHistoryStore = (store, storage) => {
  const target = resolveStorage(storage)
  if (!target) return
  try {
    if (Object.keys(store).length) target.setItem(STORAGE_KEY, JSON.stringify(store))
    else target.removeItem(STORAGE_KEY)
  } catch { /* best effort */ }
}

const scopedStorage = (first, second) => isStorage(first) || first == null
  ? { storage: first, instanceID: second }
  : { storage: second, instanceID: first }

export const loadSessionSearchHistory = (storageOrInstanceID, instanceIDOrStorage) => {
  const { storage, instanceID } = scopedStorage(storageOrInstanceID, instanceIDOrStorage)
  return readHistoryStore(storage)[instanceKey(instanceID)] || []
}

export const saveSessionSearchHistory = (entryOrInstanceID, storageOrEntry, maybeStorage) => {
  const scoped = isStorage(storageOrEntry) || storageOrEntry == null
    ? { entry: entryOrInstanceID, storage: storageOrEntry, instanceID: maybeStorage }
    : { entry: storageOrEntry, storage: maybeStorage, instanceID: entryOrInstanceID }
  const target = resolveStorage(scoped.storage)
  const normalized = normalizeSessionSearchEntry(scoped.entry)
  if (!target || !normalized) return loadSessionSearchHistory(scoped.storage, scoped.instanceID)
  const store = readHistoryStore(target)
  const scope = instanceKey(scoped.instanceID)
  const next = normalizeSessionSearchHistory([normalized, ...(store[scope] || [])])
  if (next.length) store[scope] = next
  else delete store[scope]
  writeHistoryStore(store, target)
  return next
}

export const clearSessionSearchHistory = (storageOrInstanceID, instanceIDOrStorage) => {
  const target = resolveStorage(isStorage(storageOrInstanceID) || storageOrInstanceID == null ? storageOrInstanceID : instanceIDOrStorage)
  if (!target) return []
  const scoped = isStorage(storageOrInstanceID) || storageOrInstanceID == null
    ? { instanceID: instanceIDOrStorage, all: instanceIDOrStorage == null }
    : { instanceID: storageOrInstanceID, all: false }
  if (scoped.all) {
    try { target.removeItem(STORAGE_KEY) } catch { /* best effort */ }
    return []
  }
  const store = readHistoryStore(target)
  delete store[instanceKey(scoped.instanceID)]
  writeHistoryStore(store, target)
  return []
}

export { DEFAULT_INSTANCE_KEY as CHAT_SESSION_SEARCH_DEFAULT_INSTANCE_KEY }

export const sessionSearchScopeOptions = lang => lang === 'en'
  ? [{ value:'all', label:'All' }, { value:'title', label:'Titles' }, { value:'content', label:'Messages' }, { value:'project', label:'Projects' }]
  : [{ value:'all', label:'全部' }, { value:'title', label:'标题' }, { value:'content', label:'消息内容' }, { value:'project', label:'项目' }]
