import React from 'react'
import { cleanup, render, screen } from '@testing-library/react'
import { afterEach, describe, expect, it } from 'vitest'
import { TaskRow } from './schedule.jsx'

const t = {
  autostart: '开机自启', enabled: '已启用', disabled: '已禁用', remove: '删除', empty: '空', error: '错误',
  tasks: { unnamed: '未命名', unscheduled: '未排期', manual: '手动', unnamedModel: '未命名模型', explicitEnable: '需要手动启用' },
}

afterEach(cleanup)

describe('scheduled task latest execution card', () => {
  it('keeps a failed run red and visible even when configuration is disabled', () => {
    const { container } = render(<TaskRow task={{ id: 'daily', enabled: false, schedule: '10:00', repeat: 'daily', latest_run: { status: 'failed', executed_at: '2026-08-29T10:00:00Z', reason: 'browser login timed out' } }} t={t}/>)
    expect(container.querySelector('.task-row')?.classList.contains('task-run-failed')).toBe(true)
    expect(screen.getByText('失败')).toBeTruthy()
    expect(screen.getByText('browser login timed out')).toBeTruthy()
    expect(screen.getByText('已禁用')).toBeTruthy()
    expect(screen.getByText(/最近执行/)).toBeTruthy()
  })

  it('renders blocked details with a warning execution state', () => {
    const { container } = render(<TaskRow task={{ id: 'ops', enabled: true, latest_run: { status: 'blocked', reason: '等待用户登录' } }} t={t}/>)
    expect(container.querySelector('.task-row')?.classList.contains('task-run-blocked')).toBe(true)
    expect(screen.getByText('阻塞')).toBeTruthy()
    expect(screen.getByText('等待用户登录')).toBeTruthy()
  })
})
