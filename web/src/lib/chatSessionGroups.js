const timestampMs = (value) => {
  if (typeof value === 'number' && Number.isFinite(value)) return Math.abs(value) < 1e12 ? value * 1000 : value
  if (typeof value === 'string' && value.trim()) {
    const numeric = Number(value)
    if (Number.isFinite(numeric)) return Math.abs(numeric) < 1e12 ? numeric * 1000 : numeric
    return new Date(value).getTime()
  }
  return NaN
}

const startOfDay = (date) => new Date(date.getFullYear(), date.getMonth(), date.getDate())
const startOfWeek = (date) => {
  const day = startOfDay(date)
  const mondayOffset = (day.getDay() + 6) % 7
  day.setDate(day.getDate() - mondayOffset)
  return day
}

export const RECENT_SESSION_GROUP_KEYS = ['pinned', 'today', 'yesterday', 'this_week', 'last_week', 'this_month', 'older']

export function groupRecentSessions(sessions, now = new Date()) {
  const groups = Object.fromEntries(RECENT_SESSION_GROUP_KEYS.map(key => [key, []]))
  const today = startOfDay(now)
  const yesterday = new Date(today)
  yesterday.setDate(yesterday.getDate() - 1)
  const thisWeek = startOfWeek(now)
  const lastWeek = new Date(thisWeek)
  lastWeek.setDate(lastWeek.getDate() - 7)
  const thisMonth = new Date(now.getFullYear(), now.getMonth(), 1)

  for (const session of sessions || []) {
    if (session?.pinned) {
      groups.pinned.push(session)
      continue
    }
    const ms = timestampMs(session?.updated_at)
    if (!Number.isFinite(ms)) {
      groups.older.push(session)
      continue
    }
    const date = new Date(ms)
    if (date >= today) groups.today.push(session)
    else if (date >= yesterday) groups.yesterday.push(session)
    else if (date >= thisWeek) groups.this_week.push(session)
    else if (date >= lastWeek) groups.last_week.push(session)
    else if (date >= thisMonth) groups.this_month.push(session)
    else groups.older.push(session)
  }

  return RECENT_SESSION_GROUP_KEYS.map(key => ({ key, sessions: groups[key] })).filter(group => group.sessions.length > 0)
}
