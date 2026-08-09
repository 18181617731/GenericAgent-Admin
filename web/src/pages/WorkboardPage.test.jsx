import React from 'react'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { cleanup, render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import WorkboardPage from './WorkboardPage'

const reply = (payload, ok = true, status = ok ? 200 : 409) => Promise.resolve({ ok, status, statusText: ok ? 'OK' : 'Conflict', text: async () => JSON.stringify(payload) })
const statuses = ['backlog', 'active', 'review', 'done']
const baseItem = {
  id: 'task1234', title: 'Review deploy', outcome: 'Deployment is verified',
  acceptance_criteria: ['Smoke test passes'], owner: 'Ops', risk: 'medium',
  status: 'active', revision: 3, proposal: { summary: '', evidence: [] }, events: [],
}

afterEach(() => { cleanup(); vi.restoreAllMocks() })

describe('WorkboardPage', () => {
  it('creates a persisted decision contract', async () => {
    globalThis.fetch = vi.fn((url, options = {}) => options.method === 'POST'
      ? reply({ ...baseItem, id: 'new12345', title: 'Ship runbook', outcome: 'Operators can recover service', acceptance_criteria: ['Rollback tested', 'Owner signed off'], status: 'backlog', revision: 1 })
      : reply({ statuses, items: [] }))
    vi.spyOn(window, 'confirm').mockReturnValue(true)
    const user = userEvent.setup()
    render(<WorkboardPage lang="en" />)

    expect(await screen.findByRole('heading', { name: 'Backlog' })).not.toBeNull()
    await user.type(screen.getByLabelText('Title'), 'Ship runbook')
    await user.type(screen.getByLabelText('Required outcome'), 'Operators can recover service')
    await user.type(screen.getByLabelText('Acceptance criteria'), 'Rollback tested\nOwner signed off')
    await user.type(screen.getByLabelText('Owner'), 'Ops')
    await user.selectOptions(screen.getByLabelText('Risk'), 'high')
    await user.click(screen.getByRole('button', { name: 'Add contract' }))

    expect(await screen.findByText('Ship runbook')).not.toBeNull()
    const [, options] = globalThis.fetch.mock.calls[1]
    expect(options.headers['X-GA-Confirm']).toBe('dangerous')
    expect(JSON.parse(options.body)).toEqual({
      title: 'Ship runbook', outcome: 'Operators can recover service',
      acceptance_criteria: ['Rollback tested', 'Owner signed off'], owner: 'Ops', risk: 'high',
    })
  })

  it('submits structured evidence with the visible revision', async () => {
    const reviewed = { ...baseItem, status: 'review', revision: 4, proposal: { summary: 'Safe to release', evidence: [{ label: 'Smoke test', detail: 'Passed in staging' }] }, events: [{ action: 'submit_proposal', actor: 'user', revision: 4 }] }
    globalThis.fetch = vi.fn((url, options = {}) => options.method === 'POST' ? reply(reviewed) : reply({ statuses, items: [baseItem] }))
    vi.spyOn(window, 'confirm').mockReturnValue(true)
    const user = userEvent.setup()
    render(<WorkboardPage lang="en" />)

    await user.click(await screen.findByRole('button', { name: /Review deploy/ }))
    await user.type(screen.getByLabelText('Agent proposal'), 'Safe to release')
    await user.type(screen.getByLabelText('Evidence label'), 'Smoke test')
    await user.type(screen.getByLabelText('What was verified and where?'), 'Passed in staging')
    await user.click(screen.getByRole('button', { name: 'Submit for review' }))

    await waitFor(() => expect(globalThis.fetch).toHaveBeenCalledTimes(2))
    const [url, options] = globalThis.fetch.mock.calls[1]
    expect(url).toBe('/api/workboard/task1234/commands')
    expect(options.method).toBe('POST')
    expect(options.headers['X-GA-Confirm']).toBe('dangerous')
    expect(JSON.parse(options.body)).toEqual({ action: 'submit_proposal', expected_revision: 3, proposal: 'Safe to release', evidence: [{ label: 'Smoke test', detail: 'Passed in staging' }] })
    expect(await screen.findByText('Revision 4 · Review')).not.toBeNull()
  })

  it('keeps the detail open and surfaces a stale revision conflict', async () => {
    globalThis.fetch = vi.fn((url, options = {}) => options.method === 'POST' ? reply({ error: 'work item revision conflict' }, false) : reply({ statuses, items: [{ ...baseItem, status: 'backlog' }] }))
    vi.spyOn(window, 'confirm').mockReturnValue(true)
    const user = userEvent.setup()
    render(<WorkboardPage lang="en" />)

    await user.click(await screen.findByRole('button', { name: /Review deploy/ }))
    await user.click(screen.getByRole('button', { name: 'Start work' }))
    expect((await screen.findByRole('alert')).textContent).toContain('work item revision conflict')
    expect(screen.getByRole('dialog')).not.toBeNull()
  })
})
