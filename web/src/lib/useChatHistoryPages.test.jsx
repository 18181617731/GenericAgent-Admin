import { act, cleanup, renderHook } from '@testing-library/react'
import { afterEach, expect, test, vi } from 'vitest'
import { useChatHistoryPages } from './useChatHistoryPages.js'

afterEach(cleanup)
const message = (id, extra = {}) => ({ id, content: id, content_revision: id, ...extra })
const page = (id = 'a') => ({ id, messages: [message('m2')], has_more: true, before: 'cursor', message_index: [{ id: 'm1', revision: 'm1' }, { id: 'm2', revision: 'm2' }] })
const deferred = () => { let resolve; const promise = new Promise(r => { resolve = r }); return { promise, resolve } }
function setup(api = vi.fn()) {
  const messagesRef = { current: [] }
  const thread = { scrollTop: 50, scrollHeight: 200 }
  const pauseFollow = vi.fn()
  const onConflict = vi.fn()
  const setMessages = vi.fn(update => {
    const messages = typeof update === 'function' ? update(messagesRef.current) : update
    messagesRef.current = messages
    thread.scrollHeight = 100 + messages.length * 100
  })
  const hook = renderHook(() => useChatHistoryPages({ api, messagesRef, setMessages, threadRef: { current: thread }, pauseFollow, onConflict }))
  act(() => hook.result.current.apply(page(), 'instance:a'))
  return { ...hook, messagesRef, thread, pauseFollow, onConflict }
}

test('prepends without duplicates, preserves live messages and reading anchor', async () => {
  const gate = deferred()
  const api = vi.fn(() => gate.promise)
  const h = setup(api)
  let request
  act(() => { request = h.result.current.loadOlder() })
  await act(async () => { await h.result.current.loadOlder() })
  expect(api).toHaveBeenCalledTimes(1)
  h.messagesRef.current.push(message('live'))
  await act(async () => {
    gate.resolve({ messages: [message('m1'), message('m2')], has_more: false, before: '' })
    await request
  })
  expect(h.messagesRef.current.map(m => m.id)).toEqual(['m1', 'm2', 'live'])
  expect(h.thread.scrollTop).toBe(250)
  expect(h.pauseFollow).toHaveBeenCalledOnce()
  expect(h.result.current.loading).toBe(false)
})

test('switching aborts older requests and ignores late replies', async () => {
  const gate = deferred()
  const api = vi.fn(() => gate.promise)
  const h = setup(api)
  let older
  act(() => { older = h.result.current.loadOlder() })
  act(() => { h.result.current.begin(); h.result.current.apply(page('b'), 'instance:b') })
  expect(api.mock.calls.every(([, options]) => options.signal.aborted)).toBe(true)
  await act(async () => { gate.resolve({ messages: [message('wrong')], id: 'wrong' }); await older })
  expect(h.result.current.page.id).toBe('b')
  expect(h.messagesRef.current.map(m => m.id)).toEqual(['m2'])
  expect(h.result.current.loading).toBe(false)
})

test('complete older pages survive cached session return and clear discards them', async () => {
  const full = message('m1', { content: 'full body', outputs: ['output'] })
  const api = vi.fn(async () => ({ messages: [full], has_more: false, before: '' }))
  const h = setup(api)
  await act(async () => { await h.result.current.loadOlder() })
  act(() => { h.result.current.begin(); h.result.current.apply(page('b'), 'instance:b'); h.result.current.begin(); h.result.current.apply(page(), 'instance:a') })
  expect(h.messagesRef.current[0]).toEqual(full)
  expect(h.result.current.page.has_more).toBe(false)
  act(() => { h.result.current.clear(); h.result.current.apply(page(), 'instance:a') })
  expect(h.messagesRef.current.map(m => m.id)).toEqual(['m2'])
})

test('near-top loading respects threshold, in-flight deduplication and history end', async () => {
  const gate = deferred()
  const api = vi.fn(() => gate.promise)
  const h = setup(api)
  h.thread.scrollTop = 241
  await act(async () => { await h.result.current.loadOlderNearTop() })
  expect(api).not.toHaveBeenCalled()
  h.thread.scrollTop = 240
  let request
  act(() => { request = h.result.current.loadOlderNearTop() })
  await act(async () => { await h.result.current.loadOlderNearTop() })
  expect(api).toHaveBeenCalledOnce()
  expect(api.mock.calls[0][0]).toContain('view=page&before=cursor')
  await act(async () => { gate.resolve({ messages: [message('m1')], has_more: false, before: '' }); await request })
  h.thread.scrollTop = 0
  await act(async () => { await h.result.current.loadOlderNearTop() })
  expect(api).toHaveBeenCalledOnce()
})

test('conflicts trigger refresh and errors permit retry', async () => {
  const api = vi.fn().mockRejectedValueOnce(new Error('history changed; refresh history')).mockResolvedValueOnce({ messages: [message('m1')], has_more: false })
  const h = setup(api)
  await act(async () => { await h.result.current.loadOlder() })
  expect(h.onConflict).toHaveBeenCalledOnce()
  expect(h.result.current.error).toContain('changed')
  await act(async () => { await h.result.current.loadOlder() })
  expect(h.result.current.error).toBe('')
  expect(h.messagesRef.current.map(m => m.id)).toEqual(['m1', 'm2'])
})

const compact = () => message('m2', { content: '<summary>done</summary>Answer', content_partial: true, execution_count: 1, render_key: 'pending-m2' })
const fullMessage = () => message('m2', { content: 'LLM Running (Turn 1)\nProcess\n```[Info] Final response to user.```\nAnswer' })
const lazyURL = '/api/chat/session/a?view=page&execution=lazy&instance_id=other'
function applyCompact(h) { act(() => h.result.current.apply({ ...page(), messages: [compact()] }, lazyURL)) }

test('lazy details deduplicate, retain instance and revision, preserve render identity and cache full bodies', async () => {
  const gate = deferred()
  const api = vi.fn(() => gate.promise)
  const h = setup(api); applyCompact(h)
  const m = h.messagesRef.current[0]
  const first = h.result.current.loadMessage(m)
  const second = h.result.current.loadMessage(m)
  expect(second).toBe(first)
  await act(async () => { await Promise.resolve() })
  expect(api).toHaveBeenCalledOnce()
  const url = new URL(api.mock.calls[0][0], 'http://localhost')
  expect(url.searchParams.get('instance_id')).toBe('other')
  expect(url.searchParams.get('view')).toBe('message')
  expect(url.searchParams.get('revision')).toBe('m2')
  expect(url.searchParams.has('execution')).toBe(false)
  await act(async () => { gate.resolve(fullMessage()); await first })
  expect(h.messagesRef.current[0].content_partial).toBeUndefined()
  expect(h.messagesRef.current[0].render_key).toBe('pending-m2')
  act(() => { h.result.current.begin(); h.result.current.apply(page('b'), 'instance:b'); h.result.current.begin(); h.result.current.apply({ ...page(), messages: [compact()] }, lazyURL) })
  expect(h.messagesRef.current[0].content).toBe(fullMessage().content)
  await h.result.current.loadMessage(h.messagesRef.current[0])
  expect(api).toHaveBeenCalledOnce()
})

test('late lazy body cannot overwrite another session or a revised message', async () => {
  for (const switchSession of [false, true]) {
    const gate = deferred(); const api = vi.fn(() => gate.promise)
    const h = setup(api); applyCompact(h)
    const request = h.result.current.loadMessage(h.messagesRef.current[0])
    const rejection = expect(request).rejects.toMatchObject({ name: 'AbortError' })
    await act(async () => { await Promise.resolve() })
    act(() => { if (switchSession) { h.result.current.begin(); h.result.current.apply(page('b'), 'instance:b') }
      else h.result.current.apply({ ...page(), messages: [message('m2', { content_revision: 'changed', content: 'revised' })] }, lazyURL)
    })
    await act(async () => { gate.resolve(fullMessage()); await rejection })
    expect(h.messagesRef.current[0].content).toBe(switchSession ? 'm2' : 'revised')
    h.unmount()
  }
})

test('lazy failure permits retry and mismatched body refreshes without corrupting content', async () => {
  const api = vi.fn().mockRejectedValueOnce(new Error('offline')).mockResolvedValueOnce({ ...fullMessage(), content_revision: 'changed' }).mockResolvedValueOnce(fullMessage())
  const h = setup(api); applyCompact(h)
  await act(async () => { await expect(h.result.current.loadMessage(h.messagesRef.current[0])).rejects.toThrow('offline') })
  await act(async () => { await expect(h.result.current.loadMessage(h.messagesRef.current[0])).rejects.toThrow('message changed') })
  expect(h.onConflict).toHaveBeenCalledOnce()
  expect(h.messagesRef.current[0].content_partial).toBe(true)
  await act(async () => { await h.result.current.loadMessage(h.messagesRef.current[0]) })
  expect(h.messagesRef.current[0].content).toBe(fullMessage().content)
})
