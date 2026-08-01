import React from 'react'
import { afterEach, describe, expect, test, vi } from 'vitest'
import { cleanup, render, screen, waitFor } from '@testing-library/react'
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

  test('blocks the application and validates password confirmation', async () => {
    vi.stubGlobal('fetch', vi.fn().mockResolvedValue(response({ username: 'admin', mustChangePassword: true })))
    const user = userEvent.setup()
    render(<AuthGate><div>Admin application</div></AuthGate>)
    expect(await screen.findByRole('heading', { name: /change the default password/i })).toBeTruthy()
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
    await waitFor(() => expect(screen.queryByRole('heading', { name: /change the default password/i })).toBeNull())
  })
})
