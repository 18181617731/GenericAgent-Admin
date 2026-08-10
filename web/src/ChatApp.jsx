import React, { createContext, memo, useCallback, useContext, useEffect, useLayoutEffect, useMemo, useRef, useState } from 'react'
import { createPortal } from 'react-dom'
import { applyThemeToDocument, getInitialTheme } from './themes'
import ThemePicker from './ThemePicker'
import ScalePicker from './ScalePicker.jsx'
import { createStreamDeltaBatcher, isBTWCommand, mergeFinalStreamMessage, pickResumePlaceholderId, scrollFollowAction, shouldFinishStreamFollow } from './lib/chatStream.js'
import { cacheReadTokens } from './lib/chatUsage.js'
import { computeLineDiff, computeWriteRows } from './lib/lineDiff.js'
import { Collapse, Tag } from 'antd'
import gsap from 'gsap'
import { useGSAP } from '@gsap/react'
import { Bot, Check, ChevronDown, ChevronLeft, ChevronRight, CircleAlert, CircleHelp, Clock3, Copy, CornerDownLeft, Download, Edit3, ExternalLink, FileArchive, FileCode2, FileImage, FileOutput, FilePenLine, FileSpreadsheet, FileText, FolderOpen, GitBranch, Lock, Paperclip, Menu, MessageSquarePlus, MoreHorizontal, PanelRightOpen, Pin, Plus, RotateCw, Search, Send, Sparkles, Square, Target, Trash2, Wrench, X } from 'lucide-react'
import { api, apiStream } from './lib/api'
import { addChatInstanceToURL, chatInstanceOptions, initialChatInstanceID, persistChatInstanceID } from './lib/chatInstanceScope'
import { confirmDanger } from './lib/danger'
import { formatDuration, fuzzyMatch, goalBudgetPercent, goalTurnPercent } from './lib/format'
import { JSON_TREE_CHILD_LIMIT, JSON_TREE_STRING_LIMIT, LIST_ITEM_LIMIT, LONG_TEXT_PREVIEW_CHARS, MARKDOWN_BLOCK_LIMIT, MARKDOWN_CHAR_LIMIT, MARKDOWN_LINE_LIMIT, assistantTurnFallbackTitle, isToolResultText, parseAssistantContent, previewLongText, splitMarkdownParts, textRenderStats } from './lib/chatTextSafety'
import { getAskUserPayload } from './lib/askUserPayload'
import { preferredUltraPlanOutputFile, reconcileUltraPlanTasks } from './lib/ultraPlanTasks'
import { REASONING_EFFORT_LEVELS, REASONING_EFFORT_OPTIONS, modelReasoningEffort, modelReasoningEffortSetting, normalizeReasoningEffort } from './lib/reasoningEffort'
import { deleteChatSessions, normalizeSessionIds } from './lib/chatSessionManagement'
import { clearChatSessionDrafts, listChatSessionDraftIds, loadChatSessionDraft, saveChatSessionDraft } from './lib/chatSessionDrafts'
import { groupProjectSessions } from './lib/chatProjectSessions.js'
import { createPromptPreset, normalizePromptPresets, promptPresetPatch, selectedPromptPresetView } from './lib/promptPresets'
import { commandResultSummary, reduceCommandResult } from './lib/chatCommands'
import { buildChatRunPayload, buildEditResendItem } from './lib/worldlineEdit'
import { extractGeneratedImagePaths, generatedImageDownloadURL, generatedImageURL } from './lib/generatedImages'
import { ProviderModelCascade, buildModelProviderGroups, findModelProviderValue, modelProvider, runtimeModelLabel } from './components/ModelProviderCascade.jsx'
import { SubagentStatusPanel } from './components/SubagentStatusPanel.jsx'
import { buildWorldlineRows, messageVersionInfo } from './lib/worldlineTree'
import { hasSubagentLaunch } from './lib/subagentCards'
import { chatErrorPresentation } from './lib/chatErrors.js'
import { pollGeneratedChatTitle, shouldPollGeneratedTitle } from './lib/chatTitlePolling.js'
import { consumeMemoryChatDraft } from './lib/memoryChatDraft.js'
import { firstRuntimeModelNo } from './lib/modelDefaults.js'
import { clearSessionSearchHistory, loadSessionSearchHistory, saveSessionSearchHistory, sessionSearchScopeOptions } from './lib/chatSessionSearch.js'
import { primeChatCompletionTone } from './lib/chatCompletionTone.js'
import { buildChatNotification, latestUserPrompt } from './lib/chatNotification.js'
import { publishNotification } from './lib/notifications.js'
import { NotificationCenter } from './components/NotificationUI.jsx'
import SessionSearchDialog from './components/SessionSearchDialog.jsx'

export { ProviderModelCascade } from './components/ModelProviderCascade.jsx'

gsap.registerPlugin(useGSAP)

const prefersReducedMotion = () => typeof window !== 'undefined' && window.matchMedia?.('(prefers-reduced-motion: reduce)')?.matches
const isNarrowChatViewport = () => typeof window !== 'undefined' && window.matchMedia?.('(max-width: 900px)')?.matches
const isMobileViewport = () => typeof window !== 'undefined' && window.matchMedia?.('(max-width: 560px)')?.matches
const chatLanguage = () => typeof localStorage !== 'undefined' && localStorage.getItem('ga-admin-lang') === 'en' ? 'en' : 'zh'
const ct = (zh, en) => chatLanguage() === 'en' ? en : zh
const chatLocale = () => chatLanguage() === 'en' ? 'en-US' : 'zh-CN'
const ChatFeatureHelp = ({ text }) => <span className="oa-chat-help" aria-hidden="true" data-tooltip={text} title={text}><CircleHelp size={13}/></span>
export const ChatFileScopeContext = createContext({ workspace: '', gaRoot: '' })

const timestampMs = (v) => {
  if (v instanceof Date) return v.getTime()
  if (typeof v === 'number') return Number.isFinite(v) ? (Math.abs(v) < 1e12 ? v * 1000 : v) : NaN
  if (typeof v === 'string') {
    const raw = v.trim()
    if (!raw) return NaN
    const numeric = Number(raw)
    if (Number.isFinite(numeric)) return Math.abs(numeric) < 1e12 ? numeric * 1000 : numeric
  }
  return new Date(v).getTime()
}
const dateFromTimestamp = (v) => {
  const ms = timestampMs(v)
  return Number.isFinite(ms) ? new Date(ms) : null
}
const fmtTime = (v) => dateFromTimestamp(v)?.toLocaleString(chatLocale()) || ''
const fmtTimelineDate = (v) => {
  if (!v) return ct('今天', 'Today')
  const d = dateFromTimestamp(v)
  if (!d) return ''
  const now = new Date()
  const day = new Date(d.getFullYear(), d.getMonth(), d.getDate()).getTime()
  const today = new Date(now.getFullYear(), now.getMonth(), now.getDate()).getTime()
  const diff = Math.round((today - day) / 86400000)
  if (diff === 0) return ct('今天', 'Today')
  if (diff === 1) return ct('昨天', 'Yesterday')
  return d.toLocaleDateString(chatLocale(), { year:'numeric', month:'long', day:'numeric' })
}
const timelineKey = (v) => {
  if (!v) return 'today'
  const d = dateFromTimestamp(v)
  return d ? `${d.getFullYear()}-${d.getMonth()+1}-${d.getDate()}` : 'unknown'
}
const isNearBottom = (el, gap = 96) => !el || (el.scrollHeight - el.scrollTop - el.clientHeight) <= gap
const parseBTWDisplay = (value) => {
  const raw = String(value || '')
  const match = raw.match(/^\s*(?:>\s*)?(?:🟡\s*)?\/btw(?:[ \t]+([\s\S]*))?\s*$/i)
  if (!match) return null
  return { prompt: String(match[1] || '').trim() }
}
const stripBTWEcho = (value) => {
  const lines = String(value || '').split(/\r?\n/)
  const firstContent = lines.findIndex(line => line.trim())
  if (firstContent < 0 || !parseBTWDisplay(lines[firstContent])) return String(value || '')
  lines.splice(firstContent, 1)
  while (firstContent < lines.length && !lines[firstContent].trim()) lines.splice(firstContent, 1)
  return lines.join('\n').trimStart()
}
const shortTitle = (s) => s?.title || '新会话'
const runtimeModelMatches = (m, modelID) => {
  const target = String(modelID || '').trim()
  if (!target) return false
  return [m?.model, m?.name, runtimeModelLabel(m)].some(value => String(value || '').trim() === target)
}
const messageModelIdentity = (message, models = []) => {
  if (message?.role !== 'assistant') return { label: '', title: '' }
  const modelID = typeof message.model_id === 'string' ? message.model_id.trim() : ''
  const modelNo = Number.isInteger(message.llm_no) ? message.llm_no : null
  const indexed = modelNo == null ? null : models.find(model => model.index === modelNo)
  const matched = indexed && (!modelID || runtimeModelMatches(indexed, modelID))
    ? indexed
    : models.find(model => runtimeModelMatches(model, modelID))
  const provider = matched ? modelProvider(matched) : ''
  const model = modelID || (matched ? runtimeModelLabel(matched) : '')
  const label = [provider, model].filter(Boolean).join(' · ') || '未知模型'
  const details = [`模型：${model || '未知'}`]
  if (provider) details.unshift(`服务商：${provider}`)
  if (modelNo != null) details.push(`内部编号：#${modelNo}`)
  return { label, title: details.join('；') }
}

const BUILTIN_SLASH_COMMANDS = [
	{ cmd: '/project', key: '/project', insert: '/project', desc: '列出项目并查看或切换 Project Mode', builtIn: true },
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
    desc: level === 'off' ? ct('清除 reasoning effort', 'Clear reasoning effort') : ct(`设置 reasoning effort 为 ${level}`, `Set reasoning effort to ${level}`),
    builtIn: true,
  })),
  { cmd: '/workspace <path>', key: '/workspace', insert: '/workspace ', desc: ct('为当前会话绑定项目目录', 'Bind a project directory to the current session'), builtIn: true },
  { cmd: '/workspace off', key: '/workspace off', insert: '/workspace off', desc: ct('关闭当前会话 workspace', 'Disable the current session workspace'), builtIn: true },
]
const builtinSlashKey = (cmd = '') => String(cmd || '').trim().toLowerCase()
const builtinSlashCommandKey = (c) => builtinSlashKey(c?.key || c?.cmd)
// 参数式命令：裸根命令（/goal）或以 <参数>/[参数] 占位结尾（/goal [goal]、/continue <编号>、/rewind [n]）。
// 这类命令后面的自由文本必须保留，禁止被 insert 模板覆盖（否则会清空用户已输入的内容）。
const SLASH_ARG_SUFFIX_RE = /\s(?:<[^>]+>|\[[^\]]+\])$/
const isArgumentStyleSlashCmd = (cmd = '') => {
  const s = String(cmd || '')
  if (!s) return false
  const root = s.split(/\s+/, 1)[0]
  return s === root || SLASH_ARG_SUFFIX_RE.test(s)
}
const slashCommandInsertText = (c, current = '') => {
  if (!c) return current || ''
  const text = String(current || '')
  const cmd = String(c.cmd || '')
  const root = cmd.split(/\s+/, 1)[0]
  const isArgumentFallback = isArgumentStyleSlashCmd(cmd)
  if (isArgumentFallback && new RegExp(`^\\s*${root.replace(/[.*+?^${}()|[\]\\]/g, '\\$&')}\\s+`).test(text)) {
    return text
  }
  return c?.insert ?? `${c?.cmd || ''} `
}
const slashCommandProgressiveFilter = (c, nextText = '') => {
  if (c?.cmd === '/review <request>') return 'review '
  if (c?.cmd === '/continue') return 'continue '
  if (c?.cmd === '/improve') return 'improve '
  if (c?.cmd === '/effort') return 'effort '
  if (c?.cmd === '/workspace <path>') return 'workspace '
  if (c?.cmd === '/ultraplan <goal>') return 'ultraplan '
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
  // Keep raw HTML escaped by React. The only HTML-shaped token accepted here is
  // Markdown's commonly used hard line break, <br> (including <br/> variants).
  const re = /(`([^`]+)`)|(\*\*([^*]+)\*\*)|(\*([^*]+)\*)|(\[([^\]]+)\]\((https?:\/\/[^\s)]+)\))|(~~([^~]+)~~)|(<br\s*\/?>)/gi
  let last = 0, m
  while ((m = re.exec(src)) !== null) {
    if (m.index > last) tokens.push({ type:'text', text:src.slice(last, m.index) })
    if (m[2]) tokens.push({ type:'code', text:m[2] })
    else if (m[4]) tokens.push({ type:'strong', text:m[4] })
    else if (m[6]) tokens.push({ type:'em', text:m[6] })
    else if (m[8] && m[9]) tokens.push({ type:'link', text:m[8], href:m[9] })
    else if (m[11]) tokens.push({ type:'del', text:m[11] })
    else if (m[12]) tokens.push({ type:'br' })
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
      if (t.type === 'del') return <del key={i}>{t.text}</del>
      if (t.type === 'br') return <br key={i} />
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
  return <button className={compact ? 'oa-mini-copy' : 'oa-copy'} onClick={copy} title="复制" aria-label={ok ? '已复制' : '复制'}>
    {ok ? <Check size={14}/> : <Copy size={14}/>}<span>{ok ? '已复制' : '复制'}</span>
  </button>
}

function LongTextPreview({ text = '', stats }) {
  const s = stats || textRenderStats(text)
  const preview = useMemo(() => previewLongText(text), [text])
  return <div className="oa-long-text-preview">
    <div className="oa-long-text-head">
      <b>{ct('内容过大，已切换安全预览', 'Content is large; showing a safe preview')}</b>
      <span>{s.chars.toLocaleString(chatLocale())} {ct('字符', 'characters')} · {s.linesLabel} {ct('行', 'lines')}</span>
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
      {hidden > 0 && <div className="oa-json-line oa-json-more" style={{ '--depth': depth + 1 }}>{ct(`… 已隐藏 ${hidden.toLocaleString(chatLocale())} 项，复制原始 JSON 查看全部`, `… ${hidden.toLocaleString(chatLocale())} items hidden; copy the raw JSON to view all`)}</div>}
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

const isAbsoluteLocalPath = (value = '') => /^(?:[a-z]:[\\/]|[\\/]{2}|\/)/i.test(String(value || '').trim())

const normalizedLocalPath = (value = '') => String(value || '').trim().replace(/[\\/]+/g, '/').replace(/\/$/, '')

const isWithinLocalPath = (candidate, scope) => {
  const cleanCandidate = normalizedLocalPath(candidate).toLowerCase()
  const cleanScope = normalizedLocalPath(scope).toLowerCase()
  return Boolean(cleanCandidate && cleanScope && (cleanCandidate === cleanScope || cleanCandidate.startsWith(`${cleanScope}/`)))
}

const resolveLocalPath = (base, relative) => {
  const cleanBase = normalizedLocalPath(base)
  if (!isAbsoluteLocalPath(cleanBase)) return ''
  const drive = cleanBase.match(/^[a-z]:/i)?.[0] || ''
  const separator = drive || String(base || '').includes('\\') ? '\\' : '/'
  const baseParts = cleanBase.replace(/^[a-z]:/i, '').split('/').filter(Boolean)
  for (const part of normalizedLocalPath(relative).split('/')) {
    if (!part || part === '.') continue
    if (part === '..') {
      if (!baseParts.length) return ''
      baseParts.pop()
      continue
    }
    baseParts.push(part)
  }
  const prefix = drive ? `${drive}${separator}` : cleanBase.startsWith('/') ? separator : ''
  return `${prefix}${baseParts.join(separator)}`
}

export function resolveChatToolFilePath(path, scope = {}) {
  const raw = String(path || '').trim()
  const workspace = String(scope?.workspace || '').trim()
  const gaRoot = String(scope?.gaRoot || '').trim()
  if (!raw || isAbsoluteLocalPath(raw) || !/^(?:\.\.?[\\/])/.test(raw) || !isAbsoluteLocalPath(workspace)) return raw
  const resolved = resolveLocalPath(workspace, raw)
  return isWithinLocalPath(resolved, workspace) || isWithinLocalPath(resolved, gaRoot) ? resolved : raw
}

export function extractToolResultFilePath(result = '') {
  const match = String(result || '').match(/\b(?:patch(?:ing|ed)?|writ(?:ing|ten)|creat(?:ing|ed)|delet(?:ing|ed)|remov(?:ing|ed)|renam(?:ing|ed)|mov(?:ing|ed)|cop(?:ying|ied))\s+(?:the\s+)?(?:file|folder|directory)?\s*:\s*([a-z]:[^\r\n]+)/i)
  const path = String(match?.[1] || '').trim().replace(/^["'`]+|["'`\])}>.,;:]+$/g, '')
  return isAbsoluteLocalPath(path) ? path : ''
}

const FILE_MUTATION_TOOL_RE = /\bfile_(?:write|patch|delete|remove|move|rename|copy|mkdir|create)\b/i
const hasFileMutation = (content = '') => FILE_MUTATION_TOOL_RE.test(String(content || ''))

function StepFileMutationMarker() {
  const label = ct('本步骤包含文件增删改操作', 'This step changes files')
  return <span className="oa-turn-file-marker" title={label} aria-label={label}><FilePenLine size={14}/></span>
}

function FileAttachment({ path, resolvedPath = '' }) {
  const displayPath = String(path || '').trim()
  const fileScope = useContext(ChatFileScopeContext)
  const [imagePreviewOpen, setImagePreviewOpen] = useState(false)
  const clean = resolveChatToolFilePath(resolvedPath || displayPath, fileScope)
  const name = displayPath.split(/[\\/]/).filter(Boolean).pop() || displayPath || ct('文件', 'File')
  const extMatch = name.match(/\.([^.]+)$/)
  const extension = extMatch ? extMatch[1].slice(0, 6).toUpperCase() : 'FILE'
  const splitAt = Math.max(displayPath.lastIndexOf('\\'), displayPath.lastIndexOf('/'))
  const directory = splitAt >= 0 ? displayPath.slice(0, splitAt) : ct('本地文件', 'Local file')
  const visual = getFileVisual(name)
  const { kind, Icon } = visual
  const isImage = kind === 'image'
  const imageUrl = `/api/files/image?path=${encodeURIComponent(clean)}`
  const open = async (mode) => {
    if (!confirmDanger('chat-file-open', ct(`使用系统桌面打开${mode === 'folder' ? '文件所在位置' : '文件'}：${clean}？`, `Open ${mode === 'folder' ? 'the containing folder' : 'this file'} in the desktop system: ${clean}?`))) return
    try {
      await api('/api/files/open', { dangerous:true, method:'POST', body: JSON.stringify({ path: clean, mode }) })
    } catch (e) {
      alert(ct(`打开失败：${e?.message || e}`, `Open failed: ${e?.message || e}`))
    }
  }
  const imageLabel = ct(`查看图片 ${name}`, `View image ${name}`)
  return <>
    <span className={`oa-file-card oa-file-kind-${kind}`} title={displayPath || clean}>
    <button type="button" className="oa-file-leading" onClick={() => isImage ? setImagePreviewOpen(true) : open('file')} aria-label={isImage ? imageLabel : ct(`打开文件 ${name}`, `Open file ${name}`)}>
      <Icon className="oa-file-fallback-icon" size={19}/>
      {isImage && <img src={imageUrl} alt="" loading="lazy" onError={(e)=>{ e.currentTarget.style.display='none' }} />}
    </button>
    {!isImage && <span className="oa-file-meta">
        <span className="oa-file-name-row"><b>{name}</b><small>{extension}</small></span>
        <em>{directory || ct('本地文件', 'Local file')}</em>
      </span>}
    <span className="oa-file-actions">
      <a href={`/api/files/download?path=${encodeURIComponent(clean)}`} download={name} title="下载文件" aria-label={`下载文件 ${name}`}><Download size={15}/></a>
      <button type="button" onClick={() => open('file')} title={ct('打开文件', 'Open file')} aria-label={`打开文件 ${name}`}><ExternalLink size={15}/></button>
      <button type="button" onClick={() => open('folder')} title={ct('打开所在位置', 'Open containing folder')} aria-label={`打开 ${name} 所在位置`}><FolderOpen size={15}/></button>
      <CopyButton text={displayPath || clean} compact />
    </span>
    </span>
    {isImage && <ImagePreviewDialog images={[{ name, src:imageUrl }]} activeIndex={imagePreviewOpen ? 0 : -1} onClose={() => setImagePreviewOpen(false)} />}
  </>
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
          <div className="oa-code-head"><span>{p.lang || ct('代码', 'Code')}</span><CopyButton text={p.text} compact /></div>
          <pre><code>{p.text}</code></pre>
        </div>
      : p.type === 'tool'
        ? <ToolCallBlock key={idx} call={p.call} onAskReply={onAskReply} />
        : <TextMarkdown key={idx} text={p.text} onAskReply={onAskReply}/>) }
    {parts.length >= MARKDOWN_BLOCK_LIMIT && <div className="oa-md-truncated">{ct(`内容块过多，仅渲染前 ${MARKDOWN_BLOCK_LIMIT} 块，可复制消息查看完整内容。`, `Too many content blocks. Only the first ${MARKDOWN_BLOCK_LIMIT} are rendered; copy the message to view everything.`)}</div>}
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
      <div><b>UltraPlan</b><small>{ct('显式 /ultraplan 调用结果', 'Explicit /ultraplan result')}</small></div>
      <em>{result.ok ? ct('完成', 'Completed') : ct('异常', 'Error')} · Exit {result.exitCodeText || '0'}</em>
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

export const stripUltraPlanProgressText = (text = '') => String(text || '')
  .split(/\r?\n/)
  .filter(line => !/^\s*\[(?:ultraplan|phase|subagent|result|done|next|summary)\]\s*/i.test(line))
  .join('\n')
  .replace(/\n{3,}/g, '\n\n')
  .trim()

const hasUltraPlanDashboardState = (state) => !!(state && (
  state.objective
  || state.phases?.length > 0
  || state.recentTasks?.length > 0
  || state.resultFiles?.length > 0
  || state.complete
))

const renderAssistantBody = (text = '', onAskReply, ultraplan_state) => {
  const parsedState = parseUltraPlanText(text)
  const upState = mergeUltraPlanStates(ultraplan_state, parsedState)
  const cleanText = stripUltraPlanProgressText(text)
  if (hasUltraPlanDashboardState(upState)) {
    return cleanText ? (
      <div className="oa-ultraplan-prose">
        <MarkdownBlock text={cleanText} onAskReply={onAskReply} />
      </div>
    ) : null
  }
  const result = parseUltraPlanResult(text)
  if (result) return <UltraPlanResultCard text={text} />
  return cleanText ? <MarkdownBlock text={cleanText} onAskReply={onAskReply} /> : null
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
    if (/🛠/.test(tr)) {
      flush(target())
      const mTool = tr.match(/[🛠]️?\s+(\w+)\(([\s\S]+)\)\s*$/)
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
  const preview = keys.slice(0, 3).join(' · ') + (keys.length > 3 ? ` +${keys.length - 3}` : '')
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

function SubagentOutputBlock({ text, onAskReply, isRunning }) {
  const { prefix, turns } = useMemo(() => parseSubagentOutput(text), [text])
  const latestKey = turns.length > 0 ? String(turns[turns.length - 1].n) : ''
  const [activeKeys, setActiveKeys] = useState(() => isRunning && latestKey ? [latestKey] : [])
  const previousLatestKeyRef = useRef(latestKey)
  const previousRunningRef = useRef(isRunning)

  // Follow a newly streamed turn while work is running, collapsing older turns.
  // A running -> terminal transition collapses everything once; subsequent
  // terminal renders preserve any turn the user manually reopens.
  useEffect(() => {
    const wasRunning = previousRunningRef.current
    const previousLatestKey = previousLatestKeyRef.current
    if (wasRunning && !isRunning) {
      setActiveKeys([])
    } else if (isRunning && latestKey && (!wasRunning || latestKey !== previousLatestKey)) {
      setActiveKeys([latestKey])
    }
    previousRunningRef.current = isRunning
    previousLatestKeyRef.current = latestKey
  }, [isRunning, latestKey])

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

  const turnItems = turns.map(t => {
    const summaryText = t.children.find(s => s.type === 'summary')?.text || ''
    const fallbackSource = t.children.map((seg) => {
      if (seg.type === 'tool') return `🛠️ ${seg.name}()`
      if (seg.type === 'text') return seg.text
      return ''
    }).filter(Boolean).join('\n')
    const fallbackText = assistantTurnFallbackTitle(fallbackSource, t.n)
    const previewSource = summaryText || fallbackText
    const preview = previewSource.slice(0, 52) + (previewSource.length > 52 ? '…' : '')
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
          activeKey={activeKeys}
          onChange={(keys) => setActiveKeys(Array.isArray(keys) ? keys : (keys ? [keys] : []))}
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
  const isFailed = status === 'fail' || status === 'failed'
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
        aria-expanded={hasOutput ? open : undefined}
        onKeyDown={hasOutput ? (e) => (e.key === 'Enter' || e.key === ' ') && toggle() : undefined}
        title={outputFile || task.desc || ''}
      >
        <span className={`oa-up-task-dot oa-up-task-dot-${status}`} aria-hidden="true">
          {status === 'done' ? <Check size={12} /> : isFailed ? <X size={12} /> : <Clock3 size={12} />}
        </span>
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
          {loading && <div className="oa-up-task-output-meta">Loading output…</div>}
          {error && <div className="oa-up-task-output-error">{error}</div>}
          {!loading && !error && content && <SubagentOutputBlock text={content} onAskReply={onAskReply} isRunning={isRunning} />}
          {!loading && !error && !content && status === 'running' && (
            <div className="oa-up-task-output-waiting">
              <span className="oa-up-task-output-waiting-dot" /><span className="oa-up-task-output-waiting-dot" /><span className="oa-up-task-output-waiting-dot" />
              <span>{ct('等待输出…', 'Waiting for output…')}</span>
            </div>
          )}
          {!loading && !error && !content && status === 'done' && (
            <div className="oa-up-task-output-meta" style={{color:'var(--muted-2)',fontStyle:'italic'}}>{ct('暂无输出内容', 'No output yet')}</div>
          )}
        </div>
      )}
    </div>
  )
}

function UltraPlanDashboard({ state, onAskReply }) {
  const [expanded, setExpanded] = useState(true)
  const panelId = React.useId()
  const { objective, phases = [], recentTasks = [], complete, events = [], resultFiles = [], current, taskOutputs = {}, task_outputs = {} } = state
  const outputsMap = (taskOutputs && Object.keys(taskOutputs).length) ? taskOutputs : (task_outputs || {})
  const phaseTasks = phases.flatMap((phase) => Array.isArray(phase.tasks) ? phase.tasks : [])
  const trackedItems = phases.length ? phases : recentTasks
  const completedItems = complete ? trackedItems.length : trackedItems.filter((item) => item?.status === 'done').length
  const progressPercent = complete ? 100 : (trackedItems.length ? Math.round((completedItems / trackedItems.length) * 100) : 0)
  const taskCount = phaseTasks.length || recentTasks.length
  const hasFailure = [...phases, ...phaseTasks, ...recentTasks].some((item) => item?.status === 'fail' || item?.status === 'failed')
  const hasWork = Boolean(current || phases.length || recentTasks.length)
  const statusTone = complete ? 'done' : hasFailure ? 'failed' : hasWork ? 'run' : 'pending'
  const statusLabel = complete ? '\u5df2\u5b8c\u6210' : hasFailure ? '\u9700\u5173\u6ce8' : hasWork ? '\u6267\u884c\u4e2d' : '\u51c6\u5907\u4e2d'
  const progressLabel = phases.length
    ? `${completedItems} / ${phases.length} \u9636\u6bb5\u5b8c\u6210`
    : recentTasks.length
      ? `${completedItems} / ${recentTasks.length} \u4efb\u52a1\u5b8c\u6210`
      : complete ? '\u6267\u884c\u5df2\u5b8c\u6210' : '\u7b49\u5f85\u6267\u884c\u6b65\u9aa4'
  const isEmpty = !current && phases.length === 0 && recentTasks.length === 0 && resultFiles.length === 0
  const openFile = (fp) => {
    if (!fp) return
    const u = `/api/files/read?path=${encodeURIComponent(fp)}`
    window.open(u, '_blank', 'noopener')
  }
  return (
    <div className={`oa-up-dash oa-up-${statusTone}${expanded ? '' : ' is-collapsed'}`}>
      <button type="button" className="oa-up-head" onClick={() => setExpanded(value => !value)}
        aria-expanded={expanded} aria-controls={panelId}
        aria-label={expanded ? '\u6536\u8d77 UltraPlan \u6267\u884c\u9762\u677f' : '\u5c55\u5f00 UltraPlan \u6267\u884c\u9762\u677f'}>
        <span className="oa-up-icon oa-up-mark" aria-hidden="true"><Sparkles size={15} strokeWidth={2.1} /></span>
        <span className="oa-up-heading">
          <span className="oa-up-title-row">
            <span className="oa-up-title">UltraPlan</span>
            <span className="oa-up-kicker">{'\u4efb\u52a1\u7f16\u6392'}</span>
          </span>
          <span className="oa-up-obj">{objective || '\u7b49\u5f85\u4efb\u52a1\u76ee\u6807'}</span>
        </span>
        <span className={`oa-up-badge oa-up-${statusTone}`}>{statusLabel}</span>
        <span className="oa-up-chevron" aria-hidden="true">
          {expanded ? <ChevronDown size={15} /> : <ChevronLeft size={15} />}
        </span>
      </button>
      <div id={panelId} className="oa-up-body" hidden={!expanded}>
        <section className="oa-up-overview" aria-label="UltraPlan \u6267\u884c\u6458\u8981">
          <div className="oa-up-progress-head">
            <div>
              <span className="oa-up-section-label">{'\u6267\u884c\u8fdb\u5ea6'}</span>
              <strong className="oa-up-progress-copy">{progressLabel}</strong>
            </div>
            <span className="oa-up-progress-value">{progressPercent}<small>%</small></span>
          </div>
          <div className="oa-up-progress-track" role="progressbar" aria-label="UltraPlan \u6267\u884c\u8fdb\u5ea6"
            aria-valuemin="0" aria-valuemax="100" aria-valuenow={progressPercent}>
            <span style={{ '--oa-up-progress': progressPercent / 100 }} />
          </div>
          <div className="oa-up-stats" aria-label="\u6267\u884c\u7edf\u8ba1">
            <span><strong>{phases.length}</strong>{' \u9636\u6bb5'}</span>
            <span><strong>{taskCount}</strong>{' \u4efb\u52a1'}</span>
            <span><strong>{resultFiles.length}</strong>{' \u4ea7\u7269'}</span>
          </div>
          {!complete && current && (
            <div className="oa-up-current">
              <span className="oa-up-current-dot" aria-hidden="true" />
              <span className="oa-up-current-label">{'\u5f53\u524d'}</span>
              <span>{current}</span>
            </div>
          )}
        </section>

        {isEmpty && (
          <div className="oa-up-empty">
            <Clock3 size={16} aria-hidden="true" />
            <div><strong>{'\u7b49\u5f85 UltraPlan \u53d1\u5e03\u6b65\u9aa4'}</strong><span>{'\u8ba1\u5212\u5f00\u59cb\u540e\uff0c\u9636\u6bb5\u548c\u4efb\u52a1\u4f1a\u5728\u8fd9\u91cc\u5b9e\u65f6\u66f4\u65b0\u3002'}</span></div>
          </div>
        )}

        {recentTasks.length > 0 && phases.length === 0 && (
          <section className="oa-up-section oa-up-recent">
            <div className="oa-up-section-head">
              <span className="oa-up-section-label">{'\u6267\u884c\u4efb\u52a1'}</span>
              <span>{recentTasks.length}</span>
            </div>
            <div className="oa-up-tasks">
              {recentTasks.map((task, i) => {
                const lines = (task && task.id && outputsMap && outputsMap[task.id]) ? outputsMap[task.id] : null
                const injected = lines && lines.length ? { ...task, output_lines: lines } : task
                return <UltraPlanTaskRow key={task?.id || i} task={injected} onAskReply={onAskReply} />
              })}
            </div>
          </section>
        )}

        {phases.length > 0 && (
          <section className="oa-up-section oa-up-phase-section">
            <div className="oa-up-section-head">
              <span className="oa-up-section-label">{'\u6267\u884c\u9636\u6bb5'}</span>
              <span>{completedItems}/{phases.length}</span>
            </div>
            <div className="oa-up-phases">
              {phases.map((ph, i) => {
                const phaseFailed = ph.status === 'fail' || ph.status === 'failed'
                return (
                  <div key={ph.id || ph.name || i} className={`oa-up-phase ${ph.status || 'running'}`}>
                    <span className="oa-up-phase-icon" aria-hidden="true">
                      {ph.status === 'done' ? <Check size={13} /> : phaseFailed ? <X size={13} /> : <Clock3 size={13} />}
                    </span>
                    <div className="oa-up-phase-body">
                      <div className="oa-up-phase-info">
                        <span className="oa-up-phase-name">{ph.name}</span>
                        {ph.desc && <span className="oa-up-phase-desc">{ph.desc}</span>}
                        {ph.elapsed && <span className="oa-up-phase-time">{ph.elapsed}</span>}
                      </div>
                      {ph.tasks && ph.tasks.length > 0 && (
                        <div className="oa-up-tasks">
                          {ph.tasks.map((task, j) => {
                            const lines = (task && task.id && outputsMap && outputsMap[task.id]) ? outputsMap[task.id] : null
                            const injected = lines && lines.length ? { ...task, output_lines: lines } : task
                            return <UltraPlanTaskRow key={task?.id || j} task={injected} onAskReply={onAskReply} />
                          })}
                        </div>
                      )}
                    </div>
                  </div>
                )
              })}
            </div>
          </section>
        )}

        {resultFiles.length > 0 && (
          <section className="oa-up-files">
            <div className="oa-up-files-head">
              <span className="oa-up-section-label">{'\u4ea7\u51fa\u6587\u4ef6'}</span>
              <span>{resultFiles.length}</span>
            </div>
            <div className="oa-up-files-list">
              {resultFiles.map((result, i) => (
                <button type="button" key={result.file || i} className="oa-up-file-item" onClick={() => openFile(result.file)} title={result.file}>
                  <span className="oa-up-file-icon" aria-hidden="true"><FileOutput size={15} /></span>
                  <span className="oa-up-file-body">
                    <span className="oa-up-file-desc">{result.desc || taskFileName(result.file)}</span>
                    <span className="oa-up-file-path">{result.file}</span>
                  </span>
                  <ExternalLink size={13} className="oa-up-file-open" aria-hidden="true" />
                </button>
              ))}
            </div>
          </section>
        )}

        {events.length > 0 && (
          <details className="oa-up-events">
            <summary><span>{'\u8fd0\u884c\u65e5\u5fd7'}</span><span className="oa-up-events-count">{events.length}</span></summary>
            <div className="oa-up-events-body">
              {events.map((event, i) => (
                <div key={i} className={`oa-up-event oa-up-event-${event.tag}`}>
                  <span className="oa-up-event-tag">[{event.tag}]</span>
                  <span className="oa-up-event-body">{event.body}</span>
                </div>
              ))}
            </div>
          </details>
        )}
      </div>
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
      <span className="oa-ask-avatar"><CircleHelp size={15} /></span>
      <div><b>{ct('需要用户确认', 'User confirmation required')}</b><p>{ct('智能体正在等待你的选择或补充信息', 'The agent is waiting for your choice or additional information')}</p></div>
    </div>
    {hasStructured ? <div className="oa-ask-body">
      {ask.question && <div className="oa-ask-question"><span>{ct('问题', 'Question')}</span><p>{ask.question}</p></div>}
      {ask.candidates.length > 0 && <div className="oa-ask-options"><span>{ct('快捷回复', 'Quick replies')}</span><div>{ask.candidates.map((x,i)=><button type="button" key={`${x}-${i}`} onClick={(e)=>{e.stopPropagation(); onReply?.(x)}} title={ct('点击填入输入框', 'Insert into the input')}><CornerDownLeft size={13} />{x}</button>)}</div></div>}
    </div> : call.args && <div className="oa-tool-args"><span>{ct('问题', 'Question')}</span><pre>{call.args}</pre></div>}
    {call.result && <div className="oa-tool-result oa-ask-result"><span>{ct('回复', 'Reply')}</span><pre>{call.result}</pre></div>}
  </div>
}

// Re-escape literal control chars inside JSON strings (backend sometimes pretty-prints with real newlines)
function reescapeControlChars(text) {
  const MAP = { '\n': '\\n', '\r': '\\r', '\t': '\\t' }
  const out = []
  let inStr = false, esc = false
  for (const ch of text) {
    if (!inStr) {
      if (ch === '"') inStr = true
      out.push(ch)
      continue
    }
    if (esc) {
      out.push(MAP[ch] || ch)
      esc = false
      continue
    }
    if (ch === '\\') {
      out.push(ch)
      esc = true
      continue
    }
    if (ch === '"') {
      out.push(ch)
      inStr = false
      continue
    }
    out.push(MAP[ch] || ch)
  }
  return out.join('')
}

// Parse file_write/file_patch tool arguments
function parseFileToolArgs(toolName, argsText) {
  const isFileWrite = /file_write$/i.test(toolName)
  const isFilePatch = /file_patch$/i.test(toolName)
  if (!isFileWrite && !isFilePatch) return null

  let parsed = null
  try {
    parsed = JSON.parse(argsText || '{}')
  } catch (e) {
    // Retry with control-char escaping for malformed pretty-printed JSON
    try {
      parsed = JSON.parse(reescapeControlChars(argsText || '{}'))
    } catch (e2) {
      // Final fallback: XML-style parameter tags
      const pathMatch = argsText?.match(/<parameter name="path">([^<]+)<\/antml:parameter>/i)
      const contentMatch = argsText?.match(/<parameter name="content">([^]*?)<\/antml:parameter>/i)
      const oldMatch = argsText?.match(/<parameter name="old_content">([^]*?)<\/antml:parameter>/i)
      const newMatch = argsText?.match(/<parameter name="new_content">([^]*?)<\/antml:parameter>/i)
      const modeMatch = argsText?.match(/<parameter name="mode">([^<]+)<\/antml:parameter>/i)

      if (isFileWrite && pathMatch) {
        return {
          type: 'file_write',
          path: pathMatch[1],
          content: contentMatch?.[1] || '',
          mode: modeMatch?.[1] || 'overwrite'
        }
      }
      if (isFilePatch && pathMatch) {
        return {
          type: 'file_patch',
          path: pathMatch[1],
          old_content: oldMatch?.[1] || '',
          new_content: newMatch?.[1] || ''
        }
      }
      return null
    }
  }

  if (isFileWrite && parsed?.path) {
    return {
      type: 'file_write',
      path: parsed.path,
      content: parsed.content || '',
      mode: parsed.mode || 'overwrite'
    }
  }
  if (isFilePatch && parsed?.path) {
    return {
      type: 'file_patch',
      path: parsed.path,
      old_content: parsed.old_content || '',
      new_content: parsed.new_content || ''
    }
  }
  return null
}

// Unified diff rows: line numbers + -/+ gutter, collapsed context
function DiffRows({ rows }) {
  return <div className="oa-diff" role="table" aria-label="文件改动逐行对照">
    {rows.map((row, i) => {
      if (row.type === 'gap') {
        return <div className="oa-diff-row oa-diff-gap" key={`g${i}`} role="row">
          <span className="oa-diff-no" aria-hidden="true">⋯</span>
          <span className="oa-diff-sign" aria-hidden="true" />
          <span className="oa-diff-text">{`未改动 ${row.count} 行`}</span>
        </div>
      }
      const sign = row.type === 'add' ? '+' : row.type === 'del' ? '-' : ' '
      return <div className={`oa-diff-row oa-diff-${row.type}`} key={i} role="row">
        <span className="oa-diff-no">{row.type === 'add' ? row.newNo : row.oldNo}</span>
        <span className="oa-diff-sign" aria-hidden="true">{sign}</span>
        <span className="oa-diff-text">{row.text === '' ? '\u00a0' : row.text}</span>
      </div>
    })}
  </div>
}

// Render file tool arguments in a structured way
function FileToolArgsPanel({ toolName, args, result }) {
  const fileArgs = parseFileToolArgs(toolName, args)
  const [showContent, setShowContent] = useState(false)

  const { type, path, content, old_content, new_content, mode } = fileArgs || {}
  const diff = useMemo(() => {
    if (!fileArgs) return null
    return type === 'file_patch'
      ? computeLineDiff(old_content, new_content, { context: 3 })
      : computeWriteRows(content)
  }, [fileArgs, type, old_content, new_content, content])

  if (!fileArgs) {
    return <div className="oa-tool-args"><span>{'📥 args'}</span><pre>{args}</pre></div>
  }

  const { rows, added, removed, truncated } = diff
  const changedTotal = added + removed

  return <div className="oa-tool-args oa-file-tool-args">
    <div className="oa-file-tool-header">
      <span className="oa-file-tool-badge">
        {type === 'file_write' ? '📝 写入文件' : '✏️ 修改文件'}
      </span>
      {mode && mode !== 'overwrite' && <span className="oa-file-tool-mode">{mode}</span>}
      {changedTotal > 0 && (
        <span className="oa-diff-stats">
          {added > 0 && <span className="oa-diff-stats-add">{`+${added}`}</span>}
          {removed > 0 && <span className="oa-diff-stats-del">{`-${removed}`}</span>}
        </span>
      )}
    </div>

    <FileAttachment path={path} resolvedPath={extractToolResultFilePath(result)} />

    {changedTotal === 0 && <div className="oa-file-tool-empty">无行级改动</div>}

    {changedTotal > 0 && rows.length > 0 && (
      <div className="oa-file-tool-content">
        <button
          type="button"
          className="oa-file-tool-toggle"
          onClick={() => setShowContent(v => !v)}
          aria-expanded={showContent}
        >
          {showContent ? '收起改动' : `查看改动 (+${added} / -${removed})`}
          <ChevronDown size={14} style={{ transform: showContent ? 'rotate(180deg)' : 'none' }} />
        </button>
        {showContent && (
          <div className="oa-file-tool-preview">
            {truncated && <div className="oa-diff-note">改动过大，已按块粗粒度对比</div>}
            <DiffRows rows={rows} />
          </div>
        )}
      </div>
    )}
  </div>
}

const FileSummaryCard = memo(function FileSummaryCard({ content = '' }) {
  const fileOps = useMemo(() => {
    const parts = normalizeToolParts(splitMarkdownParts(content))
    const ops = []
    for (const part of parts) {
      if (part.type !== 'tool') continue
      const call = part.call || {}
      const parsed = parseFileToolArgs(call.name, call.args)
      if (parsed && parsed.path) {
        // 计算改动统计
        let added = 0, removed = 0, summary = ''

        if (parsed.type === 'file_patch') {
          // 用真实行级 diff 统计（旧实现取 old/new 块整块行数，会把未变的上下文行也计入 ±）
          const d = computeLineDiff(parsed.old_content || '', parsed.new_content || '', { context: 0 })
          added = d.added
          removed = d.removed
          // 取 new_content 前30个字符作为摘要
          summary = (parsed.new_content || '').trim().slice(0, 50).replace(/\n/g, ' ')
        } else if (parsed.type === 'file_write') {
          const lines = (parsed.content || '').split('\n')
          added = lines.length
          // 取 content 前30个字符作为摘要
          summary = (parsed.content || '').trim().slice(0, 50).replace(/\n/g, ' ')
        }

        ops.push({
          type: parsed.type,
          path: parsed.path,
          added,
          removed,
          summary: summary ? summary + (summary.length >= 50 ? '...' : '') : '',
          // Store full content for expandable diff
          old_content: parsed.old_content || '',
          new_content: parsed.new_content || '',
          content: parsed.content || ''
        })
      }
    }
    // 按文件分组：同一文件的多次操作全部保留（此前按 path 去重会丢失前面的改动）
    const groups = new Map()
    for (const op of ops) {
      if (!groups.has(op.path)) groups.set(op.path, [])
      groups.get(op.path).push(op)
    }
    return Array.from(groups.entries()).map(([path, list]) => ({
      path,
      ops: list,
      added: list.reduce((s, o) => s + o.added, 0),
      removed: list.reduce((s, o) => s + o.removed, 0),
      summary: list[list.length - 1].summary,
    }))
  }, [content])

  const [expandedPaths, setExpandedPaths] = useState(new Set())
  const [collapsed, setCollapsed] = useState(false)

  const toggleExpand = useCallback((fp) => {
    setExpandedPaths(prev => {
      const next = new Set(prev)
      if (next.has(fp)) next.delete(fp)
      else next.add(fp)
      return next
    })
  }, [])

  if (fileOps.length === 0) return null

  return (
    <div className="oa-file-summary">
      <div
        className={'oa-file-summary-header clickable' + (collapsed ? ' collapsed' : '')}
        onClick={() => setCollapsed(v => !v)}
        role="button"
        tabIndex={0}
        onKeyDown={e => { if (e.key === 'Enter') setCollapsed(v => !v) }}
      >
        <FileText size={13} />
        <span>文件改动 · {fileOps.length}</span>
        <ChevronDown size={12} className={'oa-file-summary-toggle' + (collapsed ? '' : ' open')} />
      </div>
      {!collapsed && (
      <div className="oa-file-summary-list">
        {fileOps.map((group, i) => {
          const filename = group.path.split(/[/\\]/).pop() || group.path
          const expanded = expandedPaths.has(group.path)
          const multi = group.ops.length > 1
          // Compute diff rows on demand（每次操作各算一份）
          const diffResults = expanded
            ? group.ops.map(op => op.type === 'file_patch'
                ? computeLineDiff(op.old_content, op.new_content, { context: 3 })
                : computeWriteRows(op.content))
            : null
          return (
            <div key={i}>
              <div
                className={'oa-file-summary-item' + (expanded ? ' expanded' : '')}
                onClick={() => toggleExpand(group.path)}
                title={group.path}
                role="button"
                tabIndex={0}
                onKeyDown={e => { if (e.key === 'Enter') toggleExpand(group.path) }}
              >
                <ChevronDown size={11} className={'oa-file-chevron' + (expanded ? ' open' : '')} />
                <span className="oa-file-name">{filename}</span>
                {multi && <span className="oa-file-op-count">×{group.ops.length}</span>}
                <span className="oa-file-stats">
                  {group.removed > 0 && <span className="stat-removed">-{group.removed}</span>}
                  {group.added > 0 && <span className="stat-added">+{group.added}</span>}
                </span>
                {group.summary && !expanded && <span className="oa-file-preview">{group.summary}</span>}
              </div>
              {expanded && diffResults && group.ops.map((op, j) => (
                <div key={j}>
                  {multi && (
                    <div className="oa-file-op-label">
                      #{j + 1} · {op.type === 'file_patch' ? 'patch' : 'write'}
                      <span className="oa-file-stats">
                        {op.type === 'file_patch' && op.removed > 0 && <span className="stat-removed">-{op.removed}</span>}
                        {op.added > 0 && <span className="stat-added">+{op.added}</span>}
                      </span>
                    </div>
                  )}
                  <div className="oa-file-summary-diff">
                    <DiffRows rows={diffResults[j].rows} />
                  </div>
                </div>
              ))}
            </div>
          )
        })}
      </div>
      )}
    </div>
  )
})

function ToolCallBlock({ call, onAskReply }) {
  const toolName = String(call.name || 'unknown').trim()
  const isAskUser = /(?:^|[._-])ask_user$/i.test(toolName)
  const isFileTool = /file_(write|patch)$/i.test(toolName)
  const [open, setOpen] = useState(isAskUser)
  const resultStatus = String(call.result || '').match(/\[Status\]\s*([^\n]+)/i)?.[1]?.trim()
  const askPayload = isAskUser ? getAskUserPayload(call) : null
  const askSummary = askPayload?.question || '等待用户确认'

  // Extract file path for file tools to show in header
  const fileArgs = isFileTool ? parseFileToolArgs(toolName, call.args) : null
  const fileName = fileArgs?.path?.split(/[\\/]/).filter(Boolean).pop()

  return <div className={`oa-tool-call ${isAskUser ? 'oa-tool-ask-user' : ''} ${isFileTool ? 'oa-tool-file' : ''} ${open ? 'open' : 'collapsed'}`}>
    <button className="oa-tool-head" type="button" onClick={() => setOpen(v => !v)} aria-expanded={open}>
      <span className="oa-tool-icon">{isAskUser ? <CircleHelp size={14} /> : isFileTool ? '📁' : '🛠️'}</span>
      {!isAskUser && <span>Tool</span>}
      <b>{toolName}</b>
      {fileName && <em className="oa-tool-file-name">{fileName}</em>}
      {resultStatus && <em>{resultStatus}</em>}
      {isAskUser && !resultStatus && <em>{askPayload?.candidates?.length ? ct(`${askPayload.candidates.length} 个选项`, `${askPayload.candidates.length} options`) : ct('等待回复', 'Waiting for reply')}</em>}
      <ChevronDown size={15} className="oa-tool-chevron" />
    </button>
    {open && (isAskUser ? <AskUserPanel call={call} onReply={onAskReply} /> : <>
      {isFileTool ? (
        <FileToolArgsPanel toolName={toolName} args={call.args} result={call.result} />
      ) : (
        call.args && <div className="oa-tool-args"><span>{'📥 args'}</span><pre>{call.args}</pre></div>
      )}
      {call.result && <div className="oa-tool-result"><span>{'📤 result'}</span><pre>{call.result}</pre></div>}
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
    {hidden > 0 && <li className="oa-md-truncated">{ct(`… 已隐藏 ${hidden.toLocaleString(chatLocale())} 个列表项`, `… ${hidden.toLocaleString(chatLocale())} list items hidden`)}</li>}
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
  const heading = trimmed.match(/^\s{0,3}(#{1,6})\s+(.+?)\s*#*\s*$/)
  if (heading) {
    const Tag = `h${heading[1].length}`
    return <Tag key={key}><InlineRichText text={heading[2]} /></Tag>
  }
  if (/^\s{0,3}(?:(?:-\s*){3,}|(?:\*\s*){3,}|(?:_\s*){3,})$/.test(trimmed)) return <hr key={key} />
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

  for (let lineIdx = 0; lineIdx < lines.length; lineIdx += 1) {
    const line = lines[lineIdx]
    const nextLine = lines[lineIdx + 1] || ''
    const isTableStart = line.includes('|') && nextLine.includes('|') && splitTableRow(nextLine).every(cell => parseTableAlign(cell) !== null)
    const isOrdered = /^\s*\d+[.)]\s+/.test(line)
    const isUnordered = /^\s*[-*+]\s+/.test(line)
    const isHeading = /^\s{0,3}#{1,6}\s+/.test(line)
    const isRule = /^\s{0,3}(?:(?:-\s*){3,}|(?:\*\s*){3,}|(?:_\s*){3,})$/.test(line)
    if (isTableStart) {
      flushParagraph()
      flushList()
      const tableLines = [line, nextLine]
      lineIdx += 2
      while (lineIdx < lines.length && lines[lineIdx].includes('|')) {
        tableLines.push(lines[lineIdx])
        lineIdx += 1
      }
      lineIdx -= 1
      const nestedTable = parseMarkdownTable(tableLines.join('\n'))
      if (nestedTable) nodes.push(renderMarkdownTable(nestedTable, `${i}-t-${seq++}`))
    } else if (isHeading || isRule) {
      flushParagraph()
      flushList()
      const node = renderPlainTextBlock(line, `${i}-b-${seq++}`)
      if (node) nodes.push(node)
    } else if (isOrdered || isUnordered) {
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
  if (hiddenBlocks > 0) nodes.push(<div key="__hidden_blocks" className="oa-md-truncated">{ct(`… 已隐藏 ${hiddenBlocks.toLocaleString(chatLocale())} 个内容块，可复制消息查看完整内容。`, `… ${hiddenBlocks.toLocaleString(chatLocale())} content blocks hidden; copy the message to view all.`)}</div>)
  return <>{nodes}</>
}

const ULTRAPLAN_DRAWER_DEFAULT_WIDTH = 440
const ULTRAPLAN_DRAWER_MIN_WIDTH = 360
const ULTRAPLAN_DRAWER_MAX_WIDTH = 960
const ULTRAPLAN_DRAWER_VIEWPORT_GUTTER = 24

function getUltraPlanDrawerMaxWidth() {
  if (typeof window === 'undefined') return ULTRAPLAN_DRAWER_MAX_WIDTH
  return Math.max(
    ULTRAPLAN_DRAWER_MIN_WIDTH,
    Math.min(ULTRAPLAN_DRAWER_MAX_WIDTH, Math.floor(window.innerWidth - ULTRAPLAN_DRAWER_VIEWPORT_GUTTER)),
  )
}

function clampUltraPlanDrawerWidth(width, maxWidth = getUltraPlanDrawerMaxWidth()) {
  return Math.min(maxWidth, Math.max(ULTRAPLAN_DRAWER_MIN_WIDTH, Math.round(Number(width) || ULTRAPLAN_DRAWER_DEFAULT_WIDTH)))
}

const GOAL_CARD_TERMINAL = new Set(['achieved', 'stopped', 'failed', 'timeout', 'expired', 'error', 'given_up', 'gave_up', 'done', 'removed'])
const goalCardPathKey = (p) => String(p || '').replace(/\\/g, '/').toLowerCase()

const normalizeGoalCardState = (raw) => {
  if (!raw || typeof raw !== 'object') return null
  const g = { ...raw }
  const nowSec = Date.now() / 1000
  const start = Number(g.start_time || 0)
  if (!(Number(g.elapsed_seconds) > 0) && start > 0) {
    const end = Number(g.end_time || 0) > 0 ? Number(g.end_time) : nowSec
    g.elapsed_seconds = Math.max(0, end - start)
  }
  const budget = Number(g.budget_seconds || 0)
  if (!(Number(g.remaining_seconds) >= 0) && budget > 0) {
    g.remaining_seconds = Math.max(0, budget - Number(g.elapsed_seconds || 0))
  }
  return g
}

const formatGoalCardTime = (value) => {
  const d = dateFromTimestamp(value)
  if (!d || d.getFullYear() < 2000 || d.getTime() > Date.now() + 86400000) return ''
  return d.toLocaleString()
}

const goalCardStatusInfo = (status, removed) => {
  const s = String(status || '').toLowerCase()
  if (removed) return { text: '已结束', cls: 'is-done' }
  if (s === 'achieved' || s === 'success' || s === 'done') return { text: '已达成', cls: 'is-done' }
  if (s === 'failed' || s === 'error') return { text: '失败', cls: 'is-error' }
  if (s === 'stopped' || s === 'given_up' || s === 'gave_up') return { text: '已停止', cls: 'is-error' }
  if (s === 'timeout' || s === 'expired') return { text: '超时', cls: 'is-error' }
  if (!s || s === 'running' || s === 'active' || s === 'pending') return { text: '进行中', cls: 'is-running' }
  return { text: s, cls: 'is-running' }
}

export function GoalStatusCard({ state, pending = false }) {
  const [snap, setSnap] = useState(() => normalizeGoalCardState(state))
  const [removed, setRemoved] = useState(false)
  const [collapsed, setCollapsed] = useState(false)
  const [clockNow, setClockNow] = useState(() => Date.now())
  useEffect(() => { setSnap(prev => ({ ...(prev || {}), ...(normalizeGoalCardState(state) || {}) })) }, [state])
  const status = String(snap?.status || '').toLowerCase()
  const terminal = removed || GOAL_CARD_TERMINAL.has(status)
  const stateFile = snap?.state_file || ''
  const goalId = snap?.id || ''
  useEffect(() => {
    if (terminal) return undefined
    const timer = setInterval(() => setClockNow(Date.now()), 1000)
    return () => clearInterval(timer)
  }, [terminal])
  useEffect(() => {
    if ((!stateFile && !goalId) || terminal) return undefined
    let stop = false
    let timer = null
    const tick = async () => {
      try {
        const d = await api('/api/goals/list')
        if (stop) return
        const goals = Array.isArray(d?.goals) ? d.goals : []
        const hit = goals.find(g => (
          (stateFile && goalCardPathKey(g?.state_file) === goalCardPathKey(stateFile))
          || (goalId && String(g?.id || '') === String(goalId))
        ))
        if (hit) {
          setRemoved(false)
          setSnap(prev => normalizeGoalCardState({ ...(prev || {}), ...hit }))
        }
      } catch { /* 网络波动时保留旧快照 */ }
      if (!stop) timer = setTimeout(tick, 5000)
    }
    tick()
    return () => { stop = true; if (timer) clearTimeout(timer) }
  }, [stateFile, goalId, terminal])
  if (!snap) return null
  const startedAt = dateFromTimestamp(snap.start_time)?.getTime()
  const serverElapsed = Number(snap.elapsed_seconds || 0)
  const elapsedSeconds = !terminal && Number.isFinite(startedAt)
    ? Math.max(serverElapsed, Math.floor((clockNow - startedAt) / 1000))
    : serverElapsed
  const budgetTotal = Number(snap.budget_seconds || 0) || (serverElapsed + Number(snap.remaining_seconds || 0))
  const liveSnap = { ...snap, elapsed_seconds: elapsedSeconds, remaining_seconds: Math.max(0, budgetTotal - elapsedSeconds) }
  const info = goalCardStatusInfo(liveSnap.status, removed)
  const budgetPct = goalBudgetPercent(liveSnap)
  const turnPct = goalTurnPercent(liveSnap)
  const maxTurns = Number(liveSnap.max_turns || 0)
  const errText = liveSnap.error_class || liveSnap.last_error || ''
  const startTimeText = formatGoalCardTime(liveSnap.start_time)
  return (
    <div className={`oa-goalcard ${info.cls}`}>
      <button type="button" className="oa-goalcard-head" onClick={() => setCollapsed(v => !v)}
        aria-expanded={!collapsed} title={collapsed ? '展开目标详情' : '收起目标详情'}>
        <span className="oa-goalcard-mark"><Target size={15} /></span>
        <span className="oa-goalcard-title">
          <b>目标模式</b>
          <small>{liveSnap.objective || '(未提供目标描述)'}</small>
        </span>
        <em className={`oa-goalcard-chip ${info.cls}`}>{info.cls === 'is-running' && !removed ? <span className="oa-goalcard-dot" /> : null}{info.text}</em>
        <ChevronDown size={14} className={`oa-goalcard-chevron ${collapsed ? 'is-collapsed' : ''}`} />
      </button>
      {!collapsed && (
        <div className="oa-goalcard-body">
          <div className="oa-goalcard-bar">
            <span className="oa-goalcard-bar-label">时间预算</span>
            <span className="oa-goalcard-track"><span className="oa-goalcard-fill" style={{ width: `${budgetPct}%` }} /></span>
            <span className="oa-goalcard-bar-value">{formatDuration(liveSnap.elapsed_seconds || 0)}{budgetTotal ? ` / ${formatDuration(budgetTotal)}` : ''}</span>
          </div>
          <div className="oa-goalcard-bar">
            <span className="oa-goalcard-bar-label">轮次</span>
            <span className="oa-goalcard-track"><span className="oa-goalcard-fill" style={{ width: `${turnPct}%` }} /></span>
            <span className="oa-goalcard-bar-value">{Number(liveSnap.turns_used || 0)}{maxTurns ? ` / ${maxTurns}` : ''}</span>
          </div>
          <div className="oa-goalcard-meta">
            {startTimeText ? <span>启动 {startTimeText}</span> : null}
            {liveSnap.mode ? <span>模式 {liveSnap.mode}</span> : null}
            {liveSnap.pid ? <span>PID {liveSnap.pid}</span> : null}
            {removed ? <span>状态文件已清理</span> : null}
          </div>
          {liveSnap.summary ? <div className="oa-goalcard-summary">{String(liveSnap.summary)}</div> : null}
          {errText ? <div className="oa-goalcard-err">{String(errText)}</div> : null}
        </div>
      )}
    </div>
  )
}

function UltraPlanMessageDrawer({ content = '', state, pending = false, onAskReply }) {
  const mergedState = useMemo(
    () => mergeUltraPlanStates(state, parseUltraPlanText(content)),
    [state, content],
  )
  const available = hasUltraPlanDashboardState(mergedState)
  const [open, setOpen] = useState(false)
  const [drawerWidth, setDrawerWidth] = useState(() => clampUltraPlanDrawerWidth(ULTRAPLAN_DRAWER_DEFAULT_WIDTH))
  const [drawerMaxWidth, setDrawerMaxWidth] = useState(() => getUltraPlanDrawerMaxWidth())
  const [resizing, setResizing] = useState(false)
  const entryRef = useRef(null)
  const drawerWidthRef = useRef(drawerWidth)
  const resizeSessionRef = useRef(null)
  const autoOpenedRef = useRef(false)
  const userDismissedRef = useRef(false)
  const drawerId = React.useId()
  const titleId = `${drawerId}-title`

  const applyDrawerWidth = useCallback((nextWidth) => {
    const maxWidth = getUltraPlanDrawerMaxWidth()
    const width = clampUltraPlanDrawerWidth(nextWidth, maxWidth)
    drawerWidthRef.current = width
    setDrawerMaxWidth(maxWidth)
    setDrawerWidth(width)
  }, [])

  const beginDrawerResize = useCallback((event) => {
    if (event.button != null && event.button !== 0) return
    const startX = Number.isFinite(event.clientX) ? event.clientX : 0
    resizeSessionRef.current = {
      pointerId: event.pointerId,
      startX,
      startWidth: drawerWidthRef.current,
    }
    event.currentTarget.setPointerCapture?.(event.pointerId)
    setResizing(true)
    event.preventDefault()
  }, [])

  const moveDrawerResize = useCallback((event) => {
    const session = resizeSessionRef.current
    if (!session || (session.pointerId != null && event.pointerId !== session.pointerId)) return
    const clientX = Number.isFinite(event.clientX) ? event.clientX : session.startX
    applyDrawerWidth(session.startWidth + session.startX - clientX)
    event.preventDefault()
  }, [applyDrawerWidth])

  const finishDrawerResize = useCallback((event) => {
    const session = resizeSessionRef.current
    if (!session || (session.pointerId != null && event.pointerId !== session.pointerId)) return
    resizeSessionRef.current = null
    setResizing(false)
    if (event.currentTarget.hasPointerCapture?.(event.pointerId)) {
      event.currentTarget.releasePointerCapture(event.pointerId)
    }
  }, [])

  const resizeDrawerFromKeyboard = useCallback((event) => {
    const step = event.shiftKey ? 64 : 32
    let nextWidth = null
    if (event.key === 'ArrowLeft') nextWidth = drawerWidthRef.current + step
    else if (event.key === 'ArrowRight') nextWidth = drawerWidthRef.current - step
    else if (event.key === 'Home') nextWidth = ULTRAPLAN_DRAWER_MIN_WIDTH
    else if (event.key === 'End') nextWidth = getUltraPlanDrawerMaxWidth()
    if (nextWidth == null) return
    event.preventDefault()
    applyDrawerWidth(nextWidth)
  }, [applyDrawerWidth])

  const closeDrawer = useCallback(() => {
    userDismissedRef.current = true
    setOpen(false)
    const restoreFocus = () => entryRef.current?.focus()
    if (typeof requestAnimationFrame === 'function') requestAnimationFrame(restoreFocus)
    else setTimeout(restoreFocus, 0)
  }, [])

  useEffect(() => {
    const syncWidthToViewport = () => applyDrawerWidth(drawerWidthRef.current)
    syncWidthToViewport()
    window.addEventListener('resize', syncWidthToViewport)
    return () => window.removeEventListener('resize', syncWidthToViewport)
  }, [applyDrawerWidth])

  useEffect(() => {
    if (!available || !pending || mergedState?.complete || autoOpenedRef.current || userDismissedRef.current) return
    autoOpenedRef.current = true
    setOpen(true)
  }, [available, pending, mergedState?.complete])

  useEffect(() => {
    if (!open) return undefined
    const onKeyDown = (event) => {
      if (event.key !== 'Escape') return
      event.preventDefault()
      closeDrawer()
    }
    document.addEventListener('keydown', onKeyDown)
    return () => document.removeEventListener('keydown', onKeyDown)
  }, [open, closeDrawer])

  if (!available) return null

  const phases = Array.isArray(mergedState.phases) ? mergedState.phases : []
  const recentTasks = Array.isArray(mergedState.recentTasks) ? mergedState.recentTasks : []
  const phaseTasks = phases.flatMap(phase => Array.isArray(phase.tasks) ? phase.tasks : [])
  const total = phaseTasks.length || recentTasks.length || phases.length
  const done = mergedState.complete
    ? total
    : (phaseTasks.length ? phaseTasks : (recentTasks.length ? recentTasks : phases))
      .filter(item => String(item?.status || '').toLowerCase() === 'done').length
  const statusText = mergedState.complete
    ? '\u5df2\u5b8c\u6210'
    : (pending ? '\u6267\u884c\u4e2d' : '\u53ef\u67e5\u770b')
  const objective = String(mergedState.objective || mergedState.current || '\u67e5\u770b\u8ba1\u5212\u4e0e\u5b50\u4efb\u52a1\u8fdb\u5c55')

  return (
    <div className="oa-message-ultraplan">
      <button
        ref={entryRef}
        type="button"
        className="oa-up-entry"
        aria-expanded={open}
        aria-controls={drawerId}
        onClick={() => setOpen(true)}
      >
        <span className="oa-up-entry-mark" aria-hidden="true"><Sparkles size={15} /></span>
        <span className="oa-up-entry-copy">
          <b>UltraPlan</b>
          <small>{objective}</small>
        </span>
        <span className={`oa-up-entry-status ${mergedState.complete ? 'is-done' : 'is-running'}`}>
          {statusText}{total > 0 ? ` \u00b7 ${done}/${total}` : ''}
        </span>
        <PanelRightOpen size={16} aria-hidden="true" />
      </button>

      {open && createPortal(
        <div className="oa-message-ultraplan oa-up-drawer-layer" data-ultraplan-drawer-owner="message">
          <aside
            id={drawerId}
            className={`oa-up-drawer ${resizing ? 'is-resizing' : ''}`}
            role="region"
            aria-labelledby={titleId}
            style={{ '--oa-up-drawer-width': `${drawerWidth}px` }}
          >
            <div
              className="oa-up-drawer-resize"
              role="separator"
              aria-label={'\u8c03\u6574 UltraPlan \u4fa7\u680f\u5bbd\u5ea6'}
              aria-orientation="vertical"
              aria-controls={drawerId}
              aria-valuemin={ULTRAPLAN_DRAWER_MIN_WIDTH}
              aria-valuemax={drawerMaxWidth}
              aria-valuenow={drawerWidth}
              aria-valuetext={`${drawerWidth} px`}
              tabIndex={0}
              onPointerDown={beginDrawerResize}
              onPointerMove={moveDrawerResize}
              onPointerUp={finishDrawerResize}
              onPointerCancel={finishDrawerResize}
              onKeyDown={resizeDrawerFromKeyboard}
            />
            <header className="oa-up-drawer-head">
              <span className="oa-up-drawer-kicker">MESSAGE-LINKED PLAN</span>
              <div>
                <h2 id={titleId}>UltraPlan</h2>
                <p>{objective}</p>
              </div>
              <button type="button" className="oa-up-drawer-close" aria-label={'\u5173\u95ed UltraPlan \u8be6\u60c5'} onClick={closeDrawer}>
                <X size={18} />
              </button>
            </header>
            <div className="oa-up-drawer-scroll">
              <UltraPlanDashboard state={mergedState} onAskReply={onAskReply} />
            </div>
          </aside>
        </div>,
        document.body,
      )}
    </div>
  )
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
  if (!content && pending && !hasLiveUltraPlan) return <div className="oa-content oa-thinking">{ct('正在思考…', 'Thinking…')}</div>
  if (content && stats.tooLarge && !hasTurnSplit) return <div className="oa-content"><LongTextPreview text={content} stats={stats} /></div>
  const boxedRuns = parsed.runs.slice(0, -1)
  const lastRun = parsed.runs[parsed.runs.length - 1]
  // A persisted UltraPlan state belongs to the final user-visible branch. When a
  // response has turn markers but no explicit final marker, that branch is the
  // latest run rather than parsed.body.
  const ultraPlanStateForLastRun = !parsed.body && hasLiveUltraPlan
    ? (liveUltraPlanState || ultraplan_state)
    : undefined
  const isTurnOpen = (r, i) => openTurns[`${r.turn}-${i}`] === true
  const toggleTurn = (r, i) => setOpenTurns(xs => ({ ...xs, [`${r.turn}-${i}`]: !isTurnOpen(r, i) }))
  return <div className={`oa-content ${parsed.runs.length ? 'oa-agent-output' : ''}`}>
    {parsed.runs.length > 0 && <div className={`oa-turn-stack ${stackOpen ? 'open' : 'collapsed'}`}>
      <button className="oa-turn-stack-head" type="button" onClick={() => setStackOpen(v => !v)} aria-expanded={stackOpen} title={stackOpen ? ct('折叠执行过程', 'Collapse execution') : ct('展开执行过程', 'Expand execution')}>
        <span className="oa-run-dot"/>
        <span>{ct('执行过程', 'Execution')}</span>
        <b>{parsed.runs.length}</b>
        <em>{pending ? ct('正在生成', 'Generating') : ct('已完成', 'Completed')}</em>
        <ChevronDown className="oa-stack-chevron" size={15}/>
      </button>
      {stackOpen && boxedRuns.map((r, i) => {
        const open = isTurnOpen(r, i)
        const tu = turnUsages && turnUsages[i]
        const filesChanged = hasFileMutation(r.body)
        return <div className="oa-turn-node" key={`${r.turn}-${i}`}>
          <section className={`oa-turn-card ${open ? 'open' : 'collapsed'}`}>
            <button className="oa-turn-toggle" type="button" onClick={() => toggleTurn(r, i)} aria-expanded={open} title={r.title || ct('执行步骤', 'Execution step')}>
              <span className="oa-turn-index">{ct('步骤', 'Step')} {r.turn}</span>
              {filesChanged && <StepFileMutationMarker />}
              <b>{r.title || ct('执行步骤', 'Execution step')}</b>
              <UsageRow u={tu} className="oa-usage-inline" />
              <ChevronDown size={15} className="oa-turn-chevron"/>
            </button>
            {open && (r.body ? renderAssistantBody(r.body, onAskReply) : <p className="oa-turn-empty">{ct('该轮暂无详细输出', 'No detailed output for this turn')}</p>)}
          </section>
        </div>
      })}
      {lastRun && <section className="oa-turn-current" key={`last-${lastRun.turn}`}>
        <div className="oa-turn-current-head"><span className="oa-turn-index oa-turn-index-current">{ct('步骤', 'Step')} {lastRun.turn}</span>{hasFileMutation(lastRun.body) && <StepFileMutationMarker />}<b>{lastRun.title || ct('正在执行', 'Running')}</b><UsageRow u={turnUsages && turnUsages[boxedRuns.length]} className="oa-usage-inline" /><em>{pending ? ct('实时输出中', 'Live output') : ct('最新一轮', 'Latest turn')}</em></div>
        {lastRun.body || ultraPlanStateForLastRun
          ? renderAssistantBody(lastRun.body || '', onAskReply, ultraPlanStateForLastRun)
          : <p className="oa-turn-empty">{ct('正在等待该轮输出…', 'Waiting for this turn’s output…')}</p>}
      </section>}
    </div>}
    {(parsed.summary || parsed.body || !parsed.runs.length) && <div className={parsed.runs.length ? 'oa-final-answer' : ''}>
      {parsed.runs.length > 0 && <div className="oa-final-label">返回给用户</div>}
      {parsed.summary && <div className="oa-response-summary" aria-label="响应摘要"><span>摘要</span><b>{parsed.summary}</b></div>}
      {renderAssistantBody(parsed.body || (!parsed.summary ? content : '') || '', onAskReply, liveUltraPlanState || ultraplan_state)}
    </div>}
    <FileSummaryCard content={content} />
  </div>
})

// User messages append a generated attachment block. Cards render it separately, so hide the raw suffix.
const stripUserAttachmentBlock = (content = '') => {
  const src = String(content || '')
  const markers = ['\n[附件]', '\n[图片附件]', '\n[附件已保存]', '[附件]', '[图片附件]', '[附件已保存]']
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

const usageHasTokens = (u) => !!u && ((u.input_tokens || 0) > 0 || (u.cache_creation_tokens || 0) > 0 || cacheReadTokens(u) > 0 || (u.output_tokens || 0) > 0)
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
const formatTokens = (count = 0) => {
  const num = Math.max(0, Number(count) || 0)
  if (num >= 1e9) return `${(num / 1e9).toFixed(1)}B`
  if (num >= 1e6) return `${(num / 1e6).toFixed(1)}M`
  if (num >= 1e3) return `${(num / 1e3).toFixed(1)}K`
  return num.toLocaleString(chatLocale())
}

const UsageRow = ({ u, label, className, elapsedMs = 0, live = false, ctxChars = 0, ctxMsgs = 0 }) => {
  const hasTokens = usageHasTokens(u)
  const hasElapsed = elapsedMs > 0
  const hasCtx = ctxChars > 0 || ctxMsgs > 0
  if (!hasTokens && !hasElapsed && !hasCtx) return null
  return <div className={`oa-usage ${className || ''}`}>
    {label && <span className="oa-usage-label">{label}</span>}
    {hasElapsed && <span className={live ? 'oa-usage-time is-live' : 'oa-usage-time'} title={live ? ct('实时耗时', 'Live elapsed time') : ct('耗时', 'Elapsed time')}><svg viewBox="0 0 16 16" width="10" height="10" fill="currentColor" aria-hidden="true"><path fillRule="evenodd" d="M8 2a6 6 0 1 0 0 12A6 6 0 0 0 8 2zm0 1.5A4.5 4.5 0 1 1 8 11a4.5 4.5 0 0 1 0-7.5z"/><path d="M7.5 4.5h1v3.65l2.2 1.3-.5.9L7.5 9V4.5z"/></svg>{ct('耗时', 'Time')} <b>{formatElapsedMs(elapsedMs)}</b></span>}
    {u?.input_tokens > 0 && <span className="oa-usage-in" title={ct('输入 tokens', 'Input tokens')}><svg viewBox="0 0 16 16" width="10" height="10" aria-hidden="true"><path d="M8 11.5 3.5 7l1.1-1.1L8 9.3l3.4-3.4L12.5 7 8 11.5Z"/></svg>{ct('输入', 'Input')} <b>{formatTokens(u.input_tokens)}</b></span>}
    {u?.cache_creation_tokens > 0 && <span className="oa-usage-cache-write" title={ct('缓存写入 tokens', 'Cache creation tokens')}><svg viewBox="0 0 16 16" width="10" height="10" aria-hidden="true"><path d="M8 1v9m0 0 3-3m-3 3L5 7M3 13h10" fill="none" stroke="currentColor" strokeWidth="1.5" strokeLinecap="round" strokeLinejoin="round"/></svg>{ct('缓存写入', 'Cache write')} <b>{formatTokens(u.cache_creation_tokens)}</b></span>}
    {cacheReadTokens(u) > 0 && <span className="oa-usage-cache-read" title={ct('缓存读取 tokens', 'Cache read tokens')}><svg viewBox="0 0 16 16" width="10" height="10" aria-hidden="true"><path d="M8 15V6m0 0 3 3M8 6 5 9M3 3h10" fill="none" stroke="currentColor" strokeWidth="1.5" strokeLinecap="round" strokeLinejoin="round"/></svg>{ct('缓存读取', 'Cache read')} <b>{formatTokens(cacheReadTokens(u))}</b></span>}
    {u?.output_tokens > 0 && <span className="oa-usage-out" title={ct('输出 tokens', 'Output tokens')}><svg viewBox="0 0 16 16" width="10" height="10" aria-hidden="true"><path d="M8 4.5 12.5 9l-1.1 1.1L8 6.7l-3.4 3.4L3.5 9 8 4.5Z"/></svg>{ct('输出', 'Output')} <b>{formatTokens(u.output_tokens)}</b></span>}
    {hasCtx && <span className="oa-usage-ctx" title={ct(
      `AI 当前记住了 ${ctxMsgs} 条对话消息${ctxChars > 0 ? `，约 ${formatTokens(ctxChars)} 字` : ''}。上下文越长记忆越多，超出上限时旧消息会被自动裁剪。`,
      `AI currently holds ${ctxMsgs} messages in context${ctxChars > 0 ? ` (~${formatTokens(ctxChars)} chars)` : ''}. Older messages are trimmed when the limit is reached.`
    )}><svg viewBox="0 0 16 16" width="10" height="10" aria-hidden="true"><path d="M2 4h12v1.5H2V4zm0 3.5h9v1.5H2V7.5zm0 3.5h7v1.5H2V11z"/></svg>{ct('上下文', 'Ctx')} <b>{ctxMsgs > 0 ? `${ctxMsgs}msg` : ''}{ctxChars > 0 ? ` ${formatTokens(ctxChars)}ch` : ''}</b></span>}
  </div>
}

// 各内部 turn 用量累加得到整条回复总计
const sumUsages = (usages) => {
  if (!Array.isArray(usages) || !usages.length) return null
  return usages.reduce((acc, u) => ({
    input_tokens: acc.input_tokens + (u?.input_tokens || 0),
    cache_creation_tokens: acc.cache_creation_tokens + (u?.cache_creation_tokens || 0),
    cache_read_tokens: acc.cache_read_tokens + cacheReadTokens(u),
    output_tokens: acc.output_tokens + (u?.output_tokens || 0),
  }), { input_tokens: 0, cache_creation_tokens: 0, cache_read_tokens: 0, output_tokens: 0 })
}

export function GeneratedImageGallery({ content = '' }) {
  const paths = useMemo(() => extractGeneratedImagePaths(content), [content])
  const [activePath, setActivePath] = useState('')
  useEffect(() => {
    if (activePath && !paths.includes(activePath)) setActivePath('')
  }, [activePath, paths])
  useEffect(() => {
    if (!activePath) return undefined
    const onKeyDown = (event) => {
      if (event.key === 'Escape') setActivePath('')
      if (event.key !== 'ArrowLeft' && event.key !== 'ArrowRight') return
      const index = paths.indexOf(activePath)
      if (index < 0 || paths.length < 2) return
      const offset = event.key === 'ArrowLeft' ? -1 : 1
      setActivePath(paths[(index + offset + paths.length) % paths.length])
    }
    window.addEventListener('keydown', onKeyDown)
    const previousOverflow = document.body.style.overflow
    document.body.style.overflow = 'hidden'
    return () => {
      window.removeEventListener('keydown', onKeyDown)
      document.body.style.overflow = previousOverflow
    }
  }, [activePath, paths])
  if (!paths.length) return null
  const activeName = activePath?.split(/[\\/]/).filter(Boolean).pop() || 'generated-image'
  return <>
    <div className="oa-generated-images" aria-label="生成图片">
      {paths.map(path => {
        const name = path.split(/[\\/]/).filter(Boolean).pop() || '生成图片'
        return <button key={path} type="button" className="oa-generated-image-thumb" onClick={() => setActivePath(path)} aria-label={`查看原图 ${name}`} title={path}>
          <img src={generatedImageURL(path)} alt={name} loading="lazy" />
        </button>
      })}
    </div>
    {activePath && <div className="oa-generated-image-lightbox" role="dialog" aria-modal="true" aria-label="生成图片预览" onMouseDown={event => { if (event.target === event.currentTarget) setActivePath('') }}>
      <section>
        <header><b title={activePath}>{activeName}</b><button type="button" onClick={() => setActivePath('')} aria-label="关闭图片预览"><X size={18}/></button></header>
        <a className="oa-generated-image-original" href={generatedImageURL(activePath)} target="_blank" rel="noopener noreferrer" title="在新窗口查看原图"><img src={generatedImageURL(activePath)} alt={activeName}/></a>
        <footer>
          <a href={generatedImageURL(activePath)} target="_blank" rel="noopener noreferrer"><ExternalLink size={15}/>查看原图</a>
          <a href={generatedImageDownloadURL(activePath)} download><Download size={15}/>下载图片</a>
        </footer>
      </section>
    </div>}
  </>
}

export function ImagePreviewDialog({ images = [], activeIndex = -1, onClose, onChange }) {
  const image = images[activeIndex]
  useEffect(() => {
    if (!image) return undefined
    const onKeyDown = (event) => {
      if (event.key === 'Escape') {
        onClose?.()
        return
      }
      if (event.key !== 'ArrowLeft' && event.key !== 'ArrowRight') return
      if (images.length < 2) return
      const offset = event.key === 'ArrowLeft' ? -1 : 1
      onChange?.((activeIndex + offset + images.length) % images.length)
    }
    window.addEventListener('keydown', onKeyDown)
    const previousOverflow = document.body.style.overflow
    document.body.style.overflow = 'hidden'
    return () => {
      window.removeEventListener('keydown', onKeyDown)
      document.body.style.overflow = previousOverflow
    }
  }, [activeIndex, image, images, onChange, onClose])
  if (!image || typeof document === 'undefined') return null
  const label = image.name || ct('图片预览', 'Image preview')
  const hasMultiple = images.length > 1
  return createPortal(
    <div className="oa-image-preview-layer" role="dialog" aria-modal="true" aria-label={`图片预览 ${label}`} onMouseDown={(event) => { if (event.target === event.currentTarget) onClose?.() }}>
      <section className="oa-image-preview-dialog">
        <header className="oa-image-preview-head">
          <div><b title={label}>{label}</b><span>{activeIndex + 1} / {images.length}</span></div>
          <button type="button" onClick={onClose} aria-label="关闭图片预览" title="关闭图片预览" autoFocus><X size={18}/></button>
        </header>
        <div className="oa-image-preview-canvas">
          {hasMultiple && <button type="button" className="oa-image-preview-nav is-prev" onClick={() => onChange?.((activeIndex - 1 + images.length) % images.length)} aria-label="上一张图片" title="上一张图片"><ChevronLeft size={22}/></button>}
          <figure className="oa-image-preview-figure">
            <img src={image.src} alt={label}/>
            <figcaption>{ct('点击左右按钮或使用键盘方向键切换', 'Use the arrow buttons or keyboard arrows to switch')}</figcaption>
          </figure>
          {hasMultiple && <button type="button" className="oa-image-preview-nav is-next" onClick={() => onChange?.((activeIndex + 1) % images.length)} aria-label="下一张图片" title="下一张图片"><ChevronRight size={22}/></button>}
        </div>
        <footer className="oa-image-preview-foot">
          <span>{ct('原图保持完整比例显示', 'Original aspect ratio is preserved')}</span>
          <a href={image.src} target="_blank" rel="noopener noreferrer"><ExternalLink size={15}/>{ct('打开原图', 'Open original')}</a>
        </footer>
      </section>
    </div>,
    document.body,
  )
}

function initWorldline(sid = '') {
  return { sid, status: 'idle', data: null, error: null, switchingNodeId: '' }
}

export function worldlineLoadStarted(prev, sid) {
  const id = String(sid || '')
  if (!id) return initWorldline('')
  if (prev?.sid === id) return prev.data
    ? { ...prev, error: null }
    : { ...prev, status: 'loading', error: null, switchingNodeId: '' }
  return { sid: id, status: 'loading', data: null, error: null, switchingNodeId: '' }
}

export function worldlineForSession(state, sid) {
  const id = String(sid || '')
  if (state?.sid === id) return state
  return id ? { sid: id, status: 'loading', data: null, error: null, switchingNodeId: '' } : initWorldline('')
}

export function worldlineOwnsMappedNode(state, sid, nodeId) {
  const id = String(sid || '')
  const target = String(nodeId || '')
  return Boolean(id && target && state?.sid === id && Array.isArray(state.data?.nodes)
    && state.data.nodes.some(node => String(node.id) === target && node.mapping_status === 'mapped'))
}

function nodeVersionInfo(data, messageId) {
  if (!data || !messageId) return null
  const node = data.nodes?.find(item => String(item.tail_message_id) === String(messageId) || String(item.user_message_id) === String(messageId))
  if (!node) return null
  const siblings = (data.nodes || []).filter(item => item.parent_id != null && String(item.parent_id) === String(node.parent_id))
    .sort((a, b) => (a.ordinal ?? 0) - (b.ordinal ?? 0))
  const index = siblings.findIndex(item => String(item.id) === String(node.id))
  if (index < 0 || siblings.length < 2) return null
  return { index: index + 1, total: siblings.length, previous_node_id: siblings[index - 1]?.id || null, next_node_id: siblings[index + 1]?.id || null }
}

export function worldlineLoaded(prev, resp, sid) {
  if (prev.sid !== sid) return prev
  if (!resp || resp.status === 'unavailable') return { ...prev, status: resp?.status || 'unavailable', data: null, error: null }
  if (resp.status === 'degraded') return { ...prev, status: 'degraded', data: resp, error: null }
  return { ...prev, status: resp.nodes?.length ? 'ready' : 'empty', data: resp, error: null, switchingNodeId: '' }
}

function worldlineErrored(prev, err, sid) {
  if (prev.sid !== sid) return prev
  const message = err?.message || String(err) || '加载失败'
  return prev.data ? { ...prev, status: 'stale-error', error: message } : { ...prev, status: 'error', error: message }
}

export function projectWorldline(nodes = []) {
  const safeNodes = Array.isArray(nodes) ? nodes : []
  const byParent = new Map()
  safeNodes.forEach((node, sourceOrder) => {
    const key = String(node.parent_id ?? '')
    const items = byParent.get(key) || []
    items.push({ ...node, nodeId: String(node.id), sourceOrder })
    byParent.set(key, items)
  })
  byParent.forEach(items => items.sort((a, b) => (a.ordinal ?? a.sourceOrder) - (b.ordinal ?? b.sourceOrder)))
  const rows = []
  const visited = new Set()
  const visit = (parentId, depth) => {
    const children = byParent.get(String(parentId ?? '')) || []
    const childDepth = depth + (children.length > 1 ? 1 : 0)
    children.forEach((node, siblingIndex) => {
      if (visited.has(node.nodeId)) return
      visited.add(node.nodeId)
      rows.push({ ...node, branchDepth: childDepth, siblingCount: children.length, siblingIndex })
      visit(node.id, childDepth)
    })
  }
  visit('', 0)
  safeNodes.forEach((node, sourceOrder) => {
    const nodeId = String(node.id)
    if (!visited.has(nodeId)) rows.push({ ...node, nodeId, sourceOrder, branchDepth: 0, siblingCount: 1, siblingIndex: 0 })
  })
  return rows
}

export function WorldlineNavigator({ state, onRefresh, onSwitch, disabled, onClose }) {
  const data = state.data
  const nodes = Array.isArray(data?.nodes) ? data.nodes : []
  const [query, setQuery] = useState('')
  const currentPath = new Set((data?.current_path || []).map(String))
  const currentNodeId = data?.head == null ? '' : String(data.head)
  const switchingNodeId = String(state.switchingNodeId || '')
  const interactionLocked = disabled || Boolean(switchingNodeId)
  const showTree = nodes.length > 0 && ['ready', 'ok', 'stale-error', 'degraded'].includes(state.status)
  const projectedNodes = useMemo(() => projectWorldline(nodes), [nodes])
  const normalizedQuery = query.trim().toLocaleLowerCase()
  const visibleNodes = normalizedQuery
    ? projectedNodes.filter(node => [node.title, node.summary, node.model, node.id].filter(Boolean).join(' ').toLocaleLowerCase().includes(normalizedQuery))
    : projectedNodes
  const branchPointCount = projectedNodes.filter(node => node.siblingCount > 1 && node.siblingIndex === 0).length
  const degradedReason = data?.degraded_reason || state.error || '未知原因'

  return <aside className="oa-worldline-drawer" aria-label="对话分支导航">
    <div className="oa-worldline-header">
      <div className="oa-worldline-heading"><span className="oa-worldline-title"><GitBranch size={15}/>对话分支</span><span className="oa-worldline-subtitle">查看并切换当前对话的历史路径</span></div>
      <div className="oa-worldline-header-actions"><button type="button" className="oa-worldline-icon-btn" onClick={onRefresh} disabled={interactionLocked} aria-label="刷新对话分支"><RotateCw size={14}/></button><button type="button" className="oa-worldline-icon-btn" onClick={() => onClose?.()} aria-label="关闭对话分支"><X size={14}/></button></div>
    </div>
    <div className="oa-worldline-body">
      {showTree && <div className="oa-worldline-tools"><div className="oa-worldline-overview"><span>{nodes.length} 条记录</span><span>{branchPointCount ? `${branchPointCount} 处分叉` : '单一路径'}</span></div><label className="oa-worldline-search"><Search size={13} aria-hidden="true"/><input type="search" value={query} onChange={event => setQuery(event.target.value)} placeholder="搜索对话内容或模型" aria-label="搜索对话分支"/>{query && <button type="button" onClick={() => setQuery('')} aria-label="清空分支搜索"><X size={12}/></button>}</label></div>}
      {state.status === 'idle' && <div className="oa-worldline-state">打开一个对话后，这里会显示分支路径</div>}
      {state.status === 'loading' && <div className="oa-worldline-state">正在读取分支拓扑</div>}
      {state.status === 'unavailable' && <div className="oa-worldline-state">当前运行环境未提供分支导航</div>}
      {state.status === 'empty' && <div className="oa-worldline-state">发送消息后，这里会显示可切换的对话路径</div>}
      {state.status === 'degraded' && <div className="oa-worldline-alert" role="alert">分支服务暂不可用：{degradedReason}</div>}
      {state.status === 'error' && <div className="oa-worldline-alert" role="alert">读取分支失败：{state.error || '未知错误'}</div>}
      {state.status === 'stale-error' && <div className="oa-worldline-alert" role="alert">刷新失败，继续显示上次路径：{state.error || '未知错误'}</div>}
      {showTree && (visibleNodes.length ? <div className="oa-wl-tree">{visibleNodes.map(node => {
        const nodeId = node.nodeId
        const isCurrent = nodeId === currentNodeId
        const isPath = currentPath.has(nodeId)
        const isMapped = node.mapping_status === 'mapped'
        const isSwitching = switchingNodeId === nodeId
        const label = node.title || node.summary || `分支 ${node.id}`
        return <button key={nodeId} type="button" className={`oa-wl-node ${isPath ? 'is-path' : ''} ${isCurrent ? 'is-current' : ''} ${isSwitching ? 'is-switching' : ''}`} data-branch-depth={Math.min(Number(node.branchDepth) || 0, 7)} style={{ '--wl-depth': Math.min(Number(node.branchDepth) || 0, 7) }} onClick={() => isMapped && !isCurrent && !interactionLocked && onSwitch(node.id)} disabled={!isMapped || isCurrent || interactionLocked} aria-current={isCurrent ? 'step' : undefined}><span className="oa-wl-node-mark" aria-hidden="true">{isCurrent ? <Check size={13}/> : <GitBranch size={13}/>}</span><span className="oa-wl-node-copy"><b>{label}</b><small>{isCurrent ? '当前分支' : isPath ? '当前路径' : isMapped ? '可切换' : '旧记录 · 不可切换'}</small></span>{isSwitching && <span className="oa-wl-switching">切换中</span>}</button>
      })}</div> : <div className="oa-worldline-state oa-worldline-no-results">没有匹配的对话记录</div>)}
    </div>
  </aside>
}

export function worldlineUnavailableMessage(state) {
  const rawReason = String(state?.degraded_reason || '').trim()
  const reason = rawReason.toLowerCase()
  if (!rawReason || reason === 'missing' || reason === 'inactive') {
    return ct('当前会话还没有可用的世界线记录。完成一轮成功对话后，系统会自动创建节点。', 'No worldline records are available for this chat yet. A node will be created after a completed reply.')
  }
  if (reason === 'malformed' || reason === 'legacy') {
    return ct('当前会话的世界线记录格式异常或版本较旧，请刷新后重试。', 'This chat’s worldline data is invalid or from an older version. Refresh and try again.')
  }
  return ct(`世界线暂不可用：${rawReason}`, `Worldline is temporarily unavailable: ${rawReason}`)
}

export function WorldlinePanel({ state, loading, switchingId, disabled, onClose, onRefresh, onSwitch }) {
  const rows = useMemo(() => buildWorldlineRows(state?.nodes, state?.current_path, state?.head), [state])
  const branchCount = rows.filter(row => !row.onPath).length
  const hasState = Boolean(state)
  const unavailable = hasState && state.available === false
  return <aside className="oa-context-drawer oa-worldline-drawer" aria-label="世界线分支">
    <div className="oa-context-head">
      <div><b>世界线</b><span>{!hasState && loading ? '正在读取世界线数据' : unavailable ? '当前会话暂无世界线数据' : `共 ${rows.length} 个节点 · ${branchCount} 个分支节点`}</span></div>
      <div className="oa-context-actions"><button className="oa-context-refresh" type="button" onClick={onRefresh} disabled={loading} title={loading ? '正在刷新世界线' : '刷新世界线'}><RotateCw size={14}/><span>{loading ? '刷新中…' : '刷新'}</span></button><button type="button" onClick={onClose} aria-label="关闭世界线" title="关闭世界线"><X size={15}/></button></div>
    </div>
    {!hasState && <div className="oa-worldline-empty">{loading ? '正在初始化并读取当前会话的世界线…' : '还没有世界线记录，发送一条消息后再试。'}</div>}
    {unavailable && <div className="oa-worldline-empty">{worldlineUnavailableMessage(state)}</div>}
    {hasState && !unavailable && rows.length === 0 && <div className="oa-worldline-empty">{loading ? '加载中…' : '暂无节点'}</div>}
    {hasState && !unavailable && rows.length > 0 && <div className="oa-worldline-list">{rows.map(row => <div key={row.node.id} className={`oa-worldline-row${row.onPath ? ' on-path' : ''}${row.isCurrent ? ' is-current' : ''}`} style={{ '--wl-level':row.level }}>
      <span className="oa-worldline-dot" aria-hidden="true"/>
      <div className="oa-worldline-info"><b title={row.node.title || row.node.id}>{row.node.title || `节点 ${String(row.node.id).slice(0, 8)}`}</b><span>{row.node.untracked_changes && <em className="oa-worldline-untracked" aria-label="外部改动" title={Array.isArray(row.node.untracked_files) && row.node.untracked_files.length ? `外部改动文件：${row.node.untracked_files.join('、')}` : '该节点包含未追踪的外部改动'}>外部改动</em>}{row.node.created_at ? ` · ${fmtTime(row.node.created_at)}` : ''}</span></div>
      {row.isCurrent ? <em className="oa-worldline-current">当前</em> : row.node.mapping_status === 'mapped' ? <button type="button" className="oa-worldline-switch" disabled={disabled || !!switchingId} onClick={()=>onSwitch(row.node.id)}>{switchingId === row.node.id ? '切换中…' : '切换'}</button> : <em className="oa-worldline-unmapped" title="该节点没有可恢复的对话映射">仅记录</em>}
    </div>)}</div>}
    {state?.truncated && <div className="oa-worldline-empty">节点过多，已截断显示。</div>}
  </aside>
}

export const CommandResultCard = memo(function CommandResultCard({ result = {} }) {
  const command = `/${String(result.command || '').replace(/^\//, '')}`
  const summary = commandResultSummary(result)
  const treeNodes = Array.isArray(result.tree?.nodes) ? result.tree.nodes : []
  const services = Array.isArray(result.services) ? result.services : []
  const commands = Array.isArray(result.commands) ? result.commands : []
  const records = Array.isArray(result.records) ? result.records : []
  const status = result.session && typeof result.session === 'object' ? result.session : null

  return (
    <section className="oa-command-result" aria-label={`${command} 命令结果`}>
      <header><Check size={17}/><div><b>{summary}</b><span>{command}</span></div></header>
      {command === '/worldline' && result.action !== 'restore' && (
        treeNodes.length > 0
          ? <div className="oa-command-list" aria-label="世界线节点">
              {treeNodes.map((node, index) => {
                const id = String(node?.id || node?.node_id || node?.key || '')
                const title = String(node?.title || node?.label || node?.summary || node?.content_preview || '')
                return <div key={id || index}><code>{id || `#${index + 1}`}</code><span>{title || '未命名节点'}</span></div>
              })}
            </div>
          : <div className="oa-command-empty">暂无世界线节点</div>
      )}
      {services.length > 0 && <div className="oa-command-services">
        {services.map((service, index) => <div key={service?.name || index}>
          <i className={`oa-command-dot ${service?.running ? 'is-running' : ''}`}/>
          <b>{service?.name || `service-${index + 1}`}</b>
          <span>{service?.running ? '运行中' : '未运行'}</span>
          <em>{service?.status || service?.message || ''}</em>
        </div>)}
      </div>}
      {commands.length > 0 && <div className="oa-command-list">
        {commands.map((item, index) => {
          const name = typeof item === 'string' ? item : (item?.command || item?.name || '')
          const description = typeof item === 'string' ? '' : (item?.description || item?.usage || '')
          return <div key={name || index}><code>{name || `#${index + 1}`}</code><span>{description}</span></div>
        })}
      </div>}
      {status && command === '/status' && <dl className="oa-command-kv">
        <div><dt>Session</dt><dd>{status.id || '-'}</dd></div>
        <div><dt>Messages</dt><dd>{Number(status.message_count || 0)}</dd></div>
      </dl>}
      {records.length > 0 && <details className="oa-command-records"><summary>{records.length} 条工具审计记录</summary><pre>{JSON.stringify(records, null, 2)}</pre></details>}
      {command === '/export' && result.filename && <div className="oa-command-download"><FileOutput size={15}/><span>{result.filename}</span><b>已下载</b></div>}
    </section>
  )
})

export const worldlineRestoreCommand = (nodeID, mode = 'both', target = 'at') => {
  const id = String(nodeID || '').trim()
  const restoreMode = ['both', 'conversation', 'code'].includes(mode) ? mode : 'both'
  const restoreTarget = ['at', 'before'].includes(target) ? target : 'at'
  return id ? `/worldline restore ${id} ${restoreMode} ${restoreTarget}` : ''
}

export const isWorldlinePickerResult = (result) => {
  const commandName = String(result?.command || '').replace(/^\//, '').toLowerCase()
  const nodes = result?.tree?.nodes
  return commandName === 'worldline' && result?.action !== 'restore' && Array.isArray(nodes) && nodes.length > 0
}

export const WorldlineRestoreDialog = memo(function WorldlineRestoreDialog({ nodes = [], onClose, onSelect }) {
  const [selectedNodeID, setSelectedNodeID] = useState('')
  const [restoreMode, setRestoreMode] = useState('both')
  const [restoreTarget, setRestoreTarget] = useState('at')

  useEffect(() => {
    const onKeyDown = (event) => { if (event.key === 'Escape') onClose?.() }
    window.addEventListener('keydown', onKeyDown)
    return () => window.removeEventListener('keydown', onKeyDown)
  }, [onClose])

  return (
    <div className="oa-worldline-backdrop" onMouseDown={(event)=>{ if (event.target === event.currentTarget) onClose?.() }}>
      <section className="oa-worldline-dialog" role="dialog" aria-modal="true" aria-labelledby="oa-worldline-title">
        <header className="oa-worldline-dialog-head">
          <div>
            <small>WORLDLINE</small>
            <h2 id="oa-worldline-title">{ct('选择回退点', 'Choose a rollback point')}</h2>
            <p>{ct('配置恢复范围与位置后填入命令，由你确认后发送。', 'Choose the restore scope and position, then insert the command for review before sending.')}</p>
          </div>
          <button type="button" onClick={onClose} aria-label={ct('关闭回退点选择', 'Close rollback-point picker')}><X size={17}/></button>
        </header>
        <div className="oa-worldline-node-list">
          {nodes.map((node, index) => {
            const nodeID = String(node?.id || '').trim()
            const ordinal = node?.ordinal ?? index + 1
            const title = node?.title || node?.summary || node?.name || `${ct('回退点', 'Rollback point')} ${ordinal}`
            const selected = !!nodeID && nodeID === selectedNodeID
            return <button key={nodeID || index} className={`oa-worldline-node${selected ? ' is-selected' : ''}`} type="button" disabled={!nodeID} aria-pressed={selected} onClick={()=>setSelectedNodeID(nodeID)}>
              <span className="oa-worldline-node-no">{ordinal}</span>
              <span className="oa-worldline-node-copy"><b>{title}</b><code>{nodeID || ct('缺少节点 ID', 'Missing node ID')}</code></span>
              <span className="oa-worldline-node-action">{selected ? ct('已选择', 'Selected') : ct('选择', 'Select')}</span>
            </button>
          })}
        </div>
        <div className="oa-worldline-options">
          <fieldset>
            <legend>{ct('恢复范围', 'Restore scope')}</legend>
            <div>
              {[['both', ct('对话与代码', 'Conversation and code')], ['conversation', ct('仅对话', 'Conversation only')], ['code', ct('仅代码', 'Code only')]].map(([value, label]) =>
                <button key={value} type="button" className={restoreMode === value ? 'is-selected' : ''} aria-pressed={restoreMode === value} onClick={()=>setRestoreMode(value)}>{label}</button>)}
            </div>
          </fieldset>
          <fieldset>
            <legend>{ct('恢复位置', 'Restore position')}</legend>
            <div>
              {[['at', ct('定位到节点', 'At the node')], ['before', ct('节点之前', 'Before the node')]].map(([value, label]) =>
                <button key={value} type="button" className={restoreTarget === value ? 'is-selected' : ''} aria-pressed={restoreTarget === value} onClick={()=>setRestoreTarget(value)}>{label}</button>)}
            </div>
          </fieldset>
        </div>
        <footer>
          <span>{ct(`${nodes.length} 个可用回退点`, `${nodes.length} rollback points available`)}</span><kbd>Esc</kbd><span>{ct('关闭', 'Close')}</span>
          <button className="oa-worldline-confirm" type="button" disabled={!selectedNodeID} onClick={()=>onSelect?.(selectedNodeID, restoreMode, restoreTarget)}>{ct('确认并填入命令', 'Confirm and insert command')}</button>
        </footer>
      </section>
    </div>
  )
})

const ChatErrorCard = memo(function ChatErrorCard({ message, onRetry }) {
  const info = useMemo(() => chatErrorPresentation(message), [message])
  return <section className={`oa-chat-error-card source-${info.source}`} role="alert" aria-label={`${info.sourceLabel}错误`}>
    <header>
      <span className="oa-chat-error-icon"><CircleAlert size={18}/></span>
      <div><small>{info.sourceLabel}<code>{info.code}</code></small><h3>{info.summary}</h3></div>
    </header>
    <p>{info.hint}</p>
    <div className="oa-chat-error-actions">
      {onRetry && <button type="button" onClick={onRetry}><RotateCw size={14}/>重新发送</button>}
      {info.detail && <details><summary>查看技术详情</summary><pre>{info.detail}</pre></details>}
    </div>
  </section>
})

export const ChatMessage = memo(function ChatMessage({ message: m, models = [], pending, onAskReply, onEditResend, onRetry, editDisabled = false, clockNow = 0, version, onSwitchVersion, switchingNodeId = '', chatInstanceID = '' }) {
  const userText = m.role === 'user' ? stripUserAttachmentBlock(m.content) : m.content
  const messageFiles = Array.isArray(m.files) ? m.files : []
  const imageFiles   = messageFiles.filter(isImageFile)
  const savedFilePaths = m.role === 'user' ? extractSavedFilePaths(m.content) : []
  const pendingFiles = savedFilePaths.length > 0 ? [] : messageFiles.filter((file) => !isImageFile(file))
  const modelIdentity = messageModelIdentity(m, models)
  const turnUsages = m.role === 'assistant' && Array.isArray(m.usages) && m.usages.length > 0 ? m.usages : null
  const hasUsage = !turnUsages && m.role === 'assistant' && m.usage && (m.usage.input_tokens > 0 || m.usage.output_tokens > 0)
  const usageTotal = turnUsages ? sumUsages(turnUsages) : (hasUsage ? m.usage : null)
  const elapsedMs = getElapsedMs(m, clockNow || Date.now())
  const showUsageRow = m.role === 'assistant' && (usageHasTokens(usageTotal) || elapsedMs > 0)
  const usageLabel = m.role === 'assistant' ? '总计' : null
  const stoppedByUser = m.error && /(?:\[已中止生成\]|^已停止生成$)/.test(String(m.content || '').trim())
  const showErrorCard = m.error && !stoppedByUser
  const [editing, setEditing] = useState(false)
  const [draft, setDraft] = useState(userText)
  const [editError, setEditError] = useState('')
  const [copied, setCopied] = useState(false)
  const [previewImageIndex, setPreviewImageIndex] = useState(-1)
  const previewImages = imageFiles.map((file, index) => ({
    key: `${uploadFileName(file)}-${index}`,
    name: uploadFileName(file),
    src: addChatInstanceToURL(uploadFileSource(file), chatInstanceID),
  })).filter(image => image.src)
  const resetDraft = () => { setDraft(userText); setEditError(''); setEditing(false) }
  const submitEdit = async () => {
    if (!draft.trim() || draft.trim() === String(userText || '').trim()) { setEditing(false); return }
    setEditError('')
    try {
      await onEditResend?.(m.id, draft.trim())
      setEditing(false)
    } catch (error) {
      setEditError(error?.message || String(error))
    }
  }
  const copyContent = () => {
    navigator.clipboard?.writeText(m.role === 'user' ? userText : (m.content || '')).then?.(() => {
      setCopied(true)
      setTimeout(() => setCopied(false), 1500)
    })
  }
  return <article id={`msg-${m.id}`} data-msg-role={m.role} className={`oa-message ${m.role} ${m.error?'error':''}`}>
    <div className="oa-avatar">{m.role === 'user' ? '你' : 'GA'}</div>
    <div className="oa-bubble">
      <div className="oa-msg-body">
      <div className="oa-meta"><b className="oa-meta-author">{m.role === 'user' ? 'You' : 'GenericAgent'}</b>{modelIdentity.label && <span className="oa-model-id" title={modelIdentity.title}>{modelIdentity.label}</span>}{m.created_at && <span className="oa-meta-time">{fmtTime(m.created_at)}</span>}{m.content && <button type="button" className="oa-mini-copy" onClick={copyContent} aria-label="复制消息">{copied ? <Check size={13}/> : <Copy size={13}/>}</button>}{m.role === 'user' && !pending && typeof onEditResend === 'function' && <button type="button" className="oa-mini-copy oa-edit-btn" onClick={() => { setDraft(userText); setEditError(''); setEditing(value => !value) }} disabled={editDisabled} aria-label="编辑并重新发送"><Edit3 size={13}/></button>}</div>
      {imageFiles.length > 0 && <div className="oa-msg-images" aria-label="消息图片">{imageFiles.map((file, i) => {
        const name = uploadFileName(file)
        const src = addChatInstanceToURL(uploadFileSource(file), chatInstanceID)
        const key = `${name}-${i}`
        const previewIndex = previewImages.findIndex(image => image.key === key)
        return <button className="oa-msg-image-link" key={key} type="button" onClick={() => previewIndex >= 0 && setPreviewImageIndex(previewIndex)} disabled={!src} aria-label={src ? `查看图片 ${name}` : `图片不可用 ${name}`} title={src ? `点击查看原图：${name}` : name}>
          <span className="oa-msg-image-stage"><img className="oa-msg-image" src={src} alt={name} loading="lazy" /></span>
          <span className="oa-msg-image-name">{name}</span>
        </button>
      })}</div>}
      <ImagePreviewDialog images={previewImages} activeIndex={previewImageIndex} onClose={() => setPreviewImageIndex(-1)} onChange={setPreviewImageIndex} />
      {m.role === 'user' && (savedFilePaths.length > 0 || pendingFiles.length > 0) && <div className="oa-message-files">
        {savedFilePaths.map((savedPath, i) => <FileAttachment key={`${savedPath}-${i}`} path={savedPath} />)}
        {pendingFiles.map((file, i) => {
          const name = uploadFileName(file)
          const visual = getFileVisual(name)
          const Icon = visual.Icon
          return <span className={`oa-pending-file oa-file-kind-${visual.kind}`} key={`${name}-${i}`} title={`\u5f85\u4e0a\u4f20\uff1a${name}`}><Icon size={18}/><b>{name}</b></span>
        })}
      </div>}
      {m.role === 'assistant' ? <>{m.commandResult ? <CommandResultCard result={m.commandResult} /> : showErrorCard ? <ChatErrorCard message={m} onRetry={onRetry}/> : <AssistantContent content={m.content} pending={pending} onAskReply={onAskReply} turnUsages={turnUsages} ultraplan_state={m.ultraplan_state} />}{!showErrorCard && <GeneratedImageGallery content={m.content}/>}</> : editing ? <div className="oa-message-editor"><textarea className="oa-edit-textarea" aria-label="编辑已发送消息" value={draft} autoFocus rows={Math.min(10, (draft.match(/\n/g) || []).length + 2)} onChange={event => setDraft(event.target.value)} onKeyDown={event => { if (event.key === 'Enter' && (event.ctrlKey || event.metaKey)) submitEdit(); if (event.key === 'Escape') resetDraft() }}/>{editError && <div role="alert" className="oa-message-editor-error">{editError}</div>}<div className="oa-edit-actions"><button type="button" className="oa-edit-submit" onClick={submitEdit} disabled={!draft.trim() || editDisabled}><Send size={13}/>发送</button><button type="button" className="oa-edit-cancel" onClick={resetDraft}>取消</button></div></div> : (userText && <MarkdownBlock text={userText} />)}
      {showUsageRow && <UsageRow u={usageTotal} label={usageLabel} className="oa-usage-total" elapsedMs={elapsedMs} live={pending} />}
      {version && <div className="oa-msg-version" aria-label={`消息版本 ${version.index}/${version.total}`}><button type="button" onClick={() => onSwitchVersion?.(version.previous_node_id)} disabled={!version.previous_node_id || !!switchingNodeId} aria-label="上一个消息版本"><ChevronLeft size={14}/></button><span>{version.index} / {version.total}</span><button type="button" onClick={() => onSwitchVersion?.(version.next_node_id)} disabled={!version.next_node_id || !!switchingNodeId} aria-label="下一个消息版本"><ChevronRight size={14}/></button></div>}
      </div>
      {m.goal_state && <GoalStatusCard state={m.goal_state} pending={pending} />}
    </div>
  </article>
})

const MessageList = memo(function MessageList({ messages, models, isCurrentRunning, onAskReply, onEditResend, onRetry, clockNow, worldline, onSwitchVersion, chatInstanceID = '' }) {
  return <>
    {messages.flatMap((m, i) => {
      const day = timelineKey(m.created_at)
      const prevDay = i > 0 ? timelineKey(messages[i - 1]?.created_at) : ''
      const nodes = []
      if (i === 0 || day !== prevDay) nodes.push(<div key={`tl-${day}-${i}`} className="oa-timeline"><span>{fmtTimelineDate(m.created_at)}</span></div>)
      const retrySource = m.error && i > 0 && messages[i - 1]?.role === 'user' ? messages[i - 1] : null
      nodes.push(<ChatMessage key={m.id} message={m} models={models} pending={isCurrentRunning && i === messages.length - 1} onAskReply={onAskReply} onEditResend={onEditResend} onRetry={retrySource ? () => onRetry?.(retrySource) : undefined} editDisabled={isCurrentRunning} clockNow={clockNow} version={messageVersionInfo(worldline, m.id)} onSwitchVersion={onSwitchVersion} switchingNodeId={worldline?.switchingNodeId} chatInstanceID={chatInstanceID} />)
      return nodes
    })}
  </>
})

export function PlanTodoCard({ plan }) {
  const listRef = useRef(null)
  const [expanded, setExpanded] = useState(true)
  const panelId = React.useId()
  const active = Boolean(plan?.active)
  const items = Array.isArray(plan?.items) ? plan.items : []
  const done = Number.isFinite(Number(plan?.done)) ? Number(plan.done) : items.filter(item => item?.status === 'done').length
  const total = Number.isFinite(Number(plan?.total)) ? Number(plan.total) : items.length
  const currentIndex = items.findIndex(item => item?.status !== 'done')
  const percent = total > 0 ? Math.max(0, Math.min(100, Math.round((done / total) * 100))) : 0
  const complete = Boolean(plan?.complete)
  const placeholder = Boolean(plan?.placeholder)

  useLayoutEffect(() => {
    if (!active || !expanded || placeholder || currentIndex < 0) return undefined
    const list = listRef.current
    const current = list?.querySelector('[aria-current="step"]')
    if (!list || !current) return undefined
    const revealCurrent = () => {
      const listRect = list.getBoundingClientRect()
      const currentRect = current.getBoundingClientRect()
      if (currentRect.top < listRect.top) list.scrollTop -= listRect.top - currentRect.top + 6
      else if (currentRect.bottom > listRect.bottom) list.scrollTop += currentRect.bottom - listRect.bottom + 6
    }
    revealCurrent()
    const resizeObserver = typeof ResizeObserver === 'undefined' ? null : new ResizeObserver(revealCurrent)
    resizeObserver?.observe(list)
    resizeObserver?.observe(current)
    window.addEventListener('resize', revealCurrent)
    return () => {
      resizeObserver?.disconnect()
      window.removeEventListener('resize', revealCurrent)
    }
  }, [active, currentIndex, expanded, placeholder, total])

  if (!active) return null
  const stateLabel = complete ? ct('已完成', 'Completed') : placeholder ? ct('规划中', 'Planning') : total > 0 ? ct('执行中', 'Running') : ct('准备中', 'Preparing')
  const detailLabel = complete
    ? ct('所有步骤均已完成', 'All steps completed')
    : placeholder
      ? ct('正在生成可执行步骤', 'Generating executable steps')
      : currentIndex >= 0 && total > 0
        ? `正在处理第 ${currentIndex + 1} 步`
        : '等待任务步骤'

  return <section className={`oa-plan-card${complete ? ' is-complete' : ''}`} aria-label="任务执行计划">
    <button type="button" className="oa-plan-head" onClick={() => setExpanded(value => !value)}
      aria-expanded={expanded} aria-controls={panelId} aria-label={expanded ? '收起执行计划' : '展开执行计划'}>
      <span className="oa-plan-identity">
        <span className="oa-plan-mark" aria-hidden="true">{complete ? <Check size={15}/> : <Clock3 size={14}/>}</span>
        <span className="oa-plan-heading"><span className="oa-plan-title">执行计划</span><span className="oa-plan-detail">{detailLabel}</span></span>
      </span>
      <span className="oa-plan-summary">
        <span className="oa-plan-state"><i aria-hidden="true"/>{stateLabel}</span>
        <span className="oa-plan-count" aria-label={`已完成 ${done} 项，共 ${total} 项`}><strong>{done}</strong><span>/ {total}</span></span>
        <span className="oa-plan-chevron" aria-hidden="true">{expanded ? <ChevronDown size={15}/> : <ChevronLeft size={15}/>}</span>
      </span>
    </button>
    <div id={panelId} className="oa-plan-body" hidden={!expanded}>
      <div className="oa-plan-progress" role="progressbar" aria-label="任务完成进度" aria-valuemin="0" aria-valuemax="100" aria-valuenow={percent}>
        <span style={{ width: `${percent}%` }}/>
      </div>
      {placeholder ? <div className="oa-plan-placeholder"><span aria-hidden="true"/><span>正在整理步骤</span><code title={plan.pathHint || 'plan.md'}>{plan.pathHint || 'plan.md'}</code></div> :
        <ol ref={listRef} className="oa-plan-list">
          {items.map((item, index) => {
            const itemComplete = item?.status === 'done'
            const current = !itemComplete && index === currentIndex
            return <li key={`${index}-${item?.content || ''}`} className={`${itemComplete ? 'is-done' : 'is-open'}${current ? ' is-current' : ''}`} aria-current={current ? 'step' : undefined}>
              <span className="oa-plan-status" aria-hidden="true">{itemComplete ? <Check size={11}/> : current ? <Clock3 size={10}/> : <span>{index + 1}</span>}</span>
              <span className="oa-plan-copy">{item?.content || `步骤 ${index + 1}`}</span>
              {current && <span className="oa-plan-now">当前</span>}
            </li>
          })}
        </ol>}
      {plan.step && <div className="oa-plan-step"><span>当前动作</span><p>{plan.step}</p></div>}
    </div>
  </section>
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
      <button type="button" disabled={disabled} title={label} aria-label={ariaLabel ? `${ariaLabel}: ${label}` : undefined} onClick={() => setOpen(o => !o)}>
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

export default function ChatApp({ uiScale = 1, onUiScaleChange = () => {} }) {
  // Theme state: sync with localStorage and system preference
  const [theme, setTheme] = useState(getInitialTheme)
  useEffect(() => {
    const activeTheme = applyThemeToDocument(theme)
    localStorage.setItem('ga-admin-theme', activeTheme.id)
    window.dispatchEvent(new CustomEvent('ga-admin-theme-change', { detail: activeTheme.id }))
  }, [theme])

  useEffect(() => {
    document.documentElement.lang = chatLanguage() === 'en' ? 'en' : 'zh-CN'
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
  const [chatInstanceID, setChatInstanceID] = useState(initialChatInstanceID)
  const [chatInstances, setChatInstances] = useState([])
  const [chatInstancesLoading, setChatInstancesLoading] = useState(true)
  const [sessions, setSessions] = useState([])
  const [projects, setProjects] = useState([])
  const [sidebarTab, setSidebarTab] = useState('history')
  const [sidebarSearch, setSidebarSearch] = useState('')
  const [sessionSearchOpen, setSessionSearchOpen] = useState(false)
  const [sessionSearchQuery, setSessionSearchQuery] = useState('')
  const [sessionSearchScope, setSessionSearchScope] = useState('all')
  const [sessionSearchHistory, setSessionSearchHistory] = useState(() => loadSessionSearchHistory())
  const [sessionSearchResults, setSessionSearchResults] = useState([])
  const [sessionSearchLoading, setSessionSearchLoading] = useState(false)
  const [sessionSearchError, setSessionSearchError] = useState('')
  const [expandedProjectNames, setExpandedProjectNames] = useState(() => new Set())
  const [draftSessionIds, setDraftSessionIds] = useState(() => new Set(listChatSessionDraftIds()))
  const [sid, setSid] = useState('')
  const [messages, setMessages] = useState([])
  const [rawHistory, setRawHistory] = useState([])
  const [historyInfo, setHistoryInfo] = useState([])
  const [workingState, setWorkingState] = useState(null)
  const [planState, setPlanState] = useState(null)
  const [contextOpen, setContextOpen] = useState(false)
  const [mobileToolsOpen, setMobileToolsOpen] = useState(false)
  const mobileToolsTriggerRef = useRef(null)
  useEffect(() => {
    if (!mobileToolsOpen) return undefined
    const closeOnEscape = (event) => {
      if (event.key !== 'Escape') return
      setMobileToolsOpen(false)
      requestAnimationFrame(() => mobileToolsTriggerRef.current?.focus())
    }
    const closeAboveBreakpoint = () => {
      if (window.innerWidth > 420) setMobileToolsOpen(false)
    }
    document.addEventListener('keydown', closeOnEscape)
    window.addEventListener('resize', closeAboveBreakpoint)
    return () => {
      document.removeEventListener('keydown', closeOnEscape)
      window.removeEventListener('resize', closeAboveBreakpoint)
    }
  }, [mobileToolsOpen])
  const [btwRailOpen, setBtwRailOpen] = useState(true)
  const [prompt, setPrompt] = useState('')
  const [busy, setBusy] = useState(false)
  const [streamingSid, setStreamingSid] = useState('')
  const [subagents, setSubagents] = useState([])
  const subagentLikely = useMemo(() => hasSubagentLaunch(messages), [messages])
  useEffect(() => {
    if (!sid || !subagentLikely) { setSubagents([]); return undefined }
    let alive = true
    const tick = () => { chatApi(`/api/chat/subagents/${encodeURIComponent(sid)}`).then(res => { if (alive) setSubagents(Array.isArray(res?.subagents) ? res.subagents : []) }).catch(() => {}) }
    tick()
    const timer = busy ? setInterval(tick, 5000) : null
    return () => { alive = false; if (timer) clearInterval(timer) }
  }, [sid, busy, subagentLikely, chatInstanceID])
  const [err, setErr] = useState('')
  const [collapsed, setCollapsed] = useState(() => isNarrowChatViewport())
  const [notice, setNotice] = useState('')
  const [llms, setLlms] = useState([])
  const [llmNo, setLlmNo] = useState(0)
  const [modelSwitching, setModelSwitching] = useState(false)
  const [reasoningEffort, setReasoningEffort] = useState('off')
  const [extraSysPrompts, setExtraSysPrompts] = useState([])
  const [extraSysPromptPresetID, setExtraSysPromptPresetID] = useState('')
  const [promptPresets, setPromptPresets] = useState([])
  const [extraPromptOpen, setExtraPromptOpen] = useState(false)
  const [extraPromptSelection, setExtraPromptSelection] = useState('')
  const [extraPromptTargetSid, setExtraPromptTargetSid] = useState('')
  const [promptPresetManagerOpen, setPromptPresetManagerOpen] = useState(false)
  const [extraPromptDraft, setExtraPromptDraft] = useState([])
  const [extraPromptSaving, setExtraPromptSaving] = useState(false)
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
  const [guidingQueueId, setGuidingQueueId] = useState('')
  const [dragging, setDragging] = useState(false)
  const [autoFollow, setAutoFollow] = useState(true)
  const [showFollow, setShowFollow] = useState(false)
  const [cmdDrawer, setCmdDrawer] = useState({ open: false, filter: '', selectedIdx: 0 })
  const [cmdManagerOpen, setCmdManagerOpen] = useState(false)
  const [worldlineRestorePicker, setWorldlineRestorePicker] = useState(null)
  const [slashCommands, setSlashCommands] = useState(BUILTIN_SLASH_COMMANDS)
  const [cfg, setCfg] = useState(null)
  const [cmdEditIdx, setCmdEditIdx] = useState(-1)
  const [cmdEditCmd, setCmdEditCmd] = useState('')
  const [cmdEditDesc, setCmdEditDesc] = useState('')
  const [cmdEditContent, setCmdEditContent] = useState('')
  const [isMobile, setIsMobile] = useState(() => isMobileViewport())
  const [streamClock, setStreamClock] = useState(() => Date.now())
  const threadRef = useRef(null)
  const endRef = useRef(null)
  const fileRef = useRef(null)
  const promptRef = useRef(null)
  const cmdDrawerRef = useRef(null)
  const sessionSearchTriggerRef = useRef(null)
  const sessionSearchRequestRef = useRef(0)
  const selectedCmdRef = useRef(null)
  const streamAbortRef = useRef(null)
  const chatInstanceRef = useRef(chatInstanceID)
  const chatRequestEpochRef = useRef(0)
  const chatApi = useCallback(async (url, options) => {
    const epoch = chatRequestEpochRef.current
    const result = await api(addChatInstanceToURL(url, chatInstanceRef.current), options)
    if (epoch !== chatRequestEpochRef.current) throw new DOMException('Chat instance changed', 'AbortError')
    return result
  }, [])
  const chatFetch = useCallback(async (url, options) => {
    const epoch = chatRequestEpochRef.current
    const result = await fetch(addChatInstanceToURL(url, chatInstanceRef.current), options)
    if (epoch !== chatRequestEpochRef.current) {
      result.body?.cancel?.().catch?.(() => {})
      throw new DOMException('Chat instance changed', 'AbortError')
    }
    return result
  }, [])
  const runSeqRef = useRef(0)
  const activeRunRef = useRef(false)
  const guidingQueueRef = useRef('')
  const openSeqRef = useRef(0)
  const activeSidRef = useRef('')
  const memoryDraftRef = useRef(consumeMemoryChatDraft())
  const extraPromptSelectionSeqRef = useRef(0)
  const [worldlineOpen, setWorldlineOpen] = useState(false)
  const [worldlineState, setWorldlineState] = useState(null)
  const [worldlineLoading, setWorldlineLoading] = useState(false)
  const [worldlineSwitchingId, setWorldlineSwitchingId] = useState('')
  const worldlineSeqRef = useRef(0)
  const messagesRef = useRef([])
  // Keep a synchronous mirror of `messages` so async flows (e.g. re-attaching to a running
  // stream after a page refresh) can read the committed list without waiting for a state updater.
  useEffect(() => { messagesRef.current = messages }, [messages])
  const scrollModeRef = useRef('auto')
  const autoFollowRef = useRef(true)
  const previousScrollTopRef = useRef(0)
  useLayoutEffect(() => { autoFollowRef.current = autoFollow }, [autoFollow])
  const queuedRef = useRef([])
  const chatScope = useRef(null)
  const persistSessionDraft = useCallback((sessionId, value) => {
    const id = String(sessionId || '').trim()
    const draft = typeof value === 'string' ? value : String(value || '')
    saveChatSessionDraft(id, draft)
    if (!id) return
    setDraftSessionIds(current => {
      const hasDraft = Boolean(draft)
      if (current.has(id) === hasDraft) return current
      const next = new Set(current)
      if (hasDraft) next.add(id)
      else next.delete(id)
      return next
    })
  }, [])
  const clearSessionDrafts = useCallback((sessionIds) => {
    const values = Array.isArray(sessionIds) ? sessionIds : [sessionIds]
    const ids = values.map(value => String(value || '').trim()).filter(Boolean)
    clearChatSessionDrafts(ids)
    if (!ids.length) return
    setDraftSessionIds(current => {
      const next = new Set(current)
      let changed = false
      for (const id of ids) changed = next.delete(id) || changed
      return changed ? next : current
    })
  }, [])
  const setSessionPrompt = useCallback((value, sessionId = activeSidRef.current) => {
    const next = typeof value === 'string' ? value : String(value || '')
    setPrompt(next)
    persistSessionDraft(sessionId, next)
  }, [persistSessionDraft])
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
    if (!mq) return undefined
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
    if (!mq) return undefined
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
    const inProjectScope = slashFilter === '/project' || slashFilter.startsWith('/project ')
    const inContinueScope = slashFilter === '/continue' || slashFilter.startsWith('/continue ')
    const inReviewScope = slashFilter === '/review' || slashFilter.startsWith('/review ')
    const inImproveScope = slashFilter === '/improve' || slashFilter.startsWith('/improve ')
    const inUltraPlanScope = slashFilter === '/ultraplan' || slashFilter.startsWith('/ultraplan ')
    const isReviewNaturalLanguage = /^\/review\s+\S/.test(slashFilter) && !childAllowed('/review')
    const isContinueNumber = /^\/continue\s+\d+$/.test(slashFilter)
    const isUltraPlanObjective = /^\/ultraplan\s+\S/.test(slashFilter)
    const exactRootCandidates = slashFilter.includes(' ')
      ? []
      : allSlashCommands.filter(c => {
          const cmd = String(c.cmd || '')
          const root = cmd.split(/\s+/, 1)[0]
          return root === slashFilter
        })
    const exactRootPrimary = exactRootCandidates.find(c => String(c.cmd || '') === slashFilter) || exactRootCandidates[0]
    const argumentRoot = slashFilter.includes(' ') ? slashFilter.split(/\s+/, 1)[0] : ''
    const argumentRootCandidates = argumentRoot
      ? allSlashCommands.filter(c => String(c.cmd || '').split(/\s+/, 1)[0] === argumentRoot)
      : []
    const argumentFallback = argumentRootCandidates.find(c => SLASH_ARG_SUFFIX_RE.test(String(c.cmd || '')))
      || argumentRootCandidates.find(c => String(c.cmd || '') === argumentRoot)
      || argumentRootCandidates[0]
    return allSlashCommands.filter(c => {
      const cmd = String(c.cmd || '')
      if (exactRootPrimary) return c === exactRootPrimary
      if (argumentFallback && c === argumentFallback) return true
      if (cmd === '/review help') return childAllowed('/review') && fuzzyMatch(cmd, slashFilter)
      if (cmd === '/review <request>') {
        if (isReviewNaturalLanguage) return true
        if (slashFilter === '/review' || fuzzyMatch('/review', rawFilter) || fuzzyMatch('/review', slashFilter)) return true
        if (slashFilter.startsWith('/review ')) return false
      }
      if (cmd === '/continue <number>') {
        if (slashFilter === '/continue ') return true
        if (isContinueNumber) return true
        if (slashFilter.startsWith('/continue ')) return false
      }
      if (cmd === '/ultraplan <goal>') {
        if (slashFilter === '/ultraplan ') return true
        if (isUltraPlanObjective) return true
        if (slashFilter === '/ultraplan' || fuzzyMatch('/ultraplan', rawFilter) || fuzzyMatch('/ultraplan', slashFilter)) return true
        if (slashFilter.startsWith('/ultraplan ')) return false
      }
      if (cmd === '/project' && slashFilter.startsWith('/project ')) return true
      if (inProjectScope && !cmd.startsWith('/project')) return false
      if (inContinueScope && cmd !== '/continue <编号>') return false
      if (inReviewScope && cmd !== '/review <自然语言请求>') return false
      if (inImproveScope && cmd !== '/improve') return false
      if (inUltraPlanScope && cmd !== '/ultraplan <goal>') return false
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
    if (!confirmDanger('chat-slash-commands-save', ct('保存斜杠命令配置？会写入 GA Admin 配置文件。', 'Save slash-command configuration? This writes the GA Admin configuration file.'))) return
    try {
      const safeCmds = (newCmds || [])
        .filter(c => !isProtectedSlashCommand(c?.cmd))
        .map(c => ({ cmd: String(c?.cmd || '').trim(), desc: String(c?.desc || '').trim(), content: String(c?.content || c?.prompt || '').trim() }))
        .filter(c => c.cmd)
      const c = await api('/api/config', { method:'PUT', dangerous: true, body: JSON.stringify({...cfg, slash_commands: safeCmds}) })
      if (c?.slash_commands) { setCfg(c) }
      setCmdEditIdx(-1)
    } catch(e) { setNotice(ct('保存命令失败: ', 'Failed to save command: ') + e.message); setCmdEditIdx(-1) }
  }
  const startEdit = (idx, cmd, desc, content = '') => {
    if (idx < 0 && idx !== -2) return
    setCmdEditIdx(idx); setCmdEditCmd(cmd); setCmdEditDesc(desc); setCmdEditContent(content)
  }
  const saveEdit = () => {
    const normalized = cmdEditCmd.trim()
    if (!normalized) return
    if (isProtectedSlashCommand(normalized)) {
      setNotice(ct('这是 GA Admin 内置命令，不能覆盖或修改', 'This built-in GA Admin command cannot be overridden or edited'))
      setCmdEditIdx(-1)
      return
    }
    const cmds = cfg?.slash_commands || []
    const nextItem = { cmd: normalized, desc: cmdEditDesc.trim() || '', content: cmdEditContent.trim() || cmdEditDesc.trim() || '' }
    if (!nextItem.content) {
      setNotice(ct('请填写这个命令要展开成的指令内容', 'Enter the instruction content this command should expand into'))
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
    if (idx < 0) { setNotice(ct('这是 GA Admin 内置命令，不能删除', 'This built-in GA Admin command cannot be deleted')); return }
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
    if (Object.prototype.hasOwnProperty.call(ev, 'raw_history')) {
      setRawHistory(Array.isArray(ev.raw_history) ? ev.raw_history : [])
    }
    if (Object.prototype.hasOwnProperty.call(ev, 'history_info')) {
      setHistoryInfo(Array.isArray(ev.history_info) ? ev.history_info : [])
    }
    if (Object.prototype.hasOwnProperty.call(ev, 'working')) {
      setWorkingState(ev.working && typeof ev.working === 'object' ? ev.working : null)
    }
    if (Object.prototype.hasOwnProperty.call(ev, 'plan')) setPlanState(ev.plan || null)
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
    if (ev.type === 'ctx_stats' && typeof ev.ctx_chars === 'number') {
      setMessages(xs => isActiveSession(sessionId) ? xs.map(m =>
        m.id === pendingId ? { ...m, ctx_chars: ev.ctx_chars, ctx_msgs: ev.ctx_msgs } : m
      ) : xs)
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
        if (!finalMsg.goal_state && m.goal_state) finalMsg.goal_state = m.goal_state
        if (!finalMsg.ctx_chars) finalMsg.ctx_chars = ev.ctx_chars || m.ctx_chars || 0
        if (!finalMsg.ctx_msgs) finalMsg.ctx_msgs = ev.ctx_msgs || m.ctx_msgs || 0
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
    if (ev.type === 'goal_event' && ev.state) {
      setMessages(xs => isActiveSession(sessionId) ? xs.map(m => {
        if (m.id !== pendingId) return m
        return { ...m, goal_state: { ...(m.goal_state || {}), ...ev.state } }
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

  const createStreamBatcher = (pendingId, sessionId = '') => createStreamDeltaBatcher({
    onFlush: chunk => setMessages(xs => isActiveSession(sessionId) ? xs.map(m => (
      m.id === pendingId ? { ...m, content: (m.content || '') + chunk } : m
    )) : xs),
    schedule: callback => window.requestAnimationFrame ? window.requestAnimationFrame(callback) : window.setTimeout(callback, 16),
    cancel: handle => window.cancelAnimationFrame ? window.cancelAnimationFrame(handle) : window.clearTimeout(handle),
    // Start in replay mode: backend emits {"type":"sync"} after the backlog,
    // so reattach-after-refresh renders prior output instantly, then animates.
    live: false,
  })

  const readStream = async (res, pendingId, clientUserID = '', sessionId = '') => {
    const reader = res.body.getReader(); const dec = new TextDecoder(); let buf = ''
    const batcher = createStreamBatcher(pendingId, sessionId)
    let commandPatch = null
    let eventCount = 0
    let terminal = false
    let terminalEvent = null
    const applyEvent = (ev) => {
      if (ev?.type === 'command_result') commandPatch = reduceCommandResult(ev)
      applyStreamEvent(ev, pendingId, clientUserID, sessionId)
    }
    const consumeEvent = (ev) => {
      if (ev.type === 'sync') {
        // Replay/live boundary: flush backlog instantly, animate what follows.
        // Not stored in run.Events server-side, so it must NOT bump eventCount
        // (the reconnect cursor would skip a real event otherwise).
        batcher.beginLive()
        return
      }
      if (ev.type === 'delta' && typeof ev.delta === 'string') {
        batcher.push(ev.delta)
      } else if (ev.type === 'done' || ev.type === 'error') {
        terminal = true
        terminalEvent = ev
      } else {
        applyEvent(ev)
      }
      eventCount += 1
    }
    try {
      while (true) {
        const { value, done } = await reader.read()
        if (done) break
        buf += dec.decode(value, { stream:true })
        const lines = buf.split('\n'); buf = lines.pop() || ''
        for (const line of lines) {
          if (!line.trim()) continue
          if (!isActiveSession(sessionId)) return { commandPatch, eventCount, terminal }
          consumeEvent(JSON.parse(line))
        }
      }
      buf += dec.decode()
      if (buf.trim() && isActiveSession(sessionId)) consumeEvent(JSON.parse(buf))
      await batcher.drain()
      if (terminalEvent && isActiveSession(sessionId)) applyEvent(terminalEvent)
    } catch (error) {
      batcher.flushNow()
      error.chatStreamOutcome = { commandPatch, eventCount, terminal }
      throw error
    }
    return { commandPatch, eventCount, terminal }
  }

  const waitForStreamRetry = (signal, delay = 250) => new Promise((resolve, reject) => {
    if (signal?.aborted) {
      const err = new Error('aborted'); err.name = 'AbortError'; reject(err); return
    }
    const done = () => { signal?.removeEventListener('abort', aborted); resolve() }
    const aborted = () => {
      clearTimeout(timer)
      const err = new Error('aborted'); err.name = 'AbortError'; reject(err)
    }
    const timer = setTimeout(done, delay)
    signal?.addEventListener('abort', aborted, { once:true })
  })

  const followChatStream = async (initialRes, pendingId, clientUserID, sessionId, signal) => {
    let res = initialRes
    let cursor = 0
    let replay = false
    let commandPatch = null
    while (isActiveSession(sessionId)) {
      let completed = false
      let eventCount = 0
      try {
        const outcome = await readStream(res, pendingId, clientUserID, sessionId)
        eventCount = outcome.eventCount
        cursor += eventCount
        if (outcome.commandPatch) commandPatch = outcome.commandPatch
        completed = true
        if (outcome.terminal) return commandPatch
      } catch (e) {
        const partial = e?.chatStreamOutcome
        if (partial) {
          eventCount = partial.eventCount
          cursor += eventCount
          if (partial.commandPatch) commandPatch = partial.commandPatch
          if (partial.terminal) return commandPatch
        }
        if (e?.name === 'AbortError' || signal?.aborted) throw e
      }
      if (!isActiveSession(sessionId)) return commandPatch

      let state = null
      while (!state && isActiveSession(sessionId)) {
        try {
          state = await chatApi(`/api/chat/state/${sessionId}`, { signal })
        } catch (e) {
          if (e?.name === 'AbortError' || signal?.aborted) throw e
          await waitForStreamRetry(signal)
        }
      }
      if (!state || !isActiveSession(sessionId)) return commandPatch
      if (shouldFinishStreamFollow({ running:state.running, replay, completed, eventCount })) return commandPatch
      if (state.running) await waitForStreamRetry(signal, 120)

      while (isActiveSession(sessionId)) {
        try {
          res = await chatFetch(`/api/chat/stream/${sessionId}?from=${cursor}`, { signal })
          if (res.status === 204) return commandPatch
          if (!res.ok) throw new Error(await res.text())
          replay = true
          break
        } catch (e) {
          if (e?.name === 'AbortError' || signal?.aborted) throw e
          await waitForStreamRetry(signal)
        }
      }
    }
    return commandPatch
  }

  const cancelRun = async (id = sid) => {
    if (!id) return
    try {
      streamAbortRef.current?.abort?.()
      await chatApi(`/api/chat/cancel/${id}`, { method:'POST', body:'{}' })
      setMessages(xs => xs.map(m => (m.role === 'assistant' && !m.content) ? { ...m, content:ct('已中止。', 'Stopped.'), error:true } : m))
      setSessions(xs => xs.map(s => s.id === id ? { ...s, running:false } : s))
      setNotice(ct('已中止当前执行', 'Current run stopped'))
    } catch (e) { setErr(e.message || String(e)) }
    finally { setBusy(false); setStreamingSid(''); if (id) loadSessions(id).catch(()=>{}) }
  }

  const attachRunningStream = async (id) => {
    if (!id) return
    streamAbortRef.current?.abort?.()
    const ctrl = new AbortController()
    streamAbortRef.current = ctrl
    let streamCompleted = false
    let streamError = null
    let pendingId = `resume-${Date.now()}`
    let sessionForNotification = null
    // Resolve the placeholder id up-front: `followChatStream` below reads `pendingId` right
    // after `await fetch`, which may win the race against the state updater.
    const knownId = pickResumePlaceholderId(messagesRef.current)
    if (knownId) pendingId = knownId
    setBusy(true); setStreamingSid(id); setAutoFollow(true); setShowFollow(false)
    setMessages(xs => {
      const existingId = pickResumePlaceholderId(xs)
      if (existingId) {
        pendingId = existingId
        return xs
      }
      return [...xs, { id:pendingId, role:'assistant', content:'', created_at:Math.floor(Date.now()/1000), run_started_at_ms:Date.now() }]
    })
    try {
      const res = await chatFetch(`/api/chat/stream/${id}`, { signal: ctrl.signal })
      if (res.status === 204) return
      if (!res.ok) throw new Error(await res.text())
      await followChatStream(res, pendingId, '', id, ctrl.signal)
      streamCompleted = true
      if (isActiveSession(id)) {
        const list = await loadSessions(id)
        const currentSession = list.find(session => session.id === id)
        sessionForNotification = currentSession
        if (shouldPollGeneratedTitle(currentSession)) {
          void pollGeneratedChatTitle({ sessionId:id, loadSessions, isActive:isActiveSession }).catch(()=>{})
        }
      }
    } catch (e) {
      streamError = e
      if (e.name !== 'AbortError' && isActiveSession(id)) setErr(e.message || String(e))
    } finally {
      if (streamAbortRef.current === ctrl) {
        streamAbortRef.current = null
        if (isActiveSession(id)) {
          if (streamCompleted) publishNotification({ category: 'chat', level: 'success', ...buildChatNotification({ session: sessionForNotification, sessionId: id, prompt: latestUserPrompt(messagesRef.current), status: 'completed', lang: chatLanguage() }), route: 'chat', dedupeKey: `chat:${id}:${pendingId}:done` })
          else if (streamError?.name !== 'AbortError' && streamError) publishNotification({ category: 'chat', level: 'error', ...buildChatNotification({ session: sessionForNotification, sessionId: id, prompt: latestUserPrompt(messagesRef.current), status: 'failed', error: streamError.message || streamError, lang: chatLanguage() }), route: 'chat', dedupeKey: `chat:${id}:${pendingId}:error` })
          setBusy(false); setStreamingSid('')
        }
      }
    }
  }

  const loadChatState = async (id = '', openToken = openSeqRef.current) => {
    const st = await chatApi(id ? `/api/chat/state/${id}` : '/api/chat/state')
    if (openToken !== openSeqRef.current || !isActiveSession(id)) return null
    const nextLlms = st.llms || []
    const defaultNo = firstRuntimeModelNo(nextLlms)
    const nextNo = st.settings?.llm_no ?? st.llm_no ?? defaultNo
    const resolvedNo = nextLlms.some(model => Number(model.index) === Number(nextNo)) ? Number(nextNo) : defaultNo
    const selectedRuntimeModel = nextLlms.find(model => model.index === resolvedNo)
    const storedReasoningEffort = String(st.settings?.reasoning_effort || '').trim()
    const nextReasoningEffort = storedReasoningEffort
      ? normalizeReasoningEffort(storedReasoningEffort)
      : modelReasoningEffort(selectedRuntimeModel)
    const nextExtraSysPrompts = Array.isArray(st.extra_sys_prompts) ? st.extra_sys_prompts.filter(x => typeof x === 'string' && x.trim()).map(x => x.trim()) : []
    const nextExtraSysPromptPresetID = String(st.extra_sys_prompt_preset_id || '').trim()
    setLlms(nextLlms)
    setLlmNo(resolvedNo)
    setReasoningEffort(nextReasoningEffort)
    setExtraSysPrompts(nextExtraSysPrompts)
    setExtraSysPromptPresetID(nextExtraSysPromptPresetID)
    if (id && st.running) {
      attachRunningStream(id)
    } else if (id && streamingSid && streamingSid !== id) {
      streamAbortRef.current?.abort?.()
      streamAbortRef.current = null
      setBusy(false)
      setStreamingSid('')
    }
    return { extraSysPromptPresetID: nextExtraSysPromptPresetID, extraSysPrompts: nextExtraSysPrompts }
  }

  const openSession = async (id, refreshList = true) => {
    setWorldlineRestorePicker(null)
    const openToken = ++openSeqRef.current
    activeSidRef.current = id
    streamAbortRef.current?.abort?.()
    streamAbortRef.current = null
    scrollModeRef.current = 'auto'
    setSid(id)
    setSessionPrompt(loadChatSessionDraft(id), id)
    setBusy(false)
    setStreamingSid('')
    setAutoFollow(true)
    setShowFollow(false)
    const d = await chatApi(`/api/chat/session/${id}`)
    if (openToken !== openSeqRef.current || activeSidRef.current !== id) return
    activeSidRef.current = d.id
    scrollModeRef.current = 'auto'
    setSid(d.id)
    setMessages(d.messages || [])
    setRawHistory(Array.isArray(d.raw_history) ? d.raw_history : [])
    setHistoryInfo(Array.isArray(d.history_info) ? d.history_info : [])
    setWorkingState(d.working || null)
    setPlanState(d.plan || null)
    setLlmNo(d.settings?.llm_no ?? firstRuntimeModelNo(llms))
    setErr('')
    setNotice('')
    setMenuOpen('')
    setMenuPos(null)
    setSessions(xs => xs.map(x => x.id === d.id ? { ...x, title: d.title, workspace: d.workspace || '', project_mode: d.project_mode || '', count: d.messages?.length || x.count, updated_at: d.updated_at || x.updated_at } : x))
    await loadChatState(d.id, openToken)
    if (openToken === openSeqRef.current && worldlineOpen) loadWorldline(d.id, { force: true }).catch(() => {})
  }

  const loadWorldline = async (id = activeSidRef.current || sid, { force = false } = {}) => {
    if (!id || (!force && !worldlineOpen && worldlineState?.sessionID !== id)) return
    const token = ++worldlineSeqRef.current
    setWorldlineLoading(true)
    try {
      const d = await api(`/api/chat/worldline/${id}?activate=true`)
      if (token === worldlineSeqRef.current && activeSidRef.current === id) setWorldlineState({ sessionID:id, ...d })
    } catch (error) {
      if (token === worldlineSeqRef.current) setWorldlineState({ sessionID:id, available:false, degraded_reason:error?.message || String(error) })
    } finally {
      if (token === worldlineSeqRef.current) setWorldlineLoading(false)
    }
  }

  const toggleWorldline = () => {
    const next = !worldlineOpen
    setWorldlineOpen(next)
    if (next) loadWorldline(activeSidRef.current || sid, { force:true }).catch(() => {})
  }

  const switchWorldline = async (nodeId) => {
    const id = activeSidRef.current || sid
    if (!id || !nodeId) return
    if (busy && streamingSid === id) { setNotice('对话运行中，完成后再切换世界线'); return }
    setWorldlineSwitchingId(nodeId); setErr(''); setNotice('')
    try {
      const d = await api(`/api/chat/worldline/${id}/switch`, { method:'POST', body:JSON.stringify({ node_id:nodeId }) })
      if (activeSidRef.current !== id) return
      await openSession(id, false)
      if (d?.worldline && activeSidRef.current === id) setWorldlineState({ sessionID:id, ...d.worldline })
      setNotice('已切换到所选世界线分支')
      loadSessions(id).catch(() => {})
    } catch (error) {
      if (activeSidRef.current === id) setErr(error?.message || String(error))
    } finally {
      setWorldlineSwitchingId('')
    }
  }

  const worldlineForView = worldlineState?.sessionID === sid ? worldlineState : null

  const selectSidebarSession = (id) => {
    if (!id) return
    openSession(id).catch(error => setErr(error?.message || String(error)))
    if (isNarrowChatViewport()) setCollapsed(true)
  }

  const openSessionSearch = () => {
    setSidebarSearch('')
    setSessionSearchError('')
    setSessionSearchOpen(true)
  }

  const closeSessionSearch = () => {
    setSessionSearchOpen(false)
    setSessionSearchQuery('')
    setSessionSearchScope('all')
    setSessionSearchResults([])
    setSessionSearchError('')
    window.requestAnimationFrame?.(() => sessionSearchTriggerRef.current?.focus())
  }

  const submitSessionSearch = () => {
    const query = sessionSearchQuery.trim()
    if (!query) return
    setSessionSearchHistory(saveSessionSearchHistory({ query, scope: sessionSearchScope }))
  }

  const selectSessionSearchHistory = entry => {
    setSessionSearchQuery(entry?.query || '')
    setSessionSearchScope(entry?.scope || 'all')
    setSessionSearchError('')
  }

  const selectSessionSearchResult = id => {
    closeSessionSearch()
    selectSidebarSession(id)
  }

  useEffect(() => {
    const query = sessionSearchQuery.trim()
    const requestID = ++sessionSearchRequestRef.current
    if (!sessionSearchOpen || !query) {
      setSessionSearchResults([])
      setSessionSearchLoading(false)
      return undefined
    }
    setSessionSearchLoading(true)
    setSessionSearchError('')
    const timer = window.setTimeout(() => {
      const params = new URLSearchParams({ q: query, scope: sessionSearchScope, limit: '60' })
      api(`/api/chat/search?${params.toString()}`)
        .then(data => {
          if (requestID !== sessionSearchRequestRef.current) return
          setSessionSearchResults(Array.isArray(data?.results) ? data.results : [])
        })
        .catch(error => {
          if (requestID !== sessionSearchRequestRef.current) return
          setSessionSearchResults([])
          setSessionSearchError(error?.message || ct('搜索失败，请稍后重试。', 'Search failed. Try again.'))
        })
        .finally(() => {
          if (requestID === sessionSearchRequestRef.current) setSessionSearchLoading(false)
        })
    }, 180)
    return () => window.clearTimeout(timer)
  }, [sessionSearchOpen, sessionSearchQuery, sessionSearchScope])

  const loadSessions = async (prefer = sid, options = {}) => {
    const { open = false } = options
    const d = await chatApi('/api/chat/sessions')
    const list = d.sessions || []
    setSessions(list)
    setProjects(Array.isArray(d.projects) ? d.projects : [])
    if (open) {
      const next = prefer || list[0]?.id || ''
      if (next) await openSession(next, false)
      else await loadChatState('', openSeqRef.current)
    } else if (!prefer && !sid) {
      await loadChatState('', openSeqRef.current)
    }
    return list
  }

  const createSession = async (projectMode = '') => {
    const selectedProject = typeof projectMode === 'string' ? projectMode.trim() : ''
    if (isNarrowChatViewport()) setCollapsed(true)
    setWorldlineRestorePicker(null)
    setSessionManagerOpen(false)
    setSelectedSessionIds([])
    const openToken = ++openSeqRef.current
    activeRunRef.current = false
    streamAbortRef.current?.abort?.()
    streamAbortRef.current = null
    const d = await api('/api/chat/session/new', { method:'POST', body:JSON.stringify(projectMode ? { project_mode:projectMode } : {}) })
    if (openToken !== openSeqRef.current) return
    activeSidRef.current = d.id
    scrollModeRef.current = 'auto'
    clearSessionDrafts(d.id)
    setSid(d.id); setMessages([]); setRawHistory([]); setHistoryInfo([]); setWorkingState(null); setPlanState(null); setContextOpen(false); setSessionPrompt('', d.id); setErr(''); setNotice(ct('已创建新对话', 'New chat created')); setBusy(false); setStreamingSid(''); setAutoFollow(false); setShowFollow(false); setLlmNo(d.settings?.llm_no ?? firstRuntimeModelNo(llms))
    await loadChatState(d.id, openToken)
    if (projectMode) await loadSessions(d.id)
  }

  const newSession = async () => {
    await createSession()
  }

  const newProjectSession = async (projectMode) => {
    await createSession(projectMode)
  }

  const deleteSession = async (id) => {
    if (!id || !confirmDanger('chat-session-delete', ct('删除此会话？此操作不可恢复。', 'Delete this session? This cannot be undone.'))) return
    await chatApi(`/api/chat/session/${id}`, { method:'DELETE' })
    clearSessionDrafts(id)
    setSessions(xs => xs.filter(x => x.id !== id))
    setMenuOpen('')
    setMenuPos(null)
    if (id === sid) {
      ++openSeqRef.current
      activeSidRef.current = ''
      streamAbortRef.current?.abort?.()
      streamAbortRef.current = null
      scrollModeRef.current = 'auto'
      setSid(''); setMessages([]); setBusy(false); setStreamingSid(''); setAutoFollow(true); setShowFollow(false); setNotice(ct('会话已删除', 'Session deleted'))
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
    if (!ids.length || !confirmDanger('chat-session-batch-delete', ct(`永久删除已选的 ${ids.length} 个会话？此操作不可恢复。`, `Permanently delete ${ids.length} selected sessions? This cannot be undone.`))) return

    setBatchDeleting(true)
    setErr('')
    setNotice('')
    try {
      const result = await deleteChatSessions(ids, id => chatApi(`/api/chat/session/${id}`, { method:'DELETE' }))
      clearSessionDrafts(result.deletedIds)
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
        setPlanState(null)
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
        setErr(ct(`${result.failedIds.length} 个会话删除失败${detail ? `：${detail}` : ''}${refreshError ? `；刷新失败：${refreshError}` : ''}`, `${result.failedIds.length} sessions could not be deleted${detail ? `: ${detail}` : ''}${refreshError ? `; refresh failed: ${refreshError}` : ''}`))
      } else {
        setSelectedSessionIds([])
        setSessionManagerOpen(false)
        if (refreshError) setErr(ct(`已删除 ${result.deletedIds.length} 个会话，但刷新列表失败：${refreshError}`, `${result.deletedIds.length} sessions deleted, but list refresh failed: ${refreshError}`))
        else setNotice(ct(`已删除 ${result.deletedIds.length} 个会话`, `${result.deletedIds.length} sessions deleted`))
      }
    } finally {
      setBatchDeleting(false)
    }
  }

  const startRename = (s) => { setEditing(s.id); setDraftTitle(shortTitle(s)); setMenuOpen(''); setMenuPos(null) }
  const saveRename = async (id) => {
    const title = draftTitle.trim()
    if (!title) return
    const d = await chatApi(`/api/chat/session/${id}`, { method:'PATCH', body: JSON.stringify({ title }) })
    setSessions(xs => xs.map(x => x.id === id ? { ...x, title:d.title, updated_at:d.updated_at } : x))
    setEditing(''); setDraftTitle(''); setNotice(ct('会话已更名', 'Session renamed'))
  }

  const saveModel = async (next) => {
    if (next === llmNo || modelSwitching) return
    const previous = llmNo
    const previousReasoningEffort = reasoningEffort
    const nextModel = llms.find(model => model.index === next)
    const nextReasoningEffort = modelReasoningEffort(nextModel)
    const nextReasoningSetting = modelReasoningEffortSetting(nextModel)
    setLlmNo(next)
    setReasoningEffort(nextReasoningEffort)
    if (!sid) return
    setModelSwitching(true)
    setErr('')
    try {
      await api(`/api/chat/settings/${sid}`, { method:'POST', body: JSON.stringify({ llm_no: next, reasoning_effort: nextReasoningSetting }) })
      setNotice(`模型已切换到 #${next}，下一条消息将由该模型处理`)
    } catch (e) {
      setLlmNo(previous)
      setReasoningEffort(previousReasoningEffort)
      setErr(`模型切换失败：${e.message || String(e)}`)
    } finally {
      setModelSwitching(false)
    }
  }

  const saveReasoningEffort = async (value) => {
    const next = normalizeReasoningEffort(value)
    const prev = reasoningEffort
    setReasoningEffort(next)
    if (!sid) return
    try {
      await chatApi(`/api/chat/settings/${sid}`, { method:'POST', body: JSON.stringify({ llm_no: llmNo, reasoning_effort: next }) })
      setNotice(next === 'off' ? ct('推理强度已设为默认', 'Reasoning effort reset to default') : ct(`推理强度已设为 ${next}`, `Reasoning effort set to ${next}`))
    } catch (e) {
      setReasoningEffort(prev)
      setErr(e.message || String(e))
    }
  }

  const loadPromptPresets = async () => {
    const d = await api('/api/extra-system-prompt-presets')
    const next = normalizePromptPresets(d?.presets)
    setPromptPresets(next)
    return next
  }
  const selectExtraPromptPreset = (value) => {
    extraPromptSelectionSeqRef.current += 1
    setExtraPromptSelection(value)
  }
  const openExtraPromptEditor = () => {
    const targetSid = activeSidRef.current
    const targetOpenToken = openSeqRef.current
    const initialSelectionSeq = extraPromptSelectionSeqRef.current
    setPromptPresetManagerOpen(false)
    setExtraPromptTargetSid(targetSid)
    setExtraPromptSelection(extraSysPromptPresetID)
    setExtraPromptOpen(true)

    Promise.all([
      loadChatState(targetSid, targetOpenToken),
      loadPromptPresets(),
    ]).then(([freshState]) => {
      if (!freshState || targetOpenToken !== openSeqRef.current || activeSidRef.current !== targetSid) return
      if (extraPromptSelectionSeqRef.current === initialSelectionSeq) {
        setExtraPromptSelection(freshState.extraSysPromptPresetID)
      }
    }).catch(e => {
      if (targetOpenToken === openSeqRef.current && activeSidRef.current === targetSid) {
        setErr(e.message || String(e))
      }
    })
  }
  const openPromptPresetManager = () => {
    setExtraPromptDraft(promptPresets.map(item => ({ ...item })))
    setPromptPresetManagerOpen(true)
  }
  const updateExtraPromptDraft = (id, field, value) => {
    setExtraPromptDraft(items => items.map(item => item.id === id ? { ...item, [field]: value } : item))
  }
  const saveExtraPromptSelection = async () => {
    const targetSid = extraPromptTargetSid
    if (!targetSid) {
      setErr(ct('请先创建或打开会话', 'Create or open a session first'))
      return
    }
    if (activeSidRef.current !== targetSid) {
      setExtraPromptOpen(false)
      setErr(ct('会话已切换，请重新选择系统提示预设', 'The session changed; choose the system-prompt preset again'))
      return
    }
    const targetOpenToken = openSeqRef.current
    setExtraPromptSaving(true)
    try {
      const d = await chatApi(`/api/chat/settings/${targetSid}`, {
        method:'POST',
        body: JSON.stringify({ llm_no: llmNo, reasoning_effort: reasoningEffort, ...promptPresetPatch(extraPromptSelection) }),
      })
      if (targetOpenToken !== openSeqRef.current || activeSidRef.current !== targetSid) {
        setExtraPromptOpen(false)
        return
      }
      const savedID = String(d.extra_sys_prompt_preset_id || '').trim()
      const savedPrompts = Array.isArray(d.extra_sys_prompts) ? d.extra_sys_prompts.filter(x => typeof x === 'string' && x.trim()).map(x => x.trim()) : []
      setExtraSysPromptPresetID(savedID)
      setExtraSysPrompts(savedPrompts)
      setExtraPromptSelection(savedID)
      setExtraPromptOpen(false)
      setNotice(savedID ? ct(`已为当前会话启用「${selectedPromptPresetView({ presets: promptPresets, selectedID: savedID, snapshot: savedPrompts }).name}」`, `Enabled “${selectedPromptPresetView({ presets: promptPresets, selectedID: savedID, snapshot: savedPrompts }).name}” for this session`) : ct('当前会话已停用额外系统提示', 'Extra system prompt disabled for this session'))
    } catch (e) {
      setErr(e.message || String(e))
    } finally {
      setExtraPromptSaving(false)
    }
  }
  const savePromptPresets = async () => {
    const next = normalizePromptPresets(extraPromptDraft)
    if (next.some(item => !item.name || !item.content)) {
      setErr(ct('每个预设都需要名称和提示内容', 'Every preset needs a name and prompt content'))
      return
    }
    if (!confirmDanger('chat-extra-system-prompt-presets-save', ct(`保存 ${next.length} 个全局系统提示预设？这会写入 GA Admin 配置文件。`, `Save ${next.length} global system-prompt presets? This writes the GA Admin configuration file.`))) return
    setExtraPromptSaving(true)
    try {
      const d = await api('/api/extra-system-prompt-presets', {
        dangerous:true,
        method:'PUT',
        body: JSON.stringify({ presets: next }),
      })
      const saved = normalizePromptPresets(d?.presets)
      setPromptPresets(saved)
      setExtraPromptDraft(saved.map(item => ({ ...item })))
      setPromptPresetManagerOpen(false)
      setNotice(ct(`已保存 ${saved.length} 个系统提示预设`, `${saved.length} system-prompt presets saved`))
    } catch (e) {
      setErr(e.message || String(e))
    } finally {
      setExtraPromptSaving(false)
    }
  }


  const addAttachmentFiles = async (fileList) => {
    const files = Array.from(fileList || []).filter(Boolean)
    if (!files.length) return
    if (attachments.length + files.length > MAX_CHAT_UPLOAD_FILES) {
      setErr(ct(`附件最多上传 ${MAX_CHAT_UPLOAD_FILES} 个`, `You can upload up to ${MAX_CHAT_UPLOAD_FILES} attachments`))
      return
    }
    const tooLarge = files.find((file) => (Number(file.size) || 0) > MAX_CHAT_UPLOAD_BYTES_PER_FILE)
    if (tooLarge) {
      setErr(ct(`附件过大：${tooLarge.name || 'attachment'}，单个限制 20MB`, `Attachment too large: ${tooLarge.name || 'attachment'}; limit 20 MB per file`))
      return
    }
    const totalBytes = attachments.reduce((sum, file) => sum + (Number(file.size) || 0), 0)
      + files.reduce((sum, file) => sum + (Number(file.size) || 0), 0)
    if (totalBytes > MAX_CHAT_UPLOAD_BYTES_TOTAL) {
      setErr(ct('附件总大小限制 40MB', 'Total attachment size is limited to 40 MB'))
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
      reader.onerror = () => reject(reader.error || new Error(ct('读取附件失败', 'Failed to read attachment')))
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
    setNotice(ct(`已加入队列（${next.length} 条）。点击“引导”可中止当前回复并立即发送。`, `Added to queue (${next.length}). Use Guide to stop the current response and send immediately.`))
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
    setNotice(ct('正在编辑队列消息', 'Editing queued message'))
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
    if (!text && !(item.files || []).length) { setErr(ct('队列消息不能为空', 'Queued message cannot be empty')); return }
    syncQueue(queuedRef.current.map(x => x.id === id ? { ...x, text } : x))
    setQueueEditingId('')
    setQueueDraft('')
    setErr('')
    setNotice(ct('队列消息已更新', 'Queued message updated'))
  }
  const guideQueuedItem = (id) => {
    if (guidingQueueRef.current) return
    const item = queuedRef.current.find(x => x.id === id)
    if (!item) return
    guidingQueueRef.current = id
    setGuidingQueueId(id)
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
    setSessionPrompt(value)
    setNotice(ct('已填入快捷回复，确认后可发送', 'Quick reply inserted; review and send when ready'))
    const focusPrompt = () => {
      const el = promptRef.current
      if (!el) return
      el.focus()
      const len = value.length
      el.setSelectionRange?.(len, len)
    }
    requestAnimationFrame(focusPrompt)
    setTimeout(focusPrompt, 0)
  }, [setSessionPrompt])

  const editAndResend = async (messageId, text) => {
    const item = buildEditResendItem({
      sessionId: activeSidRef.current,
      messageId,
      text,
      busy,
      streamingSid,
    })
    await runSend(item)
  }

  const sendBTW = async (text, sessionId = activeSidRef.current || sid, retryId = '') => {
    if (!sessionId) {
      setNotice(ct('请先打开一个对话再使用 /btw', 'Open a conversation before using /btw'))
      return
    }
    const prompt = String(text || '').trim()
    const question = prompt.replace(/^\/btw(?:\s+|$)/i, '').trim()
    if (!question) {
      setNotice(ct('请在 /btw 后输入问题', 'Enter a question after /btw'))
      return
    }
    const placeholderId = retryId || `btw-${Date.now()}-${Math.random().toString(36).slice(2, 7)}`
    const placeholder = {
      id: placeholderId,
      role: 'assistant',
      kind: 'btw',
      side_question: question,
      btw_status: 'pending',
      content: '',
      created_at: Math.floor(Date.now() / 1000),
    }
    setErr(''); setNotice('')
    if (isActiveSession(sessionId)) setMessages(xs => retryId
      ? xs.map(m => m.id === retryId ? placeholder : m)
      : [...xs, placeholder])
    try {
      const data = await chatApi(`/api/chat/btw/${sessionId}`, { method:'POST', body:JSON.stringify({ prompt:`/btw ${question}` }) })
      if (!isActiveSession(sessionId)) return
      if (data?.message) setMessages(xs => xs.map(m => m.id === placeholderId ? { ...data.message, btw_status:'done' } : m))
      await loadSessions(sessionId)
    } catch (e) {
      if (!isActiveSession(sessionId)) return
      const detail = e?.message || String(e)
      setMessages(xs => xs.map(m => m.id === placeholderId
        ? { ...m, btw_status:'error', content:detail }
        : m))
    }
  }

  const runSend = async (item = {}) => {
    const guidedQueueId = guidingQueueRef.current
    if (guidedQueueId) {
      syncQueue(queuedRef.current.filter(x => x.id !== guidedQueueId))
      guidingQueueRef.current = ''
      setGuidingQueueId('')
    }
    const text = String(item.text || '').trim()
    const files = (item.files || []).map(({ name, type, dataURL }) => ({ name, type, dataURL }))
    if (!text && !files.length) return
    // Keep this as the first synchronous side effect of a send click so iOS
    // can authorize the later completion sound after the stream finishes.
    primeChatCompletionTone()
    const runToken = ++runSeqRef.current
    const openToken = openSeqRef.current
    const ctrl = new AbortController()
    activeRunRef.current = true
    streamAbortRef.current?.abort?.()
    streamAbortRef.current = ctrl
    const targetSessionID = item.sessionId || sid
    setBusy(true); setStreamingSid(targetSessionID || 'new'); setErr(''); setNotice('')
    let id = targetSessionID
    let commandPatch = null
    let optimistic = null
    let pending = null
    let notificationPrompt = text
    let streamCompleted = false
    let runError = null
    let sessionForNotification = null
    try {
      if (!id) {
        const d = await chatApi('/api/chat/session/new', { method:'POST', body:'{}' })
        if (runToken !== runSeqRef.current || openToken !== openSeqRef.current) return
        id = d.id
        clearSessionDrafts(id)
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
      const fileNote = files.length ? `\n\n[附件]\n${files.map((file) => `- ${uploadFileName(file)}`).join('\n')}` : ''
      const attachmentPrompt = text || ct('请处理这些附件', 'Please process these attachments')
      notificationPrompt = attachmentPrompt
      optimistic = { id:clientUserID, role:'user', content:attachmentPrompt + fileNote, files, created_at:Math.floor(Date.now()/1000) }
      const selectedLLMNo = item.llmNo ?? llmNo
      pending = { id:`a-${Date.now()}`, role:'assistant', content:'', llm_no:selectedLLMNo, created_at:Math.floor(Date.now()/1000), run_started_at_ms:Date.now() }
      const sourceMessageID = String(item.sourceUserMessageId || '').trim()
      setRawHistory([]); setHistoryInfo([]); setWorkingState(null); setPlanState(null)
      if (!isActiveSession(id)) return
      activeSidRef.current = id
      if (!sourceMessageID) setMessages(xs => isActiveSession(id) ? [...xs, optimistic, pending] : xs)
      const payload = buildChatRunPayload({
        prompt: attachmentPrompt,
        files,
        settings: { llm_no:selectedLLMNo, reasoning_effort:item.reasoningEffort || reasoningEffort },
        clientUserID,
        sourceUserMessageId: sourceMessageID,
      })
      const res = await chatFetch(`/api/chat/${id}`, { method:'POST', headers:{'Content-Type':'application/json'}, signal: ctrl.signal, body: JSON.stringify(payload) })
      if (!res.ok) throw new Error(await res.text())
      if (sourceMessageID) setMessages(xs => {
        if (!isActiveSession(id)) return xs
        const cutIdx = xs.findIndex(message => String(message.id) === sourceMessageID)
        if (cutIdx < 0) return xs
        return [...xs.slice(0, cutIdx), optimistic, pending]
      })
      commandPatch = await followChatStream(res, pending.id, clientUserID, id, ctrl.signal)
      streamCompleted = true
    } catch (e) {
      runError = e
      if (runToken === runSeqRef.current && openToken === openSeqRef.current && e?.name !== 'AbortError' && isActiveSession(id)) setErr(e.message || String(e))
      if (item.propagateError) throw e
    } finally {
      if (runToken !== runSeqRef.current) return
      if (openToken !== openSeqRef.current || !isActiveSession(id)) {
        activeRunRef.current = false
        return
      }
      if (id) {
        const refreshedSessions = await loadSessions(id).catch(()=>[])
        await openSession(id, false).catch(()=>{})
        const refreshedSession = refreshedSessions.find(session => session.id === id)
        sessionForNotification = refreshedSession
        if (shouldPollGeneratedTitle(refreshedSession)) {
          void pollGeneratedChatTitle({ sessionId:id, loadSessions, isActive:isActiveSession }).catch(()=>{})
        }
        if (commandPatch?.commandResult && optimistic && pending && isActiveSession(id)) {
          const showWorldlinePicker = isWorldlinePickerResult(commandPatch.commandResult)
          const resultMessage = {
            ...pending,
            content: commandResultSummary(commandPatch.commandResult),
            commandResult: commandPatch.commandResult,
            run_started_at_ms: undefined,
          }
          setMessages(xs => {
            if (!isActiveSession(id)) return xs
            const baseMessages = showWorldlinePicker ? xs.filter(m => m.id !== pending.id) : xs
            const hasUser = baseMessages.some(m => m.id === optimistic.id)
            return [...baseMessages, ...(hasUser ? [] : [optimistic]), ...(showWorldlinePicker ? [] : [resultMessage])]
          })
          if (Object.prototype.hasOwnProperty.call(commandPatch, 'prefill')) setSessionPrompt(commandPatch.prefill, id)
          if (showWorldlinePicker) {
            setWorldlineRestorePicker({ nodes:commandPatch.commandResult.tree.nodes, sessionID:id })
          }
          if (commandPatch.download) {
            const blob = new Blob([commandPatch.download.content], { type:commandPatch.download.mime })
            const url = URL.createObjectURL(blob)
            const link = document.createElement('a')
            link.href = url; link.download = commandPatch.download.filename
            document.body.appendChild(link); link.click(); link.remove()
            URL.revokeObjectURL(url)
          }
        }
      }
      if (id && isActiveSession(id)) loadWorldline(id).catch(() => {})
      const next = popQueued()
      if (next) {
        setNotice(ct(`继续发送队列消息（剩余 ${Math.max(queuedRef.current.length, 0)} 条）`, `Continuing queued messages (${Math.max(queuedRef.current.length, 0)} remaining)`))
        setTimeout(() => runSend(next), 0)
      } else {
        if (streamCompleted) publishNotification({ category: 'chat', level: 'success', ...buildChatNotification({ session: sessionForNotification, sessionId: id, prompt: notificationPrompt || latestUserPrompt(messagesRef.current), status: 'completed', lang: chatLanguage() }), route: 'chat', dedupeKey: `chat:${id}:${pending?.id || runToken}:done` })
        else if (runError?.name !== 'AbortError' && runError) publishNotification({ category: 'chat', level: 'error', ...buildChatNotification({ session: sessionForNotification, sessionId: id, prompt: notificationPrompt || latestUserPrompt(messagesRef.current), status: 'failed', error: runError.message || runError, lang: chatLanguage() }), route: 'chat', dedupeKey: `chat:${id || 'new'}:${pending?.id || runToken}:error` })
        activeRunRef.current = false
        setBusy(false)
        setStreamingSid('')
      }
    }
  }

  const retryFailedTurn = async (sourceMessage) => {
    if (!sourceMessage || busy) return
    if (modelSwitching) {
      setNotice('正在切换模型，请稍候重试')
      return
    }
    const text = stripUserAttachmentBlock(sourceMessage.content || '').trim()
    const sourceFiles = Array.isArray(sourceMessage.files) ? sourceMessage.files : []
    const reusableFiles = sourceFiles.filter(file => String(file?.dataURL || '').startsWith('data:'))
    if (!text && reusableFiles.length === 0) {
      setErr(sourceFiles.length ? '原消息仅包含历史附件，请重新选择附件后发送' : '找不到可重新发送的原消息')
      return
    }
    if (sourceFiles.length > reusableFiles.length) {
      setNotice('正在重新发送文字内容；历史附件无法自动复用，如仍需要请重新选择附件。')
    }
    await runSend({ text, files:reusableFiles, llmNo, reasoningEffort, sessionId:activeSidRef.current || sid })
  }

  const selectWorldlineRestoreNode = useCallback((nodeID, mode, target) => {
    const command = worldlineRestoreCommand(nodeID, mode, target)
    if (!command) return
    setSessionPrompt(command)
    setWorldlineRestorePicker(null)
    setNotice(ct('已填入恢复命令，确认后发送', 'Restore command inserted; review before sending'))
    window.setTimeout(() => {
      const input = promptRef.current
      if (!input) return
      input.focus()
      input.setSelectionRange?.(command.length, command.length)
    }, 0)
  }, [setSessionPrompt])

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
      if (busy || activeRunRef.current) {
        setNotice(ct('当前正在执行，完成后可使用 /new 创建新对话', 'A run is in progress. Use /new after it completes.'))
        return
      }
      setSessionPrompt('')
      await newSession()
      return
    }
    if (!text && !files.length) return
    if (isBTWCommand(text) && !files.length && !(activeSidRef.current || sid)) {
      await sendBTW(text)
      return
    }
    const item = { text, files, llmNo, reasoningEffort }
    setSessionPrompt(''); setAttachments([])
    setCmdDrawer({ open:false, filter:'', selectedIdx:0 })
    setCmdEditIdx(-1)
    if (isBTWCommand(text) && !files.length) {
      await sendBTW(text)
      return
    }
    if (busy || activeRunRef.current) {
      enqueueMessage(item)
      return
    }
    await runSend(item)
  }

  const applySlashCommand = (cmd, currentValue = prompt) => {
    if (!cmd) return
    const next = slashCommandInsertText(cmd, currentValue)
    setSessionPrompt(next)
    setCmdDrawer(slashCommandNextDrawer(cmd, next))
    setCmdEditIdx(-1)
    setTimeout(() => promptRef.current?.focus(), 0)
  }

  const handlePromptChange = (e) => {
    const v = e.target.value
    setSessionPrompt(v)
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
        const selectingNaturalReview = cmd?.cmd === '/review <request>' && /^\s*\/review\s+\S/.test(currentValue)
        const selectingBareContinue = e.key === 'Enter' && /^\s*\/continue\s*$/.test(currentValue)
        const selectingBareEffort = e.key === 'Enter' && /^\s*\/effort\s*$/.test(currentValue)
        const selectingBareImprove = e.key === 'Enter' && /^\s*\/improve\s*$/.test(currentValue)
        const selectingContinueNumber = cmd?.cmd === '/continue <编号>' && /^\s*\/continue\s+\d+\s*$/.test(currentValue)
        const selectingUltraPlanObjective = cmd?.cmd === '/ultraplan <目标>' && /^\s*\/ultraplan\s+\S/.test(currentValue)
        // 通用参数式命令（如 /goal [goal]）：输入框已是「根命令 + 自由文本」时，Enter 直接发送当前值，
        // 不再走 applySlashCommand（否则 insert 模板会清空用户后面的内容）。
        const selectedCmdText = String(cmd?.cmd || '')
        const selectedCmdRoot = selectedCmdText.split(/\s+/, 1)[0]
        const selectingArgumentFreeText = !!cmd && isArgumentStyleSlashCmd(selectedCmdText)
          && new RegExp(`^\\s*${selectedCmdRoot.replace(/[.*+?^${}()|[\]\\]/g, '\\$&')}\\s+\\S`).test(currentValue)
        if (selectingNaturalReview || selectingBareContinue || selectingBareEffort || selectingBareImprove || selectingContinueNumber || selectingUltraPlanObjective || selectingArgumentFreeText) {
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
    if (!next) {
      guidingQueueRef.current = ''
      setGuidingQueueId('')
      return
    }
    const id = sid
    const wasRunning = busy && streamingSid === sid
    ++runSeqRef.current
    try {
      if (wasRunning) {
        streamAbortRef.current?.abort?.()
        if (id) await chatApi(`/api/chat/cancel/${id}`, { method:'POST', body:'{}' })
        setMessages(xs => xs.map((m, idx) => (idx === xs.length - 1 && m.role === 'assistant' && !m.content) ? { ...m, content:ct('已中止，改为执行引导消息。', 'Stopped and switched to the guided message.'), error:true } : m))
      }
    } catch (e) {
      setErr(e.message || String(e))
    } finally {
      setBusy(false)
      setStreamingSid('')
      setNotice(ct('已引导：中止当前回复并发送队列消息', 'Guided: stopped the current response and sent the queued message'))
      setTimeout(() => runSend(next), 0)
    }
  }

  useEffect(() => {
    const initialize = async () => {
      try {
        const draft = memoryDraftRef.current
        if (draft) {
          await newSession()
          setPrompt(draft.prompt)
          setNotice(ct('已创建文件优化对话。请先审阅草稿，确认后再发送。', 'File-improvement chat created. Review the draft before sending.'))
          window.setTimeout(() => promptRef.current?.focus(), 0)
          return
        }
        await loadSessions('', { open:true })
      } catch (error) {
        setErr(error?.message || String(error))
      }
    }
    initialize()
    loadPromptPresets().catch(e=>setErr(e.message))
    api('/api/instances').then(payload => {
      const options = chatInstanceOptions(payload)
      setChatInstances(options)
      const serverDefaultID = payload?.default_instance_id || payload?.default_id
      if (!chatInstanceRef.current && serverDefaultID) {
        const defaultID = String(serverDefaultID).trim()
        chatInstanceRef.current = defaultID
        setChatInstanceID(defaultID)
        persistChatInstanceID(defaultID)
      }
    }).catch(e => setErr(e.message)).finally(() => setChatInstancesLoading(false))
    return () => streamAbortRef.current?.abort?.()
  }, [])

  useEffect(() => {
    loadSessions('', { open:true }).catch(e => { if (e?.name !== 'AbortError') setErr(e.message) })
  }, [chatInstanceID])

  useEffect(() => {
    let stopped = false
    let inFlight = false
    const refreshList = async () => {
      if (stopped || inFlight || document.hidden) return
      inFlight = true
      try {
        const d = await chatApi('/api/chat/sessions')
        if (!stopped) {
          setSessions(d.sessions || [])
          setProjects(Array.isArray(d.projects) ? d.projects : [])
        }
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
  }, [chatInstanceID])

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

  const switchChatInstance = (nextValue) => {
    const nextID = String(nextValue || '').trim()
    if (!nextID || nextID === chatInstanceRef.current) return
    streamAbortRef.current?.abort?.()
    streamAbortRef.current = null
    chatRequestEpochRef.current += 1
    openSeqRef.current += 1
    worldlineSeqRef.current += 1
    runSeqRef.current += 1
    activeRunRef.current = false
    activeSidRef.current = ''
    chatInstanceRef.current = nextID
    persistChatInstanceID(nextID)
    setChatInstanceID(nextID)
    setSid('')
    setSessions([])
    setProjects([])
    setMessages([])
    messagesRef.current = []
    setRawHistory([])
    setHistoryInfo([])
    setWorkingState(null)
    setPlanState(null)
    setSubagents([])
    setBusy(false)
    setStreamingSid('')
    setWorldlineOpen(false)
    setWorldlineState(null)
    setWorldlineLoading(false)
    setQueuedMessages([])
    setAttachments([])
    setErr('')
    setNotice(ct('已切换 GA 实例', 'GA instance switched'))
  }

  const scrollToThreadEnd = (behavior = 'auto') => {
    endRef.current?.scrollIntoView({ behavior, block:'end' })
    if (threadRef.current) previousScrollTopRef.current = threadRef.current.scrollTop
  }
  const setFollowState = (enabled) => {
    autoFollowRef.current = enabled
    setAutoFollow(enabled)
    setShowFollow(!enabled)
  }
  const resumeFollow = () => {
    setFollowState(true)
    scrollToThreadEnd('auto')
  }
  const updateFollowFromScroll = () => {
    const thread = threadRef.current
    if (!thread) return
    const scrollTop = thread.scrollTop
    const action = scrollFollowAction({
      nearBottom: isNearBottom(thread, 20),
      previousScrollTop: previousScrollTopRef.current,
      scrollTop,
    })
    previousScrollTopRef.current = scrollTop
    if (action === 'resume' && !autoFollowRef.current) setFollowState(true)
    else if (action === 'pause' && autoFollowRef.current) setFollowState(false)
  }
  const breakFollow = () => {
    if (autoFollowRef.current && !isNearBottom(threadRef.current, 12)) setFollowState(false)
  }

  useLayoutEffect(() => {
    if (autoFollow) {
      const behavior = scrollModeRef.current || 'auto'
      scrollModeRef.current = 'auto'
      scrollToThreadEnd(behavior)
    } else if (!isNearBottom(threadRef.current)) {
      setShowFollow(true)
    }
  }, [messages, busy, autoFollow])

  const lastThreadMessageId = messages.reduce((id, message) => message.kind === 'btw' ? id : message.id, '')
  useEffect(() => {
    const cards = threadRef.current?.querySelectorAll('.oa-message[data-id]')
    const tail = cards?.[cards.length - 1]
    if (!tail || typeof ResizeObserver === 'undefined') return
    const observer = new ResizeObserver(() => {
      if (autoFollowRef.current) scrollToThreadEnd('auto')
    })
    observer.observe(tail)
    return () => observer.disconnect()
  }, [lastThreadMessageId, sid])

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

  const projectSessionGroups = useMemo(() => groupProjectSessions(projects, sessions), [projects, sessions])
  const sessionSearchScopes = sessionSearchScopeOptions(chatLanguage())
  const recentSearchSessions = sessions.slice(0, 8)
  const filteredSessions = useMemo(() => {
    if (!sidebarSearch.trim()) return sessions
    const q = sidebarSearch.trim().toLowerCase()
    return sessions.filter(s => (s.title || '').toLowerCase().includes(q))
  }, [sessions, sidebarSearch])
  const filteredProjectGroups = useMemo(() => {
    if (!sidebarSearch.trim()) return projectSessionGroups
    const q = sidebarSearch.trim().toLowerCase()
    return projectSessionGroups.map(g => ({ ...g, sessions: g.sessions.filter(s => (s.title || '').toLowerCase().includes(q)) })).filter(g => g.name.toLowerCase().includes(q) || g.sessions.length > 0)
  }, [projectSessionGroups, sidebarSearch])
  const selectedSessionIdSet = useMemo(() => new Set(selectedSessionIds), [selectedSessionIds])
  const selectedSessionCount = sessions.reduce((count, session) => count + (selectedSessionIdSet.has(session.id) ? 1 : 0), 0)
  const allSessionsSelected = sessions.length > 0 && selectedSessionCount === sessions.length
  const activeModel = llms.find(x => x.index === llmNo) || llms[0]
  const selectedModelNo = activeModel?.index ?? llmNo
  const providerGroups = useMemo(() => buildModelProviderGroups(llms), [llms])
  const selectedProvider = findModelProviderValue(providerGroups, selectedModelNo) || (activeModel ? modelProvider(activeModel) : '')
  const isCurrentRunning = busy && streamingSid === sid
  const activePromptPreset = selectedPromptPresetView({ presets: promptPresets, selectedID: extraSysPromptPresetID, snapshot: extraSysPrompts })
  const contextJson = useMemo(() => JSON.stringify({ raw_history: rawHistory || [], history_info: historyInfo || [], working: workingState || {} }, null, 2), [rawHistory, historyInfo, workingState])
  const btwMessages = useMemo(() => messages.filter(message => message.kind === 'btw'), [messages])
  const copyContext = async () => {
    try {
      await navigator.clipboard.writeText(contextJson)
      setNotice(ct('模型上下文 JSON 已复制', 'Model context JSON copied'))
    } catch {
      setErr(ct('复制失败，请手动选择 JSON', 'Copy failed; select the JSON manually'))
    }
  }
  const contextHelpText = ct('查看本次对话实际发给模型的上下文快照，包括原始历史、工作状态等；这不是长期记忆。', 'View the context snapshot actually sent to the model, including raw history and working state. This is not long-term memory.')
  const worldlineHelpText = ct('查看当前对话的分支历史。选择可切换的节点后，会恢复该节点对应的对话和工作区状态；对话运行中不能切换。', 'View conversation branches. Switching to a mapped node restores its conversation and workspace state; switching is disabled while a reply is running.')

  const renderSidebarSession = session => <div key={session.id} className={`oa-session-row ${session.id === sid ? 'active' : ''} ${session.running ? 'is-running' : ''}`}>
    {editing === session.id ? <div className="oa-rename">
      <input value={draftTitle} autoFocus aria-label={ct('会话标题', 'Session title')} onChange={event=>setDraftTitle(event.target.value)} onKeyDown={event=>{ if(event.key==='Enter') saveRename(session.id); if(event.key==='Escape') setEditing('') }}/>
      <button onClick={()=>saveRename(session.id)} aria-label={ct('保存标题', 'Save title')}><Check size={14}/></button><button onClick={()=>setEditing('')} aria-label={ct('取消重命名', 'Cancel rename')}><X size={14}/></button>
    </div> : <button className="oa-session" onClick={()=>selectSidebarSession(session.id)} title={shortTitle(session)}>
      <span className="oa-session-title" title={shortTitle(session)}>{session.running && <i className="oa-session-running-dot" aria-hidden="true"/>}<b>{shortTitle(session)}</b>{draftSessionIds.has(session.id) && <em className="oa-session-draft-badge">{ct('草稿', 'Draft')}</em>}</span>
      <small><Clock3 size={11}/>{fmtTime(session.updated_at) || ct('刚刚', 'Just now')} · {ct(`${session.count || 0} 条`, `${session.count || 0} messages`)}{session.running && <em className="oa-session-running-label">{ct('运行中', 'Running')}</em>}</small>
    </button>}
    {editing !== session.id && <button className={`oa-session-more ${menuOpen === session.id ? 'is-open' : ''}`} onClick={event => {
      event.stopPropagation()
      if (menuOpen === session.id) { setMenuOpen(''); setMenuPos(null); return }
      const rect = event.currentTarget.getBoundingClientRect()
      setMenuPos({ top: Math.max(8, rect.top - 78), left: Math.max(8, rect.right - 136) })
      setMenuOpen(session.id)
    }} aria-label={ct('会话操作', 'Session actions')}><MoreHorizontal size={16} /></button>}
  </div>

  return <ChatFileScopeContext.Provider value={{ workspace: current?.workspace || '', gaRoot: cfg?.ga_root || cfg?.GARoot || '' }}>
    <div ref={chatScope} className={`oa-chat ${collapsed ? 'is-collapsed' : ''}`}>
    <aside className={`oa-sidebar ${collapsed ? 'collapsed' : ''}`}>
      <label className="oa-sidebar-instance" title={ct('切换实例会更新当前侧栏中的会话', 'Switching instances updates the sessions in this sidebar')}>
        <span>{ct('GA 实例', 'GA instance')}</span>
        <select
          aria-label={ct('选择 GA 实例', 'Select GA instance')}
          value={chatInstanceID}
          onChange={event=>switchChatInstance(event.target.value)}
          disabled={chatInstancesLoading || !chatInstances.length}
        >
          {chatInstancesLoading && <option value={chatInstanceID}>{ct('加载实例…', 'Loading instances…')}</option>}
          {!chatInstancesLoading && !chatInstances.length && <option value="">{ct('默认实例', 'Default instance')}</option>}
          {chatInstances.map(instance => <option key={instance.id} value={instance.id} disabled={instance.initializing}>{instance.name}{instance.initializing ? ct('（初始化中）', ' (initializing)') : ''}</option>)}
        </select>
      </label>
      <div className="oa-side-head">
        <div className="oa-sidebar-search" onClick={openSessionSearch}>
          <Search size={16}/>
          <input
            type="text"
            placeholder={ct('搜索会话...', 'Search sessions...')}
            value=""
            readOnly
            ref={sessionSearchTriggerRef}
            onKeyDown={event => { if (event.key === 'Enter' || event.key === ' ') openSessionSearch() }}
            aria-label={ct('搜索会话', 'Search sessions')}
            aria-haspopup="dialog"
          />
        </div>
        <button
          className="oa-new-chat"
          onClick={newSession}
          disabled={batchDeleting}
          title={ct('新对话', 'New chat')}
          aria-label={ct('新对话', 'New chat')}
        ><MessageSquarePlus size={17}/></button>
        <button className="oa-icon-btn" onClick={()=>setCollapsed(true)} title={ct('折叠', 'Collapse')}><Menu size={18}/></button>
      </div>
      <div className="oa-session-manager-head">
        <span className="oa-session-manager-title">{ct('历史会话', 'History')} <small>{sessions.length}</small></span>
        <button className="oa-session-manage-open" type="button" onClick={openSessionManager} disabled={!sessions.length}>{ct('管理', 'Manage')}</button>
      </div>
      {sidebarTab === 'history' ? <>
        <div className="oa-session-list">
          {filteredSessions.map(renderSidebarSession)}
          {!filteredSessions.length && <div className="oa-empty-list">{sidebarSearch ? ct('无匹配会话', 'No matching sessions') : ct('暂无历史会话', 'No session history')}</div>}
        </div>
      </> : <div className="oa-session-list oa-project-list">
        {filteredProjectGroups.map((group, index) => {
          const expanded = expandedProjectNames.has(group.name)
          const bodyId = `oa-project-sessions-${index}`
          const toggleLabel = ct(`${expanded ? '收起' : '展开'} ${group.name}`, `${expanded ? 'Collapse' : 'Expand'} ${group.name}`)
          return <section className={`oa-project-group ${expanded ? 'is-expanded' : 'is-collapsed'}`} key={group.name}>
            <div className="oa-project-head">
              <button className="oa-project-toggle" type="button" onClick={()=>setExpandedProjectNames(current => {
                const next = new Set(current)
                if (next.has(group.name)) next.delete(group.name)
                else next.add(group.name)
                return next
              })} aria-expanded={expanded} aria-controls={bodyId} aria-label={toggleLabel} title={toggleLabel}>
                <ChevronRight size={13} className="oa-project-chevron" aria-hidden="true"/><b title={group.name}>{group.name}</b><small>{group.sessions.length}</small>
              </button>
              <button className="oa-project-add" type="button" onClick={()=>newProjectSession(group.name)} disabled={batchDeleting} title={ct(`在 ${group.name} 中新建对话`, `Start a chat in ${group.name}`)} aria-label={ct(`在 ${group.name} 中新建对话`, `Start a chat in ${group.name}`)}><Plus size={15}/></button>
            </div>
            <div className="oa-project-body" id={bodyId} hidden={!expanded}>
              {group.sessions.map(renderSidebarSession)}
              {!group.sessions.length && <div className="oa-project-empty">{ct('暂无对话，点击 + 快速开始', 'No chats yet. Click + to start.')}</div>}
            </div>
          </section>
        })}
        {!filteredProjectGroups.length && <div className="oa-empty-list oa-projects-empty"><FolderOpen size={20}/><span>{sidebarSearch ? ct('无匹配项目', 'No matching projects') : ct('暂无可用项目', 'No projects available')}</span></div>}
      </div>}
      {!sessionManagerOpen && menuOpen && menuPos && (() => {
        const s = sessions.find(x => x.id === menuOpen)
        if (!s) return null
        return <div className="oa-session-menu" style={{ top: menuPos.top, left: menuPos.left }} onClick={e=>e.stopPropagation()}>
          <button onClick={()=>startRename(s)}><Edit3 size={14}/>{ct('重命名', 'Rename')}</button>
          <button className="danger" onClick={()=>deleteSession(s.id)}><Trash2 size={14}/>{ct('删除', 'Delete')}</button>
        </div>
      })()}
      <div className="oa-sidebar-foot">
        <button onClick={()=>window.location.href='/'}><ChevronLeft size={15}/>{ct('返回管理台', 'Back to admin')}</button>
      </div>
    </aside>
    {!collapsed && <button className="oa-sidebar-backdrop" type="button" aria-label="关闭侧栏" onClick={()=>setCollapsed(true)}/>}

    <main className="oa-main">
      <header className="oa-topbar">
        {collapsed && <div className="oa-collapsed-actions">
          <button className="oa-icon-btn oa-sidebar-toggle" onClick={()=>setCollapsed(false)} title={ct('展开侧栏', 'Expand sidebar')} aria-label={ct('展开侧栏', 'Expand sidebar')}><Menu size={18}/></button>
          <button className="oa-icon-btn oa-collapsed-new" onClick={newSession} title={ct('新对话', 'New chat')} aria-label={ct('新对话', 'New chat')}><MessageSquarePlus size={18}/></button>
        </div>}
        <div className="oa-title"><b title={current ? shortTitle(current) : '新对话'}>{current ? shortTitle(current) : '新对话'}</b><span>ChatGPT-style workspace for GenericAgent</span>{current?.project_mode && <span className="oa-project-badge" title={`Project Mode: ${current.project_mode}`}>Project: {current.project_mode}</span>}{current?.workspace && <span className="oa-workspace-badge" title={current.workspace}>Workspace: {current.workspace}</span>}</div>
        <div className="oa-topbar-actions" aria-label={ct('聊天工具', 'Chat tools')}>
          <button className={`oa-context-btn ${contextOpen ? 'is-open' : ''}`} type="button" onClick={()=>setContextOpen(v=>!v)} disabled={!sid} title={contextHelpText} aria-label={ct('查看模型上下文', 'View model context')}>
            <PanelRightOpen size={16}/><span className="oa-context-label">上下文</span><span className="oa-context-count">{rawHistory?.length || 0}</span><ChatFeatureHelp text={contextHelpText}/>
          </button>
          <button className={`oa-context-btn oa-worldline-btn ${worldlineOpen ? 'is-open' : ''}`} type="button" onClick={toggleWorldline} disabled={!sid} title={worldlineHelpText} aria-label={ct('查看和切换对话世界线', 'View and switch conversation branches')}>
            <GitBranch size={16}/><span className="oa-context-label">世界线</span>{(worldlineForView?.nodes?.length || 0) > 0 && <span className="oa-context-count">{worldlineForView.nodes.length}</span>}<ChatFeatureHelp text={worldlineHelpText}/>
          </button>
          <button
            ref={mobileToolsTriggerRef}
            className={`oa-icon-btn oa-mobile-tools-trigger ${mobileToolsOpen ? 'is-open' : ''}`}
            type="button"
            onClick={()=>setMobileToolsOpen(v=>!v)}
            aria-label={ct('打开聊天工具', 'Open chat tools')}
            aria-haspopup="dialog"
            aria-expanded={mobileToolsOpen}
            aria-controls="oa-mobile-tools-menu"
            title={ct('上下文、世界线与配色', 'Context, timeline, and theme')}
          ><MoreHorizontal size={18}/></button>
          <NotificationCenter lang={chatLanguage()} />
        </div>
      </header>

      {mobileToolsOpen && createPortal(<div className="oa-mobile-tools-layer">
        <div className="oa-mobile-tools-backdrop" aria-hidden="true" onClick={()=>setMobileToolsOpen(false)} />
        <div className="oa-mobile-tools-menu" id="oa-mobile-tools-menu" role="dialog" aria-label={ct('聊天工具', 'Chat tools')}>
          <button
            className={`oa-mobile-tools-item ${contextOpen ? 'is-active' : ''}`}
            type="button"
            disabled={!sid}
            onClick={()=>{ setMobileToolsOpen(false); setContextOpen(v=>!v) }}
          >
            <PanelRightOpen size={17}/><span className="oa-mobile-tools-item-copy">{ct('上下文', 'Context')}</span><b className="oa-mobile-tools-item-badge">{rawHistory?.length || 0}</b>
          </button>
          <button
            className={`oa-mobile-tools-item ${worldlineOpen ? 'is-active' : ''}`}
            type="button"
            disabled={!sid}
            onClick={()=>{ setMobileToolsOpen(false); toggleWorldline() }}
          >
            <GitBranch size={17}/><span className="oa-mobile-tools-item-copy">{ct('世界线', 'Timeline')}</span>{(worldlineForView?.nodes?.length || 0) > 0 && <b className="oa-mobile-tools-item-badge">{worldlineForView.nodes.length}</b>}
          </button>
          <ThemePicker
            className="oa-mobile-tools-theme"
            value={theme}
            onChange={(nextTheme)=>{ setTheme(nextTheme); setMobileToolsOpen(false) }}
            lang={chatLanguage()}
            variant="compact"
          />
          <ScalePicker value={uiScale} onChange={onUiScaleChange} lang={chatLanguage()} variant="compact" />
        </div>
      </div>, document.body)}

      {contextOpen && <aside className="oa-context-drawer" aria-label={ct('模型上下文', 'Model context')}>
        <div className="oa-context-head">
          <div><b>{ct('模型上下文', 'Model context')}</b><span>{ct('agent.llmclient.backend.history 完成后的快照', 'Snapshot after agent.llmclient.backend.history completes')}</span></div>
          <div className="oa-context-actions"><button type="button" onClick={copyContext}>{ct('复制 JSON', 'Copy JSON')}</button><button type="button" onClick={()=>setContextOpen(false)} aria-label={ct('关闭上下文', 'Close context')}><X size={15}/></button></div>
        </div>
        <div className="oa-context-json-tree"><JsonTree data={{ raw_history: rawHistory || [], history_info: historyInfo || [], working: workingState || {} }} /></div>
        <details className="oa-context-raw"><summary>{ct('原始 JSON', 'Raw JSON')}</summary><pre className="oa-context-raw-json">{contextJson}</pre></details>
      </aside>}
      {worldlineOpen && (
        <WorldlinePanel
          state={worldlineForView}
          loading={worldlineLoading}
          switchingId={worldlineSwitchingId}
          disabled={isCurrentRunning}
          onClose={() => setWorldlineOpen(false)}
          onRefresh={() => loadWorldline(sid, { force: true }).catch(() => {})}
          onSwitch={switchWorldline}
        />
      )}
      <section className="oa-thread" ref={threadRef} onScroll={updateFollowFromScroll} onWheel={e=>{ if (e.deltaY < 0) breakFollow() }} onTouchMove={breakFollow}>
        {messages.length === 0 && <div className="oa-empty">
          <h1>今天想让 GenericAgent 做什么？</h1>
          <p>支持 Markdown、代码块复制、图片输入、模型切换、会话重命名与删除。</p>
        </div>}
        <MessageList messages={messages} models={llms} isCurrentRunning={isCurrentRunning} onAskReply={fillAskReply} onEditResend={editAndResend} onRetry={retryFailedTurn} clockNow={streamClock} worldline={worldlineForView} onSwitchVersion={switchWorldline} chatInstanceID={chatInstanceID} />
        <SubagentStatusPanel states={subagents}/>
        {showFollow && <div className="oa-follow-row"><button className="oa-follow-btn" type="button" onClick={resumeFollow}><ChevronDown size={16}/>继续跟随</button></div>}
        <div ref={endRef}/>
      </section>

      <footer className="oa-composer-wrap">
        <PlanTodoCard plan={planState}/>
        {queuedMessages.length > 0 && <div className={`oa-queue-dock ${isCurrentRunning ? 'is-running' : 'is-idle'}`} aria-label={ct('待发送队列', 'Send queue')}>
          <div className="oa-queue-guide-hint">
            <Sparkles className="oa-queue-guide-icon" size={14} aria-hidden="true"/>
            <span className="oa-queue-guide-copy"><b>待发送</b><small>{isCurrentRunning ? '回复进行中，可接管任意一条立即发送' : '回复结束后将按顺序发送'}</small></span>
            <span className="oa-queue-count" aria-label={`${queuedMessages.length} 条待发送消息`}>{queuedMessages.length} 条</span>
          </div>
          {queuedMessages.map((q, i) => {
            const isEditingQueue = queueEditingId === q.id
            const isGuidingQueue = guidingQueueId === q.id
            return <div key={q.id} className={`oa-queued-item ${isEditingQueue ? 'is-editing' : ''} ${isGuidingQueue ? 'is-guiding' : ''}`}>
              <span className="oa-queue-index" aria-hidden="true">{String(i + 1).padStart(2, '0')}</span>
              <div className="oa-queue-content" title={isEditingQueue ? '' : (q.text || ct('请处理这些附件', 'Please process these attachments'))}>
                {isEditingQueue ? <textarea className="oa-queue-edit-input" value={queueDraft} autoFocus rows={2} onChange={e=>setQueueDraft(e.target.value)} onKeyDown={e=>{ if(e.key==='Enter' && (e.ctrlKey || e.metaKey)) saveQueueEdit(q.id); if(e.key==='Escape') cancelQueueEdit() }} /> : <>
                  <b>{q.text || ct('请处理这些附件', 'Please process these attachments')}</b>
                  {q.files?.length ? <em>{ct(`${q.files.length} 个附件`, `${q.files.length} attachments`)}</em> : null}
                </>}
              </div>
              <div className="oa-queue-actions">
                {isEditingQueue ? <>
                  <button className="oa-queue-action is-confirm" type="button" onClick={()=>saveQueueEdit(q.id)} title={ct('保存队列消息', 'Save queued message')} aria-label={ct('保存队列消息', 'Save queued message')}><Check size={14}/></button>
                  <button className="oa-queue-action" type="button" onClick={cancelQueueEdit} title={ct('取消编辑', 'Cancel editing')} aria-label={ct('取消编辑', 'Cancel editing')}><X size={14}/></button>
                </> : <>
                  <button className="oa-guide-btn" type="button" onClick={()=>guideQueuedItem(q.id)} disabled={!isCurrentRunning || Boolean(guidingQueueId)} title={isGuidingQueue ? '正在中止当前回复并发送这条消息' : (isCurrentRunning ? `暂停当前输出，立即发送消息${i + 1}` : '回复结束后会自动发送')}><Sparkles size={14}/>{isGuidingQueue ? '接管中…' : '引导发送'}</button>
                  <button className="oa-queue-action is-danger" type="button" onClick={()=>removeQueued(q.id)} title={ct('删除这条队列消息', 'Delete this queued message')} aria-label={ct('删除这条队列消息', 'Delete this queued message')}><Trash2 size={14}/></button>
                  <button className="oa-queue-action" type="button" onClick={()=>editQueued(q.id)} title={ct('编辑这条队列消息', 'Edit this queued message')} aria-label={ct('编辑这条队列消息', 'Edit this queued message')}><Edit3 size={14}/></button>
                </>}
              </div>
            </div>
          })}
        </div>}
        {cmdDrawer.open && <div className="oa-cmd-drawer" ref={cmdDrawerRef}>
          {filteredCmds.length === 0 && <div className="oa-cmd-item" style={{color:'var(--text-secondary)',justifyContent:'center',cursor:'default',padding:'12px 14px'}}>{ct('无匹配命令', 'No matching commands')}</div>}
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
          <div className="oa-cmd-manager" role="dialog" aria-modal="true" aria-label={ct('自定义斜杠命令', 'Custom slash commands')} onMouseDown={e=>e.stopPropagation()}>
            <div className="oa-cmd-manager-head">
              <div><h3>{ct('自定义斜杠命令', 'Custom slash commands')}</h3><p>{ct('官方命令只读锁定；用户命令可新增、编辑、删除。', 'Official commands are read-only; custom commands can be added, edited, and deleted.')}</p></div>
              <button className="oa-icon-btn" type="button" onClick={()=>setCmdManagerOpen(false)} title={ct('关闭', 'Close')}><X size={16}/></button>
            </div>
            <div className="oa-cmd-manager-actions">
              <button className="oa-guide-btn" type="button" onClick={()=>startEdit(-2, '/', '', '')}><Plus size={14}/>{ct('新增自定义命令', 'Add custom command')}</button>
              <span>{ct(`${(cfg?.slash_commands || []).filter(c => !isProtectedSlashCommand(c?.cmd)).length} 个自定义 · ${effectiveSlashCommands.length} 个官方`, `${(cfg?.slash_commands || []).filter(c => !isProtectedSlashCommand(c?.cmd)).length} custom · ${effectiveSlashCommands.length} official`)}</span>
            </div>
            {cmdEditIdx !== -1 && <div className="oa-cmd-edit-card">
              <input value={cmdEditCmd} onChange={e=>setCmdEditCmd(e.target.value)} placeholder={ct('命令，例如 /hello', 'Command, for example /hello')} autoFocus />
              <input value={cmdEditDesc} onChange={e=>setCmdEditDesc(e.target.value)} placeholder={ct('描述，例如 代码审查模板', 'Description, for example Code review template')} />
              <textarea value={cmdEditContent} onChange={e=>setCmdEditContent(e.target.value)} placeholder={ct('发送时展开成的指令内容。可用 {args} 插入 /命令 后面的参数。', 'Instruction content expanded on send. Use {args} for arguments after /command.')} rows={4}/>
              <button type="button" onClick={saveEdit}>{ct('保存', 'Save')}</button>
              <button type="button" onClick={()=>setCmdEditIdx(-1)}>{ct('取消', 'Cancel')}</button>
            </div>}
            <div className="oa-cmd-manager-list">
              <div className="oa-cmd-section-title">{ct('用户自定义', 'Custom')}</div>
              {(cfg?.slash_commands || []).filter(c => !isProtectedSlashCommand(c?.cmd)).length === 0 && <div className="oa-cmd-empty">{ct('暂无自定义命令，点击上方新增。', 'No custom commands. Use the button above to add one.')}</div>}
              {(cfg?.slash_commands || []).map((c, i) => {
                if (isProtectedSlashCommand(c?.cmd)) return null
                return <div className="oa-cmd-manage-row" key={`${c.cmd}-${i}`}>
                  <div><b>{c.cmd}</b><small>{c.desc || ct('无描述', 'No description')}</small>{(c.content || c.prompt) && <em>{c.content || c.prompt}</em>}</div>
                  <button type="button" onClick={()=>startEdit(i, c.cmd || '/', c.desc || '', c.content || c.prompt || '')}><Edit3 size={14}/>{ct('编辑', 'Edit')}</button>
                  <button type="button" onClick={()=>deleteCmd(i)}><Trash2 size={14}/>{ct('删除', 'Delete')}</button>
                </div>
              })}
              <div className="oa-cmd-section-title">{ct('官方命令', 'Official commands')}</div>
              {effectiveSlashCommands.map((c, i) => <div className="oa-cmd-manage-row is-locked" key={`${c.cmd}-${i}`}>
                <div><b>{c.cmd}</b><small>{c.desc || ct('官方命令', 'Official command')}</small></div>
                <span><Lock size={13}/>{ct('只读', 'Read-only')}</span>
              </div>)}
            </div>
          </div>
        </div>}
        {extraPromptOpen && <div className="oa-cmd-manager-backdrop" onMouseDown={()=>setExtraPromptOpen(false)}>
          <div className={`oa-cmd-manager oa-prompt-preset-dialog ${promptPresetManagerOpen ? 'is-managing' : 'is-picking'}`} role="dialog" aria-modal="true" aria-label={ct('系统提示预设', 'System-prompt presets')} onMouseDown={e=>e.stopPropagation()}>
            <div className="oa-cmd-manager-head">
              <div>
                <h3>{promptPresetManagerOpen ? ct('管理系统提示预设', 'Manage system-prompt presets') : ct('选择系统提示预设', 'Choose a system-prompt preset')}</h3>
                <p>{promptPresetManagerOpen ? ct('预设全局可用；已绑定会话会保留选择时的内容快照。', 'Presets are global; bound sessions keep a snapshot of the selected content.') : ct('每个会话可启用一个预设，运行 Agent 时动态追加。', 'Each session can enable one preset that is appended when the agent runs.')}</p>
              </div>
              <button className="oa-icon-btn" type="button" onClick={()=>setExtraPromptOpen(false)} title={ct('关闭', 'Close')}><X size={16}/></button>
            </div>
            {promptPresetManagerOpen ? <>
              <div className="oa-cmd-manager-actions">
                <button className="oa-guide-btn" type="button" onClick={()=>setExtraPromptDraft(items => [...items, createPromptPreset(items)])}><Plus size={14}/>{ct('新增预设', 'Add preset')}</button>
                <span>{ct(`${extraPromptDraft.length} 个预设`, `${extraPromptDraft.length} presets`)}</span>
              </div>
              <div className="oa-cmd-manager-list oa-prompt-preset-editor-list">
                {extraPromptDraft.length === 0 && <div className="oa-cmd-empty">{ct('暂无预设。新增后填写名称和提示内容。', 'No presets yet. Add one, then enter its name and prompt content.')}</div>}
                {extraPromptDraft.map((item, index) => <div className="oa-cmd-edit-card oa-prompt-preset-edit-card" key={item.id}>
                  <div className="oa-prompt-preset-edit-head">
                    <input value={item.name} onChange={e=>updateExtraPromptDraft(item.id, 'name', e.target.value)} placeholder={`${ct('预设名称', 'Preset name')} ${index + 1}`} aria-label={`${ct('预设名称', 'Preset name')} ${index + 1}`}/>
                    <code title={ct('稳定预设 ID', 'Stable preset ID')}>{item.id}</code>
                  </div>
                  <textarea value={item.content} onChange={e=>updateExtraPromptDraft(item.id, 'content', e.target.value)} placeholder={ct('输入追加到 Agent 系统提示中的内容', 'Enter content to append to the agent system prompt')} rows={5}/>
                  <button type="button" onClick={()=>setExtraPromptDraft(items => items.filter(preset => preset.id !== item.id))}><Trash2 size={14}/>{ct('删除', 'Delete')}</button>
                </div>)}
              </div>
              <div className="oa-cmd-manager-actions oa-prompt-preset-footer">
                <button className="oa-guide-btn" type="button" onClick={savePromptPresets} disabled={extraPromptSaving}>{extraPromptSaving ? ct('保存中…', 'Saving…') : ct('保存全局预设', 'Save global presets')}</button>
                <button type="button" onClick={()=>setPromptPresetManagerOpen(false)} disabled={extraPromptSaving}><ChevronLeft size={14}/>{ct('返回选择', 'Back to selection')}</button>
              </div>
            </> : <>
              <div className="oa-cmd-manager-actions">
                <span>{ct(`${promptPresets.length} 个可用预设`, `${promptPresets.length} presets available`)}</span>
                <button type="button" onClick={openPromptPresetManager}><Edit3 size={14}/>{ct('管理预设', 'Manage presets')}</button>
              </div>
              <div className="oa-cmd-manager-list oa-prompt-preset-picker" role="radiogroup" aria-label={ct('当前会话系统提示预设', 'Current session system-prompt preset')}>
                <label className={`oa-prompt-preset-option ${extraPromptSelection === '' ? 'is-selected' : ''}`}>
                  <input type="radio" name="extra-system-prompt-preset" value="" checked={extraPromptSelection === ''} onChange={()=>selectExtraPromptPreset('')}/>
                  <span className="oa-prompt-preset-radio"><Check size={13}/></span>
                  <span className="oa-prompt-preset-copy"><b>{ct('不使用预设', 'Do not use a preset')}</b><small>{ct('仅使用 Agent 默认系统提示', 'Use only the default agent system prompt')}</small></span>
                </label>
                {activePromptPreset.orphaned && <label className={`oa-prompt-preset-option is-orphaned ${extraPromptSelection === activePromptPreset.id ? 'is-selected' : ''}`}>
                  <input type="radio" name="extra-system-prompt-preset" value={activePromptPreset.id} checked={extraPromptSelection === activePromptPreset.id} onChange={()=>selectExtraPromptPreset(activePromptPreset.id)}/>
                  <span className="oa-prompt-preset-radio"><Check size={13}/></span>
                  <span className="oa-prompt-preset-copy"><b>{ct('已删除的预设', 'Deleted preset')}</b><small>{activePromptPreset.content || ct('当前会话仍保留原内容快照', 'This session still retains the original content snapshot')}</small></span>
                  <em>{ct('快照', 'Snapshot')}</em>
                </label>}
                {promptPresets.map(item => <label className={`oa-prompt-preset-option ${extraPromptSelection === item.id ? 'is-selected' : ''}`} key={item.id}>
                  <input type="radio" name="extra-system-prompt-preset" value={item.id} checked={extraPromptSelection === item.id} onChange={()=>selectExtraPromptPreset(item.id)}/>
                  <span className="oa-prompt-preset-radio"><Check size={13}/></span>
                  <span className="oa-prompt-preset-copy"><b>{item.name}</b><small>{item.content}</small></span>
                </label>)}
                {promptPresets.length === 0 && !activePromptPreset.orphaned && <div className="oa-cmd-empty">{ct('还没有预设。先进入“管理预设”新建一个。', 'No presets yet. Open Manage presets to create one.')}</div>}
              </div>
              <div className="oa-cmd-manager-actions oa-prompt-preset-footer">
                <button className="oa-guide-btn" type="button" onClick={saveExtraPromptSelection} disabled={extraPromptSaving}>{extraPromptSaving ? ct('应用中…', 'Applying…') : ct('应用到当前会话', 'Apply to current session')}</button>
                <button type="button" onClick={()=>setExtraPromptOpen(false)} disabled={extraPromptSaving}>{ct('取消', 'Cancel')}</button>
              </div>
            </>}
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
                <button type="button" onClick={()=>removeAttachment(attachment.id)} title={ct('移除附件', 'Remove attachment')} aria-label={ct(`移除附件 ${name}`, `Remove attachment ${name}`)}><X size={12}/></button>
              </div>
            })}
          </div>}
          {isUltraPlanPrompt && <div className="oa-ultraplan-mode" aria-live="polite"><span><Sparkles size={14}/>UltraPlan</span><b>\u5c06\u4ee5\u89c4\u5212\u6a21\u5f0f\u6267\u884c\uff0c\u5e76\u5728\u5b8c\u6210\u540e\u5c55\u793a run \u76ee\u5f55\u4e0e\u65e5\u5fd7\u6458\u8981</b></div>}
          <textarea ref={promptRef} value={prompt} onPaste={onPaste} onChange={handlePromptChange} onKeyDown={handlePromptKeyDown} placeholder={isMobile ? '发送消息或添加文件…' : '\u5411 GenericAgent \u53d1\u9001\u6d88\u606f\uff0c\u53ef\u9009\u62e9/\u7c98\u8d34/\u62d6\u62fd\u4efb\u610f\u6587\u4ef6\u2026'} rows={1}/>
          <div className="oa-composer-bar">
            <button className="oa-attach-btn" type="button" onClick={()=>fileRef.current?.click()} title={ct('添加附件', 'Add attachment')}><Paperclip size={17}/><span>{ct('附件', 'Attachments')}</span></button>
            <button className={`oa-attach-btn ${cmdManagerOpen ? 'is-open' : ''}`} type="button" onClick={()=>setCmdManagerOpen(true)} title={ct('管理自定义斜杠命令', 'Manage custom slash commands')}><Sparkles size={16}/><span>{ct('命令', 'Commands')}</span></button>
            <button className={`oa-attach-btn ${extraPromptOpen || extraSysPromptPresetID ? 'is-open' : ''}`} type="button" onClick={openExtraPromptEditor} title={extraSysPromptPresetID ? ct(`当前预设：${activePromptPreset.name}`, `Current preset: ${activePromptPreset.name}`) : ct('选择本会话的系统提示预设', 'Choose a system-prompt preset for this session')}><Bot size={16}/><span>{ct('系统提示', 'System prompt')}{extraSysPromptPresetID ? ` · ${activePromptPreset.name}` : ''}</span></button>
            <ProviderModelCascade groups={providerGroups} selectedProvider={selectedProvider}
              value={selectedModelNo} disabled={!providerGroups.length || isCurrentRunning || modelSwitching}
              disabledReason={!providerGroups.length ? '尚未配置可用模型' : modelSwitching ? '正在切换模型' : isCurrentRunning ? '回复生成期间不可切换模型' : ''}
              onChange={v=>saveModel(Number(v))} mobile={isMobile} />
            <div className="oa-model-select oa-effort-select"><span>推理</span>
              <CustomSelect value={reasoningEffort} onChange={v=>saveReasoningEffort(v)}
                options={REASONING_EFFORT_OPTIONS} native={isMobile} ariaLabel="推理强度" />
            </div>
            <button className="oa-send" type="button" disabled={modelSwitching || (!prompt.trim() && !attachments.length)} onClick={() => send()} title={modelSwitching ? '正在切换模型' : isCurrentRunning ? '加入发送队列' : '发送'} aria-label={modelSwitching ? '正在切换模型' : isCurrentRunning ? '加入发送队列' : '发送'}><Send size={17}/></button>
            {isCurrentRunning && <button className="oa-stop" type="button" onClick={()=>cancelRun(sid)} title="停止生成" aria-label="停止生成"><Square size={14}/></button>}
          </div>
        </div>
      </footer>
    </main>

    <SessionSearchDialog
      open={sessionSearchOpen}
      lang={chatLanguage()}
      query={sessionSearchQuery}
      scope={sessionSearchScope}
      scopes={sessionSearchScopes}
      history={sessionSearchHistory}
      recentSessions={recentSearchSessions}
      results={sessionSearchResults}
      loading={sessionSearchLoading}
      error={sessionSearchError}
      currentSessionID={sid}
      onQueryChange={value => setSessionSearchQuery(value)}
      onScopeChange={value => setSessionSearchScope(value)}
      onSubmit={submitSessionSearch}
      onSelectHistory={selectSessionSearchHistory}
      onClearHistory={() => setSessionSearchHistory(clearSessionSearchHistory())}
      onSelectSession={selectSessionSearchResult}
      onClose={closeSessionSearch}
    />
    {worldlineRestorePicker && worldlineRestorePicker.sessionID === sid && <WorldlineRestoreDialog nodes={worldlineRestorePicker.nodes} onClose={()=>setWorldlineRestorePicker(null)} onSelect={selectWorldlineRestoreNode}/>}
    {sessionManagerOpen && <div className="oa-session-manager-backdrop" onMouseDown={e=>{ if (e.target === e.currentTarget) closeSessionManager() }}>
      <section className="oa-session-manager-modal" role="dialog" aria-modal="true" aria-labelledby="oa-session-manager-dialog-title" onMouseDown={e=>e.stopPropagation()}>
        <header className="oa-session-manager-dialog-head">
          <div className="oa-session-manager-dialog-heading">
            <h2 id="oa-session-manager-dialog-title">管理历史会话</h2>
            <p>批量删除不再需要的会话</p>
          </div>
          <button className="oa-icon-btn oa-session-manager-dialog-close" type="button" onClick={closeSessionManager} disabled={batchDeleting} aria-label={ct('关闭会话管理', 'Close session manager')} autoFocus><X size={17}/></button>
        </header>
        <div className="oa-session-manager-dialog-tools">
          <button className="oa-session-select-all" type="button" role="checkbox" aria-checked={allSessionsSelected ? true : (selectedSessionCount ? 'mixed' : false)} onClick={toggleAllSessions} disabled={!sessions.length || batchDeleting}>
            <span className={`oa-session-check ${allSessionsSelected ? 'is-checked' : ''} ${!allSessionsSelected && selectedSessionCount ? 'is-partial' : ''}`}>{allSessionsSelected && <Check size={12}/>}</span>
            <span>{allSessionsSelected ? ct('取消全选', 'Clear selection') : ct('全选', 'Select all')}</span>
          </button>
          <span className="oa-session-selected-count">{ct('已选', 'Selected')} {selectedSessionCount} / {sessions.length}</span>
        </div>
        <div className="oa-session-manager-dialog-list">
          {sessions.map(s => {
            const selected = selectedSessionIdSet.has(s.id)
            const sourceLabel = s.title_source === 'generated' ? 'AI' : s.title_source === 'manual' ? '手动' : '旧标题'
            return <button key={s.id} className={`oa-session-manager-dialog-row ${selected ? 'is-selected' : ''}`} type="button" role="checkbox" aria-checked={selected} onClick={()=>toggleSessionSelection(s.id)} disabled={batchDeleting}>
              <span className={`oa-session-check ${selected ? 'is-checked' : ''}`}>{selected && <Check size={12}/>}</span>
              <span className="oa-session-dialog-copy">
                <span className="oa-session-dialog-title">{s.running && <i className="oa-session-running-dot" aria-hidden="true"/>}<b>{shortTitle(s)}</b>{draftSessionIds.has(s.id) && <em className="oa-session-draft-badge">{ct('草稿', 'Draft')}</em>}{s.id === sid && <em>当前</em>}<em className={`is-title-source is-${s.title_source || 'legacy'}`}>{sourceLabel}</em></span>
                <small><Clock3 size={12}/>{fmtTime(s.updated_at) || ct('刚刚', 'Just now')} · {s.count || 0} 条{s.running && <span>运行中</span>}</small>
              </span>
            </button>
          })}
          {!sessions.length && <div className="oa-session-manager-dialog-empty">{ct('暂无历史会话', 'No session history')}</div>}
        </div>
        <footer className="oa-session-manager-dialog-foot">
          <small>{ct('删除后无法恢复', 'Deleted sessions cannot be recovered')}</small>
          <div>
            <button className="oa-session-dialog-cancel" type="button" onClick={closeSessionManager} disabled={batchDeleting}>{ct('取消', 'Cancel')}</button>
            <button className="oa-session-dialog-delete" type="button" onClick={deleteSelectedSessions} disabled={!selectedSessionCount || batchDeleting}>
              <Trash2 size={15}/><span>{batchDeleting ? ct('正在删除…', 'Deleting…') : ct(`删除所选${selectedSessionCount ? ` (${selectedSessionCount})` : ''}`, `Delete selected${selectedSessionCount ? ` (${selectedSessionCount})` : ''}`)}</span>
            </button>
          </div>
        </footer>
      </section>
    </div>}
    </div>
  </ChatFileScopeContext.Provider>
}
