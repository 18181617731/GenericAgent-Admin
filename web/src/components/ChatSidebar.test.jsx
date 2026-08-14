// @vitest-environment jsdom

import React, { useState } from 'react'
import { afterEach, describe, expect, test, vi } from 'vitest'
import { cleanup, fireEvent, render, screen, waitFor, within } from '@testing-library/react'
import ChatSidebar from './ChatSidebar.jsx'

afterEach(() => cleanup())

const copy = (zh) => zh
const formatTime = () => '今天 10:30'
const sessions = [{
  id: 'session-1',
  title: '发布准备',
  updated_at: '2026-08-14T10:30:00Z',
  count: 4,
  running: true,
  pinned: true,
  hub_enabled: true,
  loop: { enabled: true, status: 'running', round: 2, max_rounds: 5 },
}]

const recentGroups = [{ key: 'today', sessions }]
const projectGroups = [{ name: 'GenericAgent', sessions }]

const baseProps = (overrides = {}) => ({
  ct: copy,
  chatInstanceID: 'default',
  chatInstances: [{ id: 'default', name: 'Default' }, { id: 'staging', name: 'Staging' }],
  recentGroups,
  projectGroups,
  recentGroupLabels: { today: '今天' },
  sessions,
  onSwitchChatInstance: vi.fn(),
  onNewSession: vi.fn(),
  onOpenSearch: vi.fn(),
  onSidebarTabChange: vi.fn(),
  onSelectSession: vi.fn(),
  onOpenMenu: vi.fn(),
  onOpenSessionManager: vi.fn(),
  onToggleProject: vi.fn(),
  onNewProjectSession: vi.fn(),
  ...overrides,
})

describe('ChatSidebar', () => {
  test('should expose instance, new-chat, and search actions through user semantics', () => {
    const props = baseProps()
    render(<ChatSidebar {...props} />)

    fireEvent.change(screen.getByRole('combobox', { name: '选择 GA 实例' }), { target: { value: 'staging' } })
    fireEvent.click(screen.getByRole('button', { name: '新对话' }))
    fireEvent.click(screen.getByRole('button', { name: '搜索聊天' }))

    expect(props.onSwitchChatInstance).toHaveBeenCalledWith('staging')
    expect(props.onNewSession).toHaveBeenCalledTimes(1)
    expect(props.onOpenSearch).toHaveBeenCalledTimes(1)
    expect(screen.getByText('Ctrl/Cmd+K')).toBeTruthy()
  })

  test('should switch between sessions and projects tabs and create a project chat', () => {
    const props = baseProps()
    function ProjectHarness() {
      const [tab, setTab] = useState('history')
      const [expanded, setExpanded] = useState(new Set())
      const toggle = name => setExpanded(current => {
        const next = new Set(current)
        if (next.has(name)) next.delete(name)
        else next.add(name)
        return next
      })
      return <ChatSidebar
        {...props}
        sidebarTab={tab}
        expandedProjectNames={expanded}
        onSidebarTabChange={setTab}
        onToggleProject={toggle}
      />
    }

    render(<ProjectHarness />)

    const sessionsTab = screen.getByRole('tab', { name: '会话' })
    const projectsTab = screen.getByRole('tab', { name: '项目' })
    expect(sessionsTab.getAttribute('aria-selected')).toBe('true')
    expect(projectsTab.getAttribute('aria-selected')).toBe('false')

    fireEvent.click(projectsTab)
    expect(projectsTab.getAttribute('aria-selected')).toBe('true')
    const expand = screen.getByRole('button', { name: '展开 GenericAgent' })
    fireEvent.click(expand)
    fireEvent.click(screen.getByRole('button', { name: '在 GenericAgent 中新建对话' }))

    expect(expand.getAttribute('aria-expanded')).toBe('true')
    expect(props.onNewProjectSession).toHaveBeenCalledWith('GenericAgent')
  })

  test('should keep title and metadata on separate accessible lines', () => {
    render(<ChatSidebar {...baseProps()} formatTime={formatTime} />)

    const sessionButton = screen.getByRole('button', { name: /发布准备/ })
    const title = within(sessionButton).getByText('发布准备')
    const metadata = within(sessionButton).getByText('4 条')

    expect(title.parentElement?.classList.contains('oa-session-title')).toBe(true)
    expect(metadata.closest('.oa-session-meta')).toBeTruthy()
    expect(within(sessionButton).getByText('今天 10:30')).toBeTruthy()
    expect(within(sessionButton).getByText('运行中')).toBeTruthy()
    expect(within(sessionButton).getByText('Loop 2/5')).toBeTruthy()
    expect(within(sessionButton).getByText('Hub')).toBeTruthy()
  })

  test('should select a session and preserve row menu actions', () => {
    const menuSession = { ...sessions[0], running: false }
    const props = baseProps({
      menuOpen: 'session-1',
      menuPos: { top: 10, left: 10 },
      menuSession,
      onStartRename: vi.fn(),
      onSetPinned: vi.fn(),
      onForkSession: vi.fn(),
      onArchiveSession: vi.fn(),
      onSetHubEnabled: vi.fn(),
      onDeleteSession: vi.fn(),
    })
    render(<ChatSidebar {...props} />)

    fireEvent.click(screen.getByRole('button', { name: /发布准备/ }))
    fireEvent.click(screen.getByRole('button', { name: '会话操作' }))
    const menu = screen.getByRole('menu', { name: '会话操作' })
    expect(within(menu).getAllByRole('menuitem').map(item => item.textContent)).toEqual([
      '重命名',
      '取消置顶',
      '分支到新会话',
      '归档',
      '退出 Hub',
      '删除',
    ])

    fireEvent.click(within(menu).getByRole('menuitem', { name: '重命名' }))
    fireEvent.click(within(menu).getByRole('menuitem', { name: '取消置顶' }))
    fireEvent.click(within(menu).getByRole('menuitem', { name: '分支到新会话' }))
    fireEvent.click(within(menu).getByRole('menuitem', { name: '归档' }))
    fireEvent.click(within(menu).getByRole('menuitem', { name: '退出 Hub' }))
    fireEvent.click(within(menu).getByRole('menuitem', { name: '删除' }))

    expect(props.onSelectSession).toHaveBeenCalledWith('session-1')
    expect(props.onOpenMenu).toHaveBeenCalledWith(sessions[0], expect.anything())
    expect(props.onStartRename).toHaveBeenCalledWith(menuSession)
    expect(props.onSetPinned).toHaveBeenCalledWith(menuSession)
    expect(props.onForkSession).toHaveBeenCalledWith(menuSession)
    expect(props.onArchiveSession).toHaveBeenCalledWith('session-1')
    expect(props.onSetHubEnabled).toHaveBeenCalledWith(menuSession)
    expect(props.onDeleteSession).toHaveBeenCalledWith('session-1')
  })

  test('should disable archiving while a session is running', () => {
    const props = baseProps({
      menuOpen: 'session-1',
      menuPos: { top: 10, left: 10 },
      menuSession: sessions[0],
      onArchiveSession: vi.fn(),
    })
    render(<ChatSidebar {...props} />)

    const archive = screen.getByRole('menuitem', { name: '运行中，暂不可归档' })
    expect(archive.disabled).toBe(true)
    expect(archive.getAttribute('title')).toBe('运行中，暂不可归档')
    expect(archive.getAttribute('aria-label')).toBe('运行中，暂不可归档')

    fireEvent.click(archive)
    expect(props.onArchiveSession).not.toHaveBeenCalled()
  })

  test('should disable every session menu action while an action is processing', () => {
    const props = baseProps({
      menuOpen: 'session-1',
      menuPos: { top: 10, left: 10 },
      menuSession: sessions[0],
      sessionActionID: 'session-1',
      onStartRename: vi.fn(),
      onSetPinned: vi.fn(),
      onForkSession: vi.fn(),
      onArchiveSession: vi.fn(),
      onSetHubEnabled: vi.fn(),
      onDeleteSession: vi.fn(),
    })
    render(<ChatSidebar {...props} />)

    const menu = screen.getByRole('menu', { name: '会话操作' })
    const menuItems = within(menu).getAllByRole('menuitem')
    expect(menu.getAttribute('aria-busy')).toBe('true')
    expect(menuItems).toHaveLength(6)
    expect(menuItems.every(item => item.disabled)).toBe(true)
  })

  test('should move focus through sessions with arrow and boundary keys', () => {
    const second = { ...sessions[0], id: 'session-2', title: '回归检查', running: false }
    render(<ChatSidebar {...baseProps({
      sessions: [...sessions, second],
      recentGroups: [{ key: 'today', sessions: [...sessions, second] }],
    })} />)

    const firstButton = screen.getByRole('button', { name: /发布准备/ })
    const secondButton = screen.getByRole('button', { name: /回归检查/ })
    firstButton.focus()
    fireEvent.keyDown(firstButton, { key: 'ArrowDown' })
    expect(document.activeElement).toBe(secondButton)
    fireEvent.keyDown(secondButton, { key: 'Home' })
    expect(document.activeElement).toBe(firstButton)
    fireEvent.keyDown(firstButton, { key: 'End' })
    expect(document.activeElement).toBe(secondButton)
  })

  test('should cycle focus through enabled menu actions with arrow keys', () => {
    const menuSession = { ...sessions[0], running: false }
    render(<ChatSidebar {...baseProps({
      menuOpen: menuSession.id,
      menuPos: { top: 10, left: 10 },
      menuSession,
    })} />)

    const menu = screen.getByRole('menu', { name: '会话操作' })
    const items = within(menu).getAllByRole('menuitem')
    items[0].focus()
    fireEvent.keyDown(items[0], { key: 'ArrowUp' })
    expect(document.activeElement).toBe(items.at(-1))
    fireEvent.keyDown(items.at(-1), { key: 'ArrowDown' })
    expect(document.activeElement).toBe(items[0])
  })

  test('should collapse the mobile sidebar after selecting a session', () => {
    function MobileHarness() {
      const [collapsed, setCollapsed] = useState(false)
      return <ChatSidebar
        {...baseProps()}
        collapsed={collapsed}
        onCollapse={() => setCollapsed(true)}
        onSelectSession={() => setCollapsed(true)}
      />
    }

    render(<MobileHarness />)
    fireEvent.click(screen.getByRole('button', { name: /发布准备/ }))

    expect(document.getElementById('oa-chat-sidebar')?.classList.contains('collapsed')).toBe(true)
  })

  test('should close the mobile drawer with Escape and restore page scrolling', async () => {
    const originalMatchMedia = window.matchMedia
    window.matchMedia = vi.fn(() => ({ matches: true }))
    function EscapeHarness() {
      const [collapsed, setCollapsed] = useState(true)
      return <>
        <button className="oa-sidebar-toggle" type="button" onClick={() => setCollapsed(false)}>展开侧栏</button>
        <ChatSidebar {...baseProps()} collapsed={collapsed} onCollapse={() => setCollapsed(true)} />
      </>
    }

    render(<EscapeHarness />)
    const trigger = screen.getByRole('button', { name: '展开侧栏' })
    trigger.focus()
    fireEvent.click(trigger)
    expect(document.body.style.overflow).toBe('hidden')
    fireEvent.keyDown(document, { key: 'Escape' })

    expect(document.getElementById('oa-chat-sidebar')?.getAttribute('aria-hidden')).toBe('true')
    expect(document.body.style.overflow).toBe('')
    await waitFor(() => expect(document.activeElement).toBe(trigger))
    window.matchMedia = originalMatchMedia
  })
})
