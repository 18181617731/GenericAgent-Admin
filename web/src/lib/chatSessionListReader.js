import { parseApiResponse } from './api.js'

const METADATA_KEYS = ['projects', 'project_items', 'pinned_projects', 'project_order']
const clone = value => value == null ? value : JSON.parse(JSON.stringify(value))
const sortSessions = sessions => sessions.sort((a, b) => {
  const byTime = (Number(b.updated_at) || 0) - (Number(a.updated_at) || 0)
  if (byTime) return byTime
  const left = String(a.id), right = String(b.id)
  return left < right ? -1 : left > right ? 1 : 0
})

// Cache private serialized snapshots, not mutable UI objects. Cursors are
// instance-URL scoped; a mutation refresh never joins an older in-flight read.
export function createChatSessionListReader({ maxBytes = 8 * 1024 * 1024, maxEntries = 4 } = {}) {
  const entries = new Map()
  const pending = new Map()
  const tokens = new Map()
  let bytes = 0
  let generation = 0
  const remove = url => {
    const entry = entries.get(url)
    if (entry) bytes -= entry.bytes
    entries.delete(url)
  }
  const load = (url, { fresh = false, fetcher = (...args) => globalThis.fetch(...args) } = {}) => {
    if (!fresh && pending.has(url)) return pending.get(url).then(clone)
    const token = Symbol(url), startGeneration = generation
    tokens.set(url, token)
    const entry = entries.get(url)
    const baseline = entry ? JSON.parse(entry.text) : null
    const assertCurrent = () => {
      if (startGeneration !== generation || tokens.get(url) !== token) {
        const error = new Error('Session list request superseded')
        error.name = 'AbortError'
        throw error
      }
    }
    const request = (async () => {
      let data, etag = '', revision = ''
      for (let attempt = 0; attempt < 2; attempt++) {
        const useBaseline = attempt === 0 ? baseline : null
        const cursor = useBaseline && entry?.revision
        const syncURL = `${url}${url.includes('?') ? '&' : '?'}sync=1${cursor ? `&since=${encodeURIComponent(cursor)}` : ''}`
        const headers = { 'Content-Type': 'application/json' }
        // Legacy backends ignore sync and still benefit from their HTTP ETag.
        if (useBaseline && !cursor && entry?.etag) headers['If-None-Match'] = entry.etag
        const res = await fetcher(syncURL, { headers })
        assertCurrent()
        if (res.status === 204 || res.status === 304) {
          if (!useBaseline) {
            if (attempt === 0) continue
            throw new Error('Session list cache miss on unchanged response')
          }
          data = useBaseline
          revision = cursor || ''
          etag = entry.etag
          break
        }
        const wire = await parseApiResponse(res, syncURL)
        assertCurrent()
        etag = res.headers?.get('ETag') || ''
        revision = wire?.revision || ''
        if (wire?.reset === false) {
          if (!useBaseline || !cursor || wire.base_revision !== cursor || !revision) {
            if (attempt === 0) continue
            throw new Error('Session list delta has no matching baseline')
          }
          const sessions = new Map((useBaseline.sessions || []).map(item => [item.id, item]))
          for (const id of wire.removed_session_ids || []) sessions.delete(id)
          for (const item of wire.session_upserts || []) sessions.set(item.id, item)
          data = { ...useBaseline, sessions: sortSessions([...sessions.values()]) }
          for (const key of METADATA_KEYS) {
            if (Object.hasOwn(wire, key)) data[key] = wire[key]
          }
        } else {
          data = wire == null ? wire : { ...wire }
          if (data) {
            delete data.reset
            delete data.revision
          }
        }
        break
      }
      assertCurrent()
      {
        remove(url)
        const text = JSON.stringify(data) ?? 'null', size = text.length * 2
        if (size <= maxBytes && maxEntries > 0) {
          while (entries.size >= maxEntries || (entries.size && bytes + size > maxBytes)) remove(entries.keys().next().value)
          entries.set(url, { text, bytes: size, etag, revision })
          bytes += size
        }
      }
      return data
    })()
    pending.set(url, request)
    request.finally(() => {
      if (pending.get(url) === request) pending.delete(url)
      if (tokens.get(url) === token) tokens.delete(url)
    }).catch(() => {})
    return request
  }
  const clear = () => {
    generation++
    pending.clear()
    tokens.clear()
    entries.clear()
    bytes = 0
  }
  return { load, clear }
}
