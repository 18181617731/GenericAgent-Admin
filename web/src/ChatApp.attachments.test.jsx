import React from 'react'
import { afterEach, describe, expect, test, vi } from 'vitest'
import { cleanup, render, screen } from '@testing-library/react'
import { ChatMessage } from './ChatApp.jsx'

afterEach(() => cleanup())

describe('chat file attachments', () => {
  test('renders image uploads with the responsive message image classes', () => {
    const { container } = render(
      <ChatMessage
        message={{
          id:'u-image',
          role:'user',
          content:'See image',
          files:[{ name:'large-photo.jpg', type:'image/jpeg', url:'data:image/jpeg;base64,AA==' }],
          created_at:0,
        }}
        pending={false}
        onAskReply={vi.fn()}
      />,
    )

    const gallery = container.querySelector('.oa-msg-images')
    const image = container.querySelector('.oa-msg-image')
    expect(gallery).toBeTruthy()
    expect(image).toBeTruthy()
    expect(image.getAttribute('src')).toBe('data:image/jpeg;base64,AA==')
    expect(image.getAttribute('alt')).toBe('large-photo.jpg')
    const imageLink = container.querySelector('.oa-msg-image-link')
    expect(imageLink).toBeTruthy()
    expect(imageLink.getAttribute('href')).toBe('data:image/jpeg;base64,AA==')
    expect(imageLink.getAttribute('target')).toBe('_blank')
  })

  test('renders a saved non-image upload as a file path card', () => {
    const content = 'Review this\n\n[附件已保存]\n[FILE:C:/tmp/report.pdf]'
    const { container } = render(
      <ChatMessage
        message={{ id:'u-file', role:'user', content, files:[], created_at:0 }}
        pending={false}
        onAskReply={vi.fn()}
      />,
    )

    expect(screen.getByText('Review this')).toBeTruthy()
    expect(screen.getByText('report.pdf')).toBeTruthy()
    expect(container.querySelector('.oa-msg-saved-paths')).toBeTruthy()
    expect(container.textContent).not.toContain('[FILE:')
  })
})
