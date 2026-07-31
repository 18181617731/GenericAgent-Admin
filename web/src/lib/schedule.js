export const DEFAULT_SCHEDULE_TASK = Object.freeze({
  schedule: '09:00',
  repeat: 'daily',
  enabled: false,
  prompt: '',
})

const validTaskLLMNo = value => value !== null && value !== undefined && value !== '' && Number.isInteger(Number(value)) && Number(value) >= 0

export const normalizeScheduleModelNo = (value, fallback = 0) => {
  if (validTaskLLMNo(value)) return Number(value)
  return validTaskLLMNo(fallback) ? Number(fallback) : 0
}

export const hasScheduleTaskModel = task => validTaskLLMNo(task?.llm_no)

export const effectiveScheduleModelNo = (task, schedulerModelNo = 0) => hasScheduleTaskModel(task)
  ? Number(task.llm_no)
  : normalizeScheduleModelNo(schedulerModelNo)

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
        return {
          ...task,
          id: String(task.id || task.name || `task-${index + 1}`),
          enabled,
          schedule: task.schedule || 'unscheduled',
          repeat: task.repeat || 'manual',
          status,
          prompt: task.prompt || '',
          llm_no: validTaskLLMNo(task.llm_no) ? Number(task.llm_no) : null,
          recent_reports: Array.isArray(task.recent_reports) ? task.recent_reports : [],
        }
      }),
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
