export function groupProjectSessions(projects, sessions) {
  const sourceProjects = Array.isArray(projects) ? projects : []
  const sourceSessions = Array.isArray(sessions) ? sessions : []
  const seen = new Set()

  return sourceProjects.reduce((groups, value) => {
    const name = typeof value === 'string' ? value.trim() : ''
    if (!name || seen.has(name)) return groups
    seen.add(name)
    groups.push({
      name,
      sessions: sourceSessions.filter(session => String(session?.project_mode || '').trim() === name),
    })
    return groups
  }, [])
}
