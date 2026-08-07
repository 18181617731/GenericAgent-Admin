const value = input => String(input ?? '').trim()

const reportKey = report => {
  if (!report) return ''
  return [value(report.path || report.name), value(report.mod_time || report.modTime), value(report.size)].filter(Boolean).join('|')
}

const goalFailure = goal => Boolean(value(goal?.error_class)) || /fail|error|timeout|crash|abort|exception|start_failed/i.test(value(goal?.status))
const goalSuccess = goal => !goalFailure(goal) && /complete|done|finish|success|passed|closed|归档|完成|成功/i.test(value(goal?.status))
const goalRunning = goal => goal?.running === true || /running|active|执行中|运行中/i.test(value(goal?.status))
const approvalExecution = item => value(item?.execution_state || (item?.decision === 'approved' ? 'queued' : item?.decision === 'rejected' ? 'not_applicable' : ''))
const approvalPending = item => item?.state === 'pending' && !item?.decision && !['completed', 'failed', 'not_applicable'].includes(approvalExecution(item))
const approvalTitle = item => value(item?.title || item?.id || '自主进化任务')

export const buildNotificationSnapshot = ({ schedule = {}, goals = [], approvals = {}, inventory = {} } = {}) => ({
  tasks: Object.fromEntries((Array.isArray(schedule?.tasks) ? schedule.tasks : []).map(task => [value(task.id), {
    id: value(task.id), status: value(task.status), error: value(task.error), report: reportKey(task.last_report),
  }]).filter(([id]) => id)),
  goals: Object.fromEntries((Array.isArray(goals) ? goals : []).map(goal => [value(goal.id), {
    id: value(goal.id), status: value(goal.status), running: goalRunning(goal), error: value(goal.error_class),
  }]).filter(([id]) => id)),
  approvals: Object.fromEntries((Array.isArray(approvals?.items) ? approvals.items : []).map(item => [value(item.id), {
    id: value(item.id), title: approvalTitle(item), state: value(item.state), decision: value(item.decision), execution: approvalExecution(item), pending: approvalPending(item),
  }]).filter(([id]) => id)),
  reports: Object.fromEntries((Array.isArray(inventory?.autonomous_reports) ? inventory.autonomous_reports : []).map(report => [reportKey(report), {
    path: value(report.path || report.name), key: reportKey(report),
  }]).filter(([key]) => key)),
})

const taskEvents = (previous, current) => {
  const events = []
  for (const [id, task] of Object.entries(current.tasks || {})) {
    const old = previous.tasks?.[id]
    if (!old) continue
    const failed = Boolean(task.error) || task.status === 'ERROR'
    const oldFailed = Boolean(old.error) || old.status === 'ERROR'
    if (task.report && task.report !== old.report) {
      events.push({
        category: 'schedule', level: failed ? 'error' : 'success', title: failed ? '定时任务执行失败' : '定时任务执行完成',
        message: failed ? `${id}：${task.error || '执行报告生成失败'}` : `${id} 已完成，本次执行报告已生成。`, route: 'tasks', subtab: 'reports', dedupeKey: `schedule:${id}:${task.report}`,
      })
    } else if (failed && !oldFailed) {
      events.push({ category: 'schedule', level: 'error', title: '定时任务执行失败', message: `${id}：${task.error || '任务状态异常'}`, route: 'tasks', dedupeKey: `schedule:${id}:error:${task.error || task.status}` })
    }
  }
  return events
}

const goalEvents = (previous, current) => {
  const events = []
  for (const [id, goal] of Object.entries(current.goals || {})) {
    const old = previous.goals?.[id]
    if (!old || old.running === goal.running && old.status === goal.status && old.error === goal.error) continue
    if (old.running && !goal.running) {
      const failed = goalFailure(goal)
      events.push({ category: 'goal', level: failed ? 'error' : goalSuccess(goal) ? 'success' : 'info', title: failed ? 'Goal 执行失败' : goalSuccess(goal) ? 'Goal 已完成' : 'Goal 已停止', message: failed ? `${id} 执行失败${goal.error ? `：${goal.error}` : '，请查看 Goal 详情。'}` : `${id} ${goalSuccess(goal) ? '已完成' : '已停止'}，可查看执行结果。`, route: 'goals', dedupeKey: `goal:${id}:${goal.status}:${goal.error}` })
    } else if (goal.error && !old.error) {
      events.push({ category: 'goal', level: 'error', title: 'Goal 运行异常', message: `${id}：${goal.error}`, route: 'goals', dedupeKey: `goal:${id}:error:${goal.error}` })
    }
  }
  return events
}

const approvalEvents = (previous, current) => {
  const events = []
  for (const [id, item] of Object.entries(current.approvals || {})) {
    const old = previous.approvals?.[id]
    if (!old && item.pending) {
      events.push({ category: 'autonomous', level: 'warning', title: '自主进化有待审批任务', message: `“${item.title}”需要你确认后才能继续执行。`, route: 'autonomous', dedupeKey: `approval:${id}:pending` })
      continue
    }
    if (!old || old.execution === item.execution) continue
    if (item.execution === 'completed') events.push({ category: 'autonomous', level: 'success', title: '自主进化任务已完成', message: `“${item.title}”已执行完成。`, route: 'autonomous', dedupeKey: `approval:${id}:completed` })
    if (item.execution === 'failed') events.push({ category: 'autonomous', level: 'error', title: '自主进化任务执行失败', message: `“${item.title}”执行失败，请查看执行记录。`, route: 'autonomous', dedupeKey: `approval:${id}:failed` })
    if (item.execution === 'queued') events.push({ category: 'autonomous', level: 'info', title: '自主进化任务已加入队列', message: `“${item.title}”已批准并等待执行。`, route: 'autonomous', dedupeKey: `approval:${id}:queued` })
  }
  return events
}

const reportEvents = (previous, current) => Object.entries(current.reports || {}).filter(([key]) => !previous.reports?.[key]).map(([, report]) => ({
  category: 'autonomous', level: 'success', title: '自主进化执行记录已更新', message: `${report.path} 已生成，可查看执行记录。`, route: 'autonomous', dedupeKey: `autonomous-report:${report.key}`,
}))

export const collectNotificationEvents = (previous, current) => {
  if (!previous || !current) return []
  return [...taskEvents(previous, current), ...goalEvents(previous, current), ...approvalEvents(previous, current), ...reportEvents(previous, current)]
}
