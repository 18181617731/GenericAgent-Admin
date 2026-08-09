import React, { useCallback, useEffect, useMemo, useState } from 'react'
import { AlertTriangle, ArrowLeft, ArrowRight, ClipboardCheck, Plus, RefreshCw, UserRound } from 'lucide-react'
import { api } from '../lib/api'
import { confirmDanger } from '../lib/danger'
import './WorkboardPage.css'

const FALLBACK_STATUSES = ['backlog', 'active', 'review', 'done']

const COPY = {
  en: {
    eyebrow: 'Reviewed workflow', title: 'Workboard', desc: 'A persistent queue for owned, risk-labelled work. Every change is reviewed before it is written.',
    newItem: 'New work item', itemTitle: 'Title', titleHint: 'What needs a reviewed outcome?', owner: 'Owner', ownerHint: 'Team or person (optional)', risk: 'Risk',
    create: 'Add to backlog', refresh: 'Refresh', loading: 'Loading workboard...', empty: 'No items in this stage.', error: 'Workboard could not be loaded.', retry: 'Try again',
    backlog: 'Backlog', active: 'Active', review: 'Review', done: 'Done', low: 'Low', medium: 'Medium', high: 'High',
    moveBack: 'Move back', moveForward: 'Move forward', noOwner: 'Unassigned', confirmCreate: 'Add this item to the persistent workboard?',
    confirmMove: (title, status) => `Move "${title}" to ${status}?`, items: n => `${n} ${n === 1 ? 'item' : 'items'}`,
  },
  zh: {
    eyebrow: '\u5ba1\u6279\u6d41\u7a0b', title: '\u5de5\u4f5c\u770b\u677f', desc: '\u6301\u4e45\u5316\u8bb0\u5f55\u8d1f\u8d23\u4eba\u548c\u98ce\u9669\u7b49\u7ea7\uff0c\u6bcf\u6b21\u53d8\u66f4\u5747\u5728\u5199\u5165\u524d\u786e\u8ba4\u3002',
    newItem: '\u65b0\u5efa\u5de5\u4f5c\u9879', itemTitle: '\u6807\u9898', titleHint: '\u9700\u8981\u5b8c\u6210\u4ec0\u4e48\u5ba1\u6279\u7ed3\u679c\uff1f', owner: '\u8d1f\u8d23\u4eba', ownerHint: '\u56e2\u961f\u6216\u4e2a\u4eba\uff08\u53ef\u9009\uff09', risk: '\u98ce\u9669',
    create: '\u52a0\u5165\u5f85\u529e', refresh: '\u5237\u65b0', loading: '\u6b63\u5728\u52a0\u8f7d\u770b\u677f...', empty: '\u6b64\u9636\u6bb5\u6682\u65e0\u5de5\u4f5c\u9879\u3002', error: '\u770b\u677f\u52a0\u8f7d\u5931\u8d25\u3002', retry: '\u91cd\u8bd5',
    backlog: '\u5f85\u529e', active: '\u8fdb\u884c\u4e2d', review: '\u5ba1\u6838', done: '\u5b8c\u6210', low: '\u4f4e', medium: '\u4e2d', high: '\u9ad8',
    moveBack: '\u9000\u56de\u4e0a\u4e00\u9636\u6bb5', moveForward: '\u63a8\u8fdb\u4e0b\u4e00\u9636\u6bb5', noOwner: '\u672a\u5206\u914d', confirmCreate: '\u5c06\u6b64\u9879\u76ee\u5199\u5165\u6301\u4e45\u5316\u770b\u677f\uff1f',
    confirmMove: (title, status) => `\u5c06\u201c${title}\u201d\u79fb\u81f3${status}\uff1f`, items: n => `${n} \u9879`,
  },
}

export default function WorkboardPage({ lang = 'en' }) {
  const text = COPY[lang] || COPY.en
  const [items, setItems] = useState([])
  const [statuses, setStatuses] = useState(FALLBACK_STATUSES)
  const [form, setForm] = useState({ title: '', owner: '', risk: 'low' })
  const [loading, setLoading] = useState(true)
  const [busy, setBusy] = useState('')
  const [error, setError] = useState('')

  const load = useCallback(async () => {
    setLoading(true); setError('')
    try {
      const data = await api('/api/workboard')
      setItems(Array.isArray(data?.items) ? data.items : [])
      setStatuses(Array.isArray(data?.statuses) && data.statuses.length ? data.statuses : FALLBACK_STATUSES)
    } catch (err) { setError(err.message || text.error) }
    finally { setLoading(false) }
  }, [text.error])

  useEffect(() => { load() }, [load])

  const grouped = useMemo(() => Object.fromEntries(statuses.map(status => [status, items.filter(item => item.status === status)])), [items, statuses])

  const createItem = async event => {
    event.preventDefault()
    const payload = { title: form.title.trim(), owner: form.owner.trim(), risk: form.risk }
    if (!payload.title || !confirmDanger('workboard-create', text.confirmCreate)) return
    setBusy('create'); setError('')
    try {
      const created = await api('/api/workboard', { dangerous: true, method: 'POST', body: JSON.stringify(payload) })
      setItems(current => [...current, created])
      setForm({ title: '', owner: '', risk: 'low' })
    } catch (err) { setError(err.message || text.error) }
    finally { setBusy('') }
  }

  const move = async (item, direction) => {
    const index = statuses.indexOf(item.status)
    const status = statuses[index + direction]
    if (!status || !confirmDanger('workboard-transition', text.confirmMove(item.title, text[status] || status))) return
    setBusy(item.id); setError('')
    try {
      const updated = await api(`/api/workboard/${encodeURIComponent(item.id)}`, { dangerous: true, method: 'PATCH', body: JSON.stringify({ status }) })
      setItems(current => current.map(candidate => candidate.id === updated.id ? updated : candidate))
    } catch (err) { setError(err.message || text.error) }
    finally { setBusy('') }
  }

  return <section className="workboard-page" aria-labelledby="workboard-title">
    <header className="workboard-intro">
      <div><p className="workboard-eyebrow"><ClipboardCheck size={15}/>{text.eyebrow}</p><h2 id="workboard-title">{text.title}</h2><p>{text.desc}</p></div>
      <button className="workboard-refresh" type="button" onClick={load} disabled={loading}><RefreshCw size={15}/>{text.refresh}</button>
    </header>

    <form className="workboard-create" onSubmit={createItem}>
      <strong>{text.newItem}</strong>
      <label className="workboard-title-field"><span>{text.itemTitle}</span><input aria-label={text.itemTitle} maxLength={160} required value={form.title} placeholder={text.titleHint} onChange={event => setForm({ ...form, title: event.target.value })}/></label>
      <label><span>{text.owner}</span><input aria-label={text.owner} maxLength={80} value={form.owner} placeholder={text.ownerHint} onChange={event => setForm({ ...form, owner: event.target.value })}/></label>
      <label><span>{text.risk}</span><select aria-label={text.risk} value={form.risk} onChange={event => setForm({ ...form, risk: event.target.value })}>{['low','medium','high'].map(risk => <option key={risk} value={risk}>{text[risk]}</option>)}</select></label>
      <button type="submit" disabled={busy === 'create' || !form.title.trim()}><Plus size={16}/>{text.create}</button>
    </form>

    {error && <div className="workboard-error" role="alert"><AlertTriangle size={17}/><span>{error}</span>{!items.length && <button type="button" onClick={load}>{text.retry}</button>}</div>}
    {loading ? <div className="workboard-state" role="status">{text.loading}</div> : <div className="workboard-columns">
      {statuses.map((status, statusIndex) => <section className={`workboard-column is-${status}`} key={status} aria-labelledby={`workboard-${status}`}>
        <header><div><span className="workboard-status-mark"/><h3 id={`workboard-${status}`}>{text[status] || status}</h3></div><span>{text.items(grouped[status]?.length || 0)}</span></header>
        <div className="workboard-stack">
          {(grouped[status] || []).map(item => <article className="workboard-card" key={item.id}>
            <div className="workboard-card-top"><span className={`workboard-risk is-${item.risk}`}>{item.risk === 'high' && <AlertTriangle size={12}/>} {text[item.risk] || item.risk}</span><code>{item.id.slice(0, 6)}</code></div>
            <h4>{item.title}</h4><p><UserRound size={14}/>{item.owner || text.noOwner}</p>
            <div className="workboard-actions">
              <button type="button" aria-label={`${text.moveBack}: ${item.title}`} title={text.moveBack} disabled={statusIndex === 0 || busy === item.id} onClick={() => move(item, -1)}><ArrowLeft size={15}/></button>
              <button type="button" aria-label={`${text.moveForward}: ${item.title}`} title={text.moveForward} disabled={statusIndex === statuses.length - 1 || busy === item.id} onClick={() => move(item, 1)}><ArrowRight size={15}/></button>
            </div>
          </article>)}
          {!grouped[status]?.length && <p className="workboard-empty">{text.empty}</p>}
        </div>
      </section>)}
    </div>}
  </section>
}
