// @vitest-environment jsdom

import React from 'react'
import { afterEach, expect, test } from 'vitest'
import { cleanup, render, screen } from '@testing-library/react'
import ChatPrivacyCurtain from './ChatPrivacyCurtain.jsx'

afterEach(() => cleanup())

test('privacy curtain exposes only generic execution state and metrics', () => {
  render(<ChatPrivacyCurtain status="running" metrics={[
    { label:'消息', value:'8 条' },
    { label:'耗时', value:'12s' },
    { label:'Token', value:'输入 1.2K · 输出 320' },
    { label:'Loop', value:'2/5' },
  ]}/>)

  const status = screen.getByRole('region', { name:'隐私模式状态' })
  expect(status.textContent).toContain('任务执行中')
  expect(status.textContent).toContain('8 条')
  expect(status.textContent).toContain('12s')
  expect(status.textContent).toContain('2/5')
  expect(status.textContent).not.toContain('SECRET_TITLE')
  expect(status.textContent).not.toContain('D:/secret.txt')
})
