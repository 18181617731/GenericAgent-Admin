import React, { useEffect, useRef } from 'react'
import { Clock3, FileText, FolderOpen, History, MessageCircle, Search, Trash2, X } from 'lucide-react'

const copyFor = lang => lang === 'en' ? {
  dialog: 'Search sessions', input: 'Search session titles, messages, or projects', close: 'Close search', history: 'Search history', clear: 'Clear history', recent: 'Recent sessions', results: 'Search results', searching: 'Searching…', empty: 'No matching sessions', noRecent: 'No sessions yet', noHistory: 'No search history yet', messages: 'messages', title: 'Title', content: 'Message', project: 'Project', archived: 'Archived', all: 'All', untitled: 'Untitled session', justNow: 'Just now', searchFailed: 'Search failed. Try again.',
} : {
  dialog: '搜索会话', input: '搜索标题、消息内容或项目', close: '关闭搜索', history: '搜索历史', clear: '清空历史', recent: '最近会话', results: '搜索结果', searching: '搜索中…', empty: '没有找到匹配的会话', noRecent: '暂无会话', noHistory: '还没有搜索记录', messages: '条消息', title: '标题', content: '消息', project: '项目', archived: '已归档', all: '全部', untitled: '未命名会话', justNow: '刚刚', searchFailed: '搜索失败，请稍后重试。',
}

const formatTime = (value, lang) => {
  if (!value) return copyFor(lang).justNow
  const date = new Date(Number(value) < 1e12 ? Number(value) * 1000 : Number(value))
  if (Number.isNaN(date.getTime())) return copyFor(lang).justNow
  return date.toLocaleString(lang === 'en' ? 'en-US' : 'zh-CN', { month:'numeric', day:'numeric', hour:'2-digit', minute:'2-digit' })
}

const resultIcon = type => type === 'project' ? FolderOpen : type === 'content' ? MessageCircle : FileText

export default function SessionSearchDialog({ open, lang = 'zh', query = '', scope = 'all', scopes = [], history = [], recentSessions = [], results = [], loading = false, error = '', currentSessionID = '', onQueryChange, onScopeChange, onSubmit, onSelectHistory, onClearHistory, onSelectSession, onClose }) {
  const copy = copyFor(lang)
  const inputRef = useRef(null)

  useEffect(() => {
    if (open) window.setTimeout(() => inputRef.current?.focus(), 0)
  }, [open])

  useEffect(() => {
    if (!open) return undefined
    const closeOnEscape = event => {
      if (event.key === 'Escape') onClose?.()
    }
    document.addEventListener('keydown', closeOnEscape)
    return () => document.removeEventListener('keydown', closeOnEscape)
  }, [open, onClose])

  if (!open) return null

  const scopeLabel = value => scopes.find(item => item.value === value)?.label || copy.all
  const matchLabels = session => {
    const values = Array.isArray(session.match_types) && session.match_types.length ? session.match_types : [session.match_type]
    return values.filter(Boolean).map(value => value === 'content' ? copy.content : value === 'project' ? copy.project : copy.title)
  }

  const renderSession = (session, type = 'recent') => {
    const Icon = resultIcon(type === 'recent' ? '' : type)
    const title = session.title || copy.untitled
    const labels = type === 'recent' ? [] : matchLabels(session)
    return <button key={session.id} type="button" className={`oa-search-result ${session.id === currentSessionID ? 'is-current' : ''}`} onClick={() => onSelectSession?.(session.id)}>
      <span className="oa-search-result-icon"><Icon size={17}/></span>
      <span className="oa-search-result-copy">
        <b>{title}</b>
        {session.snippet && <small>{session.snippet}</small>}
        <em><Clock3 size={11}/>{formatTime(session.updated_at, lang)} · {session.count || 0} {copy.messages}{labels.length > 0 && ` · ${labels.join(' · ')}`}{session.project && <span className="oa-search-result-project"><FolderOpen size={11}/>{session.project}</span>}{session.archived && <span className="oa-session-archived-badge">{copy.archived}</span>}</em>
      </span>
      {session.id === currentSessionID && <i className="oa-search-current-mark">✓</i>}
    </button>
  }

  return <div className="oa-session-search-backdrop" onMouseDown={event => { if (event.target === event.currentTarget) onClose() }}>
    <section className="oa-session-search-dialog" role="dialog" aria-modal="true" aria-label={copy.dialog} onMouseDown={event => event.stopPropagation()}>
      <div className="oa-session-search-input-wrap">
        <Search size={18}/>
        <input ref={inputRef} type="search" value={query} placeholder={copy.input} onChange={event => onQueryChange?.(event.target.value)} onKeyDown={event => { if (event.key === 'Enter') onSubmit?.() }} aria-label={copy.dialog}/>
        <button type="button" className="oa-session-search-close" onClick={onClose} aria-label={copy.close} title={copy.close}><X size={18}/></button>
      </div>
      <div className="oa-session-search-scopes" role="tablist" aria-label={copy.dialog}>
        {scopes.map(item => <button key={item.value} type="button" role="tab" aria-selected={scope === item.value} className={scope === item.value ? 'is-active' : ''} onClick={() => onScopeChange?.(item.value)}>{item.label}</button>)}
      </div>
      <div className="oa-session-search-body">
        {!query.trim() ? <>
          <section className="oa-session-search-section">
            <header><span><History size={15}/>{copy.history}</span>{history.length > 0 && <button type="button" onClick={onClearHistory}><Trash2 size={13}/>{copy.clear}</button>}</header>
            {history.length > 0 ? <div className="oa-session-search-chips">{history.map(item => <button key={`${item.scope}:${item.query}`} type="button" onClick={() => onSelectHistory?.(item)}><span>{item.query}</span><small>{scopeLabel(item.scope)}</small></button>)}</div> : <p className="oa-session-search-empty-line">{copy.noHistory}</p>}
          </section>
          <section className="oa-session-search-section oa-session-search-recent">
            <header><span><Clock3 size={15}/>{copy.recent}</span></header>
            {recentSessions.length > 0 ? recentSessions.map(session => renderSession(session)) : <p className="oa-session-search-empty-line">{copy.noRecent}</p>}
          </section>
        </> : <section className="oa-session-search-section oa-session-search-results">
          <header><span><Search size={15}/>{copy.results}</span><small>{loading ? copy.searching : `${results.length}`}</small></header>
          {loading ? <div className="oa-session-search-state">{copy.searching}</div> : error ? <div className="oa-session-search-state is-error">{error || copy.searchFailed}</div> : results.length > 0 ? results.map(session => renderSession(session, session.match_type)) : <div className="oa-session-search-state">{copy.empty}</div>}
        </section>}
      </div>
    </section>
  </div>
}
