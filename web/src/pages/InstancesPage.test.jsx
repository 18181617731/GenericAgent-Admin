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

  it('uploads an optional ZIP template as multipart form data', async () => {
    const installed = {
      default_instance_id: 'primary',
      items: [...initialPayload.items, { id: 'uploaded', name: 'uploaded', ga_root: 'C:/admin/uploaded', init_status: 'initializing' }],
      instance: { id: 'uploaded', name: 'uploaded', ga_root: 'C:/admin/uploaded', init_status: 'initializing' },
    }
    globalThis.fetch = vi.fn(url => url === '/api/instances/install' ? reply(installed) : reply(initialPayload))
    vi.spyOn(window, 'confirm').mockReturnValue(true)
    const user = userEvent.setup()
    render(<InstancesPage lang="en" />)

    await screen.findByRole('heading', { name: 'Primary' })
    await user.click(screen.getByRole('button', { name: 'One-click add' }))
    await user.type(screen.getByLabelText('Instance ID'), 'uploaded')
    const archive = new File(['zip fixture'], 'GA.zip', { type: 'application/zip' })
    await user.upload(screen.getByLabelText('GA.zip template (optional)'), archive)
    await user.click(screen.getByRole('button', { name: 'Start creating' }))

    await waitFor(() => expect(globalThis.fetch).toHaveBeenCalledTimes(2))
    const [url, options] = globalThis.fetch.mock.calls[1]
    expect(url).toBe('/api/instances/install')
    expect(options.headers['X-GA-Confirm']).toBe('dangerous')
    expect(options.headers['Content-Type']).toBeUndefined()
    expect(options.body).toBeInstanceOf(FormData)
    expect(options.body.get('id')).toBe('uploaded')
    expect(options.body.get('template').name).toBe('GA.zip')
  })

  it('adds an initializing instance immediately and polls it to ready', async () => {
    const initializing = {
      default_instance_id: 'primary',
      items: [...initialPayload.items, {
        id: 'genericagent',
        name: 'GenericAgent',
        ga_root: 'C:/admin/genericagent',
        effective_python: 'python',
        init_status: 'initializing',
      }],
    }
    const ready = {
      ...initializing,
      items: initializing.items.map(item => item.id === 'genericagent' ? { ...item, init_status: 'ready' } : item),
    }
    let listCalls = 0
    globalThis.fetch = vi.fn((url) => {
      if (url === '/api/instances/install') return reply(initializing)
      listCalls += 1
      return reply(listCalls === 1 ? initialPayload : ready)
    })
    const confirmSpy = vi.spyOn(window, 'confirm').mockReturnValue(true)
    const user = userEvent.setup()
    render(<InstancesPage lang="en" />)

    await screen.findByRole('heading', { name: 'Primary' })
    await user.click(screen.getByRole('button', { name: 'One-click add' }))
    expect(screen.getByRole('heading', { name: 'Choose the new instance ID' })).not.toBeNull()
    expect(screen.queryByLabelText('Display name')).toBeNull()
    expect(screen.queryByLabelText('GenericAgent root')).toBeNull()
    await user.type(screen.getByLabelText('Instance ID'), 'genericagent')
    await user.click(screen.getByRole('button', { name: 'Start creating' }))

    expect(confirmSpy).toHaveBeenCalledWith(expect.stringContaining('[install_instance]'))
    const [url, options] = globalThis.fetch.mock.calls[1]
    expect(url).toBe('/api/instances/install')
    expect(options.method).toBe('POST')
    expect(options.headers['X-GA-Confirm']).toBe('dangerous')
    expect(JSON.parse(options.body)).toEqual({ id: 'genericagent' })

    const installedHeading = await screen.findByRole('heading', { name: 'GenericAgent' })
    const installedCard = installedHeading.closest('article')
    expect(within(installedCard).getByText('Initializing')).not.toBeNull()
    expect(within(installedCard).getByText('C:/admin/genericagent')).not.toBeNull()
    expect(within(installedCard).getByRole('button', { name: 'Edit' }).disabled).toBe(true)
    expect(within(installedCard).getByRole('button', { name: 'Set as default' }).disabled).toBe(true)
    expect(within(installedCard).getByRole('button', { name: 'Delete' }).disabled).toBe(false)
    expect(screen.getByRole('button', { name: 'One-click add' }).disabled).toBe(false)
    expect(screen.getByText('Instance added and initializing in the background')).not.toBeNull()

    await waitFor(() => expect(within(installedCard).getByText('Ready')).not.toBeNull(), { timeout: 3000 })
    expect(within(installedCard).getByRole('button', { name: 'Edit' }).disabled).toBe(false)
    expect(within(installedCard).getByRole('button', { name: 'Set as default' }).disabled).toBe(false)
    expect(listCalls).toBe(2)
  })

  it('shows initialization failure details and stops polling', async () => {
    const initializing = {
      default_instance_id: 'primary',
      items: [...initialPayload.items, {
        id: 'genericagent',
        name: 'GenericAgent',
        ga_root: 'C:/admin/genericagent',
        effective_python: 'python',
        init_status: 'initializing',
      }],
    }
    const failed = {
      ...initializing,
      items: initializing.items.map(item => item.id === 'genericagent' ? {
        ...item,
        init_status: 'failed',
        init_error: 'download failed: test network unavailable',
      } : item),
    }
    let listCalls = 0
    globalThis.fetch = vi.fn(() => {
      listCalls += 1
      return reply(listCalls === 1 ? initializing : failed)
    })
    render(<InstancesPage lang="en" />)

    const installedHeading = await screen.findByRole('heading', { name: 'GenericAgent' })
    const installedCard = installedHeading.closest('article')
    expect(within(installedCard).getByText('Initializing')).not.toBeNull()
    await waitFor(() => expect(within(installedCard).getByText('Initialization failed')).not.toBeNull(), { timeout: 3000 })
    expect(within(installedCard).getByText('download failed: test network unavailable')).not.toBeNull()
    expect(within(installedCard).getByRole('button', { name: 'Edit' }).disabled).toBe(false)
    expect(within(installedCard).getByRole('button', { name: 'Set as default' }).disabled).toBe(false)

    const callsAfterFailure = listCalls
    await new Promise(resolve => window.setTimeout(resolve, 1400))
    expect(listCalls).toBe(callsAfterFailure)
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
