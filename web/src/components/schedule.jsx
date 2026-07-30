import { useState } from 'react'
import ReactMarkdown from 'react-markdown'
import remarkGfm from 'remark-gfm'
import { ChevronRight, Eye, FileText, Play, Power, Square } from 'lucide-react'

const taskState = (task) => {
  if (task.error || task.status === 'ERROR') return 'error'
  return task.enabled ? 'enabled' : 'disabled'
}

const taskStateLabel = (state, t) => state === 'error' ? t.error : (state === 'enabled' ? t.enabled : t.disabled)

const taskModelLabel = (task, llms, t) => {
  if (task.llm_no === null || task.llm_no === undefined || task.llm_no === '' || !Number.isInteger(Number(task.llm_no)) || Number(task.llm_no) < 0) return t.tasks.defaultModel
  const model = llms.find(item => Number(item?.index) === Number(task.llm_no))
  if (!model) return `#${task.llm_no}`
  return `${model.provider || t.tasks.unnamedProvider} · ${model.model || model.name || model.label || t.tasks.unnamedModel} · #${model.index}`
}

export function TaskRow({ task, llms = [], t, onToggle, onEdit }) {
  const id = task.id || task.name || t.tasks.unnamed
  const state = taskState(task)
  const status = taskStateLabel(state, t)
  const openTask = () => onEdit?.(id)
  const onKeyDown = event => {
    if (event.key === 'Enter' || event.key === ' ') {
      event.preventDefault()
      openTask()
    }
  }
  return (
    <article className={`task-row task-state-${state}`} role="button" tabIndex={0} onClick={openTask} onKeyDown={onKeyDown}>
      <div className="task-card-head">
        <div className="task-card-title">
          <b>{id}</b>
          <span>{task.schedule || t.tasks.unscheduled} · {task.repeat || t.tasks.manual}</span>
        </div>
        <div className="task-card-actions">
          <span className={`task-state-badge ${state}`}>{status}</span>
          <button type="button" className="task-toggle" title={task.enabled ? t.disabled : t.enabled} aria-label={task.enabled ? t.disabled : t.enabled} onClick={event => { event.stopPropagation(); onToggle?.(id, !task.enabled) }}><Power size={15}/></button>
        </div>
      </div>
      <span className="task-model"><FileText size={14}/>{taskModelLabel(task, llms, t)}</span>
      {!task.enabled && state !== 'error' && <em className="muted">{t.tasks.explicitEnable}</em>}
      {task.error && <em className="err-text">{task.error}</em>}
      {task.next_hint && <em>{task.next_hint}</em>}
      <p>{task.prompt || t.empty}</p>
    </article>
  )
}

export function ScheduleReportTree({ tasks = [], selectedPath, onSelect, t }) {
  const [expanded, setExpanded] = useState({})
  return <div className="schedule-report-tree">
    {tasks.length ? tasks.map(task => {
      const id = task.id || task.name || t.tasks.unnamed
      const reports = (task.recent_reports || []).slice(0, 30)
      const open = Boolean(expanded[id])
      return <section className="schedule-report-group" key={id}>
        <button type="button" className="schedule-report-group-toggle" aria-expanded={open} onClick={() => setExpanded(current => ({ ...current, [id]: !open }))}>
          <ChevronRight size={16} className={open ? 'expanded' : ''}/>
          <span><b>{id}</b><small>{reports.length ? t.tasks.reportCount(reports.length) : t.tasks.noReports}</small></span>
        </button>
        {open && <div className="schedule-report-items">
          {reports.length ? reports.map(report => <button type="button" key={report.path} className={selectedPath === report.path ? 'active' : ''} onClick={() => onSelect?.(report.path)}>
            <FileText size={15}/><span>{report.name}<small>{report.mod_time ? new Date(report.mod_time).toLocaleString() : ''}</small></span>
          </button>) : <p className="muted">{t.tasks.noReports}</p>}
        </div>}
      </section>
    }) : <p className="muted">{t.empty}</p>}
  </div>
}

export function ScheduleArtifactPreview({ title, content, empty }) {
  if (!content) return <div className="schedule-artifact-empty">{empty}</div>
  return <article className="artifact-view schedule-artifact-markdown" aria-label={title || '执行记录预览'}>
    <ReactMarkdown remarkPlugins={[remarkGfm]} skipHtml>{content}</ReactMarkdown>
  </article>
}

export function SchedulerServiceRow({ service, t, actionState = null, onStart, onStop, onLogs, onAutostart }) {
  const running = !!service?.running
  const isPending = actionState?.status === 'pending'
  const retryAction = actionState?.action === 'stop' ? onStop : onStart
  return (
    <article className="scheduler-service-row" aria-busy={isPending || undefined}>
      <div className="scheduler-service-head">
        <div>
          <b>{service?.name || 'reflect/scheduler.py'}</b>
          <p>{t.serviceDesc?.scheduler}</p>
        </div>
        <span className={running ? 'status-pill running' : 'status-pill stopped'}>{running ? t.running : t.stopped}</span>
      </div>
      <div className="scheduler-service-controls">
        <div className="scheduler-service-facts">
          <span>PID <b>{service?.pid || '-'}</b></span>
          <label className="toggle-inline"><input type="checkbox" checked={!!service?.autostart} onChange={event => onAutostart?.(service.name, event.target.checked)} />{t.autostart}</label>
        </div>
        <div className="svc-actions">
          <button disabled={isPending || running} onClick={() => onStart(service.name)}><Play size={14}/>{t.start}</button>
          <button disabled={isPending || !running} onClick={() => onStop(service.name)}><Square size={14}/>{t.stop}</button>
          <button onClick={() => onLogs?.(service.name)}><Eye size={14}/>{t.nav.logs}</button>
        </div>
      </div>
      {actionState?.message && <div className={`service-action-status ${actionState.status || ''}`} role={actionState.status === 'error' ? 'alert' : 'status'} aria-live="polite">
        <span>{actionState.message}</span>
        {actionState.status === 'error' && <button type="button" onClick={() => retryAction?.(service.name)}>{t.retry || 'Retry'}</button>}
      </div>}
    </article>
  )
}
