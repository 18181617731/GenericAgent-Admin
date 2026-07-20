const list = (value) => Array.isArray(value) ? value : []

const count = (value) => {
  const number = Number(value)
  return Number.isFinite(number) && number > 0 ? Math.floor(number) : 0
}

export function dashboardSummary(services, schedule = {}) {
  const managedServices = list(services)
  const runningServices = managedServices.filter(service => service?.running).length
  return {
    managedServices: managedServices.length,
    runningServices,
    taskCount: count(schedule?.task_count),
    enabledTasks: count(schedule?.enabled),
    overdueTasks: count(schedule?.overdue),
    taskErrors: count(schedule?.errors),
  }
}
