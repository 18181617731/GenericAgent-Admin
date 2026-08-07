const DEFAULT_TITLE = { zh: '新会话', en: 'New chat' }
const DEFAULT_PROMPT = { zh: '最近一轮对话', en: 'the latest turn' }

const text = value => String(value ?? '').replace(/\s+/g, ' ').trim()

const trimText = (value, limit = 72) => {
  const normalized = text(value)
  if (normalized.length <= limit) return normalized
  return `${normalized.slice(0, Math.max(1, limit - 1)).trimEnd()}…`
}

const sessionSuffix = sessionId => {
  const value = text(sessionId)
  return value ? value.slice(-6) : ''
}

export const latestUserPrompt = (messages = []) => {
  if (!Array.isArray(messages)) return ''
  const latest = [...messages].reverse().find(message => message?.role === 'user' && text(message?.content))
  return latest?.content || ''
}

export const buildChatNotification = ({ session, sessionId = '', prompt = '', status = 'completed', error = '', lang = 'zh' } = {}) => {
  const language = lang === 'en' ? 'en' : 'zh'
  const promptText = trimText(prompt, 72)
  const rawTitle = trimText(session?.title, 42)
  const placeholderTitles = language === 'en' ? ['New chat', 'Untitled chat'] : ['新会话', '新对话', '未命名会话']
  const title = rawTitle && !placeholderTitles.includes(rawTitle)
    ? rawTitle
    : trimText(promptText, 42) || DEFAULT_TITLE[language]
  const suffix = sessionSuffix(session?.id || sessionId)
  const question = promptText
  const project = trimText(session?.project_mode, 36)
  const failed = status === 'failed'
  const notificationTitle = language === 'en'
    ? `${failed ? 'Chat failed' : 'Chat completed'}: ${title}`
    : `${failed ? '对话执行失败' : '对话已完成'}：${title}`
  const details = failed
    ? (language === 'en' ? `“${title}” could not finish${text(error) ? `: ${trimText(error, 120)}.` : '.'}` : `“${title}”未能完成${text(error) ? `：${trimText(error, 120)}。` : '。'}`)
    : (language === 'en' ? `“${title}” finished replying.` : `“${title}”已完成回复。`)
  const context = language === 'en'
    ? [question ? `Latest question: ${question}` : `Latest turn: ${DEFAULT_PROMPT.en}`, project ? `Project: ${project}` : '', suffix ? `Chat code: ${suffix}` : ''].filter(Boolean).join(' · ')
    : [question ? `刚才的问题：${question}` : `范围：${DEFAULT_PROMPT.zh}`, project ? `项目：${project}` : '', suffix ? `对话尾号：${suffix}` : ''].filter(Boolean).join('；')
  return { title: notificationTitle, message: `${details}${context ? ` ${context}` : ''}` }
}
