import React, { useEffect, useRef } from 'react'
import { Archive, Bot, ChevronLeft, Edit3, EyeOff, FolderPlus, GitFork, MessageSquarePlus, PanelLeftClose, Pin, Search, Settings, ShieldCheck, Trash2 } from 'lucide-react'
import ChatSessionList from './ChatSessionList.jsx'

const defaultCopy = (zh) => zh
const EMPTY_SET = new Set()

export default function ChatSidebar({
  collapsed = false,
  ct = defaultCopy,
  chatInstanceID = '',
  chatInstances = [],
  chatInstancesLoading = false,
  onSwitchChatInstance,
  privacyMode = false,
  onPrivacyModeChange,
  onCollapse,
  onNewSession,
  onOpenSearch,
  searchTriggerRef,
  sidebarTab = 'history',
  onSidebarTabChange,
  projectDraftOpen = false,
  projectDraftName = '',
  projectCreating = false,
  onOpenProjectDraft,
  onCloseProjectDraft,
  onProjectDraftNameChange,
  onCreateProject,
  sessions = [],
  projectGroups = [],
  recentGroups = [],
  recentGroupLabels = {},
  sidebarSearch = '',
  expandedProjectNames = EMPTY_SET,
  activeSessionID = '',
  editingSessionID = '',
  draftTitle = '',
  draftSessionIds = EMPTY_SET,
  menuOpen = '',
  menuPos = null,
  menuSession = null,
  menuRef,
  sessionManagerOpen = false,
  batchDeleting = false,
  onOpenSessionManager,
  onToggleProject,
  onToggleProjectPinned,
  onNewProjectSession,
  onSelectSession,
  onDraftTitleChange,
  onSaveRename,
  onCancelRename,
  onOpenMenu,
  onStartRename,
  onSetPinned,
  onForkSession,
  onArchiveSession,
  onSetHubEnabled,
  onDeleteSession,
  sessionActionID = '',
  formatTime,
}) {
  const sessionTabLabel = ct('会话', 'Sessions')
  const historyLabel = ct('历史会话', 'History')
  const projectTabLabel = ct('项目', 'Projects')
  const collapseLabel = ct('折叠侧栏', 'Collapse sidebar')
  const searchLabel = ct('搜索聊天', 'Search chats')
  const newChatLabel = ct('新对话', 'New chat')
  const privacyLabel = ct('精简显示', 'Compact view')
  const privacyState = privacyMode ? ct('开', 'On') : ct('关', 'Off')
  const privacyTitleByID = new Map(sessions.map((session, index) => [session.id, ct(`会话 ${String(index + 1).padStart(2, '0')}`, `Session ${String(index + 1).padStart(2, '0')}`)]))
  const menuActionBusy = Boolean(menuSession && sessionActionID === menuSession.id)
  const archiveBlockedByRunning = Boolean(menuSession?.running)
  const archiveLabel = archiveBlockedByRunning ? ct('运行中，暂不可归档', 'Running; archiving unavailable') : ct('归档', 'Archive')
  const sidebarItemCount = sidebarTab === 'projects' ? projectGroups.length : sessions.length
  const collapseButtonRef = useRef(null)
  const sidebarRef = useRef(null)
  const previousFocusRef = useRef(null)

  useEffect(() => {
    const mobile = window.matchMedia?.('(max-width: 900px)')?.matches
    if (collapsed || !mobile) return undefined
    previousFocusRef.current = document.activeElement
    const previousBodyOverflow = document.body.style.overflow
    document.body.style.overflow = 'hidden'
    window.requestAnimationFrame?.(() => collapseButtonRef.current?.focus())
    const handleDrawerKeyDown = event => {
      if (event.key === 'Escape') {
        event.preventDefault()
        onCollapse?.()
        return
      }
      if (event.key !== 'Tab') return
      const focusable = [...(sidebarRef.current?.querySelectorAll('button:not(:disabled), select:not(:disabled), input:not(:disabled), [tabindex]:not([tabindex="-1"])') || [])]
      if (!focusable.length) return
      const first = focusable[0]
      const last = focusable.at(-1)
      if (event.shiftKey && document.activeElement === first) {
        event.preventDefault()
        last.focus()
      } else if (!event.shiftKey && document.activeElement === last) {
        event.preventDefault()
        first.focus()
      }
    }
    document.addEventListener('keydown', handleDrawerKeyDown)
    return () => {
      document.removeEventListener('keydown', handleDrawerKeyDown)
      document.body.style.overflow = previousBodyOverflow
      const restoreFocus = () => {
        const previous = previousFocusRef.current
        const fallback = document.querySelector('.oa-sidebar-toggle')
        const target = previous?.isConnected ? previous : fallback
        target?.focus?.()
        if (document.activeElement === document.body) fallback?.focus?.()
      }
      if (window.requestAnimationFrame) {
        window.requestAnimationFrame(() => window.requestAnimationFrame(restoreFocus))
      }
      else restoreFocus()
    }
  }, [collapsed, onCollapse])

  useEffect(() => {
    if (!menuOpen) return
    window.requestAnimationFrame?.(() => menuRef?.current?.querySelector('button:not(:disabled)')?.focus())
  }, [menuOpen, menuRef])

  const onMenuKeyDown = event => {
    if (!['ArrowDown', 'ArrowUp', 'Home', 'End'].includes(event.key)) return
    const items = [...event.currentTarget.querySelectorAll('[role="menuitem"]:not(:disabled)')]
    if (!items.length) return
    const current = items.indexOf(document.activeElement)
    let next = current
    if (event.key === 'Home') next = 0
    else if (event.key === 'End') next = items.length - 1
    else if (event.key === 'ArrowDown') next = current < 0 ? 0 : (current + 1) % items.length
    else next = current < 0 ? items.length - 1 : (current - 1 + items.length) % items.length
    event.preventDefault()
    items[next]?.focus()
  }

  return <>
    <aside ref={sidebarRef} id="oa-chat-sidebar" className={`oa-sidebar ${collapsed ? 'collapsed' : ''}`} aria-label={ct('聊天侧栏', 'Chat sidebar')} aria-hidden={collapsed || undefined}>
      <header className="oa-sidebar-heading">
        <div className="oa-sidebar-brand">
          <span className="oa-sidebar-brand-mark" aria-hidden="true"><Bot size={17}/></span>
          <span className="oa-sidebar-brand-copy"><b>GA 实例</b><small>GenericAgent</small></span>
        </div>
        <button ref={collapseButtonRef} className="oa-sidebar-collapse" type="button" onClick={onCollapse} title={collapseLabel} aria-label={collapseLabel}><PanelLeftClose size={17} aria-hidden="true"/></button>
      </header>

      <label className="oa-sidebar-instance" title={ct('切换实例会更新当前侧栏中的会话', 'Switching instances updates the sessions in this sidebar')}>
        <span>{ct('当前实例', 'Current instance')}</span>
        <select aria-label={ct('选择 GA 实例', 'Select GA instance')} value={chatInstanceID} onChange={event => onSwitchChatInstance?.(event.target.value)} disabled={chatInstancesLoading || !chatInstances.length}>
          {chatInstancesLoading && <option value={chatInstanceID}>{ct('加载实例…', 'Loading instances…')}</option>}
          {!chatInstancesLoading && !chatInstances.length && <option value="">{ct('默认实例', 'Default instance')}</option>}
          {chatInstances.map(instance => <option key={instance.id} value={instance.id} disabled={instance.initializing}>{instance.name}{instance.initializing ? ct('（初始化中）', ' (initializing)') : ''}</option>)}
        </select>
      </label>

      <button className={`oa-sidebar-privacy ${privacyMode ? 'is-on' : 'is-off'}`} type="button" role="switch" aria-checked={privacyMode} onClick={() => onPrivacyModeChange?.(!privacyMode)} title={privacyLabel}>
        <span className="oa-sidebar-privacy-icon" aria-hidden="true">{privacyMode ? <ShieldCheck size={16}/> : <EyeOff size={16}/>}</span>
        <span className="oa-sidebar-privacy-copy"><b>{privacyLabel}</b><small>{privacyState}</small></span>
        <span className="oa-sidebar-privacy-track" aria-hidden="true"><i/></span>
      </button>

      <button className="oa-new-chat oa-sidebar-wide-action" type="button" onClick={onNewSession} disabled={batchDeleting} title={newChatLabel} aria-label={newChatLabel}><MessageSquarePlus size={16} aria-hidden="true"/><span>{newChatLabel}</span></button>
      <button className="oa-sidebar-search-action" type="button" onClick={onOpenSearch} ref={searchTriggerRef} disabled={privacyMode} title={privacyMode ? ct('当前视图不可搜索', 'Search unavailable in the current view') : searchLabel} aria-label={privacyMode ? ct('当前视图不可搜索聊天', 'Chat search unavailable in the current view') : searchLabel} aria-haspopup="dialog"><Search size={16} aria-hidden="true"/><span>{searchLabel}</span><kbd>Ctrl/Cmd+K</kbd></button>

      <div className="oa-sidebar-tabs" role="tablist" aria-label={ct('聊天内容', 'Chat content')}>
        <button type="button" role="tab" aria-selected={sidebarTab === 'history'} className={sidebarTab === 'history' ? 'is-active' : ''} onClick={() => onSidebarTabChange?.('history')}>{sessionTabLabel}</button>
        <button type="button" role="tab" aria-selected={sidebarTab === 'projects'} className={sidebarTab === 'projects' ? 'is-active' : ''} onClick={() => onSidebarTabChange?.('projects')}>{projectTabLabel}</button>
      </div>

      <div className="oa-session-manager-head">
        <span className="oa-session-manager-title">{sidebarTab === 'projects' ? projectTabLabel : historyLabel} <small>{sidebarItemCount}</small></span>
        <span className="oa-session-manager-actions">
          {sidebarTab === 'projects' && <button className="oa-session-manage-open" type="button" onClick={onOpenProjectDraft} disabled={privacyMode || projectCreating || projectDraftOpen} title={privacyMode ? ct('当前视图不可新建项目', 'Projects cannot be created in the current view') : undefined}><FolderPlus size={13} aria-hidden="true"/>{ct('新建', 'New')}</button>}
          <button className="oa-session-manage-open" type="button" onClick={onOpenSessionManager} disabled={!sessions.length || batchDeleting}>{ct('管理', 'Manage')}</button>
        </span>
      </div>

      {!privacyMode && sidebarTab === 'projects' && projectDraftOpen && <form className="oa-project-draft" onSubmit={event => { event.preventDefault(); onCreateProject?.() }}>
        <input
          autoFocus
          type="text"
          value={projectDraftName}
          onChange={event => onProjectDraftNameChange?.(event.target.value)}
          onKeyDown={event => { if (event.key === 'Escape') { event.preventDefault(); onCloseProjectDraft?.() } }}
          placeholder={ct('项目名，例如 alpha', 'Project name, e.g. alpha')}
          aria-label={ct('新项目名称', 'New project name')}
          disabled={projectCreating}
        />
        <button className="oa-project-draft-save" type="submit" disabled={projectCreating || !projectDraftName.trim()}>{projectCreating ? ct('创建中…', 'Creating…') : ct('创建', 'Create')}</button>
        <button type="button" onClick={onCloseProjectDraft} disabled={projectCreating}>{ct('取消', 'Cancel')}</button>
      </form>}

      <ChatSessionList
        tab={sidebarTab}
        recentGroups={recentGroups}
        projectGroups={projectGroups}
        recentGroupLabels={recentGroupLabels}
        sidebarSearch={sidebarSearch}
        expandedProjectNames={expandedProjectNames}
        draftSessionIds={draftSessionIds}
        activeSessionID={activeSessionID}
        editingSessionID={editingSessionID}
        draftTitle={draftTitle}
        menuOpen={menuOpen}
        privacyMode={privacyMode}
        privacyTitleByID={privacyTitleByID}
        ct={ct}
        formatTime={formatTime}
        onSelectSession={onSelectSession}
        onDraftTitleChange={onDraftTitleChange}
        onSaveRename={onSaveRename}
        onCancelRename={onCancelRename}
        onOpenMenu={onOpenMenu}
        onToggleProject={onToggleProject}
        onToggleProjectPinned={onToggleProjectPinned}
        onNewProjectSession={onNewProjectSession}
        batchDeleting={batchDeleting}
      />

      {!sessionManagerOpen && menuOpen && menuPos && menuSession && <div ref={menuRef} className="oa-session-menu" style={{ top: menuPos.top, left: menuPos.left }} role="menu" aria-label={ct('会话操作', 'Session actions')} aria-busy={menuActionBusy || undefined} onClick={event => event.stopPropagation()} onKeyDown={onMenuKeyDown}>
        <button type="button" role="menuitem" disabled={menuActionBusy || privacyMode} title={privacyMode ? ct('当前视图不可重命名', 'Rename unavailable in the current view') : undefined} onClick={() => onStartRename?.(menuSession)}><Edit3 size={14} aria-hidden="true"/>{ct('重命名', 'Rename')}</button>
        <button type="button" role="menuitem" disabled={menuActionBusy} onClick={() => onSetPinned?.(menuSession)}><Pin size={14} aria-hidden="true"/>{menuSession.pinned ? ct('取消置顶', 'Unpin') : ct('置顶', 'Pin')}</button>
        <button type="button" role="menuitem" disabled={menuActionBusy} onClick={() => onForkSession?.(menuSession)}><GitFork size={14} aria-hidden="true"/>{ct('分支到新会话', 'Fork to new session')}</button>
        <button type="button" role="menuitem" disabled={menuActionBusy || archiveBlockedByRunning} title={archiveLabel} aria-label={archiveLabel} onClick={() => onArchiveSession?.(menuSession.id)}><Archive size={14} aria-hidden="true"/>{ct('归档', 'Archive')}</button>
        <button type="button" role="menuitem" disabled={menuActionBusy} onClick={() => onSetHubEnabled?.(menuSession)}><Bot size={14} aria-hidden="true"/>{menuSession.hub_enabled ? ct('退出 Hub', 'Leave Hub') : ct('入驻 Hub', 'Join Hub')}</button>
        <button className="danger" type="button" role="menuitem" disabled={menuActionBusy} onClick={() => onDeleteSession?.(menuSession.id)}><Trash2 size={14} aria-hidden="true"/>{ct('删除', 'Delete')}</button>
      </div>}

      <div className="oa-sidebar-foot">
        <button type="button" onClick={() => { window.location.href = '/' }}><ChevronLeft size={15} aria-hidden="true"/>{ct('返回管理台', 'Back to admin')}</button>
        <button type="button" onClick={() => { window.location.href = '/admin' }}><Settings size={15} aria-hidden="true"/>{ct('设置', 'Settings')}</button>
      </div>
    </aside>
    {!collapsed && <button className="oa-sidebar-backdrop" type="button" aria-label={ct('关闭侧栏', 'Close sidebar')} onClick={onCollapse}/>}
  </>
}
