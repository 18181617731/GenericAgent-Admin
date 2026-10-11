import React from 'react'
import { afterEach, beforeEach, expect, it, vi } from 'vitest'
import { act, cleanup, fireEvent, render, renderHook, screen } from '@testing-library/react'
import { api } from '../lib/api'
import { useVersionNotification, useUpdateNotificationPreferences } from '../hooks/useVersionNotification'
import { UPDATE_CHECK_INTERVAL_MS, UPDATE_CHECK_RETRY_MS, UPDATE_CHECK_TIMEOUT_MS, UPDATE_NOTIFICATION_KEY, saveNotificationPreferences } from '../lib/updateNotifications'
import VersionUpdateNotification from '../components/VersionUpdateNotification'

vi.mock('../lib/api', () => ({ api: vi.fn() }))
vi.mock('../components/SystemUpdateModal', () => ({ default: ({ open, version }) => open ? <div data-testid="update-panel">{version.check?.latest?.tag_name}</div> : null }))
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
it('dismissal survives remount and a later release is announced', async () => {
  const hook = renderHook(useVersionNotification); await flush()
  act(() => hook.result.current.dismiss()); expect(hook.result.current.release).toBe('')
  hook.unmount(); const second = renderHook(useVersionNotification); await flush()
  expect(second.result.current.release).toBe('')
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
it('offers close, persistent opt-out and review without starting an update', async () => {
  render(<VersionUpdateNotification lang="en"/>); await flush()
  expect(screen.getByLabelText('New version available')).toBeTruthy()
  fireEvent.click(screen.getByRole('button', { name: 'View update' })); await flush()
  expect(screen.getByTestId('update-panel').textContent).toBe('v1.2.3')
  expect(screen.queryByLabelText('New version available')).toBeNull()
  expect(api.mock.calls.some(([, options]) => options?.method === 'POST')).toBe(false)
})
it('close dismisses this release without disabling future reminders', async () => {
  render(<VersionUpdateNotification lang="en"/>); await flush()
  fireEvent.click(screen.getByRole('button', { name: 'Dismiss this version' }))
  expect(screen.queryByLabelText('New version available')).toBeNull()
  expect(JSON.parse(localStorage.getItem(UPDATE_NOTIFICATION_KEY))).toEqual({ enabled: true, dismissedVersion: 'v1.2.3' })
})
it('opt-out button hides the notice and persists the disabled setting', async () => {
  render(<VersionUpdateNotification lang="en"/>); await flush()
  fireEvent.click(screen.getByRole('button', { name: 'Turn off reminders' }))
  expect(screen.queryByLabelText('New version available')).toBeNull()
  expect(JSON.parse(localStorage.getItem(UPDATE_NOTIFICATION_KEY)).enabled).toBe(false)
})
