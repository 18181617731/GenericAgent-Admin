const tokenCount = (value) => {
  const count = Number(value)
  return Number.isFinite(count) && count > 0 ? count : 0
}

// cache_read_tokens is canonical. cached_tokens is retained only for sessions
// persisted by older workers, which also stored a zero-valued canonical field.
export const cacheReadTokens = (usage) => {
  const canonical = tokenCount(usage?.cache_read_tokens)
  return canonical > 0 ? canonical : tokenCount(usage?.cached_tokens)
}

// Cache hit rate uses protocol-specific denominators. Modern providers report
// uncached input, cache creation, and cache read as disjoint buckets. Legacy
// cached_tokens is already a subset of input_tokens, so adding it again would
// inflate the denominator.
export const cacheHitPercent = (usages) => {
  if (!Array.isArray(usages)) return 0
  const totals = usages.reduce((acc, usage) => {
    const read = cacheReadTokens(usage)
    const input = tokenCount(usage?.input_tokens)
    const creation = tokenCount(usage?.cache_creation_tokens)
    const hasModernBuckets = creation > 0 || tokenCount(usage?.cache_read_tokens) > 0
    acc.read += read
    acc.base += hasModernBuckets ? input + creation + read : input
    return acc
  }, { read: 0, base: 0 })
  return totals.base > 0 ? Math.round(totals.read / totals.base * 100) : 0
}

// Generation speed must use only calls with an observed first chunk -> Output
// interval. Mixing request elapsed time or unmeasured calls would include TTFT,
// tool execution and queueing, so those calls are deliberately excluded.
export const measuredOutputRate = (usages) => {
  if (!Array.isArray(usages)) return 0
  const measured = usages.reduce((acc, usage) => {
    const generationMs = tokenCount(usage?.generation_ms)
    if (generationMs <= 0) return acc
    acc.generationMs += generationMs
    acc.outputTokens += tokenCount(usage?.output_tokens)
    return acc
  }, { generationMs: 0, outputTokens: 0 })
  return measured.generationMs > 0
    ? measured.outputTokens / (measured.generationMs / 1000)
    : 0
}
