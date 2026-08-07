import { useCallback, useEffect, useMemo, useState } from 'react'
import { Check, ChevronDown, CircleAlert, CircleCheck, FileText, ListTodo, RefreshCw, Search } from 'lucide-react'
import { api } from '../lib/api'
import { filterTodoItems, normalizeTodoOverview, todoItemStatusTone, todoItemsForModule, todoModuleLabel, todoModuleSummary, todoStatusLabel, TODO_MODULES } from '../lib/todos.js'

const COPY = {
  zh: {
    title: '项目待办', source: '打开 TODO 源文件', refresh: '刷新待办', loading: '正在读取项目待办…', retry: '重试', unavailable: '暂未发现 TODO 源文件', sourceHint: 'GA 根目录下的 temp/TODO.txt', moduleTodo: module => `${module}待办`, open: '未完成', completed: '已完成', search: '搜索标题、章节或轮次', expand: '展开全部', collapse: '收起列表', empty: '这个模块暂无待办', noMatch: '没有匹配的待办', recent: '最近待办', all: '全部模块', openCount: count => `${count} 项未完成`, totalCount: count => `共 ${count} 项`, line: line => `源文件第 ${line} 行`, approved: '已批准', status: '状态', completedMark: '已完成', sourceLine: '查看源文件', syncHint: '标题显示已完成，但台账还没有勾选完成，建议复核后同步。', queuedHint: '用户已批准，等待自主服务执行，不代表已经完成。', error: '待办读取失败', sourcePath: '来源', moduleOpen: (open, total) => `${open} 未完成 / ${total} 总计`, needsSync: count => `${count} 项状态待同步`, showing: count => `显示 ${count} 项`, clearSearch: '清空搜索', jump: '进入模块', overview: '查看各模块待办分布',
  },
  en: {
    title: 'Project TODOs', source: 'Open TODO source', refresh: 'Refresh TODOs', loading: 'Loading project TODOs…', retry: 'Retry', unavailable: 'TODO source was not found', sourceHint: 'temp/TODO.txt under the GA root', moduleTodo: module => `${module} TODOs`, open: 'Open', completed: 'Completed', search: 'Search title, section, or round', expand: 'Show all', collapse: 'Collapse', empty: 'No TODOs in this module', noMatch: 'No matching TODOs', recent: 'Recent TODOs', all: 'All modules', openCount: count => `${count} open`, totalCount: count => `${count} total`, line: line => `Source line ${line}`, approved: 'Approved', status: 'Status', completedMark: 'Completed', sourceLine: 'View source', syncHint: 'The title says complete, but the ledger is not checked off. Review before syncing.', queuedHint: 'Approved and waiting for autonomous execution; this does not mean it is complete.', error: 'Could not read TODOs', sourcePath: 'Source', moduleOpen: (open, total) => `${open} open / ${total} total`, needsSync: count => `${count} need status sync`, showing: count => `Showing ${count}`, clearSearch: 'Clear search', jump: 'Open module', overview: 'TODO distribution by module',
  },
}

const copyFor = lang => COPY[lang] || COPY.zh

function StatusMark({ status, statusLabel, lang }) {
  const Icon = status === 'completed' ? CircleCheck : status === 'needs_sync' ? CircleAlert : status === 'queued' ? Check : ListTodo
  const label = todoStatusLabel(status, lang)
  return <span className={`todo-status-mark todo-status-${todoItemStatusTone(status)}`} title={label} aria-label={`${statusLabel}：${label}`}><Icon size={13}/></span>
}

function TodoItem({ item, text, lang, onOpenSource }) {
  const metadata = [item.section, item.round, item.priority].filter(Boolean).join(' · ')
  const hint = item.status === 'needs_sync' ? text.syncHint : ''
  return <li className={`module-todo-item todo-item-${todoItemStatusTone(item.status)}`}>
    <div className="module-todo-item-copy">
      <div className="module-todo-item-title"><StatusMark status={item.status} statusLabel={text.status} lang={lang}/><b title={item.title}>{item.title}</b></div>
      <div className="module-todo-item-meta">{metadata && <span>{metadata}</span>}<span className={`todo-state-label is-${todoItemStatusTone(item.status)}`} title={item.status === 'queued' ? text.queuedHint : ''}>{todoStatusLabel(item.status, lang)}</span>{item.approved && item.status !== 'queued' && <span className="todo-approved">{text.approved}</span>}</div>
      {item.summary && <p>{item.summary}</p>}
      {hint && <small className="module-todo-item-hint">{hint}</small>}
    </div>
    <button type="button" className="todo-source-button" title={`${text.sourceLine}${item.line ? ` · ${text.line(item.line)}` : ''}`} aria-label={`${text.sourceLine}：${item.title}`} onClick={() => onOpenSource?.(item)}><FileText size={15}/></button>
  </li>
}

function TodoToolbar({ text, query, setQuery, showCompleted, setShowCompleted, expanded, setExpanded, onRefresh, loading, onOpenSource, canOpenSource }) {
  return <div className="module-todo-toolbar">
    <label className="module-todo-search"><Search size={15}/><input value={query} onChange={event => setQuery(event.target.value)} placeholder={text.search} aria-label={text.search}/>{query && <button type="button" title={text.clearSearch} aria-label={text.clearSearch} onClick={() => setQuery('')}>×</button>}</label>
    <div className="module-todo-actions">
      <div className="module-todo-filter" role="group" aria-label={text.status}><button type="button" className={!showCompleted ? 'active' : ''} aria-pressed={!showCompleted} onClick={() => setShowCompleted(false)}>{text.open}</button><button type="button" className={showCompleted ? 'active' : ''} aria-pressed={showCompleted} onClick={() => setShowCompleted(true)}>{text.completed}</button></div>
      <button type="button" className="todo-icon-action" title={expanded ? text.collapse : text.expand} aria-label={expanded ? text.collapse : text.expand} aria-pressed={expanded} onClick={() => setExpanded(value => !value)}><ChevronDown size={15} className={expanded ? 'is-expanded' : ''}/></button>
      <button type="button" className="todo-icon-action" title={text.refresh} aria-label={text.refresh} onClick={onRefresh} disabled={loading}><RefreshCw size={15} className={loading ? 'is-spinning' : ''}/></button>
      <button type="button" className="todo-icon-action" title={text.source} aria-label={text.source} onClick={() => onOpenSource?.()} disabled={!canOpenSource}><FileText size={15}/></button>
    </div>
  </div>
}

function TodoList({ items, text, lang, expanded, onOpenSource }) {
  const visible = expanded ? items : items.slice(0, 5)
  if (!items.length) return <p className="module-todo-empty">{text.noMatch}</p>
  return <>
    <ul className="module-todo-list">{visible.map(item => <TodoItem key={item.id} item={item} text={text} lang={lang} onOpenSource={onOpenSource}/>)}</ul>
    {!expanded && items.length > visible.length && <button type="button" className="module-todo-more" onClick={() => onOpenSource?.({ action: 'expand' })}>{text.showing(visible.length)} · {text.expand}</button>}
  </>
}

function ModuleSummary({ overview, text, onNavigate }) {
  const modules = overview.modules.filter(item => item.total > 0)
  return <div className="module-todo-summary-grid" aria-label={text.overview}>
    {modules.map(item => <button type="button" key={item.module} className="module-todo-summary" onClick={() => onNavigate?.(item.module)} title={`${text.jump}：${todoModuleLabel(item.module)}`}>
      <span className="module-todo-summary-head"><b>{todoModuleLabel(item.module)}</b><ChevronDown size={14}/></span>
      <strong>{item.open}</strong><small>{text.moduleOpen(item.open, item.total)}</small>
      {item.needsSync > 0 && <em>{text.needsSync(item.needsSync)}</em>}
    </button>)}
  </div>
}

export function ModuleTodoPanel({ module = 'overview', lang = 'zh', onNavigate, onOpenSource }) {
  const text = copyFor(lang)
  const [overview, setOverview] = useState(() => normalizeTodoOverview(null))
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState('')
  const [query, setQuery] = useState('')
  const [showCompleted, setShowCompleted] = useState(false)
  const [expanded, setExpanded] = useState(false)
  const load = useCallback(async () => {
    setLoading(true)
    setError('')
    try { setOverview(normalizeTodoOverview(await api('/api/todos'))) }
    catch (cause) { setError(cause.message || text.error) }
    finally { setLoading(false) }
  }, [text.error])
  useEffect(() => { load() }, [load])

  const moduleItems = useMemo(() => module === 'overview' ? overview.items : todoItemsForModule(overview, module), [module, overview])
  const filteredItems = useMemo(() => filterTodoItems(moduleItems, { showCompleted, query }), [moduleItems, query, showCompleted])
  const summary = todoModuleSummary(overview, module)
  const openSource = item => onOpenSource?.(item || { sourcePath: overview.sourcePath })
  const isOverview = module === 'overview'
  const overviewItems = useMemo(() => filterTodoItems(overview.items, { showCompleted, query }).slice(0, expanded ? 30 : 5), [expanded, overview.items, query, showCompleted])

  if (loading && !overview.items.length) return <section className="module-todo-panel is-loading" aria-label={text.title}><div className="module-todo-heading"><span><ListTodo size={17}/><b>{isOverview ? text.title : text.moduleTodo(todoModuleLabel(module))}</b></span></div><p className="module-todo-state">{text.loading}</p></section>
  if (error && !overview.items.length) return <section className="module-todo-panel is-error" aria-label={text.title}><div className="module-todo-heading"><span><ListTodo size={17}/><b>{isOverview ? text.title : text.moduleTodo(todoModuleLabel(module))}</b></span><button type="button" className="todo-retry" onClick={load}><RefreshCw size={14}/>{text.retry}</button></div><p className="module-todo-state">{text.error}：{error}</p></section>
  if (!overview.sourceExists) return <section className="module-todo-panel is-empty" aria-label={text.title}><div className="module-todo-heading"><span><ListTodo size={17}/><b>{isOverview ? text.title : text.moduleTodo(todoModuleLabel(module))}</b></span></div><p className="module-todo-state">{text.unavailable}<small>{text.sourceHint}</small></p></section>

  return <section className={`module-todo-panel${isOverview ? ' is-overview' : ''}${expanded ? ' is-expanded' : ''}`} aria-label={isOverview ? text.title : text.moduleTodo(todoModuleLabel(module))}>
    <div className="module-todo-heading"><div><span><ListTodo size={17}/><b>{isOverview ? text.title : text.moduleTodo(todoModuleLabel(module))}</b></span><small>{isOverview ? text.overview : `${summary.open ? text.openCount(summary.open) : text.totalCount(summary.total)}`}</small></div><div className="module-todo-heading-count"><strong>{isOverview ? overview.open : summary.open}</strong><small>/ {isOverview ? overview.total : summary.total}</small></div></div>
    <TodoToolbar text={text} query={query} setQuery={setQuery} showCompleted={showCompleted} setShowCompleted={setShowCompleted} expanded={expanded} setExpanded={setExpanded} onRefresh={load} loading={loading} onOpenSource={openSource} canOpenSource={overview.sourceExists}/>
    {isOverview && <ModuleSummary overview={overview} text={text} onNavigate={onNavigate}/>}
    <div className="module-todo-list-heading"><b>{isOverview ? text.recent : todoModuleLabel(module)}</b><span>{text.showing(isOverview ? overviewItems.length : filteredItems.length)}</span></div>
    <TodoList items={isOverview ? overviewItems : filteredItems} text={text} lang={lang} expanded={expanded} onOpenSource={item => item?.action === 'expand' ? setExpanded(true) : openSource(item)}/>
    <div className="module-todo-foot"><span>{text.sourcePath}: <code>{overview.sourcePath}</code></span><button type="button" onClick={() => openSource()}>{text.source}</button></div>
  </section>
}

export { TODO_MODULES }
