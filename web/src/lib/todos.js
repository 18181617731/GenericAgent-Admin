export const TODO_MODULES = ['overview', 'notifications', 'tasks', 'autonomous', 'goals', 'models', 'files', 'memory', 'channels', 'usage', 'settings', 'logs']
const MODULE_LABELS = {
  zh: { overview: '总览', notifications: '消息通知', tasks: '定时任务', autonomous: '自主进化', goals: 'Goal 模式', models: '模型', files: '文件', memory: '记忆', channels: '通道', usage: '用量总览', settings: '配置', logs: '日志' },
  en: { overview: 'Overview', notifications: 'Notifications', tasks: 'Scheduled tasks', autonomous: 'Autonomous', goals: 'Goal Mode', models: 'Models', files: 'Files', memory: 'Memory', channels: 'Channels', usage: 'Usage', settings: 'Settings', logs: 'Logs' },
}

const STATUS_LABELS = {
  zh: { pending: '待规划', queued: '已批准，等待执行', needs_sync: '标题已完成，状态待同步', completed: '已完成' },
  en: { pending: 'Planned', queued: 'Approved; waiting to run', needs_sync: 'Title says done; status needs sync', completed: 'Completed' },
}

const asText = value => String(value ?? '').trim()

export const todoModuleLabel = (module, lang = 'zh') => MODULE_LABELS[lang]?.[module] || MODULE_LABELS.zh[module] || module || (lang === 'zh' ? '其他' : 'Other')

export const todoStatusLabel = (status, lang = 'zh') => STATUS_LABELS[lang]?.[status] || STATUS_LABELS.zh[status] || (lang === 'zh' ? '待确认' : 'Needs review')

export const normalizeTodoOverview = payload => {
  const source = payload && typeof payload === 'object' ? payload : {}
  const items = Array.isArray(source.items) ? source.items.map((item, index) => ({
    id: asText(item?.id) || `todo-${index}`,
    title: asText(item?.title) || (item?.source_path ? asText(item.source_path) : '未命名待办'),
    summary: asText(item?.summary),
    section: asText(item?.section),
    status: ['pending', 'queued', 'needs_sync', 'completed'].includes(item?.status) ? item.status : 'pending',
    module: TODO_MODULES.includes(item?.module) ? item.module : 'autonomous',
    round: asText(item?.round),
    priority: asText(item?.priority),
    approved: item?.approved === true,
    sourcePath: asText(item?.source_path) || 'temp/TODO.txt',
    line: Number.isInteger(item?.line) ? item.line : 0,
  })) : []
  const modules = Array.isArray(source.modules) ? source.modules.map(module => ({
    module: TODO_MODULES.includes(module?.module) ? module.module : 'autonomous',
    total: Math.max(0, Number(module?.total) || 0),
    open: Math.max(0, Number(module?.open) || 0),
    completed: Math.max(0, Number(module?.completed) || 0),
    needsSync: Math.max(0, Number(module?.needs_sync) || 0),
  })) : []
  return {
    sourcePath: asText(source.source_path) || 'temp/TODO.txt',
    sourceExists: source.source_exists === true,
    sourceTruncated: source.source_truncated === true,
    updatedAt: asText(source.updated_at),
    generatedAt: asText(source.generated_at),
    total: Math.max(0, Number(source.total) || items.length),
    open: Math.max(0, Number(source.open) || items.filter(todoIsOpen).length),
    completed: Math.max(0, Number(source.completed) || items.filter(item => item.status === 'completed').length),
    items,
    modules,
  }
}

export const todoIsOpen = item => item?.status !== 'completed'

export const todoItemsForModule = (overview, module) => (overview?.items || []).filter(item => item.module === module)

export const filterTodoItems = (items, { showCompleted = false, query = '' } = {}) => {
  const needle = asText(query).toLocaleLowerCase()
  return (Array.isArray(items) ? items : []).filter(item => {
    if (showCompleted ? item.status !== 'completed' : !todoIsOpen(item)) return false
    if (!needle) return true
    return [item.title, item.summary, item.section, item.round, item.priority, todoStatusLabel(item.status, 'zh')]
      .some(value => asText(value).toLocaleLowerCase().includes(needle))
  })
}

export const todoItemStatusTone = status => ({ completed: 'done', queued: 'queued', needs_sync: 'sync', pending: 'pending' }[status] || 'pending')

export const todoModuleSummary = (overview, module) => (overview?.modules || []).find(item => item.module === module) || { module, total: 0, open: 0, completed: 0, needsSync: 0 }
