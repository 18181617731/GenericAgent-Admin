import React from 'react'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { act, cleanup, renderHook, waitFor } from '@testing-library/react'
import { api } from '../lib/api'
import { useSidebarPreferences } from '../hooks/useSidebarPreferences'
import { normalizeSidebarPreferences, sidebarPreferencesStorageKey as key } from '../lib/chatSidebarPreferences'

vi.mock('../lib/api', () => ({ api: vi.fn() }))
const deferred = () => { let resolve; const promise = new Promise(r => { resolve = r }); return { promise, resolve } }
let server, calls
beforeEach(() => {
 localStorage.clear(); server = null; calls = []
 api.mockReset().mockImplementation(async (_url, options) => {
  if (!options) return { preferences: server }
  const body = JSON.parse(options.body); calls.push(body)
  if (!body.initialize || server == null) server = { ...server, ...body.preferences }
  return { preferences: server }
 })
})
afterEach(cleanup)

describe('backend sidebar preferences', () => {
 it('migrates old local settings only once and restores backend in a clean browser', async () => {
  localStorage.setItem(key, JSON.stringify({ sort: 'priority', showPinned: false }))
  const first = renderHook(useSidebarPreferences)
  await waitFor(() => expect(server?.sort).toBe('priority'))
  expect(calls[0].initialize).toBe(true)
  first.unmount(); localStorage.clear()
  const second = renderHook(useSidebarPreferences)
  await waitFor(() => expect(second.result.current.preferences.showPinned).toBe(false))
  expect(calls).toHaveLength(1)
 })
 it('backend wins over stale local data and StrictMode does not initialize defaults', async () => {
  server = { sort: 'priority', showPinned: false }
  localStorage.setItem(key, JSON.stringify({ sort: 'updated', showPinned: true }))
  const view = renderHook(useSidebarPreferences, { wrapper: ({ children }) => <React.StrictMode>{children}</React.StrictMode> })
  await waitFor(() => expect(view.result.current.preferences.sort).toBe('priority'))
  expect(calls).toHaveLength(0)
 })
 it('replays edits made while loading exactly once against the initial migration snapshot', async () => {
  localStorage.setItem(key, JSON.stringify({ showPinned: true }))
  const get = deferred(); api.mockImplementationOnce(() => get.promise)
  const view = renderHook(useSidebarPreferences)
  act(() => view.result.current.update('showPinned', value => !value))
  await act(async () => { get.resolve({ preferences: null }); await get.promise })
  await waitFor(() => expect(server?.showPinned).toBe(false))
  expect(calls[0].preferences.showPinned).toBe(true)
  expect(view.result.current.preferences.showPinned).toBe(false)
 })
})

it('serializes rapid edits without losing newer changes', async () => {
 server = normalizeSidebarPreferences({})
 const view = renderHook(useSidebarPreferences)
 await waitFor(() => expect(api).toHaveBeenCalled())
 await act(async () => {})
 const write = deferred(); api.mockImplementationOnce(() => write.promise)
 act(() => view.result.current.update('showPinned', false))
 act(() => { view.result.current.update('showPinned', true); view.result.current.update('sort', 'priority') })
 await act(async () => { write.resolve({ preferences: { ...server, showPinned: false } }); await write.promise })
 await waitFor(() => expect(server?.sort).toBe('priority'))
 expect(server.showPinned).toBe(true)
 expect(view.result.current.preferences.showPinned).toBe(true)
})
it('retains failed saves, retries online, and refreshes another browser on focus', async () => {
 server = normalizeSidebarPreferences({})
 const view = renderHook(useSidebarPreferences)
 await act(async () => {})
 api.mockRejectedValueOnce(new Error('offline'))
 act(() => view.result.current.update('showRecent', false))
 await waitFor(() => expect(view.result.current.error).toBe('offline'))
 expect(view.result.current.preferences.showRecent).toBe(false)
 act(() => window.dispatchEvent(new Event('online')))
 await waitFor(() => expect(server.showRecent).toBe(false))
 await waitFor(() => expect(view.result.current.error).toBe(''))
 server = { ...server, sort: 'priority' }
 act(() => window.dispatchEvent(new Event('focus')))
 await waitFor(() => expect(view.result.current.preferences.sort).toBe('priority'))
})
it('initializing migration does not overwrite another browser that saved during GET', async () => {
 localStorage.setItem(key, JSON.stringify({ showPinned: true }))
 const get = deferred(); api.mockImplementationOnce(() => get.promise)
 const view = renderHook(useSidebarPreferences)
 server = normalizeSidebarPreferences({ showPinned: false })
 await act(async () => { get.resolve({ preferences: null }); await get.promise })
 await waitFor(() => expect(view.result.current.preferences.showPinned).toBe(false))
 expect(server.showPinned).toBe(false)
})
