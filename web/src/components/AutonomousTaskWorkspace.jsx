import React, { useCallback, useEffect, useMemo, useState } from 'react'
import { Ban, Check, CircleAlert, Copy, Pause, Pencil, Play, Plus, RefreshCw, Search, Sparkles, Square, X } from 'lucide-react'
import { api } from '../lib/api.js'
import { confirmDanger } from '../lib/danger.js'
import { autonomousCopy } from '../lib/autonomousCopy.js'

const STATUS = { draft: '草稿', pending_approval: '待审批', queued: '排队中', running: '执行中', paused: '已暂停', blocked: '已阻塞', failed: '失败', completed: '已完成', cancelled: '已取消' }
const emptyDraft = { title: '', objective: '', priority: 'normal', risk: '待评估', project: '', due_at: '', next_step: '' }
const clamp = value => Math.max(0, Math.min(100, Number(value) || 0))
const statusLabel = status => STATUS[status] || status || '未知'
const dateLabel = (value, zh) => value ? new Date(value).toLocaleString(zh ? 'zh-CN' : 'en-US') : '-'
const isOverdue = task => task.due_at && new Date(task.due_at).getUTCFullYear() > 1 && new Date(task.due_at).getTime() < Date.now() && !['completed', 'cancelled'].includes(task.status)
const isAttention = task => ['blocked', 'paused', 'failed'].includes(task.status) || isOverdue(task)
const taskMetrics = tasks => ({
  total: tasks.length,
  pending: tasks.filter(task => task.status === 'pending_approval').length,
  running: tasks.filter(task => ['queued', 'running'].includes(task.status)).length,
  blocked: tasks.filter(task => ['blocked', 'paused'].includes(task.status)).length,
  failed: tasks.filter(task => task.status === 'failed').length,
  overdue: tasks.filter(isOverdue).length,
  completed: tasks.filter(task => task.status === 'completed').length,
  attention: tasks.filter(isAttention).length,
})

const metricsFromResponse = (summary, tasks) => {
  const fallback = taskMetrics(tasks)
  return Object.keys(fallback).reduce((result, key) => {
    const value = Number(summary?.[key])
    result[key] = Number.isFinite(value) ? value : fallback[key]
    return result
  }, {})
}

const facetsFromTasks = tasks => ({
  sources: [...new Set(tasks.map(task => task.source_type).filter(Boolean))].sort(),
  projects: [...new Set(tasks.map(task => task.project).filter(Boolean))].sort(),
  risks: [...new Set(tasks.map(task => task.risk).filter(Boolean))].sort(),
})

function MetricStrip({ metrics, zh, activeFilter, onFilter }) {
  const tooltip = (zh ? ['待审批', '进行中（含排队）', '需关注', '已完成'] : ['Approval', 'In progress (queued included)', 'Needs attention', 'Completed']).map((label, index) => `${label}: ${[metrics.pending, metrics.running, metrics.attention, metrics.completed][index]}`).join(' · ')
  const cards = [
    { key: 'pending', status: 'pending_approval', label: zh ? '待审批' : 'Approval', value: metrics.pending },
    { key: 'running', status: '__active', label: zh ? '进行中' : 'In progress', value: metrics.running, title: zh ? '包含排队中和执行中的任务' : 'Includes queued and running tasks' },
    { key: 'attention', status: '__attention', label: zh ? '需关注' : 'Needs attention', value: metrics.attention, title: zh ? '包含阻塞、暂停、失败或逾期任务' : 'Includes blocked, paused, failed, or overdue tasks' },
    { key: 'completed', status: 'completed', label: zh ? '已完成' : 'Completed', value: metrics.completed },
  ]
  return <div className="autonomous-task-metrics" title={tooltip}>
    {cards.map(card => <button type="button" key={card.key} title={card.title} className={activeFilter === card.status ? 'is-active' : ''} onClick={() => onFilter?.(activeFilter === card.status ? '' : card.status)}><span>{card.label}</span><b>{card.value}</b></button>)}
  </div>
}

function TaskRow({ task, selected, onOpen, onQuickAction, zh }) {
  const quick = ['draft', 'pending_approval'].includes(task.status) ? { name: 'approve', label: zh ? '批准' : 'Approve' } : task.status === 'failed' ? { name: 'retry', label: zh ? '重试' : 'Retry' } : task.status === 'running' ? { name: 'pause', label: zh ? '暂停' : 'Pause' } : null
  const progress = task.status === 'completed' ? 100 : clamp(task.progress)
  return <div className={`autonomous-task-row${selected ? ' is-selected' : ''}`}>
    <button type="button" className="autonomous-task-row-main-button" onClick={() => onOpen(task)}>
      <span className="autonomous-task-row-main"><span><strong>{task.title}</strong><small>{task.objective || (zh ? '未填写目标' : 'No objective')}</small></span><em className={`autonomous-task-status is-${task.status}`}>{statusLabel(task.status)}</em></span>
      <span className="autonomous-task-row-meta"><span>{task.current_stage || statusLabel(task.status)}</span><span>{progress}%</span></span>
      <span className="autonomous-task-progress"><i style={{ width: `${progress}%` }} /></span>
    </button>
    {quick && <button type="button" className="autonomous-task-row-quick" onClick={() => { onOpen(task); onQuickAction(task, quick.name) }}>{quick.label}</button>}
  </div>
}

function QuickCreate({ zh, onCreate, onEditFull }) {
  const copy = autonomousCopy(zh ? 'zh' : 'en')
  const [input, setInput] = useState('')
  const [phase, setPhase] = useState('idle')
  const [draft, setDraft] = useState(null)
  const [fallback, setFallback] = useState(false)
  const [error, setError] = useState('')
  const parse = async () => {
    const text = input.trim()
    if (!text) return
    setPhase('parsing'); setError('')
    try {
      const result = await api('/api/autonomous/tasks/parse', { dangerous: true, method: 'POST', body: JSON.stringify({ input: text }) })
      setDraft(result.parsed || { title: text, objective: text, priority: 'normal' })
      setFallback(Boolean(result.fallback))
      setPhase('preview')
    } catch (err) {
      setDraft({ title: text, objective: text, priority: 'normal' })
      setFallback(true)
      setPhase('preview')
    }
  }
  const reset = () => { setPhase('idle'); setDraft(null); setFallback(false); setError('') }
  const submit = async () => {
    const payload = { title: draft.title, objective: draft.objective, priority: draft.priority || 'normal', risk: draft.risk || '', project: draft.project || '', due_at: '', next_step: draft.next_step || '' }
    if (!(await confirmDanger('autonomous-task-create', zh ? `创建任务“${payload.title}”？` : `Create task "${payload.title}"?`))) return
    setPhase('creating')
    try { await onCreate(payload); setInput(''); reset() } catch (err) { setError(err.message); setPhase('preview') }
  }
  return <section className="autonomous-quick-create" aria-label={zh ? '一句话创建任务' : 'Quick create task'}>
    {phase === 'preview' ? <div className="autonomous-quick-create-preview">
      <p className="autonomous-quick-create-note">{fallback ? copy.quickCreateFallback : copy.quickCreatePreview}</p>
      <label>{zh ? '标题' : 'Title'}<input maxLength={200} value={draft.title || ''} onChange={event => setDraft(current => ({ ...current, title: event.target.value }))} /></label>
      <label>{zh ? '目标' : 'Objective'}<textarea maxLength={4000} rows={2} value={draft.objective || ''} onChange={event => setDraft(current => ({ ...current, objective: event.target.value }))} /></label>
      {error && <p className="autonomous-quick-create-error" role="alert">{error}</p>}
      <div className="autonomous-quick-create-actions">
        <button type="button" className="primary" disabled={phase === 'creating' || !(draft.title || '').trim()} onClick={submit}><Check size={15} />{copy.quickCreateCreate}</button>
        <button type="button" className="secondary" disabled={phase === 'creating'} onClick={() => { onEditFull({ ...draft, due_at: '' }); setInput(''); reset() }}>{copy.quickCreateEditFull}</button>
        <button type="button" className="secondary" disabled={phase === 'creating'} onClick={reset}>{copy.quickCreateCancel}</button>
      </div>
    </div> : <div className="autonomous-quick-create-input">
      <Sparkles size={16} />
      <input value={input} onChange={event => setInput(event.target.value)} onKeyDown={event => { if (event.key === 'Enter') { event.preventDefault(); parse() } }} maxLength={4000} placeholder={copy.quickCreatePlaceholder} aria-label={copy.quickCreatePlaceholder} />
      <button type="button" className="primary" disabled={!input.trim() || phase === 'parsing'} onClick={parse}>{phase === 'parsing' ? <><RefreshCw size={15} className="spin" />{copy.quickCreateParsing}</> : copy.quickCreate}</button>
    </div>}
  </section>
}

function TaskEditor({ draft, editing, saving, onChange, onClose, onSubmit, zh }) {
  return <section className="autonomous-task-editor" aria-label={editing ? (zh ? '编辑任务' : 'Edit task') : (zh ? '新建自主任务' : 'New autonomous task')}>
    <header><h4>{editing ? (zh ? '编辑任务' : 'Edit task') : (zh ? '新建自主任务' : 'New autonomous task')}</h4><button type="button" className="icon-button" aria-label={zh ? '关闭编辑器' : 'Close editor'} onClick={onClose}><X size={16} /></button></header>
    <form onSubmit={onSubmit}><label>{zh ? '标题' : 'Title'}<input required maxLength={200} value={draft.title} onChange={event => onChange({ title: event.target.value })} /></label><label>{zh ? '目标' : 'Objective'}<textarea maxLength={4000} rows={3} value={draft.objective} onChange={event => onChange({ objective: event.target.value })} /></label><div className="autonomous-task-editor-grid"><label>{zh ? '优先级' : 'Priority'}<select value={draft.priority} onChange={event => onChange({ priority: event.target.value })}><option value="low">{zh ? '低' : 'Low'}</option><option value="normal">{zh ? '普通' : 'Normal'}</option><option value="high">{zh ? '高' : 'High'}</option></select></label><label>{zh ? '风险' : 'Risk'}<input maxLength={80} value={draft.risk} onChange={event => onChange({ risk: event.target.value })} /></label><label>{zh ? '项目' : 'Project'}<input maxLength={160} value={draft.project} onChange={event => onChange({ project: event.target.value })} /></label><label>{zh ? '截止时间' : 'Due'}<input type="datetime-local" value={draft.due_at} onChange={event => onChange({ due_at: event.target.value })} /></label></div><label>{zh ? '下一步' : 'Next step'}<input maxLength={1000} value={draft.next_step} onChange={event => onChange({ next_step: event.target.value })} /></label><footer><button type="submit" className="primary" disabled={saving}><Check size={15} />{zh ? '保存任务' : 'Save task'}</button></footer></form>
  </section>
}

function TaskDetail({ detail, saving, onAction, onEdit, onReject, zh }) {
  if (!detail) return <div className="autonomous-task-placeholder">{zh ? '选择任务查看详情、运行和事件' : 'Select a task to inspect runs and events'}</div>
  const canApprove = ['draft', 'pending_approval'].includes(detail.status)
  const canStart = detail.status === 'queued'
  const canRetry = detail.status === 'failed'
  const canPause = detail.status === 'running'
  const canResume = ['paused', 'blocked'].includes(detail.status)
  const canCancel = !['completed', 'cancelled'].includes(detail.status)
  const events = (detail.events || []).slice(0, 10)
  const runs = detail.runs || []
  const eventTotal = (detail.events || []).length
  return <><header className="autonomous-task-detail-head"><div><span>{zh ? '任务详情' : 'Task detail'}</span><h4>{detail.title}</h4></div><button type="button" className="icon-button" aria-label={zh ? '编辑任务' : 'Edit task'} onClick={() => onEdit(detail)}><Pencil size={16} /></button></header><p className="autonomous-task-objective">{detail.objective || (zh ? '未填写目标' : 'No objective')}</p><dl><div><dt>{zh ? '状态' : 'Status'}</dt><dd>{statusLabel(detail.status)}</dd></div><div><dt>{zh ? '下一步' : 'Next step'}</dt><dd>{detail.next_step || '-'}</dd></div></dl><div className="autonomous-task-actions">{canApprove && <button type="button" className="primary" disabled={saving} onClick={() => onAction(detail, 'approve')}><Check size={15} />{zh ? '批准并排队' : 'Approve'}</button>}{canApprove && <button type="button" className="secondary" disabled={saving} onClick={() => onReject(detail)}><Ban size={15} />{zh ? '拒绝' : 'Reject'}</button>}{canStart && <button type="button" className="secondary" disabled={saving} onClick={() => onAction(detail, 'start')}><Play size={15} />{zh ? '开始执行' : 'Start'}</button>}{canPause && <button type="button" className="secondary" disabled={saving} onClick={() => onAction(detail, 'pause')}><Pause size={15} />{zh ? '请求暂停' : 'Request pause'}</button>}{canResume && <button type="button" className="secondary" disabled={saving} onClick={() => onAction(detail, 'resume')}><Play size={15} />{zh ? '恢复' : 'Resume'}</button>}{canCancel && <button type="button" className="danger" disabled={saving} onClick={() => onAction(detail, 'cancel')}><Square size={15} />{zh ? '取消' : 'Cancel'}</button>}<details className="autonomous-task-more"><summary>{zh ? '更多 ⋯' : 'More ⋯'}</summary><div className="autonomous-task-more-menu">{canRetry && <button type="button" className="secondary" disabled={saving} onClick={() => onAction(detail, 'retry')}><RefreshCw size={15} />{zh ? '重试' : 'Retry'}</button>}<button type="button" className="secondary" disabled={saving} onClick={() => onAction(detail, 'nudge')}><CircleAlert size={15} />{zh ? '催办' : 'Nudge'}</button><button type="button" className="secondary" disabled={saving} onClick={() => onAction(detail, 'duplicate')}><Copy size={15} />{zh ? '复制' : 'Duplicate'}</button></div></details></div><details className="autonomous-task-extra"><summary>{zh ? '详情' : 'Details'}</summary><dl><div><dt>{zh ? '风险/优先级' : 'Risk / priority'}</dt><dd>{detail.risk || '-'} · {detail.priority || 'normal'}</dd></div><div><dt>{zh ? '当前阶段' : 'Current stage'}</dt><dd>{detail.current_stage || '-'}</dd></div><div><dt>{zh ? '来源' : 'Source'}</dt><dd>{detail.source_path || detail.source_type || (zh ? '手动创建' : 'Manual')}</dd></div></dl></details><details className="autonomous-task-runs-events"><summary>{zh ? '运行与事件' : 'Runs & events'}{runs.length || events.length ? ` (${runs.length}/${events.length})` : ''}</summary><section className="autonomous-task-runs"><h5>{zh ? '运行实例' : 'Runs'}</h5>{runs.length ? runs.map(run => <div key={run.id}><b>{run.status}</b><span>{run.stage || '-'}</span><small>{dateLabel(run.updated_at, zh)}</small></div>) : <p>{zh ? '尚未创建运行实例' : 'No runs yet'}</p>}</section><section className="autonomous-task-events"><h5>{zh ? '事件时间线' : 'Event timeline'}</h5>{events.length ? events.map(event => <div key={event.id}><b title={event.type}>{event.type}</b><span>{event.message || '-'}</span><time>{dateLabel(event.created_at, zh)}</time></div>) : <p>{zh ? '暂无事件' : 'No events'}</p>}{eventTotal > events.length && <p className="autonomous-task-events-more">{zh ? `仅显示最近 10 条，共 ${eventTotal} 条` : `Showing latest 10 of ${eventTotal} events`}</p>}</section></details>{detail.report_path && <a className="autonomous-task-report-link" href={`/api/files/read?path=${encodeURIComponent(detail.report_path)}`}>{zh ? '打开关联报告' : 'Open linked report'}</a>}</>
}

export function AutonomousTaskWorkspace({ lang = 'zh' }) {
  const zh = lang !== 'en'
  const [tasks, setTasks] = useState([])
  const [metrics, setMetrics] = useState(() => taskMetrics([]))
  const [facets, setFacets] = useState({ sources: [], projects: [], risks: [] })
  const [visibleTotal, setVisibleTotal] = useState(0)
  const [selected, setSelected] = useState('')
  const [detail, setDetail] = useState(null)
  const [query, setQuery] = useState('')
  const [status, setStatus] = useState('')
  const [source, setSource] = useState('')
  const [project, setProject] = useState('')
  const [risk, setRisk] = useState('')
  const [priority, setPriority] = useState('')
  const [draft, setDraft] = useState(emptyDraft)
  const [editing, setEditing] = useState('')
  const [editorOpen, setEditorOpen] = useState(false)
  const [loading, setLoading] = useState(false)
  const [saving, setSaving] = useState(false)
  const [message, setMessage] = useState('')
  const [rejecting, setRejecting] = useState(null)
  const [rejectNote, setRejectNote] = useState('')
  const refresh = useCallback(async () => {
    setLoading(true)
    try {
      const params = new URLSearchParams()
      const attentionFilter = status === '__attention'
      const activeFilter = status === '__active'
      if (query) params.set('q', query)
      if (status && !attentionFilter && !activeFilter) params.set('status', status)
      if (source) params.set('source', source)
      if (project) params.set('project', project)
      if (risk) params.set('risk', risk)
      if (priority) params.set('priority', priority)
      const result = await api(`/api/autonomous/tasks${params.toString() ? `?${params}` : ''}`)
      const rawTasks = Array.isArray(result.tasks) ? result.tasks : []
      let nextTasks = rawTasks
      if (attentionFilter) nextTasks = nextTasks.filter(isAttention)
      if (activeFilter) nextTasks = nextTasks.filter(task => ['queued', 'running'].includes(task.status))
      setTasks(nextTasks)
      setMetrics(metricsFromResponse(result.summary, rawTasks))
      const reportedTotal = Number(result.filtered_total ?? result.total)
      setVisibleTotal(!attentionFilter && !activeFilter && Number.isFinite(reportedTotal) ? reportedTotal : nextTasks.length)
      const responseFacets = result.facets
      setFacets(responseFacets && typeof responseFacets === 'object'
        ? { sources: Array.isArray(responseFacets.sources) ? responseFacets.sources : [], projects: Array.isArray(responseFacets.projects) ? responseFacets.projects : [], risks: Array.isArray(responseFacets.risks) ? responseFacets.risks : [] }
        : facetsFromTasks(rawTasks))
      if (selected && !nextTasks.some(task => task.id === selected)) { setSelected(''); setDetail(null) }
    } catch (error) { setMessage(error.message) } finally { setLoading(false) }
  }, [priority, project, query, risk, selected, source, status])
  useEffect(() => { refresh() }, [refresh])
  const openTask = async task => { setSelected(task.id); setDetail(task); try { const result = await api(`/api/autonomous/tasks/${encodeURIComponent(task.id)}`); setDetail(result.task ? { ...result.task, events: result.events || [], runs: result.runs || [] } : task) } catch (error) { setMessage(error.message) } }
  const onAction = async (task, name, note = '') => { const labels = { approve: '批准', reject: '拒绝', start: '开始执行', pause: '请求暂停', resume: '恢复', cancel: '取消', retry: '重试', nudge: '催办', duplicate: '复制' }; if (!(await confirmDanger(`autonomous-task-${name}`, `${zh ? labels[name] : name}${zh ? `任务“${task.title}”？` : ` task "${task.title}"?`}`))) return; setSaving(true); try { await api(`/api/autonomous/tasks/${encodeURIComponent(task.id)}/${name}`, { dangerous: true, method: 'POST', body: JSON.stringify({ note }) }); await refresh(); await openTask(task); setMessage(zh ? '操作已记录' : 'Action recorded') } catch (error) { setMessage(error.message) } finally { setSaving(false) } }
  const confirmReject = async () => { if (!rejecting) return; const task = rejecting; setRejecting(null); const note = rejectNote; setRejectNote(''); await onAction(task, 'reject', note) }
  const saveDraft = async event => { event.preventDefault(); if (!draft.title.trim()) return; if (!(await confirmDanger(editing ? 'autonomous-task-update' : 'autonomous-task-create', editing ? (zh ? `保存任务“${draft.title}”的修改？` : `Save changes to "${draft.title}"?`) : (zh ? `创建任务“${draft.title}”？` : `Create task "${draft.title}"?`)))) return; setSaving(true); try { const payload = { ...draft, due_at: draft.due_at ? new Date(draft.due_at).toISOString() : '' }; const result = editing ? await api(`/api/autonomous/tasks/${encodeURIComponent(editing)}`, { dangerous: true, method: 'PUT', body: JSON.stringify(payload) }) : await api('/api/autonomous/tasks', { dangerous: true, method: 'POST', body: JSON.stringify(payload) }); setDraft(emptyDraft); setEditing(''); setEditorOpen(false); await refresh(); if (result.task) await openTask(result.task); setMessage(zh ? '任务已保存' : 'Task saved') } catch (error) { setMessage(error.message) } finally { setSaving(false) } }
  const fallbackFacets = useMemo(() => facetsFromTasks(tasks), [tasks])
  const sources = facets.sources.length ? facets.sources : fallbackFacets.sources
  const projects = facets.projects.length ? facets.projects : fallbackFacets.projects
  const risks = facets.risks.length ? facets.risks : fallbackFacets.risks
  const hasFilters = Boolean(query || status || source || project || risk || priority)
  const clearFilters = () => { setQuery(''); setStatus(''); setSource(''); setProject(''); setRisk(''); setPriority('') }
  const editTask = task => { setEditing(task.id); setEditorOpen(true); setDraft({ title: task.title || '', objective: task.objective || '', priority: task.priority || 'normal', risk: task.risk || '', project: task.project || '', due_at: task.due_at ? task.due_at.slice(0, 16) : '', next_step: task.next_step || '' }) }
  const advancedCount = [source, project, risk, priority].filter(Boolean).length
  const createTask = async payload => { const result = await api('/api/autonomous/tasks', { dangerous: true, method: 'POST', body: JSON.stringify(payload) }); await refresh(); if (result.task) await openTask(result.task); setMessage(zh ? '任务已创建' : 'Task created') }
  return <section className="autonomous-task-workspace" aria-label={zh ? '自主任务工作台' : 'Autonomous task workspace'}><header className="autonomous-task-head"><div><h3>{zh ? '所有自主任务' : 'All autonomous tasks'}</h3><p>{zh ? '上方统计固定按全部任务计算；搜索和筛选只影响下方列表。' : 'Headline counts always cover all tasks; search and filters only change the list below.'}</p></div><button type="button" className="primary" onClick={() => { setEditing(''); setEditorOpen(true); setDraft(emptyDraft) }}><Plus size={16} />{zh ? '新建任务' : 'New task'}</button></header><QuickCreate zh={zh} onCreate={createTask} onEditFull={values => { setEditing(''); setEditorOpen(true); setDraft({ ...emptyDraft, ...values }) }} /><MetricStrip metrics={metrics} zh={zh} activeFilter={status} onFilter={next => setStatus(next)} /><div className="autonomous-task-toolbar"><label><Search size={15} /><input value={query} onChange={event => setQuery(event.target.value)} placeholder={zh ? '搜索任务、目标或项目' : 'Search tasks, objectives, or projects'} aria-label={zh ? '搜索任务' : 'Search tasks'} /></label><select value={status} onChange={event => setStatus(event.target.value)} aria-label={zh ? '任务状态' : 'Task status'}><option value="">{zh ? '全部状态' : 'All statuses'}</option>{status === '__active' && <option value="__active">{zh ? '状态：进行中（含排队）' : 'In progress (queued included)'}</option>}{status === '__attention' && <option value="__attention">{zh ? '状态：需关注' : 'Needs attention'}</option>}{Object.entries(STATUS).map(([key, label]) => <option key={key} value={key}>{zh ? `状态：${label}` : key}</option>)}</select><details className="autonomous-task-advanced-filter"><summary>{advancedCount > 0 ? (zh ? `高级筛选 (${advancedCount})` : `Filters (${advancedCount})`) : (zh ? '高级筛选' : 'Filters')}</summary><div className="autonomous-task-advanced-filter-body"><select value={source} onChange={event => setSource(event.target.value)} aria-label={zh ? '任务来源' : 'Task source'}><option value="">{zh ? '全部来源' : 'All sources'}</option>{sources.map(item => <option key={item} value={item}>{item}</option>)}</select><select value={project} onChange={event => setProject(event.target.value)} aria-label={zh ? '项目' : 'Project'}><option value="">{zh ? '全部项目' : 'All projects'}</option>{projects.map(item => <option key={item} value={item}>{item}</option>)}</select><select value={risk} onChange={event => setRisk(event.target.value)} aria-label={zh ? '风险' : 'Risk'}><option value="">{zh ? '全部风险' : 'All risks'}</option>{risks.map(item => <option key={item} value={item}>{item}</option>)}</select><select value={priority} onChange={event => setPriority(event.target.value)} aria-label={zh ? '优先级' : 'Priority'}><option value="">{zh ? '全部优先级' : 'All priorities'}</option><option value="low">{zh ? '优先级：低' : 'Priority: low'}</option><option value="normal">{zh ? '优先级：普通' : 'Priority: normal'}</option><option value="high">{zh ? '优先级：高' : 'Priority: high'}</option></select></div></details><button type="button" className="secondary" onClick={refresh} disabled={loading}><RefreshCw size={15} className={loading ? 'spin' : ''} />{zh ? '刷新' : 'Refresh'}</button></div><div className="autonomous-task-list-summary"><strong>{zh ? '任务列表' : 'Task list'}</strong><span>{zh ? `显示 ${visibleTotal} / 共 ${metrics.total} 项` : `Showing ${visibleTotal} of ${metrics.total} tasks`}</span>{hasFilters && <span>{zh ? '当前筛选只影响列表' : 'Filters affect the list only'}</span>}</div>{message && <div className="autonomous-task-message" role="status">{message}<button type="button" className="icon-button" aria-label={zh ? '关闭消息' : 'Close message'} onClick={() => setMessage('')}><X size={15} /></button></div>}<div className="autonomous-task-layout"><div className="autonomous-task-list">{tasks.length === 0 && <div className="autonomous-empty"><span>{loading ? (zh ? '正在读取任务…' : 'Loading tasks…') : hasFilters ? (zh ? '没有匹配当前筛选的任务。' : 'No tasks match the current filters.') : (zh ? '暂无任务。可以新建任务，或等待系统从 TODO/报告导入。' : 'No tasks yet. Create one, or import from TODOs and reports.')}</span>{!loading && hasFilters && <div className="autonomous-task-empty-actions"><button type="button" className="secondary" onClick={clearFilters}>{zh ? '清除筛选' : 'Clear filters'}</button></div>}</div>}{tasks.map(task => <TaskRow key={task.id} task={task} selected={selected === task.id} onOpen={openTask} onQuickAction={onAction} zh={zh} />)}</div><aside className="autonomous-task-detail"><TaskDetail detail={detail} saving={saving} onAction={onAction} onEdit={editTask} onReject={task => { setRejecting(task); setRejectNote('') }} zh={zh} /></aside></div>{editorOpen && <TaskEditor draft={draft} editing={Boolean(editing)} saving={saving} onChange={values => setDraft(current => ({ ...current, ...values }))} onClose={() => { setEditing(''); setEditorOpen(false); setDraft(emptyDraft) }} onSubmit={saveDraft} zh={zh} />}{rejecting && <div className="autonomous-dialog-backdrop" role="presentation" onMouseDown={event => { if (event.target === event.currentTarget) setRejecting(null) }}><div className="autonomous-dialog" role="dialog" aria-modal="true" aria-labelledby="autonomous-task-reject-title"><header><b id="autonomous-task-reject-title">{zh ? `拒绝任务：${rejecting.title}` : `Reject task: ${rejecting.title}`}</b><button type="button" aria-label={zh ? '取消' : 'Cancel'} onClick={() => setRejecting(null)}><X size={18} /></button></header><label>{zh ? '拒绝意见（可选）' : 'Rejection note (optional)'}<textarea maxLength={1000} value={rejectNote} onChange={event => setRejectNote(event.target.value)} /></label><footer><button type="button" className="secondary" onClick={() => setRejecting(null)}>{zh ? '取消' : 'Cancel'}</button><button type="button" className="danger" disabled={saving} onClick={confirmReject}>{zh ? '确认拒绝' : 'Confirm reject'}</button></footer></div></div>}</section>
}

export default AutonomousTaskWorkspace
