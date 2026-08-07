const STORAGE_KEY = 'ga-admin-chat-search-history-v1'
const MAX_HISTORY = 8
const VALID_SCOPES = new Set(['all', 'title', 'content', 'project'])

const resolveStorage = storage => storage || (typeof window !== 'undefined' ? window.localStorage : null)

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

export const loadSessionSearchHistory = storage => {
  const target = resolveStorage(storage)
  if (!target) return []
  try {
    return normalizeSessionSearchHistory(JSON.parse(target.getItem(STORAGE_KEY) || '[]'))
  } catch {
    return []
  }
}

export const saveSessionSearchHistory = (entry, storage) => {
  const target = resolveStorage(storage)
  const normalized = normalizeSessionSearchEntry(entry)
  if (!target || !normalized) return loadSessionSearchHistory(target)
  const next = normalizeSessionSearchHistory([normalized, ...loadSessionSearchHistory(target)])
  try { target.setItem(STORAGE_KEY, JSON.stringify(next)) } catch { /* best effort */ }
  return next
}

export const clearSessionSearchHistory = storage => {
  const target = resolveStorage(storage)
  try { target?.removeItem(STORAGE_KEY) } catch { /* best effort */ }
  return []
}

export const sessionSearchScopeOptions = lang => lang === 'en'
  ? [{ value:'all', label:'All' }, { value:'title', label:'Titles' }, { value:'content', label:'Messages' }, { value:'project', label:'Projects' }]
  : [{ value:'all', label:'全部' }, { value:'title', label:'标题' }, { value:'content', label:'消息内容' }, { value:'project', label:'项目' }]
