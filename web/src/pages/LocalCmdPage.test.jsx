import React from 'react'
import { afterEach, describe, expect, test, vi } from 'vitest'
import { cleanup, fireEvent, render, screen, waitFor } from '@testing-library/react'
import { LocalCmdPage } from './LocalCmdPage.jsx'

globalThis.React = React

const jsonResponse = (body, ok = true, status = 200) => ({
  ok,
  status,
  statusText: ok ? 'OK' : 'Bad Request',
  text: async () => JSON.stringify(body),
})

const deferred = () => {
  let resolve
  const promise = new Promise(result => { resolve = result })
  return { promise, resolve }
}

afterEach(() => {
  cleanup()
  vi.restoreAllMocks()
})

describe('Local CMD page', () => {
  test('should keep browse and open loading states distinct', async () => {
    const browseResponse = deferred()
    const openResponse = deferred()
    globalThis.fetch = vi.fn(url => String(url).endsWith('/api/setup/browse') ? browseResponse.promise : openResponse.promise)
    vi.spyOn(window, 'confirm').mockReturnValue(true)
    render(<LocalCmdPage lang="zh" />)

    fireEvent.change(screen.getByLabelText('工作目录'), { target: { value: 'C:\\workspace' } })
    fireEvent.click(screen.getByRole('button', { name: '选择目录' }))
    expect(screen.getByRole('button', { name: '正在打开目录选择器…' }).disabled).toBe(true)
    expect(screen.getByRole('button', { name: '打开 CMD 窗口' }).disabled).toBe(true)

    browseResponse.resolve(jsonResponse({ ok: true, path: 'C:\\workspace\\selected' }))
    await waitFor(() => expect(screen.getByRole('button', { name: '打开 CMD 窗口' }).disabled).toBe(false))
    fireEvent.click(screen.getByRole('button', { name: '打开 CMD 窗口' }))
    expect(screen.getByRole('button', { name: '正在打开 CMD…' }).disabled).toBe(true)
    expect(screen.getByRole('button', { name: '选择目录' }).disabled).toBe(true)

    openResponse.resolve(jsonResponse({ ok: true, path: 'C:\\workspace\\selected' }))
    await waitFor(() => expect(screen.getByText('CMD 窗口已打开。')).toBeTruthy())
  })

  test('should browse a directory and open CMD with dangerous confirmation', async () => {
    globalThis.fetch = vi.fn(async (url) => String(url).endsWith('/api/setup/browse')
      ? jsonResponse({ ok: true, path: 'C:\\workspace\\中文 project' })
      : jsonResponse({ ok: true, path: 'C:\\workspace\\中文 project' }))
    vi.spyOn(window, 'confirm').mockReturnValue(true)
    render(<LocalCmdPage lang="zh" />)

    fireEvent.click(screen.getByRole('button', { name: '选择目录' }))
    await waitFor(() => expect(screen.getByLabelText('工作目录').value).toContain('中文 project'))
    fireEvent.click(screen.getByRole('button', { name: '打开 CMD 窗口' }))

    await waitFor(() => expect(screen.getByText('CMD 窗口已打开。')).toBeTruthy())
    const openCall = globalThis.fetch.mock.calls.find(([url, options]) => String(url).endsWith('/api/local-cmd/open') && options?.method === 'POST')
    expect(openCall?.[1]?.headers?.['X-GA-Confirm']).toBe('dangerous')
    expect(JSON.parse(openCall?.[1]?.body)).toEqual({ path: 'C:\\workspace\\中文 project' })
  })

  test('should show an API error when directory browsing fails', async () => {
    globalThis.fetch = vi.fn(async () => jsonResponse({ error: 'directory picker failed' }, false, 500))
    render(<LocalCmdPage lang="en" />)

    fireEvent.click(screen.getByRole('button', { name: 'Choose directory' }))
    expect((await screen.findByRole('alert')).textContent).toContain('directory picker failed')
  })

  test('should report chooser cancellation and confirmation cancellation', async () => {
    globalThis.fetch = vi.fn(async (url) => String(url).endsWith('/api/setup/browse')
      ? jsonResponse({ ok: false, cancelled: true })
      : jsonResponse({ ok: true }))
    const confirm = vi.spyOn(window, 'confirm').mockReturnValue(false)
    render(<LocalCmdPage lang="en" />)

    fireEvent.click(screen.getByRole('button', { name: 'Choose directory' }))
    expect(await screen.findByText('Directory selection was cancelled.')).toBeTruthy()
    fireEvent.change(screen.getByLabelText('Working directory'), { target: { value: 'C:\\temp' } })
    fireEvent.click(screen.getByRole('button', { name: 'Open CMD window' }))

    await waitFor(() => expect(confirm).toHaveBeenCalled())
    expect(screen.getByText('Opening CMD was cancelled.')).toBeTruthy()
    expect(globalThis.fetch.mock.calls.some(([url]) => String(url).endsWith('/api/local-cmd/open'))).toBe(false)
  })

  test('should reject opening before a directory is entered', () => {
    globalThis.fetch = vi.fn()
    render(<LocalCmdPage lang="zh" />)

    expect(screen.getByRole('button', { name: '打开 CMD 窗口' }).disabled).toBe(true)
    expect(globalThis.fetch).not.toHaveBeenCalled()
  })
})
