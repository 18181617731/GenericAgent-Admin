import { useCallback, useEffect, useState } from 'react'
import { AlertTriangle, BarChart3, RefreshCw } from 'lucide-react'
import { api } from '../lib/api'

const formatNumber = (value, lang) => new Intl.NumberFormat(lang === 'zh' ? 'zh-CN' : 'en-US').format(Number(value) || 0)
const formatTime = (value, lang) => value ? new Intl.DateTimeFormat(lang === 'zh' ? 'zh-CN' : 'en-US', { dateStyle: 'medium', timeStyle: 'short' }).format(new Date(value * 1000)) : '-'

const COPY = {
  zh: {
    title: '累计 Token 用量', intro: '统计本机已持久化聊天会话中的模型用量，不包含聊天正文。',
    total: '总 Token', input: '输入 Token', output: '输出 Token', sessions: '有用量会话', replies: '有用量回复',
    models: '按模型', recent: '会话明细', model: '模型', session: '会话', updated: '更新时间', empty: '尚未记录到 Token 用量。',
    loading: '正在汇总会话用量…', failed: '无法加载用量总览', retry: '重试', refresh: '刷新', skipped: '个会话文件无法读取，已跳过。', unknown: '未知模型',
  },
  en: {
    title: 'Cumulative token usage', intro: 'Calculated from locally persisted chat sessions. Message content is never returned.',
    total: 'Total tokens', input: 'Input tokens', output: 'Output tokens', sessions: 'Sessions with usage', replies: 'Measured replies',
    models: 'By model', recent: 'Session details', model: 'Model', session: 'Session', updated: 'Updated', empty: 'No token usage has been recorded yet.',
    loading: 'Aggregating session usage…', failed: 'Unable to load usage overview', retry: 'Retry', refresh: 'Refresh', skipped: 'session files could not be read and were skipped.', unknown: 'Unknown model',
  },
}

function Metric({ label, value, accent }) {
  return <div className={`usage-metric${accent ? ' usage-metric-accent' : ''}`}><span>{label}</span><strong>{value}</strong></div>
}

export function UsagePage({ lang = 'zh' }) {
  const c = COPY[lang] || COPY.zh
  const [data, setData] = useState(null)
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState('')

  const load = useCallback(async () => {
    setLoading(true); setError('')
    try { setData(await api('/api/usage/overview')) }
    catch (err) { setError(err instanceof Error ? err.message : String(err)) }
    finally { setLoading(false) }
  }, [])

  useEffect(() => { load() }, [load])
  const n = value => formatNumber(value, lang)

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
        <Metric label={c.total} value={n(data.totals?.total_tokens)} accent/>
        <Metric label={c.input} value={n(data.totals?.input_tokens)}/>
        <Metric label={c.output} value={n(data.totals?.output_tokens)}/>
        <Metric label={c.sessions} value={`${n(data.sessions_with_usage)} / ${n(data.session_count)}`}/>
        <Metric label={c.replies} value={n(data.assistant_replies)}/>
      </div>
      {data.assistant_replies === 0 ? <div className="usage-state">{c.empty}</div> : <div className="usage-grid">
        <section className="usage-panel"><h3>{c.models}</h3><div className="usage-table-wrap"><table><thead><tr><th>{c.model}</th><th>{c.replies}</th><th>{c.input}</th><th>{c.output}</th><th>{c.total}</th></tr></thead><tbody>{(data.models || []).map(item => <tr key={item.id}><td><strong>{item.name || c.unknown}</strong><small>{item.id}</small></td><td>{n(item.assistant_replies)}</td><td>{n(item.totals?.input_tokens)}</td><td>{n(item.totals?.output_tokens)}</td><td><b>{n(item.totals?.total_tokens)}</b></td></tr>)}</tbody></table></div></section>
        <section className="usage-panel"><h3>{c.recent}</h3><div className="usage-table-wrap"><table><thead><tr><th>{c.session}</th><th>{c.updated}</th><th>{c.replies}</th><th>{c.total}</th></tr></thead><tbody>{(data.sessions || []).map(item => <tr key={item.id}><td><strong>{item.name || item.id}</strong><small>{item.id}</small></td><td>{formatTime(item.updated_at, lang)}</td><td>{n(item.assistant_replies)}</td><td><b>{n(item.totals?.total_tokens)}</b></td></tr>)}</tbody></table></div></section>
      </div>}
    </>}
  </section>
}
