export const isBTWCommand = (value) => /^\/btw(?:$|[ \t])/.test(String(value || '').trim())

export const shouldFinishStreamFollow = ({ running, replay, completed, eventCount }) => (
  !running && replay && completed && eventCount === 0
)

export const mergeFinalStreamMessage = (streamed = {}, finalMessage = {}) => {
  const merged = { ...finalMessage }
  if ((!merged.model_id || !String(merged.model_id).trim()) && streamed.model_id) merged.model_id = streamed.model_id
  if (merged.usage == null && streamed.usage != null) merged.usage = streamed.usage
  if ((!Array.isArray(merged.usages) || merged.usages.length === 0) && Array.isArray(streamed.usages) && streamed.usages.length) {
    merged.usages = streamed.usages
  }
  return merged
}


// live=true (default): deltas animate frame-by-frame as before.
// live=false: deltas accumulate silently until beginLive() flushes the backlog
// in one shot (used for replayed events when reattaching after a page refresh,
// so the whole in-progress output appears instantly instead of retyping).
export const createStreamDeltaBatcher = ({ onFlush, schedule, cancel, live = true }) => {
  let pending = ''
  let scheduled = null
  let drainResolvers = []

  const resolveDrains = () => {
    if (pending || scheduled != null) return
    const resolvers = drainResolvers
    drainResolvers = []
    resolvers.forEach(resolve => resolve())
  }
  const scheduleNext = () => {
    if (pending && scheduled == null) {
      // Skip frame-by-frame animation when tab is in background
      if (typeof document !== 'undefined' && document.hidden) {
        flushNow()
      } else {
        scheduled = schedule(flushFrame)
      }
    }
  }
  const flushFrame = () => {
    scheduled = null
    if (!pending) return
    // If tab became hidden during scheduled flush, drain immediately instead of chunking
    if (typeof document !== 'undefined' && document.hidden) {
      flushNow()
      return
    }
    // Small model deltas stay immediate; network bursts are drained across a few
    // frames so the response advances continuously instead of jumping by blocks.
    const chunkSize = pending.length <= 24 ? pending.length : Math.min(64, Math.max(4, Math.ceil(pending.length / 8)))
    const chunk = pending.slice(0, chunkSize)
    pending = pending.slice(chunkSize)
    onFlush(chunk)
    scheduleNext()
    resolveDrains()
  }

  const flushNow = () => {
    if (scheduled != null) {
      cancel(scheduled)
      scheduled = null
    }
    if (!pending) return
    const chunk = pending
    pending = ''
    onFlush(chunk)
    resolveDrains()
  }

  return {
    push(delta) {
      if (!delta) return
      pending += delta
      if (live) scheduleNext()
    },
    beginLive() {
      if (live) return
      live = true
      flushNow()
    },
    flushNow,
    drain() {
      if (!live) flushNow()
      if (!pending && scheduled == null) return Promise.resolve()
      return new Promise(resolve => drainResolvers.push(resolve))
    },
  }
}

// When re-attaching to a running stream (page refresh), replayed deltas must land on the
// placeholder of *that* run. Only the tail message can be it: an empty assistant earlier in
// the history is a stale leftover from an aborted/refreshed run, and targeting one of those
// would append the replay mid-history while the tail stayed stuck on "thinking" forever.
export const pickResumePlaceholderId = (messages) => {
  const list = Array.isArray(messages) ? messages : []
  const tail = list.length ? list[list.length - 1] : null
  if (!tail || tail.role !== 'assistant') return ''
  if (tail.content) return ''
  return tail.id || ''
}
