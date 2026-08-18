import React from 'react'
import { Check, Clock3, MoreHorizontal, Pin, X } from 'lucide-react'

const defaultCopy = (zh) => zh

const sessionTitle = session => String(session?.title || '').trim() || '新会话'

export default function ChatSessionRow({
  session,
  active = false,
  editing = false,
  draftTitle = '',
  draftSessionIds = new Set(),
  loopView = null,
  menuOpen = false,
  privacyTitle = '',
  ct = defaultCopy,
  formatTime = value => value || '',
  onSelect = () => {},
  onDraftTitleChange = () => {},
  onSaveRename = () => {},
  onCancelRename = () => {},
  onOpenMenu = () => {},
}) {
  const title = privacyTitle || sessionTitle(session)
  const hasDraft = draftSessionIds.has(session?.id)
  const saveRename = event => {
    event.preventDefault()
    onSaveRename(session.id)
  }

  return <div className={`oa-session-row ${active ? 'active' : ''} ${session?.running ? 'is-running' : ''} ${session?.pinned ? 'is-pinned' : ''}`}>
    {editing ? <div className="oa-rename">
      <input
        value={draftTitle}
        autoFocus
        aria-label={ct('会话标题', 'Session title')}
        onChange={event => onDraftTitleChange(event.target.value)}
        onKeyDown={event => {
          if (event.key === 'Enter') saveRename(event)
          if (event.key === 'Escape') onCancelRename()
        }}
      />
      <button type="button" onClick={saveRename} aria-label={ct('保存标题', 'Save title')}><Check size={14}/></button>
      <button type="button" onClick={onCancelRename} aria-label={ct('取消重命名', 'Cancel rename')}><X size={14}/></button>
    </div> : <button
      className="oa-session"
      type="button"
      onClick={() => onSelect(session.id)}
      title={title}
      aria-current={active ? 'page' : undefined}
    >
      <span className="oa-session-title" title={title}>
        {session?.pinned && <span className="oa-session-pin" title={ct('已置顶', 'Pinned')}><Pin size={12} aria-hidden="true"/></span>}
        <b>{title}</b>
        <span className="oa-session-badges" aria-label={ct('会话状态', 'Session status')}>
          {loopView && <em className="oa-session-loop-badge" title={ct(`Loop 进行中 · 第 ${loopView.round}/${loopView.maxRounds} 轮`, `Loop active · round ${loopView.round}/${loopView.maxRounds}`)}>Loop {loopView.round}/{loopView.maxRounds}</em>}
          {session?.hub_enabled && <em className="oa-session-hub-badge" title={ct('已入驻官方 Hub', 'Joined official Hub')}>Hub</em>}
          {hasDraft && <em className="oa-session-draft-badge">{ct('草稿', 'Draft')}</em>}
        </span>
      </span>
      <span className="oa-session-meta">
        <span className="oa-session-time"><Clock3 size={11} aria-hidden="true"/>{formatTime(session?.updated_at) || ct('刚刚', 'Just now')}</span>
        <span>{ct(`${session?.count || 0} 条`, `${session?.count || 0} messages`)}</span>
        {session?.running && <span className="oa-session-running-label"><i aria-hidden="true"/>{ct('运行中', 'Running')}</span>}
      </span>
    </button>}
    {!editing && <button
      className={`oa-session-more ${menuOpen ? 'is-open' : ''}`}
      type="button"
      onClick={event => onOpenMenu(session, event)}
      aria-label={ct('会话操作', 'Session actions')}
      aria-haspopup="menu"
      aria-expanded={menuOpen}
    ><MoreHorizontal size={16} aria-hidden="true"/></button>}
  </div>
}
