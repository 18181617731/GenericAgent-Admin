import { useEffect, useState } from 'react'
import { CalendarDays, ChevronLeft, ChevronRight, Clock3, Database, Download, Filter, RotateCcw, Search } from 'lucide-react'
import { formatElapsed, formatNumber, formatTokens, formatUsageDateTime } from '../lib/usage'

const COPY = {
  zh: {
    title: '使用记录', intro: '记录所有已完成的模型调用，包括对话、标题生成、自主进化、定时任务、Goal 模式和模型检测。Token 只来自模型返回的 usage。',
    from: '开始日期', to: '结束日期', provider: '服务商', allProviders: '全部服务商', model: '模型名称', modelPlaceholder: '搜索模型、会话或标识',
    search: '查询', reset: '重置', export: '导出 CSV', records: '条记录', empty: '没有符合条件的使用记录。', unknownProvider: '历史记录未保存服务商', unknownSession: '未命名会话',
    time: '时间', channel: '调用渠道', source: '来源', reasoning: '推理强度', modelColumn: '模型', session: '会话', input: '输入', cache: '缓存', output: '输出', total: '总量', duration: '耗时',
    page: '第', pageOf: '页，共', pageSize: '每页', previous: '上一页', next: '下一页', loading: '正在更新记录…', exportFailed: '导出失败',
  },
  en: {
    title: 'Usage records', intro: 'Includes chat, title generation, autonomous runs, scheduled tasks, Goal Mode, and model probes. Token values come from model usage responses.',
    from: 'From', to: 'To', provider: 'Provider', allProviders: 'All providers', model: 'Model name', modelPlaceholder: 'Search model, session, or ID',
    search: 'Query', reset: 'Reset', export: 'Export CSV', records: 'records', empty: 'No usage records match the current filters.', unknownProvider: 'Provider not retained', unknownSession: 'Untitled session',
    time: 'Time', channel: 'Channel', source: 'Source', reasoning: 'Reasoning', modelColumn: 'Model', session: 'Session', input: 'Input', cache: 'Cache', output: 'Output', total: 'Total', duration: 'Duration',
    page: 'Page', pageOf: 'of', pageSize: 'Per page', previous: 'Previous', next: 'Next', loading: 'Updating records…', exportFailed: 'Export failed',
  },
}

const CHANNEL_LABELS = {
  chat: ['对话', 'Chat'], title_generation: ['标题生成', 'Title generation'], side_question: ['旁问', 'Side question'],
  autonomous: ['自主进化', 'Autonomous'], scheduled_task: ['定时任务', 'Scheduled task'], goal: ['Goal 模式', 'Goal Mode'],
  model_probe: ['模型检测', 'Model probe'], service: ['后台服务', 'Background service'],
}

function channelLabel(value, lang) {
  const labels = CHANNEL_LABELS[value]
  return labels ? labels[lang === 'zh' ? 0 : 1] : value || (lang === 'zh' ? '其他' : 'Other')
}

function reasoningLabel(value, lang) {
  const normalized = String(value || '').trim().toLowerCase()
  if (!normalized || normalized === 'off') return lang === 'zh' ? '默认' : 'Default'
  return `reasoning_effort=${normalized}`
}

function RecordModel({ record, copy }) {
  const modelName = record.model_name || record.model_id || copy.modelColumn
  return <div className="usage-record-model"><strong>{modelName}</strong>{record.model_id && record.model_id !== modelName && <small>{record.model_id}</small>}</div>
}

function RecordMetrics({ record, lang, copy }) {
  const values = [
    [copy.input, record.input_tokens],
    [copy.cache, record.cached_tokens],
    [copy.output, record.output_tokens],
    [copy.total, record.total_tokens],
  ]
  return <div className="usage-record-metrics">{values.map(([label, value]) => <span key={label}><small>{label}</small><b title={formatTokens(value, lang).full}>{formatTokens(value, lang).short}</b></span>)}</div>
}

function UsageRecordCard({ record, lang, copy }) {
  return <article className="usage-record-card">
    <div className="usage-record-card-head"><time dateTime={record.created_at_ms ? new Date(record.created_at_ms).toISOString() : undefined}>{formatUsageDateTime(record.created_at_ms, lang)}</time><span><Clock3 size={13}/>{formatElapsed(record.elapsed_ms, lang)}</span></div>
    <div className="usage-record-card-model"><span className="usage-record-provider">{record.provider || copy.unknownProvider}</span><RecordModel record={record} copy={copy}/></div>
    <div className="usage-record-card-context"><b>{channelLabel(record.channel, lang)}</b><span>{record.source || (lang === 'zh' ? '未记录来源' : 'Source not retained')}</span><em>{reasoningLabel(record.reasoning_effort, lang)}</em></div>
    <div className="usage-record-card-session"><Database size={13}/><span>{record.session_name || copy.unknownSession}</span><small>{record.session_id}</small></div>
    <RecordMetrics record={record} lang={lang} copy={copy}/>
  </article>
}

export function UsageRecords({ data = {}, lang = 'zh', onQuery, onExport, loading = false, exporting = false, exportError = '' }) {
  const copy = COPY[lang] || COPY.zh
  const [filters, setFilters] = useState({ from: '', to: '', provider: '', model: '' })
  const [pageSize, setPageSize] = useState(Number(data.record_page_size) || 20)
  const records = Array.isArray(data.records) ? data.records : []
  const providers = Array.isArray(data.record_providers) ? data.record_providers : []
  const models = Array.isArray(data.record_models) ? data.record_models : []
  const page = Math.max(1, Number(data.record_page) || 1)
  const totalPages = Math.max(0, Number(data.record_total_pages) || 0)
  const total = Math.max(0, Number(data.record_total) || 0)

  useEffect(() => {
    const nextPageSize = Number(data.record_page_size)
    if (nextPageSize > 0 && nextPageSize !== pageSize) setPageSize(nextPageSize)
  }, [data.record_page_size, pageSize])

  const patchFilter = (key, value) => setFilters(current => ({ ...current, [key]: value }))
  const submit = event => {
    event.preventDefault()
    onQuery?.(filters, 1, pageSize)
  }
  const reset = () => {
    const empty = { from: '', to: '', provider: '', model: '' }
    setFilters(empty)
    onQuery?.(empty, 1, pageSize)
  }
  const changePageSize = event => {
    const nextSize = Number(event.target.value) || 20
    setPageSize(nextSize)
    onQuery?.(filters, 1, nextSize)
  }
  const movePage = nextPage => {
    if (nextPage < 1 || nextPage > totalPages || nextPage === page) return
    onQuery?.(filters, nextPage, pageSize)
  }

  return <section className="usage-panel usage-records-panel" aria-labelledby="usage-records-title" aria-busy={loading}>
    <div className="usage-records-head"><div><h2 id="usage-records-title"><Filter size={16}/>{copy.title}</h2><p>{copy.intro}</p></div><div className="usage-record-count"><b>{formatNumber(total, lang)}</b><span>{copy.records}</span></div></div>
    <form className="usage-record-filters" onSubmit={submit}>
      <label><span><CalendarDays size={13}/>{copy.from}</span><input type="date" value={filters.from} onChange={event => patchFilter('from', event.target.value)}/></label>
      <label><span><CalendarDays size={13}/>{copy.to}</span><input type="date" value={filters.to} onChange={event => patchFilter('to', event.target.value)}/></label>
      <label><span>{copy.provider}</span><select value={filters.provider} onChange={event => patchFilter('provider', event.target.value)}><option value="">{copy.allProviders}</option>{providers.map(provider => <option key={provider} value={provider}>{provider}</option>)}</select></label>
      <label className="usage-record-model-filter"><span>{copy.model}</span><input list="usage-model-options" value={filters.model} placeholder={copy.modelPlaceholder} onChange={event => patchFilter('model', event.target.value)}/></label>
      <datalist id="usage-model-options">{models.map(model => <option key={model} value={model}/>)}</datalist>
      <div className="usage-record-filter-actions"><button type="submit"><Search size={14}/>{copy.search}</button><button type="button" className="button-quiet" onClick={reset}><RotateCcw size={14}/>{copy.reset}</button><button type="button" className="button-quiet" disabled={exporting} onClick={() => onExport?.(filters)}><Download size={14}/>{exporting ? copy.loading : copy.export}</button></div>
    </form>
    {exportError && <div className="usage-record-export-error" role="alert"><strong>{copy.exportFailed}</strong><span>{exportError}</span></div>}
    {loading && <div className="usage-record-loading" role="status">{copy.loading}</div>}
    {records.length > 0 ? <>
      <div className="usage-table-wrap usage-record-table-wrap"><table className="usage-record-table"><thead><tr><th>{copy.time}</th><th>{copy.channel}</th><th>{copy.source}</th><th>{copy.reasoning}</th><th>{copy.provider}</th><th>{copy.modelColumn}</th><th>{copy.session}</th><th>{copy.input}</th><th>{copy.cache}</th><th>{copy.output}</th><th>{copy.total}</th><th>{copy.duration}</th></tr></thead><tbody>{records.map(record => <tr key={record.id}><td><time dateTime={record.created_at_ms ? new Date(record.created_at_ms).toISOString() : undefined}>{formatUsageDateTime(record.created_at_ms, lang)}</time></td><td><span className="usage-record-channel">{channelLabel(record.channel, lang)}</span></td><td className="usage-record-source" title={record.source || ''}>{record.source || '—'}</td><td className="usage-record-reasoning">{reasoningLabel(record.reasoning_effort, lang)}</td><td>{record.provider || copy.unknownProvider}</td><td><RecordModel record={record} copy={copy}/></td><td><strong>{record.session_name || copy.unknownSession}</strong><small>{record.session_id}</small></td><td>{formatTokens(record.input_tokens, lang).short}</td><td>{formatTokens(record.cached_tokens, lang).short}</td><td>{formatTokens(record.output_tokens, lang).short}</td><td><b>{formatTokens(record.total_tokens, lang).short}</b></td><td>{formatElapsed(record.elapsed_ms, lang)}</td></tr>)}</tbody></table></div>
      <div className="usage-record-cards">{records.map(record => <UsageRecordCard key={record.id} record={record} lang={lang} copy={copy}/>)}</div>
    </> : <div className="usage-record-empty"><Database size={18}/><span>{copy.empty}</span></div>}
    <div className="usage-record-pagination"><label><span>{copy.pageSize}</span><select value={pageSize} onChange={changePageSize}><option value="10">10</option><option value="20">20</option><option value="50">50</option><option value="100">100</option></select></label><span>{copy.page} {page} {copy.pageOf} {totalPages || 1}</span><div><button type="button" className="button-icon" aria-label={copy.previous} title={copy.previous} disabled={page <= 1 || totalPages === 0} onClick={() => movePage(page - 1)}><ChevronLeft size={16}/></button><button type="button" className="button-icon" aria-label={copy.next} title={copy.next} disabled={page >= totalPages || totalPages === 0} onClick={() => movePage(page + 1)}><ChevronRight size={16}/></button></div></div>
  </section>
}
