import React from 'react'
import { afterEach, describe, expect, test, vi } from 'vitest'
import { cleanup, fireEvent, render, screen, waitFor } from '@testing-library/react'
import { ChatFileScopeContext, ChatMessage, DocumentPreviewGallery, GeneratedImageGallery, extractToolResultFilePath, resolveChatToolFilePath } from './ChatApp.jsx'

afterEach(() => { cleanup(); vi.unstubAllGlobals() })

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

    const thumb = screen.getByRole('button', { name:'查看原图 generated-photo.png' })
    expect(thumb.querySelector('span')).toBeNull()
    expect(screen.queryByText('generated-photo.png')).toBeNull()
    fireEvent.click(thumb)
    expect(screen.getByRole('dialog', { name:'生成图片预览' })).toBeTruthy()

    fireEvent.keyDown(document, { key:'Escape' })
    expect(screen.queryByRole('dialog', { name:'生成图片预览' })).toBeNull()
  })

  test('renders a screenshot result from a labeled evidence path', () => {
    const path = String.raw`C:\tmp\evidence\final-screen.png`
    const { container } = render(
      <GeneratedImageGallery content={['Evidence saved:', '', `- Screenshot: ${path}`].join('\n')} />,
    )

    const thumb = container.querySelector('.oa-generated-image-thumb')
    expect(thumb).toBeTruthy()
    expect(thumb?.getAttribute('title')).toBe(path)
    expect(thumb?.querySelector('img')?.getAttribute('src')).toBe(`/api/files/image?path=${encodeURIComponent(path)}`)
  })

  test('adds the active chat instance to saved upload image URLs', () => {
    const { container } = render(
      <ChatMessage
        chatInstanceID="default"
        message={{ id:'u-saved-image', role:'user', content:'Saved image', files:[{ name:'saved.png', type:'image/png', url:'/api/chat/file/saved.png' }], created_at:0 }}
        pending={false}
        onAskReply={vi.fn()}
      />,
    )

    expect(container.querySelector('.oa-msg-image')?.getAttribute('src')).toBe('/api/chat/file/saved.png?instance_id=default')
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

  test('turns a path-only document result into a selectable in-conversation preview', async () => {
    const path = String.raw`G:\MygenericAgent\temp\projects\闲鱼\综合报告.md`
    const fetchMock = vi.fn(async () => ({
      ok: true,
      text: async () => JSON.stringify({ name:'综合报告.md', path, size:42, content:'# 综合报告\n\n正文内容' }),
    }))
    vi.stubGlobal('fetch', fetchMock)

    render(<DocumentPreviewGallery content={['已合并至：', '', '```text', path, '```'].join('\n')} />)

    expect(screen.getByText('综合报告.md')).toBeTruthy()
    fireEvent.click(screen.getByRole('button', { name:'预览文档 综合报告.md' }))
    expect(screen.getByRole('dialog', { name:'文档预览 综合报告.md' })).toBeTruthy()
    expect(await screen.findByRole('heading', { name:'综合报告' })).toBeTruthy()
    expect(screen.getByText('正文内容')).toBeTruthy()
    expect(fetchMock).toHaveBeenCalledWith(
      `/api/files/read?path=${encodeURIComponent(path)}`,
      expect.objectContaining({ headers: expect.any(Object) }),
    )

    await waitFor(() => expect(screen.getByRole('link', { name:'下载文档' }).getAttribute('href'))
      .toBe(`/api/files/download?path=${encodeURIComponent(path)}`))
    fireEvent.keyDown(document, { key:'Escape' })
    expect(screen.queryByRole('dialog', { name:'文档预览 综合报告.md' })).toBeNull()
  })

  test('allows choosing another document while the preview is open', async () => {
    const first = String.raw`C:\tmp\first.md`
    const second = String.raw`C:\tmp\second.txt`
    const fetchMock = vi.fn(async (url) => ({
      ok: true,
      text: async () => JSON.stringify({
        content: url.includes(encodeURIComponent(second)) ? '第二份正文' : '# 第一份正文',
        size: 20,
      }),
    }))
    vi.stubGlobal('fetch', fetchMock)

    render(<DocumentPreviewGallery content={['结果文档：', '', '```text', first, second, '```'].join('\n')} />)
    fireEvent.click(screen.getByRole('button', { name:'预览文档 first.md' }))
    expect(await screen.findByRole('heading', { name:'第一份正文' })).toBeTruthy()

    fireEvent.change(screen.getByRole('combobox', { name:'选择文档' }), { target:{ value:second } })
    expect(await screen.findByText('第二份正文')).toBeTruthy()
    expect(fetchMock).toHaveBeenCalledTimes(2)
  })

  test('renders assistant image files as image-only preview cards', () => {
    const path = 'C:/tmp/schnauzer.png'
    const { container } = render(
      <ChatMessage
        message={{ id:'a-image-file', role:'assistant', content:`Done\n\n[FILE:${path}]`, created_at:0 }}
        pending={false}
        onAskReply={vi.fn()}
      />,
    )

    const card = container.querySelector('.oa-file-card.oa-file-kind-image')
    expect(card).toBeTruthy()
    expect(card.querySelector('.oa-file-meta')).toBeNull()
    expect(card.getAttribute('title')).toBe(path)
    expect(card.querySelector('img')?.getAttribute('src')).toBe(`/api/files/image?path=${encodeURIComponent(path)}`)
    expect(screen.queryByText(path)).toBeNull()
    expect(screen.getByRole('link', { name:'下载文件 schnauzer.png' })).toBeTruthy()

    fireEvent.click(screen.getByRole('button', { name:'查看图片 schnauzer.png' }))
    expect(screen.getByRole('dialog', { name:'图片预览 schnauzer.png' })).toBeTruthy()
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
