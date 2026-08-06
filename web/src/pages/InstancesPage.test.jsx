import React from 'react'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { cleanup, render, screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import InstancesPage from './InstancesPage'

const reply = (payload, ok = true) => Promise.resolve({
  ok,
  status: ok ? 200 : 400,
  statusText: ok ? 'OK' : 'Bad Request',
  text: async () => JSON.stringify(payload),
})

const initialPayload = {
  default_instance_id: 'primary',
  items: [{ id: 'primary', name: 'Primary', ga_root: 'C:/ga', python_path: '', effective_python: 'C:/Python/python.exe' }],
}

afterEach(() => {
  cleanup()
  vi.restoreAllMocks()
})

describe('InstancesPage', () => {
  it('loads and identifies the default GA instance', async () => {
    globalThis.fetch = vi.fn(() => reply(initialPayload))
    render(<InstancesPage lang="en" />)

    expect(await screen.findByRole('heading', { name: 'Primary' })).not.toBeNull()
    expect(screen.getByText('Default')).not.toBeNull()
    expect(globalThis.fetch).toHaveBeenCalledWith('/api/instances', expect.objectContaining({ headers: expect.any(Object) }))
  })

  it('confirms one-click add and sends the dangerous confirmation header', async () => {
    const installed = {
      default_instance_id: 'primary',
      items: [...initialPayload.items, { id: 'genericagent', name: 'GenericAgent', ga_root: 'C:/admin/instances/genericagent', effective_python: 'python' }],
    }
    let finishInstall
    globalThis.fetch = vi.fn((url) => {
      if (url === '/api/instances') return reply(initialPayload)
      return new Promise(resolve => { finishInstall = () => resolve({
        ok: true,
        status: 200,
        statusText: 'OK',
        text: async () => JSON.stringify(installed),
      }) })
    })
    const confirmSpy = vi.spyOn(window, 'confirm').mockReturnValue(true)
    const user = userEvent.setup()
    render(<InstancesPage lang="en" />)

    await screen.findByRole('heading', { name: 'Primary' })
    await user.click(screen.getByRole('button', { name: 'One-click add' }))

    expect(confirmSpy).toHaveBeenCalledWith(expect.stringContaining('[install_instance]'))
    const pendingButton = screen.getByRole('button', { name: 'Downloading and adding\u2026' })
    expect(pendingButton.disabled).toBe(true)
    expect(pendingButton.getAttribute('aria-busy')).toBe('true')
    const [url, options] = globalThis.fetch.mock.calls[1]
    expect(url).toBe('/api/instances/install')
    expect(options.method).toBe('POST')
    expect(options.headers['X-GA-Confirm']).toBe('dangerous')

    finishInstall()
    expect(await screen.findByRole('heading', { name: 'GenericAgent' })).not.toBeNull()
    expect(screen.getByText('GenericAgent downloaded and added')).not.toBeNull()
  })

  it('confirms creation and sends the dangerous confirmation header', async () => {
    const created = {
      default_instance_id: 'primary',
      items: [...initialPayload.items, { id: 'secondary', name: 'Secondary', ga_root: 'D:/ga', effective_python: 'python' }],
    }
    globalThis.fetch = vi.fn((url) => url === '/api/instances' ? reply(initialPayload) : reply(created))
    vi.spyOn(window, 'confirm').mockReturnValue(true)
    const user = userEvent.setup()
    render(<InstancesPage lang="en" />)

    await screen.findByRole('heading', { name: 'Primary' })
    await user.click(screen.getByRole('button', { name: 'Add instance' }))
    await user.type(screen.getByLabelText('Instance ID'), 'secondary')
    await user.type(screen.getByLabelText('Display name'), 'Secondary')
    await user.type(screen.getByLabelText('GenericAgent root'), 'D:/ga')
    await user.click(screen.getByRole('button', { name: 'Create instance' }))

    await waitFor(() => expect(globalThis.fetch).toHaveBeenCalledTimes(2))
    const [url, options] = globalThis.fetch.mock.calls[1]
    expect(url).toBe('/api/instances/create')
    expect(options.method).toBe('POST')
    expect(options.headers['X-GA-Confirm']).toBe('dangerous')
    expect(JSON.parse(options.body)).toMatchObject({ id: 'secondary', name: 'Secondary', ga_root: 'D:/ga' })
    expect(await screen.findByRole('heading', { name: 'Secondary' })).not.toBeNull()
  })

  it('updates an instance without allowing its ID to change', async () => {
    const updated = {
      ...initialPayload,
      items: [{ ...initialPayload.items[0], name: 'Primary updated', python_path: 'C:/Python/python.exe' }],
    }
    globalThis.fetch = vi.fn((url) => url === '/api/instances' ? reply(initialPayload) : reply(updated))
    vi.spyOn(window, 'confirm').mockReturnValue(true)
    const user = userEvent.setup()
    render(<InstancesPage lang="en" />)

    const heading = await screen.findByRole('heading', { name: 'Primary' })
    await user.click(within(heading.closest('article')).getByRole('button', { name: 'Edit' }))
    expect(screen.getByLabelText('Instance ID').disabled).toBe(true)
    await user.clear(screen.getByLabelText('Display name'))
    await user.type(screen.getByLabelText('Display name'), 'Primary updated')
    await user.type(screen.getByLabelText('Python path'), 'C:/Python/python.exe')
    await user.click(screen.getByRole('button', { name: 'Save changes' }))

    await waitFor(() => expect(globalThis.fetch).toHaveBeenCalledTimes(2))
    const [url, options] = globalThis.fetch.mock.calls[1]
    expect(url).toBe('/api/instances/update')
    expect(options.method).toBe('PUT')
    expect(options.headers['X-GA-Confirm']).toBe('dangerous')
    expect(JSON.parse(options.body)).toEqual({
      id: 'primary',
      name: 'Primary updated',
      ga_root: 'C:/ga',
      python_path: 'C:/Python/python.exe',
    })
    expect(await screen.findByRole('heading', { name: 'Primary updated' })).not.toBeNull()
  })

  it('sets another default before deleting the previous default', async () => {
    const secondary = { id: 'secondary', name: 'Secondary', ga_root: 'D:/ga', python_path: '', effective_python: 'python' }
    const twoInstances = { ...initialPayload, items: [...initialPayload.items, secondary] }
    const secondaryDefault = { ...twoInstances, default_instance_id: 'secondary' }
    const secondaryOnly = { ...secondaryDefault, items: [secondary] }
    globalThis.fetch = vi.fn((url) => {
      if (url === '/api/instances') return reply(twoInstances)
      if (url === '/api/instances/default') return reply(secondaryDefault)
      return reply(secondaryOnly)
    })
    vi.spyOn(window, 'confirm').mockReturnValue(true)
    const user = userEvent.setup()
    render(<InstancesPage lang="en" />)

    const primaryHeading = await screen.findByRole('heading', { name: 'Primary' })
    const primaryCard = primaryHeading.closest('article')
    expect(within(primaryCard).getByRole('button', { name: 'Delete' }).disabled).toBe(true)
    const secondaryCard = screen.getByRole('heading', { name: 'Secondary' }).closest('article')
    await user.click(within(secondaryCard).getByRole('button', { name: 'Set as default' }))

    await waitFor(() => expect(globalThis.fetch).toHaveBeenCalledTimes(2))
    let [url, options] = globalThis.fetch.mock.calls[1]
    expect(url).toBe('/api/instances/default')
    expect(options.method).toBe('PUT')
    expect(options.headers['X-GA-Confirm']).toBe('dangerous')
    expect(JSON.parse(options.body)).toEqual({ id: 'secondary' })

    await user.click(within(screen.getByRole('heading', { name: 'Primary' }).closest('article')).getByRole('button', { name: 'Delete' }))
    await waitFor(() => expect(globalThis.fetch).toHaveBeenCalledTimes(3))
    ;[url, options] = globalThis.fetch.mock.calls[2]
    expect(url).toBe('/api/instances/delete')
    expect(options.method).toBe('DELETE')
    expect(options.headers['X-GA-Confirm']).toBe('dangerous')
    expect(JSON.parse(options.body)).toEqual({ id: 'primary' })
    await waitFor(() => expect(screen.queryByRole('heading', { name: 'Primary' })).toBeNull())
    expect(screen.getByRole('heading', { name: 'Secondary' })).not.toBeNull()
  })
})
