// Log lines arrive as opaque process output, so severity is guessed from the
// conventions GA's Python services actually emit: the stdlib logging level name,
// a bracketed level, or the first line of a traceback.
const LEVEL_PATTERNS = [
  [/(?:^|[\s[(|:])(?:ERROR|ERR|CRITICAL|FATAL|EXCEPTION)(?:$|[\s\]):|])|Traceback \(most recent call last\)/i, 'error'],
  [/(?:^|[\s[(|:])(?:WARN|WARNING)(?:$|[\s\]):|])/i, 'warn'],
]

export const logLineLevel = (line) => {
  const text = String(line ?? '')
  for (const [pattern, level] of LEVEL_PATTERNS) {
    if (pattern.test(text)) return level
  }
  return ''
}

// Rows keep the position they had in the tail, so the gutter still reads as a
// line number after a filter hides the lines in between.
export const buildLogRows = (lines, filter = '') => {
  const needle = String(filter ?? '').trim().toLowerCase()
  const rows = []
  ;(lines || []).forEach((line, index) => {
    const text = String(line ?? '')
    if (needle && !text.toLowerCase().includes(needle)) return
    rows.push({ index, number: index + 1, text, level: logLineLevel(text) })
  })
  return rows
}

// Highlighting splits into plain/match segments instead of injecting markup, so
// a log line containing HTML stays inert.
export const splitLogMatch = (line, filter = '') => {
  const text = String(line ?? '')
  const needle = String(filter ?? '').trim()
  if (!needle) return [{ text, match: false }]
  const haystack = text.toLowerCase()
  const lower = needle.toLowerCase()
  const parts = []
  let from = 0
  for (;;) {
    const at = haystack.indexOf(lower, from)
    if (at < 0) break
    if (at > from) parts.push({ text: text.slice(from, at), match: false })
    parts.push({ text: text.slice(at, at + needle.length), match: true })
    from = at + needle.length
  }
  if (from < text.length) parts.push({ text: text.slice(from), match: false })
  return parts.length ? parts : [{ text, match: false }]
}
