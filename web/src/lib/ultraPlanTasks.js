const textValue = (value) => String(value ?? '').trim()

const ultraPlanOutputPathCandidates = (task = {}) => [
  task.outputFile,
  task.output_file,
  task.outFile,
  task.out_file,
  task.file,
  task.path,
].map(textValue).filter(Boolean)

export const preferredUltraPlanOutputFile = (task = {}) => {
  const candidates = ultraPlanOutputPathCandidates(task)
  const generatedOutput = candidates.find(path => /\.out\.txt$/i.test(path))
  if (generatedOutput) return generatedOutput
  const fallback = candidates[0] || ''
  return /\.txt$/i.test(fallback) ? fallback.replace(/\.txt$/i, '.out.txt') : fallback
}

const normalizedText = (value) => textValue(value)
  .normalize('NFKC')
  .toLowerCase()
  .replace(/\s+/g, ' ')

const taskDescription = (task = {}) => task.desc || task.name || task.title || task.msg || ''

export const ultraPlanTaskDescriptionKey = (task = {}) => normalizedText(taskDescription(task))
  .replace(/^(?:(?:step|task)\s*\d+|(?:phase)\s*\d+|[0-9]+)\s*[:：.)、-]\s*/i, '')

export const ultraPlanTaskStrongKeys = (task = {}) => {
  const keys = new Set()
  const add = (prefix, value) => {
    const normalized = normalizedText(value)
    if (normalized) keys.add(`${prefix}:${normalized}`)
  }

  add('id', task.id)
  add('id', task.task_id)
  add('id', task.key)

  for (const value of [task.outputFile, task.output_file, task.outFile, task.out_file, task.file, task.path]) {
    const path = textValue(value).replace(/\\/g, '/')
    if (!path) continue
    add('path', path)
    const name = path.split('/').filter(Boolean).pop() || ''
    add('file', name)
    if (name.toLowerCase().endsWith('.out.txt')) add('file', name.slice(0, -8))
    else if (name.toLowerCase().endsWith('.txt')) add('file', name.slice(0, -4))
  }
  return [...keys]
}

export const hasUltraPlanTaskStrongIdentity = (task = {}) => ultraPlanTaskStrongKeys(task).length > 0

export const ultraPlanTasksMatch = (left = {}, right = {}, { includeDescription = true } = {}) => {
  const rightStrong = new Set(ultraPlanTaskStrongKeys(right))
  if (ultraPlanTaskStrongKeys(left).some(key => rightStrong.has(key))) return true
  if (!includeDescription) return false
  const leftDesc = ultraPlanTaskDescriptionKey(left)
  return Boolean(leftDesc && leftDesc === ultraPlanTaskDescriptionKey(right))
}

const isPresent = (value) => {
  if (value === undefined || value === null) return false
  if (typeof value === 'string') return value.trim().length > 0
  if (Array.isArray(value)) return value.length > 0
  return true
}

const contentSize = (value) => {
  if (Array.isArray(value)) return value.map(item => String(item ?? '')).join('\n').length
  return String(value ?? '').length
}

const terminalStatuses = new Set([
  'done', 'complete', 'completed', 'success', 'succeeded',
  'fail', 'failed', 'error', 'cancelled', 'canceled', 'stopped',
])

const normalizedStatus = (value) => normalizedText(value)
const isTerminalStatus = (value) => terminalStatuses.has(normalizedStatus(value))

const pickStatus = (baseStatus, incomingStatus) => {
  if (!isPresent(incomingStatus)) return baseStatus
  if (!isPresent(baseStatus)) return incomingStatus
  if (isTerminalStatus(baseStatus) && !isTerminalStatus(incomingStatus)) return baseStatus
  return incomingStatus
}

const identityFields = new Set([
  'desc', 'name', 'title', 'msg', 'id', 'task_id', 'key',
  'file', 'path', 'outputFile', 'output_file', 'outFile', 'out_file',
])
const outputFields = new Set(['output', 'out', 'result', 'summary', 'output_lines'])

export const mergeUltraPlanTaskRecords = (base = {}, incoming = {}) => {
  const merged = { ...base }
  for (const [key, value] of Object.entries(incoming || {})) {
    if (key === 'status' || key === 'state' || !isPresent(value)) continue
    if (!isPresent(merged[key])) {
      merged[key] = value
    } else if (outputFields.has(key) && contentSize(value) > contentSize(merged[key])) {
      merged[key] = value
    } else if (!identityFields.has(key) && !outputFields.has(key)) {
      merged[key] = value
    }
  }

  const status = pickStatus(base.status || base.state, incoming.status || incoming.state)
  if (isPresent(status)) merged.status = status
  return merged
}

export const dedupeUltraPlanTasks = (tasks = [], options = {}) => {
  const result = []
  for (const task of Array.isArray(tasks) ? tasks : []) {
    const index = result.findIndex(existing => ultraPlanTasksMatch(existing, task, options))
    if (index < 0) result.push({ ...task })
    else result[index] = mergeUltraPlanTaskRecords(result[index], task)
  }
  return result
}

export const reconcileUltraPlanTasks = (phases = [], recentTasks = []) => {
  const claimedStrong = []
  const preparePhase = (phase = {}) => {
    const tasks = []
    for (const task of dedupeUltraPlanTasks(phase.tasks || [])) {
      const prior = claimedStrong.find(item => ultraPlanTasksMatch(item, task, { includeDescription: false }))
      if (prior) {
        Object.assign(prior, mergeUltraPlanTaskRecords(prior, task))
        continue
      }
      const owned = { ...task }
      if (hasUltraPlanTaskStrongIdentity(owned)) claimedStrong.push(owned)
      tasks.push(owned)
    }
    return {
      ...phase,
      tasks,
      children: Array.isArray(phase.children) ? phase.children.map(preparePhase) : [],
    }
  }

  const preparedPhases = (Array.isArray(phases) ? phases : []).map(preparePhase)
  const uniqueRecent = dedupeUltraPlanTasks(recentTasks)
  const matchedRecent = new Set()

  const enrichPhase = (phase = {}) => ({
    ...phase,
    tasks: (phase.tasks || []).map(task => {
      let enriched = task
      uniqueRecent.forEach((recent, index) => {
        if (!ultraPlanTasksMatch(enriched, recent)) return
        matchedRecent.add(index)
        enriched = mergeUltraPlanTaskRecords(enriched, recent)
      })
      return enriched
    }),
    children: Array.isArray(phase.children) ? phase.children.map(enrichPhase) : [],
  })

  return {
    phases: preparedPhases.map(enrichPhase),
    recentTasks: uniqueRecent.filter((_, index) => !matchedRecent.has(index)),
  }
}
