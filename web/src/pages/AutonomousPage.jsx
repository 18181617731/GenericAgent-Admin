import React, { useCallback, useEffect, useMemo, useState } from 'react'
import ReactMarkdown from 'react-markdown'
import remarkGfm from 'remark-gfm'
import { AlertCircle, ArrowLeft, Check, CheckCircle2, ChevronDown, Download, RefreshCw, Search, X } from 'lucide-react'
import { AutonomousServiceCard } from '../components/AutonomousServiceCard.jsx'
import { api } from '../lib/api.js'
import { confirmDanger } from '../lib/danger.js'
import { AutonomousTaskWorkspace } from '../components/AutonomousTaskWorkspace.jsx'
import { autonomousCopy, localizeAutonomousApprovalValue } from '../lib/autonomousCopy.js'
import { autonomousExecutionState, autonomousReviewView, autonomousSummary, filterAutonomousReports, latestAutonomousReport, readableAutonomousDate, splitAutonomousApprovals, summarizeAutonomousProblem, summarizeAutonomousReport } from '../lib/autonomous.js'

const approvalDetails = (item, copy, lang) => [
  [copy.source, item.source || item.draft_path],
  [copy.target, item.target],
  [copy.risk, localizeAutonomousApprovalValue(item.risk, lang, 'risk')],
  [copy.evidence, localizeAutonomousApprovalValue(item.evidence, lang, 'evidence')],
].filter(([, value]) => value)

const zhDetailLabel = (lang, zh, en) => lang === 'en' ? en : zh

const executionPresentation = (item, copy) => {
  const state = autonomousExecutionState(item)
  const labels = {
    queued: copy.executionQueued,
    completed: copy.executionCompleted,
    failed: copy.executionFailed,
    report_missing: copy.executionReportMissing,
    not_applicable: copy.executionNotApplicable,
  }
  return { state, label: labels[state] || copy.executionUnknown }
}

const reviewTagLabels = (copy) => ({
  choice: copy.reviewTagChoice,
  blocked: copy.reviewTagBlocked,
  file_change: copy.reviewTagFileChange,
  config_change: copy.reviewTagConfigChange,
  verification: copy.reviewTagVerification,
  documentation: copy.reviewTagDocumentation,
  observation: copy.reviewTagObservation,
  completed: copy.reviewTagCompleted,
  manual: copy.reviewTagManual,
})

const reviewFocusText = (item, copy, lang) => {
  const tags = Array.isArray(item?.review_tags) ? item.review_tags : []
  if (tags.includes('choice')) return copy.reviewFocusChoice
  if (tags.includes('blocked')) return copy.reviewFocusBlocked
  if (tags.includes('file_change') && tags.includes('config_change')) return copy.reviewFocusFileAndConfig
  if (tags.includes('file_change')) return copy.reviewFocusFileChange
  if (tags.includes('config_change')) return copy.reviewFocusConfigChange
  if (tags.includes('verification')) return copy.reviewFocusVerification
  if (tags.includes('documentation')) return copy.reviewFocusDocumentation
  if (tags.includes('observation')) return copy.reviewFocusObservation
  return lang === 'en' ? copy.reviewFocusGeneral : item?.review_focus || copy.reviewFocusGeneral
}

function ApprovalReviewContext({ item, lang, copy }) {
  const tags = Array.isArray(item?.review_tags) ? item.review_tags : []
  const options = Array.isArray(item?.review_options) ? item.review_options : []
  if (!tags.length && !options.length && !item?.review_focus) return null
  const labels = reviewTagLabels(copy)
  const visibleOptions = options.slice(0, 4)
  return <section className="autonomous-approval-focus" aria-label={copy.reviewFocus}>
    <div className="autonomous-approval-focus-head">
      <strong>{copy.reviewFocus}</strong>
      <div className="autonomous-review-tags">{tags.map(tag => labels[tag] ? <span key={tag}>{labels[tag]}</span> : null)}</div>
    </div>
    <p>{reviewFocusText(item, copy, lang)}</p>
    {visibleOptions.length > 0 && <div className="autonomous-review-options">
      <b>{copy.reviewOptions}</b>
      <ol>{visibleOptions.map(option => <li key={`${option.key}-${option.title}`}>
        <div><strong>{option.key}. {localizeAutonomousApprovalValue(option.title, lang, 'reviewOption')}</strong>{option.recommended && <em>{copy.reviewRecommended}</em>}</div>
        {option.summary && <span>{localizeAutonomousApprovalValue(option.summary, lang, 'reviewOption')}</span>}
      </li>)}</ol>
      {options.length > visibleOptions.length && <small>{copy.reviewMoreOptions(options.length - visibleOptions.length)}</small>}
    </div>}
  </section>
}

function ApprovalCard({ item, lang, busy, selected, reply, onReply, onSelect, onApprove, onReject, onOpenReport }) {
  const copy = autonomousCopy(lang)
  const pending = item.state === 'pending'
  const review = autonomousReviewView(item, lang)
  const execution = executionPresentation(item, copy)
  const showExecution = !pending && (item.decision === 'approved' || execution.state)
  return <article className={`autonomous-approval is-${item.state}${selected ? ' is-selected' : ''}`}>
    <header>
      <div className="autonomous-approval-heading">
        {pending && <label className="autonomous-approval-select">
          <input type="checkbox" checked={selected} aria-label={`${copy.selectItem}：${item.title}`} onChange={event => onSelect?.(item.id, event.target.checked)}/>
        </label>}
        <div><b>{localizeAutonomousApprovalValue(item.title, lang, 'title')}</b><span>{localizeAutonomousApprovalValue(item.status, lang, 'status') || (pending ? copy.pending : copy.handled)}</span></div>
      </div>
      <em>{pending ? copy.pending : item.state === 'approved' ? (lang === 'en' ? 'Approved' : '已批准') : item.state === 'rejected' ? (lang === 'en' ? 'Rejected' : '已拒绝') : (lang === 'en' ? 'Archived' : '已归档')}</em>
    </header>
    {review.hasReviewData && <section className={`autonomous-approval-review is-${review.kind}`} aria-label={copy.reviewMethod}>
      <div className="autonomous-approval-review-head"><strong>{copy.reviewMethod}</strong><em>{review.badge}</em></div>
      <details className="autonomous-approval-review-details"><summary>{zhDetailLabel(lang, 'AI 审核详情', 'AI review details')}</summary>
        <p><span>{review.method}</span></p>
        <p>{review.summary}</p>
        {review.basis.length > 0 && <p className="autonomous-approval-review-basis"><b>{copy.reviewBasis}：</b>{review.basis.join('；')}</p>}
        {(review.model || review.decision || review.confidence) && <div className="autonomous-approval-review-meta">
          {review.model && <span><b>{copy.reviewModel}</b>{review.model}</span>}
          {review.decision && <span><b>{copy.reviewDecision}</b>{review.decision}</span>}
          {review.confidence && <span><b>{copy.reviewConfidence}</b>{review.confidence}</span>}
        </div>}
      </details>
    </section>}
    <section className="autonomous-approval-problem">
      <strong>{copy.problem}</strong>
      <p>{summarizeAutonomousProblem(item, lang)}</p>
    </section>
    <ApprovalReviewContext item={item} lang={lang} copy={copy}/>
    <details className="autonomous-approval-extra"><summary>{zhDetailLabel(lang, '详情', 'Details')}</summary><dl>{approvalDetails(item, copy, lang).map(([label, value]) => <div key={label}><dt>{label}</dt><dd>{value}</dd></div>)}</dl></details>
    {!pending && (item.decided_at || item.note || showExecution) && <details className="autonomous-handled-extra"><summary>{item.decided_at ? readableAutonomousDate(item.decided_at, lang) : zhDetailLabel(lang, '处理详情', 'Handled details')}{showExecution ? ` · ${execution.label}` : ''}</summary>
      {(item.decided_at || item.note) && <div className="autonomous-decision-meta">
        {item.decided_at && <span>{copy.decidedAt}：{readableAutonomousDate(item.decided_at, lang)}</span>}
        {item.note && <span>{copy.note}：{localizeAutonomousApprovalValue(item.note, lang, 'note')}</span>}
      </div>}
      {showExecution && <div className={`autonomous-execution is-${execution.state || 'unknown'}`}>
        <div className="autonomous-execution-head"><span>{copy.execution}</span><b>{execution.label}</b></div>
        {item.execution_summary && <p><strong>{copy.executionSummary}：</strong>{localizeAutonomousApprovalValue(item.execution_summary, lang, 'executionSummary')}</p>}
        {item.execution_error && <p className="autonomous-execution-error">{localizeAutonomousApprovalValue(item.execution_error, lang, 'executionError')}</p>}
        {item.execution_report && <button type="button" className="autonomous-execution-link" aria-label={`${copy.openExecutionReport}：${item.execution_report.name || item.execution_report.path}`} onClick={() => onOpenReport?.(item.execution_report)}>{copy.openExecutionReport}：{item.execution_report.name || item.execution_report.path}</button>}
      </div>}
    </details>}
    {item.review_reports?.length > 0 && <div className="autonomous-review-reports"><span>{copy.reviewReports}</span><div>{item.review_reports.map(report => <button type="button" key={report.path} onClick={() => onOpenReport?.(report)}>{copy.openReviewReport}：{report.name || report.path}</button>)}</div></div>}
    {pending && <div className="autonomous-approval-reply"><label><span>{copy.reply}</span><textarea aria-label={copy.reply} maxLength={1000} rows={3} value={reply} onChange={event => onReply(event.target.value)}/></label><small>{copy.replyHelp}</small></div>}
    {pending && <footer>
      <button type="button" className="primary" disabled={busy} onClick={() => onApprove(item, reply)}><Check size={15}/>{copy.approve}</button>
      <button type="button" className="secondary" disabled={busy} onClick={() => onReject(item, reply)}><X size={15}/>{copy.reject}</button>
    </footer>}
  </article>
}

function BulkProgressPanel({ progress, copy, lang, onRetryFailed, onClear }) {
  if (!progress) return null
  const percent = progress.total ? Math.round((progress.completed / progress.total) * 100) : 0
  const failedItems = progress.results.filter(result => result.status === 'failed')
  return <section className={`autonomous-bulk-progress is-${progress.running ? 'running' : 'complete'}${failedItems.length ? ' has-failures' : ''}`} role="status" aria-live="polite" aria-label={copy.bulkProgressTitle}>
    <header className="autonomous-bulk-progress-head">
      <div><b>{copy.bulkProgressTitle}</b><span>{progress.running ? copy.bulkProgressWorking : copy.bulkProgressFinished}</span></div>
      {!progress.running && <button type="button" className="autonomous-bulk-progress-close" aria-label={copy.closeProgress} onClick={onClear}><X size={16}/></button>}
    </header>
    <div className="autonomous-bulk-progress-summary"><strong>{copy.bulkProgressCount(progress.completed, progress.total)}</strong><span>{copy.bulkProgressPercent(percent)}</span></div>
    <progress max={progress.total} value={progress.completed} aria-label={copy.bulkProgressCount(progress.completed, progress.total)}/>
    <div className="autonomous-bulk-progress-stats">
      <span className="is-success"><CheckCircle2 size={14}/>{copy.bulkProgressSuccess(progress.successCount)}</span>
      <span className="is-failed"><AlertCircle size={14}/>{copy.bulkProgressFailed(progress.failedCount)}</span>
    </div>
    {progress.currentTitle && <p className="autonomous-bulk-progress-current"><span>{progress.running ? copy.bulkProgressCurrent : copy.bulkProgressLast}</span><b>{localizeAutonomousApprovalValue(progress.currentTitle, lang, 'title')}</b></p>}
    {progress.results.length > 0 && <div className="autonomous-bulk-progress-details">
      <div className="autonomous-bulk-progress-details-head"><b>{copy.bulkProgressDetails}</b><span>{progress.results.length}</span></div>
      <ul>{progress.results.filter(result => result.status !== 'success').map(result => <li className={`is-${result.status}`} key={`${result.id}-${result.attempt}`}>
        <AlertCircle size={15}/><div><b>{localizeAutonomousApprovalValue(result.title, lang, 'title')}</b><span>{`${copy.bulkProgressError}：${result.error}`}</span></div>
      </li>)}</ul>
      {progress.successCount > 0 && <details className="autonomous-bulk-progress-successes"><summary>{lang === 'en' ? `Show successes (${progress.successCount})` : `查看成功 ${progress.successCount} 项`}</summary><ul>{progress.results.filter(result => result.status === 'success').map(result => <li className="is-success" key={`${result.id}-${result.attempt}`}>
        <CheckCircle2 size={15}/><div><b>{localizeAutonomousApprovalValue(result.title, lang, 'title')}</b><span>{result.queued ? copy.bulkProgressQueued : copy.bulkProgressRecorded}</span></div>
      </li>)}</ul></details>}
    </div>}
    {!progress.running && failedItems.length > 0 && <button type="button" className="secondary autonomous-bulk-progress-retry" onClick={() => onRetryFailed?.(failedItems)}><RefreshCw size={15}/>{copy.retryFailed(failedItems.length)}</button>}
  </section>
}

function ApprovalCardList({ items, lang, busyIDs, selectedIDs, replies, onReply, onSelect, onApprove, onReject, onOpenReport }) {
  return <div className="autonomous-approval-list">{items.map(item => <ApprovalCard key={item.id} item={item} lang={lang} busy={busyIDs.has(item.id)} selected={selectedIDs.includes(item.id)} reply={replies[item.id] || ''} onReply={value => onReply(item.id, value)} onSelect={onSelect} onApprove={onApprove} onReject={onReject} onOpenReport={onOpenReport}/>)}</div>
}

function HandledApprovalGroups({ groups, copy, cardProps }) {
  const [expandedKeys, setExpandedKeys] = useState(() => groups[0]?.key ? [groups[0].key] : [])
  useEffect(() => {
    setExpandedKeys(current => {
      const availableKeys = new Set(groups.map(group => group.key))
      const preservedKeys = current.filter(key => availableKeys.has(key))
      return preservedKeys.length || groups.length === 0 ? preservedKeys : [groups[0].key]
    })
  }, [groups])
  return <section className="autonomous-approval-groups" aria-label={copy.handledGroupsHelp}>
    <p className="autonomous-approval-groups-help">{copy.handledGroupsHelp}</p>
    <div className="autonomous-approval-group-list">
      {groups.map(group => {
        const detail = copy.handledGroups[group.key] || copy.handledGroups.archived
        const expanded = expandedKeys.includes(group.key)
        return <details key={group.key} className={`autonomous-approval-group${expanded ? ' is-expanded' : ''}`} open={expanded} onToggle={event => {
          const isOpen = event.currentTarget?.open
          setExpandedKeys(current => isOpen ? [...new Set([...current, group.key])] : current.filter(key => key !== group.key))
        }}>
          <summary>
            <span className="autonomous-approval-group-heading"><ChevronDown size={16}/><span><b title={detail.description}>{detail.label}</b></span></span>
            <strong>{group.items.length}</strong>
          </summary>
          <ApprovalCardList items={group.items} {...cardProps}/>
        </details>
      })}
    </div>
  </section>
}

function ApprovalPane({ overview, lang, busyIDs, reviewBusy, bulkProgress, onReview, onApprove, onReject, onApproveMany, onRejectMany, onRetryFailed, onClearBulkProgress, onOpenReport }) {
  const copy = autonomousCopy(lang)
  const [mode, setMode] = useState('pending')
  const [replies, setReplies] = useState({})
  const [selectedIDs, setSelectedIDs] = useState([])
  const groups = useMemo(() => splitAutonomousApprovals(overview?.items || []), [overview?.items])
  const items = groups[mode]
  const selectedItems = groups.pending.filter(item => selectedIDs.includes(item.id))
  const allSelected = groups.pending.length > 0 && selectedItems.length === groups.pending.length
  useEffect(() => {
    const pendingIDs = new Set(groups.pending.map(item => item.id))
    setSelectedIDs(current => current.filter(id => pendingIDs.has(id)))
  }, [groups.pending])
  const toggleSelected = (id, checked) => setSelectedIDs(current => checked ? [...new Set([...current, id])] : current.filter(itemID => itemID !== id))
  const toggleAll = () => setSelectedIDs(allSelected ? [] : groups.pending.map(item => item.id))
  const cardProps = {
    lang,
    busyIDs,
    selectedIDs,
    replies,
    onReply: (id, value) => setReplies(current => ({ ...current, [id]: value })),
    onSelect: toggleSelected,
    onApprove,
    onReject,
    onOpenReport,
  }
  return <div className="autonomous-approvals-pane">
    <details className="autonomous-callout"><summary><b>{lang === 'en' ? 'Approval boundary' : '审批边界'}</b></summary><span>{copy.approvalIntro}</span></details>
    <div className="autonomous-filter-tabs" role="tablist" aria-label={copy.approvals}>
      <button type="button" role="tab" aria-selected={mode === 'pending'} className={mode === 'pending' ? 'active' : ''} onClick={() => setMode('pending')}>{copy.pending}<span>{groups.pending.length}</span></button>
      <button type="button" role="tab" aria-selected={mode === 'handled'} className={mode === 'handled' ? 'active' : ''} onClick={() => setMode('handled')}>{copy.handled}<span>{groups.handled.length}</span></button>
    </div>
    {mode === 'pending' && Boolean(groups.pending.length) && <div className="autonomous-approval-toolbar">
      <div className="autonomous-approval-selection">
        <button type="button" className="secondary" aria-pressed={allSelected} disabled={busyIDs.size > 0} onClick={toggleAll}>{allSelected ? copy.clearSelection : copy.selectAll}</button>
        <span>{copy.selectedCount(selectedItems.length)}</span>
      </div>
      <button type="button" className="secondary" disabled={reviewBusy || busyIDs.size > 0} onClick={onReview}><RefreshCw size={15} className={reviewBusy ? 'spin' : ''}/>{reviewBusy ? copy.reviewStarted : copy.reviewNow}</button>
      {selectedItems.length > 0 && <div className="autonomous-approval-bulk-actions">
        <button type="button" className="primary" disabled={busyIDs.size > 0} onClick={() => onApproveMany?.(selectedItems.map(item => ({ item, note: replies[item.id] || '' })))}><Check size={15}/>{copy.approveMany}</button>
        <button type="button" className="secondary" disabled={busyIDs.size > 0} onClick={() => onRejectMany?.(selectedItems)}><X size={15}/>{copy.rejectMany}</button>
      </div>}
    </div>}
    <BulkProgressPanel progress={bulkProgress} copy={copy} lang={lang} onRetryFailed={onRetryFailed} onClear={onClearBulkProgress}/>
    {!overview?.source_exists && <div className="autonomous-empty">{copy.noLedger}</div>}
    {overview?.source_exists && !items.length && <div className="autonomous-empty">{mode === 'pending' ? copy.noPending : copy.noHandled}</div>}
    {mode === 'handled' ? <HandledApprovalGroups groups={groups.handledGroups} copy={copy} cardProps={cardProps}/> : <ApprovalCardList items={items} {...cardProps}/>}
  </div>
}

function ReportPane({ reports, lang, initialReport }) {
  const copy = autonomousCopy(lang)
  const [query, setQuery] = useState('')
  const [selected, setSelected] = useState(null)
  const [content, setContent] = useState('')
  const [status, setStatus] = useState('idle')
  const [latest, setLatest] = useState({ status: 'idle', summary: '' })
  const visibleReports = useMemo(() => filterAutonomousReports(reports, query), [reports, query])
  const latestReport = useMemo(() => latestAutonomousReport(reports), [reports])
  const openReport = useCallback(async report => {
    setSelected(report); setStatus('loading'); setContent('')
    try {
      const result = await api(`/api/files/read?path=${encodeURIComponent(report.path)}`)
      setContent(result.content || ''); setStatus('ready')
    } catch (error) { setContent(error.message); setStatus('error') }
  }, [])
  useEffect(() => {
    if (initialReport?.path) openReport(initialReport)
  }, [initialReport, openReport])
  useEffect(() => {
    let active = true
    if (!latestReport) { setLatest({ status: 'empty', summary: '' }); return () => { active = false } }
    setLatest({ status: 'loading', summary: '' })
    api(`/api/files/read?path=${encodeURIComponent(latestReport.path)}`).then(result => {
      if (active) setLatest({ status: 'ready', summary: summarizeAutonomousReport(result.content) })
    }).catch(error => { if (active) setLatest({ status: 'error', summary: error.message }) })
    return () => { active = false }
  }, [latestReport])
  return <div className="autonomous-records-pane">
    {latestReport && <details className="autonomous-latest-result" aria-label={copy.latestResult}><summary><div><span>{copy.latestResult}</span><b>{latestReport.name}</b></div><em>{copy.reportReady}</em></summary><p>{latest.status === 'loading' ? `${copy.loading}…` : latest.status === 'error' ? `${copy.loadFailed}：${latest.summary}` : latest.summary || copy.noResult}</p><button type="button" onClick={() => openReport(latestReport)}>{copy.openReport}</button></details>}
    <div className={`autonomous-report-reader ${selected ? 'has-selection' : ''}`}>
    <aside className="autonomous-report-index">
      <label className="autonomous-report-search"><Search size={16}/><input value={query} onChange={event => setQuery(event.target.value)} placeholder={copy.reportSearch} aria-label={copy.reportSearch}/></label>
      <span className="autonomous-report-count">{copy.reportCount(visibleReports.length)}</span>
      <div className="autonomous-report-list">{visibleReports.map(report => <button type="button" key={report.path} className={selected?.path === report.path ? 'active' : ''} onClick={() => openReport(report)}>
        <b>{report.name}</b><span>{readableAutonomousDate(report.mod_time, lang)}</span>
      </button>)}</div>
      {!visibleReports.length && <div className="autonomous-empty">{copy.noReports}</div>}
    </aside>
    <article className="autonomous-report-content">
      {selected && <header><button type="button" className="autonomous-report-back" onClick={() => setSelected(null)}><ArrowLeft size={16}/>{copy.backToReports}</button><div><b>{selected.name}</b><span>{readableAutonomousDate(selected.mod_time, lang)}</span></div><a href={`/api/files/download?path=${encodeURIComponent(selected.path)}`} target="_blank" rel="noreferrer"><Download size={15}/>{copy.download}</a></header>}
      {!selected && <div className="autonomous-report-placeholder">{copy.selectReport}</div>}
      {selected && status === 'loading' && <div className="autonomous-report-placeholder">{copy.loading}…</div>}
      {selected && status === 'error' && <div className="autonomous-report-error" role="alert">{copy.loadFailed}：{content}</div>}
      {selected && status === 'ready' && <div className="autonomous-markdown"><ReactMarkdown remarkPlugins={[remarkGfm]} skipHtml>{content}</ReactMarkdown></div>}
    </article>
    </div>
  </div>
}

export function AutonomousPage({ lang = 'zh', services = [], llms = [], actionStates = {}, reports = [], onStart, onStop, onLogs, onAutostart, onModel, onRefresh, setMessage }) {
  const copy = autonomousCopy(lang)
  const [approvals, setApprovals] = useState({ items: [], pending: 0, source_exists: false })
  const [loading, setLoading] = useState(true)
  const [busyIDs, setBusyIDs] = useState(new Set())
  const [reviewBusy, setReviewBusy] = useState(false)
  const [bulkProgress, setBulkProgress] = useState(null)
  const [reportToOpen, setReportToOpen] = useState(null)
  const [rejecting, setRejecting] = useState(null)
  const [rejectNote, setRejectNote] = useState('')
  const loadApprovals = useCallback(async () => {
    setLoading(true)
    try { setApprovals(await api('/api/autonomous/approvals')) }
    catch (error) { setMessage?.(`${copy.approvalFailed}：${error.message}`, 'error') }
    finally { setLoading(false) }
  }, [copy.approvalFailed, setMessage])
  useEffect(() => { loadApprovals() }, [loadApprovals])
  const summary = autonomousSummary({ services, approvals, reports })
  const refresh = async () => { await Promise.all([loadApprovals(), onRefresh?.()]) }
  const reviewPending = async () => {
    if (!confirmDanger('autonomous-review', copy.reviewConfirm)) return
    setReviewBusy(true)
    setMessage?.(copy.reviewStarted, 'pending')
    try {
      const result = await api('/api/autonomous/approvals/review', { dangerous: true, method: 'POST', body: JSON.stringify({}) })
      setApprovals(result.overview || approvals)
      setMessage?.(copy.reviewCompleted(Number(result.reviewed) || 0), 'success')
    } catch (error) {
      setMessage?.(`${copy.reviewFailed}：${error.message}`, 'error')
    } finally { setReviewBusy(false) }
  }
  const decideMany = async (entries, decision, sharedNote = '', options = {}) => {
    const normalizedEntries = entries.filter(entry => entry?.item?.id).map(entry => ({ ...entry, note: entry.note ?? sharedNote }))
    if (!normalizedEntries.length) return
    const showProgress = options.forceProgress || normalizedEntries.length > 1
    const action = decision === 'approved' ? copy.approve : copy.confirmReject
    const confirmText = normalizedEntries.length > 1
      ? (decision === 'approved' ? copy.approveManyConfirm(normalizedEntries.length) : copy.rejectManyConfirm(normalizedEntries.length))
      : (options.confirmText || `${action}“${normalizedEntries[0].item.title}”？`)
    if (!confirmDanger('autonomous-approval', confirmText)) return
    setBusyIDs(new Set(normalizedEntries.map(entry => entry.item.id)))
    setBulkProgress(showProgress ? { running: true, decision, total: normalizedEntries.length, completed: 0, successCount: 0, failedCount: 0, currentTitle: normalizedEntries[0].item.title, results: [] } : null)
    let latestOverview = approvals
    let completed = 0
    let failed = 0
    let queued = 0
    let lastError = ''
    try {
      for (const entry of normalizedEntries) {
        if (showProgress) setBulkProgress(current => current ? { ...current, currentTitle: entry.item.title } : current)
        try {
          const result = await api('/api/autonomous/approvals', { dangerous: true, method: 'POST', body: JSON.stringify({ id: entry.item.id, decision, note: entry.note }) })
          latestOverview = result.overview || latestOverview
          completed += 1
          if (result.queued) queued += 1
          setApprovals(latestOverview)
          if (showProgress) setBulkProgress(current => current ? { ...current, completed: current.completed + 1, successCount: current.successCount + 1, results: [...current.results, { id: entry.item.id, title: entry.item.title, status: 'success', queued: Boolean(result.queued), entry, attempt: current.results.length + 1 }] } : current)
        } catch (error) {
          failed += 1
          lastError = error.message
          if (showProgress) setBulkProgress(current => current ? { ...current, completed: current.completed + 1, failedCount: current.failedCount + 1, results: [...current.results, { id: entry.item.id, title: entry.item.title, status: 'failed', error: error.message, entry, attempt: current.results.length + 1 }] } : current)
        }
      }
      setApprovals(latestOverview)
      if (completed === normalizedEntries.length) {
        setRejecting(null); setRejectNote('')
        const message = normalizedEntries.length === 1
          ? (queued ? copy.approvalQueued : copy.approvalRecorded)
          : copy.bulkSuccess(completed)
        if (!showProgress) setMessage?.(message, 'success')
      } else {
        if (!showProgress) setMessage?.(copy.bulkPartial(completed, normalizedEntries.length, lastError || copy.approvalFailed), 'error')
      }
    } finally {
      setBusyIDs(new Set())
      if (showProgress) setBulkProgress(current => current ? { ...current, running: false, completed: completed + failed } : current)
    }
  }
  const retryBulkFailures = failedItems => {
    const entries = failedItems.map(result => result.entry).filter(Boolean)
    if (!entries.length || !bulkProgress) return
    decideMany(entries, bulkProgress.decision, '', { forceProgress: true, confirmText: copy.retryFailedConfirm(entries.length) })
  }
  const [tab, setTab] = useState('overview')
  const tabs = [['overview', lang === 'en' ? 'Tasks overview' : '任务总览'], ['approvals', `${copy.approvals}${summary.pending ? ` (${summary.pending})` : ''}`], ['records', copy.records]]
  return <section className="autonomous-page">
    <div className="autonomous-toolbar"><div className="autonomous-tabs" role="tablist">{tabs.map(([id, label]) => <button type="button" role="tab" aria-selected={tab === id} className={tab === id ? 'active' : ''} key={id} onClick={() => setTab(id)}>{label}</button>)}</div><button type="button" className="autonomous-refresh" disabled={loading || bulkProgress?.running} onClick={refresh}><RefreshCw className={loading || bulkProgress?.running ? 'spin' : ''} size={16}/>{copy.refresh}</button></div>
    {tab === 'overview' && <div className="autonomous-overview-pane"><div className="autonomous-service-strip">{services.length ? services.map(service => <AutonomousServiceCard key={service.name} service={service} lang={lang} llms={llms} actionState={actionStates[service.name]} onStart={onStart} onStop={onStop} onLogs={onLogs} onAutostart={onAutostart} onModel={onModel} compact/>) : <div className="autonomous-empty">{lang === 'en' ? 'No autonomous service was found' : '未发现自主进化服务'}</div>}</div><AutonomousTaskWorkspace lang={lang} /></div>}
    {tab === 'approvals' && <ApprovalPane
      overview={approvals}
      lang={lang}
      busyIDs={busyIDs}
      reviewBusy={reviewBusy}
      bulkProgress={bulkProgress}
      onReview={reviewPending}
      onApprove={(item, note) => decideMany([{ item, note }], 'approved')}
      onReject={(item, note) => { setRejecting({ items: [item] }); setRejectNote(note) }}
      onApproveMany={entries => decideMany(entries, 'approved')}
      onRejectMany={items => { setRejecting({ items }); setRejectNote('') }}
      onRetryFailed={retryBulkFailures}
      onClearBulkProgress={() => setBulkProgress(null)}
      onOpenReport={report => { setReportToOpen({ ...report, open_token: Date.now() }); setTab('records') }}
    />}
    {tab === 'records' && <ReportPane reports={reports} lang={lang} initialReport={reportToOpen}/>}
    {rejecting && <div className="autonomous-dialog-backdrop" role="presentation" onMouseDown={event => { if (event.target === event.currentTarget) setRejecting(null) }}><div className="autonomous-dialog" role="dialog" aria-modal="true" aria-labelledby="autonomous-reject-title"><header><b id="autonomous-reject-title">{rejecting.items.length === 1 ? `${copy.reject}：${rejecting.items[0].title}` : copy.rejectManyTitle(rejecting.items.length)}</b><button type="button" aria-label={copy.cancel} onClick={() => setRejecting(null)}><X size={18}/></button></header><label>{copy.rejectNote}<textarea maxLength={1000} value={rejectNote} onChange={event => setRejectNote(event.target.value)}/></label><footer><button type="button" className="secondary" onClick={() => setRejecting(null)}>{copy.cancel}</button><button type="button" className="danger" disabled={busyIDs.size > 0} onClick={() => decideMany(rejecting.items.map(item => ({ item, note: rejectNote })), 'rejected')}>{copy.confirmReject}</button></footer></div></div>}
  </section>
}

export default AutonomousPage
