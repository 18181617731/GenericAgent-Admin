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

export const generateChatSessionTitles = async (ids, generateOne, concurrency = 2) => {
  const normalized = normalizeSessionIds(ids)
  if (normalized.length === 0) return { generated: [], generatedIds: [], failedIds: [], failures: [] }
  if (typeof generateOne !== 'function') throw new TypeError('generateOne must be a function')

  const generated = []
  const failedIds = []
  const failures = []
  let cursor = 0
  const workerCount = Math.max(1, Math.min(normalized.length, Number.isInteger(concurrency) ? concurrency : 2))
  const workers = Array.from({ length: workerCount }, async () => {
    while (cursor < normalized.length) {
      const index = cursor
      cursor += 1
      const id = normalized[index]
      try {
        generated.push({ id, session: await generateOne(id) })
      } catch (reason) {
        failedIds.push(id)
        failures.push({ id, error: reason instanceof Error ? reason : new Error(String(reason)) })
      }
    }
  })
  await Promise.all(workers)
  return {
    generated,
    generatedIds: generated.map(item => item.id),
    failedIds,
    failures,
  }
}
