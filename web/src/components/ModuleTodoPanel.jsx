import { useCallback, useEffect, useMemo, useState } from 'react'
import { Check, ChevronDown, CircleAlert, CircleCheck, FileText, ListTodo, RefreshCw, Search } from 'lucide-react'
import { api } from '../lib/api'
import { filterTodoItems, normalizeTodoOverview, todoItemStatusTone, todoItemsForModule, todoModuleLabel, todoModuleSummary, todoOverviewStatusSummary, todoStatusLabel, TODO_MODULES } from '../lib/todos.js'

const COPY = {
  zh: {
    title: '项目待办', source: '打开 TODO 源文件', refresh: '刷新待办', loading: '正在读取项目待办…', retry: '重试', unavailable: '暂未发现 TODO 源文件', sourceHint: 'GA 根目录下的 temp/TODO.txt', moduleTodo: module => `${module}待办`, open: '未完成', completed: '已完成', search: '搜索标题、章节或轮次', expand: '展开全部', collapse: '收起列表', empty: '这个模块暂无待办', noMatch: '没有匹配的待办', recent: '待办清单', summaryTitle: '按处理状态', summaryHint: '模块仅作为条目标签，不再作为总览分类', statusGroups: { pending: '待处理', queued: '待执行', needs_sync: '待同步', completed: '已完成' }, statusCount: count => `${count} 项`, openCount: count => `${count} 项未完成`, totalCount: count => `共 ${count} 项`, line: line => `源文件第 ${line} 行`, approved: '已批准', status: '状态', completedMark: '已完成', sourceLine: '查看源文件', syncHint: '标题显示已完成，但台账还没有勾选完成，建议复核后同步。', queuedHint: '用户已批准，等待自主服务执行，不代表已经完成。', error: '待办读取失败', sourcePath: '来源', moduleOpen: (open, total) => `${open} 未完成 / ${total} 总计`, needsSync: count => `${count} 项状态待同步`, showing: count => `显示 ${count} 项`, clearSearch: '清空搜索', overview: '按处理状态查看待办',
  },
  en: {
    title: 'Project TODOs', source: 'Open TODO source', refresh: 'Refresh TODOs', loading: 'Loading project TODOs…', retry: 'Retry', unavailable: 'TODO source was not found', sourceHint: 'temp/TODO.txt under the GA root', moduleTodo: module => `${module} TODOs`, open: 'Open', completed: 'Completed', search: 'Search title, section, or round', expand: 'Show all', collapse: 'Collapse', empty: 'No TODOs in this module', noMatch: 'No matching TODOs', recent: 'TODO list', summaryTitle: 'By processing status', summaryHint: 'Modules stay as item labels instead of overview categories', statusGroups: { pending: 'To do', queued: 'Queued', needs_sync: 'Needs sync', completed: 'Completed' }, statusCount: count => `${count} items`, openCount: count => `${count} open`, totalCount: count => `${count} total`, line: line => `Source line ${line}`, approved: 'Approved', status: 'Status', completedMark: 'Completed', sourceLine: 'View source', syncHint: 'The title says complete, but the ledger is not checked off. Review before syncing.', queuedHint: 'Approved and waiting for autonomous execution; this does not mean it is complete.', error: 'Could not read TODOs', sourcePath: 'Source', moduleOpen: (open, total) => `${open} open / ${total} total`, needsSync: count => `${count} need status sync`, showing: count => `Showing ${count}`, clearSearch: 'Clear search', overview: 'View TODOs by processing status',
  },
}

const copyFor = lang => COPY[lang] || COPY.zh

function StatusMark({ status, statusLabel, lang }) {
  const Icon = status === 'completed' ? CircleCheck : status === 'needs_sync' ? CircleAlert : status === 'queued' ? Check : ListTodo
  const label = todoStatusLabel(status, lang)
  return <span className={`todo-status-mark todo-status-${todoItemStatusTone(status)}`} title={label} aria-label={`${statusLabel}：${label}`}><Icon size={13}/></span>
}

function TodoItem({ item, text, lang, onOpenSource, showModule = false }) {
  const metadata = [item.section, item.round, item.priority].filter(Boolean).join(' · ')
  const hint = item.status === 'needs_sync' ? text.syncHint : ''
  return <li className={`module-todo-item todo-item-${todoItemStatusTone(item.status)}`}>
    <div className="module-todo-item-copy">
      <div className="module-todo-item-title"><StatusMark status={item.status} statusLabel={text.status} lang={lang}/><b title={item.title}>{item.title}</b></div>
      <div className="module-todo-item-meta">{showModule && <span className="todo-module-label">{todoModuleLabel(item.module, lang)}</span>}{metadata && <span>{metadata}</span>}<span className={`todo-state-label is-${todoItemStatusTone(item.status)}`} title={item.status === 'queued' ? text.queuedHint : ''}>{todoStatusLabel(item.status, lang)}</span>{item.approved && item.status !== 'queued' && <span className="todo-approved">{text.approved}</span>}</div>
      {item.summary && <p>{item.summary}</p>}
      {hint && <small className="module-todo-item-hint">{hint}</small>}
    </div>
    <button type="button" className="todo-source-button" title={`${text.sourceLine}${item.line ? ` · ${text.line(item.line)}` : ''}`} aria-label={`${text.sourceLine}：${item.title}`} onClick={() => onOpenSource?.(item)}><FileText size={15}/></button>
  </li>
}

function TodoToolbar({ text, query, setQuery, statusFilter, setStatusFilter, expanded, setExpanded, onRefresh, loading, onOpenSource, canOpenSource }) {
  return <div className="module-todo-toolbar">
    <label className="module-todo-search"><Search size={15}/><input value={query} onChange={event => setQuery(event.target.value)} placeholder={text.search} aria-label={text.search}/>{query && <button type="button" title={text.clearSearch} aria-label={text.clearSearch} onClick={() => setQuery('')}>×</button>}</label>
    <div className="module-todo-actions">
      <div className="module-todo-filter" role="group" aria-label={text.status}><button type="button" className={statusFilter === 'open' ? 'active' : ''} aria-pressed={statusFilter === 'open'} onClick={() => setStatusFilter('open')}>{text.open}</button><button type="button" className={statusFilter === 'completed' ? 'active' : ''} aria-pressed={statusFilter === 'completed'} onClick={() => setStatusFilter('completed')}>{text.completed}</button></div>
      <button type="button" className="todo-icon-action" title={expanded ? text.collapse : text.expand} aria-label={expanded ? text.collapse : text.expand} aria-pressed={expanded} onClick={() => setExpanded(value => !value)}><ChevronDown size={15} className={expanded ? 'is-expanded' : ''}/></button>
      <button type="button" className="todo-icon-action" title={text.refresh} aria-label={text.refresh} onClick={onRefresh} disabled={loading}><RefreshCw size={15} className={loading ? 'is-spinning' : ''}/></button>
      <button type="button" className="todo-icon-action" title={text.source} aria-label={text.source} onClick={() => onOpenSource?.()} disabled={!canOpenSource}><FileText size={15}/></button>
    </div>
  </div>
}

function TodoList({ items, text, lang, expanded, onOpenSource, showModule = false }) {
  const visible = expanded ? items : items.slice(0, 5)
  if (!items.length) return <p className="module-todo-empty">{text.noMatch}</p>
  return <>
    <ul className="module-todo-list">{visible.map(item => <TodoItem key={item.id} item={item} text={text} lang={lang} onOpenSource={onOpenSource} showModule={showModule}/>)}</ul>
    {!expanded && items.length > visible.length && <button type="button" className="module-todo-more" onClick={() => onOpenSource?.({ action: 'expand' })}>{text.showing(visible.length)} · {text.expand}</button>}
  </>
}

function ModuleSummary({ overview, text, statusFilter, onSelect }) {
  const statuses = todoOverviewStatusSummary(overview)
  return <div className="module-todo-status-summary" aria-label={text.overview}>
    <div className="module-todo-status-summary-heading"><b>{text.summaryTitle}</b><span>{text.summaryHint}</span></div>
    <div className="module-todo-status-summary-grid">
      {statuses.map(item => <button type="button" key={item.status} className={`module-todo-status-card is-${todoItemStatusTone(item.status)}${statusFilter === item.status ? ' is-active' : ''}`} onClick={() => onSelect(item.status)} aria-pressed={statusFilter === item.status} aria-label={`${text.statusGroups[item.status]}：${text.statusCount(item.count)}`}>
        <span className="module-todo-status-card-head"><b>{text.statusGroups[item.status]}</b><ChevronDown size={14}/></span>
        <strong>{item.count}</strong><small>{text.statusCount(item.count)}</small>
      </button>)}
    </div>
  </div>
}

export function ModuleTodoPanel({ module = 'overview', lang = 'zh', onOpenSource }) {
  const text = copyFor(lang)
  const [overview, setOverview] = useState(() => normalizeTodoOverview(null))
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState('')
  const [query, setQuery] = useState('')
  const [statusFilter, setStatusFilter] = useState('open')
  const [expanded, setExpanded] = useState(false)
  const load = useCallback(async () => {
    setLoading(true)
    setError('')
    try { setOverview(normalizeTodoOverview(await api('/api/todos'))) }
    catch (cause) { setError(cause.message || text.error) }
    finally { setLoading(false) }
  }, [text.error])
  useEffect(() => { load() }, [load])
  useEffect(() => { setStatusFilter('open') }, [module])

  const moduleItems = useMemo(() => module === 'overview' ? overview.items : todoItemsForModule(overview, module), [module, overview])
  const filter = useMemo(() => statusFilter === 'open' ? { query } : statusFilter === 'completed' ? { query, showCompleted: true } : { query, status: statusFilter }, [query, statusFilter])
  const filteredItems = useMemo(() => filterTodoItems(moduleItems, filter), [filter, moduleItems])
  const summary = todoModuleSummary(overview, module)
  const openSource = item => onOpenSource?.(item || { sourcePath: overview.sourcePath })
  const isOverview = module === 'overview'
  const overviewItems = useMemo(() => filteredItems.slice(0, expanded ? 30 : 5), [expanded, filteredItems])

  if (loading && !overview.items.length) return <section className="module-todo-panel is-loading" aria-label={text.title}><div className="module-todo-heading"><span><ListTodo size={17}/><b>{isOverview ? text.title : text.moduleTodo(todoModuleLabel(module))}</b></span></div><p className="module-todo-state">{text.loading}</p></section>
  if (error && !overview.items.length) return <section className="module-todo-panel is-error" aria-label={text.title}><div className="module-todo-heading"><span><ListTodo size={17}/><b>{isOverview ? text.title : text.moduleTodo(todoModuleLabel(module))}</b></span><button type="button" className="todo-retry" onClick={load}><RefreshCw size={14}/>{text.retry}</button></div><p className="module-todo-state">{text.error}：{error}</p></section>
  if (!overview.sourceExists) return <section className="module-todo-panel is-empty" aria-label={text.title}><div className="module-todo-heading"><span><ListTodo size={17}/><b>{isOverview ? text.title : text.moduleTodo(todoModuleLabel(module))}</b></span></div><p className="module-todo-state">{text.unavailable}<small>{text.sourceHint}</small></p></section>

  return <section className={`module-todo-panel${isOverview ? ' is-overview' : ''}${expanded ? ' is-expanded' : ''}`} aria-label={isOverview ? text.title : text.moduleTodo(todoModuleLabel(module))}>
    <div className="module-todo-heading"><div><span><ListTodo size={17}/><b>{isOverview ? text.title : text.moduleTodo(todoModuleLabel(module))}</b></span><small>{isOverview ? text.overview : `${summary.open ? text.openCount(summary.open) : text.totalCount(summary.total)}`}</small></div><div className="module-todo-heading-count"><strong>{isOverview ? overview.open : summary.open}</strong><small>/ {isOverview ? overview.total : summary.total}</small></div></div>
    <TodoToolbar text={text} query={query} setQuery={setQuery} statusFilter={statusFilter} setStatusFilter={setStatusFilter} expanded={expanded} setExpanded={setExpanded} onRefresh={load} loading={loading} onOpenSource={openSource} canOpenSource={overview.sourceExists}/>
    {isOverview && <ModuleSummary overview={overview} text={text} statusFilter={statusFilter} onSelect={setStatusFilter}/>}
    <div className="module-todo-list-heading"><b>{isOverview ? text.recent : todoModuleLabel(module)}</b><span>{text.showing(isOverview ? overviewItems.length : filteredItems.length)}</span></div>
    <TodoList items={isOverview ? overviewItems : filteredItems} text={text} lang={lang} expanded={expanded} showModule={isOverview} onOpenSource={item => item?.action === 'expand' ? setExpanded(true) : openSource(item)}/>
    <div className="module-todo-foot"><span>{text.sourcePath}: <code>{overview.sourcePath}</code></span><button type="button" onClick={() => openSource()}>{text.source}</button></div>
  </section>
}

export { TODO_MODULES }
