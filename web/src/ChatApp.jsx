import { useEffect, useMemo, useRef, useState } from 'react'
import { Bot, Check, ChevronDown, ChevronLeft, Clock3, Copy, Edit3, Menu, MessageSquarePlus, MoreHorizontal, RefreshCw, Send, Sparkles, Trash2, X } from 'lucide-react'

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
const modelLabel = (m) => m?.label || [m?.name || m?.var_name || `模型 ${m?.index || ''}`, m?.model].filter(Boolean).join(' · ')

const escapeHtml = (s = '') => String(s).replace(/[&<>"']/g, c => ({ '&':'&amp;', '<':'&lt;', '>':'&gt;', '"':'&quot;', "'":'&#39;' }[c]))
const inlineMarkdown = (s = '') => escapeHtml(s)
  .replace(/`([^`]+)`/g, '<code>$1</code>')
  .replace(/\*\*([^*]+)\*\*/g, '<strong>$1</strong>')
  .replace(/\*([^*]+)\*/g, '<em>$1</em>')
  .replace(/\[([^\]]+)\]\((https?:\/\/[^\s)]+)\)/g, '<a href="$2" target="_blank" rel="noreferrer">$1</a>')

function CopyButton({ text, compact = false }) {
  const [ok, setOk] = useState(false)
  const copy = async (e) => {
    e?.stopPropagation?.()
    try {
      await navigator.clipboard.writeText(text || '')
      setOk(true)
      setTimeout(() => setOk(false), 1200)
    } catch {}
  }
  return <button className={compact ? 'oa-mini-copy' : 'oa-copy'} onClick={copy} title="复制">
    {ok ? <Check size={14}/> : <Copy size={14}/>}<span>{ok ? '已复制' : '复制'}</span>
  </button>
}

const splitMarkdownParts = (text = '') => {
  const parts = []
  const re = /(`{3,})([^\n`]*)\n?([\s\S]*?)\1/g
  let last = 0, m
  while ((m = re.exec(text)) !== null) {
    if (m.index > last) parts.push({ type:'text', text:text.slice(last, m.index) })
    parts.push({ type:'code', fence:m[1], lang:(m[2] || '').trim(), text:m[3] || '' })
    last = re.lastIndex
  }
  if (last < text.length) parts.push({ type:'text', text:text.slice(last) })
  if (!parts.length) parts.push({ type:'text', text })
  return parts
}

const isToolResultText = (text = '') => /^\s*\[(Action|Status|Stdout|Stderr|Result|Output)\]/mi.test(String(text || ''))

const normalizeToolParts = (parts = []) => {
  const out = []
  for (let i = 0; i < parts.length; i++) {
    let p = parts[i]
    if (p.type !== 'text') { out.push(p); continue }
    const marker = String(p.text || '').match(/(?:^|\n)🛠️\s*Tool:/)
    if (marker && marker.index > 0) {
      const markerIndex = marker.index + (marker[0].startsWith('\n') ? 1 : 0)
      const prefix = p.text.slice(0, markerIndex)
      if (prefix.trim()) out.push({ type:'text', text:prefix })
      p = { ...p, text:p.text.slice(markerIndex) }
    }
    const tool = parseToolCallBlock(p.text)
    if (!tool) { out.push(p); continue }

    let j = i + 1
    let sawArgs = Boolean(tool.args)
    let pendingArgsFence = /📥\s*args\s*:\s*$/i.test(String(p.text || '').trim())
    let sawResult = false
    while (j < parts.length) {
      const next = parts[j]
      if (next.type === 'text') {
        const args = parseToolArgsBlock(next.text)
        const trimmed = String(next.text || '').trim()
        if (args !== null) {
          tool.args = [tool.args, args].filter(Boolean).join('\n\n')
          sawArgs = true
          pendingArgsFence = false
          j += 1
          continue
        }
        if (isToolResultText(trimmed)) {
          tool.result = [tool.result, trimmed].filter(Boolean).join('\n\n')
          sawResult = true
          j += 1
          continue
        }
        if (!trimmed) { j += 1; continue }
      }
      if (next.type === 'code') {
        if (isToolResultText(next.text) || sawResult) {
          tool.result = [tool.result, next.text].filter(Boolean).join('\n\n')
          sawResult = true
          j += 1
          continue
        }
        if (!sawArgs || pendingArgsFence) {
          tool.args = [tool.args, next.text].filter(Boolean).join('\n\n')
          sawArgs = true
          pendingArgsFence = false
          j += 1
          continue
        }
      }
      break
    }
    out.push({ type:'tool', call:tool })
    i = j - 1
  }
  return out
}

function MarkdownBlock({ text = '' }) {
  const parts = normalizeToolParts(splitMarkdownParts(text))
  return <div className="oa-md">
    {parts.map((p, idx) => p.type === 'code'
      ? <div className="oa-code-card" key={idx}>
          <div className="oa-code-head"><span>{p.lang || '代码'}</span><CopyButton text={p.text} compact /></div>
          <pre><code>{p.text}</code></pre>
        </div>
      : p.type === 'tool'
        ? <ToolCallBlock key={idx} call={p.call} />
        : <TextMarkdown key={idx} text={p.text}/>) }
  </div>
}

const parseToolCallBlock = (block = '') => {
  const text = String(block || '').trim()
  const tool = text.match(/^🛠️\s*Tool:\s*([\s\S]*)$/i)
  if (!tool) return null
  const rest = (tool[1] || '').trim()
  const argsMarker = rest.match(/📥\s*args\s*:/i)
  const cleanName = (name = '') => String(name || '').trim().replace(/^`+|`+$/g, '')
  if (!argsMarker) return { name: cleanName(rest), args: '' }
  const markerIndex = argsMarker.index || 0
  return {
    name: cleanName(rest.slice(0, markerIndex)),
    args: rest.slice(markerIndex + argsMarker[0].length).trim(),
  }
}

const parseToolArgsBlock = (block = '') => {
  const m = String(block || '').trim().match(/^📥\s*args:\s*([\s\S]*)$/i)
  return m ? (m[1] || '').trim() : null
}

function ToolCallBlock({ call }) {
  const [open, setOpen] = useState(false)
  const resultStatus = String(call.result || '').match(/\[Status\]\s*([^\n]+)/i)?.[1]?.trim()
  return <div className={`oa-tool-call ${open ? 'open' : 'collapsed'}`}>
    <button className="oa-tool-head" type="button" onClick={() => setOpen(v => !v)} aria-expanded={open}>
      <span className="oa-tool-icon">🛠️</span><span>Tool</span><b>{call.name || 'unknown'}</b>
      {resultStatus && <em>{resultStatus}</em>}
      <ChevronDown size={15} className="oa-tool-chevron" />
    </button>
    {open && <>
      {call.args && <div className="oa-tool-args"><span>📥 args</span><pre>{call.args}</pre></div>}
      {call.result && <div className="oa-tool-result"><span>📤 result</span><pre>{call.result}</pre></div>}
    </>}
  </div>
}

function renderTextBlock(b, i) {
  const lines = b.split('\n')
  if (lines.every(x => /^\s*([-*]|\d+\.)\s+/.test(x)) && lines.length > 1) {
    return <ul key={i} className="oa-list">{lines.map((x,j)=><li key={j} dangerouslySetInnerHTML={{__html:inlineMarkdown(x.replace(/^\s*([-*]|\d+\.)\s+/, ''))}} />)}</ul>
  }
  if (/^#{1,3}\s+/.test(b.trim())) {
    const level = Math.min(3, b.trim().match(/^#+/)[0].length)
    const body = b.trim().replace(/^#{1,3}\s+/, '')
    const Tag = `h${level + 2}`
    return <Tag key={i} dangerouslySetInnerHTML={{__html:inlineMarkdown(body)}} />
  }
  return <p key={i} dangerouslySetInnerHTML={{__html:inlineMarkdown(b)}} />
}

function TextMarkdown({ text = '' }) {
  const blocks = String(text || '').replace(/\r\n/g, '\n').split(/\n{2,}/)
  const nodes = []
  for (let i = 0; i < blocks.length; i++) {
    const toolCall = parseToolCallBlock(blocks[i])
    if (toolCall) {
      let j = i + 1
      while (j < blocks.length) {
        const args = parseToolArgsBlock(blocks[j])
        if (args === null) break
        toolCall.args = [toolCall.args, args].filter(Boolean).join('\n\n')
        j += 1
      }
      nodes.push(<ToolCallBlock key={i} call={toolCall} />)
      i = j - 1
      continue
    }
    const standaloneArgs = parseToolArgsBlock(blocks[i])
    if (standaloneArgs !== null) {
      nodes.push(<ToolCallBlock key={i} call={{ name: 'unknown', args: standaloneArgs }} />)
      continue
    }
    nodes.push(renderTextBlock(blocks[i], i))
  }
  return <>{nodes}</>
}

const FINAL_MARKER_RE = /```+\s*\n?\[Info\]\s*Final response to user\.\s*\n?```+/i
const TURN_HEADER_RE = /\*\*\s*LLM Running \(Turn\s+(\d+)\)\s*\.\.\.\s*\*\*/gi

const cleanRunBody = (s = '') => String(s || '')
  .replace(/<summary>[\s\S]*?<\/summary>/gi, '')
  .replace(/\n{3,}/g, '\n\n')
  .trim()

const parseAssistantContent = (raw = '') => {
  const full = String(raw || '').replace(/\r\n/g, '\n')
  const finalMatch = full.match(FINAL_MARKER_RE)
  const processText = finalMatch ? full.slice(0, finalMatch.index) : full
  const finalText = finalMatch ? full.slice(finalMatch.index + finalMatch[0].length) : ''
  const runs = []
  const matches = [...processText.matchAll(TURN_HEADER_RE)]

  if (matches.length) {
    matches.forEach((m, i) => {
      const start = m.index + m[0].length
      const end = i + 1 < matches.length ? matches[i + 1].index : processText.length
      const chunk = processText.slice(start, end).trim()
      const summary = chunk.match(/<summary>([\s\S]*?)<\/summary>/i)
      const title = summary?.[1]?.trim() || `Turn ${m[1]}`
      runs.push({ turn: Number(m[1]) || i + 1, title, body: cleanRunBody(chunk) })
    })
    return { runs, body: (finalText || '').replace(/\n{3,}/g, '\n\n').trim() }
  }

  return { runs: [], body: full.replace(FINAL_MARKER_RE, '').replace(/\n{3,}/g, '\n\n').trim() }
}

function AssistantContent({ content, pending }) {
  const [openTurns, setOpenTurns] = useState({})
  if (!content && pending) return <div className="oa-content oa-thinking">正在思考…</div>
  const parsed = parseAssistantContent(content)
  const boxedRuns = parsed.runs.slice(0, -1)
  const lastRun = parsed.runs[parsed.runs.length - 1]
  const isTurnOpen = (r, i) => openTurns[`${r.turn}-${i}`] === true
  const toggleTurn = (r, i) => setOpenTurns(xs => ({ ...xs, [`${r.turn}-${i}`]: !isTurnOpen(r, i) }))
  return <div className={`oa-content ${parsed.runs.length ? 'oa-agent-output' : ''}`}>
    {parsed.runs.length > 0 && <div className="oa-turn-stack">
      <div className="oa-turn-stack-head"><span className="oa-run-dot"/>执行过程 <b>{parsed.runs.length}</b> 轮</div>
      {boxedRuns.map((r, i) => {
        const open = isTurnOpen(r, i)
        return <section className={`oa-turn-card ${open ? 'open' : 'collapsed'}`} key={`${r.turn}-${i}`}>
          <button className="oa-turn-toggle" type="button" onClick={() => toggleTurn(r, i)} aria-expanded={open}>
            <span className="oa-turn-pill">Turn {r.turn}</span><b>{r.title}</b><ChevronDown size={15}/>
          </button>
          {open && (r.body ? <MarkdownBlock text={r.body} /> : <p className="oa-turn-empty">该轮暂无详细输出</p>)}
        </section>
      })}
      {lastRun && <section className="oa-turn-current" key={`last-${lastRun.turn}`}>
        <div className="oa-turn-current-head"><span>Turn {lastRun.turn}</span><b>{lastRun.title}</b></div>
        {lastRun.body ? <MarkdownBlock text={lastRun.body} /> : <p className="oa-turn-empty">该轮暂无详细输出</p>}
      </section>}
    </div>}
    {(parsed.body || !parsed.runs.length) && <div className={parsed.runs.length ? 'oa-final-answer' : ''}>
      {parsed.runs.length && <div className="oa-final-label">返回给用户</div>}
      <MarkdownBlock text={parsed.body || content || ''} />
    </div>}
  </div>
}
const examples = [
  ['巡检系统', '概览当前 GenericAgent 状态'],
  ['定位错误', '帮我检查最近的错误日志，并给出可执行修复方案'],
  ['规划进化', '规划下一步自主进化任务，拆成可验证里程碑'],
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
  const [llms, setLlms] = useState([])
  const [llmNo, setLlmNo] = useState(0)
  const [menuOpen, setMenuOpen] = useState('')
  const [editing, setEditing] = useState('')
  const [draftTitle, setDraftTitle] = useState('')
  const endRef = useRef(null)
  const current = useMemo(() => sessions.find(s => s.id === sid), [sessions, sid])

  const loadChatState = async (id) => {
    if (!id) return
    const st = await api(`/api/chat/state/${id}`)
    setLlms(st.llms || [])
    setLlmNo(st.llm_no || st.settings?.llm_no || 0)
  }

  const openSession = async (id, refreshList = true) => {
    const d = await api(`/api/chat/session/${id}`)
    setSid(d.id)
    setMessages(d.messages || [])
    setLlmNo(d.settings?.llm_no || 0)
    setErr('')
    setNotice('')
    setMenuOpen('')
    if (refreshList) setSessions(xs => xs.map(x => x.id === d.id ? { ...x, title: d.title, count: d.messages?.length || x.count, updated_at: d.updated_at || x.updated_at } : x))
    await loadChatState(d.id)
  }

  const loadSessions = async (prefer = sid) => {
    const d = await api('/api/chat/sessions')
    const list = d.sessions || []
    setSessions(list)
    const next = prefer || list[0]?.id || ''
    if (next) await openSession(next, false)
    return list
  }

  const newSession = async () => {
    const d = await api('/api/chat/session/new', { method:'POST', body:'{}' })
    setSessions(xs => [{ id:d.id, title:d.title, updated_at:d.updated_at, count:0 }, ...xs])
    setSid(d.id); setMessages([]); setPrompt(''); setErr(''); setNotice('已创建新对话'); setLlmNo(d.settings?.llm_no || 0)
    await loadChatState(d.id)
  }

  const deleteSession = async (id) => {
    if (!id || !confirm('删除此会话？此操作不可恢复。')) return
    await api(`/api/chat/session/${id}`, { method:'DELETE' })
    setSessions(xs => xs.filter(x => x.id !== id))
    setMenuOpen('')
    if (id === sid) { setSid(''); setMessages([]); setNotice('会话已删除') }
    setTimeout(() => loadSessions('').catch(()=>{}), 0)
  }

  const startRename = (s) => { setEditing(s.id); setDraftTitle(shortTitle(s)); setMenuOpen('') }
  const saveRename = async (id) => {
    const title = draftTitle.trim()
    if (!title) return
    const d = await api(`/api/chat/session/${id}`, { method:'PATCH', body: JSON.stringify({ title }) })
    setSessions(xs => xs.map(x => x.id === id ? { ...x, title:d.title, updated_at:d.updated_at } : x))
    setEditing(''); setDraftTitle(''); setNotice('会话已更名')
  }

  const saveModel = async (next) => {
    setLlmNo(next)
    if (!sid) return
    await api(`/api/chat/settings/${sid}`, { method:'POST', body: JSON.stringify({ llm_no: next }) })
    setNotice('模型已切换')
  }

  const send = async () => {
    const text = prompt.trim()
    if (!text || busy) return
    setBusy(true); setErr(''); setNotice('')
    let id = sid
    try {
      if (!id) {
        const d = await api('/api/chat/session/new', { method:'POST', body:'{}' })
        id = d.id; setSid(id); setSessions(xs => [{ id:d.id, title:d.title, updated_at:d.updated_at, count:0 }, ...xs])
      }
      const clientUserID = `u-${Date.now()}`
      setPrompt('')
      const optimistic = { id:clientUserID, role:'user', content:text, created_at:Math.floor(Date.now()/1000) }
      const pending = { id:`a-${Date.now()}`, role:'assistant', content:'', created_at:Math.floor(Date.now()/1000) }
      setMessages(xs => [...xs, optimistic, pending])
      const res = await fetch(`/api/chat/${id}`, { method:'POST', headers:{'Content-Type':'application/json'}, body: JSON.stringify({ prompt:text, settings:{ llm_no: llmNo }, client_user_id:clientUserID }) })
      if (!res.ok) throw new Error(await res.text())
      const reader = res.body.getReader(); const dec = new TextDecoder(); let buf = ''
      while (true) {
        const { value, done } = await reader.read()
        if (done) break
        buf += dec.decode(value, { stream:true })
        const lines = buf.split('\n'); buf = lines.pop() || ''
        for (const line of lines) {
          if (!line.trim()) continue
          const ev = JSON.parse(line)
          if (ev.type === 'user') setMessages(xs => xs.map(m => m.id === clientUserID ? ev.message : m))
          if (ev.type === 'delta' && typeof ev.delta === 'string') {
            setMessages(xs => xs.map(m => m.id === pending.id ? { ...m, content: (m.content || '') + ev.delta } : m))
          }
          if (ev.message && (ev.type === 'done' || ev.type === 'error')) setMessages(xs => xs.map(m => m.id === pending.id ? ev.message : m))
        }
      }
      await loadSessions(id)
    } catch (e) {
      setErr(e.message || String(e))
    } finally { setBusy(false) }
  }

  useEffect(() => { loadSessions().catch(e=>setErr(e.message)) }, [])
  useEffect(() => { endRef.current?.scrollIntoView({ behavior:'smooth', block:'end' }) }, [messages, busy])

  const activeModel = llms.find(x => x.index === llmNo) || llms[0]

  return <div className="oa-chat">
    <aside className={`oa-sidebar ${collapsed ? 'collapsed' : ''}`}>
      <div className="oa-side-head">
        <div className="oa-logo"><Bot size={18}/><span>GenericAgent</span></div>
        <button className="oa-icon-btn" onClick={()=>setCollapsed(true)} title="折叠"><Menu size={18}/></button>
      </div>
      <button className="oa-new-chat" onClick={newSession}><MessageSquarePlus size={16}/><span>新对话</span></button>
      <div className="oa-session-list">
        {sessions.map(s => <div key={s.id} className={`oa-session-row ${s.id===sid?'active':''}`}>
          {editing === s.id ? <div className="oa-rename">
            <input value={draftTitle} autoFocus onChange={e=>setDraftTitle(e.target.value)} onKeyDown={e=>{ if(e.key==='Enter') saveRename(s.id); if(e.key==='Escape') setEditing('') }}/>
            <button onClick={()=>saveRename(s.id)}><Check size={14}/></button><button onClick={()=>setEditing('')}><X size={14}/></button>
          </div> : <button className="oa-session" onClick={()=>openSession(s.id)}>
            <span>{shortTitle(s)}</span><small><Clock3 size={11}/>{fmtTime(s.updated_at) || '刚刚'} · {s.count || 0} 条</small>
          </button>}
          {editing !== s.id && <button className="oa-session-more" onClick={(e)=>{e.stopPropagation(); setMenuOpen(menuOpen === s.id ? '' : s.id)}}><MoreHorizontal size={16}/></button>}
          {menuOpen === s.id && <div className="oa-session-menu">
            <button onClick={()=>startRename(s)}><Edit3 size={14}/>重命名</button>
            <button className="danger" onClick={()=>deleteSession(s.id)}><Trash2 size={14}/>删除</button>
          </div>}
        </div>)}
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
        <div className="oa-title"><b>{current ? shortTitle(current) : '新对话'}</b><span>ChatGPT-style workspace for GenericAgent</span></div>
        <div className="oa-top-actions">
          <label className="oa-model-select"><span>{activeModel ? '模型' : '默认模型'}</span><select value={llmNo} onChange={e=>saveModel(Number(e.target.value))}>
            <option value={0}>自动 / 默认</option>{llms.map(m => <option key={m.index} value={m.index}>{modelLabel(m)}</option>)}
          </select><ChevronDown size={14}/></label>
          <button onClick={newSession}>新对话</button>
          <button disabled={!sid} onClick={()=>deleteSession(sid)}><Trash2 size={15}/>删除</button>
        </div>
      </header>

      {(err || notice) && <div className={`oa-banner ${err ? 'error' : ''}`}>{err || notice}</div>}

      <section className="oa-thread">
        {messages.length === 0 && <div className="oa-empty">
          <div className="oa-hero-badge"><Sparkles size={16}/>Agent cockpit</div>
          <h1>今天想让 GenericAgent 做什么？</h1>
          <p>支持 Markdown、代码块复制、模型切换、会话重命名与删除。</p>
          <div className="oa-prompts">{examples.map(([k, x]) => <button className="oa-prompt" key={x} onClick={()=>setPrompt(x)}><b>{k}</b><span>{x}</span></button>)}</div>
        </div>}
        {messages.map(m => <article key={m.id} className={`oa-message ${m.role} ${m.error?'error':''}`}>
          <div className="oa-avatar">{m.role === 'user' ? '你' : 'GA'}</div>
          <div className="oa-bubble">
            <div className="oa-meta"><b>{m.role === 'user' ? 'You' : 'GenericAgent'}</b>{m.created_at && <span>{fmtTime(m.created_at)}</span>}{m.content && <CopyButton text={m.content} compact />}</div>
            {m.role === 'assistant' ? <AssistantContent content={m.content} pending={busy && !m.content} /> : <MarkdownBlock text={m.content} />}
          </div>
        </article>)}
        <div ref={endRef}/>
      </section>

      <footer className="oa-composer-wrap">
        <div className="oa-composer">
          <textarea value={prompt} onChange={e=>setPrompt(e.target.value)} onKeyDown={e=>{ if(e.key==='Enter' && !e.shiftKey) { e.preventDefault(); send() } }} placeholder="向 GenericAgent 发送消息…" rows={1}/>
          <button className="oa-send" disabled={busy || !prompt.trim()} onClick={send}><Send size={17}/></button>
        </div>
        <p>Enter 发送 · Shift + Enter 换行 · 支持 Markdown 与代码块复制</p>
      </footer>
    </main>
  </div>
}
