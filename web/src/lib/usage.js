export const formatNumber = (value, lang = 'zh') => new Intl.NumberFormat(lang === 'zh' ? 'zh-CN' : 'en-US').format(Number(value) || 0)

export const formatTokens = (value, lang = 'zh') => {
  const number = Number(value) || 0
  const full = formatNumber(number, lang)
  let short
  if (lang === 'zh') {
    if (number >= 1e8) short = `${+(number / 1e8).toFixed(2)}亿`
    else if (number >= 1e4) short = `${+(number / 1e4).toFixed(2)}万`
    else short = full
  } else {
    if (number >= 1e9) short = `${+(number / 1e9).toFixed(2)}B`
    else if (number >= 1e6) short = `${+(number / 1e6).toFixed(2)}M`
    else if (number >= 1e3) short = `${+(number / 1e3).toFixed(2)}K`
    else short = full
  }
  return { short, full }
}

export const formatElapsed = (value, lang = 'zh') => {
  const milliseconds = Number(value) || 0
  if (milliseconds <= 0) return lang === 'zh' ? '未记录' : 'Not recorded'
  if (milliseconds < 1000) return `${milliseconds} ms`
  const seconds = milliseconds / 1000
  if (seconds < 60) return `${seconds.toFixed(seconds >= 10 ? 0 : 1)} s`
  const minutes = Math.floor(seconds / 60)
  const remainder = Math.round(seconds % 60)
  return lang === 'zh' ? `${minutes} 分 ${remainder} 秒` : `${minutes}m ${remainder}s`
}

export const formatUsageDateTime = (value, lang = 'zh') => {
  const milliseconds = Number(value) || 0
  if (!milliseconds) return lang === 'zh' ? '时间未知' : 'Unknown time'
  return new Intl.DateTimeFormat(lang === 'zh' ? 'zh-CN' : 'en-US', {
    dateStyle: 'medium',
    timeStyle: 'medium',
  }).format(new Date(milliseconds))
}

export const usageQueryString = (filters = {}, page = 1, pageSize = 20) => {
  const params = new URLSearchParams()
  if (filters.from) params.set('from', filters.from)
  if (filters.to) params.set('to', filters.to)
  if (filters.provider) params.set('provider', filters.provider)
  if (filters.model) params.set('model', filters.model)
  params.set('page', String(page))
  params.set('page_size', String(pageSize))
  return params.toString()
}
