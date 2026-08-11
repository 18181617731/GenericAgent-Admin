import React from 'react'
import { afterEach, describe, expect, test, vi } from 'vitest'
import { cleanup, render, screen } from '@testing-library/react'
import { ChatFileScopeContext, ChatMessage, extractToolResultFilePath, resolveChatToolFilePath } from './ChatApp.jsx'

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

  test('keeps four uploaded images separate and ignores tool-log image paths', () => {
    const files = Array.from({ length: 4 }, (_, index) => ({
      name: `dog-${index + 1}.jpg`,
      type: 'image/jpeg',
      url: `data:image/jpeg;base64,dog-${index + 1}`,
    }))
    const userView = render(<ChatMessage message={{ id:'u-four-images', role:'user', content:'Four photos', files, created_at:0 }} pending={false} onAskReply={vi.fn()} />)
    expect(userView.container.querySelectorAll('.oa-msg-image')).toHaveLength(4)
    userView.unmount()

    const assistantView = render(
      <ChatMessage
        message={{
          id:'a-tool-images',
          role:'assistant',
          content:[
            'Tool: code_run',
            '```python',
            String.raw`remote = "/sdcard/ga_shot.png"`,
            String.raw`path = r"G:\\MygenericAgent\\temp\\dog_new1.jpg"`,
            '```',
            String.raw`[Stdout] G:\\MygenericAgent\\temp\\dog_new1.jpg: 1 file pushed`,
          ].join('\n'),
          created_at:0,
        }}
        pending={false}
        onAskReply={vi.fn()}
      />,
    )
    expect(assistantView.container.querySelector('.oa-generated-images')).toBeNull()
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
    expect(container.querySelector('.oa-message-files')).toBeTruthy()
    expect(container.textContent).not.toContain('[FILE:')
  })

  test('renders assistant FILE output as a remote download link', () => {
    render(
      <ChatMessage
        message={{ id:'a-file', role:'assistant', content:'Done\n\n[FILE:C:/tmp/report.pdf]', created_at:0 }}
        pending={false}
        onAskReply={vi.fn()}
      />,
    )

    expect(screen.getByText('Done')).toBeTruthy()
    expect(screen.getByText('report.pdf')).toBeTruthy()
    const download = screen.getByRole('link', { name:'下载文件 report.pdf' })
    expect(download.getAttribute('href')).toBe('/api/files/download?path=C%3A%2Ftmp%2Freport.pdf')
    expect(download.getAttribute('download')).toBe('report.pdf')
  })

  test('resolves workspace-relative tool paths within the GA root', () => {
    const scope = {
      workspace: 'D:\\workspace_ai\\GenericAgent\\gmsl',
      gaRoot: 'D:\\workspace_ai\\GenericAgent',
    }

    expect(resolveChatToolFilePath('../memory/gmsl_build_fetch_sop.md', scope))
      .toBe('D:\\workspace_ai\\GenericAgent\\memory\\gmsl_build_fetch_sop.md')
    expect(resolveChatToolFilePath('../../outside.txt', scope)).toBe('../../outside.txt')
  })

  test('uses the absolute file path reported by a tool result', () => {
    expect(extractToolResultFilePath('[Action] Patching file: D:\\workspace_ai\\GenericAgent\\memory\\gmsl_build_fetch_sop.md'))
      .toBe('D:\\workspace_ai\\GenericAgent\\memory\\gmsl_build_fetch_sop.md')
    expect(extractToolResultFilePath('[Action] Patching file: ../memory/gmsl_build_fetch_sop.md')).toBe('')
  })

  test('uses the resolved workspace path for file card downloads', () => {
    render(
      <ChatFileScopeContext.Provider value={{ workspace: 'D:\\workspace_ai\\GenericAgent\\gmsl', gaRoot: 'D:\\workspace_ai\\GenericAgent' }}>
        <ChatMessage message={{ id:'a-workspace-file', role:'assistant', content:'已生成\n\n[FILE:../memory/gmsl_build_fetch_sop.md]', created_at:0 }} pending={false} onAskReply={vi.fn()} />
      </ChatFileScopeContext.Provider>,
    )

    expect(screen.getByRole('link', { name:'下载文件 gmsl_build_fetch_sop.md' }).getAttribute('href'))
      .toBe('/api/files/download?path=D%3A%5Cworkspace_ai%5CGenericAgent%5Cmemory%5Cgmsl_build_fetch_sop.md')
  })

  test('marks a file-changing step before the user expands it', () => {
    const content = [
      'LLM Running (Turn 37)',
      '<summary>补充归档说明</summary>',
      '🛠️ Tool: file_patch',
      '📥 args: {"path":"../memory/gmsl_build_fetch_sop.md","old_content":"before","new_content":"after"}',
    ].join('\n')
    render(<ChatMessage message={{ id:'a-file-step', role:'assistant', content, created_at:0 }} pending={false} onAskReply={vi.fn()} />)

    expect(screen.getByLabelText('本步骤包含文件增删改操作')).toBeTruthy()
  })
})
