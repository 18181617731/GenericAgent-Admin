const LOOP_RECORD_LIMIT = 40
const LOOP_RECORD_TEXT_LIMIT = 360

const cleanText = value => String(value ?? '').trim().slice(0, LOOP_RECORD_TEXT_LIMIT)

export function normalizeLoopRecords(loop) {
  if (!loop || typeof loop !== 'object' || !Array.isArray(loop.records)) return []
  return loop.records
    .slice(-LOOP_RECORD_LIMIT)
    .map((record, index) => {
      if (!record || typeof record !== 'object') return null
      const atMS = Number(record.created_at_ms)
      const roundValue = Number(record.round)
      const summary = cleanText(record.summary)
      const prompt = cleanText(record.prompt)
      if (!summary && !prompt) return null
      return {
        key: `${Number.isFinite(atMS) ? atMS : 0}-${index}`,
        atMS: Number.isFinite(atMS) && atMS > 0 ? atMS : 0,
        round: Number.isFinite(roundValue) ? Math.max(0, Math.floor(roundValue)) : 0,
        phase: cleanText(record.phase).toLowerCase() || 'activity',
        summary,
        prompt,
      }
    })
    .filter(Boolean)
    .reverse()
}
