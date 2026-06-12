import { memo, useCallback, useEffect, useLayoutEffect, useMemo, useRef, useState } from 'react'
import gsap from 'gsap'
import { useGSAP } from '@gsap/react'
import { Bot, Check, ChevronDown, ChevronLeft, Clock3, Copy, Edit3, FileImage, FileText, ImagePlus, Menu, MessageSquarePlus, MoreHorizontal, PanelRightOpen, Pin, RefreshCw, Send, Sparkles, Square, Trash2, Wrench, X } from 'lucide-react'
import { api, apiStream } from './lib/api'
import { confirmDanger } from './lib/danger'
import { JSON_TREE_CHILD_LIMIT, JSON_TREE_STRING_LIMIT, LIST_ITEM_LIMIT, LONG_TEXT_PREVIEW_CHARS, MARKDOWN_BLOCK_LIMIT, MARKDOWN_CHAR_LIMIT, MARKDOWN_LINE_LIMIT, parseAssistantContent, previewLongText, textRenderStats } from './lib/chatTextSafety'

gsap.registerPlugin(useGSAP)

const prefersReducedMotion = () => typeof window !== 'undefined' && window.matchMedia?.('(prefers-reduced-motion: reduce)').matches

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

const tokenizeInlineMarkdown = (text = '') => {
  const src = String(text || '')
  const tokens = []
  const re = /(`([^`]+)`)|(\*\*([^*]+)\*\*)|(\*([^*]+)\*)|(\[([^\]]+)\]\((https?:\/\/[^\s)]+)\))/g
  let last = 0, m
  while ((m = re.exec(src)) !== null) {
    if (m.index > last) tokens.push({ type:'text', text:src.slice(last, m.index) })
    if (m[2]) tokens.push({ type:'code', text:m[2] })
    else if (m[4]) tokens.push({ type:'strong', text:m[4] })
    else if (m[6]) tokens.push({ type:'em', text:m[6] })
    else if (m[8] && m[9]) tokens.push({ type:'link', text:m[8], href:m[9] })
    last = re.lastIndex
  }
  if (last < src.length) tokens.push({ type:'text', text:src.slice(last) })
  return tokens
}

function InlineMarkdown({ text = '' }) {
  return <>
    {tokenizeInlineMarkdown(text).map((t, i) => {
      if (t.type === 'code') return <code key={i}>{t.text}</code>
      if (t.type === 'strong') return <strong key={i}>{t.text}</strong>
      if (t.type === 'em') return <em key={i}>{t.text}</em>
      if (t.type === 'link') return <a key={i} href={t.href} target="_blank" rel="noreferrer">{t.text}</a>
      return <span key={i}>{t.text}</span>
    })}
  </>
}

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

function LongTextPreview({ text = '', stats }) {
  const s = stats || textRenderStats(text)
  const preview = useMemo(() => previewLongText(text), [text])
  return <div className="oa-long-text-preview">
    <div className="oa-long-text-head">
      <b>内容过大，已切换安全预览</b>
      <span>{s.chars.toLocaleString()} 字符 · {s.linesLabel} 行</span>
      <CopyButton text={text} compact />
    </div>
    <pre>{preview}</pre>
  </div>
}

function JsonTree({ data, name = 'root', depth = 0 }) {
  const [open, setOpen] = useState(depth < 2)
  const isArray = Array.isArray(data)
  const isObject = data && typeof data === 'object' && !isArray
  if (!isArray && !isObject) {
    const cls = data === null ? 'is-null' : typeof data === 'string' ? 'is-string' : typeof data === 'number' ? 'is-number' : typeof data === 'boolean' ? 'is-bool' : ''
    const raw = typeof data === 'string' ? data : JSON.stringify(data)
    const long = typeof raw === 'string' && raw.length > JSON_TREE_STRING_LIMIT
    const shown = long ? `${raw.slice(0, JSON_TREE_STRING_LIMIT)}… (${raw.length.toLocaleString()} chars)` : raw
    return <div className="oa-json-line" style={{ '--depth': depth }}><span className="oa-json-key">{name}:</span> <span className={`oa-json-value ${cls}`}>{typeof data === 'string' ? JSON.stringify(shown) : shown}</span></div>
  }
  const entries = isArray ? data.map((v, i) => [i, v]) : Object.entries(data)
  const shownEntries = entries.slice(0, JSON_TREE_CHILD_LIMIT)
  const hidden = entries.length - shownEntries.length
  const label = isArray ? `Array(${data.length})` : `Object(${entries.length})`
  return <div className="oa-json-node">
    <button type="button" className="oa-json-toggle" style={{ '--depth': depth }} onClick={()=>setOpen(v=>!v)}>
      <span className="oa-json-caret">{open ? '▾' : '▸'}</span><span className="oa-json-key">{name}</span><span className="oa-json-type">{label}</span>
    </button>
    {open && <div>
      {shownEntries.map(([k, v]) => <JsonTree key={String(k)} name={String(k)} data={v} depth={depth + 1} />)}
      {hidden > 0 && <div className="oa-json-line oa-json-more" style={{ '--depth': depth + 1 }}>… 已隐藏 {hidden.toLocaleString()} 项，复制原始 JSON 查看全部</div>}
    </div>}
  </div>
}

function isImageFile(f) {
  if (!f) return false
  const mime = String(f.type || f.mime || '')
  if (mime.startsWith('image/')) return true
  const ref = String(f.name || f.url || f.path || f.dataURL || '').split(/[?#]/)[0]
  return /\.(png|jpe?g|gif|webp|bmp|svg)$/i.test(ref)
}

function FileAttachment({ path }) {
  const clean = String(path || '').trim()
  const name = clean.split(/[\\/]/).filter(Boolean).pop() || clean || '文件'
  const isImage = /\.(png|jpe?g|gif|webp|bmp|svg)$/i.test(clean.split(/[?#]/)[0] || clean)
  const imageUrl = `/api/files/image?path=${encodeURIComponent(clean)}`
  const open = async (mode) => {
    if (!confirmDanger('chat-file-open', `使用系统桌面打开${mode === 'folder' ? '文件所在位置' : '文件'}：${clean}？`)) return
    try {
      await api('/api/files/open', { dangerous:true, method:'POST', body: JSON.stringify({ path: clean, mode }) })
    } catch (e) {
      alert(`打开失败：${e?.message || e}`)
    }
  }
  return <span className={`oa-file-card ${isImage ? 'oa-file-card-image' : ''}`}>
    {isImage ? <button type="button" className="oa-file-preview" onClick={() => open('file')} title="打开图片">
      <img src={imageUrl} alt={name} loading="lazy" onError={(e)=>{ e.currentTarget.style.display='none' }} />
    </button> : <span className="oa-file-icon"><FileText size={18}/></span>}
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
    if (m.index > last) nodes.push(<InlineMarkdown key={`t${n++}`} text={src.slice(last, m.index)} />)
    nodes.push(<FileAttachment key={`f${n++}`} path={m[1]} />)
    last = re.lastIndex
  }
  if (last < src.length) nodes.push(<InlineMarkdown key={`t${n++}`} text={src.slice(last)} />)
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

const MarkdownBlock = memo(function MarkdownBlock({ text = '', onAskReply }) {
  const stats = useMemo(() => textRenderStats(text), [text])
  const parts = useMemo(() => stats.tooLarge ? [] : normalizeToolParts(splitMarkdownParts(text)).slice(0, MARKDOWN_BLOCK_LIMIT), [text, stats.tooLarge])
  if (stats.tooLarge) return <div className="oa-md"><LongTextPreview text={text} stats={stats} /></div>
  return <div className="oa-md">
    {parts.map((p, idx) => p.type === 'code'
      ? <div className="oa-code-card" key={idx}>
          <div className="oa-code-head"><span>{p.lang || '代码'}</span><CopyButton text={p.text} compact /></div>
          <pre><code>{p.text}</code></pre>
        </div>
      : p.type === 'tool'
        ? <ToolCallBlock key={idx} call={p.call} onAskReply={onAskReply} />
        : <TextMarkdown key={idx} text={p.text} onAskReply={onAskReply}/>) }
    {parts.length >= MARKDOWN_BLOCK_LIMIT && <div className="oa-md-truncated">内容块过多，仅渲染前 {MARKDOWN_BLOCK_LIMIT} 块，可复制消息查看完整内容。</div>}
  </div>
})

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
  const source = String(raw || '').trim()
  const stripFence = (x = '') => String(x || '').trim().replace(/^```(?:json|text)?\s*/i, '').replace(/```$/i, '').trim()
  const choices = [stripFence(source)]
  const jsonLike = source.match(/\{[\s\S]*"(?:question|candidates)"[\s\S]*\}/)
  if (jsonLike) choices.unshift(stripFence(jsonLike[0]))
  for (const text of choices) {
    if (!text) continue
    try {
      const data = JSON.parse(text)
      const question = String(data?.question || data?.prompt || data?.message || '').trim()
      const opts = Array.isArray(data?.candidates) ? data.candidates.map(x => String(x || '').trim()).filter(Boolean) : []
      if (question || opts.length) return { question, candidates:opts, raw:text, structured:true }
    } catch {}
  }
  const text = stripFence(source)
  if (!text) return { question:'', candidates:[], raw:'', structured:false }
  const q = text.match(/"question"\s*:\s*"([\s\S]*?)"/i)?.[1]
  const question = q ? q.replace(/\\n/g, '\n').replace(/\\"/g, '"') : text
  return { question, candidates:[], raw:text, structured:false }
}

const getAskUserPayload = (call = {}) => {
  const fromResult = parseAskUserPayload(call.result)
  if (fromResult.structured) return fromResult
  const fromArgs = parseAskUserPayload(call.args)
  if (fromArgs.structured || fromArgs.question || fromArgs.candidates.length) return fromArgs
  return fromResult
}

function AskUserPanel({ call, onReply }) {
  const ask = getAskUserPayload(call)
  const hasStructured = Boolean(ask.question || ask.candidates.length)
  return <div className="oa-ask-panel">
    <div className="oa-ask-banner">
      <span className="oa-ask-avatar">?</span>
      <div><b>{'\u9700\u8981\u7528\u6237\u786e\u8ba4'}</b><p>{'\u667a\u80fd\u4f53\u6b63\u5728\u7b49\u5f85\u4f60\u7684\u9009\u62e9\u6216\u8865\u5145\u4fe1\u606f'}</p></div>
    </div>
    {hasStructured ? <div className="oa-ask-body">
      {ask.question && <div className="oa-ask-question"><span>{'\u95ee\u9898'}</span><p>{ask.question}</p></div>}
      {ask.candidates.length > 0 && <div className="oa-ask-options"><span>{'\u5feb\u6377\u56de\u590d'}</span><div>{ask.candidates.map((x,i)=><button type="button" key={`${x}-${i}`} onClick={(e)=>{e.stopPropagation(); onReply?.(x)}} title={'\u70b9\u51fb\u586b\u5165\u8f93\u5165\u6846'}>{x}</button>)}</div></div>}
    </div> : call.args && <div className="oa-tool-args"><span>{'\ud83d\udcac question'}</span><pre>{call.args}</pre></div>}
    {call.result && <div className="oa-tool-result oa-ask-result"><span>{'\ud83d\udce4 result'}</span><pre>{call.result}</pre></div>}
  </div>
}

function ToolCallBlock({ call, onAskReply }) {
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
    {open && (isAskUser ? <AskUserPanel call={call} onReply={onAskReply} /> : <>
      {call.args && <div className="oa-tool-args"><span>{'\ud83d\udce5 args'}</span><pre>{call.args}</pre></div>}
      {call.result && <div className="oa-tool-result"><span>{'\ud83d\udce4 result'}</span><pre>{call.result}</pre></div>}
    </>)}
  </div>
}

const splitTableRow = (line = '') => {
  let src = String(line || '').trim()
  if (src.startsWith('|')) src = src.slice(1)
  if (src.endsWith('|') && !src.endsWith('\\|')) src = src.slice(0, -1)
  const cells = []
  let cur = ''
  let escaped = false
  for (const ch of src) {
    if (escaped) { cur += ch; escaped = false; continue }
    if (ch === '\\') { escaped = true; cur += ch; continue }
    if (ch === '|') { cells.push(cur.trim().replace(/\\\|/g, '|')); cur = ''; continue }
    cur += ch
  }
  cells.push(cur.trim().replace(/\\\|/g, '|'))
  return cells
}

const parseTableAlign = (cell = '') => {
  const s = String(cell || '').trim()
  if (!/^:?-{3,}:?$/.test(s)) return null
  if (s.startsWith(':') && s.endsWith(':')) return 'center'
  if (s.endsWith(':')) return 'right'
  return 'left'
}

const parseMarkdownTable = (block = '') => {
  const lines = String(block || '').split('\n').filter(x => x.trim())
  if (lines.length < 2 || !lines[0].includes('|') || !lines[1].includes('|')) return null
  const head = splitTableRow(lines[0])
  const aligns = splitTableRow(lines[1]).map(parseTableAlign)
  if (!head.length || aligns.some(x => x === null) || aligns.length < head.length) return null
  const rows = lines.slice(2).map(splitTableRow).filter(cells => cells.length > 0)
  return { head, aligns, rows }
}

function renderMarkdownTable(table, key) {
  return <div key={key} className="oa-table-wrap">
    <table className="oa-md-table">
      <thead><tr>{table.head.map((cell, i) => <th key={i} style={{ textAlign: table.aligns[i] || 'left' }}><InlineRichText text={cell} /></th>)}</tr></thead>
      <tbody>{table.rows.map((row, r) => <tr key={r}>{table.head.map((_, c) => <td key={c} style={{ textAlign: table.aligns[c] || 'left' }}><InlineRichText text={row[c] || ''} /></td>)}</tr>)}</tbody>
    </table>
  </div>
}

function renderListBlock(lines, i, ordered) {
  const itemRe = ordered ? /^\s*(\d+)[.)]\s+/ : /^\s*[-*+]\s+/
  const Tag = ordered ? 'ol' : 'ul'
  const shownLines = lines.slice(0, LIST_ITEM_LIMIT)
  const hidden = Math.max(0, lines.length - shownLines.length)
  const firstNumber = ordered ? Number(String(lines[0] || '').match(itemRe)?.[1] || 1) : undefined
  const props = ordered ? { start: firstNumber } : {}
  return <Tag key={i} className={`oa-list ${ordered ? 'oa-list-ordered' : 'oa-list-unordered'}`} {...props}>
    {shownLines.map((x,j)=>{
      const itemNumber = ordered ? Number(String(x || '').match(itemRe)?.[1] || firstNumber + j) : undefined
      const liProps = ordered ? { value: itemNumber } : {}
      return <li key={j} {...liProps}><InlineRichText text={x.replace(itemRe, '')} /></li>
    })}
    {hidden > 0 && <li className="oa-md-truncated">… 已隐藏 {hidden.toLocaleString()} 个列表项</li>}
  </Tag>
}

function renderPlainTextBlock(b, key) {
  const trimmed = String(b || '').trim()
  if (!trimmed) return null
  const lines = trimmed.split('\n')
  const orderedOnly = lines.every(x => /^\s*\d+[.)]\s+/.test(x))
  const unorderedOnly = lines.every(x => /^\s*[-*+]\s+/.test(x))
  if (orderedOnly) return renderListBlock(lines, key, true)
  if (unorderedOnly) return renderListBlock(lines, key, false)
  if (/^#{1,3}\s+/.test(trimmed)) {
    const level = Math.min(3, trimmed.match(/^#+/)[0].length)
    const body = trimmed.replace(/^#{1,3}\s+/, '')
    const Tag = `h${level + 2}`
    return <Tag key={key}><InlineRichText text={body} /></Tag>
  }
  return <p key={key}><InlineRichText text={trimmed} /></p>
}

function renderTextBlock(b, i) {
  const table = parseMarkdownTable(b)
  if (table) return renderMarkdownTable(table, i)

  const lines = String(b || '').split('\n')
  const nodes = []
  let paragraph = []
  let list = []
  let listOrdered = null
  let seq = 0
  const flushParagraph = () => {
    if (!paragraph.length) return
    const node = renderPlainTextBlock(paragraph.join('\n'), `${i}-p-${seq++}`)
    if (node) nodes.push(node)
    paragraph = []
  }
  const flushList = () => {
    if (!list.length) return
    nodes.push(renderListBlock(list, `${i}-l-${seq++}`, listOrdered === true))
    list = []
    listOrdered = null
  }

  for (const line of lines) {
    const isOrdered = /^\s*\d+[.)]\s+/.test(line)
    const isUnordered = /^\s*[-*+]\s+/.test(line)
    if (isOrdered || isUnordered) {
      flushParagraph()
      const ordered = isOrdered
      if (list.length && listOrdered !== ordered) flushList()
      listOrdered = ordered
      list.push(line)
    } else {
      flushList()
      paragraph.push(line)
    }
  }
  flushParagraph()
  flushList()
  if (nodes.length === 1) return nodes[0]
  if (nodes.length > 1) return <div key={i} className="oa-md-fragment">{nodes}</div>
  return null
}

function TextMarkdown({ text = '', onAskReply }) {
  const allBlocks = String(text || '').replace(/\r\n/g, '\n').split(/\n{2,}/)
  const blocks = allBlocks.slice(0, MARKDOWN_BLOCK_LIMIT)
  const hiddenBlocks = Math.max(0, allBlocks.length - blocks.length)
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
      nodes.push(<ToolCallBlock key={i} call={toolCall} onAskReply={onAskReply} />)
      i = j - 1
      continue
    }
    const standaloneArgs = parseToolArgsBlock(blocks[i])
    if (standaloneArgs !== null) {
      nodes.push(<ToolCallBlock key={i} call={{ name: 'unknown', args: standaloneArgs }} onAskReply={onAskReply} />)
      continue
    }
    nodes.push(renderTextBlock(blocks[i], i))
  }
  if (hiddenBlocks > 0) nodes.push(<div key="__hidden_blocks" className="oa-md-truncated">… 已隐藏 {hiddenBlocks.toLocaleString()} 个内容块，可复制消息查看完整内容。</div>)
  return <>{nodes}</>
}

const AssistantContent = memo(function AssistantContent({ content, pending, onAskReply }) {
  const [openTurns, setOpenTurns] = useState({})
  const stats = useMemo(() => textRenderStats(content), [content])
  const parsed = useMemo(() => parseAssistantContent(content), [content])
  const hasTurnSplit = parsed.runs.length > 0
  if (!content && pending) return <div className="oa-content oa-thinking">正在思考…</div>
  if (content && stats.tooLarge && !hasTurnSplit) return <div className="oa-content"><LongTextPreview text={content} stats={stats} /></div>
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
          {open && (r.body ? <MarkdownBlock text={r.body} onAskReply={onAskReply} /> : <p className="oa-turn-empty">该轮暂无详细输出</p>)}
        </section>
      })}
      {lastRun && <section className="oa-turn-current" key={`last-${lastRun.turn}`}>
        <div className="oa-turn-current-head"><span>Turn {lastRun.turn}</span><b>{lastRun.title || '正在执行'}</b><em>{pending ? '实时输出中' : '最新一轮'}</em></div>
        {lastRun.body ? <MarkdownBlock text={lastRun.body} onAskReply={onAskReply} /> : <p className="oa-turn-empty">正在等待该轮输出…</p>}
      </section>}
    </div>}
    {(parsed.body || !parsed.runs.length) && <div className={parsed.runs.length ? 'oa-final-answer' : ''}>
      {parsed.runs.length && <div className="oa-final-label">返回给用户</div>}
      <MarkdownBlock text={parsed.body || content || ''} onAskReply={onAskReply} />
    </div>}
  </div>
})

// 用户消息正文里会被自动追加附件清单（前端乐观态的“[图片附件]”或后端保存后的“[附件已保存]\n[FILE:...]”）。
// 这些附件已经由 oa-message-images 单独渲染，若再经 InlineRichText 渲染 [FILE:] 会导致图片重复显示，故在展示前剥离该尾块。
const stripUserAttachmentBlock = (content = '') => {
  const src = String(content || '')
  const markers = ['\n[图片附件]', '\n[附件已保存]', '[图片附件]', '[附件已保存]']
  let cut = -1
  for (const marker of markers) {
    const i = src.lastIndexOf(marker)
    if (i >= 0 && (cut < 0 || i < cut)) cut = i
  }
  return cut >= 0 ? src.slice(0, cut).trimEnd() : src
}

const ChatMessage = memo(function ChatMessage({ message: m, pending, onAskReply }) {
  const userText = m.role === 'user' ? stripUserAttachmentBlock(m.content) : m.content
  return <article className={`oa-message ${m.role} ${m.error?'error':''}`}>
    <div className="oa-avatar">{m.role === 'user' ? '你' : 'GA'}</div>
    <div className="oa-bubble">
      <div className="oa-meta"><b>{m.role === 'user' ? 'You' : 'GenericAgent'}</b>{m.created_at && <span>{fmtTime(m.created_at)}</span>}{m.content && <CopyButton text={m.role === 'user' ? userText : m.content} compact />}</div>
      {Array.isArray(m.files) && m.files.some(isImageFile) && <div className="oa-message-images">{m.files.filter(isImageFile).map((f, i) => <img key={f.name || i} src={f.dataURL || f.url} alt={f.name || 'image'} />)}</div>}
      {m.role === 'assistant' ? <AssistantContent content={m.content} pending={pending && !m.content} onAskReply={onAskReply} /> : (userText && <MarkdownBlock text={userText} />)}
    </div>
  </article>
})

const MessageList = memo(function MessageList({ messages, isCurrentRunning, onAskReply }) {
  return <>
    {messages.flatMap((m, i) => {
      const day = timelineKey(m.created_at)
      const prevDay = i > 0 ? timelineKey(messages[i - 1]?.created_at) : ''
      const nodes = []
      if (i === 0 || day !== prevDay) nodes.push(<div key={`tl-${day}-${i}`} className="oa-timeline"><span>{fmtTimelineDate(m.created_at)}</span></div>)
      nodes.push(<ChatMessage key={m.id} message={m} pending={isCurrentRunning && i === messages.length - 1} onAskReply={onAskReply} />)
      return nodes
    })}
  </>
})
export default function ChatApp() {
  const [sessions, setSessions] = useState([])
  const [sid, setSid] = useState('')
  const [messages, setMessages] = useState([])
  const [rawHistory, setRawHistory] = useState([])
  const [historyInfo, setHistoryInfo] = useState([])
  const [workingState, setWorkingState] = useState(null)
  const [contextOpen, setContextOpen] = useState(false)
  const [prompt, setPrompt] = useState('')
  const [busy, setBusy] = useState(false)
  const [streamingSid, setStreamingSid] = useState('')
  const [err, setErr] = useState('')
  const [collapsed, setCollapsed] = useState(false)
  const [notice, setNotice] = useState('')
  const [llms, setLlms] = useState([])
  const [llmNo, setLlmNo] = useState(0)
  const [toolsMode, setToolsMode] = useState('official')
  const [menuOpen, setMenuOpen] = useState('')
  const [menuPos, setMenuPos] = useState(null)
  const [editing, setEditing] = useState('')
  const [draftTitle, setDraftTitle] = useState('')
  const [attachments, setAttachments] = useState([])
  const [queuedMessages, setQueuedMessages] = useState([])
  const [queueEditingId, setQueueEditingId] = useState('')
  const [queueDraft, setQueueDraft] = useState('')
  const [dragging, setDragging] = useState(false)
  const [autoFollow, setAutoFollow] = useState(true)
  const [showFollow, setShowFollow] = useState(false)
  const [toolsMenuOpen, setToolsMenuOpen] = useState(false)
  const toolsMenuRef = useRef(null)
  const threadRef = useRef(null)
  const endRef = useRef(null)
  const fileRef = useRef(null)
  const promptRef = useRef(null)
  const streamAbortRef = useRef(null)
  const runSeqRef = useRef(0)
  const openSeqRef = useRef(0)
  const activeSidRef = useRef('')
  const scrollModeRef = useRef('auto')
  const queuedRef = useRef([])
  const chatScope = useRef(null)
  // Auto-grow composer textarea to fit content (clamped), reset to single row when cleared.
  const COMPOSER_MAX_H = 160
  useLayoutEffect(() => {
    const el = promptRef.current
    if (!el) return
    el.style.height = 'auto'
    const next = Math.min(el.scrollHeight, COMPOSER_MAX_H)
    el.style.height = next + 'px'
    el.style.overflowY = el.scrollHeight > COMPOSER_MAX_H ? 'auto' : 'hidden'
  }, [prompt])
  const current = useMemo(() => sessions.find(s => s.id === sid), [sessions, sid])
  useEffect(() => { activeSidRef.current = sid }, [sid])

  const isActiveSession = (sessionId) => !sessionId || activeSidRef.current === sessionId

  const applyStreamEvent = (ev, pendingId, clientUserID = '', sessionId = '') => {
    if (!isActiveSession(sessionId)) return
    if (ev.type === 'user' && ev.message) {
      setMessages(xs => {
        if (!isActiveSession(sessionId)) return xs
        return clientUserID
          ? xs.map(m => m.id === clientUserID ? ev.message : m)
          : (xs.some(m => m.id === ev.message.id) ? xs : [...xs, ev.message])
      })
    }
    if (ev.message && (ev.type === 'done' || ev.type === 'error')) {
      setMessages(xs => isActiveSession(sessionId) ? xs.map(m => m.id === pendingId ? ev.message : m) : xs)
    }
  }

  const createStreamBatcher = (pendingId, sessionId = '') => {
    let pendingDelta = ''
    let raf = 0
    const flush = () => {
      raf = 0
      if (!pendingDelta) return
      if (!isActiveSession(sessionId)) { pendingDelta = ''; return }
      const chunk = pendingDelta
      pendingDelta = ''
      setMessages(xs => isActiveSession(sessionId) ? xs.map(m => m.id === pendingId ? { ...m, content: (m.content || '') + chunk } : m) : xs)
    }
    const schedule = () => {
      if (raf) return
      raf = window.requestAnimationFrame ? window.requestAnimationFrame(flush) : window.setTimeout(flush, 16)
    }
    return {
      push(delta) {
        if (!delta) return
        pendingDelta += delta
        schedule()
      },
      flushNow() {
        if (raf) {
          if (window.cancelAnimationFrame) window.cancelAnimationFrame(raf)
          else window.clearTimeout(raf)
          raf = 0
        }
        flush()
      },
    }
  }

  const readStream = async (res, pendingId, clientUserID = '', sessionId = '') => {
    const reader = res.body.getReader(); const dec = new TextDecoder(); let buf = ''
    const batcher = createStreamBatcher(pendingId, sessionId)
    try {
      while (true) {
        const { value, done } = await reader.read()
        if (done) break
        buf += dec.decode(value, { stream:true })
        const lines = buf.split('\n'); buf = lines.pop() || ''
        for (const line of lines) {
          if (!line.trim()) continue
          if (!isActiveSession(sessionId)) return
          const ev = JSON.parse(line)
          if (ev.type === 'delta' && typeof ev.delta === 'string') {
            batcher.push(ev.delta)
          } else {
            batcher.flushNow()
            applyStreamEvent(ev, pendingId, clientUserID, sessionId)
          }
        }
      }
      if (buf.trim() && isActiveSession(sessionId)) {
        const ev = JSON.parse(buf)
        if (ev.type === 'delta' && typeof ev.delta === 'string') batcher.push(ev.delta)
        else { batcher.flushNow(); applyStreamEvent(ev, pendingId, clientUserID, sessionId) }
      }
    } finally {
      batcher.flushNow()
    }
  }

  const cancelRun = async (id = sid) => {
    if (!id) return
    try {
      streamAbortRef.current?.abort?.()
      await api(`/api/chat/cancel/${id}`, { method:'POST', body:'{}' })
      setMessages(xs => xs.map(m => (m.role === 'assistant' && !m.content) ? { ...m, content:'已中止。', error:true } : m))
      setSessions(xs => xs.map(s => s.id === id ? { ...s, running:false } : s))
      setNotice('已中止当前执行')
    } catch (e) { setErr(e.message || String(e)) }
    finally { setBusy(false); setStreamingSid(''); if (id) loadSessions(id).catch(()=>{}) }
  }

  const attachRunningStream = async (id) => {
    if (!id) return
    streamAbortRef.current?.abort?.()
    const ctrl = new AbortController()
    streamAbortRef.current = ctrl
    const pendingId = `resume-${Date.now()}`
    setBusy(true); setStreamingSid(id); setAutoFollow(true); setShowFollow(false)
    setMessages(xs => xs.some(m => m.role === 'assistant' && !m.content) ? xs : [...xs, { id:pendingId, role:'assistant', content:'', created_at:Math.floor(Date.now()/1000) }])
    try {
      const res = await fetch(`/api/chat/stream/${id}`, { signal: ctrl.signal })
      if (res.status === 204) return
      if (!res.ok) throw new Error(await res.text())
      await readStream(res, pendingId, '', id)
      if (isActiveSession(id)) await loadSessions(id)
    } catch (e) {
      if (e.name !== 'AbortError' && isActiveSession(id)) setErr(e.message || String(e))
    } finally {
      if (streamAbortRef.current === ctrl) {
        streamAbortRef.current = null
        if (isActiveSession(id)) { setBusy(false); setStreamingSid('') }
      }
    }
  }

  const loadChatState = async (id = '', openToken = openSeqRef.current) => {
    const st = await api(id ? `/api/chat/state/${id}` : '/api/chat/state')
    if (openToken !== openSeqRef.current || !isActiveSession(id)) return
    const nextLlms = st.llms || []
    const nextNo = st.settings?.llm_no ?? st.llm_no ?? nextLlms[0]?.index ?? 0
    const nextToolsMode = st.settings?.tools_mode === 'fixed' ? 'fixed' : 'official'
    setLlms(nextLlms)
    setLlmNo(nextLlms.some(m => m.index === nextNo) ? nextNo : (nextLlms[0]?.index ?? 0))
    setToolsMode(nextToolsMode)
    if (id && st.running) {
      attachRunningStream(id)
    } else if (id && streamingSid && streamingSid !== id) {
      streamAbortRef.current?.abort?.()
      streamAbortRef.current = null
      setBusy(false)
      setStreamingSid('')
    }
  }

  const openSession = async (id, refreshList = true) => {
    const openToken = ++openSeqRef.current
    activeSidRef.current = id
    streamAbortRef.current?.abort?.()
    streamAbortRef.current = null
    scrollModeRef.current = 'auto'
    setSid(id)
    setBusy(false)
    setStreamingSid('')
    setAutoFollow(true)
    setShowFollow(false)
    const d = await api(`/api/chat/session/${id}`)
    if (openToken !== openSeqRef.current || activeSidRef.current !== id) return
    activeSidRef.current = d.id
    setSid(d.id)
    scrollModeRef.current = 'auto'
    setMessages(d.messages || [])
    setRawHistory(Array.isArray(d.raw_history) ? d.raw_history : [])
    setHistoryInfo(Array.isArray(d.history_info) ? d.history_info : [])
    setWorkingState(d.working || null)
    setLlmNo(d.settings?.llm_no || 0)
    setToolsMode(d.settings?.tools_mode === 'fixed' ? 'fixed' : 'official')
    setErr('')
    setNotice('')
    setMenuOpen('')
    setMenuPos(null)
    if (refreshList) setSessions(xs => xs.map(x => x.id === d.id ? { ...x, title: d.title, count: d.messages?.length || x.count, updated_at: d.updated_at || x.updated_at } : x))
    await loadChatState(d.id, openToken)
  }

  const loadSessions = async (prefer = sid, options = {}) => {
    const { open = false } = options
    const d = await api('/api/chat/sessions')
    const list = d.sessions || []
    setSessions(list)
    if (open) {
      const next = prefer || list[0]?.id || ''
      if (next) await openSession(next, false)
      else await loadChatState('', openSeqRef.current)
    } else if (!prefer && !sid) {
      await loadChatState('', openSeqRef.current)
    }
    return list
  }

  const newSession = async () => {
    const openToken = ++openSeqRef.current
    streamAbortRef.current?.abort?.()
    streamAbortRef.current = null
    const d = await api('/api/chat/session/new', { method:'POST', body:'{}' })
    if (openToken !== openSeqRef.current) return
    activeSidRef.current = d.id
    scrollModeRef.current = 'auto'
    setSessions(xs => [{ id:d.id, title:d.title, updated_at:d.updated_at, count:0 }, ...xs])
    setSid(d.id); setMessages([]); setRawHistory([]); setHistoryInfo([]); setWorkingState(null); setContextOpen(false); setPrompt(''); setErr(''); setNotice('已创建新对话'); setBusy(false); setStreamingSid(''); setAutoFollow(false); setShowFollow(false); setLlmNo(d.settings?.llm_no || 0); setToolsMode(d.settings?.tools_mode === 'fixed' ? 'fixed' : 'official')
    await loadChatState(d.id, openToken)
  }

  const deleteSession = async (id) => {
    if (!id || !confirmDanger('chat-session-delete', '删除此会话？此操作不可恢复。')) return
    await api(`/api/chat/session/${id}`, { method:'DELETE' })
    setSessions(xs => xs.filter(x => x.id !== id))
    setMenuOpen('')
    setMenuPos(null)
    if (id === sid) {
      ++openSeqRef.current
      activeSidRef.current = ''
      streamAbortRef.current?.abort?.()
      streamAbortRef.current = null
      scrollModeRef.current = 'auto'
      setSid(''); setMessages([]); setBusy(false); setStreamingSid(''); setAutoFollow(true); setShowFollow(false); setNotice('会话已删除')
    }
    setTimeout(() => loadSessions('', { open:true }).catch(()=>{}), 0)
  }

  const startRename = (s) => { setEditing(s.id); setDraftTitle(shortTitle(s)); setMenuOpen(''); setMenuPos(null) }
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
    await api(`/api/chat/settings/${sid}`, { method:'POST', body: JSON.stringify({ llm_no: next, tools_mode: toolsMode }) })
    setNotice('模型已切换')
  }

  const setToolsModeTo = async (next) => {
    if (next === toolsMode) { setToolsMenuOpen(false); return }
    const prev = toolsMode
    setToolsMode(next)
    setToolsMenuOpen(false)
    if (!sid) return
    try {
      await api(`/api/chat/settings/${sid}`, { method:'POST', body: JSON.stringify({ llm_no: llmNo, tools_mode: next }) })
      setNotice(next === 'fixed' ? '已设为自动注入：每次发消息都带上工具' : '已设为官方行为：会话开始按 GA 默认方式注入工具，需要时可点“立即注入一次”')
    } catch (e) {
      setToolsMode(prev)
      setErr(e.message || String(e))
    }
  }

  const reinjectTools = async () => {
    if (!sid) return
    if (isCurrentRunning) { setNotice('当前正在执行，完成后再重注入 Tools'); return }
    try {
      const d = await api(`/api/chat/reinject-tools/${sid}`, { method:'POST' })
      setNotice(d?.message || 'Tools 已重注入')
    } catch (e) {
      setErr(e.message || String(e))
    }
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
  const syncQueue = (next) => { queuedRef.current = next; setQueuedMessages(next) }
  const popQueued = () => {
    const [first, ...rest] = queuedRef.current
    syncQueue(rest)
    return first
  }
  const enqueueMessage = (item) => {
    const next = [...queuedRef.current, { ...item, id:`q-${Date.now()}-${Math.random().toString(16).slice(2)}`, queuedAt:Date.now() }]
    syncQueue(next)
    setNotice(`已加入队列（${next.length} 条）。点击“引导”可中止当前回复并立即发送。`)
  }
  const removeQueued = (id) => {
    syncQueue(queuedRef.current.filter(x => x.id !== id))
    if (queueEditingId === id) { setQueueEditingId(''); setQueueDraft('') }
  }
  const editQueued = (id) => {
    const item = queuedRef.current.find(x => x.id === id)
    if (!item) return
    setQueueEditingId(id)
    setQueueDraft(item.text || '')
    setNotice('正在编辑队列消息')
  }
  const cancelQueueEdit = () => {
    setQueueEditingId('')
    setQueueDraft('')
    setNotice('')
  }
  const saveQueueEdit = (id) => {
    const text = queueDraft.trim()
    const item = queuedRef.current.find(x => x.id === id)
    if (!item) return
    if (!text && !(item.files || []).length) { setErr('队列消息不能为空'); return }
    syncQueue(queuedRef.current.map(x => x.id === id ? { ...x, text } : x))
    setQueueEditingId('')
    setQueueDraft('')
    setErr('')
    setNotice('队列消息已更新')
  }
  const guideQueuedItem = (id) => {
    const item = queuedRef.current.find(x => x.id === id)
    if (!item) return
    syncQueue(queuedRef.current.filter(x => x.id !== id))
    guideQueued(item)
  }
  const onPaste = (e) => {
    const imgs = Array.from(e.clipboardData?.files || []).filter(f => f.type?.startsWith('image/'))
    if (imgs.length) {
      e.preventDefault()
      addImageFiles(imgs)
    }
  }
  const onDropImages = (e) => {
    e.preventDefault(); setDragging(false)
    addImageFiles(e.dataTransfer?.files)
  }


  const fillAskReply = useCallback((text) => {
    const value = String(text || '')
    setPrompt(value)
    setNotice('已填入快捷回复，确认后可发送')
    const focusPrompt = () => {
      const el = promptRef.current
      if (!el) return
      el.focus()
      const len = value.length
      el.setSelectionRange?.(len, len)
    }
    requestAnimationFrame(focusPrompt)
    setTimeout(focusPrompt, 0)
  }, [])

  const runSend = async (item = {}) => {
    const text = String(item.text || '').trim()
    const files = (item.files || []).map(({ name, type, dataURL }) => ({ name, type, dataURL }))
    if (!text && !files.length) return
    const runToken = ++runSeqRef.current
    const openToken = openSeqRef.current
    const ctrl = new AbortController()
    streamAbortRef.current?.abort?.()
    streamAbortRef.current = ctrl
    setBusy(true); setStreamingSid(sid || 'new'); setErr(''); setNotice('')
    let id = sid
    try {
      if (!id) {
        const d = await api('/api/chat/session/new', { method:'POST', body:'{}' })
        if (runToken !== runSeqRef.current || openToken !== openSeqRef.current) return
        id = d.id
        activeSidRef.current = id
        scrollModeRef.current = 'auto'
        setSid(id); setStreamingSid(id); setSessions(xs => [{ id:d.id, title:d.title, updated_at:d.updated_at, count:0 }, ...xs])
      } else if (!isActiveSession(id)) {
        return
      }
      const clientUserID = `u-${Date.now()}`
      setStreamingSid(id)
      setSessions(xs => xs.map(s => s.id === id ? { ...s, running:true } : s))
      setAutoFollow(true); setShowFollow(false)
      const fileNote = files.length ? `\n\n[图片附件]\n${files.map(f => `- ${f.name}`).join('\n')}` : ''
      const optimistic = { id:clientUserID, role:'user', content:(text || '请分析这张图片') + fileNote, files, created_at:Math.floor(Date.now()/1000) }
      const pending = { id:`a-${Date.now()}`, role:'assistant', content:'', created_at:Math.floor(Date.now()/1000) }
      setRawHistory([]); setHistoryInfo([]); setWorkingState(null)
      if (!isActiveSession(id)) return
      activeSidRef.current = id
      setMessages(xs => isActiveSession(id) ? [...xs, optimistic, pending] : xs)
      const res = await fetch(`/api/chat/${id}`, { method:'POST', headers:{'Content-Type':'application/json'}, signal: ctrl.signal, body: JSON.stringify({ prompt:text || '请分析这张图片', files, settings:{ llm_no: item.llmNo ?? llmNo, tools_mode: item.toolsMode || toolsMode }, client_user_id:clientUserID }) })
      if (!res.ok) throw new Error(await res.text())
      await readStream(res, pending.id, clientUserID, id)
    } catch (e) {
      if (runToken === runSeqRef.current && openToken === openSeqRef.current && e?.name !== 'AbortError' && isActiveSession(id)) setErr(e.message || String(e))
    } finally {
      if (runToken !== runSeqRef.current || openToken !== openSeqRef.current || !isActiveSession(id)) return
      if (id) {
        await loadSessions(id).catch(()=>{})
        await openSession(id, false).catch(()=>{})
      }
      const next = popQueued()
      if (next) {
        setNotice(`继续发送队列消息（剩余 ${Math.max(queuedRef.current.length, 0)} 条）`)
        setTimeout(() => runSend(next), 0)
      } else {
        setBusy(false)
        setStreamingSid('')
      }
    }
  }

  const send = async () => {
    const text = prompt.trim()
    const files = attachments.map(({ name, type, dataURL }) => ({ name, type, dataURL }))
    if (text === '/new' && !files.length) {
      setPrompt('')
      if (busy) {
        setNotice('当前正在执行，完成后可使用 /new 创建新对话')
        return
      }
      await newSession()
      return
    }
    if (!text && !files.length) return
    const item = { text, files, llmNo, toolsMode }
    setPrompt(''); setAttachments([])
    if (busy) {
      enqueueMessage(item)
      return
    }
    await runSend(item)
  }

  const guideQueued = async (item = null) => {
    const next = item || popQueued()
    if (!next) return
    const id = sid
    const wasRunning = busy && streamingSid === sid
    ++runSeqRef.current
    try {
      if (wasRunning) {
        streamAbortRef.current?.abort?.()
        if (id) await api(`/api/chat/cancel/${id}`, { method:'POST', body:'{}' })
        setMessages(xs => xs.map((m, idx) => (idx === xs.length - 1 && m.role === 'assistant' && !m.content) ? { ...m, content:'已中止，改为执行引导消息。', error:true } : m))
      }
    } catch (e) {
      setErr(e.message || String(e))
    } finally {
      setBusy(false)
      setStreamingSid('')
      setNotice('已引导：中止当前回复并发送队列消息')
      setTimeout(() => runSend(next), 0)
    }
  }

  useEffect(() => { loadSessions('', { open:true }).catch(e=>setErr(e.message)); return () => streamAbortRef.current?.abort?.() }, [])

  useEffect(() => {
    if (!toolsMenuOpen) return
    const onDown = (e) => { if (!toolsMenuRef.current?.contains(e.target)) setToolsMenuOpen(false) }
    const onKey = (e) => { if (e.key === 'Escape') setToolsMenuOpen(false) }
    document.addEventListener('mousedown', onDown)
    document.addEventListener('keydown', onKey)
    return () => { document.removeEventListener('mousedown', onDown); document.removeEventListener('keydown', onKey) }
  }, [toolsMenuOpen])

  const scrollToThreadEnd = (behavior = 'auto') => endRef.current?.scrollIntoView({ behavior, block:'end' })
  const resumeFollow = () => {
    setAutoFollow(true)
    setShowFollow(false)
    scrollToThreadEnd('auto')
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
      const behavior = scrollModeRef.current || 'auto'
      scrollModeRef.current = 'auto'
      scrollToThreadEnd(behavior)
    } else if (!isNearBottom(threadRef.current)) {
      setShowFollow(true)
    }
  }, [messages, busy, autoFollow])

  useGSAP(() => {
    if (prefersReducedMotion()) return
    const q = gsap.utils.selector(chatScope)
    gsap.from(q('.oa-sidebar'), { x: -24, autoAlpha: 0, duration: 0.52, ease: 'power3.out', clearProps: 'transform,opacity,visibility' })
    gsap.from(q('.oa-topbar, .oa-thread, .oa-composer-wrap'), { y: 18, autoAlpha: 0, duration: 0.5, stagger: 0.08, ease: 'power3.out', clearProps: 'transform,opacity,visibility' })
  }, { scope: chatScope })

  useGSAP(() => {
    if (prefersReducedMotion() || !messages.length) return
    const lastMessage = chatScope.current?.querySelector('.oa-message:last-of-type, .oa-turn:last-of-type')
    if (lastMessage) gsap.from(lastMessage, { y: 14, autoAlpha: 0, duration: 0.32, ease: 'power2.out' })
  }, { scope: chatScope, dependencies: [messages.length] })

  const activeModel = llms.find(x => x.index === llmNo) || llms[0]
  const selectedModelNo = activeModel?.index ?? llmNo
  const isCurrentRunning = busy && streamingSid === sid
  const isFixedToolsMode = toolsMode === 'fixed'
  const contextJson = useMemo(() => JSON.stringify({ raw_history: rawHistory || [], history_info: historyInfo || [], working: workingState || {} }, null, 2), [rawHistory, historyInfo, workingState])
  const copyContext = async () => {
    try {
      await navigator.clipboard.writeText(contextJson)
      setNotice('模型上下文 JSON 已复制')
    } catch {
      setErr('复制失败，请手动选择 JSON')
    }
  }

  return <div ref={chatScope} className={`oa-chat ${collapsed ? 'is-collapsed' : ''}`}>
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
            <span className="oa-session-title" title={shortTitle(s)}>{s.running && <i className="oa-session-running-dot" aria-hidden="true"/>}<b>{shortTitle(s)}</b></span>
            <small><Clock3 size={11}/>{fmtTime(s.updated_at) || '刚刚'} · {s.count || 0} 条{s.running && <em className="oa-session-running-label">运行中</em>}</small>
          </button>}
          {editing !== s.id && <button className={`oa-session-more ${menuOpen === s.id ? 'is-open' : ''}`} onClick={(e)=>{
            e.stopPropagation()
            if (menuOpen === s.id) { setMenuOpen(''); setMenuPos(null); return }
            const r = e.currentTarget.getBoundingClientRect()
            setMenuPos({ top: Math.max(8, r.top - 78), left: Math.max(8, r.right - 136) })
            setMenuOpen(s.id)
          }} aria-label="会话操作"><MoreHorizontal size={16}/></button>}
        </div>)}
        {!sessions.length && <div className="oa-empty-list">暂无历史会话</div>}
      </div>
      {menuOpen && menuPos && (() => {
        const s = sessions.find(x => x.id === menuOpen)
        if (!s) return null
        return <div className="oa-session-menu" style={{ top: menuPos.top, left: menuPos.left }} onClick={e=>e.stopPropagation()}>
          <button onClick={()=>startRename(s)}><Edit3 size={14}/>重命名</button>
          <button className="danger" onClick={()=>deleteSession(s.id)}><Trash2 size={14}/>删除</button>
        </div>
      })()}
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
        <button className={`oa-context-btn ${contextOpen ? 'is-open' : ''}`} type="button" onClick={()=>setContextOpen(v=>!v)} disabled={!sid} title="查看发给模型的 raw_history">
          <PanelRightOpen size={16}/>上下文<span>{rawHistory?.length || 0}</span>
        </button>
      </header>

      {contextOpen && <aside className="oa-context-drawer" aria-label="模型上下文">
        <div className="oa-context-head">
          <div><b>模型上下文</b><span>agent.llmclient.backend.history 完成后的快照</span></div>
          <div className="oa-context-actions"><button type="button" onClick={copyContext}>复制 JSON</button><button type="button" onClick={()=>setContextOpen(false)} aria-label="关闭上下文"><X size={15}/></button></div>
        </div>
        <div className="oa-context-json-tree"><JsonTree data={{ raw_history: rawHistory || [], history_info: historyInfo || [], working: workingState || {} }} /></div>
        <details className="oa-context-raw"><summary>原始 JSON</summary><pre className="oa-context-raw-json">{contextJson}</pre></details>
      </aside>}

      <div className="oa-banner-slot" aria-live="polite">
        {(err || notice) && <div className={`oa-banner ${err ? 'error' : ''}`}>{err || notice}</div>}
      </div>

      <section className="oa-thread" ref={threadRef} onScroll={updateFollowFromScroll} onWheel={e=>{ if (e.deltaY < 0) breakFollow() }} onTouchMove={breakFollow}>
        {messages.length === 0 && <div className="oa-empty">
          <h1>今天想让 GenericAgent 做什么？</h1>
          <p>支持 Markdown、代码块复制、图片输入、模型切换、会话重命名与删除。</p>
        </div>}
        <MessageList messages={messages} isCurrentRunning={isCurrentRunning} onAskReply={fillAskReply} />
        {showFollow && <div className="oa-follow-row"><button className="oa-follow-btn" type="button" onClick={resumeFollow}><ChevronDown size={16}/>继续跟随</button></div>}
        <div ref={endRef}/>
      </section>

      <footer className="oa-composer-wrap">
        {queuedMessages.length > 0 && <div className="oa-queue-dock" aria-label="待发送队列">
          {queuedMessages.map((q, i) => {
            const isEditingQueue = queueEditingId === q.id
            return <div key={q.id} className={`oa-queued-item ${isEditingQueue ? 'is-editing' : ''}`}>
              <div className="oa-queue-content" title={isEditingQueue ? '' : (q.text || '请分析这张图片')}>
                {isEditingQueue ? <textarea className="oa-queue-edit-input" value={queueDraft} autoFocus rows={2} onChange={e=>setQueueDraft(e.target.value)} onKeyDown={e=>{ if(e.key==='Enter' && (e.ctrlKey || e.metaKey)) saveQueueEdit(q.id); if(e.key==='Escape') cancelQueueEdit() }} /> : <>
                  <b>{q.text || '请分析这张图片'}</b>
                  {q.files?.length ? <em>{q.files.length} 张图片</em> : null}
                </>}
              </div>
              <div className="oa-queue-actions">
                <span className="oa-queue-index">消息{i + 1}</span>
                {isEditingQueue ? <>
                  <button className="oa-queue-action" type="button" onClick={()=>saveQueueEdit(q.id)} title="保存队列消息" aria-label="保存队列消息"><Check size={14}/></button>
                  <button className="oa-queue-action" type="button" onClick={cancelQueueEdit} title="取消编辑" aria-label="取消编辑"><X size={14}/></button>
                </> : <>
                  <button className="oa-guide-btn" type="button" onClick={()=>guideQueuedItem(q.id)} disabled={!isCurrentRunning} title={isCurrentRunning ? `暂停当前输出，立即发送消息${i + 1}` : 'AI 回复时可引导'}><Sparkles size={14}/>引导</button>
                  <button className="oa-queue-action" type="button" onClick={()=>removeQueued(q.id)} title="删除这条队列消息" aria-label="删除这条队列消息"><Trash2 size={14}/></button>
                  <button className="oa-queue-action" type="button" onClick={()=>editQueued(q.id)} title="编辑这条队列消息" aria-label="编辑这条队列消息"><Edit3 size={14}/></button>
                </>}
              </div>
            </div>
          })}
        </div>}
        <div className={`oa-composer ${dragging ? 'is-dragging' : ''}`} onDragOver={e=>{e.preventDefault(); setDragging(true)}} onDragLeave={()=>setDragging(false)} onDrop={onDropImages}>
          <input ref={fileRef} type="file" accept="image/*" multiple hidden onChange={e=>{ addImageFiles(e.target.files); e.target.value='' }} />
          {attachments.length > 0 && <div className="oa-attach-preview">
            {attachments.map(a => <div className="oa-attach-thumb" key={a.id}>
              <img src={a.dataURL} alt={a.name}/><span><FileImage size={12}/>{a.name}</span><button type="button" onClick={()=>removeAttachment(a.id)}><X size={12}/></button>
            </div>)}
          </div>}
          <textarea ref={promptRef} value={prompt} onPaste={onPaste} onChange={e=>setPrompt(e.target.value)} onKeyDown={e=>{ if(e.key==='Enter' && !e.shiftKey) { e.preventDefault(); send() } }} placeholder="向 GenericAgent 发送消息，可粘贴/拖拽图片…" rows={1}/>
          <div className="oa-composer-bar">
            <button className="oa-attach-btn" type="button" onClick={()=>fileRef.current?.click()} title="添加图片"><ImagePlus size={17}/><span>图片</span></button>
            <div className="oa-tools-menu" ref={toolsMenuRef}>
              <button className={`oa-tools-trigger ${toolsMenuOpen ? 'is-open' : ''}`} type="button" disabled={!sid} onClick={()=>setToolsMenuOpen(o=>!o)} aria-haspopup="menu" aria-expanded={toolsMenuOpen} title="工具注入设置">
                <Wrench size={16}/><span>工具</span>{isFixedToolsMode && <span className="oa-tools-state">自动</span>}<ChevronDown size={14}/>
              </button>
              {toolsMenuOpen && (
                <div className="oa-tools-pop" role="menu">
                  <div className="oa-tools-pop-head">工具注入方式</div>
                  <button className={`oa-tools-opt ${!isFixedToolsMode ? 'is-active' : ''}`} type="button" role="menuitemradio" aria-checked={!isFixedToolsMode} onClick={()=>setToolsModeTo('official')}>
                    <Wrench size={16}/>
                    <span className="oa-tools-opt-text"><b>官方行为<span className="oa-tools-tag">默认</span></b><small>会话开始按 GA 默认方式注入工具，需要时再点“立即注入一次”</small></span>
                    {!isFixedToolsMode && <Check size={16}/>}
                  </button>
                  <button className={`oa-tools-opt ${isFixedToolsMode ? 'is-active' : ''}`} type="button" role="menuitemradio" aria-checked={isFixedToolsMode} onClick={()=>setToolsModeTo('fixed')}>
                    <Pin size={16}/>
                    <span className="oa-tools-opt-text"><b>自动注入</b><small>每次发消息都自动带上工具</small></span>
                    {isFixedToolsMode && <Check size={16}/>}
                  </button>
                  {!isFixedToolsMode && (
                    <>
                      <div className="oa-tools-pop-sep"/>
                      <button className="oa-tools-act" type="button" disabled={!sid || isCurrentRunning} onClick={()=>{ setToolsMenuOpen(false); reinjectTools() }}>
                        <RefreshCw size={15}/><span>立即注入一次</span>
                      </button>
                    </>
                  )}
                </div>
              )}
            </div>
            <label className="oa-model-select oa-composer-model"><span>{activeModel ? '模型' : '模型不可用'}</span><select value={selectedModelNo} disabled={!llms.length} onChange={e=>saveModel(Number(e.target.value))}>
              {llms.length ? llms.map(m => <option key={m.index} value={m.index}>{modelLabel(m)}</option>) : <option value={0}>未发现模型</option>}
            </select><ChevronDown size={14}/></label>
            <button className="oa-send" type="button" disabled={!prompt.trim() && !attachments.length} onClick={send} title={isCurrentRunning ? '加入发送队列' : '发送'} aria-label={isCurrentRunning ? '加入发送队列' : '发送'}><Send size={17}/></button>
            {isCurrentRunning && <button className="oa-stop" type="button" onClick={()=>cancelRun(sid)} title="停止生成" aria-label="停止生成"><Square size={14}/></button>}
          </div>
        </div>
        <p>Enter 发送 · Shift + Enter 换行 · 回复中发送会排队 · 工具：{isFixedToolsMode ? '每次自动注入' : '官方默认'}</p>
      </footer>
    </main>
  </div>
}
