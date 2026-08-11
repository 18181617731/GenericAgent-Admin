export const hubSessions = (sessions, query = '') => {
  const normalizedQuery = String(query || '').trim().toLowerCase()
  return (Array.isArray(sessions) ? sessions : []).filter((session) => {
    if (!session?.hub_enabled) return false
    return !normalizedQuery || String(session.title || '').toLowerCase().includes(normalizedQuery)
  })
}
