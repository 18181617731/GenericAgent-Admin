// @vitest-environment jsdom

import React from 'react'
import { afterEach, describe, expect, test, vi } from 'vitest'
import { cleanup, fireEvent, render, screen, waitFor } from '@testing-library/react'
import { AutonomousTaskWorkspace } from './AutonomousTaskWorkspace.jsx'
import { registerDialogAdapter } from '../lib/danger.js'

const tasks = [
  { id: 'task-1', title: '等待批准', objective: '检查发布证据', status: 'pending_approval', risk: '高', priority: 'high', progress: 20, source_type: 'todo' },
  { id: 'task-2', title: '失败任务', objective: '重新运行验证', status: 'failed', progress: 60, source_type: 'autonomous_report' },
]

const response = body => Promise.resolve({ ok: true, text: () => Promise.resolve(JSON.stringify(body)) })

let unregisterDialogAdapter = () => {}

afterEach(() => {
  unregisterDialogAdapter()
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

  test('keeps headline counts stable while filtering and renders completed work as 100%', async () => {
    const allTasks = [
      { id: 'completed', title: '已完成任务', objective: '已完成目标', status: 'completed', progress: 0 },
      { id: 'pending', title: '待审批任务', objective: '待审批目标', status: 'pending_approval', progress: 20 },
      { id: 'running', title: '执行中任务', objective: '执行中目标', status: 'queued', progress: 40 },
    ]
    const summary = { total: 3, pending: 1, running: 1, blocked: 0, failed: 0, overdue: 0, completed: 1, attention: 0 }
    const fetchMock = vi.fn(url => String(url).includes('status=completed')
      ? response({ tasks: [allTasks[0]], filtered_total: 1, summary })
      : response({ tasks: allTasks, filtered_total: 3, summary }))
    vi.stubGlobal('fetch', fetchMock)
    render(<AutonomousTaskWorkspace />)

    expect(await screen.findByText('已完成任务')).toBeTruthy()
    expect(screen.getByText('显示 3 / 共 3 项')).toBeTruthy()
    expect(screen.getByText('100%')).toBeTruthy()
    fireEvent.click(screen.getAllByRole('button', { name: /已完成\s*1/ })[0])

    await waitFor(() => expect(screen.getByText('显示 1 / 共 3 项')).toBeTruthy())
    expect(screen.getByText('已完成任务')).toBeTruthy()
    expect(screen.queryByText('待审批任务')).toBeNull()
    expect(screen.getByRole('button', { name: /待审批\s*1/ })).toBeTruthy()
  })

  test('creates a task with explicit confirmation and dangerous header', async () => {
    const created = { id: 'task-new', title: '新任务', status: 'draft' }
    const fetchMock = vi.fn((url, options = {}) => {
      if (url === '/api/autonomous/tasks' && options.method === 'POST') return response({ ok: true, task: created })
      if (url.includes('/task-new')) return response({ task: created, runs: [], events: [] })
      return response({ tasks: [] })
    })
    vi.stubGlobal('fetch', fetchMock)
    const dialog = vi.fn(() => true)
    unregisterDialogAdapter = registerDialogAdapter(dialog)
    render(<AutonomousTaskWorkspace />)

    await screen.findByText(/暂无任务/)
    fireEvent.click(screen.getByRole('button', { name: '新建任务' }))
    fireEvent.change(screen.getByLabelText('标题'), { target: { value: '新任务' } })
    fireEvent.click(screen.getByRole('button', { name: '保存任务' }))

    await waitFor(() => expect(fetchMock.mock.calls.some(([url, options]) => url === '/api/autonomous/tasks' && options?.method === 'POST' && options.headers['X-GA-Confirm'] === 'dangerous')).toBe(true))
    expect(dialog).toHaveBeenCalledWith(expect.objectContaining({
      kind: 'confirm',
      operation: 'autonomous-task-create',
      message: expect.stringContaining('创建任务“新任务”？'),
    }))
  })
})
