import { autonomousCopy, localizeAutonomousApprovalValue } from './autonomousCopy.js'

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

export const autonomousExecutionState = item => {
  if (item?.execution_state) return item.execution_state
  if (item?.decision === 'approved') return 'queued'
  if (item?.decision === 'rejected') return 'not_applicable'
  return ''
}

export const autonomousHandledProgress = item => {
  const state = autonomousExecutionState(item)
  if (state === 'queued' || state === 'completed' || state === 'report_missing' || state === 'failed' || state === 'not_applicable') return state
  return item?.decision === 'approved' ? 'queued' : 'unknown'
}

export const splitAutonomousApprovals = (items = []) => {
  const pending = items.filter(item => item?.state === 'pending')
  const handled = items.filter(item => item?.state !== 'pending')
  const handledGroups = ['queued', 'completed', 'report_missing', 'failed', 'not_applicable', 'unknown']
    .map(key => ({ key, items: handled.filter(item => autonomousHandledProgress(item) === key) }))
    .filter(group => group.items.length > 0)
  return { pending, handled, handledGroups }
}

const approvalOutcomeValue = item => item?.expected_outcome || item?.expected_result || item?.expected_effect || item?.outcome

const cleanApprovalOutcome = value => String(value || '').replace(/\s+/g, ' ').replace(/^[-*]+\s*/, '').trim()

export const summarizeAutonomousApproval = (item = {}, lang = 'zh') => {
  const explicit = cleanApprovalOutcome(localizeAutonomousApprovalValue(approvalOutcomeValue(item), lang, 'expectedOutcome'))
  if (explicit) return explicit
  const target = cleanApprovalOutcome(item.target)
  const nextStep = cleanApprovalOutcome(localizeAutonomousApprovalValue(item.next_step, lang, 'nextStep'))
  if (lang === 'en') {
    if (target) return `After approval, the proposal will be put into ${target} so it can be reused for similar situations.`
    if (nextStep) return `After approval, the autonomous workflow will ${nextStep.charAt(0).toLowerCase()}${nextStep.slice(1)}.`
    return 'After approval, the proposal will be added to the autonomous queue for SOP-based execution and reporting.'
  }
  if (target) return `批准后会把相关方案整理到 ${target}，以后遇到同类问题时可以直接参考。`
  if (nextStep) return `批准后会${nextStep.replace(/^用户批准后(?:再)?/, '').replace(/^批准后(?:再)?/, '')}。`
  return '批准后会加入自主任务队列，由自主服务按规则执行并生成报告。'
}

const reviewReasonText = value => String(value || '').replace(/\s+/g, ' ').trim()

const reviewReasonParts = (reason, copy, lang) => {
  const text = reviewReasonText(reason)
  const parts = []
  if (/approval gate|审批门槛|明确要求.*审批/i.test(text)) parts.push(copy.reviewGate)
  if (/retry scheduled|model review scheduled|review scheduled|安排重试|审核已安排|重试/i.test(text)) parts.push(copy.reviewRetryScheduled)
  if (/conservative rule|保守规则/i.test(text)) parts.push(copy.reviewConservative)
  if (!parts.length && text) parts.push(localizeAutonomousApprovalValue(text, lang, 'reviewReason'))
  return [...new Set(parts)]
}

const reviewModelText = (item, lang) => {
  const provider = reviewReasonText(item.review_provider)
  const model = reviewReasonText(item.review_model)
  const number = Number.isInteger(Number(item.review_model_no)) ? ` (#${Number(item.review_model_no)})` : ''
  if (!provider && !model && !number) return ''
  if (lang === 'en') return [provider, model].filter(Boolean).join(' · ') + number
  return [provider, model].filter(Boolean).join(' · ') + number
}

export const autonomousReviewView = (item = {}, lang = 'zh') => {
  const copy = autonomousCopy(lang)
  const status = reviewReasonText(item.review_status).toLowerCase()
  const reason = reviewReasonText(item.review_reason)
  const unavailable = /model review unavailable|模型审核不可用|审核模型.*(?:不可用|失败)|model.*review.*(?:unavailable|failed)/i.test(reason)
    || status === 'fallback'
  const rulesOnly = status === 'rule_fallback' || unavailable
  const pending = !rulesOnly && status.includes('pending')
  const modelUsed = !rulesOnly && !pending && (status.includes('model') || item.review_model || item.review_decision || item.review_confidence)
  const hasReviewData = Boolean(status || reason || item.review_model || item.review_decision || item.review_confidence)
  const kind = unavailable ? 'unavailable' : rulesOnly ? 'rules' : pending ? 'pending' : modelUsed ? 'model' : 'manual'
  const summary = kind === 'unavailable'
    ? copy.reviewModelUnavailableSummary
    : kind === 'rules'
      ? copy.reviewRuleFallbackSummary
      : kind === 'pending'
        ? copy.reviewPendingSummary
        : kind === 'model' ? copy.reviewModelUsedSummary : copy.reviewManualSummary
  const badge = kind === 'unavailable'
    ? copy.reviewModelUnavailable
    : kind === 'rules' ? copy.reviewRuleFallback
      : kind === 'pending' ? copy.reviewPending
        : kind === 'model' ? copy.reviewModelUsed : copy.reviewManual
  const decision = reviewReasonText(item.review_decision)
  return {
    kind,
    badge,
    method: rulesOnly ? copy.reviewRuleFallback : kind === 'model' ? copy.reviewModelUsed : badge,
    summary,
    basis: reviewReasonParts(reason, copy, lang),
    model: reviewModelText(item, lang),
    decision: decision === 'needs_approval' ? copy.reviewNoAutoApproval : localizeAutonomousApprovalValue(decision, lang, 'reviewDecision'),
    confidence: localizeAutonomousApprovalValue(reviewReasonText(item.review_confidence), lang, 'reviewConfidence'),
    hasReviewData,
  }
}

const approvalTargetText = (item, lang) => {
  const target = cleanApprovalOutcome(item?.target)
  if (!target) return ''
  return lang === 'en' ? ` The affected target is ${target}.` : `涉及的目标位置是 ${target}。`
}

export const summarizeAutonomousReviewNeed = (item = {}, review = autonomousReviewView(item), lang = 'zh') => {
  const evidence = cleanApprovalOutcome(localizeAutonomousApprovalValue(item.evidence, lang, 'evidence'))
  const target = approvalTargetText(item, lang)
  if (lang === 'en') {
    if (review.kind === 'unavailable') return `The review model is unavailable. The system only screened report approval markers, status, risk, and evidence with script rules; it did not make a model judgment. You must decide whether to approve it, and it must not be treated as model-approved.${target}`
    if (review.kind === 'rules') return `There is no usable model-review result. Conservative rules only kept this proposal pending, so the system cannot decide whether it should run. Review it yourself.${target}`
    if (review.kind === 'pending') return `The review model has not returned a result, so the system cannot decide whether this proposal should run. Review it yourself.${target}`
    if (review.kind === 'model') return `This proposal still needs your confirmation. The model is reference only and does not approve execution for you.${target}`
    return evidence ? `The report records this reason: ${evidence}. The proposal therefore needs your confirmation.${target}` : `The system marked this proposal for manual confirmation because no usable model conclusion is available.${target}`
  }
  if (review.kind === 'unavailable') return `用于审核的模型当前不可用。系统只根据报告中的审批标记、状态、风险和核查证据做脚本规则筛选，没有做模型判断；因此需要你人工决定批准还是拒绝，不能把这张卡片当成“模型已审核通过”。${target}`
  if (review.kind === 'rules') return `当前没有可用的模型审核结论。系统只按报告标记和保守规则把这项建议保留为待审批，不能判断它是否适合执行，需要你人工确认。${target}`
  if (review.kind === 'pending') return `审核模型还没有返回结果，系统暂时无法判断这项建议是否应该执行，需要你人工确认。${target}`
  if (review.kind === 'model') return `这项建议仍需要你确认。模型结论只能作为参考，不会替你批准执行。${target}`
  return evidence ? `报告记录的原因是：${evidence}。因此这项建议需要你人工确认。${target}` : `系统把这项建议标记为需要人工确认，当前没有可用的模型审核结论。${target}`
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
