// @vitest-environment jsdom

import React from 'react'
import { afterEach, describe, expect, test, vi } from 'vitest'
import { cleanup, fireEvent, render, screen, waitFor } from '@testing-library/react'
import { AutonomousTaskWorkspace } from './AutonomousTaskWorkspace.jsx'

const tasks = [
  { id: 'task-1', title: '等待批准', objective: '检查发布证据', status: 'pending_approval', risk: '高', priority: 'high', progress: 20, source_type: 'todo' },
  { id: 'task-2', title: '失败任务', objective: '重新运行验证', status: 'failed', progress: 60, source_type: 'autonomous_report' },
]

const response = body => Promise.resolve({ ok: true, text: () => Promise.resolve(JSON.stringify(body)) })

afterEach(() => {
  cleanup()
  vi.unstubAllGlobals()
  vi.restoreAllMocks()
})

describe('AutonomousTaskWorkspace', () => {
  test('loads metrics, filters, and task details', async () => {
    const fetchMock = vi.fn((url) => url.includes('/task-1')
      ? response({ task: tasks[0], runs: [], events: [{ id: 'event-1', type: 'created', message: '任务已创建', created_at: '2026-08-19T00:00:00Z' }] })
      : response({ tasks }))
    vi.stubGlobal('fetch', fetchMock)
    render(<AutonomousTaskWorkspace />)

    expect(await screen.findByText('等待批准')).toBeTruthy()
    expect(screen.getByText('失败任务')).toBeTruthy()
    expect(screen.getByRole('combobox', { name: '任务状态' })).toBeTruthy()
    fireEvent.click(screen.getByRole('button', { name: /等待批准/ }))
    expect(await screen.findByText('任务已创建')).toBeTruthy()
    expect(fetchMock).toHaveBeenCalledWith('/api/autonomous/tasks/task-1', expect.any(Object))
  })

  test('creates a task with explicit confirmation and dangerous header', async () => {
    const created = { id: 'task-new', title: '新任务', status: 'draft' }
    const fetchMock = vi.fn((url, options = {}) => {
      if (url === '/api/autonomous/tasks' && options.method === 'POST') return response({ ok: true, task: created })
      if (url.includes('/task-new')) return response({ task: created, runs: [], events: [] })
      return response({ tasks: [] })
    })
    vi.stubGlobal('fetch', fetchMock)
    vi.stubGlobal('confirm', vi.fn(() => true))
    render(<AutonomousTaskWorkspace />)

    await screen.findByText(/暂无任务/)
    fireEvent.click(screen.getByRole('button', { name: '新建任务' }))
    fireEvent.change(screen.getByLabelText('标题'), { target: { value: '新任务' } })
    fireEvent.click(screen.getByRole('button', { name: '保存任务' }))

    await waitFor(() => expect(fetchMock.mock.calls.some(([url, options]) => url === '/api/autonomous/tasks' && options?.method === 'POST' && options.headers['X-GA-Confirm'] === 'dangerous')).toBe(true))
    expect(globalThis.confirm).toHaveBeenCalledWith(expect.stringContaining('[autonomous-task-create]'))
  })
})
