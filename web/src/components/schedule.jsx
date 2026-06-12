import { Eye } from 'lucide-react'

const statusLabel = (task) => {
  if (task.error) return '错误'
  if (task.status) return task.status
  return task.enabled ? '已启用' : '已停用'
}

export function TaskRow({ task, t, onToggle, onEdit, onArtifact }) {
  const id = task.id || task.name || '未命名任务'
  const status = statusLabel(task)
  return (
    <div className={`task-row status-${String(status).toLowerCase()}`}>
      <div>
        <b>{id}</b>
        <span>{task.schedule || '未排程'} - {task.repeat || '手动'} - {status}</span>
        {!task.enabled && !task.error && <em className="muted">需显式启用后才会运行</em>}
        {task.error && <em className="err-text">{task.error}</em>}
        {task.next_hint && <em>{task.next_hint}</em>}
        <p>{task.prompt || t.empty}</p>
        {task.recent_reports?.length > 0 && <div className="mini-reports">{task.recent_reports.map((r, idx)=><button key={r.path || r.name || idx} onClick={()=>onArtifact(r.path)} disabled={!r.path}>{r.name || r.path || '报告'}</button>)}</div>}
      </div>
      <div className="actions">
        <button onClick={()=>onEdit(id)}><Eye size={14}/>{t.read}</button>
        <button onClick={()=>onToggle(id, !task.enabled)}>{task.enabled ? t.disabled : t.enabled}</button>
      </div>
    </div>
  )
}
