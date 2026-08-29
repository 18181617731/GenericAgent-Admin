import React from 'react'
import { afterEach, describe, expect, test, vi } from 'vitest'
import { cleanup, fireEvent, render, screen } from '@testing-library/react'
import { PendingAttachments } from './ChatApp.jsx'

const imageAttachment = {
  id: 'image-1',
  name: 'photo.png',
  type: 'image/png',
  dataURL: 'data:image/png;base64,cHJldmlldw==',
}

const fileAttachment = {
  id: 'file-1',
  name: 'report.pdf',
  type: 'application/pdf',
  dataURL: 'data:application/pdf;base64,cGRm',
}

afterEach(() => {
  cleanup()
  document.body.style.overflow = ''
})

describe('pending attachment previews', () => {
  test('opens an image preview and closes it with Escape', () => {
    render(<PendingAttachments attachments={[imageAttachment]} onRemove={vi.fn()}/>)

    const open = screen.getByRole('button', { name:'\u9884\u89c8\u56fe\u7247 photo.png' })
    expect(open.querySelector('img')?.getAttribute('src')).toBe(imageAttachment.dataURL)
    fireEvent.click(open)

    const dialog = screen.getByRole('dialog', { name:'\u56fe\u7247\u9884\u89c8 photo.png' })
    expect(dialog.querySelector('img')?.getAttribute('src')).toBe(imageAttachment.dataURL)
    expect(document.body.style.overflow).toBe('hidden')

    fireEvent.keyDown(window, { key:'Escape' })
    expect(screen.queryByRole('dialog')).toBeNull()
    expect(document.body.style.overflow).toBe('')
  })

  test('closes from the close button or the preview backdrop', () => {
    render(<PendingAttachments attachments={[imageAttachment]} onRemove={vi.fn()}/>)
    const open = screen.getByRole('button', { name:'\u9884\u89c8\u56fe\u7247 photo.png' })
    fireEvent.click(open)

    const dialog = screen.getByRole('dialog')
    fireEvent.mouseDown(dialog)
    expect(screen.getByRole('dialog')).toBeTruthy()

    fireEvent.click(screen.getByRole('button', { name:'\u5173\u95ed\u56fe\u7247\u9884\u89c8' }))
    expect(screen.queryByRole('dialog')).toBeNull()

    fireEvent.click(open)
    fireEvent.mouseDown(document.querySelector('.oa-attachment-lightbox'))
    expect(screen.queryByRole('dialog')).toBeNull()
  })

  test('keeps non-image attachments non-previewable and removable', () => {
    const onRemove = vi.fn()
    render(<PendingAttachments attachments={[fileAttachment]} onRemove={onRemove}/>)

    expect(screen.queryByRole('button', { name:/\u9884\u89c8\u56fe\u7247/ })).toBeNull()
    expect(screen.getByText('PDF')).toBeTruthy()
    fireEvent.click(screen.getByRole('button', { name:'\u79fb\u9664\u9644\u4ef6 report.pdf' }))
    expect(onRemove).toHaveBeenCalledOnce()
    expect(onRemove).toHaveBeenCalledWith('file-1')
  })
})
