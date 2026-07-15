import React from 'react'
import { afterEach, describe, expect, test, vi } from 'vitest'
import { cleanup, render, screen } from '@testing-library/react'
import { ChatMessage } from './ChatApp.jsx'

afterEach(() => cleanup())

describe('chat file attachments', () => {
  test('renders a saved non-image upload as a file path card', () => {
    const content = 'Review this\n\n[\u9644\u4ef6\u5df2\u4fdd\u5b58]\n[FILE:C:/tmp/report.pdf]'
    const { container } = render(
      <ChatMessage
        message={{ id:'u-file', role:'user', content, files:[], created_at:0 }}
        pending={false}
        onAskReply={vi.fn()}
      />,
    )

    expect(screen.getByText('Review this')).toBeTruthy()
    expect(screen.getByText('report.pdf')).toBeTruthy()
    expect(container.querySelector('.oa-message-files')).toBeTruthy()
    expect(container.textContent).not.toContain('[FILE:')
  })
})
