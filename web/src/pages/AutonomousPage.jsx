import React, { useCallback, useEffect, useMemo, useState } from 'react'
import ReactMarkdown from 'react-markdown'
import remarkGfm from 'remark-gfm'
import { ArrowLeft, Check, Download, RefreshCw, Search, X } from 'lucide-react'
import { AutonomousServiceCard } from '../components/AutonomousServiceCard.jsx'
import { api } from '../lib/api.js'
import { confirmDanger } from '../lib/danger.js'
import { autonomousCopy } from '../lib/autonomousCopy.js'
import { autonomousExecutionState, autonomousSummary, filterAutonomousReports, latestAutonomousReport, readableAutonomousDate, splitAutonomousApprovals, summarizeAutonomousReport } from '../lib/autonomous.js'

const approvalDetails = (item, copy) => [
  [copy.source, item.source || item.draft_path],
  [copy.target, item.target],
  [copy.risk, item.risk],
  [copy.evidence, item.evidence],
  [copy.nextStep, item.next_step],
].filter(([, value]) => value)

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

function ApprovalCard({ item, lang, busy, reply, onReply, onApprove, onReject, onOpenReport }) {
  const copy = autonomousCopy(lang)
  const pending = item.state === 'pending'
  const execution = executionPresentation(item, copy)
  const showExecution = !pending && (item.decision === 'approved' || execution.state)
  return <article className={`autonomous-approval is-${item.state}`}>
    <header>
      <div><b>{item.title}</b><span>{item.status || (pending ? copy.pending : copy.handled)}</span></div>
      <em>{pending ? copy.pending : item.state === 'approved' ? (lang === 'en' ? 'Approved' : '已批准') : item.state === 'rejected' ? (lang === 'en' ? 'Rejected' : '已拒绝') : (lang === 'en' ? 'Archived' : '已归档')}</em>
    </header>
    <dl>{approvalDetails(item, copy).map(([label, value]) => <div key={label}><dt>{label}</dt><dd>{value}</dd></div>)}</dl>
    {!pending && (item.decided_at || item.note) && <div className="autonomous-decision-meta">
      {item.decided_at && <span>{copy.decidedAt}：{readableAutonomousDate(item.decided_at, lang)}</span>}
      {item.note && <span>{copy.note}：{item.note}</span>}
    </div>}
    {showExecution && <div className={`autonomous-execution is-${execution.state || 'unknown'}`}>
      <div className="autonomous-execution-head"><span>{copy.execution}</span><b>{execution.label}</b></div>
      {item.execution_summary && <p><strong>{copy.executionSummary}：</strong>{item.execution_summary}</p>}
      {item.execution_error && <p className="autonomous-execution-error">{item.execution_error}</p>}
      {item.execution_report && <button type="button" className="autonomous-execution-link" onClick={() => onOpenReport?.(item.execution_report)}>{copy.openExecutionReport}：{item.execution_report.name || item.execution_report.path}</button>}
    </div>}
    {pending && <div className="autonomous-approval-reply"><label><span>{copy.reply}</span><textarea aria-label={copy.reply} maxLength={1000} rows={3} value={reply} onChange={event => onReply(event.target.value)}/></label><small>{copy.replyHelp}</small></div>}
    {pending && <footer>
      <button type="button" className="primary" disabled={busy} onClick={() => onApprove(item, reply)}><Check size={15}/>{copy.approve}</button>
      <button type="button" className="secondary" disabled={busy} onClick={() => onReject(item, reply)}><X size={15}/>{copy.reject}</button>
    </footer>}
  </article>
}

function ApprovalPane({ overview, lang, busyID, onApprove, onReject, onOpenReport }) {
  const copy = autonomousCopy(lang)
  const [mode, setMode] = useState('pending')
  const [replies, setReplies] = useState({})
  const groups = splitAutonomousApprovals(overview?.items || [])
  const items = groups[mode]
  return <div className="autonomous-approvals-pane">
    <div className="autonomous-callout"><b>{lang === 'en' ? 'Approval boundary' : '审批边界'}</b><span>{copy.approvalIntro}</span></div>
    <div className="autonomous-filter-tabs" role="tablist" aria-label={copy.approvals}>
      <button type="button" role="tab" aria-selected={mode === 'pending'} className={mode === 'pending' ? 'active' : ''} onClick={() => setMode('pending')}>{copy.pending}<span>{groups.pending.length}</span></button>
      <button type="button" role="tab" aria-selected={mode === 'handled'} className={mode === 'handled' ? 'active' : ''} onClick={() => setMode('handled')}>{copy.handled}<span>{groups.handled.length}</span></button>
    </div>
    {!overview?.source_exists && <div className="autonomous-empty">{copy.noLedger}</div>}
    {overview?.source_exists && !items.length && <div className="autonomous-empty">{mode === 'pending' ? copy.noPending : copy.noHandled}</div>}
    <div className="autonomous-approval-list">{items.map(item => <ApprovalCard key={item.id} item={item} lang={lang} busy={busyID === item.id} reply={replies[item.id] || ''} onReply={value => setReplies(current => ({ ...current, [item.id]: value }))} onApprove={onApprove} onReject={onReject} onOpenReport={onOpenReport}/>)}</div>
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
  const [busyID, setBusyID] = useState('')
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
  const decide = async (item, decision, note = '') => {
    const action = decision === 'approved' ? copy.approve : copy.confirmReject
    if (!confirmDanger('autonomous-approval', `${action}“${item.title}”？`)) return
    setBusyID(item.id)
    try {
      const result = await api('/api/autonomous/approvals', { dangerous: true, method: 'POST', body: JSON.stringify({ id: item.id, decision, note }) })
      setApprovals(result.overview); setRejecting(null); setRejectNote('')
      await onRefresh?.()
      setMessage?.(result.queued ? copy.approvalQueued : copy.approvalRecorded, 'success')
    } catch (error) { setMessage?.(`${copy.approvalFailed}：${error.message}`, 'error') }
    finally { setBusyID('') }
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
    {tab === 'approvals' && <ApprovalPane overview={approvals} lang={lang} busyID={busyID} onApprove={(item, note) => decide(item, 'approved', note)} onReject={(item, note) => { setRejecting(item); setRejectNote(note) }} onOpenReport={report => { setReportToOpen({ ...report, open_token: Date.now() }); setTab('records') }}/>}
    {tab === 'records' && <ReportPane reports={reports} lang={lang} initialReport={reportToOpen}/>}
    {rejecting && <div className="autonomous-dialog-backdrop" role="presentation" onMouseDown={event => { if (event.target === event.currentTarget) setRejecting(null) }}><div className="autonomous-dialog" role="dialog" aria-modal="true" aria-labelledby="autonomous-reject-title"><header><b id="autonomous-reject-title">{copy.reject}：{rejecting.title}</b><button type="button" aria-label={copy.cancel} onClick={() => setRejecting(null)}><X size={18}/></button></header><label>{copy.rejectNote}<textarea maxLength={1000} value={rejectNote} onChange={event => setRejectNote(event.target.value)}/></label><footer><button type="button" className="secondary" onClick={() => setRejecting(null)}>{copy.cancel}</button><button type="button" className="danger" disabled={busyID === rejecting.id} onClick={() => decide(rejecting, 'rejected', rejectNote)}>{copy.confirmReject}</button></footer></div></div>}
  </section>
}

export default AutonomousPage
