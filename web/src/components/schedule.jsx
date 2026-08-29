import React, { useEffect, useState } from 'react'
import ReactMarkdown from 'react-markdown'
import remarkGfm from 'remark-gfm'
import { ChevronRight, Eye, FileText, History, LoaderCircle, Play, Power, Square, Trash2 } from 'lucide-react'
import { effectiveScheduleModelNo, hasScheduleTaskModel } from '../lib/schedule'
import { firstRuntimeModelNo, runtimeModelDescription } from '../lib/modelDefaults.js'
import { ProviderModelCascade, buildModelProviderGroups, findModelProviderValue } from './ModelProviderCascade.jsx'

const taskRunState = task => task?.latest_run?.status || 'never_run'

const taskRunStateLabel = (state, t) => {
  const zh = t.autostart === '开机自启'
  const labels = zh
    ? { success: '成功', partial: '部分完成', blocked: '阻塞', waiting: '等待中', failed: '失败', skipped: '已跳过', unknown: '结果未知', never_run: '未运行' }
    : { success: 'Success', partial: 'Partial', blocked: 'Blocked', waiting: 'Waiting', failed: 'Failed', skipped: 'Skipped', unknown: 'Unknown', never_run: 'Never run' }
  return labels[state] || labels.unknown
}

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

export function TaskRow({ task, llms = [], t, schedulerModelNo = 0, onToggle, onEdit, onDelete, onRun, onReports, runState: activeRunState = null, selected = false }) {
  const id = task.id || task.name || t.tasks.unnamed
  const runState = taskRunState(task)
  const status = taskRunStateLabel(runState, t)
  const isRunning = activeRunState?.status === 'pending'
  const zh = t.autostart === '开机自启'
  const executedAt = task.latest_run?.executed_at ? new Date(task.latest_run.executed_at) : null
  const validExecutedAt = executedAt && !Number.isNaN(executedAt.getTime())
  const resultDetail = task.latest_run?.reason || task.latest_run?.summary || ''
  const openTask = () => onEdit?.(id)
  const onKeyDown = event => {
    if (event.key === 'Enter' || event.key === ' ') {
      event.preventDefault()
      openTask()
    }
  }
  return (
    <article className={`task-row task-run-${runState}${selected ? ' is-selected' : ''}`} role="button" aria-pressed={selected} tabIndex={0} onClick={openTask} onKeyDown={onKeyDown}>
      <div className="task-card-head">
        <div className="task-card-title">
          <b>{id}</b>
          <span>{task.schedule || t.tasks.unscheduled} · {task.repeat || t.tasks.manual}</span>
        </div>
        <div className="task-card-actions">
          <span className={`task-state-badge run-${runState}`}>{status}</span>
          <button type="button" className="task-run" title={isRunning ? t.tasks.runPending : t.tasks.runNow} aria-label={`${isRunning ? t.tasks.runPending : t.tasks.runNow} ${id}`} disabled={isRunning} onClick={event => { event.stopPropagation(); onRun?.(id) }}>
            {isRunning ? <LoaderCircle size={14} className="is-spinning"/> : <Play size={14}/>}<span>{isRunning ? t.tasks.runPending : t.tasks.runNow}</span>
          </button>
          <button type="button" className="task-reports" title={`${t.tasks.reportsAction}：${id}`} aria-label={`${t.tasks.reportsAction} ${id}`} onClick={event => { event.stopPropagation(); onReports?.(id) }}><History size={14}/><span>{t.tasks.reportsAction}</span></button>
          <button type="button" className="task-toggle" title={task.enabled ? t.disabled : t.enabled} aria-label={task.enabled ? t.disabled : t.enabled} onClick={event => { event.stopPropagation(); onToggle?.(id, !task.enabled) }}><Power size={15}/></button>
          <button type="button" className="task-delete" title={`${t.remove} ${id}`} aria-label={`${t.remove} ${id}`} onClick={event => { event.stopPropagation(); onDelete?.(id) }}><Trash2 size={15}/></button>
        </div>
      </div>
      <div className="task-latest-run">
        <span>{zh ? '最近执行' : 'Latest run'}：{validExecutedAt ? executedAt.toLocaleString() : (zh ? '暂无' : 'None')}</span>
        <small className={`task-config-state ${task.enabled ? 'enabled' : 'disabled'}`}>{task.enabled ? t.enabled : t.disabled}</small>
      </div>
      {resultDetail && <em className={`task-run-detail ${runState}`}>{resultDetail}</em>}
      <span className="task-model"><FileText size={14}/>{taskModelLabel(task, llms, t, schedulerModelNo)}</span>
      {!task.enabled && <em className="muted">{t.tasks.explicitEnable}</em>}
      {task.error && <em className="err-text">{task.error}</em>}
      {task.next_hint && <em>{task.next_hint}</em>}
      {activeRunState?.message && <div className={`task-run-status ${activeRunState.status || ''}`} role={activeRunState.status === 'error' ? 'alert' : 'status'} aria-live="polite">{activeRunState.message}</div>}
      <p>{task.prompt || t.empty}</p>
    </article>
  )
}

export function ScheduleReportTree({ tasks = [], selectedPath, onSelect, focusTaskId, t }) {
  const [expanded, setExpanded] = useState({})
  useEffect(() => {
    const id = String(focusTaskId || '').trim()
    if (id) setExpanded(current => ({ ...current, [id]: true }))
  }, [focusTaskId])
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
