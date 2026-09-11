export const DEFAULT_SCHEDULE_TASK = Object.freeze({
  schedule: '09:00',
  repeat: 'daily',
  enabled: false,
  prompt: '',
})

const validTaskLLMNo = value => value !== null && value !== undefined && value !== '' && Number.isInteger(Number(value)) && Number(value) >= 0
const validTaskModelKey = value => typeof value === 'string' && value.trim().length > 0

export const normalizeScheduleModelNo = (value, fallback = 0) => {
  if (validTaskLLMNo(value)) return Number(value)
  return validTaskLLMNo(fallback) ? Number(fallback) : 0
}

export const hasScheduleTaskModel = task => validTaskModelKey(task?.model_key) || validTaskLLMNo(task?.llm_no)

export const resolveScheduleTaskModel = (task, llms = [], schedulerModelNo = 0) => {
  const models = Array.isArray(llms) ? llms : []
  const taskKey = validTaskModelKey(task?.model_key) ? task.model_key.trim() : ''
  if (taskKey) {
    const stable = models.find(model => model?.model_key === taskKey)
    if (stable) return { model: stable, index: Number(stable.index), source: 'task', unavailable: false }
    return {
      model: null,
      index: validTaskLLMNo(task?.llm_no) ? Number(task.llm_no) : null,
      source: 'task',
      unavailable: true,
    }
  }
  if (validTaskLLMNo(task?.llm_no)) {
    const index = Number(task.llm_no)
    return { model: models.find(item => Number(item?.index) === index) || null, index, source: 'task', unavailable: false }
  }
  const index = normalizeScheduleModelNo(schedulerModelNo)
  return { model: models.find(item => Number(item?.index) === index) || null, index, source: 'scheduler', unavailable: false }
}

export const effectiveScheduleModelNo = (task, schedulerModelNo = 0) => validTaskLLMNo(task?.llm_no)
  ? Number(task.llm_no)
  : normalizeScheduleModelNo(schedulerModelNo)

const runStatusAliases = Object.freeze({
  success: 'success', successful: 'success', passed: 'success', pass: 'success', ok: 'success',
  partial: 'partial', partially: 'partial', blocked: 'blocked', waiting: 'waiting', pending: 'waiting',
  failed: 'failed', failure: 'failed', error: 'failed', skipped: 'skipped', skip: 'skipped',
  never_run: 'never_run', never: 'never_run', unknown: 'unknown',
})

export const normalizeScheduleLatestRun = (latestRun, lastReport = null) => {
  const src = latestRun && typeof latestRun === 'object' ? latestRun : null
  const report = lastReport && typeof lastReport === 'object' ? lastReport : null
  const rawStatus = String(src?.status || (report ? 'unknown' : 'never_run')).trim().toLowerCase().replace(/[ -]+/g, '_')
  return {
    ...(src || {}),
    status: runStatusAliases[rawStatus] || 'unknown',
    executed_at: src?.executed_at || report?.mod_time || '',
    summary: String(src?.summary || '').trim(),
    reason: String(src?.reason || '').trim(),
    report_path: String(src?.report_path || report?.path || '').trim(),
  }
}

export const normalizeScheduleTasksPayload = (payload = {}) => {
  const src = payload && typeof payload === 'object' ? payload : {}
  const tasks = Array.isArray(src.tasks) ? src.tasks : []
  const error = String(src.error || '').trim()
  return {
    ...src,
    enabled: Boolean(src.enabled) && !error,
    error,
    version: src.version || 'unknown',
    tasks: tasks
      .filter(task => task && typeof task === 'object')
      .map((task, index) => {
        const enabled = Boolean(task.enabled)
        const status = task.status || (enabled ? 'enabled' : 'disabled')
        const latestRun = normalizeScheduleLatestRun(task.latest_run, task.last_report)
        return {
          ...task,
          id: String(task.id || task.name || `task-${index + 1}`),
          enabled,
          schedule: task.schedule || 'unscheduled',
          repeat: task.repeat || 'manual',
          status,
          latest_run: latestRun,
          prompt: task.prompt || '',
          llm_no: validTaskLLMNo(task.llm_no) ? Number(task.llm_no) : null,
          model_key: validTaskModelKey(task.model_key) ? task.model_key.trim() : '',
          folder_id: typeof task.folder_id === 'string' ? task.folder_id.trim() : '',
          recent_reports: Array.isArray(task.recent_reports) ? task.recent_reports : [],
        }
      }),
    folders: Array.isArray(src.folders)
      ? src.folders.filter(folder => folder && typeof folder === 'object' && String(folder.id || '').trim()).map(folder => ({
        ...folder,
        id: String(folder.id).trim(),
        name: String(folder.name || '').trim() || String(folder.id).trim(),
        order: Number.isFinite(Number(folder.order)) ? Number(folder.order) : 0,
      })).sort((a, b) => a.order - b.order || a.name.localeCompare(b.name))
      : [],
  }
}

export const buildScheduleCreateRequest = (id, task = DEFAULT_SCHEDULE_TASK) => ({
  id: String(id || '').trim(),
  task: { ...DEFAULT_SCHEDULE_TASK, ...(task || {}) },
})

export const firstScheduleTaskID = (tasks = []) => {
  if (!Array.isArray(tasks)) return ''
  const first = tasks.find(task => task && typeof task === 'object')
  return String(first?.id || first?.name || '').trim()
}
