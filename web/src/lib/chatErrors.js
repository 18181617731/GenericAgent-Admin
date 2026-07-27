const HTTP_STATUS_RE = /\bHTTP\s+(\d{3})\b/i
const HTML_TITLE_RE = /<title[^>]*>\s*([\s\S]*?)\s*<\/title>/i

const compactDetail = (value = '') => {
  const raw = String(value || '').trim().replace(/^!!!Error:\s*/i, '')
  if (!raw) return ''
  if (/<!doctype\s+html|<html\b/i.test(raw)) {
    const status = raw.match(HTTP_STATUS_RE)?.[1]
    const title = raw.match(HTML_TITLE_RE)?.[1]?.replace(/<[^>]+>/g, '').replace(/\s+/g, ' ').trim()
    return [status ? `HTTP ${status}` : '', title ? `上游 HTML 错误页：${title}` : '上游返回 HTML 错误页'].filter(Boolean).join('；')
  }
  const normalized = raw.replace(/\s+/g, ' ')
  return normalized.length > 1200 ? `${normalized.slice(0, 1200)}…` : normalized
}

const httpPresentation = (status, detail) => {
  const base = { source:'model_service', sourceLabel:'模型服务', code:`HTTP_${status}`, detail, retryable:false }
  if (status === 401) return { ...base, summary:'模型服务拒绝访问（HTTP 401）', hint:'请检查当前服务商的 API Key 是否正确、是否已过期。' }
  if (status === 403) return { ...base, summary:'模型服务拒绝请求（HTTP 403）', hint:'请检查 API Key 权限、模型权限、来源限制或服务商的安全防护配置。' }
  if (status === 404) return { ...base, summary:'模型服务找不到请求地址或模型（HTTP 404）', hint:'请检查服务商地址、API 路径和模型 ID 是否匹配。' }
  if (status === 408 || status === 504) return { ...base, summary:`模型服务响应超时（HTTP ${status}）`, hint:'可以稍后重试，也可以检查网络和服务商响应状态。', retryable:true }
  if (status === 429) return { ...base, summary:'模型服务请求过于频繁（HTTP 429）', hint:'请稍后重试，或检查服务商的额度与限流策略。', retryable:true }
  if (status >= 500) return { ...base, summary:`模型服务暂时异常（HTTP ${status}）`, hint:'这通常是服务商侧异常，可以稍后重试；若持续发生请检查服务商状态。', retryable:true }
  return { ...base, summary:`模型服务请求失败（HTTP ${status}）`, hint:'请检查服务商配置和当前模型是否可用。' }
}

export function chatErrorPresentation(message = {}) {
  const raw = String(message.content || '')
  const stored = message.error_info && typeof message.error_info === 'object' ? message.error_info : null
  if (stored?.summary) {
    return {
      source:stored.source || 'system',
      sourceLabel:stored.source_label || 'GenericAgent',
      code:stored.code || 'CHAT_ERROR',
      summary:stored.summary,
      hint:stored.hint || '请检查相关配置后重新发送。',
      detail:compactDetail(stored.detail || raw),
      retryable:Boolean(stored.retryable),
    }
  }
  const detail = compactDetail(raw)
  const status = Number(raw.match(HTTP_STATUS_RE)?.[1] || 0)
  if (status) return httpPresentation(status, detail)
  if (/ModuleNotFoundError|ImportError|Traceback|agentmain|llmcore|worker exited|failed to start/i.test(raw)) {
    return { source:'project_runtime', sourceLabel:'项目运行环境', code:'PROJECT_RUNTIME_ERROR', summary:'GenericAgent 项目运行异常，本次对话未完成', hint:'请到“总览”的系统状态检查中查看 Python、依赖和 GA 运行环境；修复后可重新发送。', detail, retryable:false }
  }
  if (/timed?\s*out|connection (?:refused|reset)|\bDNS\b|\bTLS\b|network|\bEOF\b/i.test(raw)) {
    return { source:'network', sourceLabel:'网络连接', code:'NETWORK_ERROR', summary:'无法连接模型服务，本次对话未完成', hint:'请检查网络、代理和服务商地址，确认后可重新发送。', detail, retryable:true }
  }
  return { source:'system', sourceLabel:'GenericAgent', code:'CHAT_ERROR', summary:'对话执行异常，本次回复未完成', hint:'可查看技术详情判断原因，修复相关配置后重新发送。', detail, retryable:true }
}
