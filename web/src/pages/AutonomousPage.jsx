import React, { useCallback, useEffect, useMemo, useState } from 'react'
import ReactMarkdown from 'react-markdown'
import remarkGfm from 'remark-gfm'
import { ArrowLeft, Check, Download, RefreshCw, Search, X } from 'lucide-react'
import { AutonomousServiceCard } from '../components/AutonomousServiceCard.jsx'
import { api } from '../lib/api.js'
import { confirmDanger } from '../lib/danger.js'
import { autonomousCopy } from '../lib/autonomousCopy.js'
import { autonomousReviewView, autonomousSummary, filterAutonomousReports, latestAutonomousReport, readableAutonomousDate, splitAutonomousApprovals, summarizeAutonomousApproval, summarizeAutonomousReport, summarizeAutonomousReviewNeed } from '../lib/autonomous.js'

const approvalDetails = (item, copy) => [
  [copy.source, item.source || item.draft_path],
  [copy.target, item.target],
  [copy.risk, item.risk],
  [copy.evidence, item.evidence],
  [copy.nextStep, item.next_step],
].filter(([, value]) => value)

function ApprovalCard({ item, lang, busy, selected, reply, onReply, onSelect, onApprove, onReject }) {
  const copy = autonomousCopy(lang)
  const pending = item.state === 'pending'
  const review = autonomousReviewView(item, lang)
  return <article className={`autonomous-approval is-${item.state}${selected ? ' is-selected' : ''}`}>
    <header>
      <div className="autonomous-approval-heading">
        {pending && <label className="autonomous-approval-select">
          <input type="checkbox" checked={selected} aria-label={`${copy.selectItem}：${item.title}`} onChange={event => onSelect?.(item.id, event.target.checked)}/>
        </label>}
        <div><b>{item.title}</b><span>{item.status || (pending ? copy.pending : copy.handled)}</span></div>
      </div>
      <em>{pending ? copy.pending : item.state === 'approved' ? (lang === 'en' ? 'Approved' : '已批准') : item.state === 'rejected' ? (lang === 'en' ? 'Rejected' : '已拒绝') : (lang === 'en' ? 'Archived' : '已归档')}</em>
    </header>
    {review.hasReviewData && <section className={`autonomous-approval-review is-${review.kind}`} aria-label={copy.reviewMethod}>
      <div className="autonomous-approval-review-head"><strong>{copy.reviewMethod}</strong><span>{review.method}</span><em>{review.badge}</em></div>
      <p>{review.summary}</p>
      {review.basis.length > 0 && <p className="autonomous-approval-review-basis"><b>{copy.reviewBasis}：</b>{review.basis.join('；')}</p>}
      {(review.model || review.decision || review.confidence) && <div className="autonomous-approval-review-meta">
        {review.model && <span><b>{copy.reviewModel}</b>{review.model}</span>}
        {review.decision && <span><b>{copy.reviewDecision}</b>{review.decision}</span>}
        {review.confidence && <span><b>{copy.reviewConfidence}</b>{review.confidence}</span>}
      </div>}
    </section>}
    <section className="autonomous-approval-reason">
      <strong>{copy.reviewWhy}</strong>
      <p>{summarizeAutonomousReviewNeed(item, review, lang)}</p>
    </section>
    <section className="autonomous-approval-outcome">
      <strong>{copy.expectedOutcome}</strong>
      <p>{summarizeAutonomousApproval(item, lang)}</p>
    </section>
    <dl>{approvalDetails(item, copy).map(([label, value]) => <div key={label}><dt>{label}</dt><dd>{value}</dd></div>)}</dl>
    {!pending && (item.decided_at || item.note) && <div className="autonomous-decision-meta">
      {item.decided_at && <span>{copy.decidedAt}：{readableAutonomousDate(item.decided_at, lang)}</span>}
      {item.note && <span>{copy.note}：{item.note}</span>}
    </div>}
    {pending && <div className="autonomous-approval-reply"><label><span>{copy.reply}</span><textarea aria-label={copy.reply} maxLength={1000} rows={3} value={reply} onChange={event => onReply(event.target.value)}/></label><small>{copy.replyHelp}</small></div>}
    {pending && <footer>
      <button type="button" className="primary" disabled={busy} onClick={() => onApprove(item, reply)}><Check size={15}/>{copy.approve}</button>
      <button type="button" className="secondary" disabled={busy} onClick={() => onReject(item, reply)}><X size={15}/>{copy.reject}</button>
    </footer>}
  </article>
}

function ApprovalPane({ overview, lang, busyIDs, reviewBusy, onReview, onApprove, onReject, onApproveMany, onRejectMany }) {
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
  return <div className="autonomous-approvals-pane">
    <div className="autonomous-callout"><b>{lang === 'en' ? 'Approval boundary' : '审批边界'}</b><span>{copy.approvalIntro}</span></div>
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
    {!overview?.source_exists && <div className="autonomous-empty">{copy.noLedger}</div>}
    {overview?.source_exists && !items.length && <div className="autonomous-empty">{mode === 'pending' ? copy.noPending : copy.noHandled}</div>}
    <div className="autonomous-approval-list">{items.map(item => <ApprovalCard key={item.id} item={item} lang={lang} busy={busyIDs.has(item.id)} selected={selectedIDs.includes(item.id)} reply={replies[item.id] || ''} onReply={value => setReplies(current => ({ ...current, [item.id]: value }))} onSelect={toggleSelected} onApprove={onApprove} onReject={onReject}/>)}</div>
  </div>
}

function ReportPane({ reports, lang }) {
  const copy = autonomousCopy(lang)
  const [query, setQuery] = useState('')
  const [selected, setSelected] = useState(null)
  const [content, setContent] = useState('')
  const [status, setStatus] = useState('idle')
  const [latest, setLatest] = useState({ status: 'idle', summary: '' })
  const visibleReports = useMemo(() => filterAutonomousReports(reports, query), [reports, query])
  const latestReport = useMemo(() => latestAutonomousReport(reports), [reports])
  const openReport = async report => {
    setSelected(report); setStatus('loading'); setContent('')
    try {
      const result = await api(`/api/files/read?path=${encodeURIComponent(report.path)}`)
      setContent(result.content || ''); setStatus('ready')
    } catch (error) { setContent(error.message); setStatus('error') }
  }
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
    {latestReport && <section className="autonomous-latest-result" aria-label={copy.latestResult}><header><div><span>{copy.latestResult}</span><b>{latestReport.name}</b></div><em>{copy.reportReady}</em></header><p>{latest.status === 'loading' ? `${copy.loading}…` : latest.status === 'error' ? `${copy.loadFailed}：${latest.summary}` : latest.summary || copy.noResult}</p><button type="button" onClick={() => openReport(latestReport)}>{copy.openReport}</button></section>}
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
  const [tab, setTab] = useState('services')
  const [approvals, setApprovals] = useState({ items: [], pending: 0, source_exists: false })
  const [loading, setLoading] = useState(true)
  const [busyIDs, setBusyIDs] = useState(new Set())
  const [reviewBusy, setReviewBusy] = useState(false)
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
  const decideMany = async (entries, decision, sharedNote = '') => {
    const normalizedEntries = entries.filter(entry => entry?.item?.id).map(entry => ({ ...entry, note: entry.note ?? sharedNote }))
    if (!normalizedEntries.length) return
    const action = decision === 'approved' ? copy.approve : copy.confirmReject
    const confirmText = normalizedEntries.length > 1
      ? (decision === 'approved' ? copy.approveManyConfirm(normalizedEntries.length) : copy.rejectManyConfirm(normalizedEntries.length))
      : `${action}“${normalizedEntries[0].item.title}”？`
    if (!confirmDanger('autonomous-approval', confirmText)) return
    setBusyIDs(new Set(normalizedEntries.map(entry => entry.item.id)))
    if (normalizedEntries.length > 1) setMessage?.(copy.bulkProcessing(normalizedEntries.length), 'pending')
    let latestOverview = approvals
    let completed = 0
    let queued = 0
    let lastError = ''
    try {
      for (const entry of normalizedEntries) {
        try {
          const result = await api('/api/autonomous/approvals', { dangerous: true, method: 'POST', body: JSON.stringify({ id: entry.item.id, decision, note: entry.note }) })
          latestOverview = result.overview || latestOverview
          completed += 1
          if (result.queued) queued += 1
        } catch (error) { lastError = error.message }
      }
      setApprovals(latestOverview)
      if (completed === normalizedEntries.length) {
        setRejecting(null); setRejectNote('')
        const message = normalizedEntries.length === 1
          ? (queued ? copy.approvalQueued : copy.approvalRecorded)
          : copy.bulkSuccess(completed)
        setMessage?.(message, 'success')
      } else {
        setMessage?.(copy.bulkPartial(completed, normalizedEntries.length, lastError || copy.approvalFailed), 'error')
      }
    } finally { setBusyIDs(new Set()) }
  }
  const tabs = [['services', copy.services], ['approvals', `${copy.approvals}${summary.pending ? ` (${summary.pending})` : ''}`], ['records', copy.records]]
  return <section className="autonomous-page">
    <div className="autonomous-overview">
      <div><span>{copy.serviceStat}</span><b>{summary.running}<small> / {summary.total}</small></b></div>
      <div><span>{copy.pendingStat}</span><b>{summary.pending}</b></div>
      <div><span>{copy.reportStat}</span><b>{summary.reports}</b></div>
      <div><span>{copy.latestStat}</span><b className="is-date">{summary.latestReport ? readableAutonomousDate(summary.latestReport.mod_time, lang) : copy.noRecent}</b></div>
    </div>
    <div className="autonomous-toolbar"><div className="autonomous-tabs" role="tablist">{tabs.map(([id, label]) => <button type="button" role="tab" aria-selected={tab === id} className={tab === id ? 'active' : ''} key={id} onClick={() => setTab(id)}>{label}</button>)}</div><button type="button" className="autonomous-refresh" disabled={loading} onClick={refresh}><RefreshCw className={loading ? 'spin' : ''} size={16}/>{copy.refresh}</button></div>
    {tab === 'services' && <div className="autonomous-services-pane"><div className="autonomous-callout"><b>{lang === 'en' ? 'How services work' : '服务说明'}</b><span>{copy.serviceIntro}</span></div><div className="autonomous-service-grid">{services.length ? services.map(service => <AutonomousServiceCard key={service.name} service={service} lang={lang} llms={llms} actionState={actionStates[service.name]} onStart={onStart} onStop={onStop} onLogs={onLogs} onAutostart={onAutostart} onModel={onModel}/>) : <div className="autonomous-empty">{lang === 'en' ? 'No autonomous service was found' : '未发现自主进化服务'}</div>}</div></div>}
    {tab === 'approvals' && <ApprovalPane
      overview={approvals}
      lang={lang}
      busyIDs={busyIDs}
      reviewBusy={reviewBusy}
      onReview={reviewPending}
      onApprove={(item, note) => decideMany([{ item, note }], 'approved')}
      onReject={(item, note) => { setRejecting({ items: [item] }); setRejectNote(note) }}
      onApproveMany={entries => decideMany(entries, 'approved')}
      onRejectMany={items => { setRejecting({ items }); setRejectNote('') }}
    />}
    {tab === 'records' && <ReportPane reports={reports} lang={lang}/>}
    {rejecting && <div className="autonomous-dialog-backdrop" role="presentation" onMouseDown={event => { if (event.target === event.currentTarget) setRejecting(null) }}><div className="autonomous-dialog" role="dialog" aria-modal="true" aria-labelledby="autonomous-reject-title"><header><b id="autonomous-reject-title">{rejecting.items.length === 1 ? `${copy.reject}：${rejecting.items[0].title}` : copy.rejectManyTitle(rejecting.items.length)}</b><button type="button" aria-label={copy.cancel} onClick={() => setRejecting(null)}><X size={18}/></button></header><label>{copy.rejectNote}<textarea maxLength={1000} value={rejectNote} onChange={event => setRejectNote(event.target.value)}/></label><footer><button type="button" className="secondary" onClick={() => setRejecting(null)}>{copy.cancel}</button><button type="button" className="danger" disabled={busyIDs.size > 0} onClick={() => decideMany(rejecting.items.map(item => ({ item, note: rejectNote })), 'rejected')}>{copy.confirmReject}</button></footer></div></div>}
  </section>
}

export default AutonomousPage
