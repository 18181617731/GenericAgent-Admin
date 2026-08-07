// @vitest-environment jsdom

import React from 'react'
import { afterEach, describe, expect, test, vi } from 'vitest'
import { cleanup, fireEvent, render, screen } from '@testing-library/react'
import SessionSearchDialog from './SessionSearchDialog.jsx'

afterEach(() => cleanup())

const scopes = [
  { value: 'all', label: '全部' },
  { value: 'title', label: '标题' },
  { value: 'content', label: '消息内容' },
  { value: 'project', label: '项目' },
]

test('search dialog supports scope, history, results, and escape close', () => {
  const onQueryChange = vi.fn()
  const onScopeChange = vi.fn()
  const onSelectHistory = vi.fn()
  const onSelectSession = vi.fn()
  const onClose = vi.fn()
  const view = render(<SessionSearchDialog
    open
    query=""
    scope="all"
    scopes={scopes}
    history={[{ query: '上游同步', scope: 'title' }]}
    recentSessions={[{ id: 'session-1', title: '最近会话', count: 3, updated_at: 1710000000 }]}
    results={[]}
    currentSessionID=""
    onQueryChange={onQueryChange}
    onScopeChange={onScopeChange}
    onSelectHistory={onSelectHistory}
    onSelectSession={onSelectSession}
    onClose={onClose}
  />)

  expect(screen.getByRole('dialog', { name: '搜索会话' })).toBeTruthy()
  fireEvent.change(screen.getByRole('searchbox'), { target: { value: '项目' } })
  expect(onQueryChange).toHaveBeenCalledWith('项目')
  fireEvent.click(screen.getByRole('tab', { name: '项目' }))
  expect(onScopeChange).toHaveBeenCalledWith('project')
  fireEvent.click(screen.getByRole('button', { name: /上游同步/ }))
  expect(onSelectHistory).toHaveBeenCalledWith({ query: '上游同步', scope: 'title' })

  view.rerender(<SessionSearchDialog
    open
    query="模型"
    scope="all"
    scopes={scopes}
    history={[]}
    recentSessions={[]}
    results={[{ id: 'session-1', title: '模型切换', count: 3, updated_at: 1710000000, snippet: '已经完成模型切换', match_type: 'content', match_types: ['content'] }]}
    currentSessionID=""
    onQueryChange={onQueryChange}
    onScopeChange={onScopeChange}
    onSelectHistory={onSelectHistory}
    onSelectSession={onSelectSession}
    onClose={onClose}
  />)
  fireEvent.click(screen.getByRole('button', { name: /模型切换/ }))
  expect(onSelectSession).toHaveBeenCalledWith('session-1')
  fireEvent.keyDown(document, { key: 'Escape' })
  expect(onClose).toHaveBeenCalledTimes(1)
})
