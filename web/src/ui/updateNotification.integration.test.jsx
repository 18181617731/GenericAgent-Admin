import React from 'react'
import { afterEach, beforeEach, expect, it, vi } from 'vitest'
import { act, cleanup, fireEvent, render, renderHook, screen } from '@testing-library/react'
import { api } from '../lib/api'
import { useVersionNotification, useUpdateNotificationPreferences } from '../hooks/useVersionNotification'
import { UPDATE_CHECK_INTERVAL_MS, UPDATE_CHECK_RETRY_MS, UPDATE_CHECK_TIMEOUT_MS, UPDATE_NOTIFICATION_KEY, saveNotificationPreferences } from '../lib/updateNotifications'
import ChatVersionBadge from '../components/ChatVersionBadge'

vi.mock('../lib/api', () => ({ api: vi.fn() }))
vi.mock('../components/SystemUpdateModal', () => ({ default: ({ open, version, onClose }) => open ? <div data-testid="update-panel">{version.check?.latest?.tag_name}<button onClick={onClose}>Close update panel</button></div> : null }))
const candidate = { update: true, latest: { tag_name: 'v1.2.3', body: 'Notes' } }
const flush = async () => { await act(async () => { await Promise.resolve(); await Promise.resolve() }) }
const checks = () => api.mock.calls.filter(([path]) => path === '/api/version/check')
beforeEach(() => {
  vi.useFakeTimers(); vi.clearAllMocks(); localStorage.clear()
  Object.defineProperty(document, 'visibilityState', { configurable: true, value: 'visible' })
  api.mockImplementation(async path => {
    if (path === '/api/version/info') return { version: 'v1.2.2', update_supported: true }
    if (path === '/api/version/check') return candidate
    if (path === '/api/version/status') return { stage: '', running: false }
    return {}
  })
})
afterEach(() => { cleanup(); vi.useRealTimers() })
it('checks on start, reuses cache after remount, and polls only once per six hours', async () => {
  const hook = renderHook(useVersionNotification); await flush()
  expect(hook.result.current.release).toBe('v1.2.3'); expect(checks()).toHaveLength(1)
  act(() => document.dispatchEvent(new Event('visibilitychange'))); await flush()
  expect(checks()).toHaveLength(1)
  hook.unmount()
  const second = renderHook(useVersionNotification); await flush()
  expect(second.result.current.release).toBe('v1.2.3'); expect(checks()).toHaveLength(1)
  await act(async () => { await vi.advanceTimersByTimeAsync(UPDATE_CHECK_INTERVAL_MS) })
  expect(checks()).toHaveLength(2)
})
it('persistent disable cancels checks while retaining manual API access', async () => {
  saveNotificationPreferences({ enabled: false })
  const hook = renderHook(useVersionNotification); await flush()
  expect(api).not.toHaveBeenCalled()
  act(() => hook.result.current.setEnabled(true)); await flush()
  expect(checks()).toHaveLength(1)
  act(() => hook.result.current.setEnabled(false))
  await act(async () => { await vi.advanceTimersByTimeAsync(UPDATE_CHECK_INTERVAL_MS * 2) })
  expect(checks()).toHaveLength(1); expect(hook.result.current.release).toBe('')
})
it('legacy dismissal cannot hide an uninstalled release across remounts', async () => {
  saveNotificationPreferences({ dismissedVersion: 'v1.2.3' })
  const hook = renderHook(useVersionNotification); await flush()
  expect(hook.result.current.release).toBe('v1.2.3')
  hook.unmount(); const second = renderHook(useVersionNotification); await flush()
  expect(second.result.current.release).toBe('v1.2.3')
  api.mockImplementation(async path => path === '/api/version/info' ? { version: 'v1.2.2' } : { ...candidate, latest: { tag_name: 'v1.2.4' } })
  await act(async () => { await vi.advanceTimersByTimeAsync(UPDATE_CHECK_INTERVAL_MS) })
  expect(second.result.current.release).toBe('v1.2.4')
})
it('does not check in hidden pages and wakes on visibility', async () => {
  Object.defineProperty(document, 'visibilityState', { configurable: true, value: 'hidden' })
  const hook = renderHook(useVersionNotification); await flush(); expect(api).not.toHaveBeenCalled()
  Object.defineProperty(document, 'visibilityState', { configurable: true, value: 'visible' })
  act(() => document.dispatchEvent(new Event('visibilitychange'))); await flush()
  expect(hook.result.current.release).toBe('v1.2.3')
})
it('restores a detected release even when version info is temporarily unavailable', async () => {
  const hook = renderHook(useVersionNotification); await flush(); hook.unmount()
  api.mockRejectedValue(new Error('offline'))
  const second = renderHook(useVersionNotification); await flush()
  expect(second.result.current.release).toBe('v1.2.3')
})
it('keeps a detected release when a later check says no update but the running version is unchanged', async () => {
  const hook = renderHook(useVersionNotification); await flush()
  api.mockImplementation(async path => path === '/api/version/info' ? { version: 'v1.2.2' } : { update: false })
  await act(async () => { await vi.advanceTimersByTimeAsync(UPDATE_CHECK_INTERVAL_MS) })
  expect(hook.result.current.release).toBe('v1.2.3')
  hook.unmount()
  const second = renderHook(useVersionNotification); await flush()
  expect(second.result.current.release).toBe('v1.2.3')
})
it('silently retries failure after thirty minutes without rapid polling', async () => {
  api.mockImplementation(async path => { if (path === '/api/version/info') return { version: 'v1.2.2' }; throw new Error('rate limit') })
  const hook = renderHook(useVersionNotification); await flush()
  expect(hook.result.current.release).toBe(''); expect(checks()).toHaveLength(1)
  await act(async () => { await vi.advanceTimersByTimeAsync(UPDATE_CHECK_RETRY_MS - 1) })
  expect(checks()).toHaveLength(1)
  await act(async () => { await vi.advanceTimersByTimeAsync(1) }); expect(checks()).toHaveLength(2)
})
it('synchronizes preference changes from another tab', () => {
  const hook = renderHook(useUpdateNotificationPreferences)
  localStorage.setItem(UPDATE_NOTIFICATION_KEY, JSON.stringify({ enabled: false }))
  act(() => window.dispatchEvent(new StorageEvent('storage', { key: UPDATE_NOTIFICATION_KEY })))
  expect(hook.result.current.preferences.enabled).toBe(false)
})
it('aborts an in-flight check on disable and ignores its late result', async () => {
  let resolve, signal
  api.mockImplementation((path, options) => {
    if (path === '/api/version/info') return Promise.resolve({ version: 'v1.2.2' })
    signal = options.signal; return new Promise(done => { resolve = done })
  })
  const hook = renderHook(useVersionNotification); await flush()
  act(() => hook.result.current.setEnabled(false)); expect(signal.aborted).toBe(true)
  await act(async () => resolve(candidate)); expect(hook.result.current.release).toBe('')
})
it('bounds a stalled check and quietly retries later', async () => {
  let signal
  api.mockImplementation((path, options) => {
    if (path === '/api/version/info') return Promise.resolve({ version: 'v1.2.2' })
    signal = options.signal
    return new Promise((_, reject) => signal.addEventListener('abort', () => reject(new Error('aborted')), { once: true }))
  })
  const hook = renderHook(useVersionNotification); await flush()
  await act(async () => vi.advanceTimersByTime(UPDATE_CHECK_TIMEOUT_MS))
  expect(signal.aborted).toBe(true)
  expect(hook.result.current.release).toBe('')
  expect(checks()).toHaveLength(1)
  await act(async () => vi.advanceTimersByTime(UPDATE_CHECK_RETRY_MS))
  expect(checks()).toHaveLength(2)
})
it('shows only a red dot beside the chat version, with no banner or automatic dialog', async () => {
  render(<ChatVersionBadge version="v1.2.2"/>); await flush()
  const dot = screen.getByRole('img', { name: '有新版本' })
  expect(dot.className).toBe('oa-chat-version-dot')
  expect(dot.previousElementSibling.textContent).toBe('v1.2.2')
  expect(dot.getAttribute('title')).toBe('v1.2.3')
  expect(document.querySelector('.version-update-notice')).toBeNull()
  expect(screen.queryByTestId('update-panel')).toBeNull()
  expect(api.mock.calls.some(([, options]) => options?.method === 'POST')).toBe(false)
})
it('opening and closing the update panel keeps the dot across remounts', async () => {
  const view = render(<ChatVersionBadge version="v1.2.2"/>); await flush()
  fireEvent.click(screen.getByRole('button', { name: 'GenericAgent Admin v1.2.2' }))
  expect(screen.getByRole('img', { name: '有新版本' })).toBeTruthy()
  fireEvent.click(screen.getByRole('button', { name: '版本更新' })); await flush()
  expect(screen.getByRole('img', { name: '有新版本' })).toBeTruthy()
  expect(screen.getByTestId('update-panel').textContent).toContain('v1.2.3')
  expect(localStorage.getItem(UPDATE_NOTIFICATION_KEY)).toBeNull()
  fireEvent.click(screen.getByRole('button', { name: 'Close update panel' }))
  expect(screen.queryByTestId('update-panel')).toBeNull()
  expect(screen.getByRole('img', { name: '有新版本' })).toBeTruthy()
  expect(api.mock.calls.some(([, options]) => options?.method === 'POST')).toBe(false)
  view.unmount()
  render(<ChatVersionBadge version="v1.2.2"/>); await flush()
  expect(screen.getByRole('img', { name: '有新版本' })).toBeTruthy()
})
it('only clears the dot once the running version matches the release', async () => {
  const view = render(<ChatVersionBadge version="v1.2.2"/>); await flush()
  expect(screen.getByRole('img', { name: '有新版本' })).toBeTruthy()
  api.mockImplementation(async path => path === '/api/version/info' ? { version: 'v1.2.3' } : candidate)
  act(() => document.dispatchEvent(new Event('visibilitychange'))); await flush()
  expect(screen.queryByRole('img', { name: '有新版本' })).toBeNull()
  view.unmount()
  render(<ChatVersionBadge version="v1.2.3"/>); await flush()
  expect(screen.queryByRole('img', { name: '有新版本' })).toBeNull()
})
it('keeps a detected release during transient check failures and across remounts', async () => {
  const hook = renderHook(useVersionNotification); await flush()
  api.mockImplementation(async path => { if (path === '/api/version/info') return { version: 'v1.2.2' }; throw new Error('offline') })
  await act(async () => { await vi.advanceTimersByTimeAsync(UPDATE_CHECK_INTERVAL_MS) })
  expect(hook.result.current.release).toBe('v1.2.3')
  hook.unmount()
  const second = renderHook(useVersionNotification); await flush()
  expect(second.result.current.release).toBe('v1.2.3')
  await act(async () => { await vi.advanceTimersByTimeAsync(UPDATE_CHECK_RETRY_MS) })
  expect(second.result.current.release).toBe('v1.2.3')
})
it('does not show a dot when there is no new release or checking fails', async () => {
  api.mockImplementation(async path => path === '/api/version/info' ? { version: 'v1.2.2' } : { update: false })
  const view = render(<ChatVersionBadge version="v1.2.2"/>); await flush()
  expect(screen.queryByRole('img', { name: '有新版本' })).toBeNull()
  view.unmount(); localStorage.clear()
  api.mockRejectedValue(new Error('offline'))
  render(<ChatVersionBadge version="v1.2.2"/>); await flush()
  expect(screen.queryByRole('img', { name: '有新版本' })).toBeNull()
})
it('shows a dot for a future release after the previous release was seen', async () => {
  saveNotificationPreferences({ dismissedVersion: 'v1.2.2' })
  render(<ChatVersionBadge version="v1.2.1"/>); await flush()
  expect(screen.getByRole('img', { name: '有新版本' })).toBeTruthy()
})
