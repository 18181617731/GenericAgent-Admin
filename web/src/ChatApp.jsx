import { memo, useCallback, useEffect, useLayoutEffect, useMemo, useRef, useState } from 'react'
import { createPortal } from 'react-dom'
import { Collapse, Tag } from 'antd'
import gsap from 'gsap'
import { useGSAP } from '@gsap/react'
import { Bot, Check, ChevronDown, ChevronLeft, ChevronRight, Clock3, Copy, Edit3, ExternalLink, FileArchive, FileCode2, FileImage, FileOutput, FileSpreadsheet, FileText, FolderOpen, Lock, Paperclip, Menu, MessageSquarePlus, MoreHorizontal, PanelRightOpen, Pin, Plus, RefreshCw, Send, Sparkles, Square, Trash2, Wrench, X } from 'lucide-react'
import { api, apiStream } from './lib/api'
import { confirmDanger } from './lib/danger'
import { fuzzyMatch } from './lib/format'
import { JSON_TREE_CHILD_LIMIT, JSON_TREE_STRING_LIMIT, LIST_ITEM_LIMIT, LONG_TEXT_PREVIEW_CHARS, MARKDOWN_BLOCK_LIMIT, MARKDOWN_CHAR_LIMIT, MARKDOWN_LINE_LIMIT, isToolResultText, parseAssistantContent, previewLongText, splitMarkdownParts, textRenderStats } from './lib/chatTextSafety'
import { getAskUserPayload } from './lib/askUserPayload'
import { preferredUltraPlanOutputFile, reconcileUltraPlanTasks } from './lib/ultraPlanTasks'
import { REASONING_EFFORT_LEVELS, REASONING_EFFORT_OPTIONS, normalizeReasoningEffort } from './lib/reasoningEffort'
import { deleteChatSessions, normalizeSessionIds } from './lib/chatSessionManagement'

gsap.registerPlugin(useGSAP)

const prefersReducedMotion = () => typeof window !== 'undefined' && window.matchMedia?.('(prefers-reduced-motion: reduce)').matches
const isNarrowChatViewport = () => typeof window !== 'undefined' && window.matchMedia?.('(max-width: 900px)').matches
const isMobileViewport = () => typeof window !== 'undefined' && window.matchMedia?.('(max-width: 560px)').matches

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
const modelProvider = (m) => {
  const provider = String(m?.provider || '').trim()
  if (provider) return provider
  const name = String(m?.name || '').trim()
  const model = String(m?.model || '').trim()
  if (name && model && name.endsWith(`/${model}`)) return name.slice(0, -(model.length + 1))
  const split = name.lastIndexOf('/')
  return (split > 0 ? name.slice(0, split) : name) || '未分组服务商'
}
const runtimeModelLabel = (m) => {
  const model = String(m?.model || '').trim()
  if (model) return model
  const label = modelLabel(m)
  return label.includes('/') ? label.split('/').pop() : label
}

const BUILTIN_SLASH_COMMANDS = [
  { cmd: '/continue', key: '/continue', insert: '/continue', desc: '列出可恢复的官方 GA 会话', builtIn: true },
  { cmd: '/continue <编号>', key: '/continue', insert: '/continue ', desc: '恢复第 N 个官方 GA 会话，可继续对话', builtIn: true },
  { cmd: '/review <自然语言请求>', key: '/review', insert: '/review ', desc: '审阅当前改动；可继续输入范围或关注点', builtIn: true },
  { cmd: '/review help', key: '/review help', insert: '/review help', desc: '显示 /review 帮助，不启动审阅', builtIn: true },
  { cmd: '/ultraplan <目标>', key: '/ultraplan', insert: '/ultraplan ', desc: '显式进入 UltraPlan 规划模式，并生成本地 run 目录', builtIn: true },
  { cmd: '/improve', key: '/improve', insert: '/improve', desc: '发送记忆提炼请求（L3 skill + L1 索引）', builtIn: true },
  { cmd: '/effort', key: '/effort', insert: '/effort', desc: '查看当前 reasoning effort', builtIn: true },
  ...REASONING_EFFORT_LEVELS.map(level => ({
    cmd: `/effort ${level}`,
    key: `/effort ${level}`,
    insert: `/effort ${level}`,
    desc: level === 'off' ? '清除 reasoning effort' : `设置 reasoning effort 为 ${level}`,
    builtIn: true,
  })),
  { cmd: '/workspace <路径>', key: '/workspace', insert: '/workspace ', desc: '为当前会话绑定项目目录', builtIn: true },
  { cmd: '/workspace off', key: '/workspace off', insert: '/workspace off', desc: '关闭当前会话 workspace', builtIn: true },
]
const builtinSlashKey = (cmd = '') => String(cmd || '').trim().toLowerCase()
const builtinSlashCommandKey = (c) => builtinSlashKey(c?.key || c?.cmd)
const slashCommandInsertText = (c, current = '') => {
  if (!c) return current || ''
  if (c.cmd === '/review <自然语言请求>') {
    const text = String(current || '')
    return /^\s*\/review\s+/.test(text) ? text : (c.insert ?? '/review ')
  }
  if (c.cmd === '/continue <编号>') {
    const text = String(current || '')
    return /^\s*\/continue\s+/.test(text) ? text : (c.insert ?? '/continue ')
  }
  if (c.cmd === '/workspace <路径>') {
    const text = String(current || '')
    return /^\s*\/workspace\s+/.test(text) ? text : (c.insert ?? '/workspace ')
  }
  if (c.cmd === '/ultraplan <目标>') {
    const text = String(current || '')
    return /^\s*\/ultraplan\s+/.test(text) ? text : (c.insert ?? '/ultraplan ')
  }
  return c?.insert ?? `${c?.cmd || ''} `
}
const slashCommandProgressiveFilter = (c, nextText = '') => {
  if (c?.cmd === '/review <自然语言请求>') return 'review '
  if (c?.cmd === '/continue') return 'continue '
  if (c?.cmd === '/improve') return 'improve '
  if (c?.cmd === '/effort') return 'effort '
  if (c?.cmd === '/workspace <路径>') return 'workspace '
  if (c?.cmd === '/ultraplan <目标>') return 'ultraplan '
  const text = String(nextText || '').trimStart()
  if (text === '/review') return 'review '
  if (text === '/continue') return 'continue '
  if (text === '/improve') return 'improve '
  if (text === '/effort') return 'effort '
  if (text === '/workspace') return 'workspace '
  if (text === '/ultraplan') return 'ultraplan '
  return ''
}
const slashCommandNextDrawer = (c, nextText = '') => {
  const filter = slashCommandProgressiveFilter(c, nextText)
  return filter ? { open:true, filter, selectedIdx:0 } : { open:false, filter:'', selectedIdx:0 }
}


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

const MAX_CHAT_UPLOAD_FILES = 8
const MAX_CHAT_UPLOAD_BYTES_PER_FILE = 20 * 1024 * 1024
const MAX_CHAT_UPLOAD_BYTES_TOTAL = 40 * 1024 * 1024

const uploadFileName = (f) => String(f?.name || f?.Name || 'attachment')
const uploadFileSource = (f) => String(f?.dataURL || f?.DataURL || f?.url || f?.URL || '')

function isImageFile(f) {
  if (!f) return false
  const mime = String(f.type || f.Type || f.mime || f.Mime || '')
  if (mime.startsWith('image/')) return true
  const ref = String(f.name || f.Name || f.url || f.URL || f.path || f.Path || f.dataURL || f.DataURL || '').split(/[?#]/)[0]
  return /\.(png|jpe?g|gif|webp|bmp|svg)$/i.test(ref)
}

const FILE_KIND_RULES = [
  { kind:'image', re:/\.(png|jpe?g|gif|webp|bmp|svg)$/i, Icon:FileImage },
  { kind:'archive', re:/\.(zip|rar|7z|tar|gz|bz2|xz)$/i, Icon:FileArchive },
  { kind:'sheet', re:/\.(csv|xls|xlsx|ods)$/i, Icon:FileSpreadsheet },
  { kind:'code', re:/\.(c|cc|cpp|cs|css|go|h|hpp|html?|java|js|jsx|json|kt|kts|md|php|py|rb|rs|sh|sql|swift|toml|ts|tsx|vue|xml|ya?ml)$/i, Icon:FileCode2 },
  { kind:'pdf', re:/\.pdf$/i, Icon:FileOutput },
]

const getFileVisual = (value) => FILE_KIND_RULES.find((rule) => rule.re.test(String(value || '').split(/[?#]/)[0])) || { kind:'file', Icon:FileText }

function FileAttachment({ path }) {
  const clean = String(path || '').trim()
  const name = clean.split(/[\\/]/).filter(Boolean).pop() || clean || '文件'
  const extMatch = name.match(/\.([^.]+)$/)
  const extension = extMatch ? extMatch[1].slice(0, 6).toUpperCase() : 'FILE'
  const splitAt = Math.max(clean.lastIndexOf('\\'), clean.lastIndexOf('/'))
  const directory = splitAt >= 0 ? clean.slice(0, splitAt) : '本地文件'
  const visual = getFileVisual(name)
  const { kind, Icon } = visual
  const isImage = kind === 'image'
  const imageUrl = `/api/files/image?path=${encodeURIComponent(clean)}`
  const open = async (mode) => {
    if (!confirmDanger('chat-file-open', `使用系统桌面打开${mode === 'folder' ? '文件所在位置' : '文件'}：${clean}？`)) return
    try {
      await api('/api/files/open', { dangerous:true, method:'POST', body: JSON.stringify({ path: clean, mode }) })
    } catch (e) {
      alert(`打开失败：${e?.message || e}`)
    }
  }
  return <span className={`oa-file-card oa-file-kind-${kind}`} title={clean}>
    <button type="button" className="oa-file-leading" onClick={() => open('file')} aria-label={`打开文件 ${name}`}>
      <Icon className="oa-file-fallback-icon" size={19}/>
      {isImage && <img src={imageUrl} alt="" loading="lazy" onError={(e)=>{ e.currentTarget.style.display='none' }} />}
    </button>
    <span className="oa-file-meta">
      <span className="oa-file-name-row"><b>{name}</b><small>{extension}</small></span>
      <em>{directory || '本地文件'}</em>
    </span>
    <span className="oa-file-actions">
      <button type="button" onClick={() => open('file')} title="打开文件" aria-label={`打开文件 ${name}`}><ExternalLink size={15}/></button>
      <button type="button" onClick={() => open('folder')} title="打开所在位置" aria-label={`打开 ${name} 所在位置`}><FolderOpen size={15}/></button>
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

const parseUltraPlanResult = (text = '') => {
  const src = String(text || '').trim()
  if (!src.includes('UltraPlan invoked by explicit `/ultraplan` opt-in.')) return null
  const pick = (re) => {
    const m = src.match(re)
    return m ? String(m[1] || '').trim() : ''
  }
  const fence = (label) => {
    const safeLabel = String(label || '').replace(/[.*+?^${}()|[\]\\]/g, '\\$&')
    const re = new RegExp(safeLabel + ':\\s*\\n```\\n([\\s\\S]*?)\\n```', 'i')
    const m = src.match(re)
    return m ? String(m[1] || '').trim() : ''
  }
  const exitCodeText = pick(/^Exit code:\s*([^\n]+)/m)
  const exitCode = Number(exitCodeText)
  return {
    objective: pick(/^Objective:\s*([^\n]+)/m),
    script: pick(/^Script:\s*`?([^`\n]+)`?/m),
    runDir: pick(/^Run dir:\s*`?([^`\n]+)`?/m),
    exitCodeText,
    ok: Number.isFinite(exitCode) ? exitCode === 0 : true,
    stdout: fence('stdout'),
    stderr: fence('stderr'),
  }
}

// Parse raw ultraplan log text (streamed as plain content) into ultraplan_state shape
function parseUltraPlanText(text = '') {
  if (!text.includes('[ultraplan]') && !text.includes('[phase]')) return null
  const lines = text.split('\n')
  let objective = ''
  const phases = []
  const events = []       // {tag, body} - all raw log entries preserved in order
  const resultFiles = []  // {desc, file} - dedup by file path
  let current = ''        // last activity label
  let currentPhase = null

  const pushEvent = (tag, body) => events.push({ tag, body })

  for (const raw of lines) {
    const t = raw.trim()
    if (!t) continue
    const tagM = t.match(/^\[([a-z][a-z_-]*)\]\s*(.*)$/i)
    if (tagM) pushEvent(tagM[1].toLowerCase(), tagM[2])
    // [ultraplan] objective: xxx
    const objM = t.match(/^\[ultraplan\]\s+objective:\s*(.+)$/)
    if (objM) { objective = objM[1].trim(); current = `objective: ${objective}`; continue }
    // [phase] name - description
    const phM = t.match(/^\[phase\]\s+(\S+)\s+-\s+(.+)$/)
    if (phM) {
      currentPhase = { name: phM[1], desc: phM[2].trim(), status: 'running', tasks: [] }
      phases.push(currentPhase)
      current = `phase: ${currentPhase.name}`
      continue
    }
    // [subagent] desc -> filepath
    const saM = t.match(/^\[subagent\]\s+(.+?)\s+->\s+(.+)$/)
    if (saM && currentPhase) {
      currentPhase.tasks.push({ desc: saM[1].trim(), file: saM[2].trim(), status: 'running' })
      current = `task: ${saM[1].trim()}`
      continue
    }
    // [result] desc -> filepath  (marks last running subagent task done)
    const resM = t.match(/^\[result\]\s+(.+?)\s+->\s+(.+)$/)
    if (resM && currentPhase) {
      const desc = resM[1].trim(); const file = resM[2].trim()
      const lastRunning = [...currentPhase.tasks].reverse().find(tk => tk.status === 'running')
      if (lastRunning) { lastRunning.status = 'done'; lastRunning.file = file }
      if (!resultFiles.some(r => r.file === file)) resultFiles.push({ desc, file })
      continue
    }
    // [done] name (elapsed)
    const doneM = t.match(/^\[done\]\s+(\S+)\s+\((.+?)\)$/)
    if (doneM && currentPhase) {
      currentPhase.status = 'done'
      currentPhase.elapsed = doneM[2]
      currentPhase.tasks.forEach(tk => { if (tk.status === 'running') tk.status = 'done' })
      current = `done: ${doneM[1]}`
      continue
    }
    // [summary] key: value
    const sumM = t.match(/^\[summary\]\s+(.+)$/)
    if (sumM && currentPhase) {
      currentPhase.tasks.push({ desc: sumM[1].trim(), status: 'done' })
      continue
    }
  }

  if (!objective && phases.length === 0) return null
  const complete = phases.length > 0 && phases.every(ph => ph.status === 'done')
  return { objective, phases, complete, events, resultFiles, current }
}

const taskOutputToText = (value) => {
  if (!value) return ''
  if (Array.isArray(value)) return value.filter(v => v !== undefined && v !== null).join('\n')
  return String(value)
}

const ultraPlanTaskKeys = (task = {}) => {
  const keys = []
  const add = (v) => {
    const s = String(v || '').trim()
    if (s && !keys.includes(s)) keys.push(s)
  }
  add(task.id)
  add(task.task_id)
  add(task.key)
  add(task.file)
  add(task.path)
  add(task.outputFile)
  add(task.output_file)
  add(task.outFile)
  add(task.out_file)
  const file = task.outputFile || task.output_file || task.outFile || task.out_file || task.file || task.path || ''
  if (file) {
    const name = String(file).split(/[\\/]/).pop()
    add(name)
    if (name.endsWith('.out.txt')) add(name.slice(0, -8))
    if (name.endsWith('.txt')) add(name.slice(0, -4))
  }
  return keys
}

const normalizeUltraPlanTask = (task = {}, taskOutputs = {}) => {
  const statusRaw = String(task.status || task.state || '').toLowerCase()
  const status = statusRaw === 'run' ? 'running' : (statusRaw || 'running')
  const taskKeys = ultraPlanTaskKeys(task)
  const liveOutput = taskKeys.map(k => taskOutputToText(taskOutputs[k])).find(Boolean) || ''
  const output = liveOutput || task.output || task.out || task.result || task.summary || ''
  const outputFile = preferredUltraPlanOutputFile(task)
  return {
    ...task,
    status,
    desc: task.desc || task.name || task.title || task.msg || '',
    file: outputFile,
    output,
    outputFile,
  }
}

const normalizeUltraPlanPhase = (phase = {}, taskOutputs = {}) => {
  const statusRaw = String(phase.status || phase.state || '').toLowerCase()
  const status = statusRaw === 'run' ? 'running' : (statusRaw || 'running')
  const children = Array.isArray(phase.children) ? phase.children.map(ch => normalizeUltraPlanPhase(ch, taskOutputs)) : []
  let rawTasks = Array.isArray(phase.tasks) ? phase.tasks : []
  // If parent phase is done, any child task still marked "running" is a stale streaming artifact — fix to done
  if (status === 'done') {
    rawTasks = rawTasks.map(t => String(t.status || '').toLowerCase() === 'running' ? { ...t, status: 'done' } : t)
  }
  return {
    ...phase,
    status,
    tasks: rawTasks.map(t => normalizeUltraPlanTask(t, taskOutputs)),
    children,
  }
}

const isUltraPlanPhaseDone = (phase = {}) => {
  const children = Array.isArray(phase.children) ? phase.children : []
  return phase.status && !['run', 'running'].includes(String(phase.status).toLowerCase()) && children.every(isUltraPlanPhaseDone)
}

const normalizeUltraPlanEvent = (event = {}) => {
  if (typeof event === 'string') return { tag: 'event', body: event }
  const tag = event.tag || event.type || 'event'
  const body = event.body || event.msg || event.message || event.desc || ''
  const elapsed = event.elapsed ?? event.time  // preserve as separate display field
  // Remove time/elapsed from spread to prevent repeated normalization accumulating prefix
  const { time, elapsed: _e, body: _b, msg: _m, message: _msg, desc: _d, tag: _t, type: _ty, ...rest } = event
  return { ...rest, tag, body, ...(elapsed !== undefined ? { elapsed } : {}) }
}

const normalizeUltraPlanState = (raw) => {
  if (!raw || typeof raw !== 'object') return null
  const taskOutputs = raw.taskOutputs || raw.task_outputs || {}

  // Pre-process: fix backend streaming bug that leaks ALL rich tasks (with id) into the last phase.
  // A "simple" task has no id — it's the phase's declared intent (desc + status only).
  // A "rich" task has id + output_file — it's the actual executed result injected by the backend.
  //
  // Two categories of rich tasks:
  //   leaked  = rich task whose desc matches a simple task in ANY phase → belongs to that phase, not here
  //   native  = rich task whose desc has NO matching simple task anywhere → truly belongs to this phase
  //
  // For each phase:
  //   1. Keep native rich tasks (e.g. verify's "completeness check" which only appears as rich, never simple)
  //   2. Enrich simple tasks with data from matching rich task (output_file, id, etc.)
  //   3. Drop leaked rich tasks (they've been redistributed to their owner phases via step 2)
  const allSimpleDescs = new Set()
  const richByDesc = {}
  if (Array.isArray(raw.phases)) {
    for (const ph of raw.phases) {
      for (const t of (ph.tasks || [])) {
        if (!t.id && t.desc) allSimpleDescs.add(t.desc)
        if (t.id && t.desc) richByDesc[t.desc] = t
      }
    }
  }
  const phasesRaw = Array.isArray(raw.phases) ? raw.phases.map(ph => {
    const simpleTasks = (ph.tasks || []).filter(t => !t.id)
    // Native rich tasks: have id but desc not declared as a simple task in any phase
    const nativeRich = (ph.tasks || []).filter(t => t.id && t.desc && !allSimpleDescs.has(t.desc))
    // Enrich simple tasks with matching rich task data (output_file, id)
    const enrichedSimple = simpleTasks.map(t => {
      const rich = richByDesc[t.desc]
      if (!rich) return t
      return { ...rich, desc: t.desc, status: t.status }
    })
    return { ...ph, tasks: [...enrichedSimple, ...nativeRich] }
  }) : []

  const normalizedPhases = phasesRaw.map(ph => normalizeUltraPlanPhase(ph, taskOutputs))
  const recentTasksRaw = Array.isArray(raw.recentTasks) ? raw.recentTasks : (Array.isArray(raw.recent_tasks) ? raw.recent_tasks : (Array.isArray(raw.tasks) ? raw.tasks : []))
  const normalizedRecentTasks = recentTasksRaw.map(t => normalizeUltraPlanTask(t, taskOutputs))
  // A rich task may arrive both under a phase and in the live/recent stream.
  // Merge its output/status into the phase row, then render only genuinely unmatched recent work.
  const { phases, recentTasks } = reconcileUltraPlanTasks(normalizedPhases, normalizedRecentTasks)
  const resultFiles = Array.isArray(raw.resultFiles) ? raw.resultFiles : (Array.isArray(raw.result_files) ? raw.result_files : [])
  const complete = Boolean(raw.complete || raw.done || (phases.length > 0 && phases.every(isUltraPlanPhaseDone)))
  return {
    ...raw,
    taskOutputs,
    task_outputs: taskOutputs,
    phases,
    recentTasks,
    resultFiles,
    events: Array.isArray(raw.events) ? raw.events.map(normalizeUltraPlanEvent) : [],
    complete,
  }
}

const mergeUltraPlanStates = (...states) => {
  const normalized = states.map(normalizeUltraPlanState).filter(Boolean)
  if (!normalized.length) return null
  const merged = {}
  const eventSeen = new Set()
  const fileSeen = new Set()
  const mergedEvents = []
  const mergedFiles = []
  const mergedTaskOutputs = {}

  for (const st of normalized) {
    const {
      phases,
      recentTasks,
      recent_tasks,
      tasks,
      events,
      resultFiles,
      result_files,
      taskOutputs,
      task_outputs,
      ...rest
    } = st
    Object.assign(merged, rest)
    if (taskOutputs && typeof taskOutputs === 'object') Object.assign(mergedTaskOutputs, taskOutputs)
    if (task_outputs && typeof task_outputs === 'object') Object.assign(mergedTaskOutputs, task_outputs)
    if (Array.isArray(phases) && phases.length > 0) merged.phases = phases
    if (Array.isArray(recentTasks) && recentTasks.length > 0) merged.recentTasks = recentTasks
    const eventList = Array.isArray(events) ? events : []
    for (const ev of eventList) {
      const key = `${ev.tag || ''}|${ev.body || ''}`
      if (!eventSeen.has(key)) { eventSeen.add(key); mergedEvents.push(ev) }
    }
    const fileList = Array.isArray(resultFiles) ? resultFiles : []
    for (const rf of fileList) {
      const key = rf.file || `${rf.desc || ''}|${JSON.stringify(rf)}`
      if (!fileSeen.has(key)) { fileSeen.add(key); mergedFiles.push(rf) }
    }
  }

  merged.taskOutputs = mergedTaskOutputs
  merged.task_outputs = mergedTaskOutputs
  merged.events = mergedEvents
  merged.resultFiles = mergedFiles
  merged.complete = normalized.some(st => st.complete || st.done)
    || (Array.isArray(merged.phases) && merged.phases.length > 0 && merged.phases.every(isUltraPlanPhaseDone))
  return normalizeUltraPlanState(merged)
}

function UltraPlanResultCard({ text = '' }) {
  const result = parseUltraPlanResult(text)
  if (!result) return null
  return <div className={`oa-ultraplan-result ${result.ok ? 'is-ok' : 'is-error'}`}>
    <div className="oa-ultraplan-head">
      <span className="oa-ultraplan-orb"><Sparkles size={16}/></span>
      <div><b>UltraPlan</b><small>显式 /ultraplan 调用结果</small></div>
      <em>{result.ok ? '完成' : '异常'} · Exit {result.exitCodeText || '0'}</em>
    </div>
    {result.objective && <div className="oa-ultraplan-objective">{result.objective}</div>}
    <div className="oa-ultraplan-meta">
      {result.runDir && <span><b>Run dir</b><code>{result.runDir}</code></span>}
      {result.script && <span><b>Script</b><code>{result.script}</code></span>}
    </div>
    {(result.stdout || result.stderr) && <div className="oa-ultraplan-logs">
      {result.stdout && <details open><summary>stdout</summary><pre>{result.stdout}</pre></details>}
      {result.stderr && <details open={!result.ok}><summary>stderr</summary><pre>{result.stderr}</pre></details>}
    </div>}
  </div>
}

const renderAssistantBody = (text = '', onAskReply, ultraplan_state) => {
  const parsedState = parseUltraPlanText(text)
  const upState = mergeUltraPlanStates(ultraplan_state, parsedState)
  if (upState && (upState.phases?.length > 0 || upState.recentTasks?.length > 0 || upState.objective)) {
    return <UltraPlanDashboard state={upState} text={text} onAskReply={onAskReply} />
  }
  const result = parseUltraPlanResult(text)
  if (result) return <UltraPlanResultCard text={text} />
  return <MarkdownBlock text={text} onAskReply={onAskReply} />
}

const taskFileName = (fp = '') => String(fp || '').split(/[\\/]/).filter(Boolean).pop() || ''

/*─── SubagentOutputBlock: structured rendering of subagent turn logs ───*/
// Returns { prefix: seg[], turns: [{n, children: seg[]}] }
// prefix = segs before first Turn; each turn groups its own segs
function parseSubagentOutput(raw) {
  const lines = (raw || '').split('\n')
  const prefix = []
  const turns = []
  let cur = null   // current turn group (children array)
  let buf = []
  let i = 0

  const flush = (target) => {
    const t = buf.join('\n').trim()
    if (t) target.push({ type: 'text', text: t })
    buf = []
  }
  const target = () => cur ? cur.children : prefix

  while (i < lines.length) {
    const ln = lines[i], tr = ln.trim()
    const mT = tr.match(/^LLM Running \(Turn (\d+)\)/)
    if (mT) {
      flush(target())
      cur = { n: +mT[1], children: [] }
      turns.push(cur)
      i++; continue
    }
    const mS = tr.match(/^<summary>([\s\S]*?)<\/summary>$/)
    if (mS) { flush(target()); target().push({ type: 'summary', text: mS[1] }); i++; continue }
    if (/\uD83D\uDEE0/.test(tr)) {
      flush(target())
      const mTool = tr.match(/[\uD83D\uDEE0]\uFE0F?\s+(\w+)\(([\s\S]+)\)\s*$/)
      if (mTool) {
        let args = {}
        try { args = JSON.parse(mTool[2]) } catch (_) {}
        target().push({ type: 'tool', name: mTool[1], args, rawArgs: mTool[2] })
      } else {
        target().push({ type: 'tool', name: tr, args: {}, rawArgs: '' })
      }
      i++; continue
    }
    if (tr.startsWith('Executed subtask')) { flush(target()); target().push({ type: 'exec', text: tr }); i++; continue }
    if (tr.startsWith('Result:')) { flush(target()); target().push({ type: 'result', text: tr.slice(7).trim() }); i++; continue }
    if (tr === 'Artifact:') {
      flush(target()); i++
      while (i < lines.length && !lines[i].trim()) i++
      if (i < lines.length) { target().push({ type: 'artifact', path: lines[i].trim() }); i++ }
      continue
    }
    if (tr === '[ROUND END]') { flush(target()); target().push({ type: 'roundend' }); i++; continue }
    buf.push(ln); i++
  }
  flush(target())
  return { prefix, turns }
}

function ToolCallCollapse({ name, args }) {
  const keys = Object.keys(args)
  const preview = keys.slice(0, 3).join(' \u00b7 ') + (keys.length > 3 ? ` +${keys.length - 3}` : '')
  const label = (
    <span className="sa-tool-collapse-label">
      <Tag color="blue" style={{ fontFamily: 'var(--mono,ui-monospace,monospace)', fontSize: 11, marginRight: 6 }}>{name}</Tag>
      {keys.length > 0 && <span className="sa-tool-preview">{preview}</span>}
    </span>
  )
  if (keys.length === 0) return (
    <div className="sa-tool-empty">
      <Tag color="blue" style={{ fontFamily: 'var(--mono,ui-monospace,monospace)', fontSize: 11 }}>{name}</Tag>
    </div>
  )
  return (
    <Collapse ghost size="small" className="sa-tool-collapse" items={[{
      key: '1',
      label,
      children: <pre className="sa-tool-json">{JSON.stringify(args, null, 2)}</pre>
    }]} />
  )
}

function SubagentOutputBlock({ text, onAskReply }) {
  const { prefix, turns } = useMemo(() => parseSubagentOutput(text), [text])

  const renderSeg = (seg, i) => {
    if (seg.type === 'summary') return (
      <div key={i} className="sa-out-summary">{seg.text}</div>
    )
    if (seg.type === 'tool') return (
      <ToolCallCollapse key={i} name={seg.name} args={seg.args} />
    )
    if (seg.type === 'exec') return (
      <div key={i} className="sa-out-exec">{seg.text}</div>
    )
    if (seg.type === 'result') return (
      <div key={i} className="sa-out-result-block">
        <span className="sa-out-result-label">Result</span>
        <span className="sa-out-result-text">{seg.text}</span>
      </div>
    )
    if (seg.type === 'artifact') {
      const fname = seg.path.replace(/\\/g, '/').split('/').pop()
      return (
        <div key={i} className="sa-out-artifact">
          <span className="sa-out-artifact-label">Artifact</span>
          <span className="sa-out-artifact-path" title={seg.path}>{fname}</span>
        </div>
      )
    }
    if (seg.type === 'roundend') return (
      <div key={i} className="sa-out-roundend">&#x2014; Round End &#x2014;</div>
    )
    if (seg.type === 'text' && seg.text) return (
      <MarkdownBlock key={i} text={seg.text} onAskReply={onAskReply} />
    )
    return null
  }

  // last turn open by default, others collapsed
  const defaultOpen = turns.length > 0 ? [String(turns[turns.length - 1].n)] : []

  const turnItems = turns.map(t => {
    const summaryText = t.children.find(s => s.type === 'summary')?.text || ''
    const toolCount = t.children.filter(s => s.type === 'tool').length
    const preview = summaryText
      ? summaryText.slice(0, 52) + (summaryText.length > 52 ? '\u2026' : '')
      : toolCount > 0 ? `${toolCount} tool call${toolCount > 1 ? 's' : ''}` : ''
    const label = (
      <span className="sa-turn-label">
        <Tag color="purple" style={{ fontSize: 10, padding: '0 5px', lineHeight: '18px', marginRight: 6 }}>
          Turn {t.n}
        </Tag>
        {preview && <span className="sa-turn-preview">{preview}</span>}
      </span>
    )
    return {
      key: String(t.n),
      label,
      children: <div className="sa-turn-body">{t.children.map(renderSeg)}</div>
    }
  })

  return (
    <div className="sa-out">
      {prefix.map(renderSeg)}
      {turnItems.length > 0 && (
        <Collapse
          size="small"
          className="sa-turn-collapse"
          defaultActiveKey={defaultOpen}
          items={turnItems}
        />
      )}
    </div>
  )
}

function UltraPlanTaskRow({ task, onAskReply }) {
  const linesJoined = Array.isArray(task.output_lines) ? task.output_lines.join('\n') : ''
  const initialContent = task.output || linesJoined || ''
  const outputFile = preferredUltraPlanOutputFile(task)
  const status = task.status || 'running'
  const isRunning = status === 'running'
  const [open, setOpen] = useState(() => isRunning)
  const [loading, setLoading] = useState(false)
  const [error, setError] = useState('')
  const [content, setContent] = useState(initialContent)
  // running tasks are always expandable (live stream), done tasks are always expandable
  const hasOutput = isRunning || status === 'done' || Boolean(task.output || linesJoined || outputFile)

  // When a task enters running state, open it by default. After it becomes done,
  // keep the current open state so the user can collapse it manually.
  useEffect(() => {
    if (isRunning) setOpen(true)
  }, [isRunning])

  // Sync content from SSE-pushed task.output / task.output_lines
  useEffect(() => {
    const next = task.output || (Array.isArray(task.output_lines) ? task.output_lines.join('\n') : '')
    if (next && next !== content) setContent(next)
  }, [task.output, task.output_lines])

  // Poll output file while running; do a final fetch when done (covers running→done transition)
  useEffect(() => {
    if (!open || !outputFile) return
    let cancelled = false
    const fetchFile = async () => {
      try {
        const d = await api(`/api/files/read?path=${encodeURIComponent(outputFile)}`)
        if (!cancelled && d?.content) setContent(d.content)
      } catch (_) {}
    }
    if (isRunning) {
      fetchFile() // immediate first fetch on open
      const timer = setInterval(fetchFile, 500)
      return () => { cancelled = true; clearInterval(timer) }
    } else {
      // Done: one-time fetch (handles: open after done, OR running→done while panel was open)
      fetchFile()
      return () => { cancelled = true }
    }
  }, [open, isRunning, outputFile])

  const toggle = async () => {
    if (!hasOutput) return
    const nextOpen = !open
    setOpen(nextOpen)
    // running tasks are handled by the polling useEffect above
    if (!nextOpen || isRunning || content || !outputFile) return
    setLoading(true)
    setError('')
    try {
      const d = await api(`/api/files/read?path=${encodeURIComponent(outputFile)}`)
      setContent(d?.content || '')
      if (!d?.content) setError('Output file is empty.')
    } catch (err) {
      setError(err?.message || String(err))
    } finally {
      setLoading(false)
    }
  }

  return (
    <div className={`oa-up-task-wrap ${status}${open ? ' is-open' : ''}`}>
      <div
        className={`oa-up-task ${status}${hasOutput ? ' has-output' : ''}`}
        onClick={hasOutput ? toggle : undefined}
        role={hasOutput ? 'button' : undefined}
        tabIndex={hasOutput ? 0 : undefined}
        onKeyDown={hasOutput ? (e) => (e.key === 'Enter' || e.key === ' ') && toggle() : undefined}
        title={outputFile || task.desc || ''}
      >
        <span className={`oa-up-task-dot oa-up-task-dot-${status}`} />
        <span className="oa-up-task-desc">{task.desc}</span>
        {outputFile && <span className="oa-up-task-file">{taskFileName(outputFile)}</span>}
        {hasOutput && (
          <span className="oa-up-task-chevron-wrap">
            <ChevronRight size={13} className="oa-up-task-chevron" />
          </span>
        )}
      </div>
      {open && hasOutput && (
        <div className="oa-up-task-output">
          {loading && <div className="oa-up-task-output-meta">Loading output\u2026</div>}
          {error && <div className="oa-up-task-output-error">{error}</div>}
          {!loading && !error && content && <SubagentOutputBlock text={content} onAskReply={onAskReply} />}
          {!loading && !error && !content && status === 'running' && (
            <div className="oa-up-task-output-waiting">
              <span className="oa-up-task-output-waiting-dot" /><span className="oa-up-task-output-waiting-dot" /><span className="oa-up-task-output-waiting-dot" />
              <span>等待输出…</span>
            </div>
          )}
          {!loading && !error && !content && status === 'done' && (
            <div className="oa-up-task-output-meta" style={{color:'var(--muted-2)',fontStyle:'italic'}}>暂无输出内容</div>
          )}
        </div>
      )}
    </div>
  )
}

function UltraPlanDashboard({ state, text, onAskReply }) {
  const { objective, phases = [], recentTasks = [], complete, events = [], resultFiles = [], current, taskOutputs = {}, task_outputs = {} } = state
  const outputsMap = (taskOutputs && Object.keys(taskOutputs).length) ? taskOutputs : (task_outputs || {})
  const openFile = (fp) => {
    if (!fp) return
    const u = `/api/files/read?path=${encodeURIComponent(fp)}`
    window.open(u, '_blank', 'noopener')
  }
  return (
    <div className="oa-up-dash">
      <div className="oa-up-head">
        <span className="oa-up-icon">{'\u26a1'}</span>
        <span className="oa-up-title">UltraPlan</span>
        {objective && <span className="oa-up-obj">{objective}</span>}
        {complete
          ? <span className="oa-up-badge oa-up-done">{'\u5b8c\u6210'}</span>
          : (phases.length > 0 || recentTasks.length > 0) && <span className="oa-up-badge oa-up-run">{'\u6267\u884c\u4e2d\u2026'}</span>}
      </div>
      {!complete && current && (
        <div className="oa-up-current"><span className="oa-up-current-dot"></span>{current}</div>
      )}
      {recentTasks.length > 0 && (
        <div className="oa-up-recent">
          <div className="oa-up-recent-head">Subagents / 最近任务</div>
          <div className="oa-up-tasks">
            {recentTasks.map((t, j) => {
              const lines = (t && t.id && outputsMap?.[t.id]) ? outputsMap[t.id] : null
              const injected = lines && lines.length ? { ...t, output_lines: lines } : t
              return <UltraPlanTaskRow key={j} task={injected} onAskReply={onAskReply} />
            })}
          </div>
        </div>
      )}
      {phases.length > 0 && (
        <div className="oa-up-phases">
          {phases.map((ph, i) => (
            <div key={i} className={`oa-up-phase ${ph.status || 'running'}`}>
              <span className="oa-up-phase-icon">
                {ph.status === 'done' ? '\u2713' : ph.status === 'fail' ? '\u2717' : '\u25cc'}
              </span>
              <div className="oa-up-phase-body">
                <div className="oa-up-phase-info">
                  <span className="oa-up-phase-name">{ph.name}</span>
                  {ph.desc && <span className="oa-up-phase-desc">{ph.desc}</span>}
                  {ph.elapsed && <span className="oa-up-phase-time">{ph.elapsed}</span>}
                </div>
                {ph.tasks && ph.tasks.length > 0 && (
                  <div className="oa-up-tasks">
                    {ph.tasks.map((t, j) => {
                      const lines = (t && t.id && outputsMap && outputsMap[t.id]) ? outputsMap[t.id] : null
                      const injected = lines && lines.length ? { ...t, output_lines: lines } : t
                      return <UltraPlanTaskRow key={j} task={injected} onAskReply={onAskReply} />
                    })}
                  </div>
                )}
              </div>
            </div>
          ))}
        </div>
      )}
      {resultFiles.length > 0 && (
        <div className="oa-up-files">
          <div className="oa-up-files-head">{'\u4ea7\u51fa\u6587\u4ef6'} ({resultFiles.length})</div>
          <div className="oa-up-files-list">
            {resultFiles.map((r, i) => (
              <div key={i} className="oa-up-file-item" onClick={() => openFile(r.file)} title={r.file}>
                <span className="oa-up-file-icon">{'\ud83d\udcc4'}</span>
                <div className="oa-up-file-body">
                  <div className="oa-up-file-desc">{r.desc}</div>
                  <div className="oa-up-file-path">{r.file}</div>
                </div>
              </div>
            ))}
          </div>
        </div>
      )}
      {events.length > 0 && (
        <details className="oa-up-events">
          <summary>{'\u65e5\u5fd7'} ({events.length})</summary>
          <div className="oa-up-events-body">
            {events.map((e, i) => (
              <div key={i} className={`oa-up-event oa-up-event-${e.tag}`}>
                <span className="oa-up-event-tag">[{e.tag}]</span>
                {e.elapsed !== undefined && <span className="oa-up-event-time">{e.elapsed}s</span>}
                <span className="oa-up-event-body">{e.body}</span>
              </div>
            ))}
          </div>
        </details>
      )}
      {complete && (() => {
        // Only show text that is NOT ultraplan log lines (e.g. extra agent commentary after the block)
        const resultText = (text || '').split('\n').filter(ln => {
          const t = ln.trim()
          return t && !t.match(/^\[(ultraplan|phase|subagent|result|done|next|summary)\]/)
        }).join('\n').trim()
        return resultText
          ? <div className="oa-up-result"><MarkdownBlock text={resultText} onAskReply={onAskReply} /></div>
          : null
      })()}
    </div>
  )
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

const AssistantContent = memo(function AssistantContent({ content, pending, onAskReply, turnUsages, ultraplan_state }) {
  const [openTurns, setOpenTurns] = useState({})
  const [stackOpen, setStackOpen] = useState(pending)
  // 生成中自动展开过程；完成后自动折叠，只留最终回复。手动切换在 pending 不变时保留
  useEffect(() => { setStackOpen(pending) }, [pending])
  const liveUltraPlanState = useMemo(() => normalizeUltraPlanState(ultraplan_state), [ultraplan_state])
  const stats = useMemo(() => textRenderStats(content), [content])
  const parsed = useMemo(() => parseAssistantContent(content), [content])
  const hasTurnSplit = parsed.runs.length > 0
  const hasLiveUltraPlan = !!(liveUltraPlanState && (liveUltraPlanState.phases?.length > 0 || liveUltraPlanState.recentTasks?.length > 0 || liveUltraPlanState.objective))
  if (!content && pending && !hasLiveUltraPlan) return <div className="oa-content oa-thinking">正在思考…</div>
  if (content && stats.tooLarge && !hasTurnSplit) return <div className="oa-content"><LongTextPreview text={content} stats={stats} /></div>
  const boxedRuns = parsed.runs.slice(0, -1)
  const lastRun = parsed.runs[parsed.runs.length - 1]
  const isTurnOpen = (r, i) => openTurns[`${r.turn}-${i}`] === true
  const toggleTurn = (r, i) => setOpenTurns(xs => ({ ...xs, [`${r.turn}-${i}`]: !isTurnOpen(r, i) }))
  return <div className={`oa-content ${parsed.runs.length ? 'oa-agent-output' : ''}`}>
    {parsed.runs.length > 0 && <div className={`oa-turn-stack ${stackOpen ? 'open' : 'collapsed'}`}>
      <button className="oa-turn-stack-head" type="button" onClick={() => setStackOpen(v => !v)} aria-expanded={stackOpen} title={stackOpen ? '折叠执行过程' : '展开执行过程'}>
        <span className="oa-run-dot"/>
        <span>执行过程</span>
        <b>{parsed.runs.length}</b>
        <em>{pending ? '正在生成' : '已完成'}</em>
        <ChevronDown className="oa-stack-chevron" size={15}/>
      </button>
      {stackOpen && boxedRuns.map((r, i) => {
        const open = isTurnOpen(r, i)
        const tu = turnUsages && turnUsages[i]
        return <section className={`oa-turn-card ${open ? 'open' : 'collapsed'}`} key={`${r.turn}-${i}`}>
          <button className="oa-turn-toggle" type="button" onClick={() => toggleTurn(r, i)} aria-expanded={open} title={r.title || '执行步骤'}>
            <span className="oa-turn-pill">Turn {r.turn}</span>
            <b>{r.title || '执行步骤'}</b>
            <UsageRow u={tu} className="oa-usage-inline" />
            <ChevronDown size={15} className="oa-turn-chevron"/>
          </button>
          {open && (r.body ? renderAssistantBody(r.body, onAskReply) : <p className="oa-turn-empty">该轮暂无详细输出</p>)}
        </section>
      })}
      {lastRun && <section className="oa-turn-current" key={`last-${lastRun.turn}`}>
        <div className="oa-turn-current-head"><span>Turn {lastRun.turn}</span><b>{lastRun.title || '正在执行'}</b><UsageRow u={turnUsages && turnUsages[boxedRuns.length]} className="oa-usage-inline" /><em>{pending ? '实时输出中' : '最新一轮'}</em></div>
        {lastRun.body ? renderAssistantBody(lastRun.body, onAskReply) : <p className="oa-turn-empty">正在等待该轮输出…</p>}
      </section>}
    </div>}
    {(parsed.body || !parsed.runs.length) && <div className={parsed.runs.length ? 'oa-final-answer' : ''}>
      {parsed.runs.length > 0 && <div className="oa-final-label">返回给用户</div>}
      {renderAssistantBody(parsed.body || content || '', onAskReply, liveUltraPlanState || ultraplan_state)}
    </div>}
  </div>
})

// User messages append a generated attachment block. Cards render it separately, so hide the raw suffix.
const stripUserAttachmentBlock = (content = '') => {
  const src = String(content || '')
  const markers = ['\n[\u9644\u4ef6]', '\n[\u56fe\u7247\u9644\u4ef6]', '\n[\u9644\u4ef6\u5df2\u4fdd\u5b58]', '[\u9644\u4ef6]', '[\u56fe\u7247\u9644\u4ef6]', '[\u9644\u4ef6\u5df2\u4fdd\u5b58]']
  let cut = -1
  for (const marker of markers) {
    const i = src.lastIndexOf(marker)
    if (i >= 0 && (cut < 0 || i < cut)) cut = i
  }
  return cut >= 0 ? src.slice(0, cut).trimEnd() : src
}

const extractSavedFilePaths = (content = '') => Array.from(
  String(content || '').matchAll(/\[FILE:([^\]]+)\]/g),
  (match) => match[1].trim(),
).filter(Boolean)

const usageHasTokens = (u) => !!u && ((u.input_tokens || 0) > 0 || (u.output_tokens || 0) > 0 || (u.cached_tokens || 0) > 0)
const formatElapsedMs = (ms = 0) => {
  const safe = Math.max(0, Number(ms) || 0)
  if (safe < 1000) return `${Math.max(0.1, safe / 1000).toFixed(1)}s`
  const totalSeconds = Math.floor(safe / 1000)
  const minutes = Math.floor(totalSeconds / 60)
  const seconds = totalSeconds % 60
  if (minutes <= 0) return `${seconds}s`
  const hours = Math.floor(minutes / 60)
  const mm = minutes % 60
  if (hours > 0) return `${hours}h ${mm}m ${seconds}s`
  return `${minutes}m ${seconds}s`
}
const getElapsedMs = (m, now = Date.now()) => {
  if (!m || m.role !== 'assistant') return 0
  if (m.elapsed_ms > 0) return m.elapsed_ms
  if (m.run_started_at_ms > 0) return Math.max(0, now - m.run_started_at_ms)
  return 0
}

const UsageRow = ({ u, label, className, elapsedMs = 0, live = false }) => {
  const hasTokens = usageHasTokens(u)
  const hasElapsed = elapsedMs > 0
  if (!hasTokens && !hasElapsed) return null
  return <div className={`oa-usage ${className || ''}`}>
    {label && <span className="oa-usage-label">{label}</span>}
    {hasElapsed && <span className={live ? 'oa-usage-time is-live' : 'oa-usage-time'} title={live ? '实时耗时' : '耗时'}><svg viewBox="0 0 16 16" width="10" height="10" fill="currentColor" aria-hidden="true"><path fillRule="evenodd" d="M8 2a6 6 0 1 0 0 12A6 6 0 0 0 8 2zm0 1.5A4.5 4.5 0 1 1 8 11a4.5 4.5 0 0 1 0-7.5z"/><path d="M7.5 4.5h1v3.65l2.2 1.3-.5.9L7.5 9V4.5z"/></svg>耗时 <b>{formatElapsedMs(elapsedMs)}</b></span>}
    {u?.input_tokens > 0 && <span className="oa-usage-in" title="输入 tokens"><svg viewBox="0 0 16 16" width="10" height="10" aria-hidden="true"><path d="M8 11.5 3.5 7l1.1-1.1L8 9.3l3.4-3.4L12.5 7 8 11.5Z"/></svg>输入 <b>{u.input_tokens.toLocaleString()}</b></span>}
    {u?.cached_tokens > 0 && <span className="oa-usage-cache" title="缓存 tokens"><svg viewBox="0 0 16 16" width="10" height="10" aria-hidden="true"><path d="M8.5 1 2 9h4.2l-1 6L13 7H8.5l1-6Z"/></svg>缓存 <b>{u.cached_tokens.toLocaleString()}</b></span>}
    {u?.output_tokens > 0 && <span className="oa-usage-out" title="输出 tokens"><svg viewBox="0 0 16 16" width="10" height="10" aria-hidden="true"><path d="M8 4.5 12.5 9l-1.1 1.1L8 6.7l-3.4 3.4L3.5 9 8 4.5Z"/></svg>输出 <b>{u.output_tokens.toLocaleString()}</b></span>}
  </div>
}

// 各内部 turn 用量累加得到整条回复总计
const sumUsages = (usages) => {
  if (!Array.isArray(usages) || !usages.length) return null
  return usages.reduce((acc, u) => ({
    input_tokens: acc.input_tokens + (u?.input_tokens || 0),
    cached_tokens: acc.cached_tokens + (u?.cached_tokens || 0),
    output_tokens: acc.output_tokens + (u?.output_tokens || 0),
  }), { input_tokens: 0, cached_tokens: 0, output_tokens: 0 })
}

export const ChatMessage = memo(function ChatMessage({ message: m, pending, onAskReply, clockNow = 0 }) {
  const userText = m.role === 'user' ? stripUserAttachmentBlock(m.content) : m.content
  const messageFiles = Array.isArray(m.files) ? m.files : []
  const imageFiles = messageFiles.filter(isImageFile)
  const savedFilePaths = m.role === 'user' ? extractSavedFilePaths(m.content) : []
  const pendingFiles = savedFilePaths.length > 0 ? [] : messageFiles.filter((file) => !isImageFile(file))
  const modelID = m.role === 'assistant' && typeof m.model_id === 'string' ? m.model_id.trim() : ''
  const modelNo = m.role === 'assistant' && Number.isInteger(m.llm_no) ? m.llm_no : null
  const modelIdentity = modelNo == null ? modelID : `#${modelNo} · ${modelID || '未知模型'}`
  const turnUsages = m.role === 'assistant' && Array.isArray(m.usages) && m.usages.length > 0 ? m.usages : null
  const hasUsage = !turnUsages && m.role === 'assistant' && m.usage && (m.usage.input_tokens > 0 || m.usage.output_tokens > 0)
  const usageTotal = turnUsages ? sumUsages(turnUsages) : (hasUsage ? m.usage : null)
  const elapsedMs = getElapsedMs(m, clockNow || Date.now())
  const showUsageRow = m.role === 'assistant' && (usageHasTokens(usageTotal) || elapsedMs > 0)
  const usageLabel = m.role === 'assistant' ? '总计' : null
  return <article id={`msg-${m.id}`} data-msg-role={m.role} className={`oa-message ${m.role} ${m.error?'error':''}`}>
    <div className="oa-avatar">{m.role === 'user' ? '你' : 'GA'}</div>
    <div className="oa-bubble">
      <div className="oa-meta"><b>{m.role === 'user' ? 'You' : 'GenericAgent'}</b>{modelIdentity && <span className="oa-model-id" title={`Model: ${modelIdentity}`}>{modelIdentity}</span>}{m.created_at && <span>{fmtTime(m.created_at)}</span>}{m.content && <CopyButton text={m.role === 'user' ? userText : m.content} compact />}</div>
      {imageFiles.length > 0 && <div className="oa-message-images">{imageFiles.map((file, i) => <img key={uploadFileName(file) || i} src={uploadFileSource(file)} alt={uploadFileName(file)} />)}</div>}
      {m.role === 'user' && (savedFilePaths.length > 0 || pendingFiles.length > 0) && <div className="oa-message-files">
        {savedFilePaths.map((savedPath, i) => <FileAttachment key={`${savedPath}-${i}`} path={savedPath} />)}
        {pendingFiles.map((file, i) => {
          const name = uploadFileName(file)
          const visual = getFileVisual(name)
          const Icon = visual.Icon
          return <span className={`oa-pending-file oa-file-kind-${visual.kind}`} key={`${name}-${i}`} title={`\u5f85\u4e0a\u4f20\uff1a${name}`}><Icon size={18}/><b>{name}</b></span>
        })}
      </div>}
      {m.role === 'assistant' ? <AssistantContent content={m.content} pending={pending} onAskReply={onAskReply} turnUsages={turnUsages} ultraplan_state={m.ultraplan_state} /> : (userText && <MarkdownBlock text={userText} />)}
      {showUsageRow && <UsageRow u={usageTotal} label={usageLabel} className="oa-usage-total" elapsedMs={elapsedMs} live={pending} />}
    </div>
  </article>
})

const MessageList = memo(function MessageList({ messages, isCurrentRunning, onAskReply, clockNow }) {
  return <>
    {messages.flatMap((m, i) => {
      const day = timelineKey(m.created_at)
      const prevDay = i > 0 ? timelineKey(messages[i - 1]?.created_at) : ''
      const nodes = []
      if (i === 0 || day !== prevDay) nodes.push(<div key={`tl-${day}-${i}`} className="oa-timeline"><span>{fmtTimelineDate(m.created_at)}</span></div>)
      nodes.push(<ChatMessage key={m.id} message={m} pending={isCurrentRunning && i === messages.length - 1} onAskReply={onAskReply} clockNow={clockNow} />)
      return nodes
    })}
  </>
})

function ProviderModelMenu({ groups, selectedProvider, previewProvider, value, onPreview, onSelect, onClose, mobile }) {
  const previewGroup = groups.find(group => group.value === previewProvider) || groups[0]
  return <>
    {mobile && <div className="oa-mobile-picker-head"><div><b>选择模型</b><span>先选择服务商，再选择模型</span></div><button type="button" onClick={onClose} aria-label="关闭模型选择"><X size={18}/></button></div>}
    <div className="oa-cascade-providers">
      {groups.map(group => <button key={group.value} type="button" className={group.value === previewGroup?.value ? 'active' : ''}
        onMouseEnter={() => onPreview(group.value)} onFocus={() => onPreview(group.value)} onClick={() => onPreview(group.value)}>
        <span>{group.label}</span><ChevronRight size={13}/>
      </button>)}
    </div>
    <div className="oa-cascade-models">
      <div className="oa-cascade-heading">{previewGroup?.label || '模型'}</div>
      {previewGroup?.models.length ? previewGroup.models.map(model => <button key={model.value} type="button"
        className={previewGroup.value === selectedProvider && String(model.value) === String(value) ? 'active' : ''}
        onClick={() => onSelect(model.value)}>
        {previewGroup.value === selectedProvider && String(model.value) === String(value) && <Check size={12}/>}<span>{model.label}</span>
      </button>) : <div className="oa-cascade-empty">未发现模型</div>}
    </div>
  </>
}

function ProviderModelCascade({ groups, selectedProvider, value, onChange, disabled, mobile = false }) {
  const [open, setOpen] = useState(false)
  const [previewProvider, setPreviewProvider] = useState(selectedProvider || groups[0]?.value || '')
  const ref = useRef()
  const menuRef = useRef()
  useEffect(() => {
    if (!open) return
    const close = () => setOpen(false)
    const h = e => { if (!ref.current?.contains(e.target) && !menuRef.current?.contains(e.target)) close() }
    const onKeyDown = e => { if (e.key === 'Escape') close() }
    const onScroll = e => { if (!ref.current?.contains(e.target) && !menuRef.current?.contains(e.target)) close() }
    document.addEventListener('mousedown', h)
    document.addEventListener('keydown', onKeyDown)
    window.addEventListener('scroll', onScroll, true)
    return () => { document.removeEventListener('mousedown', h); document.removeEventListener('keydown', onKeyDown); window.removeEventListener('scroll', onScroll, true) }
  }, [open, mobile])
  useEffect(() => {
    if (selectedProvider && groups.some(group => group.value === selectedProvider)) setPreviewProvider(selectedProvider)
    else if (groups[0]) setPreviewProvider(groups[0].value)
    else setPreviewProvider('')
  }, [selectedProvider, groups])

  const activeGroup = groups.find(group => group.value === selectedProvider)
  const activeModel = activeGroup?.models.find(model => String(model.value) === String(value))
  const displayModel = activeModel?.label || '\u672a\u53d1\u73b0\u6a21\u578b'
  const selectModel = next => { onChange(next); setOpen(false) }
  const menu = <div className={`oa-cascade-menu ${mobile ? 'oa-cascade-modal' : ''}`} ref={menuRef} role="dialog" aria-modal={mobile || undefined} aria-label="服务商和模型">
    <ProviderModelMenu groups={groups} selectedProvider={selectedProvider} previewProvider={previewProvider} value={value}
      onPreview={setPreviewProvider} onSelect={selectModel} onClose={() => setOpen(false)} mobile={mobile}/>
  </div>

  return (
    <>
      <div className="oa-model-select oa-composer-cascade" ref={ref}>
        <span>模型</span>
        <button type="button" disabled={disabled} title={displayModel} aria-label={`选择模型，当前 ${displayModel}`} aria-expanded={open} onClick={() => setOpen(o => !o)}>
          <span className="oa-cascade-current-model">{displayModel}</span><ChevronDown size={13}/>
        </button>
        {open && !mobile && menu}
      </div>
      {open && mobile && createPortal(<div className="oa-mobile-picker-backdrop" onMouseDown={e => { if (e.target === e.currentTarget) setOpen(false) }}>{menu}</div>, document.body)}
    </>
  )
}

function CustomSelect({ value, onChange, options, disabled, native = false, ariaLabel = '选择选项' }) {
  const [open, setOpen] = useState(false)
  const ref = useRef()
  useEffect(() => {
    if (!open) return
    const close = () => setOpen(false)
    const h = e => { if (!ref.current?.contains(e.target)) close() }
    const onScroll = e => { if (!ref.current?.contains(e.target)) close() }
    document.addEventListener('mousedown', h)
    window.addEventListener('scroll', onScroll, true)
    return () => { document.removeEventListener('mousedown', h); window.removeEventListener('scroll', onScroll, true) }
  }, [open])
  const label = options.find(o => String(o.value) === String(value))?.label ?? String(value)
  const displayLabel = label.includes('/') ? label.split('/').pop() : label
  if (native) return <select className="oa-native-select" value={value} onChange={e => onChange(e.target.value)} disabled={disabled} aria-label={ariaLabel}>
    {options.map(option => <option key={option.value} value={option.value}>{option.label}</option>)}
  </select>
  return (
    <div className="oa-cselect" ref={ref}>
      <button type="button" disabled={disabled} title={label} onClick={() => setOpen(o => !o)}>
        <span>{displayLabel}</span><ChevronDown size={13}/>
      </button>
      {open && <ul role="listbox">
        {options.map(o => (
          <li key={o.value} role="option" aria-selected={String(o.value)===String(value)}
            className={String(o.value)===String(value)?'active':''}
            onMouseDown={() => { onChange(o.value); setOpen(false) }}>
            {String(o.value)===String(value) && <Check size={11}/>}{o.label}
          </li>
        ))}
      </ul>}
    </div>
  )
}

export default function ChatApp() {
  useEffect(() => {
    api('/api/config').then(cfg => {
      setCfg(cfg)
    }).catch(() => {})
    api('/api/slash-commands').then(res => {
      const items = Array.isArray(res?.commands) ? res.commands : []
      const normalized = items
        .filter(c => c && typeof c.cmd === 'string' && c.cmd.trim().startsWith('/'))
        .map(c => ({
          ...c,
          cmd: c.cmd.trim(),
          key: c.key || c.cmd.trim(),
          insert: c.insert || c.cmd.trim(),
          builtIn: c.builtIn !== false,
        }))
      if (normalized.length) {
        const serverKeys = new Set(normalized.map(c => builtinSlashKey(c.cmd)))
        const missing = BUILTIN_SLASH_COMMANDS.filter(c => !serverKeys.has(builtinSlashKey(c.cmd)))
        setSlashCommands(missing.length ? [...normalized, ...missing] : normalized)
      }
    }).catch(() => {})
  }, [])
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
  const [collapsed, setCollapsed] = useState(() => isNarrowChatViewport())
  const [notice, setNotice] = useState('')
  const [llms, setLlms] = useState([])
  const [llmNo, setLlmNo] = useState(0)
  const [modelSwitching, setModelSwitching] = useState(false)
  const [toolsMode, setToolsMode] = useState('official')
  const [reasoningEffort, setReasoningEffort] = useState('off')
  const [menuOpen, setMenuOpen] = useState('')
  const [menuPos, setMenuPos] = useState(null)
  const [editing, setEditing] = useState('')
  const [draftTitle, setDraftTitle] = useState('')
  const [sessionManagerOpen, setSessionManagerOpen] = useState(false)
  const [selectedSessionIds, setSelectedSessionIds] = useState([])
  const [batchDeleting, setBatchDeleting] = useState(false)
  const [attachments, setAttachments] = useState([])
  const [queuedMessages, setQueuedMessages] = useState([])
  const [queueEditingId, setQueueEditingId] = useState('')
  const [queueDraft, setQueueDraft] = useState('')
  const [dragging, setDragging] = useState(false)
  const [autoFollow, setAutoFollow] = useState(true)
  const [showFollow, setShowFollow] = useState(false)
  const [toolsMenuOpen, setToolsMenuOpen] = useState(false)
  const [cmdDrawer, setCmdDrawer] = useState({ open: false, filter: '', selectedIdx: 0 })
  const [cmdManagerOpen, setCmdManagerOpen] = useState(false)
  const [slashCommands, setSlashCommands] = useState(BUILTIN_SLASH_COMMANDS)
  const [cfg, setCfg] = useState(null)
  const [cmdEditIdx, setCmdEditIdx] = useState(-1)
  const [cmdEditCmd, setCmdEditCmd] = useState('')
  const [cmdEditDesc, setCmdEditDesc] = useState('')
  const [cmdEditContent, setCmdEditContent] = useState('')
  const [isMobile, setIsMobile] = useState(() => isMobileViewport())
  const [streamClock, setStreamClock] = useState(() => Date.now())
  const toolsMenuRef = useRef(null)
  const threadRef = useRef(null)
  const endRef = useRef(null)
  const fileRef = useRef(null)
  const promptRef = useRef(null)
  const cmdDrawerRef = useRef(null)
  const selectedCmdRef = useRef(null)
  const streamAbortRef = useRef(null)
  const runSeqRef = useRef(0)
  const openSeqRef = useRef(0)
  const activeSidRef = useRef('')
  const messagesRef = useRef([])
  const scrollModeRef = useRef('auto')
  const queuedRef = useRef([])
  const chatScope = useRef(null)
  // Auto-grow composer textarea to fit content (clamped), reset to single row when cleared.
  const COMPOSER_MAX_H = 160

  useEffect(() => {
    if (!busy && !streamingSid) return undefined
    const tick = () => setStreamClock(Date.now())
    tick()
    const timer = window.setInterval(tick, 500)
    return () => window.clearInterval(timer)
  }, [busy, streamingSid])

  useEffect(() => {
    if (typeof window === 'undefined' || !window.matchMedia) return undefined
    const mq = window.matchMedia('(max-width: 900px)')
    const syncCollapsed = () => setCollapsed(mq.matches)
    syncCollapsed()
    mq.addEventListener?.('change', syncCollapsed)
    mq.addListener?.(syncCollapsed)
    return () => {
      mq.removeEventListener?.('change', syncCollapsed)
      mq.removeListener?.(syncCollapsed)
    }
  }, [])

  useEffect(() => {
    if (typeof window === 'undefined' || !window.matchMedia) return undefined
    const mq = window.matchMedia('(max-width: 560px)')
    const sync = () => setIsMobile(mq.matches)
    sync()
    mq.addEventListener?.('change', sync)
    mq.addListener?.(sync)
    return () => {
      mq.removeEventListener?.('change', sync)
      mq.removeListener?.(sync)
    }
  }, [])

  useLayoutEffect(() => {
    const el = promptRef.current
    if (!el) return
    el.style.height = 'auto'
    const next = Math.min(el.scrollHeight, COMPOSER_MAX_H)
    el.style.height = next + 'px'
    el.style.overflowY = el.scrollHeight > COMPOSER_MAX_H ? 'auto' : 'hidden'
  }, [prompt])
  const current = useMemo(() => sessions.find(s => s.id === sid), [sessions, sid])
  const isUltraPlanPrompt = /^\s*\/ultraplan(?:\s|$)/.test(prompt)
  const effectiveSlashCommands = slashCommands.length ? slashCommands : BUILTIN_SLASH_COMMANDS
  const officialSlashKeys = useMemo(() => new Set(effectiveSlashCommands.map(c => builtinSlashCommandKey(c))), [effectiveSlashCommands])
  const isProtectedSlashCommand = useCallback((cmd = '') => officialSlashKeys.has(builtinSlashKey(cmd)), [officialSlashKeys])
  const allSlashCommands = useMemo(() => {
    const custom = (cfg?.slash_commands || []).filter(c => !officialSlashKeys.has(builtinSlashKey(c.cmd)))
    return [...effectiveSlashCommands, ...custom]
  }, [cfg?.slash_commands, effectiveSlashCommands, officialSlashKeys])
  const filteredCmds = useMemo(() => {
    if (!cmdDrawer.open) return []
    const rawFilter = String(cmdDrawer.filter || '').trimStart()
    const slashFilter = rawFilter.startsWith('/') ? rawFilter : `/${rawFilter}`
    const childAllowed = (base) => {
      const childRoot = `${base} `
      if (slashFilter === childRoot) return true
      if (!slashFilter.startsWith(childRoot)) return false
      const rest = slashFilter.slice(childRoot.length).trimStart()
      return rest.length > 0 && 'help'.startsWith(rest)
    }
    const inContinueScope = slashFilter === '/continue' || slashFilter.startsWith('/continue ')
    const inReviewScope = slashFilter === '/review' || slashFilter.startsWith('/review ')
    const inImproveScope = slashFilter === '/improve' || slashFilter.startsWith('/improve ')
    const inUltraPlanScope = slashFilter === '/ultraplan' || slashFilter.startsWith('/ultraplan ')
    const isReviewNaturalLanguage = /^\/review\s+\S/.test(slashFilter) && !childAllowed('/review')
    const isContinueNumber = /^\/continue\s+\d+$/.test(slashFilter)
    const isUltraPlanObjective = /^\/ultraplan\s+\S/.test(slashFilter)
    return allSlashCommands.filter(c => {
      const cmd = String(c.cmd || '')
      if (cmd === '/review help') return childAllowed('/review') && fuzzyMatch(cmd, slashFilter)
      if (cmd === '/review <自然语言请求>') {
        if (isReviewNaturalLanguage) return true
        if (slashFilter === '/review' || fuzzyMatch('/review', rawFilter) || fuzzyMatch('/review', slashFilter)) return true
        if (slashFilter.startsWith('/review ')) return false
      }
      if (cmd === '/continue <编号>') {
        if (slashFilter === '/continue ') return true
        if (isContinueNumber) return true
        if (slashFilter.startsWith('/continue ')) return false
      }
      if (cmd === '/ultraplan <目标>') {
        if (slashFilter === '/ultraplan ') return true
        if (isUltraPlanObjective) return true
        if (slashFilter === '/ultraplan' || fuzzyMatch('/ultraplan', rawFilter) || fuzzyMatch('/ultraplan', slashFilter)) return true
        if (slashFilter.startsWith('/ultraplan ')) return false
      }
      if (inContinueScope && cmd !== '/continue <编号>') return false
      if (inReviewScope && cmd !== '/review <自然语言请求>') return false
      if (inImproveScope && cmd !== '/improve') return false
      if (inUltraPlanScope && cmd !== '/ultraplan <目标>') return false
      return fuzzyMatch(cmd, rawFilter) || fuzzyMatch(cmd, slashFilter) || fuzzyMatch(c.desc || '', rawFilter)
    })
  }, [cmdDrawer.open, cmdDrawer.filter, allSlashCommands])
  useLayoutEffect(() => {
    if (!cmdDrawer.open) return
    selectedCmdRef.current?.scrollIntoView({ block: 'nearest' })
  }, [cmdDrawer.open, cmdDrawer.selectedIdx, filteredCmds.length])
  useEffect(() => {
    if (cmdDrawer.open) setCmdEditIdx(-1)
  }, [cmdDrawer.open, cmdDrawer.filter])
  useEffect(() => {
    if (!cmdManagerOpen) setCmdEditIdx(-1)
  }, [cmdManagerOpen])
  const saveSlashCmds = async (newCmds) => {
    if (!confirmDanger('chat-slash-commands-save', '保存斜杠命令配置？会写入 GA Admin 配置文件。')) return
    try {
      const safeCmds = (newCmds || [])
        .filter(c => !isProtectedSlashCommand(c?.cmd))
        .map(c => ({ cmd: String(c?.cmd || '').trim(), desc: String(c?.desc || '').trim(), content: String(c?.content || c?.prompt || '').trim() }))
        .filter(c => c.cmd)
      const c = await api('/api/config', { method:'PUT', dangerous: true, body: JSON.stringify({...cfg, slash_commands: safeCmds}) })
      if (c?.slash_commands) { setCfg(c) }
      setCmdEditIdx(-1)
    } catch(e) { setNotice('保存命令失败: ' + e.message); setCmdEditIdx(-1) }
  }
  const startEdit = (idx, cmd, desc, content = '') => {
    if (idx < 0 && idx !== -2) return
    setCmdEditIdx(idx); setCmdEditCmd(cmd); setCmdEditDesc(desc); setCmdEditContent(content)
  }
  const saveEdit = () => {
    const normalized = cmdEditCmd.trim()
    if (!normalized) return
    if (isProtectedSlashCommand(normalized)) {
      setNotice('这是 GA Admin 内置命令，不能覆盖或修改')
      setCmdEditIdx(-1)
      return
    }
    const cmds = cfg?.slash_commands || []
    const nextItem = { cmd: normalized, desc: cmdEditDesc.trim() || '', content: cmdEditContent.trim() || cmdEditDesc.trim() || '' }
    if (!nextItem.content) {
      setNotice('请填写这个命令要展开成的指令内容')
      return
    }
    if (cmdEditIdx === -2) {
      saveSlashCmds([...cmds, nextItem])
    } else if (cmdEditIdx >= 0) {
      const newCmds = [...cmds]
      newCmds[cmdEditIdx] = nextItem
      saveSlashCmds(newCmds)
    }
  }
  const deleteCmd = (idx) => {
    if (idx < 0) { setNotice('这是 GA Admin 内置命令，不能删除'); return }
    const cmds = cfg?.slash_commands || []; saveSlashCmds(cmds.filter((_, i) => i !== idx))
  }
  const moveUpCmd = (cmd) => {
    if (cmd?.builtIn) return
    const cmds = cfg?.slash_commands || []
    const idx = cmds.findIndex(c => c.cmd === cmd.cmd && c.desc === cmd.desc)
    if (idx <= 0) return
    const newCmds = [...cmds]
    ;[newCmds[idx-1], newCmds[idx]] = [newCmds[idx], newCmds[idx-1]]
    saveSlashCmds(newCmds)
  }
  useEffect(() => { activeSidRef.current = sid }, [sid])

  const isActiveSession = (sessionId) => !sessionId || activeSidRef.current === sessionId

  const applyStreamEvent = (ev, pendingId, clientUserID = '', sessionId = '') => {
    if (!isActiveSession(sessionId)) return
    if (Object.prototype.hasOwnProperty.call(ev, 'workspace') || Object.prototype.hasOwnProperty.call(ev, 'project_mode')) {
      setSessions(xs => xs.map(x => x.id === sessionId ? {
        ...x,
        ...(Object.prototype.hasOwnProperty.call(ev, 'workspace') ? { workspace: ev.workspace || '' } : {}),
        ...(Object.prototype.hasOwnProperty.call(ev, 'project_mode') ? { project_mode: ev.project_mode || '' } : {}),
      } : x))
    }
    if (ev.type === 'user' && ev.message) {
      setMessages(xs => {
        if (!isActiveSession(sessionId)) return xs
        return clientUserID
          ? xs.map(m => m.id === clientUserID ? ev.message : m)
          : (xs.some(m => m.id === ev.message.id) ? xs : [...xs, ev.message])
      })
    }
    if (ev.type === 'start' && ev.run_started_at_ms > 0) {
      setMessages(xs => isActiveSession(sessionId) ? xs.map(m =>
        m.id === pendingId ? { ...m, run_started_at_ms: ev.run_started_at_ms } : m
      ) : xs)
    }
    if (ev.type === 'model' && typeof ev.model_id === 'string' && ev.model_id.trim()) {
      const modelID = ev.model_id.trim()
      setMessages(xs => isActiveSession(sessionId) ? xs.map(m =>
        m.id === pendingId ? { ...m, model_id: modelID, ...(Number.isInteger(ev.llm_no) ? { llm_no: ev.llm_no } : {}) } : m
      ) : xs)
    }
    if (ev.type === 'turn_usage' && ev.usage && typeof ev.index === 'number') {
      setMessages(xs => isActiveSession(sessionId) ? xs.map(m => {
        if (m.id !== pendingId) return m
        const usages = Array.isArray(m.usages) ? m.usages.slice() : []
        usages[ev.index] = ev.usage
        return { ...m, usages }
      }) : xs)
    }
    if (ev.message && (ev.type === 'done' || ev.type === 'error')) {
      if (typeof ev.reasoning_effort === 'string') setReasoningEffort(normalizeReasoningEffort(ev.reasoning_effort))
      setMessages(xs => isActiveSession(sessionId) ? xs.map(m => {
        if (m.id !== pendingId) return m
        const elapsedMs = getElapsedMs(m)
        const finalMsg = { ...ev.message }
        if ((!finalMsg.model_id || !String(finalMsg.model_id).trim()) && m.model_id) finalMsg.model_id = m.model_id
        if (!Number.isInteger(finalMsg.llm_no) && Number.isInteger(m.llm_no)) finalMsg.llm_no = m.llm_no
        if (elapsedMs > 0 && !(finalMsg.elapsed_ms > 0)) finalMsg.elapsed_ms = elapsedMs
        finalMsg.ultraplan_state = mergeUltraPlanStates(m.ultraplan_state, finalMsg.ultraplan_state) || finalMsg.ultraplan_state || m.ultraplan_state
        return finalMsg
      }) : xs)
    }
    if (ev.type === 'ultraplan_event' && ev.state) {
      setMessages(xs => isActiveSession(sessionId) ? xs.map(m => {
        if (m.id !== pendingId) return m
        const nextState = mergeUltraPlanStates(m.ultraplan_state, ev.state) || ev.state
        return { ...m, ultraplan_state: nextState }
      }) : xs)
    }
    if (ev.type === 'ultraplan_output' && ev.task_id && Array.isArray(ev.lines)) {
      setMessages(xs => isActiveSession(sessionId) ? xs.map(m => {
        if (m.id !== pendingId) return m
        const prevState = m.ultraplan_state || {}
        const prevOutputs = prevState.taskOutputs || prevState.task_outputs || {}
        const prevLines = Array.isArray(prevOutputs[ev.task_id]) ? prevOutputs[ev.task_id] : []
        const taskOutputs = { ...prevOutputs, [ev.task_id]: [...prevLines, ...ev.lines] }
        const nextState = mergeUltraPlanStates(prevState, { taskOutputs, task_outputs: taskOutputs }) || { ...prevState, taskOutputs, task_outputs: taskOutputs }
        return { ...m, ultraplan_state: nextState }
      }) : xs)
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
    let pendingId = `resume-${Date.now()}`
    setBusy(true); setStreamingSid(id); setAutoFollow(true); setShowFollow(false)
    setMessages(xs => {
      const existing = xs.find(m => m.role === 'assistant' && !m.content)
      if (existing?.id) {
        pendingId = existing.id
        return xs
      }
      return [...xs, { id:pendingId, role:'assistant', content:'', created_at:Math.floor(Date.now()/1000), run_started_at_ms:Date.now() }]
    })
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
    const nextReasoningEffort = normalizeReasoningEffort(st.settings?.reasoning_effort)
    setLlms(nextLlms)
    setLlmNo(nextLlms.some(m => m.index === nextNo) ? nextNo : (nextLlms[0]?.index ?? 0))
    setToolsMode(nextToolsMode)
    setReasoningEffort(nextReasoningEffort)
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
    setSessions(xs => xs.map(x => x.id === d.id ? { ...x, title: d.title, workspace: d.workspace || '', project_mode: d.project_mode || '', count: d.messages?.length || x.count, updated_at: d.updated_at || x.updated_at } : x))
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
    setSessionManagerOpen(false)
    setSelectedSessionIds([])
    const openToken = ++openSeqRef.current
    streamAbortRef.current?.abort?.()
    streamAbortRef.current = null
    const d = await api('/api/chat/session/new', { method:'POST', body:'{}' })
    if (openToken !== openSeqRef.current) return
    activeSidRef.current = d.id
    scrollModeRef.current = 'auto'
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

  const openSessionManager = () => {
    setSessionManagerOpen(true)
    setSelectedSessionIds([])
    setEditing('')
    setDraftTitle('')
    setMenuOpen('')
    setMenuPos(null)
  }

  const closeSessionManager = () => {
    if (batchDeleting) return
    setSessionManagerOpen(false)
    setSelectedSessionIds([])
  }

  const toggleSessionSelection = (id) => {
    if (!id || batchDeleting) return
    setSelectedSessionIds(ids => ids.includes(id) ? ids.filter(value => value !== id) : [...ids, id])
  }

  const toggleAllSessions = () => {
    if (batchDeleting) return
    setSelectedSessionIds(ids => {
      const selected = new Set(ids)
      return sessions.length > 0 && sessions.every(session => selected.has(session.id))
        ? []
        : sessions.map(session => session.id)
    })
  }

  const deleteSelectedSessions = async () => {
    if (batchDeleting) return
    const available = new Set(sessions.map(session => session.id))
    const ids = normalizeSessionIds(selectedSessionIds).filter(id => available.has(id))
    if (!ids.length || !confirmDanger('chat-session-batch-delete', `永久删除已选的 ${ids.length} 个会话？此操作不可恢复。`)) return

    setBatchDeleting(true)
    setErr('')
    setNotice('')
    try {
      const result = await deleteChatSessions(ids, id => api(`/api/chat/session/${id}`, { method:'DELETE' }))
      const deleted = new Set(result.deletedIds)
      const activeDeleted = deleted.has(sid)
      if (deleted.size) setSessions(xs => xs.filter(session => !deleted.has(session.id)))

      if (activeDeleted) {
        ++openSeqRef.current
        activeSidRef.current = ''
        streamAbortRef.current?.abort?.()
        streamAbortRef.current = null
        scrollModeRef.current = 'auto'
        setSid('')
        setMessages([])
        setRawHistory([])
        setHistoryInfo([])
        setWorkingState(null)
        setContextOpen(false)
        setBusy(false)
        setStreamingSid('')
        setAutoFollow(true)
        setShowFollow(false)
      }

      let refreshError = ''
      if (deleted.size) {
        try {
          await loadSessions(activeDeleted ? '' : sid, { open: activeDeleted })
        } catch (e) {
          refreshError = e?.message || String(e)
        }
      }

      if (result.failedIds.length) {
        setSelectedSessionIds(result.failedIds)
        const detail = result.failures[0]?.error?.message || ''
        setErr(`${result.failedIds.length} 个会话删除失败${detail ? `：${detail}` : ''}${refreshError ? `；刷新失败：${refreshError}` : ''}`)
      } else {
        setSelectedSessionIds([])
        setSessionManagerOpen(false)
        if (refreshError) setErr(`已删除 ${result.deletedIds.length} 个会话，但刷新列表失败：${refreshError}`)
        else setNotice(`已删除 ${result.deletedIds.length} 个会话`)
      }
    } finally {
      setBatchDeleting(false)
    }
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
    if (next === llmNo || modelSwitching) return
    const previous = llmNo
    setLlmNo(next)
    if (!sid) return
    setModelSwitching(true)
    setErr('')
    try {
      await api(`/api/chat/settings/${sid}`, { method:'POST', body: JSON.stringify({ llm_no: next, tools_mode: toolsMode, reasoning_effort: reasoningEffort }) })
      setNotice(`模型已切换到 #${next}，下一条消息将由该模型处理`)
    } catch (e) {
      setLlmNo(previous)
      setErr(`模型切换失败：${e.message || String(e)}`)
    } finally {
      setModelSwitching(false)
    }
  }

  const setToolsModeTo = async (next) => {
    if (next === toolsMode) { setToolsMenuOpen(false); return }
    const prev = toolsMode
    setToolsMode(next)
    setToolsMenuOpen(false)
    if (!sid) return
    try {
      await api(`/api/chat/settings/${sid}`, { method:'POST', body: JSON.stringify({ llm_no: llmNo, tools_mode: next, reasoning_effort: reasoningEffort }) })
      setNotice(next === 'fixed' ? '已设为自动注入：每次发消息都带上工具' : '已设为官方行为：会话开始按 GA 默认方式注入工具，需要时可点“立即注入一次”')
    } catch (e) {
      setToolsMode(prev)
      setErr(e.message || String(e))
    }
  }

  const saveReasoningEffort = async (value) => {
    const next = normalizeReasoningEffort(value)
    const prev = reasoningEffort
    setReasoningEffort(next)
    if (!sid) return
    try {
      await api(`/api/chat/settings/${sid}`, { method:'POST', body: JSON.stringify({ llm_no: llmNo, tools_mode: toolsMode, reasoning_effort: next }) })
      setNotice(next === 'off' ? '推理强度已设为默认' : `推理强度已设为 ${next}`)
    } catch (e) {
      setReasoningEffort(prev)
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

  const addAttachmentFiles = async (fileList) => {
    const files = Array.from(fileList || []).filter(Boolean)
    if (!files.length) return
    if (attachments.length + files.length > MAX_CHAT_UPLOAD_FILES) {
      setErr(`\u9644\u4ef6\u6700\u591a\u4e0a\u4f20 ${MAX_CHAT_UPLOAD_FILES} \u4e2a`)
      return
    }
    const tooLarge = files.find((file) => (Number(file.size) || 0) > MAX_CHAT_UPLOAD_BYTES_PER_FILE)
    if (tooLarge) {
      setErr(`\u9644\u4ef6\u8fc7\u5927\uff1a${tooLarge.name || 'attachment'}\uff0c\u5355\u4e2a\u9650\u5236 20MB`)
      return
    }
    const totalBytes = attachments.reduce((sum, file) => sum + (Number(file.size) || 0), 0)
      + files.reduce((sum, file) => sum + (Number(file.size) || 0), 0)
    if (totalBytes > MAX_CHAT_UPLOAD_BYTES_TOTAL) {
      setErr('\u9644\u4ef6\u603b\u5927\u5c0f\u9650\u5236 40MB')
      return
    }
    const readOne = (file) => new Promise((resolve, reject) => {
      const reader = new FileReader()
      reader.onload = () => resolve({
        id:`file-${Date.now()}-${Math.random().toString(16).slice(2)}`,
        name:file.name || `attachment-${Date.now()}`,
        type:file.type || 'application/octet-stream',
        size:Number(file.size) || 0,
        dataURL:String(reader.result || ''),
      })
      reader.onerror = () => reject(reader.error || new Error('\u8bfb\u53d6\u9644\u4ef6\u5931\u8d25'))
      reader.readAsDataURL(file)
    })
    try {
      const next = await Promise.all(files.map(readOne))
      setAttachments((current) => [...current, ...next].slice(0, MAX_CHAT_UPLOAD_FILES))
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
    const files = Array.from(e.clipboardData?.files || []).filter(Boolean)
    if (files.length) {
      e.preventDefault()
      addAttachmentFiles(files)
    }
  }
  const onDropFiles = (e) => {
    e.preventDefault(); setDragging(false)
    addAttachmentFiles(e.dataTransfer?.files)
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
        setSid(id); setStreamingSid(id)
      } else if (!isActiveSession(id)) {
        return
      }
      const clientUserID = `u-${Date.now()}`
      setStreamingSid(id)
      setSessions(xs => xs.map(s => s.id === id ? { ...s, running:true } : s))
      setAutoFollow(true); setShowFollow(false)
      const fileNote = files.length ? `\n\n[\u9644\u4ef6]\n${files.map((file) => `- ${uploadFileName(file)}`).join('\n')}` : ''
      const attachmentPrompt = text || '\u8bf7\u5904\u7406\u8fd9\u4e9b\u9644\u4ef6'
      const optimistic = { id:clientUserID, role:'user', content:attachmentPrompt + fileNote, files, created_at:Math.floor(Date.now()/1000) }
      const pending = { id:`a-${Date.now()}`, role:'assistant', content:'', created_at:Math.floor(Date.now()/1000), run_started_at_ms:Date.now() }
      setRawHistory([]); setHistoryInfo([]); setWorkingState(null)
      if (!isActiveSession(id)) return
      activeSidRef.current = id
      setMessages(xs => isActiveSession(id) ? [...xs, optimistic, pending] : xs)
      const res = await fetch(`/api/chat/${id}`, { method:'POST', headers:{'Content-Type':'application/json'}, signal: ctrl.signal, body: JSON.stringify({ prompt:attachmentPrompt, files, settings:{ llm_no: item.llmNo ?? llmNo, tools_mode: item.toolsMode || toolsMode, reasoning_effort: item.reasoningEffort || reasoningEffort }, client_user_id:clientUserID }) })
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

  const expandCustomSlashCommand = useCallback((value) => {
    const raw = String(value || '').trim()
    if (!raw.startsWith('/')) return raw
    const custom = (cfg?.slash_commands || [])
      .filter(c => c?.cmd && !isProtectedSlashCommand(c.cmd))
      .map(c => ({ ...c, cmd: String(c.cmd || '').trim() }))
      .sort((a, b) => b.cmd.length - a.cmd.length)
    const hit = custom.find(c => raw === c.cmd || raw.startsWith(`${c.cmd} `) || raw.startsWith(`${c.cmd}\n`))
    if (!hit) return raw
    const args = raw.slice(hit.cmd.length).trim()
    let body = String(hit.content || hit.prompt || hit.desc || '').trim()
    if (!body) return raw
    if (body.includes('{{args}}') || body.includes('{args}')) {
      body = body.replaceAll('{{args}}', args).replaceAll('{args}', args)
    } else if (args) {
      body = `${body}\n\n${args}`
    }
    return body
  }, [cfg?.slash_commands, isProtectedSlashCommand])

  const send = async (textOverride = null) => {
    if (modelSwitching) {
      setNotice('正在切换模型，请稍候发送')
      return
    }
    const hasStringOverride = typeof textOverride === 'string'
    const sourceText = hasStringOverride ? textOverride : prompt
    const text = expandCustomSlashCommand(String(sourceText || '').trim())
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
    const item = { text, files, llmNo, toolsMode, reasoningEffort }
    setPrompt(''); setAttachments([])
    setCmdDrawer({ open:false, filter:'', selectedIdx:0 })
    setCmdEditIdx(-1)
    if (busy) {
      enqueueMessage(item)
      return
    }
    await runSend(item)
  }

  const applySlashCommand = (cmd, currentValue = prompt) => {
    if (!cmd) return
    const next = slashCommandInsertText(cmd, currentValue)
    setPrompt(next)
    setCmdDrawer(slashCommandNextDrawer(cmd, next))
    setCmdEditIdx(-1)
    setTimeout(() => promptRef.current?.focus(), 0)
  }

  const handlePromptChange = (e) => {
    const v = e.target.value
    setPrompt(v)
    if (v.startsWith('/')) {
      setCmdDrawer({ open:true, filter:v.slice(1), selectedIdx:0 })
      setCmdEditIdx(-1)
    } else if (cmdDrawer.open) {
      setCmdDrawer({ open:false, filter:'', selectedIdx:0 })
      setCmdEditIdx(-1)
    }
  }

  const handlePromptKeyDown = (e) => {
    const currentValue = e.currentTarget.value
    if (cmdDrawer.open && cmdEditIdx === -1) {
      if (e.key === 'ArrowDown') {
        e.preventDefault()
        setCmdDrawer(prev => ({ ...prev, selectedIdx: Math.min(prev.selectedIdx + 1, Math.max(filteredCmds.length - 1, 0)) }))
        return
      }
      if (e.key === 'ArrowUp') {
        e.preventDefault()
        setCmdDrawer(prev => ({ ...prev, selectedIdx: Math.max(prev.selectedIdx - 1, 0) }))
        return
      }
      if (e.key === 'Enter' || e.key === 'Tab') {
        const cmd = filteredCmds[cmdDrawer.selectedIdx]
        const selectingNaturalReview = cmd?.cmd === '/review <自然语言请求>' && /^\s*\/review\s+\S/.test(currentValue)
        const selectingBareContinue = e.key === 'Enter' && /^\s*\/continue\s*$/.test(currentValue)
        const selectingBareEffort = e.key === 'Enter' && /^\s*\/effort\s*$/.test(currentValue)
        const selectingBareImprove = e.key === 'Enter' && /^\s*\/improve\s*$/.test(currentValue)
        const selectingContinueNumber = cmd?.cmd === '/continue <编号>' && /^\s*\/continue\s+\d+\s*$/.test(currentValue)
        const selectingUltraPlanObjective = cmd?.cmd === '/ultraplan <目标>' && /^\s*\/ultraplan\s+\S/.test(currentValue)
        if (selectingNaturalReview || selectingBareContinue || selectingBareEffort || selectingBareImprove || selectingContinueNumber || selectingUltraPlanObjective) {
          e.preventDefault()
          setCmdDrawer({ open:false, filter:'', selectedIdx:0 })
          setCmdEditIdx(-1)
          if (e.key === 'Enter') send(currentValue)
          return
        }
        if (cmd) {
          e.preventDefault()
          applySlashCommand(cmd, currentValue)
          return
        }
        e.preventDefault()
        setCmdDrawer({ open:false, filter:'', selectedIdx:0 })
        setCmdEditIdx(-1)
        if (e.key === 'Enter') send(currentValue)
        return
      }
      if (e.key === 'Escape') {
        setCmdDrawer({ open:false, filter:'', selectedIdx:0 })
        setCmdEditIdx(-1)
        return
      }
    }
    if (e.key === 'Enter' && !e.shiftKey) {
      e.preventDefault()
      send(currentValue)
    }
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
    let stopped = false
    let inFlight = false
    const refreshList = async () => {
      if (stopped || inFlight || document.hidden) return
      inFlight = true
      try {
        const d = await api('/api/chat/sessions')
        if (!stopped) setSessions(d.sessions || [])
      } catch {
        // Background refresh is best-effort; keep manual refresh errors visible only.
      } finally {
        inFlight = false
      }
    }
    const timer = window.setInterval(refreshList, 3000)
    const onVisible = () => { if (!document.hidden) refreshList() }
    document.addEventListener('visibilitychange', onVisible)
    return () => {
      stopped = true
      window.clearInterval(timer)
      document.removeEventListener('visibilitychange', onVisible)
    }
  }, [])

  useEffect(() => {
    if (!toolsMenuOpen) return
    const onDown = (e) => { if (!toolsMenuRef.current?.contains(e.target)) setToolsMenuOpen(false) }
    const onKey = (e) => { if (e.key === 'Escape') setToolsMenuOpen(false) }
    document.addEventListener('mousedown', onDown)
    document.addEventListener('keydown', onKey)
    return () => { document.removeEventListener('mousedown', onDown); document.removeEventListener('keydown', onKey) }
  }, [toolsMenuOpen])

  useEffect(() => {
    if (!sessionManagerOpen) return
    const previousOverflow = document.body.style.overflow
    const onKey = (e) => {
      if (e.key !== 'Escape' || batchDeleting) return
      setSessionManagerOpen(false)
      setSelectedSessionIds([])
    }
    document.body.style.overflow = 'hidden'
    document.addEventListener('keydown', onKey)
    return () => {
      document.body.style.overflow = previousOverflow
      document.removeEventListener('keydown', onKey)
    }
  }, [sessionManagerOpen, batchDeleting])

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

  const selectedSessionIdSet = useMemo(() => new Set(selectedSessionIds), [selectedSessionIds])
  const selectedSessionCount = sessions.reduce((count, session) => count + (selectedSessionIdSet.has(session.id) ? 1 : 0), 0)
  const allSessionsSelected = sessions.length > 0 && selectedSessionCount === sessions.length
  const activeModel = llms.find(x => x.index === llmNo) || llms[0]
  const selectedModelNo = activeModel?.index ?? llmNo
  const providerGroups = useMemo(() => {
    const groups = new Map()
    llms.forEach(model => {
      const provider = modelProvider(model)
      if (!groups.has(provider)) groups.set(provider, [])
      groups.get(provider).push({ value: model.index, label: runtimeModelLabel(model) })
    })
    return Array.from(groups, ([provider, models]) => ({ value: provider, label: provider, models }))
  }, [llms])
  const selectedProvider = activeModel ? modelProvider(activeModel) : (providerGroups[0]?.value || '')
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
      <button className="oa-new-chat" onClick={newSession} disabled={batchDeleting}><MessageSquarePlus size={16}/><span>新对话</span></button>
      <div className="oa-session-manager-head">
        <span className="oa-session-manager-title">历史会话 <small>{sessions.length}</small></span>
        <button className="oa-session-manage-open" type="button" onClick={openSessionManager} disabled={!sessions.length}>管理</button>
      </div>
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
      {!sessionManagerOpen && menuOpen && menuPos && (() => {
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
        <div className="oa-title"><b title={current ? shortTitle(current) : '新对话'}>{current ? shortTitle(current) : '新对话'}</b><span>ChatGPT-style workspace for GenericAgent</span>{current?.project_mode && <span className="oa-project-badge" title={`Project Mode: ${current.project_mode}`}>Project: {current.project_mode}</span>}{current?.workspace && <span className="oa-workspace-badge" title={current.workspace}>Workspace: {current.workspace}</span>}</div>
        <button className={`oa-context-btn ${contextOpen ? 'is-open' : ''}`} type="button" onClick={()=>setContextOpen(v=>!v)} disabled={!sid} title="查看发给模型的 raw_history">
          <PanelRightOpen size={16}/><span className="oa-context-label">上下文</span><span className="oa-context-count">{rawHistory?.length || 0}</span>
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
        <MessageList messages={messages} isCurrentRunning={isCurrentRunning} onAskReply={fillAskReply} clockNow={streamClock} />
        {showFollow && <div className="oa-follow-row"><button className="oa-follow-btn" type="button" onClick={resumeFollow}><ChevronDown size={16}/>继续跟随</button></div>}
        <div ref={endRef}/>
      </section>

      <footer className="oa-composer-wrap">
        {queuedMessages.length > 0 && <div className="oa-queue-dock" aria-label="待发送队列">
          {queuedMessages.map((q, i) => {
            const isEditingQueue = queueEditingId === q.id
            return <div key={q.id} className={`oa-queued-item ${isEditingQueue ? 'is-editing' : ''}`}>
              <div className="oa-queue-content" title={isEditingQueue ? '' : (q.text || '\u8bf7\u5904\u7406\u8fd9\u4e9b\u9644\u4ef6')}>
                {isEditingQueue ? <textarea className="oa-queue-edit-input" value={queueDraft} autoFocus rows={2} onChange={e=>setQueueDraft(e.target.value)} onKeyDown={e=>{ if(e.key==='Enter' && (e.ctrlKey || e.metaKey)) saveQueueEdit(q.id); if(e.key==='Escape') cancelQueueEdit() }} /> : <>
                  <b>{q.text || '\u8bf7\u5904\u7406\u8fd9\u4e9b\u9644\u4ef6'}</b>
                  {q.files?.length ? <em>{q.files.length} {'\u4e2a\u9644\u4ef6'}</em> : null}
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
        {cmdDrawer.open && <div className="oa-cmd-drawer" ref={cmdDrawerRef}>
          {filteredCmds.length === 0 && <div className="oa-cmd-item" style={{color:'var(--text-secondary)',justifyContent:'center',cursor:'default',padding:'12px 14px'}}>无匹配命令</div>}
          {filteredCmds.map((c,i)=>{
            return (
              <div key={c.cmd+i} ref={i===cmdDrawer.selectedIdx ? selectedCmdRef : null} className={`oa-cmd-item${i===cmdDrawer.selectedIdx?' selected':''}`} onMouseEnter={() => setCmdDrawer(d => ({ ...d, selectedIdx: i }))} onMouseDown={e=>{e.preventDefault();applySlashCommand(c,promptRef.current?.value ?? prompt)}}>
                <span className="oa-cmd-name">{c.cmd}</span>
                <span className="oa-cmd-desc">{c.desc}</span>
              </div>
            )
          })}
        </div>}
        {cmdManagerOpen && <div className="oa-cmd-manager-backdrop" onMouseDown={()=>setCmdManagerOpen(false)}>
          <div className="oa-cmd-manager" role="dialog" aria-modal="true" aria-label="自定义斜杠命令" onMouseDown={e=>e.stopPropagation()}>
            <div className="oa-cmd-manager-head">
              <div><h3>自定义斜杠命令</h3><p>官方命令只读锁定；用户命令可新增、编辑、删除。</p></div>
              <button className="oa-icon-btn" type="button" onClick={()=>setCmdManagerOpen(false)} title="关闭"><X size={16}/></button>
            </div>
            <div className="oa-cmd-manager-actions">
              <button className="oa-guide-btn" type="button" onClick={()=>startEdit(-2, '/', '', '')}><Plus size={14}/>新增自定义命令</button>
              <span>{(cfg?.slash_commands || []).filter(c => !isProtectedSlashCommand(c?.cmd)).length} 个自定义 · {effectiveSlashCommands.length} 个官方</span>
            </div>
            {cmdEditIdx !== -1 && <div className="oa-cmd-edit-card">
              <input value={cmdEditCmd} onChange={e=>setCmdEditCmd(e.target.value)} placeholder="命令，例如 /hello" autoFocus />
              <input value={cmdEditDesc} onChange={e=>setCmdEditDesc(e.target.value)} placeholder="描述，例如 代码审查模板" />
              <textarea value={cmdEditContent} onChange={e=>setCmdEditContent(e.target.value)} placeholder="发送时展开成的指令内容。可用 {args} 插入 /命令 后面的参数。" rows={4}/>
              <button type="button" onClick={saveEdit}>保存</button>
              <button type="button" onClick={()=>setCmdEditIdx(-1)}>取消</button>
            </div>}
            <div className="oa-cmd-manager-list">
              <div className="oa-cmd-section-title">用户自定义</div>
              {(cfg?.slash_commands || []).filter(c => !isProtectedSlashCommand(c?.cmd)).length === 0 && <div className="oa-cmd-empty">暂无自定义命令，点击上方新增。</div>}
              {(cfg?.slash_commands || []).map((c, i) => {
                if (isProtectedSlashCommand(c?.cmd)) return null
                return <div className="oa-cmd-manage-row" key={`${c.cmd}-${i}`}>
                  <div><b>{c.cmd}</b><small>{c.desc || '无描述'}</small>{(c.content || c.prompt) && <em>{c.content || c.prompt}</em>}</div>
                  <button type="button" onClick={()=>startEdit(i, c.cmd || '/', c.desc || '', c.content || c.prompt || '')}><Edit3 size={14}/>编辑</button>
                  <button type="button" onClick={()=>deleteCmd(i)}><Trash2 size={14}/>删除</button>
                </div>
              })}
              <div className="oa-cmd-section-title">官方命令</div>
              {effectiveSlashCommands.map((c, i) => <div className="oa-cmd-manage-row is-locked" key={`${c.cmd}-${i}`}>
                <div><b>{c.cmd}</b><small>{c.desc || '官方命令'}</small></div>
                <span><Lock size={13}/>只读</span>
              </div>)}
            </div>
          </div>
        </div>}
        <div className={`oa-composer ${dragging ? 'is-dragging' : ''}`} onDragOver={e=>{e.preventDefault(); setDragging(true)}} onDragLeave={()=>setDragging(false)} onDrop={onDropFiles}>
          <input ref={fileRef} type="file" multiple hidden onChange={e=>{ addAttachmentFiles(e.target.files); e.target.value='' }} />
          {attachments.length > 0 && <div className="oa-attach-preview">
            {attachments.map((attachment) => {
              const name = uploadFileName(attachment)
              const image = isImageFile(attachment)
              const visual = getFileVisual(name)
              const Icon = visual.Icon
              const extension = (name.match(/\.([^.]+)$/)?.[1] || 'FILE').slice(0, 6).toUpperCase()
              return <div className={`oa-attach-thumb ${image ? 'is-image' : `is-file oa-file-kind-${visual.kind}`}`} key={attachment.id} title={name}>
                {image ? <img src={uploadFileSource(attachment)} alt={name}/> : <div className="oa-attach-file-icon"><Icon size={25}/><small>{extension}</small></div>}
                <span>{image ? <FileImage size={12}/> : <Icon size={12}/>} {name}</span>
                <button type="button" onClick={()=>removeAttachment(attachment.id)} title={'\u79fb\u9664\u9644\u4ef6'} aria-label={`\u79fb\u9664\u9644\u4ef6 ${name}`}><X size={12}/></button>
              </div>
            })}
          </div>}
          {isUltraPlanPrompt && <div className="oa-ultraplan-mode" aria-live="polite"><span><Sparkles size={14}/>UltraPlan</span><b>\u5c06\u4ee5\u89c4\u5212\u6a21\u5f0f\u6267\u884c\uff0c\u5e76\u5728\u5b8c\u6210\u540e\u5c55\u793a run \u76ee\u5f55\u4e0e\u65e5\u5fd7\u6458\u8981</b></div>}
          <textarea ref={promptRef} value={prompt} onPaste={onPaste} onChange={handlePromptChange} onKeyDown={handlePromptKeyDown} placeholder={isMobile ? '发送消息或添加文件…' : '\u5411 GenericAgent \u53d1\u9001\u6d88\u606f\uff0c\u53ef\u9009\u62e9/\u7c98\u8d34/\u62d6\u62fd\u4efb\u610f\u6587\u4ef6\u2026'} rows={1}/>
          <div className="oa-composer-bar">
            <button className="oa-attach-btn" type="button" onClick={()=>fileRef.current?.click()} title={'\u6dfb\u52a0\u9644\u4ef6'}><Paperclip size={17}/><span>{'\u9644\u4ef6'}</span></button>
            <button className={`oa-attach-btn ${cmdManagerOpen ? 'is-open' : ''}`} type="button" onClick={()=>setCmdManagerOpen(true)} title="管理自定义斜杠命令"><Sparkles size={16}/><span>命令</span></button>
            <div className="oa-tools-menu" ref={toolsMenuRef}>
              <button className={`oa-tools-trigger ${toolsMenuOpen ? 'is-open' : ''}`} type="button" disabled={!sid} onClick={()=>setToolsMenuOpen(o=>!o)} aria-haspopup="menu" aria-expanded={toolsMenuOpen} title="工具注入设置">
                <Wrench size={16}/><span>工具</span>{isFixedToolsMode && <span className="oa-tools-state">自动</span>}<ChevronDown size={14}/>
              </button>
              {toolsMenuOpen && isMobile && <div className="oa-tools-backdrop" onClick={()=>setToolsMenuOpen(false)}/>}
              {toolsMenuOpen && (
                <div className={`oa-tools-pop ${isMobile ? 'oa-tools-modal' : ''}`} role={isMobile ? 'dialog' : 'menu'} aria-modal={isMobile || undefined}>
                  {isMobile && <div className="oa-tools-modal-bar"/>}
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
            <ProviderModelCascade groups={providerGroups} selectedProvider={selectedProvider}
              value={selectedModelNo} disabled={!providerGroups.length || isCurrentRunning || modelSwitching}
              onChange={v=>saveModel(Number(v))} mobile={isMobile} />
            <div className="oa-model-select oa-effort-select"><span>推理</span>
              <CustomSelect value={reasoningEffort} onChange={v=>saveReasoningEffort(v)}
                options={REASONING_EFFORT_OPTIONS} native={isMobile} ariaLabel="推理强度" />
            </div>
            <button className="oa-send" type="button" disabled={modelSwitching || (!prompt.trim() && !attachments.length)} onClick={() => send()} title={modelSwitching ? '正在切换模型' : isCurrentRunning ? '加入发送队列' : '发送'} aria-label={modelSwitching ? '正在切换模型' : isCurrentRunning ? '加入发送队列' : '发送'}><Send size={17}/></button>
            {isCurrentRunning && <button className="oa-stop" type="button" onClick={()=>cancelRun(sid)} title="停止生成" aria-label="停止生成"><Square size={14}/></button>}
          </div>
        </div>
        <p>Enter 发送 · Shift + Enter 换行 · 回复中发送会排队 · 工具：{isFixedToolsMode ? '每次自动注入' : '官方默认'}</p>
      </footer>
    </main>

    {sessionManagerOpen && <div className="oa-session-manager-backdrop" onMouseDown={e=>{ if (e.target === e.currentTarget) closeSessionManager() }}>
      <section className="oa-session-manager-modal" role="dialog" aria-modal="true" aria-labelledby="oa-session-manager-dialog-title" onMouseDown={e=>e.stopPropagation()}>
        <header className="oa-session-manager-dialog-head">
          <div>
            <h2 id="oa-session-manager-dialog-title">管理历史会话</h2>
            <p>选择不再需要的会话并批量删除</p>
          </div>
          <button className="oa-icon-btn" type="button" onClick={closeSessionManager} disabled={batchDeleting} aria-label="关闭会话管理" autoFocus><X size={17}/></button>
        </header>
        <div className="oa-session-manager-toolbar">
          <button className="oa-session-dialog-select-all" type="button" role="checkbox" aria-checked={allSessionsSelected ? true : (selectedSessionCount ? 'mixed' : false)} onClick={toggleAllSessions} disabled={!sessions.length || batchDeleting}>
            <span className={`oa-session-check ${allSessionsSelected ? 'is-checked' : ''} ${!allSessionsSelected && selectedSessionCount ? 'is-partial' : ''}`}>{allSessionsSelected && <Check size={12}/>}</span>
            <span>{allSessionsSelected ? '取消全选' : '全选'}</span>
          </button>
          <span className="oa-session-dialog-count">已选 {selectedSessionCount} / {sessions.length}</span>
        </div>
        <div className="oa-session-manager-dialog-list">
          {sessions.map(s => {
            const selected = selectedSessionIdSet.has(s.id)
            return <button key={s.id} className={`oa-session-manager-dialog-row ${selected ? 'is-selected' : ''}`} type="button" role="checkbox" aria-checked={selected} onClick={()=>toggleSessionSelection(s.id)} disabled={batchDeleting}>
              <span className={`oa-session-check ${selected ? 'is-checked' : ''}`}>{selected && <Check size={12}/>}</span>
              <span className="oa-session-dialog-copy">
                <span className="oa-session-dialog-title">{s.running && <i className="oa-session-running-dot" aria-hidden="true"/>}<b>{shortTitle(s)}</b>{s.id === sid && <em>当前</em>}</span>
                <small><Clock3 size={12}/>{fmtTime(s.updated_at) || '刚刚'} · {s.count || 0} 条{s.running && <span>运行中</span>}</small>
              </span>
            </button>
          })}
          {!sessions.length && <div className="oa-session-manager-dialog-empty">暂无历史会话</div>}
        </div>
        <footer className="oa-session-manager-dialog-foot">
          <small>删除后无法恢复</small>
          <div>
            <button className="oa-session-dialog-cancel" type="button" onClick={closeSessionManager} disabled={batchDeleting}>取消</button>
            <button className="oa-session-dialog-delete" type="button" onClick={deleteSelectedSessions} disabled={!selectedSessionCount || batchDeleting}>
              <Trash2 size={15}/><span>{batchDeleting ? '正在删除…' : `删除所选${selectedSessionCount ? ` (${selectedSessionCount})` : ''}`}</span>
            </button>
          </div>
        </footer>
      </section>
    </div>}
  </div>
}
