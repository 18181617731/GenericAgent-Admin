// @vitest-environment jsdom

import React from 'react'
import { afterEach, describe, expect, test, vi } from 'vitest'
import { cleanup, fireEvent, render, screen, waitFor } from '@testing-library/react'
import { AutonomousTaskWorkspace } from './AutonomousTaskWorkspace.jsx'
import { registerDialogAdapter } from '../lib/danger.js'

const tasks = [
  { id: 'task-1', title: '等待批准', objective: '检查发布证据', status: 'pending_approval', risk: '高', priority: 'high', progress: 20, source_type: 'todo' },
  { id: 'task-2', title: '排队任务', objective: '等待执行', status: 'queued', progress: 0, source_type: 'todo' },
  { id: 'task-3', title: '已闭环任务', objective: '验证已完成', status: 'completed', progress: 100, source_type: 'todo' },
]

const response = body => Promise.resolve({ ok: true, text: () => Promise.resolve(JSON.stringify(body)) })

afterEach(() => {
  cleanup()
  unregisterDialogAdapter()
  vi.unstubAllGlobals()
  vi.restoreAllMocks()
})

let unregisterDialogAdapter = () => {}

describe('AutonomousTaskWorkspace', () => {
  test('loads metrics, filters, and task details', async () => {
    const fetchMock = vi.fn((url) => url.includes('/task-1')
      ? response({ task: tasks[0], runs: [], events: [{ id: 'event-1', type: 'created', message: '任务已创建', created_at: '2026-08-19T00:00:00Z' }] })
      : response({ tasks }))
    vi.stubGlobal('fetch', fetchMock)
    render(<AutonomousTaskWorkspace />)

    expect(await screen.findByText('等待批准')).toBeTruthy()
    expect(screen.getByText('已闭环任务')).toBeTruthy()
    expect(screen.getByRole('combobox', { name: '任务状态' })).toBeTruthy()
    expect(screen.getAllByText('待批准').length).toBeGreaterThan(0)
    expect(screen.getAllByText('排队中').length).toBeGreaterThan(0)
    expect(screen.getAllByText('已闭环').length).toBeGreaterThan(0)
    expect(screen.queryByText('需关注')).toBeNull()
    expect(screen.queryByText('失败')).toBeNull()
    fireEvent.click(screen.getByRole('button', { name: /等待批准/ }))
    expect(await screen.findByText('任务已创建')).toBeTruthy()
    expect(fetchMock).toHaveBeenCalledWith('/api/autonomous/tasks/task-1', expect.any(Object))
  })

  test('renders only the three public TODO states', async () => {
    const fetchMock = vi.fn(() => response({ tasks: [
      { id: 'r720', title: 'R720', objective: '已闭环', status: 'completed', progress: 100, source_type: 'todo' },
      { id: 'r721', title: 'R721', objective: '已闭环', status: 'completed', progress: 100, source_type: 'todo' },
      { id: 'r722', title: 'R722', objective: '待批准', status: 'pending_approval', progress: 0, source_type: 'todo' },
      { id: 'r723', title: 'R723', objective: '排队中', status: 'queued', progress: 0, source_type: 'todo' },
      { id: 'legacy', title: '旧运行任务', objective: '不应显示', status: 'running', progress: 50, source_type: 'ledger' },
    ] }))
    vi.stubGlobal('fetch', fetchMock)
    render(<AutonomousTaskWorkspace />)

    expect(await screen.findByText('R720')).toBeTruthy()
    expect(screen.getByText('R721')).toBeTruthy()
    expect(screen.getByText('R722')).toBeTruthy()
    expect(screen.getByText('R723')).toBeTruthy()
    expect(screen.queryByText('旧运行任务')).toBeNull()
    expect(screen.getByRole('combobox', { name: '任务状态' }).querySelectorAll('option')).toHaveLength(4)
    expect(screen.queryByText('运行中')).toBeNull()
    expect(screen.queryByText('失败')).toBeNull()
  })

  test('creates a task with explicit confirmation and dangerous header', async () => {
    const created = { id: 'task-new', title: '新任务', status: 'pending_approval' }
    const fetchMock = vi.fn((url, options = {}) => {
      if (url === '/api/autonomous/tasks' && options.method === 'POST') return response({ ok: true, task: created })
      if (url.includes('/task-new')) return response({ task: created, runs: [], events: [] })
      return response({ tasks: [] })
    })
    vi.stubGlobal('fetch', fetchMock)
    const dialogMock = vi.fn(() => true)
    unregisterDialogAdapter = registerDialogAdapter(dialogMock)
    render(<AutonomousTaskWorkspace />)

    await screen.findByText(/暂无任务/)
    fireEvent.click(screen.getByRole('button', { name: '新建任务' }))
    fireEvent.change(screen.getByLabelText('标题'), { target: { value: '新任务' } })
    fireEvent.click(screen.getByRole('button', { name: '保存任务' }))

    await waitFor(() => expect(fetchMock.mock.calls.some(([url, options]) => url === '/api/autonomous/tasks' && options?.method === 'POST' && options.headers['X-GA-Confirm'] === 'dangerous')).toBe(true))
    expect(dialogMock).toHaveBeenCalledWith(expect.objectContaining({ operation: 'autonomous-task-create' }))
  })

  test('approves a pending task through the minimal action and keeps it queued', async () => {
    let currentTasks = [...tasks]
    const approved = { ...tasks[0], status: 'queued', progress: 0 }
    const fetchMock = vi.fn((url, options = {}) => {
      if (url === '/api/autonomous/tasks/task-1/approve') {
        currentTasks = [approved, ...tasks.slice(1)]
        return response({ ok: true, task: approved })
      }
      if (url === '/api/autonomous/tasks/task-1') return response({ task: currentTasks[0], runs: [], events: [] })
      return response({ tasks: currentTasks })
    })
    vi.stubGlobal('fetch', fetchMock)
    const dialogMock = vi.fn(() => true)
    unregisterDialogAdapter = registerDialogAdapter(dialogMock)
    render(<AutonomousTaskWorkspace />)

    const taskButton = await screen.findByRole('button', { name: /等待批准/ })
    fireEvent.click(taskButton)
    fireEvent.click(await screen.findByRole('button', { name: '批准并排队' }))

    await waitFor(() => expect(fetchMock.mock.calls.some(([url, options]) => url === '/api/autonomous/tasks/task-1/approve' && options.headers['X-GA-Confirm'] === 'dangerous')).toBe(true))
    expect(dialogMock).toHaveBeenCalledWith(expect.objectContaining({ operation: 'autonomous-task-approve' }))
    expect((await screen.findAllByText('排队中')).length).toBeGreaterThan(0)
  })

  test('rejects a pending task without introducing a fourth public state', async () => {
    let currentTasks = [...tasks]
    const fetchMock = vi.fn((url, options = {}) => {
      if (url === '/api/autonomous/tasks/task-1/reject') {
        currentTasks = [{ ...tasks[0], status: 'pending_approval' }, ...tasks.slice(1)]
        return response({ ok: true, task: currentTasks[0] })
      }
      if (url === '/api/autonomous/tasks/task-1') return response({ task: currentTasks[0], runs: [], events: [] })
      return response({ tasks: currentTasks })
    })
    vi.stubGlobal('fetch', fetchMock)
    const dialogMock = vi.fn(() => true)
    unregisterDialogAdapter = registerDialogAdapter(dialogMock)
    render(<AutonomousTaskWorkspace />)

    fireEvent.click(await screen.findByRole('button', { name: /等待批准/ }))
    fireEvent.click(await screen.findByRole('button', { name: '拒绝' }))
    fireEvent.click(screen.getByRole('button', { name: '确认拒绝' }))

    await waitFor(() => expect(fetchMock.mock.calls.some(([url, options]) => url === '/api/autonomous/tasks/task-1/reject' && options.headers['X-GA-Confirm'] === 'dangerous')).toBe(true))
    expect(dialogMock).toHaveBeenCalledWith(expect.objectContaining({ operation: 'autonomous-task-reject' }))
    expect((await screen.findAllByText('待批准')).length).toBeGreaterThan(0)
    expect(screen.queryByText('已拒绝')).toBeNull()
    expect(screen.queryByText('失败')).toBeNull()
  })
})
