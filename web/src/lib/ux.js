const ERROR_WORDS = /(?:失败|错误|异常|不可用|未生效|error|failed|failure|invalid|denied)/i
const SUCCESS_WORDS = /(?:成功|完成|已保存|已删除|已更新|已启动|已停止|已复制|success|saved|completed|updated)/i

export function feedbackTone(message, explicit = 'auto') {
  if (explicit && explicit !== 'auto') return explicit
  const text = String(message || '')
  if (ERROR_WORDS.test(text)) return 'error'
  if (SUCCESS_WORDS.test(text)) return 'success'
  return 'info'
}

export function filterModelProviderGroups(groups = [], query = '') {
  const needle = String(query || '').trim().toLocaleLowerCase()
  if (!needle) return groups
  return groups.flatMap(group => {
    const providerMatches = String(group?.label || '').toLocaleLowerCase().includes(needle)
    const models = providerMatches
      ? (group.models || [])
      : (group.models || []).filter(model => String(model?.label || '').toLocaleLowerCase().includes(needle))
    return models.length ? [{ ...group, models }] : []
  })
}

export function modelGroupStats(groups = []) {
  return {
    providers: groups.length,
    models: groups.reduce((total, group) => total + (group?.models?.length || 0), 0),
  }
}

export function updateStatusPresentation(status = {}) {
  const stage = String(status?.stage || '')
  const error = String(status?.error || '')
  const interrupted = !status?.running && stage === 'error' && /(?:中断|interrupted|重启前未完成)/i.test(`${status?.message || ''} ${error}`)
  const failed = !status?.running && (stage === 'error' || Boolean(error))
  return {
    interrupted,
    failed,
    canRetry: interrupted || failed,
    title: status?.running ? '升级中' : interrupted ? '升级已中断' : failed ? '升级失败' : stage === 'done' ? '升级完成' : '升级状态',
    actionLabel: interrupted ? '重新开始升级' : failed ? '重试升级' : '一键升级',
    detail: [status?.message, error && error !== status?.message ? error : '', stage ? `阶段：${stage}` : '', status?.id ? `任务：${status.id}` : ''].filter(Boolean).join('\n'),
  }
}

export function shouldConfirmFileReplacement({ dirty, loadedPath, nextPath }) {
  if (!dirty) return false
  const current = String(loadedPath || '').trim()
  const next = String(nextPath || '').trim()
  return Boolean(next && (!current || current !== next))
}
