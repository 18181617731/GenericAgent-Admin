import React from 'react'
import { afterEach, describe, expect, test } from 'vitest'
import { cleanup, render } from '@testing-library/react'
import { ChatStats } from './ChatApp.jsx'

afterEach(() => cleanup())

describe('chat stats', () => {
  test('renders zero-value stats for a new conversation', () => {
    const { container } = render(<ChatStats messages={[]} />)
    const stats = container.querySelector('.oa-chat-stats')

    expect(stats).toBeTruthy()
    expect(stats.textContent).toContain('0 轮 · 0 步')
    expect(stats.textContent).toContain('LLM 0.0s')
    expect(stats.textContent).toContain('缓存命中 0%')
    expect(stats.textContent).toContain('输入 0 · 输出 0')
  })
})
