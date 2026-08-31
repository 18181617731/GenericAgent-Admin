import React from 'react'
import { afterEach, describe, expect, test, vi } from 'vitest'
import { cleanup, fireEvent, render, screen, waitFor, within } from '@testing-library/react'
import { I18N } from '../lib/i18n.js'
import { ScheduledTaskWorkbench } from './ScheduledTaskWorkbench.jsx'

globalThis.React = React
afterEach(cleanup)

const t = I18N.zh
const taskA = {
  id: 'alpha', enabled: true, schedule: '09:00', repeat: 'daily', prompt: 'alpha prompt', next_hint: 'tomorrow 09:00',
  recent_reports: [{ name: 'alpha-run.md', path: 'sche_tasks/done/alpha-run.md', mod_time: '2026-08-28T09:00:00Z' }],
}
const taskB = { id: 'beta', enabled: false, schedule: '10:00', repeat: 'weekly', prompt: 'beta prompt', recent_reports: [] }
const taskC = { id: 'gamma', enabled: true, status: 'ERROR', error: 'broken schema', prompt: 'gamma prompt', recent_reports: [] }
const tasks = [taskA, taskB, taskC]

const props = (overrides = {}) => ({
  tasks,
  selectedTask: null,
  selectedTaskId: '',
  scheduleLoading: false,
  scheduleError: '',
  scheduleLogExists: true,
  newTaskId: 'new_task',
  setNewTaskId: vi.fn(),
  createTask: vi.fn(),
  loadScheduleTasks: vi.fn(),
  onScheduleLog: vi.fn(),
  loadTask: vi.fn(),
  clearTaskSelection: vi.fn(),
  taskEditor: JSON.stringify(taskA, null, 2),
  setTaskEditor: vi.fn(),
  editorMode: 'form',
  setEditorMode: vi.fn(),
  taskDirty: true,
  saveTask: vi.fn(),
  busy: false,
  llms: [],
  t,
  schedulerModelNo: 0,
  scheduleArtifactTitle: '',
  scheduleArtifact: '',
  onSelectArtifact: vi.fn(),
  onToggle: vi.fn(),
  onDelete: vi.fn(),
  onRun: vi.fn(),
  onReports: vi.fn(),
  taskRunStates: {},
  ...overrides,
})

describe('ScheduledTaskWorkbench', () => {
  test('supports search, state filters, create disclosure, selection, and mobile close', () => {
    const initial = props()
    const view = render(<ScheduledTaskWorkbench {...initial}/>)
    expect(screen.getAllByRole('option')).toHaveLength(3)
    fireEvent.change(screen.getByRole('searchbox'), { target: { value: 'beta prompt' } })
    expect(screen.getAllByRole('option')).toHaveLength(1)
    expect(screen.getByRole('option', { name: /beta/ })).toBeTruthy()
    fireEvent.click(screen.getByRole('button', { name: /清除筛选/ }))
    fireEvent.click(screen.getByRole('button', { name: /已暂停/ }))
    expect(screen.getAllByRole('option')).toHaveLength(1)
    fireEvent.click(screen.getByRole('button', { name: /异常/ }))
    expect(screen.getByRole('option', { name: /gamma/ })).toBeTruthy()
    fireEvent.click(screen.getByRole('button', { name: '创建' }))
    const disclosure = document.querySelector('.scheduled-task-create-disclosure')
    expect(disclosure).toBeTruthy()
    fireEvent.click(within(disclosure).getByRole('button', { name: '创建' }))
    expect(initial.createTask).toHaveBeenCalledTimes(1)

    const loadTask = vi.fn()
    fireEvent.click(screen.getByRole('button', { name: /清除筛选/ }))
    view.rerender(<ScheduledTaskWorkbench {...initial} loadTask={loadTask}/>)
    fireEvent.click(screen.getByRole('option', { name: /alpha/ }))
    expect(loadTask).toHaveBeenCalledWith('alpha')
    const clearTaskSelection = vi.fn()
    view.rerender(<ScheduledTaskWorkbench {...props({ selectedTask: taskA, selectedTaskId: 'alpha', clearTaskSelection })}/>)
    fireEvent.click(screen.getByRole('button', { name: /返回任务列表/ }))
    expect(clearTaskSelection).toHaveBeenCalledTimes(1)
  })

  test('keeps create disclosure open when creation is cancelled and closes after success', async () => {
    const createTask = vi.fn().mockResolvedValueOnce(false).mockResolvedValueOnce(true)
    render(<ScheduledTaskWorkbench {...props({ createTask })}/>)
    fireEvent.click(within(document.querySelector('.scheduled-workbench-toolbar')).getByRole('button', { name: '创建' }))
    const disclosure = document.querySelector('.scheduled-task-create-disclosure')
    expect(disclosure).toBeTruthy()
    fireEvent.click(within(disclosure).getByRole('button', { name: '创建' }))
    await waitFor(() => expect(createTask).toHaveBeenCalledTimes(1))
    expect(document.querySelector('.scheduled-task-create-disclosure')).toBeTruthy()
    fireEvent.click(within(document.querySelector('.scheduled-task-create-disclosure')).getByRole('button', { name: '创建' }))
    await waitFor(() => expect(document.querySelector('.scheduled-task-create-disclosure')).toBeNull())
  })

  test('shows the latest failed execution as a red card with its reason and separate config state', () => {
    const failed = { ...taskB, latest_run: { status: 'failed', executed_at: '2026-08-29T10:00:00Z', reason: 'browser login timed out' } }
    const { container } = render(<ScheduledTaskWorkbench {...props({ tasks: [failed] })}/>)
    const card = container.querySelector('.scheduled-task-row')
    expect(card?.classList.contains('task-run-failed')).toBe(true)
    expect(within(card).getByText('失败')).toBeTruthy()
    expect(within(card).getByText('browser login timed out')).toBeTruthy()
    expect(within(card).getByText('停用')).toBeTruthy()
    expect(within(card).getByText(/最近执行/)).toBeTruthy()
  })

  test('labels an overdue schedule separately from a successful latest execution', () => {
    const overdue = {
      ...taskA,
      status: 'OVERDUE',
      next_hint: 'last report 44.5 hours ago',
      latest_run: { status: 'success', executed_at: '2026-08-29T18:06:41Z', summary: 'report saved' },
    }
    const { container } = render(<ScheduledTaskWorkbench {...props({ tasks: [overdue] })}/>)
    const card = container.querySelector('.scheduled-task-row')
    expect(card?.classList.contains('task-run-success')).toBe(true)
    expect(within(card).getByText('成功')).toBeTruthy()
    expect(within(card).getByText('调度逾期')).toBeTruthy()
    expect(within(card).getByText('last report 44.5 hours ago')).toBeTruthy()
    expect(within(card).queryByText('异常')).toBeNull()
    expect(card?.querySelector('.task-config-state')?.classList.contains('overdue')).toBe(true)
  })

  test('keeps actions and editor available, previews a report in place, and replaces history on task switch', () => {
    const onSelectArtifact = vi.fn()
    const handlers = { onSelectArtifact, onRun: vi.fn(), onReports: vi.fn(), onToggle: vi.fn(), onDelete: vi.fn(), saveTask: vi.fn(), setEditorMode: vi.fn() }
    const view = render(<ScheduledTaskWorkbench {...props({ ...handlers, selectedTask: taskA, selectedTaskId: 'alpha' })}/>)
    fireEvent.click(screen.getByRole('button', { name: '立即执行' }))
    fireEvent.click(screen.getByRole('button', { name: '执行记录' }))
    fireEvent.click(screen.getByRole('button', { name: '停用' }))
    fireEvent.click(screen.getByRole('button', { name: '删除' }))
    expect(handlers.onRun).toHaveBeenCalledWith('alpha')
    expect(handlers.onReports).toHaveBeenCalledWith('alpha')
    expect(handlers.onToggle).toHaveBeenCalledWith('alpha', false)
    expect(handlers.onDelete).toHaveBeenCalledWith('alpha')
    fireEvent.click(screen.getByRole('button', { name: 'JSON 编辑' }))
    fireEvent.click(screen.getByRole('button', { name: '保存' }))
    expect(handlers.setEditorMode).toHaveBeenCalledWith('json')
    expect(handlers.saveTask).toHaveBeenCalledTimes(1)
    fireEvent.click(screen.getByRole('button', { name: /alpha-run\.md/ }))
    expect(onSelectArtifact).toHaveBeenCalledWith('sche_tasks/done/alpha-run.md')
    view.rerender(<ScheduledTaskWorkbench {...props({ selectedTask: taskA, selectedTaskId: 'alpha', scheduleArtifactTitle: 'alpha-run.md', scheduleArtifact: '# Alpha report' })}/>)
    expect(screen.getByText('Alpha report')).toBeTruthy()
    view.rerender(<ScheduledTaskWorkbench {...props({ selectedTask: taskB, selectedTaskId: 'beta' })}/>)
    expect(screen.getByText(/暂无执行记录/)).toBeTruthy()
    expect(screen.queryByText('alpha-run.md')).toBeNull()
  })
})
