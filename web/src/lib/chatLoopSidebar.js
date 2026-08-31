const ACTIVE_LOOP_STATUSES = new Set(['waiting', 'running', 'evaluating'])

export function loopSidebarView(loop) {
  if (!loop || typeof loop !== 'object' || !loop.enabled) return null
  const status = String(loop.status || '').trim().toLowerCase()
  if (!ACTIVE_LOOP_STATUSES.has(status)) return null
  const roundValue = Number(loop.round)
  const round = Number.isFinite(roundValue) ? Math.max(0, Math.floor(roundValue)) : 0
  return { status, round }
}

export function updateSessionLoop(sessions, sessionID, loop) {
  if (!Array.isArray(sessions) || !sessionID || !loop || typeof loop !== 'object') return sessions
  return sessions.map((session) => session?.id === sessionID ? { ...session, loop } : session)
}
