import { useState } from 'react'
import { Archive, BookOpen, Braces, ChevronDown, CircleHelp, Copy, Download, ExternalLink, FileCode2, FileText, FolderOpen, History, Lightbulb, MessageSquare, RefreshCw, ScrollText } from 'lucide-react'

const MEMORY_GROUP_PREVIEW_LIMIT = 8

const fileName = path => String(path || '').replace(/\\/g, '/').split('/').filter(Boolean).pop() || path || '-'

const formatSize = (size) => {
  const bytes = Number(size)
  if (!Number.isFinite(bytes) || bytes < 0) return '-'
  if (bytes < 1024) return `${bytes} B`
  if (bytes < 1024 ** 2) return `${Math.ceil(bytes / 1024)} KB`
  return `${(bytes / 1024 ** 2).toFixed(1)} MB`
}

const isWorkflow = entry => /(?:^|_)sop\.md$/i.test(String(entry?.name || ''))
const isChatOptimizable = entry => entry.kind !== 'dir' && /\.(?:md|markdown|txt|py|js|jsx|ts|tsx|json|ya?ml|toml|ini)$/i.test(String(entry?.name || ''))

const asFile = (file) => ({
  name: fileName(file?.path),
  path: file?.path || '',
  kind: 'file',
  size: file?.size,
  mod_time: file?.mod_time,
})

function MemoryHelp({ label, text }) {
  return <span className="memory-help" tabIndex="0" title={text} aria-label={`${label}：${text}`} data-tooltip={text}><CircleHelp size={15}/></span>
}

function MemoryFileActions({ entry, text, onOpen, onDownload, onReveal, onCopy, onDiscuss }) {
  const isDirectory = entry.kind === 'dir'
  return <div className="memory-file-actions" aria-label={`${entry.name} ${text.actions.title}`}>
    <button type="button" title={text.actions.open} aria-label={`${text.actions.open}: ${entry.name}`} onClick={() => onOpen(entry)}><FolderOpen size={15}/></button>
    <button type="button" title={text.actions.download} aria-label={`${text.actions.download}: ${entry.name}`} onClick={() => onDownload(entry.path)} disabled={isDirectory}><Download size={15}/></button>
    <button type="button" title={text.actions.reveal} aria-label={`${text.actions.reveal}: ${entry.name}`} onClick={() => onReveal(entry)}><ExternalLink size={15}/></button>
    <button type="button" title={text.actions.copy} aria-label={`${text.actions.copy}: ${entry.name}`} onClick={() => onCopy(entry.path)}><Copy size={15}/></button>
    {isChatOptimizable(entry) && <button type="button" className="memory-discuss-action" title={text.actions.discuss} aria-label={`${text.actions.discuss}: ${entry.name}`} onClick={() => onDiscuss(entry, text.chatPrompt(entry))}><MessageSquare size={15}/></button>}
  </div>
}

function MemoryFileCard({ entry, group, text, onOpen, onDownload, onReveal, onCopy, onDiscuss }) {
  const isDirectory = entry.kind === 'dir'
  const Icon = isDirectory ? FolderOpen : /\.py$/i.test(entry.name) ? FileCode2 : FileText
  const description = text.fileHelp(entry, group.id)
  return <article className={`memory-file-card${isDirectory ? ' is-directory' : ''}`}>
    <span className="memory-file-icon"><Icon size={18}/></span>
    <div className="memory-file-main">
      <div className="memory-file-title"><b title={entry.name}>{entry.name}</b><MemoryHelp label={entry.name} text={description}/></div>
      <span title={entry.path}>{entry.path}</span>
      <small>{isDirectory ? text.directoryMeta : `${text.fileMeta(entry)} · ${formatSize(entry.size)}`}</small>
    </div>
    <MemoryFileActions entry={entry} text={text} onOpen={onOpen} onDownload={onDownload} onReveal={onReveal} onCopy={onCopy} onDiscuss={onDiscuss}/>
  </article>
}

function MemoryGroup({ group, text, onOpen, onDownload, onReveal, onCopy, onDiscuss }) {
  const Icon = group.icon
  const [expanded, setExpanded] = useState(false)
  const visibleItems = expanded ? group.items : group.items.slice(0, MEMORY_GROUP_PREVIEW_LIMIT)
  const hasMoreItems = group.items.length > MEMORY_GROUP_PREVIEW_LIMIT
  return <section className={`memory-group memory-group-${group.id}`} aria-labelledby={`memory-group-${group.id}`}>
    <div className="memory-group-head">
      <span className="memory-group-icon"><Icon size={18}/></span>
      <div><div className="memory-group-title"><h3 id={`memory-group-${group.id}`}>{group.title}</h3><MemoryHelp label={group.title} text={group.description}/></div><p>{group.description}</p></div>
      <b className="memory-group-count">{text.items(group.items.length)}</b>
    </div>
    <div className="memory-file-list">
      {visibleItems.map(entry => <MemoryFileCard key={entry.path} entry={entry} group={group} text={text} onOpen={onOpen} onDownload={onDownload} onReveal={onReveal} onCopy={onCopy} onDiscuss={onDiscuss}/>)}</div>
    {hasMoreItems && <button type="button" className="memory-group-more" aria-expanded={expanded} onClick={() => setExpanded(value => !value)}>{expanded ? text.collapse : text.showAll(group.items.length)}<ChevronDown size={15}/></button>}
  </section>
}

const groupsFor = (memory, text) => {
  const sops = memory?.sops || []
  const workflow = sops.filter(isWorkflow)
  const notes = sops.filter(entry => !isWorkflow(entry))
  return [
    { id: 'core', icon: Lightbulb, title: text.groups.core.title, description: text.groups.core.description, items: [memory?.insight, memory?.facts].filter(entry => entry?.exists).map(asFile) },
    { id: 'workflow', icon: ScrollText, title: text.groups.workflow.title, description: text.groups.workflow.description, items: workflow },
    { id: 'notes', icon: BookOpen, title: text.groups.notes.title, description: text.groups.notes.description, items: notes },
    { id: 'tools', icon: Braces, title: text.groups.tools.title, description: text.groups.tools.description, items: memory?.utils || [] },
    { id: 'packages', icon: FolderOpen, title: text.groups.packages.title, description: text.groups.packages.description, items: memory?.workspaces || [] },
    { id: 'materials', icon: Archive, title: text.groups.materials.title, description: text.groups.materials.description, items: memory?.materials || [] },
    { id: 'history', icon: History, title: text.groups.history.title, description: text.groups.history.description, items: memory?.raw_sessions || [] },
  ].filter(group => group.items.length)
}

export function MemoryPage({ t, memory, onOpen, onDownload, onReveal, onCopy, onDiscuss, onRefresh, refreshing = false }) {
  const text = t.memoryWorkspace
  const groups = groupsFor(memory, text)
  return <section className="memory-workspace">
    <div className="memory-workspace-head">
      <p className="memory-workspace-intro">{text.intro}</p>
      <button type="button" className="memory-refresh" onClick={onRefresh} disabled={refreshing}><RefreshCw size={15} className={refreshing ? 'is-spinning' : ''}/>{refreshing ? text.refreshing : text.refresh}</button>
    </div>
    {groups.length
      ? <div className="memory-groups">{groups.map(group => <MemoryGroup key={group.id} group={group} text={text} onOpen={onOpen} onDownload={onDownload} onReveal={onReveal} onCopy={onCopy} onDiscuss={onDiscuss}/>)}</div>
      : <p className="muted">{text.empty}</p>}
  </section>
}
