// @vitest-environment jsdom
import React from 'react'
import { afterEach, describe, expect, test } from 'vitest'
import { cleanup, render } from '@testing-library/react'
import { ChatStats } from './ChatApp.jsx'

afterEach(() => cleanup())

describe('chat stats', () => {
  test('converts long LLM durations to readable units', () => {
    const { container } = render(<ChatStats messages={[
      { role: 'assistant', elapsed_ms: 1_158_500 },
    ]} />)

    expect(container.querySelector('.oa-chat-stats')?.textContent).toContain('LLM 19m18s')
  })

  test('shows LLM and tool durations separately', () => {
    const { container } = render(<ChatStats messages={[{
      role: 'assistant', elapsed_ms: 11_180_000, llm_elapsed_ms: 565_000, tool_elapsed_ms: 11_212_000,
    }]} />)
    const text = container.querySelector('.oa-chat-stats')?.textContent || ''
    expect(text).toContain('LLM 9m25s · 工具调用 3h6m52s')
  })
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
