import React, { useCallback, useEffect, useMemo, useState } from 'react'
import ReactMarkdown from 'react-markdown'
import remarkGfm from 'remark-gfm'
import { ArrowLeft, Download, RefreshCw, Search } from 'lucide-react'
import { AutonomousServiceCard } from '../components/AutonomousServiceCard.jsx'
import { api } from '../lib/api.js'
import { AutonomousTaskWorkspace } from '../components/AutonomousTaskWorkspace.jsx'
import { autonomousCopy } from '../lib/autonomousCopy.js'
import { filterAutonomousReports, latestAutonomousReport, readableAutonomousDate, summarizeAutonomousReport } from '../lib/autonomous.js'

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
    <p className="autonomous-records-note">{lang === 'en' ? 'Execution reports are records only; the autonomous task list comes exclusively from temp/TODO.txt.' : '这里仅展示执行报告记录；自主任务列表唯一来源是 temp/TODO.txt。'}</p>
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

export function AutonomousPage({ lang = 'zh', services = [], llms = [], actionStates = {}, reports = [], onStart, onStop, onLogs, onAutostart, onModel, onRefresh }) {
  const copy = autonomousCopy(lang)
  const [refreshing, setRefreshing] = useState(false)
  const refresh = useCallback(async () => {
    setRefreshing(true)
    try { await onRefresh?.() } finally { setRefreshing(false) }
  }, [onRefresh])
  const [tab, setTab] = useState('overview')
  const tabs = [['overview', lang === 'en' ? 'Tasks overview' : '任务总览'], ['records', copy.records]]
  return <section className="autonomous-page">
    <div className="autonomous-toolbar"><div className="autonomous-tabs" role="tablist">{tabs.map(([id, label]) => <button type="button" role="tab" aria-selected={tab === id} className={tab === id ? 'active' : ''} key={id} onClick={() => setTab(id)}>{label}</button>)}</div><button type="button" className="autonomous-refresh" disabled={refreshing} onClick={refresh}><RefreshCw className={refreshing ? 'spin' : ''} size={16}/>{copy.refresh}</button></div>
    {tab === 'overview' && <div className="autonomous-overview-pane"><div className="autonomous-service-strip">{services.length ? services.map(service => <AutonomousServiceCard key={service.name} service={service} lang={lang} llms={llms} actionState={actionStates[service.name]} onStart={onStart} onStop={onStop} onLogs={onLogs} onAutostart={onAutostart} onModel={onModel} compact/>) : <div className="autonomous-empty">{lang === 'en' ? 'No autonomous service was found' : '未发现自主进化服务'}</div>}</div><AutonomousTaskWorkspace lang={lang} /></div>}
    {tab === 'records' && (
      <ReportPane reports={reports} lang={lang}/>
    )}
  </section>
}

export default AutonomousPage
