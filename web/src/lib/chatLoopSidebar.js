const ACTIVE_LOOP_STATUSES = new Set(['waiting', 'running', 'evaluating'])

export function loopSidebarView(loop) {
  if (!loop || typeof loop !== 'object' || !loop.enabled) return null
  const status = String(loop.status || '').trim().toLowerCase()
  if (!ACTIVE_LOOP_STATUSES.has(status)) return null
  const roundValue = Number(loop.round)
  const maxRoundsValue = Number(loop.max_rounds)
  const round = Number.isFinite(roundValue) ? Math.max(0, Math.floor(roundValue)) : 0
  const maxRounds = Number.isFinite(maxRoundsValue) ? Math.max(1, Math.floor(maxRoundsValue)) : 1
  return { status, round, maxRounds }
}

export function updateSessionLoop(sessions, sessionID, loop) {
  if (!Array.isArray(sessions) || !sessionID || !loop || typeof loop !== 'object') return sessions
  return sessions.map((session) => session?.id === sessionID ? { ...session, loop } : session)
}
