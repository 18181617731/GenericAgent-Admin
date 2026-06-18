const toText = (value) => String(value ?? '')

export const stripAskUserFence = (value = '') => {
  let text = toText(value).trim()
  const open = text.match(/^(`{3,}|~{3,})([A-Za-z0-9_-]+)?[ \t]*\r?\n?/)
  if (open) {
    const fence = open[1]
    text = text.slice(open[0].length)
    const closeRe = new RegExp(`\\r?\\n?${fence.replace(/[`~]/g, '\\$&')}[ \\t]*$`)
    text = text.replace(closeRe, '')
  }
  return text.trim()
}

const normalizeParsedPayload = (data, raw, structured = true) => {
  if (!data || typeof data !== 'object') return null
  const payload = data.data && typeof data.data === 'object' ? { ...data, ...data.data } : data
  const question = toText(payload.question ?? payload.prompt ?? payload.message ?? '').trim()
  const candidates = Array.isArray(payload.candidates)
    ? payload.candidates.map(x => toText(x).trim()).filter(Boolean)
    : []
  if (!question && !candidates.length) return null
  return { question, candidates, raw: toText(raw).trim(), structured }
}

const tryParseJson = (text = '') => {
  const src = stripAskUserFence(text)
  if (!src) return null
  try { return normalizeParsedPayload(JSON.parse(src), src, true) } catch {}
  return null
}

const findBalancedJsonObjects = (text = '') => {
  const src = toText(text)
  const out = []
  let start = -1
  let depth = 0
  let inString = false
  let escaped = false
  for (let i = 0; i < src.length; i++) {
    const ch = src[i]
    if (inString) {
      if (escaped) { escaped = false; continue }
      if (ch === '\\') { escaped = true; continue }
      if (ch === '"') inString = false
      continue
    }
    if (ch === '"') { inString = true; continue }
    if (ch === '{') {
      if (depth === 0) start = i
      depth += 1
      continue
    }
    if (ch === '}' && depth > 0) {
      depth -= 1
      if (depth === 0 && start >= 0) out.push(src.slice(start, i + 1))
    }
  }
  return out
}

const parseJsonStringAt = (src, quoteIndex) => {
  if (quoteIndex < 0 || src[quoteIndex] !== '"') return null
  let out = ''
  for (let i = quoteIndex + 1; i < src.length; i++) {
    const ch = src[i]
    if (ch === '\\') {
      if (i + 1 >= src.length) return { value: out, partial: true, end: i + 1 }
      const next = src[i + 1]
      const seq = ch + next
      try { out += JSON.parse(`"${seq}"`) } catch { out += next }
      i += 1
      continue
    }
    if (ch === '"') return { value: out, partial: false, end: i + 1 }
    out += ch
  }
  return { value: out, partial: true, end: src.length }
}

const fallbackStringField = (src, names) => {
  for (const name of names) {
    const re = new RegExp(`"${name}"\\s*:\\s*"`, 'i')
    const m = re.exec(src)
    if (!m) continue
    const parsed = parseJsonStringAt(src, m.index + m[0].length - 1)
    if (parsed?.value) return parsed.value.trim()
  }
  return ''
}

const fallbackCandidates = (src) => {
  const m = /"candidates"\s*:\s*\[/i.exec(src)
  if (!m) return []
  const out = []
  let i = m.index + m[0].length
  while (i < src.length) {
    const ch = src[i]
    if (ch === ']') break
    if (ch === '"') {
      const parsed = parseJsonStringAt(src, i)
      if (!parsed) break
      const value = toText(parsed.value).trim()
      if (value) out.push(value)
      i = Math.max(parsed.end, i + 1)
      continue
    }
    i += 1
  }
  return out
}

export const parseAskUserPayload = (raw = '') => {
  const source = toText(raw).trim()
  const candidatesToParse = []
  const push = (x) => {
    const text = stripAskUserFence(x)
    if (text && !candidatesToParse.includes(text)) candidatesToParse.push(text)
  }
  push(source)
  for (const jsonText of findBalancedJsonObjects(source)) push(jsonText)

  for (const text of candidatesToParse) {
    const parsed = tryParseJson(text)
    if (parsed) return parsed
  }

  const text = stripAskUserFence(source)
  if (!text) return { question:'', candidates:[], raw:'', structured:false }
  const scanTarget = candidatesToParse.find(x => /"(?:question|prompt|message|candidates)"/i.test(x)) || text
  const question = fallbackStringField(scanTarget, ['question', 'prompt', 'message']) || text
  const opts = fallbackCandidates(scanTarget)
  return { question, candidates:opts, raw:text, structured:Boolean(question || opts.length) }
}

export const getAskUserPayload = (call = {}) => {
  const fromResult = parseAskUserPayload(call.result)
  if (fromResult.structured) return fromResult
  const fromArgs = parseAskUserPayload(call.args)
  if (fromArgs.structured || fromArgs.question || fromArgs.candidates.length) return fromArgs
  return fromResult
}
