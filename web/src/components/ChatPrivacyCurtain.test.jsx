// @vitest-environment jsdom

import React from 'react'
import { afterEach, expect, test, vi } from 'vitest'
import { act, cleanup, fireEvent, render, screen } from '@testing-library/react'
import ChatPrivacyCurtain from './ChatPrivacyCurtain.jsx'

afterEach(() => cleanup())

test('privacy curtain exposes only generic execution state and metrics', () => {
  render(<ChatPrivacyCurtain status="running" metrics={[
    { label:'消息', value:'8 条' },
    { label:'耗时', value:'12s' },
    { label:'Token', value:'输入 1.2K · 输出 320' },
    { label:'Loop', value:'2/5' },
  ]}/>)

  const status = screen.getByRole('region', { name:'任务状态' })
  expect(status.textContent).toContain('任务执行中')
  expect(status.textContent).toContain('8 条')
  expect(status.textContent).toContain('12s')
  expect(status.textContent).toContain('2/5')
  expect(status.textContent).not.toContain('SECRET_TITLE')
  expect(status.textContent).not.toContain('D:/secret.txt')
  expect(status.textContent).not.toContain('隐私')
})

test('completed curtain mounts the latest result only while deliberately revealed', () => {
  const renderResult = vi.fn(() => <p>SECRET_FINAL_RESULT</p>)
  render(<ChatPrivacyCurtain status="completed" renderResult={renderResult}/>)
  const status = screen.getByRole('region', { name:'任务状态' })
  expect(document.body.innerHTML).not.toContain('SECRET_FINAL_RESULT')
  expect(renderResult).not.toHaveBeenCalled()

  fireEvent.pointerEnter(status, { pointerType:'mouse' })
  expect(screen.getByRole('region', { name:'最后结果' }).textContent).toContain('SECRET_FINAL_RESULT')
  fireEvent.click(screen.getByRole('button', { name:'收起最后结果' }))
  expect(document.body.innerHTML).not.toContain('SECRET_FINAL_RESULT')
  expect(screen.getByRole('button', { name:'临时查看最后结果' })).toBeTruthy()
  fireEvent.pointerLeave(status, { pointerType:'mouse' })
  fireEvent.pointerEnter(status, { pointerType:'mouse' })
  expect(screen.getByText('SECRET_FINAL_RESULT')).toBeTruthy()
  fireEvent.pointerLeave(status, { pointerType:'mouse' })
  expect(document.body.innerHTML).not.toContain('SECRET_FINAL_RESULT')
})

test('latest result supports keyboard focus and a time-limited click reveal', () => {
  vi.useFakeTimers()
  render(<ChatPrivacyCurtain status="completed" renderResult={() => <p>KEYBOARD_RESULT</p>}/>)
  const reveal = screen.getByRole('button', { name:'临时查看最后结果' })
  fireEvent.focus(reveal)
  expect(screen.getByText('KEYBOARD_RESULT')).toBeTruthy()
  fireEvent.blur(reveal)
  expect(screen.queryByText('KEYBOARD_RESULT')).toBeNull()

  fireEvent.click(reveal)
  expect(screen.getByText('KEYBOARD_RESULT')).toBeTruthy()
  act(() => vi.advanceTimersByTime(12000))
  expect(screen.queryByText('KEYBOARD_RESULT')).toBeNull()
  vi.useRealTimers()
})

test('running curtain never offers or mounts a result reveal', () => {
  const renderResult = vi.fn(() => <p>STREAMING_SECRET</p>)
  render(<ChatPrivacyCurtain status="running" renderResult={renderResult}/>)
  fireEvent.pointerEnter(screen.getByRole('region', { name:'任务状态' }), { pointerType:'mouse' })
  expect(screen.queryByRole('button', { name:/最后结果/ })).toBeNull()
  expect(document.body.innerHTML).not.toContain('STREAMING_SECRET')
  expect(renderResult).not.toHaveBeenCalled()
})
