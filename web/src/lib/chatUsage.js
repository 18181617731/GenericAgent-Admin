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

// Cache hit rate: cache_read / (output + cache_read). Measures what portion
// of generated content came from cache rather than being freshly produced.
// For legacy APIs, use output as denominator since cached_tokens relates to input.
export const cacheHitPercent = (usages) => {
  if (!Array.isArray(usages)) return 0
  const totals = usages.reduce((acc, usage) => {
    const read = cacheReadTokens(usage)
    const output = tokenCount(usage?.output_tokens)
    acc.read += read
    acc.output += output
    return acc
  }, { read: 0, output: 0 })
  const denominator = totals.output + totals.read
  return denominator > 0 ? Math.round(totals.read / denominator * 100) : 0
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
