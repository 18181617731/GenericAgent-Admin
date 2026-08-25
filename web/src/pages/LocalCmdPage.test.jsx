import React from 'react'
import { afterEach, beforeEach, describe, expect, test, vi } from 'vitest'
import { cleanup, fireEvent, render, screen, waitFor } from '@testing-library/react'
import { LocalCmdPage } from './LocalCmdPage.jsx'

globalThis.React = React

const jsonResponse = (body, ok = true, status = 200) => ({
  ok,
  status,
  statusText: ok ? 'OK' : 'Bad Request',
  text: async () => JSON.stringify(body),
})

const streamResponse = events => {
  let sent = false
  return {
    ok: true,
    body: { getReader: () => ({ read: async () => {
      if (sent) return { done: true }
      sent = true
      return { done: false, value: new TextEncoder().encode(events) }
    } }) },
  }
}

const eventLines = events => `${events.map(event => JSON.stringify(event)).join('\n')}\n`
const session = { id: 'session-1', path: 'C:\\workspace', status: 'running', seq: 0 }
const roots = ['C:\\']

const makeFetch = ({ streamEvents = [], createError = false, directoryError = false } = {}) => vi.fn(async (url, options = {}) => {
  const parsed = new URL(String(url), 'http://localhost')
  const method = options.method || 'GET'
  if (parsed.pathname === '/api/local-cmd/directories') {
    if (directoryError) return jsonResponse({ error: 'directory list failed' }, false, 500)
    const path = parsed.searchParams.get('path') || ''
    if (!path) return jsonResponse({ ok: true, current: '', parent: '', roots, entries: [] })
    return jsonResponse({ ok: true, current: path, parent: 'C:\\', roots: [], entries: [{ name: '中文 project', path: `${path}\\中文 project` }] })
  }
  if (parsed.pathname === '/api/local-cmd/sessions' && method === 'POST') {
    if (createError) return jsonResponse({ error: 'session creation failed' }, false, 500)
    return jsonResponse(session)
  }
  if (parsed.pathname.endsWith('/stream')) return streamResponse(eventLines(streamEvents))
  if (parsed.pathname.endsWith('/input') || parsed.pathname.endsWith('/resize')) return jsonResponse({ ok: true })
  if (parsed.pathname.includes('/sessions/')) {
    if (method === 'DELETE') return jsonResponse({ ok: true, id: session.id })
    return jsonResponse(session)
  }
  throw new Error(`unexpected request: ${method} ${parsed.pathname}`)
})

const runningEvents = [{ type: 'sync', seq: 0, status: 'running', path: session.path }]
const exitedEvents = [
  ...runningEvents,
  { type: 'data', seq: 1, data: btoa('REMOTE_CMD_OK\r\n') },
  { type: 'exit', seq: 2, exit_code: 0 },
]

beforeEach(() => {
  window.localStorage.clear()
  vi.spyOn(window, 'confirm').mockReturnValue(true)
})

afterEach(() => {
  cleanup()
  vi.restoreAllMocks()
  delete globalThis.fetch
})

describe('Remote CMD page', () => {
  test('describes server execution and uses the dedicated directory API', async () => {
    globalThis.fetch = makeFetch()
    render(<LocalCmdPage lang="zh" />)

    expect(screen.getByText(/命令在运行 GA Admin 的 Windows 主机执行/)).toBeTruthy()
    await waitFor(() => expect(globalThis.fetch).toHaveBeenCalledWith('/api/local-cmd/directories', expect.anything()))
    const urls = globalThis.fetch.mock.calls.map(([url]) => String(url))
    expect(urls.some(url => url.includes('/api/setup/browse') || url.includes('/api/local-cmd/open'))).toBe(false)
  })

  test('creates a session with dangerous confirmation and renders streamed output', async () => {
    globalThis.fetch = makeFetch({ streamEvents: exitedEvents })
    render(<LocalCmdPage lang="zh" />)
    fireEvent.change(screen.getByLabelText('服务端工作目录'), { target: { value: session.path } })
    fireEvent.click(screen.getByRole('button', { name: '新建远程会话' }))

    await waitFor(() => expect(screen.getByText('REMOTE_CMD_OK')).toBeTruthy())
    const createCall = globalThis.fetch.mock.calls.find(([url, options]) => String(url) === '/api/local-cmd/sessions' && options.method === 'POST')
    expect(createCall[1].headers['X-GA-Confirm']).toBe('dangerous')
    expect(JSON.parse(createCall[1].body)).toEqual({ path: session.path, cols: 120, rows: 32 })
    expect(window.localStorage.getItem('ga-admin.remote-cmd.session')).toBe(session.id)
  })

  test('browses server directories without using setup browse', async () => {
    globalThis.fetch = makeFetch()
    render(<LocalCmdPage lang="en" />)
    await waitFor(() => expect(screen.getByRole('button', { name: 'C:\\' })).toBeTruthy())
    fireEvent.click(screen.getByRole('button', { name: 'C:\\' }))
    await waitFor(() => expect(screen.getByRole('button', { name: /中文 project/ })).toBeTruthy())
    fireEvent.click(screen.getByRole('button', { name: /中文 project/ }))

    await waitFor(() => expect(screen.getByLabelText('Server working directory').value).toContain('中文 project'))
    expect(globalThis.fetch.mock.calls.some(([url]) => String(url).includes('/api/setup/browse'))).toBe(false)
    expect(globalThis.fetch.mock.calls.some(([url]) => String(url).includes('/api/local-cmd/directories?path='))).toBe(true)
  })

  test('sends command input and mobile shortcuts with dangerous headers', async () => {
    globalThis.fetch = makeFetch({ streamEvents: runningEvents })
    render(<LocalCmdPage lang="en" />)
    fireEvent.change(screen.getByLabelText('Server working directory'), { target: { value: session.path } })
    fireEvent.click(screen.getByRole('button', { name: 'New remote session' }))
    await waitFor(() => expect(screen.getByLabelText('Command input')).toBeTruthy())

    fireEvent.change(screen.getByLabelText('Command input'), { target: { value: 'echo hi' } })
    fireEvent.keyDown(screen.getByLabelText('Command input'), { key: 'Enter' })
    fireEvent.click(screen.getByRole('button', { name: 'Ctrl+C' }))
    fireEvent.click(screen.getByRole('button', { name: 'Tab' }))
    await waitFor(() => expect(globalThis.fetch.mock.calls.filter(([url]) => String(url).endsWith('/input')).length).toBe(3))

    const inputCalls = globalThis.fetch.mock.calls.filter(([url]) => String(url).endsWith('/input'))
    expect(inputCalls.every(([, options]) => options.headers['X-GA-Confirm'] === 'dangerous')).toBe(true)
    expect(inputCalls.map(([, options]) => JSON.parse(options.body).base64)).toEqual([
      btoa('echo hi\r'), btoa('\x03'), btoa('\t'),
    ])
  })

  test('restores a saved session and resumes its stream', async () => {
    window.localStorage.setItem('ga-admin.remote-cmd.session', session.id)
    globalThis.fetch = makeFetch({ streamEvents: exitedEvents })
    render(<LocalCmdPage lang="en" />)

    await waitFor(() => expect(screen.getByText('REMOTE_CMD_OK')).toBeTruthy())
    expect(globalThis.fetch.mock.calls.some(([url]) => String(url).endsWith(`/sessions/${session.id}`))).toBe(true)
    expect(globalThis.fetch.mock.calls.some(([url]) => String(url).includes('/api/local-cmd/open'))).toBe(false)
  })

  test('reports directory and session errors, and keeps confirmation cancellation local', async () => {
    globalThis.fetch = makeFetch({ directoryError: true, createError: true })
    render(<LocalCmdPage lang="en" />)
    expect((await screen.findByRole('alert')).textContent).toContain('directory list failed')
    fireEvent.change(screen.getByLabelText('Server working directory'), { target: { value: session.path } })
    fireEvent.click(screen.getByRole('button', { name: 'New remote session' }))
    expect((await screen.findByRole('alert')).textContent).toContain('session creation failed')

    window.confirm.mockReturnValueOnce(false)
    fireEvent.click(screen.getByRole('button', { name: 'New remote session' }))
    await waitFor(() => expect(screen.getByText('Remote CMD creation cancelled.')).toBeTruthy())
    expect(globalThis.fetch.mock.calls.filter(([url]) => String(url) === '/api/local-cmd/sessions').length).toBe(1)
  })

  test('resizes and ends a session through dangerous mutation APIs', async () => {
    globalThis.fetch = makeFetch({ streamEvents: runningEvents })
    render(<LocalCmdPage lang="en" />)
    fireEvent.change(screen.getByLabelText('Server working directory'), { target: { value: session.path } })
    fireEvent.click(screen.getByRole('button', { name: 'New remote session' }))
    await waitFor(() => expect(screen.getByRole('button', { name: 'Apply size' })).toBeTruthy())
    fireEvent.click(screen.getByRole('button', { name: 'Apply size' }))
    await waitFor(() => expect(globalThis.fetch.mock.calls.some(([url]) => String(url).endsWith('/resize'))).toBe(true))
    const resizeCall = globalThis.fetch.mock.calls.find(([url]) => String(url).endsWith('/resize'))
    expect(resizeCall[1].headers['X-GA-Confirm']).toBe('dangerous')

    fireEvent.click(screen.getByRole('button', { name: 'End session' }))
    await waitFor(() => expect(globalThis.fetch.mock.calls.some(([url], index) => String(url).endsWith(`/sessions/${session.id}`) && globalThis.fetch.mock.calls[index][1]?.method === 'DELETE')).toBe(true))
    const deleteCall = globalThis.fetch.mock.calls.find(([url], index) => String(url).endsWith(`/sessions/${session.id}`) && globalThis.fetch.mock.calls[index][1]?.method === 'DELETE')
    expect(deleteCall[1].headers['X-GA-Confirm']).toBe('dangerous')
  })
})
