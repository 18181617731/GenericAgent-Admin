export const AUTORUN_IDLE_MS = 30 * 60 * 1000
export const AUTORUN_FIRST_WAIT_MS = 60 * 1000

export function autorunInitialReplyAt(nowMs = Date.now()) {
  return nowMs - (AUTORUN_IDLE_MS - AUTORUN_FIRST_WAIT_MS)
}

export function shouldTriggerAutorun({ enabled, nowMs, lastReplyAtMs, blocked }) {
  if (!enabled || blocked) return false
  const now = Number(nowMs)
  const lastReply = Number(lastReplyAtMs)
  if (!Number.isFinite(now) || !Number.isFinite(lastReply)) return false
  return now - lastReply > AUTORUN_IDLE_MS
}
