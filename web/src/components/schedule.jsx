import { Eye, Play, Power, Square } from 'lucide-react'

const statusLabel = (task, t) => {
  if (task.error) return t.error
  if (task.status) return task.status
  return task.enabled ? t.enabled : t.disabled
}

export function TaskRow({ task, t, onToggle, onEdit, onArtifact }) {
  const id = task.id || task.name || t.tasks.unnamed
  const status = statusLabel(task, t)
  return (
    <div className={`task-row status-${String(status).toLowerCase()}`}>
      <div>
        <b>{id}</b>
        <span>{task.schedule || t.tasks.unscheduled} - {task.repeat || t.tasks.manual} - <b className="status-badge">{status}</b></span>
        {!task.enabled && !task.error && <em className="muted">{t.tasks.explicitEnable}</em>}
        {task.error && <em className="err-text">{task.error}</em>}
        {task.next_hint && <em>{task.next_hint}</em>}
        <p>{task.prompt || t.empty}</p>
        {task.recent_reports?.length > 0 && <div className="mini-reports">{task.recent_reports.map((r, idx)=><button key={r.path || r.name || idx} onClick={()=>onArtifact(r.path)} disabled={!r.path}>{r.name || r.path || t.tasks.report}</button>)}</div>}
        <div className="actions">
          <button onClick={()=>onEdit(id)}><Eye size={14}/>{t.read}</button>
          <button onClick={()=>onToggle(id, !task.enabled)}><Power size={14}/>{task.enabled ? t.disabled : t.enabled}</button>
        </div>
      </div>
    </div>
  )
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
