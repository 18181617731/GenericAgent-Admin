import React from 'react'
import { ChevronRight, FolderOpen, Pin } from 'lucide-react'
import ChatSessionRow from './ChatSessionRow.jsx'
import ProjectActionsMenu from './ProjectActionsMenu.jsx'
import ProjectDragHandle from './ProjectDragHandle.jsx'
import { loopSidebarView } from '../lib/chatLoopSidebar.js'

const defaultCopy = (zh) => zh
const EMPTY_SET = new Set()

const moveSessionFocus = event => {
  if (!['ArrowDown', 'ArrowUp', 'Home', 'End'].includes(event.key)) return
  const list = event.currentTarget
  const sessions = [...list.querySelectorAll('.oa-session:not(:disabled)')]
  if (!sessions.length) return
  const current = sessions.indexOf(document.activeElement)
  let next = current
  if (event.key === 'Home') next = 0
  else if (event.key === 'End') next = sessions.length - 1
  else if (event.key === 'ArrowDown') next = current < 0 ? 0 : Math.min(current + 1, sessions.length - 1)
  else next = current < 0 ? sessions.length - 1 : Math.max(current - 1, 0)
  event.preventDefault()
  sessions[next]?.focus()
}

export default function ChatSessionList({
  tab = 'history',
  recentGroups = [],
  projectGroups = [],
  recentGroupLabels = {},
  sidebarSearch = '',
  expandedProjectNames = EMPTY_SET,
  draftSessionIds = EMPTY_SET,
  activeSessionID = '',
  editingSessionID = '',
  draftTitle = '',
  menuOpen = '',
  privacyMode = false,
  privacyTitleByID = new Map(),
  ct = defaultCopy,
  formatTime,
  onSelectSession,
  onDraftTitleChange,
  onSaveRename,
  onCancelRename,
  onOpenMenu,
  onToggleProject,
  onToggleProjectPinned,
  onNewProjectSession,
  onOpenProjectFolder,
  projectSortMode = false,
  projectOrderSaving = false,
  projectSessionGroups = [],
  onProjectOrder,
  batchDeleting = false,
}) {
  const row = session => <ChatSessionRow
    key={session.id}
    session={session}
    active={session.id === activeSessionID}
    editing={session.id === editingSessionID}
    draftTitle={draftTitle}
    draftSessionIds={draftSessionIds}
    loopView={loopSidebarView(session.loop)}
    menuOpen={session.id === menuOpen}
    privacyTitle={privacyMode ? privacyTitleByID.get(session.id) : ''}
    ct={ct}
    formatTime={formatTime}
    onSelect={onSelectSession}
    onDraftTitleChange={onDraftTitleChange}
    onSaveRename={onSaveRename}
    onCancelRename={onCancelRename}
    onOpenMenu={onOpenMenu}
  />

  if (tab === 'projects') return <div className="oa-session-list oa-project-list" aria-label={ct('项目会话', 'Project sessions')} onKeyDown={moveSessionFocus}>
    {projectGroups.map((group, index) => {
      const projectKey = group.key || group.name
      const expanded = expandedProjectNames.has(projectKey)
      const bodyId = `oa-project-sessions-${index}`
      const toggleLabel = ct(`${expanded ? '收起' : '展开'} ${group.name}`, `${expanded ? 'Collapse' : 'Expand'} ${group.name}`)
      const newLabel = ct(`在 ${group.name} 中新建对话`, `Start a chat in ${group.name}`)
      const pinLabel = group.pinned ? ct(`取消置顶 ${group.name}`, `Unpin ${group.name}`) : ct(`置顶 ${group.name}`, `Pin ${group.name}`)
      const folderLabel = ct(`打开 ${group.name} 的项目文件夹`, `Open ${group.name} project folder`)
      const reorderLabel = ct('长按拖动排序', 'Hold to reorder')
      const displayName = privacyMode ? ct(`项目 ${String(index + 1).padStart(2, '0')}`, `Project ${String(index + 1).padStart(2, '0')}`) : group.name
      const privateToggleLabel = ct(`${expanded ? '收起' : '展开'} ${displayName}`, `${expanded ? 'Collapse' : 'Expand'} ${displayName}`)
      const privateNewLabel = ct(`在 ${displayName} 中新建对话`, `Start a chat in ${displayName}`)
      const privatePinLabel = group.pinned ? ct(`取消置顶 ${displayName}`, `Unpin ${displayName}`) : ct(`置顶 ${displayName}`, `Pin ${displayName}`)
      return <section data-project-name={privacyMode ? undefined : projectKey} className={`oa-project-group ${expanded ? 'is-expanded' : 'is-collapsed'} ${group.pinned ? 'is-pinned' : ''}`} key={projectKey}>
        <div className="oa-project-head">
          <button className="oa-project-toggle" type="button" onClick={() => onToggleProject?.(projectKey)} aria-expanded={expanded} aria-controls={bodyId} aria-label={privacyMode ? privateToggleLabel : toggleLabel} title={privacyMode ? privateToggleLabel : toggleLabel}>
            <ChevronRight size={13} className="oa-project-chevron" aria-hidden="true"/><b title={displayName}>{displayName}</b><small>{group.sessions.length}</small>
          </button>
          {group.pinned && <span className="oa-project-pinned-badge" title={ct('项目已置顶', 'Project pinned')}><Pin size={11} aria-hidden="true"/>{ct('置顶', 'Pinned')}</span>}
          {projectSortMode && (
            <ProjectDragHandle name={projectKey} groups={projectSessionGroups.length ? projectSessionGroups : projectGroups} disabled={batchDeleting || projectOrderSaving} onReorder={onProjectOrder} label={reorderLabel}/>
          )}
          <button className="oa-project-add" type="button" onClick={() => onNewProjectSession?.(group.provider ? group : group.name)} disabled={batchDeleting} title={privacyMode ? privateNewLabel : newLabel} aria-label={privacyMode ? privateNewLabel : newLabel}><span aria-hidden="true">+</span></button>
          {!privacyMode && <ProjectActionsMenu label={ct('项目操作', 'Project actions')}>
            <button className={`oa-project-pin ${group.pinned ? 'is-pinned' : ''}`} type="button" onClick={() => onToggleProjectPinned?.(projectKey, !group.pinned)} aria-pressed={group.pinned} title={pinLabel} aria-label={pinLabel}><Pin size={14} aria-hidden="true"/>{pinLabel}</button>
            <button type="button" onClick={() => onOpenProjectFolder?.(group)} title={folderLabel}><FolderOpen size={14} aria-hidden="true"/>{folderLabel}</button>
          </ProjectActionsMenu>}
        </div>
        <div className="oa-project-body" id={bodyId} hidden={!expanded}>
          {group.sessions.map(row)}
          {!group.sessions.length && <div className="oa-project-empty">{ct('暂无对话，点击 + 快速开始', 'No chats yet. Click + to start.')}</div>}
        </div>
      </section>
    })}
    {!projectGroups.length && <div className="oa-empty-list oa-projects-empty"><FolderOpen size={20} aria-hidden="true"/><span>{sidebarSearch ? ct('无匹配项目', 'No matching projects') : ct('暂无可用项目', 'No projects available')}</span></div>}
  </div>

  return <div className="oa-session-list" aria-label={ct('历史会话', 'Session history')} onKeyDown={moveSessionFocus}>
    {recentGroups.map(group => <section className={`oa-recent-group oa-recent-group-${group.key}`} key={group.key}>
      <div className="oa-recent-group-head">{group.key === 'pinned' && <Pin size={12} aria-hidden="true"/>}<span>{recentGroupLabels[group.key]}</span><small>{group.sessions.length}</small></div>
      <div className="oa-recent-group-body">{group.sessions.map(row)}</div>
    </section>)}
    {!recentGroups.length && <div className="oa-empty-list">{sidebarSearch ? ct('无匹配会话', 'No matching sessions') : ct('暂无历史会话', 'No session history')}</div>}
  </div>
}
