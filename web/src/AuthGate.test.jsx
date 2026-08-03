import React from 'react'
import { afterEach, describe, expect, test, vi } from 'vitest'
import { cleanup, render, screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { AuthGate } from './components/AuthGate.jsx'

const response = (body, status = 200) => ({
  ok: status >= 200 && status < 300,
  status,
  json: async () => body,
})

afterEach(() => {
  cleanup()
  vi.restoreAllMocks()
})

describe('AuthGate', () => {
  test('renders the application when no password change is required', async () => {
    vi.stubGlobal('fetch', vi.fn().mockResolvedValue(response({ username: 'admin', mustChangePassword: false })))
    render(<AuthGate><div>Admin application</div></AuthGate>)
    expect(await screen.findByText('Admin application')).toBeTruthy()
  })

  test('exposes working language and theme controls during first-time setup', async () => {
    vi.stubGlobal('fetch', vi.fn().mockResolvedValue(response({ username: 'admin', mustChangePassword: true, initialized: false })))
    const user = userEvent.setup()
    const onLanguageChange = vi.fn()
    const onThemeChange = vi.fn()

    const { rerender } = render(<AuthGate lang="en" theme="warm" onLanguageChange={onLanguageChange} onThemeChange={onThemeChange}><div>Admin application</div></AuthGate>)
    await screen.findByRole('heading', { name: 'Set the administrator password' })

    const language = screen.getByRole('group', { name: 'Language' })
    await user.click(within(language).getByRole('button', { name: '中' }))
    expect(onLanguageChange).toHaveBeenCalledWith('zh')

    rerender(<AuthGate lang="zh" theme="warm" onLanguageChange={onLanguageChange} onThemeChange={onThemeChange}><div>Admin application</div></AuthGate>)
    expect(screen.getByRole('heading', { name: '设置管理员密码' })).toBeTruthy()
    expect(within(screen.getByRole('group', { name: '语言' })).getByRole('button', { name: '中' }).getAttribute('aria-pressed')).toBe('true')

    await user.click(screen.getByRole('button', { name: /外观/ }))
    await user.click((await screen.findByText('深色')).closest('button'))
    expect(onThemeChange).toHaveBeenCalledWith('dark')
  })

  test('sets the first administrator password without asking for a current credential', async () => {
    const fetchMock = vi.fn()
      .mockResolvedValueOnce(response({ username: 'admin', mustChangePassword: true, initialized: false }))
      .mockResolvedValueOnce(response({ ok: true, mustChangePassword: false, initialized: true }))
    vi.stubGlobal('fetch', fetchMock)
    const user = userEvent.setup()

    render(<AuthGate theme="warm"><div>Admin application</div></AuthGate>)
    expect(await screen.findByRole('heading', { name: 'Set the administrator password' })).toBeTruthy()
    expect(screen.queryByLabelText('Current password')).toBeNull()
    expect(screen.getByLabelText('New password').minLength).toBe(8)

    await user.type(screen.getByLabelText('New password'), 'A-first-password-1')
    await user.type(screen.getByLabelText('Confirm new password'), 'A-first-password-1')
    await user.click(screen.getByRole('button', { name: 'Set password' }))

    expect(await screen.findByText('Admin application')).toBeTruthy()
    expect(fetchMock).toHaveBeenNthCalledWith(2, '/api/auth/change-password', expect.objectContaining({
      method: 'POST',
      body: JSON.stringify({ currentPassword: '', newPassword: 'A-first-password-1', confirmPassword: 'A-first-password-1' }),
    }))
  })

  test('localizes first-run setup and exposes the appearance picker portal', async () => {
    vi.stubGlobal('fetch', vi.fn().mockResolvedValue(response({ username: 'admin', mustChangePassword: true, initialized: false })))
    const onThemeChange = vi.fn()
    const user = userEvent.setup()

    render(<AuthGate lang="zh" theme="warm" onThemeChange={onThemeChange}><div>Admin application</div></AuthGate>)
    expect(await screen.findByRole('heading', { name: '设置管理员密码' })).toBeTruthy()
    expect(screen.getByLabelText('新密码')).toBeTruthy()
    expect(screen.queryByLabelText('当前密码')).toBeNull()

    await user.click(screen.getByRole('button', { name: /外观.*暖色/ }))
    const dialog = await screen.findByRole('dialog', { name: '选择外观' })
    expect(document.body.contains(dialog)).toBe(true)
    await user.click(within(dialog).getByRole('radio', { name: /浅色/ }))
    expect(onThemeChange).toHaveBeenCalledWith('light')
  })

  test('blocks the application and validates password confirmation', async () => {
    vi.stubGlobal('fetch', vi.fn().mockResolvedValue(response({ username: 'admin', mustChangePassword: true })))
    const user = userEvent.setup()
    render(<AuthGate><div>Admin application</div></AuthGate>)
    expect(await screen.findByRole('heading', { name: /change the administrator password/i })).toBeTruthy()
    expect(screen.queryByText('Admin application')).toBeNull()

    await user.type(screen.getByLabelText('Current password'), 'admin')
    await user.type(screen.getByLabelText('New password'), 'A-long-new-password-1')
    await user.type(screen.getByLabelText('Confirm new password'), 'different-password')
    await user.click(screen.getByRole('button', { name: /set new password/i }))
    expect((await screen.findByRole('alert')).textContent).toContain('The passwords do not match.')
    expect(fetch).toHaveBeenCalledTimes(1)
  })

  test('posts the new credential and unlocks the application', async () => {
    const fetchMock = vi.fn()
      .mockResolvedValueOnce(response({ username: 'admin', mustChangePassword: true }))
      .mockResolvedValueOnce(response({ ok: true, mustChangePassword: false }))
    vi.stubGlobal('fetch', fetchMock)
    const user = userEvent.setup()
    render(<AuthGate><div>Admin application</div></AuthGate>)
    await screen.findByLabelText('Current password')
    await user.type(screen.getByLabelText('Current password'), 'admin')
    await user.type(screen.getByLabelText('New password'), 'A-long-new-password-1')
    await user.type(screen.getByLabelText('Confirm new password'), 'A-long-new-password-1')
    await user.click(screen.getByRole('button', { name: /set new password/i }))

    expect(await screen.findByText('Admin application')).toBeTruthy()
    expect(fetchMock).toHaveBeenNthCalledWith(2, '/api/auth/change-password', expect.objectContaining({
      method: 'POST',
      body: JSON.stringify({ currentPassword: 'admin', newPassword: 'A-long-new-password-1', confirmPassword: 'A-long-new-password-1' }),
    }))
    await waitFor(() => expect(screen.queryByRole('heading', { name: /change the administrator password/i })).toBeNull())
  })
})
