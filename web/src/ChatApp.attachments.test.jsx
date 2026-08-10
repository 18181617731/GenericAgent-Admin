import React from 'react'
import { afterEach, describe, expect, test, vi } from 'vitest'
import { cleanup, fireEvent, render, screen } from '@testing-library/react'
import { ChatFileScopeContext, ChatMessage, GeneratedImageGallery, extractToolResultFilePath, resolveChatToolFilePath } from './ChatApp.jsx'

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
    const imageLink = screen.getByRole('button', { name:'查看图片 large-photo.jpg' })
    expect(imageLink).toBeTruthy()
    expect(imageLink.getAttribute('type')).toBe('button')
    expect(screen.getByText('large-photo.jpg')).toBeTruthy()

    fireEvent.click(imageLink)
    expect(screen.getByRole('dialog', { name:'图片预览 large-photo.jpg' })).toBeTruthy()
    expect(screen.getByRole('button', { name:'关闭图片预览' })).toBeTruthy()
    expect(screen.getByText('1 / 1')).toBeTruthy()

    fireEvent.keyDown(document, { key:'Escape' })
    expect(screen.queryByRole('dialog', { name:'图片预览 large-photo.jpg' })).toBeNull()
  })

  test('supports switching between full-size image previews', () => {
    render(
      <ChatMessage
        message={{
          id:'u-images',
          role:'user',
          content:'See images',
          files:[
            { name:'first.png', type:'image/png', url:'data:image/png;base64,AA==' },
            { name:'second.png', type:'image/png', url:'data:image/png;base64,BB==' },
          ],
          created_at:0,
        }}
        pending={false}
        onAskReply={vi.fn()}
      />,
    )

    fireEvent.click(screen.getByRole('button', { name:'查看图片 first.png' }))
    expect(screen.getByRole('dialog', { name:'图片预览 first.png' })).toBeTruthy()
    fireEvent.click(screen.getByRole('button', { name:'下一张图片' }))
    expect(screen.getByRole('dialog', { name:'图片预览 second.png' })).toBeTruthy()
  })

  test('opens generated images in a lightbox and closes with Escape', () => {
    render(<GeneratedImageGallery content={'[FILE:C:\\tmp\\generated-photo.png]'} />)

    fireEvent.click(screen.getByRole('button', { name:'查看原图 generated-photo.png' }))
    expect(screen.getByRole('dialog', { name:'生成图片预览' })).toBeTruthy()

    fireEvent.keyDown(document, { key:'Escape' })
    expect(screen.queryByRole('dialog', { name:'生成图片预览' })).toBeNull()
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
