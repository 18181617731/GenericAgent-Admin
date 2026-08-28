import React, { useMemo, useState } from 'react'
import { CalendarClock, ChevronLeft, ChevronRight, Circle, CircleAlert, Code2, FileCode2, FileText, History, LoaderCircle, Play, Power, RefreshCw, Search, SlidersHorizontal, Trash2 } from 'lucide-react'
import { ScheduleArtifactPreview, ScheduleTaskHistory, TaskFormEditor, taskModelLabel, taskState } from './schedule.jsx'

const workbenchCopy = t => {
  const zh = t?.autostart === '开机自启'
  return zh
    ? {
    title: '已安排的任务', summary: (shown, total) => `显示 ${shown} / ${total} 项`, search: '搜索任务名称或提示词', filterLabel: '状态筛选', all: '全部', enabled: '已启用', paused: '已暂停', anomaly: '异常', noMatch: '没有匹配的任务', clear: '清除筛选', choose: '选择一个任务查看详情', createHelp: '输入任务 ID 后创建新的定时任务', close: '返回任务列表', detail: '任务详情', prompt: '任务提示词', next: '下次执行', model: '执行模型', noNext: '暂无下一次执行提示', service: '调度服务',
    }
    : {
    title: 'Scheduled tasks', summary: (shown, total) => `${shown} of ${total} tasks`, search: 'Search task name or prompt', filterLabel: 'Status filter', all: 'All', enabled: 'Enabled', paused: 'Paused', anomaly: 'Attention', noMatch: 'No matching tasks', clear: 'Clear filters', choose: 'Select a task to view details', createHelp: 'Enter a task ID to create a new scheduled task', close: 'Back to task list', detail: 'Task details', prompt: 'Task prompt', next: 'Next run', model: 'Execution model', noNext: 'No next-run hint', service: 'Scheduler service',
    }
}

const taskID = (task, unnamed) => task?.id || task?.name || unnamed || 'task'

const stateLabel = (state, t, copy) => {
  if (state === 'enabled') return t?.enabled || copy.enabled
  if (state === 'disabled') return t?.disabled || copy.paused
  return copy.anomaly
}

function TaskListItem({ task, selected, llms, t, schedulerModelNo, onSelect }) {
  const copy = workbenchCopy(t)
  const id = taskID(task, t?.tasks?.unnamed)
  const state = taskState(task)
  const summary = task.next_hint || taskModelLabel(task, llms, t, schedulerModelNo)
  const cadence = `${task.schedule || t?.tasks?.unscheduled || '未排程'} · ${task.repeat || t?.tasks?.manual || '手动'}`
  return <button type="button" className={`scheduled-task-row task-row task-state-${state}${selected ? ' is-selected' : ''}`} role="option" aria-selected={selected} onClick={() => onSelect?.(id)}>
    <span className={`scheduled-task-row-state ${state}`} aria-hidden="true">{state === 'error' ? <CircleAlert size={17}/> : selected ? <Circle size={17} fill="currentColor"/> : <Circle size={17}/>}</span>
    <span className="scheduled-task-row-main">
      <span className="scheduled-task-row-title"><b>{id}</b><em className={`task-state-badge ${state}`}>{stateLabel(state, t, copy)}</em></span>
      <span className="scheduled-task-row-meta">{cadence}</span>
      <small>{summary || copy.noNext}</small>
    </span>
    <ChevronRight size={16} className="scheduled-task-row-chevron" aria-hidden="true"/>
  </button>
}

function TaskFilters({ tasks, query, filter, onQuery, onFilter, onClear, t }) {
  const copy = workbenchCopy(t)
  const counts = useMemo(() => tasks.reduce((result, task) => {
    const state = taskState(task)
    result.all += 1
    result[state] = (result[state] || 0) + 1
    return result
  }, { all: 0, enabled: 0, disabled: 0, error: 0 }), [tasks])
  const filters = [['all', copy.all, counts.all], ['enabled', copy.enabled, counts.enabled], ['disabled', copy.paused, counts.disabled], ['error', copy.anomaly, counts.error]]
  return <div className="scheduled-task-filters">
    <label className="scheduled-task-search"><Search size={16} aria-hidden="true"/><input type="search" aria-label={copy.search} placeholder={copy.search} value={query} onChange={event => onQuery(event.target.value)}/></label>
    <div className="scheduled-task-filter-buttons" role="group" aria-label={copy.filterLabel}>
      {filters.map(([key, label, count]) => <button type="button" key={key} className={filter === key ? 'active' : ''} aria-pressed={filter === key} onClick={() => onFilter(key)}>{label}<span>{count}</span></button>)}
    </div>
    {(query || filter !== 'all') && <button type="button" className="scheduled-task-clear" onClick={onClear}>{copy.clear}</button>}
  </div>
}

function TaskDetailHeader({ task, llms, t, schedulerModelNo, runState, busy, onRun, onReports, onToggle, onDelete, onClose }) {
  const copy = workbenchCopy(t)
  const id = taskID(task, t?.tasks?.unnamed)
  const state = taskState(task)
  const running = runState?.status === 'pending'
  return <header className="scheduled-task-detail-header">
    <button type="button" className="scheduled-task-mobile-back" onClick={onClose}><ChevronLeft size={17}/>{copy.close}</button>
    <div className="scheduled-task-detail-heading">
      <div className="scheduled-task-detail-eyebrow"><span className={`scheduled-task-status-dot ${state}`} aria-hidden="true"/><span>{stateLabel(state, t, copy)}</span><span>·</span><span>{task.schedule || t?.tasks?.unscheduled || '未排程'} · {task.repeat || t?.tasks?.manual || '手动'}</span></div>
      <h3>{id}</h3>
      <p>{taskModelLabel(task, llms, t, schedulerModelNo)}</p>
    </div>
    <div className="scheduled-task-detail-actions">
      <button type="button" className="primary task-run" disabled={running || busy} onClick={() => onRun?.(id)}>{running ? <LoaderCircle size={14} className="is-spinning"/> : <Play size={14}/>}<span>{running ? t?.tasks?.runPending : t?.tasks?.runNow}</span></button>
      <button type="button" className="secondary task-reports" onClick={() => onReports?.(id)}><History size={14}/><span>{t?.tasks?.reportsAction}</span></button>
      <button type="button" className="secondary task-toggle" disabled={busy} onClick={() => onToggle?.(id, !task.enabled)}><Power size={15}/><span>{task.enabled ? t?.disabled : t?.enabled}</span></button>
      <button type="button" className="task-delete" disabled={busy} onClick={() => onDelete?.(id)}><Trash2 size={15}/><span>{t?.remove}</span></button>
    </div>
    {runState?.message && <div className={`task-run-status ${runState.status || ''}`} role={runState.status === 'error' ? 'alert' : 'status'} aria-live="polite">{runState.message}</div>}
  </header>
}

function TaskEditorCard({ taskEditor, setTaskEditor, editorMode, setEditorMode, taskDirty, onSave, busy, t, llms, schedulerModelNo }) {
  return <section className="scheduled-task-editor-card">
    <div className="scheduled-task-section-heading"><div><span className="scheduled-task-section-kicker">{t?.lists?.editor || '编辑器'}</span><h4>{t?.tasks?.form || '表单编辑'}</h4></div><span className={taskDirty ? 'status-pill warn' : 'status-pill ok'}>{taskDirty ? '有未保存更改' : '编辑器已同步'}</span></div>
    <div className="editor-mode-toggle">
      <button type="button" className={editorMode === 'form' ? 'active' : ''} onClick={() => setEditorMode('form')}><SlidersHorizontal size={14}/>{t?.tasks?.form || '表单编辑'}</button>
      <button type="button" className={editorMode === 'json' ? 'active' : ''} onClick={() => setEditorMode('json')}><Code2 size={14}/>{t?.tasks?.json || 'JSON 编辑'}</button>
    </div>
    <p className="muted scheduled-task-editor-help">{editorMode === 'json' ? t?.hints?.jsonHelp : t?.tasks?.formHelp}</p>
    {editorMode === 'json' ? <textarea className="json-editor compact-editor scheduled-task-json-editor" value={taskEditor} onChange={event => setTaskEditor(event.target.value)}/> : <TaskFormEditor value={taskEditor} onChange={setTaskEditor} t={t} llms={llms} schedulerModelNo={schedulerModelNo}/>}
    <div className="scheduled-task-save-row"><button type="button" className="primary" onClick={onSave} disabled={!taskDirty || busy}><FileText size={14}/>{t?.save || '保存'}</button></div>
  </section>
}

function TaskDetail({ task, taskEditor, setTaskEditor, editorMode, setEditorMode, taskDirty, onSave, busy, llms, t, schedulerModelNo, scheduleArtifactTitle, scheduleArtifact, onSelectArtifact, runState, onRun, onReports, onToggle, onDelete, onClose }) {
  const copy = workbenchCopy(t)
  const prompt = String(task?.prompt || '').trim()
  return <section className="scheduled-task-detail" aria-label={`${copy.detail}: ${taskID(task, t?.tasks?.unnamed)}`}>
    <TaskDetailHeader task={task} llms={llms} t={t} schedulerModelNo={schedulerModelNo} runState={runState} busy={busy} onRun={onRun} onReports={onReports} onToggle={onToggle} onDelete={onDelete} onClose={onClose}/>
    <div className="scheduled-task-detail-scroll">
      <div className="scheduled-task-prompt-card"><span>{copy.prompt}</span><p>{prompt || t?.empty}</p></div>
      <div className="scheduled-task-detail-facts"><span><b>{copy.next}</b><em>{task.next_hint || copy.noNext}</em></span><span><b>{copy.model}</b><em>{taskModelLabel(task, llms, t, schedulerModelNo)}</em></span></div>
      <TaskEditorCard taskEditor={taskEditor} setTaskEditor={setTaskEditor} editorMode={editorMode} setEditorMode={setEditorMode} taskDirty={taskDirty} onSave={onSave} busy={busy} t={t} llms={llms} schedulerModelNo={schedulerModelNo}/>
      <section className="scheduled-task-history-card">
        <div className="scheduled-task-section-heading"><div><span className="scheduled-task-section-kicker">{t?.lists?.recentReports || '执行记录'}</span><h4>{taskID(task, t?.tasks?.unnamed)}</h4></div><span className="scheduled-task-history-count">{(task.recent_reports || []).length}</span></div>
        <ScheduleTaskHistory task={task} selectedPath={scheduleArtifactTitle} onSelect={onSelectArtifact} t={t}/>
        {scheduleArtifactTitle && <div className="schedule-task-history-preview"><div className="schedule-task-history-preview-title">{scheduleArtifactTitle}</div><ScheduleArtifactPreview title={scheduleArtifactTitle} content={scheduleArtifact} empty={t?.empty}/></div>}
      </section>
    </div>
  </section>
}

export function ScheduledTaskWorkbench({ tasks = [], selectedTask, selectedTaskId = '', scheduleLoading = false, scheduleError = '', scheduleLogExists = false, newTaskId, setNewTaskId, createTask, loadScheduleTasks, onScheduleLog, loadTask, clearTaskSelection, taskEditor, setTaskEditor, editorMode, setEditorMode, taskDirty, saveTask, busy, llms, t, schedulerModelNo, scheduleArtifactTitle, scheduleArtifact, onSelectArtifact, onToggle, onDelete, onRun, onReports, taskRunStates = {} }) {
  const copy = workbenchCopy(t)
  const [query, setQuery] = useState('')
  const [filter, setFilter] = useState('all')
  const [createOpen, setCreateOpen] = useState(false)
  const [createSubmitting, setCreateSubmitting] = useState(false)
  const filteredTasks = useMemo(() => {
    const needle = query.trim().toLowerCase()
    return tasks.filter(task => {
      const state = taskState(task)
      const id = taskID(task, t?.tasks?.unnamed)
      const matchesFilter = filter === 'all' || state === filter
      const haystack = `${id} ${task.name || ''} ${task.prompt || ''}`.toLowerCase()
      return matchesFilter && (!needle || haystack.includes(needle))
    })
  }, [filter, query, t?.tasks?.unnamed, tasks])
  const clearFilters = () => { setQuery(''); setFilter('all') }
  const submitCreate = async event => {
    event.preventDefault()
    if (busy || createSubmitting || !newTaskId?.trim()) return
    setCreateSubmitting(true)
    try {
      if (await createTask?.() === true) setCreateOpen(false)
    } finally {
      setCreateSubmitting(false)
    }
  }
  return <section className="scheduled-task-workbench" aria-label={copy.title}>
    <div className="scheduled-workbench-toolbar">
      <div><span className="scheduled-workbench-kicker">{t?.lists?.scheduledTasks || copy.title}</span><h3>{copy.title}</h3><p>{copy.summary(filteredTasks.length, tasks.length)}</p></div>
      <div className="scheduled-workbench-toolbar-actions"><button type="button" className="primary" aria-expanded={createOpen} onClick={() => setCreateOpen(value => !value)}><FileCode2 size={15}/>{t?.create || '创建'}</button><button type="button" className="secondary" onClick={() => loadScheduleTasks?.()} disabled={scheduleLoading}><RefreshCw size={15}/>{t?.refresh || '刷新'}</button>{scheduleLogExists && <button type="button" className="secondary" onClick={onScheduleLog}><FileText size={15}/>{t?.nav?.logs || '日志'}</button>}</div>
    </div>
    {createOpen && <form className="scheduled-task-create-disclosure" onSubmit={submitCreate}><div><b>{t?.create || '创建'}</b><p>{copy.createHelp}</p></div><input aria-label={t?.hints?.newTaskId || 'new_task'} value={newTaskId || ''} onChange={event => setNewTaskId?.(event.target.value)} placeholder={t?.hints?.newTaskId || 'new_task'}/><button type="submit" className="primary" disabled={busy || createSubmitting || !newTaskId?.trim()}>{t?.create || '创建'}</button></form>}
    <TaskFilters tasks={tasks} query={query} filter={filter} onQuery={setQuery} onFilter={setFilter} onClear={clearFilters} t={t}/>
    {scheduleError && <p className="err-text scheduled-task-error">{scheduleError}</p>}
    <div className={`scheduled-workbench-body${selectedTask ? ' has-selection' : ''}`}>
      <aside className="scheduled-task-list-pane"><div className="scheduled-task-list-heading"><div><b>{t?.lists?.scheduledTasks || copy.title}</b><span>{copy.summary(filteredTasks.length, tasks.length)}</span></div><span className="scheduled-task-list-count">{filteredTasks.length}</span></div><div className="scheduled-task-list" role="listbox" aria-label={copy.title} aria-busy={scheduleLoading}>{scheduleLoading ? <p className="muted">{t?.busy || '执行中'}</p> : filteredTasks.length ? filteredTasks.map((task, index) => <TaskListItem key={taskID(task, `${index}`)} task={task} selected={String(selectedTaskId) === String(taskID(task))} llms={llms} t={t} schedulerModelNo={schedulerModelNo} onSelect={loadTask}/>) : <div className="scheduled-task-list-empty"><CircleAlert size={20}/><p>{copy.noMatch}</p>{(query || filter !== 'all') && <button type="button" onClick={clearFilters}>{copy.clear}</button>}</div>}</div></aside>
      {selectedTask ? <TaskDetail task={selectedTask} taskEditor={taskEditor} setTaskEditor={setTaskEditor} editorMode={editorMode} setEditorMode={setEditorMode} taskDirty={taskDirty} onSave={saveTask} busy={busy} llms={llms} t={t} schedulerModelNo={schedulerModelNo} scheduleArtifactTitle={scheduleArtifactTitle} scheduleArtifact={scheduleArtifact} onSelectArtifact={onSelectArtifact} runState={taskRunStates[taskID(selectedTask)]} onRun={onRun} onReports={onReports} onToggle={onToggle} onDelete={onDelete} onClose={clearTaskSelection}/> : <section className="scheduled-task-detail scheduled-task-detail-empty"><div><CalendarClock size={27}/><h3>{copy.choose}</h3><p>{t?.desc?.schedule || t?.desc?.tasks}</p></div></section>}
    </div>
  </section>
}
