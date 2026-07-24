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


export const createStreamDeltaBatcher = ({ onFlush, schedule, cancel }) => {
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
    if (pending && scheduled == null) scheduled = schedule(flushFrame)
  }
  const flushFrame = () => {
    scheduled = null
    if (!pending) return
    // Small model deltas stay immediate; network bursts are drained across a few
    // frames so the response advances continuously instead of jumping by blocks.
    const chunkSize = pending.length <= 24 ? pending.length : Math.min(64, Math.max(4, Math.ceil(pending.length / 8)))
    const chunk = pending.slice(0, chunkSize)
    pending = pending.slice(chunkSize)
    onFlush(chunk)
    scheduleNext()
    resolveDrains()
  }

  return {
    push(delta) {
      if (!delta) return
      pending += delta
      scheduleNext()
    },
    flushNow() {
      if (scheduled != null) {
        cancel(scheduled)
        scheduled = null
      }
      if (!pending) return
      const chunk = pending
      pending = ''
      onFlush(chunk)
      resolveDrains()
    },
    drain() {
      if (!pending && scheduled == null) return Promise.resolve()
      return new Promise(resolve => drainResolvers.push(resolve))
    },
  }
}
