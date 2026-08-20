import React from 'react'
import { afterEach, describe, expect, test, vi } from 'vitest'
import { cleanup, fireEvent, render } from '@testing-library/react'
import { ChatMessage } from './ChatApp.jsx'

const askUserContent = [
  '\u{1F6E0}\uFE0F Tool: `ask_user`',
  '```text',
  JSON.stringify({ question: 'Choose a mode', candidates: ['Fast', 'Careful'] }),
  '```',
].join('\n')

const makeVerboseAskUserContent = ({ question, candidates }) => [
  '\u{1F6E0}\uFE0F Tool: `ask_user`  \u{1F4E5} args:',
  '````text',
  JSON.stringify({ question, ...(candidates ? { candidates } : {}) }, null, 2),
  '````',
  '`````',
  'Waiting for your answer ...',
  '`````',
].join('\n')

const renderAskUser = (props = {}) => render(
  <ChatMessage
    message={{ id: 'ask-1', role: 'assistant', content: askUserContent, files: [], created_at: 0 }}
    pending={false}
    onAskReply={vi.fn()}
    {...props}
  />,
)

const expandAskUser = (container) => {
  const fold = container.querySelector('.ga-fold.fold-tool')
  if (!fold) throw new Error(container.innerHTML.slice(0, 4000))
  if (!fold.open) fireEvent.click(fold.querySelector('summary'))
  return container.querySelectorAll('.oa-ask-options button')
}

afterEach(() => cleanup())

describe('ask_user quick replies', () => {
  test('defaults open only in the latest assistant branch', () => {
    const latest = renderAskUser({ isLatestMessage: true })
    expect(latest.container.querySelector('.ga-fold.fold-tool')?.open).toBe(true)
    latest.unmount()

    const historical = renderAskUser()
    expect(historical.container.querySelector('.ga-fold.fold-tool')?.open).toBe(false)
  })

  test('loads a candidate into the composer instead of sending it', () => {
    const onAskReply = vi.fn()
    const onQuickReply = vi.fn()
    const { container } = renderAskUser({ onAskReply, onQuickReply })

    const options = expandAskUser(container)
    expect(options).toHaveLength(2)
    expect(options[0].title).toContain('输入框')
    expect(options[0].disabled).toBe(false)
    fireEvent.click(options[0])

    expect(onAskReply).toHaveBeenCalledOnce()
    expect(onAskReply).toHaveBeenCalledWith('Fast')
    expect(onQuickReply).not.toHaveBeenCalled()
  })

  test('keeps candidates insertable while a message is running', () => {
    const onAskReply = vi.fn()
    const onQuickReply = vi.fn()
    const { container } = renderAskUser({ onAskReply, onQuickReply, quickReplyDisabled: true })

    const options = expandAskUser(container)
    expect(options[0].disabled).toBe(false)
    fireEvent.click(options[0])
    expect(onAskReply).toHaveBeenCalledWith('Fast')
    expect(onQuickReply).not.toHaveBeenCalled()
  })

  test('keeps historical candidates available as insert-only replies', () => {
    const onAskReply = vi.fn()
    const { container } = renderAskUser({ onAskReply })

    const options = expandAskUser(container)
    expect(options[1].title).toContain('输入框')
    fireEvent.click(options[1])
    expect(onAskReply).toHaveBeenCalledWith('Careful')
  })

  test('renders the question from four-backtick args before a plain waiting result', () => {
    const question = '这是一条测试问题，请问你想让我帮你做什么？'
    const { container } = renderAskUser({
      message: {
        id: 'ask-verbose-question',
        role: 'assistant',
        content: makeVerboseAskUserContent({ question }),
        files: [],
        created_at: 0,
      },
    })

    expandAskUser(container)
    const receiptTarget = container.querySelector('.ga-receipt-target')
    expect(receiptTarget?.textContent).toBe(question)
    expect(receiptTarget?.title).toBe(question)
    expect(container.querySelector('.ga-receipt.is-complete .ga-receipt-status')).toBeNull()
    expect(container.querySelector('.ga-receipt.is-complete')?.getAttribute('aria-label')).not.toContain('已完成')
    expect(container.querySelector('.oa-ask-question')?.textContent).toBe(question)
    expect(container.querySelector('.oa-ask-question')?.textContent).not.toContain('Waiting for your answer')
    expect(container.querySelector('.oa-ask-result')).toBeNull()
    expect(container.querySelectorAll('.oa-ask-options button')).toHaveLength(0)
  })

  test('renders all candidates from four-backtick args before a plain waiting result', () => {
    const candidates = ['咖啡', '茶', '果汁', '白开水']
    const { container } = renderAskUser({
      message: {
        id: 'ask-verbose-candidates',
        role: 'assistant',
        content: makeVerboseAskUserContent({ question: '今天想喝点什么？', candidates }),
        files: [],
        created_at: 0,
      },
    })

    const options = expandAskUser(container)
    expect(container.querySelector('.oa-ask-question')?.textContent).toBe('今天想喝点什么？')
    expect(Array.from(options, option => option.textContent)).toEqual(candidates)
  })
})
