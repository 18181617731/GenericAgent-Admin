const COPY = {
  zh: {
    services: '运行控制', approvals: '待审批', records: '执行记录', refresh: '刷新', running: '运行中', stopped: '已停止',
    serviceStat: '自主服务', pendingStat: '待审批', reportStat: '执行记录', latestStat: '最近执行', noRecent: '暂无记录',
    serviceIntro: '这里只显示可独立管理的自主执行服务。Goal/Hive 任务组件由对应工作流管理，服务看护器已归入总览的运行保障区域。',
    approvalIntro: '模型会先判断风险和证据：低/中风险、证据充分且置信度足够的建议会自动批准并加入队列；高风险、风险不明、证据不足或模型不可用的建议仍需人工确认。批准只会加入执行队列，不会立即修改文件。',
    pending: '待处理', handled: '已处理', noPending: '当前没有待审批任务', noHandled: '还没有审批记录', noLedger: '未发现待批草案台账', handledGroupsHelp: '已处理记录按执行进展分类，点击分类标题展开或收起。', handledGroups: { queued: { label: '已加入 TODO，等待执行', description: '审批已通过，任务已进入 TODO 队列，等待自主服务执行' }, completed: { label: '已完成并归档', description: '任务已执行完成，并且已经生成归档报告' }, report_missing: { label: '已完成，但没有归档文件', description: '任务已结束，但暂未找到对应的归档报告' }, failed: { label: '执行失败', description: '任务执行过程中发生错误，需要检查失败原因' }, not_applicable: { label: '已拒绝，不执行', description: '审批未通过，不会进入自主任务队列' }, not_required: { label: '无需审批，不执行', description: '系统已确认这项工作不需要人工审批' }, closed: { label: '已完成或已关闭', description: '任务已完成、归档或被替代，不再需要审批' }, unknown: { label: '已处理，等待状态同步', description: '审批结果已记录，但执行状态还未同步' } },
    selectAll: '全选待审批', clearSelection: '取消全选', selectedCount: count => `已选 ${count} 项`, selectItem: '选择',
    approveMany: '批量批准并加入队列', rejectMany: '批量拒绝', approveManyConfirm: count => `确定批准选中的 ${count} 项，并加入自主任务队列吗？`, rejectManyTitle: count => `拒绝 ${count} 项待审批`, rejectManyConfirm: count => `确定拒绝选中的 ${count} 项吗？`, bulkProcessing: count => `正在处理 ${count} 项待审批，请稍候…`, bulkSuccess: count => `已处理 ${count} 项待审批`, bulkPartial: (done, total, error) => `已处理 ${done}/${total} 项，剩余项目未完成：${error}`,
    bulkProgressTitle: '批量处理进度', bulkProgressWorking: '处理中', bulkProgressFinished: '处理完成', bulkProgressCount: (done, total) => `已处理 ${done} / ${total} 项`, bulkProgressPercent: percent => `${percent}%`, bulkProgressSuccess: count => `成功 ${count} 项`, bulkProgressFailed: count => `失败 ${count} 项`, bulkProgressCurrent: '当前处理', bulkProgressLast: '最后处理', bulkProgressDetails: '处理明细', bulkProgressQueued: '已批准并加入队列', bulkProgressRecorded: '审批结果已记录', bulkProgressError: '失败原因', retryFailed: count => `重试失败项（${count}）`, retryFailedConfirm: count => `确定重试失败的 ${count} 项吗？`, closeProgress: '关闭进度明细',
    approve: '批准并加入队列', reject: '拒绝', confirmReject: '确认拒绝', cancel: '取消', rejectNote: '拒绝原因（可选）',
    reply: '审批意见或补充要求（可选）', replyHelp: '用大白话写下你的要求；批准时会一起放进任务队列，供自主服务执行时参考。',
    approvalQueued: '已批准并加入自主任务队列', approvalRecorded: '审批决定已记录', approvalFailed: '审批失败', reviewNow: '重新审核仍待处理项', reviewConfirm: '将使用“自主进化”配置的模型重新审核待处理建议；符合低/中风险自动批准条件的会直接加入队列，其余仍保留给人工确认。是否继续？', reviewStarted: '正在调用审核模型，请稍候…', reviewCompleted: (count, autoApproved = 0) => autoApproved > 0 ? `已完成 ${count} 项审核，模型自动批准 ${autoApproved} 项；其余仍需人工确认` : `已完成 ${count} 项审核，未满足自动批准条件的项目仍需人工确认`, reviewFailed: '审核请求失败',
    reviewMethod: '审核方式', reviewWhy: '为什么需要你审核', reviewRuleFallback: '仅规则筛选（未经过模型判断）', reviewModelUnavailable: '模型审核不可用', reviewModelUsed: '模型已参与判断', reviewModelAutoApproved: '模型自动批准', reviewManual: '等待人工确认', reviewPending: '等待模型审核', reviewFocus: '审批重点', reviewOptions: '报告中的可选方案', reviewRecommended: '推荐', reviewMoreOptions: count => `还有 ${count} 项方案，请打开关联报告查看。`, reviewTagChoice: '有多个方案', reviewTagBlocked: '存在阻塞或证据不足', reviewTagFileChange: '涉及文件或代码', reviewTagConfigChange: '涉及配置切换', reviewTagVerification: '需要验证或验收', reviewTagDocumentation: 'SOP/文档沉淀', reviewTagObservation: '只读观察', reviewTagCompleted: '报告显示已完成', reviewTagManual: '需要人工确认', reviewFocusChoice: '报告给出了多个可选方案，批准前需要确认采用哪一个方案。', reviewFocusBlocked: '报告存在阻塞或审批证据不足，需先确认是否继续处理。', reviewFocusFileAndConfig: '这项建议同时涉及文件或代码与运行配置变更，需确认变更范围。', reviewFocusFileChange: '这项建议可能修改文件或代码，需确认是否允许落地。', reviewFocusConfigChange: '这项建议会改变运行配置或调度参数，需确认是否切换。', reviewFocusVerification: '这项建议主要用于验证、实测或健康检查，需确认是否继续执行。', reviewFocusDocumentation: '这项建议主要完善 SOP、记忆或项目文档，需确认是否写入。', reviewFocusObservation: '这项建议以只读观察或环境探测为主，不应直接修改文件。', reviewFocusGeneral: '只有高风险、风险不明、证据不足或低置信度项目才会留给人工确认。',
    reviewRuleFallbackSummary: '用于审核的模型当前不可用。这张卡片不是模型审核通过的结果，也不是系统自动批准；系统只根据报告标记和保守规则把它保留在待审批列表，请你人工判断。点击重新审核时会按模型页面配置再次尝试。',
    reviewModelUnavailableSummary: '用于审核的模型当前不可用。这张卡片不是模型审核通过的结果，也不是系统自动批准；系统只根据报告中的审批标记、状态、风险和核查证据做脚本规则筛选，请你人工决定批准还是拒绝。点击重新审核时会按模型页面配置再次尝试。',
    reviewModelUsedSummary: '审核模型已参与判断。只有模型建议批准、风险为低/中、置信度为中/高且没有阻塞或缺失证据时，系统才会自动批准；当前这项仍需你确认。',
    reviewAutoApprovedSummary: '审核模型判断这项建议为低/中风险，证据充分且置信度足够，系统已自动批准并加入自主任务队列。批准只代表允许服务继续处理，不会立即修改文件。',
    reviewPendingSummary: '审核模型尚未给出结果，当前卡片暂时不能视为模型审核通过。',
    reviewManualSummary: '当前卡片需要人工确认，页面没有可用的模型审核结论。',
    reviewBasis: '系统依据', reviewDecision: '模型结论', reviewRisk: '模型风险', reviewConfidence: '置信度', reviewModel: '审核模型', reviewReason: '审核原因', reviewNoAutoApproval: '未自动批准', reviewRetryScheduled: '重新审核时会再次尝试', reviewGate: '报告明确要求人工审批', reviewBlocked: '报告处于阻塞状态', reviewChangeUnconfirmed: '拟议源码变更尚未确认实施', reviewEvidenceMissing: '审批证据缺失或无法核验', reviewConservative: '暂时按保守规则保留为待审批',
    problem: '要解决什么问题', source: '来自哪里', target: '会改哪里', risk: '风险大小', evidence: '为什么要处理', nextStep: '批准后做什么', decidedAt: '处理时间', note: '备注',
    execution: '执行状态', executionQueued: '已排队，等待自主服务执行', executionCompleted: '已完成', executionFailed: '执行失败', executionReportMissing: '执行已结束，但报告缺失', executionNotApplicable: '无需执行', executionUnknown: '等待执行状态', executionSummary: '执行摘要', openExecutionReport: '查看执行结果', reviewReports: '关联审核报告', openReviewReport: '查看报告',
    reportSearch: '搜索执行记录', reportCount: count => `${count} 条记录`, noReports: '没有匹配的执行记录', selectReport: '选择左侧记录查看详情',
    backToReports: '返回记录列表', download: '下载原文件', loading: '正在加载', loadFailed: '读取失败',
    latestResult: '最近执行结果', reportReady: '已生成报告', openReport: '查看完整记录', noResult: '报告已生成，打开完整记录查看执行详情。',
    quickCreatePlaceholder: '用一句话描述你的任务，例如：每天早上整理一次收件箱', quickCreate: '智能创建', manualCreate: '手动新建', quickCreateParsing: 'AI 解析中…', quickCreatePreview: 'AI 已解析出任务草稿，可直接创建或继续编辑：', quickCreateFallback: '智能解析暂不可用，将按你的原文创建草稿。', quickCreateCreate: '创建草稿', quickCreateEditFull: '编辑完整表单', quickCreateCancel: '取消', quickCreateModel: count => `解析模型：${count}`,
  },
  en: {
    services: 'Runtime', approvals: 'Approvals', records: 'Execution records', refresh: 'Refresh', running: 'Running', stopped: 'Stopped',
    serviceStat: 'Autonomous services', pendingStat: 'Pending approvals', reportStat: 'Execution records', latestStat: 'Latest run', noRecent: 'No records',
    serviceIntro: 'Only independently managed autonomous execution services appear here. Goal/Hive run components are managed by their workflow, while the watchdog is shown under runtime protection on Overview.',
    approvalIntro: 'The model checks risk and evidence first: low or medium-risk proposals with sufficient evidence and confidence are auto-approved and queued; high-risk, unknown, blocked, or model-unavailable proposals stay for human confirmation. Approval only adds work to the queue; it does not edit files immediately.',
    pending: 'Pending', handled: 'Handled', noPending: 'No tasks are waiting for approval', noHandled: 'No approval decisions yet', noLedger: 'No pending-draft ledger was found', handledGroupsHelp: 'Handled records are grouped by execution progress. Select a group header to expand or collapse it.', handledGroups: { queued: { label: 'Added to TODO; waiting to run', description: 'Approved and queued for the autonomous service' }, completed: { label: 'Completed and archived', description: 'Execution finished and an archive report is available' }, report_missing: { label: 'Completed, but no archive file', description: 'Execution finished but the archive report is missing' }, failed: { label: 'Execution failed', description: 'Execution failed and needs investigation' }, not_applicable: { label: 'Rejected; not executed', description: 'Rejected items do not enter the autonomous queue' }, not_required: { label: 'No approval required; not executed', description: 'The system confirmed that this work does not need human approval' }, closed: { label: 'Completed or closed', description: 'The task is complete, archived, or superseded and no longer needs approval' }, unknown: { label: 'Handled; waiting for status sync', description: 'The decision is recorded but execution status is not synchronized yet' } },
    selectAll: 'Select all pending', clearSelection: 'Clear selection', selectedCount: count => `${count} selected`, selectItem: 'Select',
    approveMany: 'Approve selected and queue', rejectMany: 'Reject selected', approveManyConfirm: count => `Approve the ${count} selected items and add them to the autonomous queue?`, rejectManyTitle: count => `Reject ${count} pending items`, rejectManyConfirm: count => `Reject the ${count} selected items?`, bulkProcessing: count => `Handling ${count} pending items, please wait…`, bulkSuccess: count => `Handled ${count} pending items`, bulkPartial: (done, total, error) => `Handled ${done}/${total}; the remaining items were not completed: ${error}`,
    bulkProgressTitle: 'Batch progress', bulkProgressWorking: 'In progress', bulkProgressFinished: 'Finished', bulkProgressCount: (done, total) => `${done} / ${total} handled`, bulkProgressPercent: percent => `${percent}%`, bulkProgressSuccess: count => `${count} succeeded`, bulkProgressFailed: count => `${count} failed`, bulkProgressCurrent: 'Current item', bulkProgressLast: 'Last item', bulkProgressDetails: 'Item details', bulkProgressQueued: 'Approved and queued', bulkProgressRecorded: 'Decision recorded', bulkProgressError: 'Failure reason', retryFailed: count => `Retry failed items (${count})`, retryFailedConfirm: count => `Retry the ${count} failed items?`, closeProgress: 'Close progress details',
    approve: 'Approve and queue', reject: 'Reject', confirmReject: 'Confirm rejection', cancel: 'Cancel', rejectNote: 'Reason for rejection (optional)',
    reply: 'Approval note or extra requirements (optional)', replyHelp: 'Write the requirement in plain language. For approvals, it is added to the queue as execution context.',
    approvalQueued: 'Approved and added to the autonomous task queue', approvalRecorded: 'Approval decision recorded', approvalFailed: 'Approval failed', reviewNow: 'Re-review items still waiting', reviewConfirm: 'The configured autonomy model will re-review pending proposals; items that meet the low or medium-risk auto-approval policy will be queued directly, while the rest stay for human confirmation. Continue?', reviewStarted: 'The review model is running, please wait…', reviewCompleted: (count, autoApproved = 0) => autoApproved > 0 ? `${count} review(s) completed; ${autoApproved} auto-approved by the model, the rest still need human confirmation` : `${count} review(s) completed; items that do not meet the auto-approval policy still need human confirmation`, reviewFailed: 'Review request failed',
    reviewMethod: 'Review method', reviewWhy: 'Why you need to review it', reviewRuleFallback: 'Rule-only screening (no model judgment)', reviewModelUnavailable: 'Review model unavailable', reviewModelUsed: 'Model participated', reviewModelAutoApproved: 'Model auto-approved', reviewManual: 'Manual confirmation', reviewPending: 'Model review pending', reviewFocus: 'Approval focus', reviewOptions: 'Options found in the report', reviewRecommended: 'Recommended', reviewMoreOptions: count => `${count} more option(s); open the related report to see them.`, reviewTagChoice: 'Multiple options', reviewTagBlocked: 'Blocked or evidence missing', reviewTagFileChange: 'File or code change', reviewTagConfigChange: 'Configuration change', reviewTagVerification: 'Verification required', reviewTagDocumentation: 'SOP/documentation', reviewTagObservation: 'Read-only observation', reviewTagCompleted: 'Report says completed', reviewTagManual: 'Manual confirmation needed', reviewFocusChoice: 'The report offers multiple options; confirm which one should be used before approval.', reviewFocusBlocked: 'The report is blocked or lacks approval evidence; confirm whether it should proceed.', reviewFocusFileAndConfig: 'This proposal changes files/code and runtime configuration; confirm the scope.', reviewFocusFileChange: 'This proposal may modify files or code; confirm whether it may be applied.', reviewFocusConfigChange: 'This proposal changes runtime configuration or scheduling; confirm whether to switch it.', reviewFocusVerification: 'This proposal is mainly verification, testing, or health checking; confirm whether to run it.', reviewFocusDocumentation: 'This proposal mainly updates SOP, memory, or project documentation; confirm whether to write it.', reviewFocusObservation: 'This proposal is read-only observation or environment probing and should not modify files.', reviewFocusGeneral: 'Only high-risk, unknown-risk, blocked, evidence-deficient, or low-confidence items stay for human confirmation.',
    reviewRuleFallbackSummary: 'The review model is currently unavailable. This card is not a model-approved result and is not auto-approved; the system kept it pending using report markers and conservative rules. Review it yourself; clicking re-review will try again with the model settings.',
    reviewModelUnavailableSummary: 'The review model is currently unavailable. This card is not a model-approved result and is not auto-approved; the system only screened report approval markers, status, risk, and evidence with script rules. Decide whether to approve or reject it; clicking re-review will try again with the model settings.',
    reviewModelUsedSummary: 'The review model participated. The system auto-approves only when the model recommends approval, rates the risk low or medium, has medium or high confidence, and finds no blocker or missing evidence; this item still needs your confirmation.',
    reviewAutoApprovedSummary: 'The review model rated this proposal low or medium risk with sufficient evidence and confidence. It was auto-approved and added to the autonomous queue. Approval lets the service continue; it does not edit files immediately.',
    reviewPendingSummary: 'The review model has not returned a result yet, so this card must not be treated as model-approved.',
    reviewManualSummary: 'This card requires manual confirmation; no usable model-review result is available.',
    reviewBasis: 'System basis', reviewDecision: 'Model decision', reviewRisk: 'Model risk', reviewConfidence: 'Confidence', reviewModel: 'Review model', reviewReason: 'Review reason', reviewNoAutoApproval: 'Not auto-approved', reviewRetryScheduled: 'Will try again on re-review', reviewGate: 'The report explicitly requires human approval', reviewBlocked: 'The report is blocked', reviewChangeUnconfirmed: 'The proposed source change is not confirmed as implemented', reviewEvidenceMissing: 'Approval evidence is missing or unverifiable', reviewConservative: 'Kept pending under conservative rules',
    problem: 'Problem this solves', source: 'Where it came from', target: 'What it changes', risk: 'Risk level', evidence: 'Why it matters', nextStep: 'What happens next', decidedAt: 'Decided', note: 'Note',
    execution: 'Execution', executionQueued: 'Queued; waiting for the autonomous service', executionCompleted: 'Completed', executionFailed: 'Execution failed', executionReportMissing: 'Finished, but the report is missing', executionNotApplicable: 'Not applicable', executionUnknown: 'Waiting for execution status', executionSummary: 'Execution summary', openExecutionReport: 'View execution result', reviewReports: 'Related reports', openReviewReport: 'Open report',
    reportSearch: 'Search execution records', reportCount: count => `${count} records`, noReports: 'No matching execution records', selectReport: 'Select a record to view details',
    backToReports: 'Back to records', download: 'Download original', loading: 'Loading', loadFailed: 'Could not load',
    latestResult: 'Latest result', reportReady: 'Report ready', openReport: 'Open full record', noResult: 'The report is ready. Open the full record for execution details.',
    quickCreatePlaceholder: 'Describe your task in one line, e.g. tidy the inbox every morning', quickCreate: 'Smart create', manualCreate: 'New manually', quickCreateParsing: 'Parsing with AI…', quickCreatePreview: 'AI drafted a task. Create it directly or keep editing:', quickCreateFallback: 'Smart parsing is unavailable; the draft will use your raw input.', quickCreateCreate: 'Create draft', quickCreateEditFull: 'Edit full form', quickCreateCancel: 'Cancel', quickCreateModel: count => `Parsed by ${count}`,
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
    unknown: '未知',
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

const escapeRegExp = value => value.replace(/[.*+?^${}()|[\]\\]/g, '\\$&')

export const localizeAutonomousApprovalValue = (value, lang = 'zh', field = '') => {
  if (value === null || value === undefined || lang === 'en') return value
  const text = String(value)
  const exact = APPROVAL_EXACT_ZH[field]?.[text.trim().toLowerCase()]
  if (exact) return exact
  const phrases = field === 'title' ? [...APPROVAL_PHRASES_ZH, ...APPROVAL_TITLE_PHRASES_ZH] : APPROVAL_PHRASES_ZH
  return phrases.reduce((result, [source, target]) => result.replace(new RegExp(escapeRegExp(source), 'gi'), target), text)
}

export const autonomousCopy = lang => COPY[lang === 'en' ? 'en' : 'zh']
