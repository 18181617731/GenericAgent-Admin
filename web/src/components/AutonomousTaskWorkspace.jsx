import React, { useCallback, useEffect, useMemo, useState } from 'react'
import { Check, Plus, RefreshCw, Search, Sparkles, X } from 'lucide-react'
import { api } from '../lib/api.js'
import { confirmDanger } from '../lib/danger.js'
import { autonomousCopy } from '../lib/autonomousCopy.js'

const STATUS = Object.freeze({
  pending_approval: { zh: '待批准', en: 'Pending' },
  queued: { zh: '排队中', en: 'Queued' },
  completed: { zh: '已闭环', en: 'Closed' },
})
const STATUS_KEYS = Object.keys(STATUS)
const emptyDraft = { title: '', objective: '', next_step: '' }

const publicStatus = status => STATUS[status] ? status : 'pending_approval'
const statusLabel = (status, zh) => STATUS[publicStatus(status)][zh ? 'zh' : 'en']
const clamp = value => Math.max(0, Math.min(100, Number(value) || 0))
const dateLabel = (value, zh) => value ? new Date(value).toLocaleString(zh ? 'zh-CN' : 'en-US') : '-'

const taskMetrics = tasks => STATUS_KEYS.reduce((metrics, status) => {
  metrics[status] = tasks.filter(task => publicStatus(task.status) === status).length
  return metrics
}, {})

function MetricStrip({ metrics, zh, activeFilter, onFilter }) {
  const cards = STATUS_KEYS.map(status => ({
    status,
    label: statusLabel(status, zh),
    value: metrics[status] || 0,
  }))
  return <div className="autonomous-task-metrics" title={cards.map(card => `${card.label}: ${card.value}`).join(' · ')}>
    {cards.map(card => <button type="button" key={card.status} className={activeFilter === card.status ? 'is-active' : ''} onClick={() => onFilter?.(activeFilter === card.status ? '' : card.status)}>
      <span>{card.label}</span><b>{card.value}</b>
    </button>)}
  </div>
}

function TaskRow({ task, selected, onOpen, onApprove, zh }) {
  const status = publicStatus(task.status)
  const canApprove = status === 'pending_approval'
  return <div className={`autonomous-task-row${selected ? ' is-selected' : ''}`}>
    <button type="button" className="autonomous-task-row-main-button" onClick={() => onOpen(task)}>
      <span className="autonomous-task-row-main">
        <span><strong>{task.title}</strong><small>{task.objective || (zh ? '未填写目标' : 'No objective')}</small></span>
        <em className={`autonomous-task-status is-${status}`}>{statusLabel(status, zh)}</em>
      </span>
      <span className="autonomous-task-row-meta"><span>{statusLabel(status, zh)}</span><span>{clamp(task.progress)}%</span></span>
      <span className="autonomous-task-progress"><i style={{ width: `${clamp(task.progress)}%` }} /></span>
    </button>
    {canApprove && <button type="button" className="autonomous-task-row-quick" onClick={() => onApprove(task)}>{zh ? '批准' : 'Approve'}</button>}
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
    setPhase('parsing')
    setError('')
    try {
      const result = await api('/api/autonomous/tasks/parse', { dangerous: true, method: 'POST', body: JSON.stringify({ input: text }) })
      setDraft(result.parsed || { title: text, objective: text })
      setFallback(Boolean(result.fallback))
    } catch {
      setDraft({ title: text, objective: text })
      setFallback(true)
    }
    setPhase('preview')
  }

  const reset = () => { setPhase('idle'); setDraft(null); setFallback(false); setError('') }
  const submit = async () => {
    const payload = { title: draft.title, objective: draft.objective, next_step: draft.next_step || '' }
    if (!await confirmDanger('autonomous-task-create', zh ? `创建任务“${payload.title}”？` : `Create task "${payload.title}"?`)) return
    setPhase('creating')
    try { await onCreate(payload); setInput(''); reset() } catch (err) { setError(err.message); setPhase('preview') }
  }

  if (phase === 'preview') return <section className="autonomous-quick-create" aria-label={zh ? '一句话创建任务' : 'Quick create task'}>
    <div className="autonomous-quick-create-preview">
      <p className="autonomous-quick-create-note">{fallback ? (zh ? '智能解析暂不可用，将按原文创建任务。' : 'Smart parsing is unavailable; the task will use your text.') : copy.quickCreatePreview}</p>
      <label>{zh ? '标题' : 'Title'}<input maxLength={200} value={draft.title || ''} onChange={event => setDraft(current => ({ ...current, title: event.target.value }))} /></label>
      <label>{zh ? '目标' : 'Objective'}<textarea maxLength={4000} rows={2} value={draft.objective || ''} onChange={event => setDraft(current => ({ ...current, objective: event.target.value }))} /></label>
      {error && <p className="autonomous-quick-create-error" role="alert">{error}</p>}
      <div className="autonomous-quick-create-actions">
        <button type="button" className="primary" disabled={phase === 'creating' || !(draft.title || '').trim()} onClick={submit}><Check size={15} />{zh ? '创建任务' : 'Create task'}</button>
        <button type="button" className="secondary" disabled={phase === 'creating'} onClick={() => { onEditFull({ ...draft }); setInput(''); reset() }}>{zh ? '继续填写' : 'Continue editing'}</button>
        <button type="button" className="secondary" disabled={phase === 'creating'} onClick={reset}>{zh ? '取消' : 'Cancel'}</button>
      </div>
    </div>
  </section>

  return <section className="autonomous-quick-create" aria-label={zh ? '一句话创建任务' : 'Quick create task'}>
    <div className="autonomous-quick-create-input"><Sparkles size={16} /><input value={input} onChange={event => setInput(event.target.value)} onKeyDown={event => { if (event.key === 'Enter') { event.preventDefault(); parse() } }} maxLength={4000} placeholder={copy.quickCreatePlaceholder} aria-label={copy.quickCreatePlaceholder} /><button type="button" className="primary" disabled={!input.trim() || phase === 'parsing'} onClick={parse}>{phase === 'parsing' ? <><RefreshCw size={15} className="spin" />{zh ? '解析中…' : 'Parsing…'}</> : (zh ? '智能创建' : 'Smart create')}</button></div>
  </section>
}

function TaskEditor({ draft, saving, onChange, onClose, onSubmit, zh }) {
  return <section className="autonomous-task-editor" aria-label={zh ? '新建自主任务' : 'New autonomous task'}>
    <header><h4>{zh ? '新建自主任务' : 'New autonomous task'}</h4><button type="button" className="icon-button" aria-label={zh ? '关闭编辑器' : 'Close editor'} onClick={onClose}><X size={16} /></button></header>
    <form onSubmit={onSubmit}>
      <label>{zh ? '标题' : 'Title'}<input required maxLength={200} value={draft.title} onChange={event => onChange({ title: event.target.value })} /></label>
      <label>{zh ? '目标' : 'Objective'}<textarea maxLength={4000} rows={3} value={draft.objective} onChange={event => onChange({ objective: event.target.value })} /></label>
      <label>{zh ? '下一步（可选）' : 'Next step (optional)'}<input maxLength={1000} value={draft.next_step} onChange={event => onChange({ next_step: event.target.value })} /></label>
      <footer><button type="submit" className="primary" disabled={saving}><Check size={15} />{zh ? '保存任务' : 'Save task'}</button></footer>
    </form>
  </section>
}

function TaskDetail({ detail, saving, onAction, onReject, zh }) {
  if (!detail) return <div className="autonomous-task-placeholder">{zh ? '选择任务查看详情、运行和事件' : 'Select a task to inspect runs and events'}</div>
  const status = publicStatus(detail.status)
  const canApprove = status === 'pending_approval'
  const events = (detail.events || []).slice(0, 10)
  const runs = detail.runs || []
  const eventTotal = (detail.events || []).length
  return <>
    <header className="autonomous-task-detail-head"><div><span>{zh ? '任务详情' : 'Task detail'}</span><h4>{detail.title}</h4></div></header>
    <p className="autonomous-task-objective">{detail.objective || (zh ? '未填写目标' : 'No objective')}</p>
    <dl><div><dt>{zh ? '状态' : 'Status'}</dt><dd>{statusLabel(status, zh)}</dd></div><div><dt>{zh ? '下一步' : 'Next step'}</dt><dd>{detail.next_step || '-'}</dd></div><div><dt>{zh ? '来源' : 'Source'}</dt><dd>{detail.source_path || 'temp/TODO.txt'}</dd></div></dl>
    {canApprove && <div className="autonomous-task-actions"><button type="button" className="primary" disabled={saving} onClick={() => onAction(detail, 'approve')}><Check size={15} />{zh ? '批准并排队' : 'Approve and queue'}</button><button type="button" className="secondary" disabled={saving} onClick={() => onReject(detail)}><X size={15} />{zh ? '拒绝' : 'Reject'}</button></div>}
    <details className="autonomous-task-runs-events"><summary>{zh ? '运行与事件' : 'Runs & events'}{runs.length || events.length ? ` (${runs.length}/${events.length})` : ''}</summary>
      <section className="autonomous-task-runs"><h5>{zh ? '运行记录' : 'Execution records'}</h5>{runs.length ? runs.map(run => <div key={run.id}><b>{zh ? '已记录' : 'Recorded'}</b><span>{run.stage || (zh ? '执行记录' : 'Execution record')}</span><small>{dateLabel(run.updated_at, zh)}</small></div>) : <p>{zh ? '尚未创建运行记录' : 'No execution records yet'}</p>}</section>
      <section className="autonomous-task-events"><h5>{zh ? '事件时间线' : 'Event timeline'}</h5>{events.length ? events.map(event => <div key={event.id}><b title={event.type}>{event.type}</b><span>{event.message || '-'}</span><time>{dateLabel(event.created_at, zh)}</time></div>) : <p>{zh ? '暂无事件' : 'No events'}</p>}{eventTotal > events.length && <p className="autonomous-task-events-more">{zh ? `仅显示最近 10 条，共 ${eventTotal} 条` : `Showing latest 10 of ${eventTotal} events`}</p>}</section>
    </details>
    {detail.report_path && <a className="autonomous-task-report-link" href={`/api/files/read?path=${encodeURIComponent(detail.report_path)}`}>{zh ? '打开关联报告' : 'Open linked report'}</a>}
  </>
}

export function AutonomousTaskWorkspace({ lang = 'zh' }) {
  const zh = lang !== 'en'
  const [tasks, setTasks] = useState([])
  const [selected, setSelected] = useState('')
  const [detail, setDetail] = useState(null)
  const [query, setQuery] = useState('')
  const [status, setStatus] = useState('')
  const [draft, setDraft] = useState(emptyDraft)
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
      if (query) params.set('q', query)
      const result = await api(`/api/autonomous/tasks${params.toString() ? `?${params}` : ''}`)
      const nextTasks = (Array.isArray(result.tasks) ? result.tasks : []).filter(task => STATUS[task.status])
      setTasks(nextTasks)
      if (selected && !nextTasks.some(task => task.id === selected)) { setSelected(''); setDetail(null) }
    } catch (error) { setMessage(error.message) } finally { setLoading(false) }
  }, [query, selected])

  useEffect(() => { refresh() }, [refresh])

  const openTask = async task => {
    setSelected(task.id)
    setDetail({ ...task, status: publicStatus(task.status) })
    try {
      const result = await api(`/api/autonomous/tasks/${encodeURIComponent(task.id)}`)
      if (result.task) setDetail({ ...result.task, status: publicStatus(result.task.status), events: result.events || [], runs: result.runs || [] })
    } catch (error) { setMessage(error.message) }
  }

  const onAction = async (task, name, note = '') => {
    if (name !== 'approve' && name !== 'reject') return
    const label = name === 'approve' ? (zh ? '批准' : 'Approve') : (zh ? '拒绝' : 'Reject')
    if (!await confirmDanger(`autonomous-task-${name}`, `${label}${zh ? `任务“${task.title}”？` : ` task "${task.title}"?`}`)) return
    setSaving(true)
    try {
      await api(`/api/autonomous/tasks/${encodeURIComponent(task.id)}/${name}`, { dangerous: true, method: 'POST', body: JSON.stringify({ note }) })
      await refresh()
      await openTask(task)
      setMessage(zh ? '操作已记录到 TODO.txt' : 'Action recorded in TODO.txt')
    } catch (error) { setMessage(error.message) } finally { setSaving(false) }
  }

  const confirmReject = async () => {
    if (!rejecting) return
    const task = rejecting
    setRejecting(null)
    const note = rejectNote
    setRejectNote('')
    await onAction(task, 'reject', note)
  }

  const saveTask = async event => {
    event.preventDefault()
    if (!draft.title.trim()) return
    if (!await confirmDanger('autonomous-task-create', zh ? `创建任务“${draft.title}”？` : `Create task "${draft.title}"?`)) return
    setSaving(true)
    try {
      const result = await api('/api/autonomous/tasks', { dangerous: true, method: 'POST', body: JSON.stringify({ ...draft, priority: 'normal' }) })
      setDraft(emptyDraft)
      setEditorOpen(false)
      await refresh()
      if (result.task) await openTask(result.task)
      setMessage(zh ? '任务已写入 TODO.txt，状态为待批准' : 'Task added to TODO.txt as pending')
    } catch (error) { setMessage(error.message) } finally { setSaving(false) }
  }

  const visibleTasks = useMemo(() => STATUS[status] ? tasks.filter(task => publicStatus(task.status) === status) : tasks, [status, tasks])
  const metrics = useMemo(() => taskMetrics(tasks), [tasks])
  const createTask = async payload => {
    const result = await api('/api/autonomous/tasks', { dangerous: true, method: 'POST', body: JSON.stringify({ ...payload, priority: 'normal' }) })
    await refresh()
    if (result.task) await openTask(result.task)
    setMessage(zh ? '任务已写入 TODO.txt，状态为待批准' : 'Task added to TODO.txt as pending')
  }

  return <section className="autonomous-task-workspace" aria-label={zh ? '自主任务工作台' : 'Autonomous task workspace'}>
    <header className="autonomous-task-head"><div><h3>{zh ? '所有自主任务' : 'All autonomous tasks'}</h3><p>{zh ? '任务唯一来源：temp/TODO.txt；状态只有待批准、排队中、已闭环。' : 'Source: temp/TODO.txt. States: pending, queued, and closed.'}</p></div><button type="button" className="primary" onClick={() => { setEditorOpen(true); setDraft(emptyDraft) }}><Plus size={16} />{zh ? '新建任务' : 'New task'}</button></header>
    <QuickCreate zh={zh} onCreate={createTask} onEditFull={values => { setEditorOpen(true); setDraft({ ...emptyDraft, ...values }) }} />
    <MetricStrip metrics={metrics} zh={zh} activeFilter={status} onFilter={setStatus} />
    <div className="autonomous-task-toolbar"><label><Search size={15} /><input value={query} onChange={event => setQuery(event.target.value)} placeholder={zh ? '搜索任务或目标' : 'Search tasks or objectives'} aria-label={zh ? '搜索任务' : 'Search tasks'} /></label><select value={status} onChange={event => setStatus(event.target.value)} aria-label={zh ? '任务状态' : 'Task status'}><option value="">{zh ? '全部状态' : 'All states'}</option>{STATUS_KEYS.map(key => <option key={key} value={key}>{statusLabel(key, zh)}</option>)}</select><button type="button" className="secondary" onClick={refresh} disabled={loading}><RefreshCw size={15} className={loading ? 'spin' : ''} />{zh ? '刷新' : 'Refresh'}</button></div>
    {message && <div className="autonomous-task-message" role="status">{message}<button type="button" className="icon-button" aria-label={zh ? '关闭消息' : 'Close message'} onClick={() => setMessage('')}><X size={15} /></button></div>}
    <div className="autonomous-task-layout"><div className="autonomous-task-list">{visibleTasks.length === 0 && <div className="autonomous-empty">{loading ? (zh ? '正在读取任务…' : 'Loading tasks…') : (zh ? '暂无任务。请在 TODO.txt 中添加任务，或点击新建任务。' : 'No tasks. Add one to TODO.txt or choose New task.')}</div>}{visibleTasks.map(task => <TaskRow key={task.id} task={task} selected={selected === task.id} onOpen={openTask} onApprove={task => onAction(task, 'approve')} zh={zh} />)}</div><aside className="autonomous-task-detail"><TaskDetail detail={detail} saving={saving} onAction={onAction} onReject={task => { setRejecting(task); setRejectNote('') }} zh={zh} /></aside></div>
    {editorOpen && <TaskEditor draft={draft} saving={saving} onChange={values => setDraft(current => ({ ...current, ...values }))} onClose={() => { setEditorOpen(false); setDraft(emptyDraft) }} onSubmit={saveTask} zh={zh} />}
    {rejecting && <div className="autonomous-dialog-backdrop" role="presentation" onMouseDown={event => { if (event.target === event.currentTarget) setRejecting(null) }}><div className="autonomous-dialog" role="dialog" aria-modal="true" aria-labelledby="autonomous-task-reject-title"><header><b id="autonomous-task-reject-title">{zh ? `拒绝任务：${rejecting.title}` : `Reject task: ${rejecting.title}`}</b><button type="button" aria-label={zh ? '取消' : 'Cancel'} onClick={() => setRejecting(null)}><X size={18} /></button></header><label>{zh ? '拒绝意见（可选）' : 'Rejection note (optional)'}<textarea maxLength={1000} value={rejectNote} onChange={event => setRejectNote(event.target.value)} /></label><footer><button type="button" className="secondary" onClick={() => setRejecting(null)}>{zh ? '取消' : 'Cancel'}</button><button type="button" className="danger" disabled={saving} onClick={confirmReject}>{zh ? '确认拒绝' : 'Confirm reject'}</button></footer></div></div>}
  </section>
}

export default AutonomousTaskWorkspace
