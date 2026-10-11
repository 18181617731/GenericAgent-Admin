import { createChatSessionCache } from './chatSessionCache.js'

// Coalesce background reads but never join a pre-mutation request when a caller
// explicitly refreshes after writing. URLs include the selected instance.
export function createChatSessionListReader() {
  const cache = createChatSessionCache({ maxBytes: 8 * 1024 * 1024, maxEntries: 4 })
  const pending = new Map()
  const load = (url, { fresh = false, fetcher = fetch } = {}) => {
    if (!fresh && pending.has(url)) {
      // Independent views keep optimistic UI updates out of other consumers.
      return pending.get(url).then(data => data == null ? data : JSON.parse(JSON.stringify(data)))
    }
    const request = cache.load(url, { fetcher })
    pending.set(url, request)
    request.finally(() => {
      if (pending.get(url) === request) pending.delete(url)
    }).catch(() => {})
    return request
  }
  const clear = () => { pending.clear(); cache.clear() }
  return { load, clear }
}
