export function rememberSessionScroll(snapshots, sessionID, scrollTop, autoFollow) {
  const id = String(sessionID || '').trim()
  if (!(snapshots instanceof Map) || !id) return null
  const numericTop = Number(scrollTop)
  const snapshot = {
    scrollTop: Number.isFinite(numericTop) ? Math.max(0, numericTop) : 0,
    autoFollow: autoFollow !== false,
  }
  snapshots.set(id, snapshot)
  return snapshot
}

export function sessionScrollRestore(snapshots, sessionID) {
  const id = String(sessionID || '').trim()
  const snapshot = snapshots instanceof Map && id ? snapshots.get(id) : null
  if (!snapshot || snapshot.autoFollow !== false) return null
  const numericTop = Number(snapshot.scrollTop)
  return {
    scrollTop: Number.isFinite(numericTop) ? Math.max(0, numericTop) : 0,
    autoFollow: false,
  }
}

export function forgetSessionScroll(snapshots, sessionIDs) {
  if (!(snapshots instanceof Map)) return
  const ids = Array.isArray(sessionIDs) ? sessionIDs : [sessionIDs]
  for (const sessionID of ids) {
    const id = String(sessionID || '').trim()
    if (id) snapshots.delete(id)
  }
}
