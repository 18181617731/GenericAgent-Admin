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

export const sessionBatchResult = (ids, settled) => {
  const normalized = normalizeSessionIds(ids)
  const succeededIds = []
  const failedIds = []
  const failures = []
  normalized.forEach((id, index) => {
    const result = settled[index]
    if (result?.status === 'fulfilled') {
      succeededIds.push(id)
      return
    }
    const reason = result?.reason
    failedIds.push(id)
    failures.push({ id, error:reason instanceof Error ? reason : new Error(String(reason || 'Unknown failure')) })
  })
  return { succeededIds, failedIds, failures }
}

export const runChatSessionBatch = async (ids, actionOne) => {
  const normalized = normalizeSessionIds(ids)
  if (normalized.length === 0) return { succeededIds: [], failedIds: [], failures: [] }
  if (typeof actionOne !== 'function') throw new TypeError('actionOne must be a function')
  return sessionBatchResult(normalized, await Promise.allSettled(normalized.map(id => actionOne(id))))
}

export const deleteChatSessions = async (ids, deleteOne) => {
  const normalized = normalizeSessionIds(ids)
  if (normalized.length === 0) return { deletedIds: [], failedIds: [], failures: [] }
  if (typeof deleteOne !== 'function') throw new TypeError('deleteOne must be a function')

  const result = await runChatSessionBatch(normalized, deleteOne)
  return { deletedIds:result.succeededIds, failedIds:result.failedIds, failures:result.failures }
}
