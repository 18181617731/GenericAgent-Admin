import React from 'react'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { cleanup, render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import SetupWizard from './SetupWizard.jsx'
import { SETUP_TEXT } from '../lib/i18n.js'

const copy = SETUP_TEXT.en

// antd's responsive Steps queries matchMedia, which jsdom does not implement.
const installMatchMedia = () => {
  Object.defineProperty(window, 'matchMedia', {
    writable: true,
    configurable: true,
    value: vi.fn().mockImplementation(query => ({
      matches: false,
      media: query,
      onchange: null,
      addListener: vi.fn(),
      removeListener: vi.fn(),
      addEventListener: vi.fn(),
      removeEventListener: vi.fn(),
      dispatchEvent: vi.fn(),
    })),
  })
}

const reply = (payload, ok = true) => Promise.resolve({
  ok,
  status: ok ? 200 : 400,
  statusText: ok ? 'OK' : 'Bad Request',
  text: async () => JSON.stringify(payload),
})

const HEALTHY_ENV = {
  ok: true,
  checked: '2026-08-14T00:00:00Z',
  tools: [
    { name: 'git', ok: true, version: 'git version 2.44.0' },
    { name: 'python', ok: true, version: 'Python 3.12.1' },
    { name: 'uv', ok: false, error: 'uv not found' },
    { name: 'npm', ok: true, version: '10.5.0' },
  ],
  python_installer: true,
  effective_python: 'C:/ga/.venv/Scripts/python.exe',
}

const readyState = (extra = {}) => ({
  ok: true,
  bootstrap_done: false,
  ga_root: 'C:/ga',
  python: 'C:/ga/.venv/Scripts/python.exe',
  health: { ok: true },
  venv: { ok: true, path: 'C:/ga/.venv', python: 'C:/ga/.venv/Scripts/python.exe' },
  ...extra,
})

const mockBackend = ({ state = readyState(), env = HEALTHY_ENV, routes = {}, onCall } = {}) => {
  const fetchMock = vi.fn((url, options = {}) => {
    const path = String(url)
    onCall?.(path, options)
    if (path.includes('/api/setup/state')) return reply(state)
    if (path.includes('/api/setup/env')) return reply(env)
    for (const [route, payload] of Object.entries(routes)) {
      if (path.includes(route)) return typeof payload === 'function' ? payload(options) : reply(payload)
    }
    return reply({ ok: true })
  })
  globalThis.fetch = fetchMock
  return fetchMock
}

const renderWizard = (props = {}) => {
  installMatchMedia()
  return render(<SetupWizard lang="en" text={copy} {...props} />)
}

const button = (name) => screen.getByRole('button', { name: new RegExp(name, 'i') })

afterEach(() => {
  cleanup()
  vi.restoreAllMocks()
  window.localStorage.clear()
})

describe('first-run wizard step model', () => {
  it('reports every probed tool, including the optional ones the server checks', async () => {
    mockBackend()
    renderWizard()
    await waitFor(() => expect(screen.getByText('git version 2.44.0')).toBeTruthy())
    expect(screen.getByText('Python 3.12.1')).toBeTruthy()
    expect(screen.getByText('uv not found')).toBeTruthy()
    expect(screen.getByText('10.5.0')).toBeTruthy()
    expect(screen.getByText(copy.env.checkedAt(HEALTHY_ENV.checked))).toBeTruthy()
  })

  // The wizard is the screen shown when GA health fails, so a saved-but-broken
  // root has to be called out rather than treated as a completed first step.
  it('explains why a configured but unhealthy root cannot finish', async () => {
    mockBackend({ state: readyState({ health: { ok: false }, venv: { ok: false } }) })
    renderWizard()
    await waitFor(() => expect(screen.getByText(copy.root.unhealthy)).toBeTruthy())
    expect(screen.getByText(copy.runtime.blocked.unhealthyRoot)).toBeTruthy()
    expect(button(copy.runtime.finish).disabled).toBe(true)
  })

  it('states the missing interpreter instead of silently disabling finish', async () => {
    mockBackend({ state: readyState({ venv: { ok: false, path: 'C:/ga/.venv' } }) })
    renderWizard()
    await waitFor(() => expect(screen.getByText(copy.runtime.blocked.noInterpreter)).toBeTruthy())
    expect(button(copy.runtime.finish).disabled).toBe(true)
    expect(screen.getByText(copy.runtime.venvMissing)).toBeTruthy()
  })

  it('enables finish on a healthy root with a venv the server can see', async () => {
    mockBackend()
    renderWizard()
    await waitFor(() => expect(button(copy.runtime.finish).disabled).toBe(false))
    // Dependencies are unproven on this machine, so the caveat replaces the block.
    expect(screen.getByText(copy.runtime.depsUnconfirmed)).toBeTruthy()
  })

  // Regression: the smoke result used to live in component state only, so a
  // reload disabled finish until the user ran the smoke test all over again.
  it('keeps a previously verified interpreter across a remount', async () => {
    const state = readyState({ venv: { ok: false, path: 'C:/ga/.venv' } })
    mockBackend({ state, routes: { '/api/setup/smoke': { ok: true, root: 'C:/ga', python: 'C:/py/python.exe' } } })
    vi.spyOn(window, 'confirm').mockReturnValue(true)

    renderWizard()
    await waitFor(() => expect(button(copy.runtime.smoke).disabled).toBe(false))
    await userEvent.click(button(copy.runtime.smoke))
    await waitFor(() => expect(button(copy.runtime.finish).disabled).toBe(false))

    cleanup()
    mockBackend({ state })
    renderWizard()
    await waitFor(() => expect(button(copy.runtime.finish).disabled).toBe(false))
    expect(screen.queryByText(copy.runtime.blocked.noInterpreter)).toBeNull()
  }, 20000)

  it('records a failed smoke test against the root and surfaces the error', async () => {
    mockBackend({
      state: readyState({ venv: { ok: false, path: 'C:/ga/.venv' } }),
      routes: { '/api/setup/smoke': () => reply({ error: 'interpreter exploded' }, false) },
    })
    vi.spyOn(window, 'confirm').mockReturnValue(true)
    renderWizard()
    await waitFor(() => expect(button(copy.runtime.smoke).disabled).toBe(false))
    await userEvent.click(button(copy.runtime.smoke))
    await waitFor(() => expect(screen.getByText('interpreter exploded')).toBeTruthy())
    expect(button(copy.runtime.finish).disabled).toBe(true)
  }, 20000)
})

describe('first-run wizard actions', () => {
  it('asks for confirmation and sends the dangerous header when validating a root', async () => {
    const calls = []
    mockBackend({
      state: readyState({ ga_root: '', health: undefined, venv: undefined }),
      routes: { '/api/setup/validate': { ok: true, root: 'C:/ga', health: { ok: true } } },
      onCall: (path, options) => calls.push([path, options]),
    })
    const confirm = vi.spyOn(window, 'confirm').mockReturnValue(true)
    renderWizard()

    await waitFor(() => expect(screen.getByPlaceholderText(copy.root.existingPlaceholder)).toBeTruthy())
    await userEvent.type(screen.getByPlaceholderText(copy.root.existingPlaceholder), 'C:/ga')
    await userEvent.click(button(copy.root.validate))

    await waitFor(() => expect(calls.some(([path]) => path.includes('/api/setup/validate'))).toBe(true))
    expect(confirm).toHaveBeenCalledWith(expect.stringContaining('C:/ga'))
    const [, options] = calls.find(([path]) => path.includes('/api/setup/validate'))
    expect(options.headers['X-GA-Confirm']).toBe('dangerous')
    expect(options.method).toBe('POST')
  })

  it('does not call the backend when the confirmation is declined', async () => {
    const calls = []
    mockBackend({ onCall: (path) => calls.push(path) })
    vi.spyOn(window, 'confirm').mockReturnValue(false)
    renderWizard()

    await waitFor(() => expect(button(copy.runtime.createVenv).disabled).toBe(false))
    await userEvent.click(button(copy.runtime.createVenv))
    expect(calls.some(path => path.includes('/api/setup/venv/create'))).toBe(false)
  })

  // The server exposes a native directory picker that the previous wizard never
  // called, leaving users to hand-type Windows paths.
  it('fills the root field from the native directory picker', async () => {
    mockBackend({
      state: readyState({ ga_root: '' }),
      routes: { '/api/setup/browse': { ok: true, path: 'D:/picked/GenericAgent' } },
    })
    renderWizard()

    await waitFor(() => expect(screen.getAllByRole('button', { name: /Browse/i }).length).toBe(2))
    await userEvent.click(screen.getAllByRole('button', { name: /Browse/i })[0])
    await waitFor(() => expect(screen.getByPlaceholderText(copy.root.existingPlaceholder).value).toBe('D:/picked/GenericAgent'))
  })

  it('reports a cancelled directory picker without changing the field', async () => {
    mockBackend({
      state: readyState({ ga_root: '' }),
      routes: { '/api/setup/browse': { ok: false, cancelled: true } },
    })
    renderWizard()

    await waitFor(() => expect(screen.getAllByRole('button', { name: /Browse/i }).length).toBe(2))
    await userEvent.click(screen.getAllByRole('button', { name: /Browse/i })[0])
    await waitFor(() => expect(screen.getByText(copy.messages.browseCancelled)).toBeTruthy())
    expect(screen.getByPlaceholderText(copy.root.existingPlaceholder).value).toBe('')
  })

  it('previews the directory the install action will create', async () => {
    mockBackend({ state: readyState({ ga_root: '' }) })
    renderWizard()

    await waitFor(() => expect(screen.getByPlaceholderText(copy.root.installPlaceholder)).toBeTruthy())
    await userEvent.type(screen.getByPlaceholderText(copy.root.installPlaceholder), 'D:\\code')
    await waitFor(() => expect(screen.getByText('D:\\code\\GenericAgent')).toBeTruthy())
  })

  it('streams dependency output and then unlocks finishing without a caveat', async () => {
    const chunks = [
      `${JSON.stringify({ type: 'start', line: 'python -m pip install -e .' })}\n`,
      `${JSON.stringify({ line: 'Collecting requests' })}\n${JSON.stringify({ line: 'Successfully installed' })}\n`,
      `${JSON.stringify({ type: 'done', ok: true, code: 0 })}\n`,
    ]
    mockBackend({
      routes: {
        '/api/setup/deps/install': () => Promise.resolve({
          ok: true,
          status: 200,
          statusText: 'OK',
          body: {
            getReader: () => {
              let index = 0
              return {
                read: async () => index < chunks.length
                  ? { value: new TextEncoder().encode(chunks[index++]), done: false }
                  : { value: undefined, done: true },
              }
            },
          },
        }),
      },
    })
    vi.spyOn(window, 'confirm').mockReturnValue(true)
    renderWizard()

    await waitFor(() => expect(button(copy.runtime.installDeps).disabled).toBe(false))
    await userEvent.click(button(copy.runtime.installDeps))

    await waitFor(() => expect(screen.getByText(/Successfully installed/)).toBeTruthy())
    expect(screen.getByText(/Collecting requests/)).toBeTruthy()
    await waitFor(() => expect(screen.getByText(copy.messages.depsDone)).toBeTruthy())
    expect(screen.queryByText(copy.runtime.depsUnconfirmed)).toBeNull()
  })

  it('hands the saved configuration back when setup completes', async () => {
    const onComplete = vi.fn()
    mockBackend({
      routes: { '/api/setup/complete': { ok: true, root: 'C:/ga', config: { ga_root: 'C:/ga' } } },
    })
    vi.spyOn(window, 'confirm').mockReturnValue(true)
    renderWizard({ onComplete })

    await waitFor(() => expect(button(copy.runtime.finish).disabled).toBe(false))
    await userEvent.click(button(copy.runtime.finish))
    await waitFor(() => expect(onComplete).toHaveBeenCalledWith(expect.objectContaining({ root: 'C:/ga' })))
  })

  it('stays usable when the environment probe fails', async () => {
    mockBackend({ env: () => reply({ error: 'probe crashed' }, false) })
    globalThis.fetch = vi.fn((url) => {
      const path = String(url)
      if (path.includes('/api/setup/state')) return reply(readyState())
      if (path.includes('/api/setup/env')) return reply({ error: 'probe crashed' }, false)
      return reply({ ok: true })
    })
    renderWizard()
    await waitFor(() => expect(screen.getByText('probe crashed')).toBeTruthy())
    expect(button(copy.runtime.finish).disabled).toBe(false)
  })
})
