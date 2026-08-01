import { useState } from 'react'
import ReactMarkdown from 'react-markdown'
import remarkGfm from 'remark-gfm'
import { ChevronRight, Eye, FileText, Play, Power, Square, Trash2 } from 'lucide-react'
import { effectiveScheduleModelNo, hasScheduleTaskModel } from '../lib/schedule'
import { firstRuntimeModelNo, runtimeModelDescription } from '../lib/modelDefaults.js'
import { ProviderModelCascade, buildModelProviderGroups, findModelProviderValue } from './ModelProviderCascade.jsx'

const taskState = (task) => {
  if (task.error || task.status === 'ERROR') return 'error'
  return task.enabled ? 'enabled' : 'disabled'
}

const taskStateLabel = (state, t) => state === 'error' ? t.error : (state === 'enabled' ? t.enabled : t.disabled)

const taskModelLabel = (task, llms, t, schedulerModelNo) => {
  const modelNo = effectiveScheduleModelNo(task, schedulerModelNo)
  const model = llms.find(item => Number(item?.index) === modelNo)
  const modelText = model
    ? runtimeModelDescription(model, t.tasks.unnamedModel)
    : `#${modelNo}`
  const zh = t.autostart === '开机自启'
  const prefix = hasScheduleTaskModel(task)
    ? (t.tasks.taskModelPrefix || (zh ? '任务指定模型' : 'Task model'))
    : (t.tasks.schedulerModelPrefix || (zh ? '调度器实际模型' : 'Scheduler actual model'))
  return `${prefix}${zh ? '：' : ': '}${modelText}`
}

export function TaskRow({ task, llms = [], t, schedulerModelNo = 0, onToggle, onEdit, onDelete, selected = false }) {
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
    <article className={`task-row task-state-${state}${selected ? ' is-selected' : ''}`} role="button" aria-pressed={selected} tabIndex={0} onClick={openTask} onKeyDown={onKeyDown}>
      <div className="task-card-head">
        <div className="task-card-title">
          <b>{id}</b>
          <span>{task.schedule || t.tasks.unscheduled} · {task.repeat || t.tasks.manual}</span>
        </div>
        <div className="task-card-actions">
          <span className={`task-state-badge ${state}`}>{status}</span>
          <button type="button" className="task-toggle" title={task.enabled ? t.disabled : t.enabled} aria-label={task.enabled ? t.disabled : t.enabled} onClick={event => { event.stopPropagation(); onToggle?.(id, !task.enabled) }}><Power size={15}/></button>
          <button type="button" className="task-delete" title={`${t.remove} ${id}`} aria-label={`${t.remove} ${id}`} onClick={event => { event.stopPropagation(); onDelete?.(id) }}><Trash2 size={15}/></button>
        </div>
      </div>
      <span className="task-model"><FileText size={14}/>{taskModelLabel(task, llms, t, schedulerModelNo)}</span>
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

export function SchedulerServiceRow({ service, llms = [], t, actionState = null, onStart, onStop, onLogs, onAutostart, onModel }) {
  const running = !!service?.running
  const isPending = actionState?.status === 'pending'
  const retryAction = actionState?.action === 'stop' ? onStop : onStart
  const defaultModelNo = firstRuntimeModelNo(llms)
  const defaultModel = llms.find(item => Number(item?.index) === defaultModelNo)
  const defaultModelText = defaultModel ? runtimeModelDescription(defaultModel, `#${defaultModelNo}`) : `#${defaultModelNo}`
  const defaultLabel = `${t.tasks.schedulerDefaultLabel || t.tasks.defaultModel || 'GA default model'}: ${defaultModelText}`
  const modelGroups = buildModelProviderGroups(llms, { defaultLabel })
  const modelValue = service?.model_no ?? ''
  const selectedProvider = findModelProviderValue(modelGroups, modelValue)
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
          <div className="scheduler-model-control">
            <span>{t.tasks?.executionModel || '执行模型'}</span>
            <ProviderModelCascade
              groups={modelGroups}
              selectedProvider={selectedProvider}
              value={modelValue}
              showLabel={false}
              placement="auto"
              align="start"
              className="scheduler-model-cascade"
              disabled={isPending || running || !llms.length}
              disabledReason={running ? (t.tasks.executionModelRunning || 'Stop the scheduler before changing its model') : ''}
              onChange={value => onModel?.(service.name, value === '' ? null : Number(value))}
            />
          </div>
          <label className="toggle-inline"><input type="checkbox" checked={!!service?.autostart} onChange={event => onAutostart?.(service.name, event.target.checked)} />{t.startWithAdmin || (t.autostart === '开机自启' ? '随 GA Admin 启动' : 'Start with GA Admin')}</label>
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
