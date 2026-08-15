export function groupProjectSessions(projects, sessions, pinnedProjects) {
  const sourceProjects = Array.isArray(projects) ? projects : []
  const sourceSessions = Array.isArray(sessions) ? sessions : []
  const pinned = new Set((Array.isArray(pinnedProjects) ? pinnedProjects : [])
    .map(value => typeof value === 'string' ? value.trim() : '')
    .filter(Boolean))
  const seen = new Set()

  const groups = sourceProjects.reduce((acc, value) => {
    const name = typeof value === 'string' ? value.trim() : ''
    if (!name || seen.has(name)) return acc
    seen.add(name)
    acc.push({
      name,
      pinned: pinned.has(name),
      sessions: sourceSessions.filter(session => String(session?.project_mode || '').trim() === name),
    })
    return acc
  }, [])

  // Pinned projects float to the top; everything else keeps the order the server
  // sent, so the list stays stable as sessions come and go.
  const rank = (group) => group.pinned ? 0 : 1
  return groups
    .map((group, index) => ({ group, index }))
    .sort((a, b) => rank(a.group) - rank(b.group) || a.index - b.index)
    .map(entry => entry.group)
}
