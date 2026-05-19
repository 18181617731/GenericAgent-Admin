import { useEffect, useMemo, useRef, useState } from 'react'
import { Bot, Check, ChevronDown, ChevronLeft, Clock3, Copy, Edit3, FileImage, FileText, ImagePlus, Menu, MessageSquarePlus, MoreHorizontal, RefreshCw, Send, Sparkles, Square, Trash2, X } from 'lucide-react'

const api = async (url, options = {}) => {
  const res = await fetch(url, { headers: { 'Content-Type': 'application/json', ...(options.headers || {}) }, ...options })
  if (!res.ok) throw new Error(await res.text())
  return res.json()
}

const fmtTime = (v) => {
  if (!v) return ''
  try { return new Date(v * 1000).toLocaleString() } catch { return '' }
}
const fmtTimelineDate = (v) => {
  if (!v) return '今天'
  try {
    const d = new Date(v * 1000)
    const now = new Date()
    const day = new Date(d.getFullYear(), d.getMonth(), d.getDate()).getTime()
    const today = new Date(now.getFullYear(), now.getMonth(), now.getDate()).getTime()
    const diff = Math.round((today - day) / 86400000)
    if (diff === 0) return '今天'
    if (diff === 1) return '昨天'
    return d.toLocaleDateString(undefined, { year:'numeric', month:'long', day:'numeric' })
  } catch { return '' }
}
const timelineKey = (v) => {
  if (!v) return 'today'
  try {
    const d = new Date(v * 1000)
    return `${d.getFullYear()}-${d.getMonth()+1}-${d.getDate()}`
  } catch { return 'unknown' }
}
const isNearBottom = (el, gap = 96) => !el || (el.scrollHeight - el.scrollTop - el.clientHeight) <= gap
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

function FileAttachment({ path }) {
  const clean = String(path || '').trim()
  const name = clean.split(/[\\/]/).filter(Boolean).pop() || clean || '文件'
  const open = async (mode) => {
    try {
      await api('/api/files/open', { method:'POST', body: JSON.stringify({ path: clean, mode }) })
    } catch (e) {
      alert(`打开失败：${e?.message || e}`)
    }
  }
  return <span className="oa-file-card">
    <span className="oa-file-icon"><FileText size={18}/></span>
    <span className="oa-file-meta"><b>{name}</b><em>{clean}</em></span>
    <span className="oa-file-actions">
      <button type="button" onClick={() => open('file')}>打开</button>
      <button type="button" onClick={() => open('folder')}>位置</button>
      <CopyButton text={clean} compact />
    </span>
  </span>
}

function InlineRichText({ text = '' }) {
  const src = String(text || '')
  const re = /\[FILE:([^\]]+)\]/g
  const nodes = []
  let last = 0, m, n = 0
  while ((m = re.exec(src)) !== null) {
    if (m.index > last) nodes.push(<span key={`t${n++}`} dangerouslySetInnerHTML={{__html:inlineMarkdown(src.slice(last, m.index))}} />)
    nodes.push(<FileAttachment key={`f${n++}`} path={m[1]} />)
    last = re.lastIndex
  }
  if (last < src.length) nodes.push(<span key={`t${n++}`} dangerouslySetInnerHTML={{__html:inlineMarkdown(src.slice(last))}} />)
  return <>{nodes}</>
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

const parseAskUserPayload = (raw = '') => {
  const text = String(raw || '').trim().replace(/^```(?:json|text)?\s*/i, '').replace(/```$/i, '').trim()
  if (!text) return { question:'', candidates:[], raw:'' }
  try {
    const data = JSON.parse(text)
    const question = String(data?.question || data?.prompt || data?.message || '').trim()
    const candidates = Array.isArray(data?.candidates) ? data.candidates.map(x => String(x || '').trim()).filter(Boolean) : []
    return { question, candidates, raw:text }
  } catch {
    const q = text.match(/"question"\s*:\s*"([\s\S]*?)"/i)?.[1]
    return { question: q ? q.replace(/\\n/g, '\n') : text, candidates:[], raw:text }
  }
}

function AskUserPanel({ call }) {
  const ask = getAskUserPayload(call)
  const hasStructured = Boolean(ask.question || ask.candidates.length)
  return <div className="oa-ask-panel">
    <div className="oa-ask-banner">
      <span className="oa-ask-avatar">?</span>
      <div><b>{'\u9700\u8981\u7528\u6237\u786e\u8ba4'}</b><p>{'\u667a\u80fd\u4f53\u6b63\u5728\u7b49\u5f85\u4f60\u7684\u9009\u62e9\u6216\u8865\u5145\u4fe1\u606f'}</p></div>
    </div>
    {hasStructured ? <div className="oa-ask-body">
      {ask.question && <div className="oa-ask-question"><span>{'\u95ee\u9898'}</span><p>{ask.question}</p></div>}
      {ask.candidates.length > 0 && <div className="oa-ask-options"><span>{'\u5feb\u6377\u56de\u590d'}</span><div>{ask.candidates.map((x,i)=><button type="button" key={`${x}-${i}`} onClick={(e)=>{e.stopPropagation(); navigator.clipboard?.writeText(x)}} title={'\u70b9\u51fb\u590d\u5236\u8be5\u56de\u590d'}>{x}</button>)}</div></div>}
    </div> : call.args && <div className="oa-tool-args"><span>{'\ud83d\udcac question'}</span><pre>{call.args}</pre></div>}
    {call.result && <div className="oa-tool-result oa-ask-result"><span>{'\ud83d\udce4 result'}</span><pre>{call.result}</pre></div>}
  </div>
}

function ToolCallBlock({ call }) {
  const toolName = String(call.name || 'unknown').trim()
  const isAskUser = /(?:^|[._-])ask_user$/i.test(toolName)
  const [open, setOpen] = useState(isAskUser)
  const resultStatus = String(call.result || '').match(/\[Status\]\s*([^\n]+)/i)?.[1]?.trim()
  const askPayload = isAskUser ? getAskUserPayload(call) : null
  const askSummary = askPayload?.question || '\u7b49\u5f85\u7528\u6237\u786e\u8ba4'
  return <div className={`oa-tool-call ${isAskUser ? 'oa-tool-ask-user' : ''} ${open ? 'open' : 'collapsed'}`}>
    <button className="oa-tool-head" type="button" onClick={() => setOpen(v => !v)} aria-expanded={open}>
      <span className="oa-tool-icon">{isAskUser ? '\u2753' : '\ud83d\udee0\ufe0f'}</span><span>{isAskUser ? 'Ask user' : 'Tool'}</span><b>{toolName}</b>
      {isAskUser && <strong className="oa-ask-headline">{askSummary}</strong>}
      {resultStatus && <em>{resultStatus}</em>}
      {isAskUser && !resultStatus && <em>{askPayload?.candidates?.length ? `${askPayload.candidates.length} \u4e2a\u9009\u9879` : '\u7b49\u5f85\u56de\u590d'}</em>}
      <ChevronDown size={15} className="oa-tool-chevron" />
    </button>
    {open && (isAskUser ? <AskUserPanel call={call} /> : <>
      {call.args && <div className="oa-tool-args"><span>{'\ud83d\udce5 args'}</span><pre>{call.args}</pre></div>}
      {call.result && <div className="oa-tool-result"><span>{'\ud83d\udce4 result'}</span><pre>{call.result}</pre></div>}
    </>)}
  </div>
}

function renderTextBlock(b, i) {
  const lines = b.split('\n')
  if (lines.every(x => /^\s*([-*]|\d+\.)\s+/.test(x)) && lines.length > 1) {
    return <ul key={i} className="oa-list">{lines.map((x,j)=><li key={j}><InlineRichText text={x.replace(/^\s*([-*]|\d+\.)\s+/, '')} /></li>)}</ul>
  }
  if (/^#{1,3}\s+/.test(b.trim())) {
    const level = Math.min(3, b.trim().match(/^#+/)[0].length)
    const body = b.trim().replace(/^#{1,3}\s+/, '')
    const Tag = `h${level + 2}`
    return <Tag key={i}><InlineRichText text={body} /></Tag>
  }
  return <p key={i}><InlineRichText text={b} /></p>
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
const TURN_HEADER_RE = /(?:^|\n)\s*(?:\*\*)?\s*LLM Running\s*\(Turn\s+(\d+)(?:\))?\s*(?:\.\.\.)?\s*(?:\*\*)?/gi

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
      <div className="oa-turn-stack-head">
        <span className="oa-run-dot"/>
        <span>执行过程</span>
        <b>{parsed.runs.length}</b>
        <em>{pending ? '正在生成' : '已完成'}</em>
      </div>
      {boxedRuns.map((r, i) => {
        const open = isTurnOpen(r, i)
        return <section className={`oa-turn-card ${open ? 'open' : 'collapsed'}`} key={`${r.turn}-${i}`}>
          <button className="oa-turn-toggle" type="button" onClick={() => toggleTurn(r, i)} aria-expanded={open} title={open ? '收起该轮详情' : '展开该轮详情'}>
            <span className="oa-turn-pill">Turn {r.turn}</span>
            <b>{r.title || '执行步骤'}</b>
            <em>{open ? '收起详情' : '展开详情'}</em>
            <ChevronDown size={15}/>
          </button>
          {open && (r.body ? <MarkdownBlock text={r.body} /> : <p className="oa-turn-empty">该轮暂无详细输出</p>)}
        </section>
      })}
      {lastRun && <section className="oa-turn-current" key={`last-${lastRun.turn}`}>
        <div className="oa-turn-current-head"><span>Turn {lastRun.turn}</span><b>{lastRun.title || '正在执行'}</b><em>{pending ? '实时输出中' : '最新一轮'}</em></div>
        {lastRun.body ? <MarkdownBlock text={lastRun.body} /> : <p className="oa-turn-empty">正在等待该轮输出…</p>}
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
  const [attachments, setAttachments] = useState([])
  const [dragging, setDragging] = useState(false)
  const [autoFollow, setAutoFollow] = useState(true)
  const [showFollow, setShowFollow] = useState(false)
  const threadRef = useRef(null)
  const endRef = useRef(null)
  const fileRef = useRef(null)
  const streamAbortRef = useRef(null)
  const current = useMemo(() => sessions.find(s => s.id === sid), [sessions, sid])

  const applyStreamEvent = (ev, pendingId, clientUserID = '') => {
    if (ev.type === 'user' && ev.message) {
      setMessages(xs => clientUserID
        ? xs.map(m => m.id === clientUserID ? ev.message : m)
        : (xs.some(m => m.id === ev.message.id) ? xs : [...xs, ev.message]))
    }
    if (ev.type === 'delta' && typeof ev.delta === 'string') {
      setMessages(xs => xs.map(m => m.id === pendingId ? { ...m, content: (m.content || '') + ev.delta } : m))
    }
    if (ev.message && (ev.type === 'done' || ev.type === 'error')) {
      setMessages(xs => xs.map(m => m.id === pendingId ? ev.message : m))
    }
  }

  const readStream = async (res, pendingId, clientUserID = '') => {
    const reader = res.body.getReader(); const dec = new TextDecoder(); let buf = ''
    while (true) {
      const { value, done } = await reader.read()
      if (done) break
      buf += dec.decode(value, { stream:true })
      const lines = buf.split('\n'); buf = lines.pop() || ''
      for (const line of lines) {
        if (!line.trim()) continue
        applyStreamEvent(JSON.parse(line), pendingId, clientUserID)
      }
    }
  }

  const cancelRun = async (id = sid) => {
    if (!id || !busy) return
    try {
      streamAbortRef.current?.abort?.()
      await api(`/api/chat/cancel/${id}`, { method:'POST', body:'{}' })
      setMessages(xs => xs.map(m => (m.role === 'assistant' && !m.content) ? { ...m, content:'已中止。', error:true } : m))
      setSessions(xs => xs.map(s => s.id === id ? { ...s, running:false } : s))
      setNotice('已中止当前执行')
    } catch (e) { setErr(e.message || String(e)) }
    finally { setBusy(false); if (id) loadSessions(id).catch(()=>{}) }
  }

  const attachRunningStream = async (id) => {
    if (!id) return
    streamAbortRef.current?.abort?.()
    const ctrl = new AbortController()
    streamAbortRef.current = ctrl
    const pendingId = `resume-${Date.now()}`
    setBusy(true); setAutoFollow(true); setShowFollow(false)
    setMessages(xs => xs.some(m => m.role === 'assistant' && !m.content) ? xs : [...xs, { id:pendingId, role:'assistant', content:'', created_at:Math.floor(Date.now()/1000) }])
    try {
      const res = await fetch(`/api/chat/stream/${id}`, { signal: ctrl.signal })
      if (res.status === 204) return
      if (!res.ok) throw new Error(await res.text())
      await readStream(res, pendingId)
      await loadSessions(id)
    } catch (e) {
      if (e.name !== 'AbortError') setErr(e.message || String(e))
    } finally {
      if (streamAbortRef.current === ctrl) { streamAbortRef.current = null; setBusy(false) }
    }
  }

  const loadChatState = async (id) => {
    if (!id) return
    const st = await api(`/api/chat/state/${id}`)
    const nextLlms = st.llms || []
    const nextNo = st.settings?.llm_no ?? st.llm_no ?? nextLlms[0]?.index ?? 0
    setLlms(nextLlms)
    setLlmNo(nextLlms.some(m => m.index === nextNo) ? nextNo : (nextLlms[0]?.index ?? 0))
    if (st.running) attachRunningStream(id)
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

  const addImageFiles = async (fileList) => {
    const files = Array.from(fileList || []).filter(f => f && f.type?.startsWith('image/'))
    if (!files.length) return
    const tooLarge = files.find(f => f.size > 8 * 1024 * 1024)
    if (tooLarge) { setErr(`图片过大：${tooLarge.name}，单张限制 8MB`); return }
    const readOne = (file) => new Promise((resolve, reject) => {
      const reader = new FileReader()
      reader.onload = () => resolve({ id:`img-${Date.now()}-${Math.random().toString(16).slice(2)}`, name:file.name || `pasted-${Date.now()}.png`, type:file.type || 'image/png', size:file.size || 0, dataURL:String(reader.result || '') })
      reader.onerror = () => reject(reader.error || new Error('读取图片失败'))
      reader.readAsDataURL(file)
    })
    try {
      const next = await Promise.all(files.map(readOne))
      setAttachments(xs => [...xs, ...next].slice(0, 8))
      setErr('')
    } catch (e) { setErr(e.message || String(e)) }
  }

  const removeAttachment = (id) => setAttachments(xs => xs.filter(x => x.id !== id))
  const onPaste = (e) => {
    const imgs = Array.from(e.clipboardData?.files || []).filter(f => f.type?.startsWith('image/'))
    if (imgs.length) addImageFiles(imgs)
  }
  const onDropImages = (e) => {
    e.preventDefault(); setDragging(false)
    addImageFiles(e.dataTransfer?.files)
  }


  const send = async () => {
    const text = prompt.trim()
    const files = attachments.map(({ name, type, dataURL }) => ({ name, type, dataURL }))
    if ((!text && !files.length) || busy) return
    setBusy(true); setErr(''); setNotice('')
    let id = sid
    try {
      if (!id) {
        const d = await api('/api/chat/session/new', { method:'POST', body:'{}' })
        id = d.id; setSid(id); setSessions(xs => [{ id:d.id, title:d.title, updated_at:d.updated_at, count:0 }, ...xs])
      }
      const clientUserID = `u-${Date.now()}`
      setSessions(xs => xs.map(s => s.id === id ? { ...s, running:true } : s))
      setPrompt(''); setAttachments([]); setAutoFollow(true); setShowFollow(false)
      const fileNote = files.length ? `\n\n[图片附件]\n${files.map(f => `- ${f.name}`).join('\n')}` : ''
      const optimistic = { id:clientUserID, role:'user', content:(text || '请分析这张图片') + fileNote, files, created_at:Math.floor(Date.now()/1000) }
      const pending = { id:`a-${Date.now()}`, role:'assistant', content:'', created_at:Math.floor(Date.now()/1000) }
      setMessages(xs => [...xs, optimistic, pending])
      const res = await fetch(`/api/chat/${id}`, { method:'POST', headers:{'Content-Type':'application/json'}, body: JSON.stringify({ prompt:text || '请分析这张图片', files, settings:{ llm_no: llmNo }, client_user_id:clientUserID }) })
      if (!res.ok) throw new Error(await res.text())
      await readStream(res, pending.id, clientUserID)
    } catch (e) {
      setErr(e.message || String(e))
    } finally {
      if (id) await loadSessions(id).catch(()=>{})
      setBusy(false)
    }
  }

  useEffect(() => { loadSessions().catch(e=>setErr(e.message)); return () => streamAbortRef.current?.abort?.() }, [])

  const scrollToThreadEnd = (behavior = 'smooth') => endRef.current?.scrollIntoView({ behavior, block:'end' })
  const resumeFollow = () => {
    setAutoFollow(true)
    setShowFollow(false)
    scrollToThreadEnd('smooth')
  }
  const updateFollowFromScroll = () => {
    const near = isNearBottom(threadRef.current)
    setAutoFollow(near)
    setShowFollow(!near)
  }
  const breakFollow = () => {
    if (autoFollow && !isNearBottom(threadRef.current, 12)) {
      setAutoFollow(false)
      setShowFollow(true)
    }
  }

  useEffect(() => {
    if (autoFollow) {
      scrollToThreadEnd('smooth')
    } else if (!isNearBottom(threadRef.current)) {
      setShowFollow(true)
    }
  }, [messages, busy, autoFollow])

  const activeModel = llms.find(x => x.index === llmNo) || llms[0]
  const selectedModelNo = activeModel?.index ?? llmNo

  return <div className={`oa-chat ${collapsed ? 'is-collapsed' : ''}`}>
    <aside className={`oa-sidebar ${collapsed ? 'collapsed' : ''}`}>
      <div className="oa-side-head">
        <div className="oa-logo"><Bot size={18}/><span>GenericAgent</span></div>
        <button className="oa-icon-btn" onClick={()=>setCollapsed(true)} title="折叠"><Menu size={18}/></button>
      </div>
      <button className="oa-new-chat" onClick={newSession}><MessageSquarePlus size={16}/><span>新对话</span></button>
      <div className="oa-session-list">
        {sessions.map(s => <div key={s.id} className={`oa-session-row ${s.id===sid?'active':''} ${s.running?'is-running':''}`}>
          {editing === s.id ? <div className="oa-rename">
            <input value={draftTitle} autoFocus onChange={e=>setDraftTitle(e.target.value)} onKeyDown={e=>{ if(e.key==='Enter') saveRename(s.id); if(e.key==='Escape') setEditing('') }}/>
            <button onClick={()=>saveRename(s.id)}><Check size={14}/></button><button onClick={()=>setEditing('')}><X size={14}/></button>
          </div> : <button className="oa-session" onClick={()=>openSession(s.id)} title={shortTitle(s)}>
            <span title={shortTitle(s)}>{s.running && <i className="oa-session-running-dot" aria-hidden="true"/>}{shortTitle(s)}</span><small><Clock3 size={11}/>{fmtTime(s.updated_at) || '刚刚'} · {s.count || 0} 条{s.running && <em className="oa-session-running-label">运行中</em>}</small>
          </button>}
          {editing !== s.id && <button className={`oa-session-more ${menuOpen === s.id ? 'is-open' : ''}`} onClick={(e)=>{e.stopPropagation(); setMenuOpen(menuOpen === s.id ? '' : s.id)}} aria-label="会话操作"><MoreHorizontal size={16}/></button>}
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
        {collapsed && <div className="oa-collapsed-actions">
          <button className="oa-icon-btn oa-sidebar-toggle" onClick={()=>setCollapsed(false)} title="展开侧栏" aria-label="展开侧栏"><Menu size={18}/></button>
          <button className="oa-icon-btn oa-collapsed-new" onClick={newSession} title="新对话" aria-label="新对话"><MessageSquarePlus size={18}/></button>
        </div>}
        <div className="oa-title"><b>{current ? shortTitle(current) : '新对话'}</b><span>ChatGPT-style workspace for GenericAgent</span></div>
      </header>

      {(err || notice) && <div className={`oa-banner ${err ? 'error' : ''}`}>{err || notice}</div>}

      <section className="oa-thread" ref={threadRef} onScroll={updateFollowFromScroll} onWheel={e=>{ if (e.deltaY < 0) breakFollow() }} onTouchMove={breakFollow}>
        {messages.length === 0 && <div className="oa-empty">
          <div className="oa-hero-badge"><Sparkles size={16}/>Agent cockpit</div>
          <h1>今天想让 GenericAgent 做什么？</h1>
          <p>支持 Markdown、代码块复制、模型切换、会话重命名与删除。</p>
          <div className="oa-prompts">{examples.map(([k, x]) => <button className="oa-prompt" key={x} onClick={()=>setPrompt(x)}><b>{k}</b><span>{x}</span></button>)}</div>
        </div>}
        {messages.flatMap((m, i) => {
          const day = timelineKey(m.created_at)
          const prevDay = i > 0 ? timelineKey(messages[i - 1]?.created_at) : ''
          const nodes = []
          if (i === 0 || day !== prevDay) nodes.push(<div key={`tl-${day}-${i}`} className="oa-timeline"><span>{fmtTimelineDate(m.created_at)}</span></div>)
          nodes.push(<article key={m.id} className={`oa-message ${m.role} ${m.error?'error':''}`}>
            <div className="oa-avatar">{m.role === 'user' ? '你' : 'GA'}</div>
            <div className="oa-bubble">
              <div className="oa-meta"><b>{m.role === 'user' ? 'You' : 'GenericAgent'}</b>{m.created_at && <span>{fmtTime(m.created_at)}</span>}{m.content && <CopyButton text={m.content} compact />}</div>
              {Array.isArray(m.files) && m.files.some(f => String(f.type || '').startsWith('image/')) && <div className="oa-message-images">{m.files.filter(f => String(f.type || '').startsWith('image/')).map((f, i) => <img key={f.name || i} src={f.dataURL || f.url} alt={f.name || 'image'} />)}</div>}
              {m.role === 'assistant' ? <AssistantContent content={m.content} pending={busy && !m.content} /> : <MarkdownBlock text={m.content} />}
            </div>
          </article>)
          return nodes
        })}
        <div ref={endRef}/>
      </section>
      {showFollow && <button className="oa-follow-btn" type="button" onClick={resumeFollow}><ChevronDown size={16}/>继续跟随</button>}

      <footer className="oa-composer-wrap">
        <div className={`oa-composer ${dragging ? 'is-dragging' : ''}`} onDragOver={e=>{e.preventDefault(); setDragging(true)}} onDragLeave={()=>setDragging(false)} onDrop={onDropImages}>
          <input ref={fileRef} type="file" accept="image/*" multiple hidden onChange={e=>{ addImageFiles(e.target.files); e.target.value='' }} />
          {attachments.length > 0 && <div className="oa-attach-preview">
            {attachments.map(a => <div className="oa-attach-thumb" key={a.id}>
              <img src={a.dataURL} alt={a.name}/><span><FileImage size={12}/>{a.name}</span><button type="button" onClick={()=>removeAttachment(a.id)}><X size={12}/></button>
            </div>)}
          </div>}
          <textarea value={prompt} onPaste={onPaste} onChange={e=>setPrompt(e.target.value)} onKeyDown={e=>{ if(e.key==='Enter' && !e.shiftKey) { e.preventDefault(); send() } }} placeholder="向 GenericAgent 发送消息，可粘贴/拖拽图片…" rows={1}/>
          <div className="oa-composer-bar">
            <button className="oa-attach-btn" type="button" onClick={()=>fileRef.current?.click()} title="添加图片"><ImagePlus size={17}/><span>图片</span></button>
            <label className="oa-model-select oa-composer-model"><span>{activeModel ? '模型' : '模型不可用'}</span><select value={selectedModelNo} disabled={!llms.length} onChange={e=>saveModel(Number(e.target.value))}>
              {llms.length ? llms.map(m => <option key={m.index} value={m.index}>{modelLabel(m)}</option>) : <option value={0}>未发现模型</option>}
            </select><ChevronDown size={14}/></label>
            {busy ? <button className="oa-stop" type="button" onClick={()=>cancelRun(sid)} title="\u505c\u6b62\u751f\u6210"><Square size={14}/></button> : <button className="oa-send" disabled={!prompt.trim() && !attachments.length} onClick={send}><Send size={17}/></button>}
          </div>
        </div>
        <p>Enter 发送 · Shift + Enter 换行 · 支持 Markdown、代码块复制、图片粘贴/拖拽与模型切换</p>
      </footer>
    </main>
  </div>
}
