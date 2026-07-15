export const normalizeSessionIds = (ids = []) => {
  const seen = new Set()
  const normalized = []
  for (const value of Array.isArray(ids) ? ids : []) {
    const id = typeof value === 'string' ? value.trim() : ''
    if (!id || seen.has(id)) continue
    seen.add(id)
    normalized.push(id)
  }
  return normalized
}

export const deleteChatSessions = async (ids, deleteOne) => {
  const normalized = normalizeSessionIds(ids)
  if (normalized.length === 0) return { deletedIds: [], failedIds: [], failures: [] }
  if (typeof deleteOne !== 'function') throw new TypeError('deleteOne must be a function')

  const settled = await Promise.allSettled(normalized.map(id => deleteOne(id)))
  const deletedIds = []
  const failedIds = []
  const failures = []
  settled.forEach((result, index) => {
    const id = normalized[index]
    if (result.status === 'fulfilled') {
      deletedIds.push(id)
      return
    }
    failedIds.push(id)
    failures.push({ id, error: result.reason instanceof Error ? result.reason : new Error(String(result.reason)) })
  })
  return { deletedIds, failedIds, failures }
}
