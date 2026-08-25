import React from 'react'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { cleanup, render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { KeychainPage } from './KeychainPage'
import { SETTINGS_TEXT } from '../lib/i18n'

const reply = payload => Promise.resolve({
  ok: true,
  status: 200,
  statusText: 'OK',
  text: async () => JSON.stringify(payload),
})

const setup = (initial = ['ALPHA_KEY', 'BETA_KEY']) => {
  let keys = [...initial]
  const calls = []
  globalThis.fetch = vi.fn((url, options = {}) => {
    calls.push([url, options])
    const body = options.body ? JSON.parse(options.body) : null
    if (options.method === 'PUT') keys = [...new Set([...keys, body.name])].sort()
    if (options.method === 'DELETE') keys = keys.filter(key => key !== body.name)
    return reply({ keys })
  })
  render(<KeychainPage text={SETTINGS_TEXT.en}/>)
  return calls
}

afterEach(() => {
  cleanup()
  vi.restoreAllMocks()
})

describe('KeychainPage', () => {
  it('lists names and never renders secret values', async () => {
    setup()
    expect(await screen.findByText('ALPHA_KEY')).not.toBeNull()
    expect(screen.getByText('BETA_KEY')).not.toBeNull()
    expect(screen.queryByText('actual-secret')).toBeNull()
    expect(screen.getByText('2')).not.toBeNull()
  })

  it('writes a key with dangerous confirmation and clears the secret input', async () => {
    const calls = setup([])
    vi.spyOn(window, 'confirm').mockReturnValue(true)
    const user = userEvent.setup()

    await screen.findByText('No keys saved')
    await user.type(screen.getByLabelText('Name'), 'NEW_KEY')
    const secret = screen.getByLabelText('Secret value')
    await user.type(secret, 'actual-secret')
    await user.click(screen.getByRole('button', { name: 'Save key' }))

    await screen.findByText('NEW_KEY')
    const put = calls.find(([, options]) => options.method === 'PUT')
    expect(put[1].headers['X-GA-Confirm']).toBe('dangerous')
    expect(JSON.parse(put[1].body)).toEqual({ name: 'NEW_KEY', value: 'actual-secret' })
    expect(secret.value).toBe('')
    expect(screen.queryByText('actual-secret')).toBeNull()
  })

  it('deletes by name with dangerous confirmation', async () => {
    const calls = setup(['REMOVE_ME'])
    vi.spyOn(window, 'confirm').mockReturnValue(true)
    const user = userEvent.setup()

    await screen.findByText('REMOVE_ME')
    await user.click(screen.getByRole('button', { name: 'Remove REMOVE_ME' }))
    await waitFor(() => expect(screen.queryByText('REMOVE_ME')).toBeNull())

    const remove = calls.find(([, options]) => options.method === 'DELETE')
    expect(remove[1].headers['X-GA-Confirm']).toBe('dangerous')
    expect(JSON.parse(remove[1].body)).toEqual({ name: 'REMOVE_ME' })
  })
})
