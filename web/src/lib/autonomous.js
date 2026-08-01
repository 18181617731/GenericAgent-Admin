const SERVICE_DETAILS = {
  'reflect/autonomous.py': {
    zh: ['主自主引擎', '检测空闲状态并按自主行动 SOP 推进一项任务。', '核心后台服务。它定期检查用户是否处于空闲状态，满足条件后从自主任务队列选择一项工作，并按 SOP 执行和生成报告。仅在需要无人值守推进任务时启用；执行模型可在服务停止后调整。'],
    en: ['Autonomy engine', 'Detects idle time and advances one task under the autonomy SOP.', 'Core background service. It periodically checks whether the user is idle, selects one queued task, runs it under the SOP, and writes a report. Enable it only for unattended work; change its model while stopped.'],
  },
  'reflect/agent_team_worker.py': {
    zh: ['团队协作工作器', '承接多智能体协作中的独立执行任务。', '用于多智能体协作场景，接收团队调度器分配的独立任务。普通单智能体使用无需常驻启动，避免占用模型并发和产生额外调用。'],
    en: ['Team worker', 'Handles independent work assigned by multi-agent collaboration.', 'Receives independent work from multi-agent coordination. It does not need to run for ordinary single-agent use, which avoids consuming model concurrency and extra calls.'],
  },
  'reflect/checklist_master.py': {
    zh: ['检查清单管理器', '按检查清单跟踪执行步骤和完成状态。', '负责读取和推进检查清单型任务，记录步骤是否完成。仅在使用检查清单工作流时启动；它不会替代主自主引擎，也不会自行创建审批决定。'],
    en: ['Checklist manager', 'Tracks execution steps and completion through checklists.', 'Reads and advances checklist-based tasks and records step completion. Start it only for checklist workflows; it does not replace the autonomy engine or make approval decisions.'],
  },
  'reflect/watchdog.py': {
    zh: ['服务看护器', '监测关键后台服务，并在异常时提供恢复信号。', '监测自主相关后台进程的存活状态并提供异常恢复信号。适合需要长期运行自主服务的环境；只做看护，不执行自主任务，也不代表被看护服务本身已健康。'],
    en: ['Service watchdog', 'Monitors background services and emits recovery signals.', 'Monitors autonomous background processes and emits recovery signals. Use it for long-running deployments; it does not execute autonomous tasks or prove that a monitored service is healthy.'],
  },
}

export const autonomousServiceView = (service, lang = 'zh') => {
  const fallback = String(service?.name || '').split(/[\\/]/).pop()?.replace(/\.py$/i, '') || '-'
  const detail = SERVICE_DETAILS[service?.name]?.[lang] || [fallback, lang === 'en' ? 'Auxiliary autonomous service.' : '自主进化辅助服务。']
  return { title: detail[0], description: detail[1], help: detail[2] || detail[1], technicalName: service?.name || fallback }
}

export const autonomousSummary = ({ services = [], approvals = {}, reports = [] } = {}) => ({
  running: services.filter(service => service?.running).length,
  total: services.length,
  pending: Number(approvals?.pending) || 0,
  reports: reports.length,
  latestReport: reports[0] || null,
})

export const filterAutonomousReports = (reports = [], query = '') => {
  const needle = String(query || '').trim().toLocaleLowerCase()
  if (!needle) return reports
  return reports.filter(report => `${report?.name || ''} ${report?.path || ''}`.toLocaleLowerCase().includes(needle))
}

export const splitAutonomousApprovals = (items = []) => ({
  pending: items.filter(item => item?.state === 'pending'),
  handled: items.filter(item => item?.state !== 'pending'),
})

export const autonomousExecutionState = item => {
  if (item?.execution_state) return item.execution_state
  if (item?.decision === 'approved') return 'queued'
  if (item?.decision === 'rejected') return 'not_applicable'
  return ''
}

export const readableAutonomousDate = (value, lang = 'zh') => {
  const date = new Date(value)
  if (!value || Number.isNaN(date.getTime())) return lang === 'en' ? 'Unknown time' : '时间未知'
  return date.toLocaleString(lang === 'en' ? 'en-US' : 'zh-CN')
}

export const latestAutonomousReport = (reports = []) => reports.find(report => /\.md$/i.test(String(report?.path || report?.name || ''))) || reports[0] || null

export const summarizeAutonomousReport = (content, limit = 260) => {
  const lines = String(content || '').replace(/\r\n/g, '\n').split('\n')
  const lastHeading = lines.reduce((index, line, current) => /^#{1,6}\s+/.test(line.trim()) ? current : index, -1)
  const candidates = lines.slice(lastHeading + 1).map(line => line.trim()).filter(line => line && !/^```/.test(line) && !/^\|?(?:\s*:?-+:?\s*\|)+$/.test(line) && !/^\|/.test(line))
  const summary = candidates.join(' ').replace(/^[-*+]\s+/, '').replace(/[`*_>#]/g, '').replace(/\s+/g, ' ').trim()
  if (!summary) return ''
  return summary.length > limit ? `${summary.slice(0, limit).trim()}...` : summary
}
