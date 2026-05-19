import { useEffect, useMemo, useRef, useState } from 'react'
import { Bot, ChevronLeft, Clock3, Menu, MessageSquarePlus, RefreshCw, Send, Sparkles, Trash2 } from 'lucide-react'

const api = async (url, options = {}) => {
  const res = await fetch(url, { headers: { 'Content-Type': 'application/json', ...(options.headers || {}) }, ...options })
  if (!res.ok) throw new Error(await res.text())
  return res.json()
}

const fmtTime = (v) => {
  if (!v) return ''
  try { return new Date(v * 1000).toLocaleString() } catch { return '' }
}

const shortTitle = (s) => s?.title || '新会话'
const examples = [
  ['巡检系统', '概览当前 GenericAgent 状态'],
  ['定位错误', '帮我检查最近的错误日志'],
  ['规划进化', '规划下一步自主进化任务'],
  ['审查模型', '总结当前模型配置风险'],
]

export default function ChatApp() {
  const [sessions, setSessions] = useState([])
  const [sid, setSid] = useState('')
  const [messages, setMessages] = useState([])
  const [prompt, setPrompt] = useState('')
  const [busy, setBusy] = useState(false)
  const [err, setErr] = useState('')
  const [collapsed, setCollapsed] = useState(false)
  const [notice, setNotice] = useState('')
  const endRef = useRef(null)
  const current = useMemo(() => sessions.find(s => s.id === sid), [sessions, sid])

  const loadSessions = async (prefer = sid) => {
    const d = await api('/api/chat/sessions')
    const list = d.sessions || []
    setSessions(list)
    const next = prefer || list[0]?.id || ''
    if (next) await openSession(next, false)
    return list
  }

  const openSession = async (id, refreshList = true) => {
    const d = await api(`/api/chat/session/${id}`)
    setSid(d.id)
    setMessages(d.messages || [])
    setErr('')
    setNotice('')
    if (refreshList) {
      setSessions(xs => xs.map(x => x.id === d.id ? { ...x, title: d.title, count: d.messages?.length || x.count, updated_at: d.updated_at || x.updated_at } : x))
    }
  }

  const newSession = async () => {
    const d = await api('/api/chat/session/new', { method:'POST', body:'{}' })
    setSid(d.id)
    setMessages([])
    setPrompt('')
    setErr('')
    setNotice('已创建新会话')
    await loadSessions(d.id)
  }

  const deleteSession = async (id) => {
    if (!id || !confirm('删除这个会话？')) return
    await api(`/api/chat/session/${id}`, { method:'DELETE' })
    setNotice('会话已删除')
    setSid('')
    setMessages([])
    await loadSessions('')
  }

  useEffect(()=>{ loadSessions('').catch(e=>setErr(e.message)) }, [])
  useEffect(()=>{ endRef.current?.scrollIntoView({ behavior:'smooth', block:'end' }) }, [messages, busy])

  const send = async () => {
    const text = prompt.trim()
    if (!text || busy) return
    setBusy(true)
    setErr('')
    setNotice('')
    setPrompt('')
    let useSid = sid
    let optimistic = [...messages]
    if (!useSid) {
      const d = await api('/api/chat/session/new', { method:'POST', body:'{}' })
      useSid = d.id
      setSid(useSid)
      await loadSessions(useSid)
      optimistic = []
    }
    const now = Math.floor(Date.now()/1000)
    const user = { id:`u-${Date.now()}`, role:'user', content:text, created_at:now }
    const assistant = { id:`a-${Date.now()}`, role:'assistant', content:'', created_at:now }
    setMessages([...optimistic, user, assistant])
    try {
      const res = await fetch(`/api/chat/${useSid}`, { method:'POST', headers:{'Content-Type':'application/json'}, body:JSON.stringify({ prompt:text, client_user_id:user.id }) })
      if (!res.ok || !res.body) throw new Error(await res.text())
      const reader = res.body.getReader()
      const dec = new TextDecoder()
      let buf = ''
      while (true) {
        const { done, value } = await reader.read()
        if (done) break
        buf += dec.decode(value, { stream:true })
        const parts = buf.split('\n\n')
        buf = parts.pop() || ''
        for (const part of parts) {
          const line = part.split('\n').find(x=>x.startsWith('data:'))
          if (!line) continue
          const raw = line.slice(5).trim()
          if (!raw) continue
          const ev = JSON.parse(raw)
          if (ev.type === 'message' && ev.message) {
            setMessages(ms => ms.map(m => m.id === assistant.id ? ev.message : m))
          }
          if ((ev.type === 'done' || ev.type === 'error') && ev.message) {
            setMessages(ms => ms.map(m => m.id === assistant.id ? ev.message : m))
          }
          if (ev.type === 'error') setErr(ev.error || 'Worker error')
        }
      }
      await loadSessions(useSid)
    } catch (e) {
      setErr(e.message)
      setMessages(ms => ms.map(m => m.id === assistant.id ? {...m, content:e.message, error:true} : m))
    } finally {
      setBusy(false)
    }
  }

  return <div className={`oa-chat ${collapsed ? 'is-collapsed' : ''}`}>
    <aside className="oa-sidebar">
      <div className="oa-brand-row">
        <div className="oa-brand-mark"><Sparkles size={17}/></div>
        <div><b>GenericAgent</b><span>Command Chat</span></div>
        <button className="oa-icon-btn" onClick={()=>setCollapsed(true)} title="收起侧栏"><Menu size={18}/></button>
      </div>
      <button className="oa-new-chat" onClick={newSession}><MessageSquarePlus size={16}/><span>开启新对话</span></button>
      <div className="oa-section-label">最近会话</div>
      <div className="oa-session-list">
        {sessions.map(s => <button key={s.id} className={`oa-session ${s.id===sid?'active':''}`} onClick={()=>openSession(s.id)}>
          <span>{shortTitle(s)}</span>
          <small><Clock3 size={11}/>{fmtTime(s.updated_at) || '刚刚'} · {s.count || 0} 条</small>
        </button>)}
        {!sessions.length && <div className="oa-empty-list">暂无历史会话</div>}
      </div>
      <div className="oa-sidebar-foot">
        <button onClick={()=>loadSessions().catch(e=>setErr(e.message))}><RefreshCw size={15}/>刷新会话</button>
        <button onClick={()=>window.location.href='/'}><ChevronLeft size={15}/>返回管理台</button>
      </div>
    </aside>

    <main className="oa-main">
      <header className="oa-topbar">
        {collapsed && <button className="oa-icon-btn" onClick={()=>setCollapsed(false)} title="展开侧栏"><Menu size={18}/></button>}
        <div className="oa-title"><b>{current ? shortTitle(current) : '新对话'}</b><span>OpenAI-style workspace for local agent operations</span></div>
        <div className="oa-top-actions">
          <button onClick={newSession}>新对话</button>
          <button disabled={!sid} onClick={()=>deleteSession(sid)}><Trash2 size={15}/>删除</button>
        </div>
      </header>

      {(err || notice) && <div className={`oa-banner ${err ? 'error' : ''}`}>{err || notice}</div>}

      <section className="oa-thread">
        {messages.length === 0 && <div className="oa-empty">
          <div className="oa-hero-badge"><Sparkles size={16}/>Agent cockpit</div>
          <h1>今天想让 GenericAgent 做什么？</h1>
          <p>像 ChatGPT 一样对话，但面向本地生命周期管理、排错、模型配置和自主进化。</p>
          <div className="oa-prompts">{examples.map(([k, x]) => <button key={x} onClick={()=>setPrompt(x)}><b>{k}</b><span>{x}</span></button>)}</div>
        </div>}
        {messages.map(m => <article key={m.id} className={`oa-message ${m.role} ${m.error?'error':''}`}>
          <div className="oa-avatar">{m.role === 'user' ? '你' : 'GA'}</div>
          <div className="oa-bubble">
            <div className="oa-meta"><b>{m.role === 'user' ? 'You' : 'GenericAgent'}</b>{m.created_at && <span>{fmtTime(m.created_at)}</span>}</div>
            <div className="oa-content">{m.content || (busy && m.role === 'assistant' ? '正在思考…' : '')}</div>
          </div>
        </article>)}
        <div ref={endRef}/>
      </section>

      <footer className="oa-composer-wrap">
        <div className="oa-composer">
          <textarea value={prompt} onChange={e=>setPrompt(e.target.value)} onKeyDown={e=>{ if(e.key==='Enter' && (e.ctrlKey || e.metaKey)) send() }} placeholder="向 GenericAgent 发送消息…" rows={1}/>
          <button className="oa-send" disabled={busy || !prompt.trim()} onClick={send}><Send size={17}/></button>
        </div>
        <p>Ctrl / ⌘ + Enter 发送 · 本地执行前请确认意图清晰</p>
      </footer>
    </main>
  </div>
}
