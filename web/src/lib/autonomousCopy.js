const COPY = {
  zh: {
    services: '运行控制', approvals: '待审批', records: '执行记录', refresh: '刷新', running: '运行中', stopped: '已停止',
    serviceStat: '自主服务', pendingStat: '待审批', reportStat: '执行记录', latestStat: '最近执行', noRecent: '暂无记录',
    serviceIntro: '这里只显示可独立管理的自主执行服务。Goal/Hive 任务组件由对应工作流管理，服务看护器已归入总览的运行保障区域。',
    approvalIntro: '批准只会记录决定并加入自主任务队列；具体变更仍由自主服务按 SOP 执行。已处理卡片会持续显示排队、完成或失败状态，并可直接打开执行报告。',
    pending: '待处理', handled: '已处理', noPending: '当前没有待审批任务', noHandled: '还没有审批记录', noLedger: '未发现待批草案台账',
    approve: '批准并加入队列', reject: '拒绝', confirmReject: '确认拒绝', cancel: '取消', rejectNote: '拒绝原因（可选）',
    reply: '审批意见或补充要求（可选）', replyHelp: '此内容会随审批决定保存；批准时还会一并写入自主任务队列，供执行时参考。',
    approvalQueued: '已批准并加入自主任务队列', approvalRecorded: '审批决定已记录', approvalFailed: '审批失败',
    source: '来源', target: '落地目标', risk: '风险', evidence: '核查证据', nextStep: '批准后动作', reviewModel: '审批模型', reviewDecision: '模型判断', reviewConfidence: '置信度', reviewReason: '判断理由', reviewReports: '关联报告', openReviewReport: '查看报告', decidedAt: '处理时间', note: '备注',
    execution: '执行状态', executionQueued: '已排队，等待自主服务执行', executionCompleted: '已完成', executionFailed: '执行失败', executionReportMissing: '执行已结束，但报告缺失', executionNotApplicable: '无需执行', executionUnknown: '等待执行状态', executionSummary: '执行摘要', openExecutionReport: '查看执行结果',
    reportSearch: '搜索执行记录', reportCount: count => `${count} 条记录`, noReports: '没有匹配的执行记录', selectReport: '选择左侧记录查看详情',
    backToReports: '返回记录列表', download: '下载原文件', loading: '正在加载', loadFailed: '读取失败',
    latestResult: '最近执行结果', reportReady: '已生成报告', openReport: '查看完整记录', noResult: '报告已生成，打开完整记录查看执行详情。',
  },
  en: {
    services: 'Runtime', approvals: 'Approvals', records: 'Execution records', refresh: 'Refresh', running: 'Running', stopped: 'Stopped',
    serviceStat: 'Autonomous services', pendingStat: 'Pending approvals', reportStat: 'Execution records', latestStat: 'Latest run', noRecent: 'No records',
    serviceIntro: 'Only independently managed autonomous execution services appear here. Goal/Hive run components are managed by their workflow, while the watchdog is shown under runtime protection on Overview.',
    approvalIntro: 'Approval records the decision and queues a task. The autonomous service still applies changes under its SOP; handled cards show queued, completed, or failed status and link to the execution report.',
    pending: 'Pending', handled: 'Handled', noPending: 'No tasks are waiting for approval', noHandled: 'No approval decisions yet', noLedger: 'No pending-draft ledger was found',
    approve: 'Approve and queue', reject: 'Reject', confirmReject: 'Confirm rejection', cancel: 'Cancel', rejectNote: 'Reason for rejection (optional)',
    reply: 'Approval note or extra requirements (optional)', replyHelp: 'Saved with the decision. For approvals, it is also added to the autonomous task queue for execution context.',
    approvalQueued: 'Approved and added to the autonomous task queue', approvalRecorded: 'Approval decision recorded', approvalFailed: 'Approval failed',
    source: 'Source', target: 'Target', risk: 'Risk', evidence: 'Evidence', nextStep: 'Action after approval', reviewModel: 'Review model', reviewDecision: 'Model decision', reviewConfidence: 'Confidence', reviewReason: 'Reason', reviewReports: 'Related reports', openReviewReport: 'Open report', decidedAt: 'Decided', note: 'Note',
    execution: 'Execution', executionQueued: 'Queued; waiting for the autonomous service', executionCompleted: 'Completed', executionFailed: 'Execution failed', executionReportMissing: 'Finished, but the report is missing', executionNotApplicable: 'Not applicable', executionUnknown: 'Waiting for execution status', executionSummary: 'Execution summary', openExecutionReport: 'View execution result',
    reportSearch: 'Search execution records', reportCount: count => `${count} records`, noReports: 'No matching execution records', selectReport: 'Select a record to view details',
    backToReports: 'Back to records', download: 'Download original', loading: 'Loading', loadFailed: 'Could not load',
    latestResult: 'Latest result', reportReady: 'Report ready', openReport: 'Open full record', noResult: 'The report is ready. Open the full record for execution details.',
  },
}

const APPROVAL_EXACT_ZH = {
  status: {
    'report requires human approval': '报告需要人工审批',
    'human review required': '需要人工复核',
    'pending approval': '待审批',
    'awaiting user approval': '等待用户审批',
    needs_approval: '需要审批',
    not_required: '无需审批',
    uncertain: '无法确定',
    pending: '待审批',
    approved: '已批准',
    rejected: '已拒绝',
    closed: '已关闭',
    tracked: '已跟踪',
  },
  risk: {
    'human review required': '需要人工复核',
    'high risk': '高风险',
    high_risk: '高风险',
    medium_risk: '中风险',
    low_risk: '低风险',
    requires_human_review: '需要人工复核',
    needs_approval: '需要审批',
    high: '高',
    medium: '中',
    low: '低',
    review: '需复核',
  },
  reviewDecision: {
    needs_approval: '需要审批',
    not_required: '无需审批',
    uncertain: '无法确定',
  },
  reviewConfidence: {
    high: '高',
    medium: '中',
    low: '低',
  },
  executionSummary: {
    completed: '已完成',
    failed: '执行失败',
    queued: '已排队',
  },
  executionError: {
    'execution failed': '执行失败',
    'report is missing': '报告缺失',
    'service is not running': '服务未运行',
  },
}

const APPROVAL_PHRASES_ZH = [
  ['report is blocked', '报告处于阻塞状态'],
  ['approval evidence is missing or unverifiable', '审批证据缺失或无法核验'],
  ['approval evidence cannot be verified', '审批证据无法核验'],
  ['approval evidence is missing', '审批证据缺失'],
  ['the proposed source change is not confirmed as implemented', '拟议源码变更尚未确认实施'],
  ['model review unavailable', '模型审核不可用'],
  ['model review in progress', '模型审核进行中'],
  ['model review scheduled', '模型审核已排队'],
  ['model review timed out after', '模型审核超时，耗时'],
  ['model omitted this report', '模型未返回该报告的审核结果'],
  ['GA runtime is not available', 'GA 运行时不可用'],
  ['model review failed', '模型审核失败'],
  ['retry scheduled', '已安排重试'],
  ['conservative rule retained', '已保留保守规则'],
  ['model review batch limit reached', '已达到模型审核批次上限'],
  ['report contains an explicit approval gate', '报告包含明确的审批门槛'],
  ['review the report evidence, then approve or reject explicitly', '请核查报告证据后明确批准或拒绝'],
  ['report requires human approval', '报告需要人工审批'],
  ['human review required', '需要人工复核'],
  ['pending approval', '待审批'],
  ['awaiting user approval', '等待用户审批'],
  ['needs_approval', '需要审批'],
  ['not_required', '无需审批'],
  ['uncertain', '无法确定'],
]

const APPROVAL_TITLE_PHRASES_ZH = [
  ['complete_task', '完成任务'],
  ['autonomous approval report', '自主审批报告'],
  ['approval', '审批'],
  ['blocked', '阻塞'],
  ['review', '复核'],
  ['report', '报告'],
]

const escapeRegExp = value => value.replace(/[.*+?^${}()|[\\]\\\\]/g, '\\\\$&')

export const localizeAutonomousApprovalValue = (value, lang = 'zh', field = '') => {
  if (value === null || value === undefined || lang === 'en') return value
  const text = String(value)
  const exact = APPROVAL_EXACT_ZH[field]?.[text.trim().toLowerCase()]
  if (exact) return exact
  const phrases = field === 'title' ? [...APPROVAL_PHRASES_ZH, ...APPROVAL_TITLE_PHRASES_ZH] : APPROVAL_PHRASES_ZH
  return phrases.reduce((result, [source, target]) => result.replace(new RegExp(escapeRegExp(source), 'gi'), target), text)
}

export const autonomousCopy = lang => COPY[lang === 'en' ? 'en' : 'zh']
