import { useCallback, useEffect, useState } from 'react'
import { AlertTriangle, BarChart3, RefreshCw } from 'lucide-react'
import { UsageRecords } from '../components/UsageRecords'
import { api } from '../lib/api'
import { formatNumber, formatTokens, usageQueryString } from '../lib/usage'

const COPY = {
  zh: {
    title: '累计 Token 用量', intro: '汇总所有已记录的模型调用，包括对话、后台自动化、Goal、定时任务、标题生成和模型检测。',
    total: '总 Token', input: '输入 Token', output: '输出 Token', sessions: '有用量运行', replies: '模型调用次数',
    models: '按模型', model: '模型', empty: '尚未记录到 Token 用量。',
    loading: '正在汇总会话用量…', failed: '无法加载用量总览', retry: '重试', refresh: '刷新', skipped: '个会话文件无法读取，已跳过。', unknown: '未知模型',
    heatmap: '每日活跃度', heatmapHint: '过去 52 周 · 按每日 Token 用量着色', less: '少', more: '多',
  },
  en: {
    title: 'Cumulative token usage', intro: 'Summarizes chat, background automation, Goal Mode, scheduled tasks, title generation, and model probes.',
    total: 'Total tokens', input: 'Input tokens', output: 'Output tokens', sessions: 'Runs with usage', replies: 'Model calls',
    models: 'By model', model: 'Model', empty: 'No token usage has been recorded yet.',
    loading: 'Aggregating session usage…', failed: 'Unable to load usage overview', retry: 'Retry', refresh: 'Refresh', skipped: 'session files could not be read and were skipped.', unknown: 'Unknown model',
    heatmap: 'Daily activity', heatmapHint: 'Past 52 weeks · colored by daily token usage', less: 'Less', more: 'More',
  },
}

function Metric({ label, value, title, accent }) {
  return <div className={`usage-metric${accent ? ' usage-metric-accent' : ''}`} title={title}><span>{label}</span><strong>{value}</strong></div>
}

function UsageHeatmap({ daily = [], lang, copy }) {
  const values = new Map(daily.map(day => [day.date, day]))
  const end = new Date(); end.setHours(0, 0, 0, 0)
  const start = new Date(end); start.setDate(end.getDate() - (51 * 7 + end.getDay()))
  const cells = []
  for (const cursor = new Date(start); cursor <= end; cursor.setDate(cursor.getDate() + 1)) {
    const date = `${cursor.getFullYear()}-${String(cursor.getMonth() + 1).padStart(2, '0')}-${String(cursor.getDate()).padStart(2, '0')}`
    const day = values.get(date) || { date, assistant_replies: 0, totals: { total_tokens: 0 } }
    cells.push(day)
  }
  const active = cells.map(day => Number(day.totals?.total_tokens) || 0).filter(Boolean).sort((a, b) => a - b)
  const level = value => {
    if (!value || !active.length) return 0
    const rank = active.findIndex(item => item >= value)
    return Math.max(1, Math.min(4, Math.ceil(((rank + 1) / active.length) * 4)))
  }
  const dateFormat = new Intl.DateTimeFormat(lang === 'zh' ? 'zh-CN' : 'en-US', { dateStyle: 'medium' })
  const monthFormat = new Intl.DateTimeFormat(lang === 'zh' ? 'zh-CN' : 'en-US', { month: 'short' })
  const months = []
  const seenMonths = new Set()
  cells.forEach((day, index) => {
    const date = new Date(`${day.date}T00:00:00`)
    const monthKey = `${date.getFullYear()}-${date.getMonth()}`
    if (date.getDate() <= 7 && !seenMonths.has(monthKey)) {
      seenMonths.add(monthKey)
      months.push({ key: monthKey, column: Math.floor(index / 7) + 1, label: monthFormat.format(date) })
    }
  })
  const weekdays = lang === 'zh' ? ['一', '三', '五'] : ['Mon', 'Wed', 'Fri']
  return <section className="usage-card usage-heatmap-card" aria-labelledby="usage-heatmap-title">
    <div className="usage-section-head"><div><h2 id="usage-heatmap-title">{copy.heatmap}</h2><p>{copy.heatmapHint}</p></div></div>
    <div className="usage-heatmap-scroll">
      <div className="usage-heatmap-frame">
        <div className="usage-months" aria-hidden="true">{months.map(month => <span key={month.key} style={{ gridColumn: month.column }}>{month.label}</span>)}</div>
        <div className="usage-heatmap-body">
          <div className="usage-weekdays" aria-hidden="true"><span>{weekdays[0]}</span><span>{weekdays[1]}</span><span>{weekdays[2]}</span></div>
          <div className="usage-heatmap" role="img" aria-label={copy.heatmapHint}>
            {cells.map(day => { const tokens = Number(day.totals?.total_tokens) || 0; const label = `${dateFormat.format(new Date(`${day.date}T00:00:00`))}: ${formatNumber(tokens, lang)} Token, ${formatNumber(day.assistant_replies, lang)} ${copy.replies}`; return <span key={day.date} className="usage-heat-cell" data-level={level(tokens)} title={label} aria-label={label} /> })}
          </div>
        </div>
        <div className="usage-heat-legend"><span>{copy.less}</span>{[0, 1, 2, 3, 4].map(item => <i key={item} data-level={item} />)}<span>{copy.more}</span></div>
      </div>
    </div>
  </section>
}

export function UsagePage({ lang = 'zh' }) {
  const c = COPY[lang] || COPY.zh
  const [data, setData] = useState(null)
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState('')
  const [exportError, setExportError] = useState('')
  const [exporting, setExporting] = useState(false)
  const [recordFilters, setRecordFilters] = useState({ from: '', to: '', provider: '', model: '' })
  const [recordPage, setRecordPage] = useState(1)
  const [recordPageSize, setRecordPageSize] = useState(20)

  const load = useCallback(async () => {
    setLoading(true); setError('')
    try { setData(await api(`/api/usage/overview?${usageQueryString(recordFilters, recordPage, recordPageSize)}`)) }
    catch (err) { setError(err instanceof Error ? err.message : String(err)) }
    finally { setLoading(false) }
  }, [recordFilters, recordPage, recordPageSize])

  useEffect(() => { load() }, [load])
  const n = value => formatNumber(value, lang)
  const tok = value => formatTokens(value, lang)
  const queryRecords = (filters, page, pageSize) => {
    setRecordFilters(filters)
    setRecordPage(page)
    setRecordPageSize(pageSize)
  }
  const exportRecords = async filters => {
    setExporting(true)
    setExportError('')
    try {
      const response = await fetch(`/api/usage/export?${usageQueryString(filters, 1, 100)}`)
      if (!response.ok) {
        const message = await response.text()
        throw new Error(message || `${response.status} ${response.statusText}`)
      }
      const blob = await response.blob()
      const objectUrl = window.URL.createObjectURL(blob)
      const anchor = document.createElement('a')
      anchor.href = objectUrl
      anchor.download = 'usage-records.csv'
      document.body.appendChild(anchor)
      anchor.click()
      anchor.remove()
      window.URL.revokeObjectURL(objectUrl)
    } catch (err) {
      setExportError(err instanceof Error ? err.message : String(err))
    } finally {
      setExporting(false)
    }
  }

  return <section className="usage-page" aria-busy={loading}>
    <div className="usage-intro">
      <div><span className="usage-eyebrow"><BarChart3 size={15}/>{c.title}</span><p>{c.intro}</p></div>
      <button type="button" onClick={load} disabled={loading}><RefreshCw size={15} className={loading ? 'spin' : ''}/>{c.refresh}</button>
    </div>

    {loading && !data && <div className="usage-state" role="status">{c.loading}</div>}
    {error && <div className="usage-state usage-error" role="alert"><strong>{c.failed}</strong><span>{error}</span><button type="button" onClick={load}>{c.retry}</button></div>}
    {!error && data && <>
      {data.skipped_sessions > 0 && <div className="usage-warning"><AlertTriangle size={16}/><span>{n(data.skipped_sessions)} {c.skipped}</span></div>}
      <div className="usage-metrics">
        <Metric label={c.total} value={tok(data.totals?.total_tokens).full} title={tok(data.totals?.total_tokens).full} accent/>
        <Metric label={c.input} value={tok(data.totals?.input_tokens).full} title={tok(data.totals?.input_tokens).full}/>
        <Metric label={c.output} value={tok(data.totals?.output_tokens).full} title={tok(data.totals?.output_tokens).full}/>
        <Metric label={c.sessions} value={`${n(data.sessions_with_usage)} / ${n(data.session_count)}`}/>
        <Metric label={c.replies} value={n(data.assistant_replies)}/>
      </div>
      {data.assistant_replies === 0 ? <div className="usage-state">{c.empty}</div> : <>
        <UsageRecords data={data} lang={lang} onQuery={queryRecords} onExport={exportRecords} loading={loading} exporting={exporting} exportError={exportError}/>
        <UsageHeatmap daily={data.daily} lang={lang} copy={c}/>
        <section className="usage-panel"><h3>{c.models}</h3><div className="usage-table-wrap"><table><thead><tr><th>{c.model}</th><th>{c.replies}</th><th>{c.input}</th><th>{c.output}</th><th>{c.total}</th></tr></thead><tbody>{(data.models || []).map(item => <tr key={item.id}><td><strong>{item.name || c.unknown}</strong><small>{item.id}</small></td><td>{n(item.assistant_replies)}</td><td title={tok(item.totals?.input_tokens).full}>{tok(item.totals?.input_tokens).short}</td><td title={tok(item.totals?.output_tokens).full}>{tok(item.totals?.output_tokens).short}</td><td title={tok(item.totals?.total_tokens).full}><b>{tok(item.totals?.total_tokens).short}</b></td></tr>)}</tbody></table></div></section></>}
    </>}
  </section>
}
