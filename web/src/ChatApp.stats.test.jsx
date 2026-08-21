// @vitest-environment jsdom
import React from 'react'
import { afterEach, describe, expect, test } from 'vitest'
import { cleanup, render } from '@testing-library/react'
import { ChatStats, buildChatStats } from './ChatApp.jsx'

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
  test('uses the live run clock for an unfinished assistant turn', () => {
    const messages = [{ role: 'assistant', run_started_at_ms: 2_000 }]
    const stats = buildChatStats(messages, 5_000)

    expect(stats.llmElapsedMs).toBe(3_000)
    expect(stats.toolElapsedMs).toBe(0)

    const { container } = render(<ChatStats messages={messages} now={5_000} />)
    expect(container.querySelector('.oa-chat-stats')?.textContent).toContain('LLM 3s · 工具调用 0.0s')
  })

  test('projects an active tool timer from the latest server snapshot', () => {
    const stats = buildChatStats([{
      role: 'assistant', run_started_at_ms: 1_000,
      tool_live_elapsed_ms: 250, tool_live_active_count: 1, tool_live_updated_at_ms: 1_000,
    }], 1_750)

    expect(stats.toolElapsedMs).toBe(1_000)
    const { container } = render(<ChatStats messages={[{
      role: 'assistant', run_started_at_ms: 1_000,
      tool_live_elapsed_ms: 250, tool_live_active_count: 1, tool_live_updated_at_ms: 1_000,
    }]} now={1_750} />)
    expect(container.querySelector('.oa-chat-stats')?.textContent).toContain('工具调用 1s')
  })
  test('uses the median of per-call model TTFT samples', () => {
    const messages = [{
      role: 'assistant',
      first_token_ms: 25,
      usages: [{ ttft_ms: 100 }, { ttft_ms: 1000 }, { ttft_ms: 400 }],
    }]

    const stats = buildChatStats(messages)
    expect(stats.firstTokenMs).toBe(400)
    expect(stats.firstTokenSamples).toBe(3)
    expect(stats.firstTokenIsModelTTFT).toBe(true)

    const { container } = render(<ChatStats messages={messages} />)
    expect(container.querySelector('.oa-chat-stats')?.textContent).toContain('\u6a21\u578b TTFT \u4e2d\u4f4d 0.4s \u00b7 3\u6b21')
  })

  test('falls back to legacy message timing when TTFT samples are absent', () => {
    const stats = buildChatStats([
      { role: 'assistant', first_token_ms: 900, elapsed_ms: 1 },
      { role: 'assistant', first_token_ms: 100, elapsed_ms: 1 },
    ])

    expect(stats.firstTokenMs).toBe(500)
    expect(stats.firstTokenSamples).toBe(2)
    expect(stats.firstTokenIsModelTTFT).toBe(false)
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
