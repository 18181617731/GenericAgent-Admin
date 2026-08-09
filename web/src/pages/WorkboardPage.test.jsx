import React from 'react'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { cleanup, render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import WorkboardPage from './WorkboardPage'

const reply = (payload, ok = true) => Promise.resolve({ ok, status: ok ? 200 : 400, statusText: ok ? 'OK' : 'Bad Request', text: async () => JSON.stringify(payload) })
const statuses = ['backlog', 'active', 'review', 'done']

afterEach(() => { cleanup(); vi.restoreAllMocks() })

describe('WorkboardPage', () => {
  it('loads every workflow stage and creates a reviewed item', async () => {
    globalThis.fetch = vi.fn((url, options = {}) => options.method === 'POST'
      ? reply({ id: 'new12345', title: 'Ship runbook', owner: 'Ops', risk: 'high', status: 'backlog' })
      : reply({ statuses, items: [] }))
    vi.spyOn(window, 'confirm').mockReturnValue(true)
    const user = userEvent.setup()
    render(<WorkboardPage lang="en" />)

    expect(await screen.findByRole('heading', { name: 'Backlog' })).not.toBeNull()
    expect(screen.getByRole('heading', { name: 'Review' })).not.toBeNull()
    await user.type(screen.getByLabelText('Title'), 'Ship runbook')
    await user.type(screen.getByLabelText('Owner'), 'Ops')
    await user.selectOptions(screen.getByLabelText('Risk'), 'high')
    await user.click(screen.getByRole('button', { name: 'Add to backlog' }))

    expect(await screen.findByRole('heading', { name: 'Ship runbook' })).not.toBeNull()
    const [, options] = globalThis.fetch.mock.calls[1]
    expect(options.headers['X-GA-Confirm']).toBe('dangerous')
    expect(JSON.parse(options.body)).toEqual({ title: 'Ship runbook', owner: 'Ops', risk: 'high' })
  })

  it('moves only one stage and sends the dangerous confirmation header', async () => {
    const item = { id: 'task1234', title: 'Review deploy', owner: '', risk: 'medium', status: 'active' }
    globalThis.fetch = vi.fn((url, options = {}) => options.method === 'PATCH' ? reply({ ...item, status: 'review' }) : reply({ statuses, items: [item] }))
    vi.spyOn(window, 'confirm').mockReturnValue(true)
    const user = userEvent.setup()
    render(<WorkboardPage lang="en" />)

    await screen.findByRole('heading', { name: 'Review deploy' })
    await user.click(screen.getByRole('button', { name: 'Move forward: Review deploy' }))
    await waitFor(() => expect(globalThis.fetch).toHaveBeenCalledTimes(2))
    const [url, options] = globalThis.fetch.mock.calls[1]
    expect(url).toBe('/api/workboard/task1234')
    expect(options.method).toBe('PATCH')
    expect(options.headers['X-GA-Confirm']).toBe('dangerous')
    expect(JSON.parse(options.body)).toEqual({ status: 'review' })
  })
})
