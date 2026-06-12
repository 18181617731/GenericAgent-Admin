export const REQUIRED_BBS_ENDPOINTS = [
  'GET /posts?limit=10&key=BOARD_KEY',
  'GET /post?id=1&key=BOARD_KEY',
  'POST /reply?key=BOARD_KEY',
  'POST /posts?key=BOARD_KEY',
]

export const isLikelyHttpUrl = (value = '') => {
  try {
    const url = new URL(String(value || '').trim())
    return url.protocol === 'http:' || url.protocol === 'https:'
  } catch {
    return false
  }
}

export const validateBBSConnectionInput = ({ mode = 'builtin', base_url = '', board_key = '' } = {}) => {
  const errors = []
  if (mode === 'external') {
    if (!String(base_url || '').trim()) errors.push('External BBS needs a base URL.')
    else if (!isLikelyHttpUrl(base_url)) errors.push('External BBS base must be an http(s) URL.')
  }
  if (!String(board_key || '').trim()) errors.push('Board key is required.')
  return { ok: errors.length === 0, errors }
}

export const normalizeBBSConnection = ({ status = {}, config = {}, fallbackBase = 'http://127.0.0.1:8787' } = {}) => {
  const mode = config.mode || status.mode || 'builtin'
  const builtinBase = status.builtin_base_url || config.builtin_base_url || (mode === 'builtin' ? status.base_url : '') || ''
  const externalBase = config.base_url || (mode === 'external' ? status.base_url : '') || ''
  const activeBase = mode === 'external' ? externalBase : (builtinBase || status.base_url || '')
  const hasConfigBoardKey = Object.prototype.hasOwnProperty.call(config, 'board_key')
  const boardKey = hasConfigBoardKey ? config.board_key : (status.board_key || 'ga-team')
  const input = validateBBSConnectionInput({ mode, base_url: externalBase, board_key: boardKey })
  return {
    mode,
    enabled: status.enabled !== false && !status.error && input.ok,
    builtinBase,
    externalBase,
    activeBase,
    boardKey,
    postCount: Number.isFinite(status.posts) ? status.posts : 0,
    error: status.error || input.errors[0] || '',
    inputErrors: input.errors,
    fallbackBase,
  }
}

export const buildWorkerSetting = (conn, name = 'worker-1') => JSON.stringify({
  base_url: conn?.activeBase || conn?.fallbackBase || 'http://127.0.0.1:8787',
  board_key: conn?.boardKey || 'ga-team',
  name,
})

export const validateBBSReadmeContract = (readme = '') => REQUIRED_BBS_ENDPOINTS.every(endpoint => readme.includes(endpoint)) && /GA worker:/i.test(readme)
