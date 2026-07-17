import React from 'react'
import { afterEach, describe, expect, test, vi } from 'vitest'
import { cleanup, fireEvent, render, screen, waitFor } from '@testing-library/react'
import { ChannelServiceTable } from './components/common.jsx'
import App, { ChannelsPage } from './App.jsx'
import { ChatMessage } from './ChatApp.jsx'
import { Models } from './pages/ModelsPage.jsx'

globalThis.React = React

const t = {
  refresh: 'Refresh',
  save: 'Save',
  busy: 'Busy',
  empty: 'Empty',
  start: 'Start',
  stop: 'Stop',
  logs: 'Logs',
  running: 'Running',
  stopped: 'Stopped',
  autostart: 'Autostart',
  desc: { channels: 'Channel services' },
  lists: { frontendServices: 'Frontend services' },
  hints: { savedSecret: 'saved secret' },
  hide: 'Hide',
  show: 'Show',
}


const jsonResponse = (body) => ({
  ok: true,
  status: 200,
  statusText: 'OK',
  text: async () => JSON.stringify(body),
  json: async () => body,
})

const installBrowserPolyfills = () => {
  Object.defineProperty(window, 'matchMedia', {
    writable: true,
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

const setupFetch = vi.fn(async (url) => {
  const path = String(url)
  if (path.includes('/api/config')) return jsonResponse({ ga_root: '' })
  if (path.includes('/api/ga/health')) return jsonResponse({ ok: false, error: 'GA root not configured' })
  if (path.includes('/api/autostart/status')) return jsonResponse({ supported: false, enabled: false })
  if (path.includes('/api/version/info')) return jsonResponse({ version: 'test' })
  if (path.includes('/api/version/status')) return jsonResponse({})
  if (path.includes('/api/observability/status')) return jsonResponse({ ok: false })
  if (path.includes('/api/setup/state')) return jsonResponse({ status: 'needs_setup', env: {}, ga_root: '' })
  throw new Error(`unexpected url ${url}`)
})

const reflectService = {
  name: 'agentmain --reflect',
  kind: 'reflect',
  running: false,
  autostart: false,
  command: ['agentmain', '--reflect'],
}

afterEach(() => {
  cleanup()
  vi.restoreAllMocks()
})

describe('channel frontend gates', () => {
  test('ChannelServiceTable routes reflect service start through onReflectStart', () => {
    const onStart = vi.fn()
    const onReflectStart = vi.fn()

    render(
      <ChannelServiceTable
        services={[reflectService]}
        t={t}
        onStart={onStart}
        onStop={vi.fn()}
        onLogs={vi.fn()}
        onAutostart={vi.fn()}
        onReflectStart={onReflectStart}
      />,
    )

    fireEvent.click(screen.getByRole('button', { name: /Start/i }))
    expect(onReflectStart).toHaveBeenCalledWith(reflectService.name)
    expect(onStart).not.toHaveBeenCalled()
  })

  test('ChannelsPage renders service actions without missing callback references', async () => {
    globalThis.fetch = vi.fn(async (url) => {
      if (String(url).includes('/api/channels')) {
        const body = JSON.stringify({ path: 'mykey.py', profiles: [] })
        return {
          ok: true,
          status: 200,
          statusText: 'OK',
          text: async () => body,
        }
      }
      throw new Error(`unexpected url ${url}`)
    })

    const onReflectStart = vi.fn()
    render(
      <ChannelsPage
        frontendSvcs={[reflectService]}
        t={t}
        onStart={vi.fn()}
        onStop={vi.fn()}
        onLogs={vi.fn()}
        onAutostart={vi.fn()}
        onReflectStart={onReflectStart}
      />,
    )

    await waitFor(() => expect(screen.getByText(/mykey\.py/i)).toBeTruthy())
    fireEvent.click(screen.getByRole('button', { name: /Start/i }))
    expect(onReflectStart).toHaveBeenCalledWith(reflectService.name)
  })
})

describe('Models provider editor', () => {
  test('keeps focus in the provider name while its controlled value changes', () => {
    installBrowserPolyfills()

    function ModelsHarness() {
      const [profiles, setProfiles] = React.useState([{
        var_name: 'native_oai_config_demo',
        type: 'native_oai',
        apibase: 'https://api.example.com/v1',
        apikey: 'masked',
        model: 'demo-model',
        models: ['demo-model'],
      }])
      const patchProfile = (idx, patch) => {
        setProfiles(current => current.map((profile, index) => (
          index === idx ? { ...profile, ...patch } : profile
        )))
      }

      return (
        <Models
          t={{}}
          profiles={profiles}
          persistedProfiles={profiles}
          setProfiles={setProfiles}
          patchProfile={patchProfile}
          importModels={vi.fn()}
          previewModels={vi.fn()}
          riskCatalog={[]}
          getProfileKey={(idx, profile) => `${profile.var_name}:${profile.type}:${profile.apibase}:${idx}`}
        />
      )
    }

    const { container } = render(<ModelsHarness />)
    const nameInput = container.querySelector('.model-field--provider input')
    nameInput.focus()
    expect(document.activeElement).toBe(nameInput)

    fireEvent.change(nameInput, { target: { value: 'renamed' } })

    const updatedNameInput = container.querySelector('.model-field--provider input')
    expect(updatedNameInput.value).toBe('renamed')
    expect(document.activeElement).toBe(updatedNameInput)
  })
})


describe('chat response identity and time', () => {
  test('renders the concrete model ID and message time above its assistant response', () => {
    const createdAt = '2026-07-17T08:09:10.000Z'
    const { container } = render(
      <ChatMessage
        message={{ id: 'a1', role: 'assistant', content: 'Finished', model_id: '  vendor/model-v1  ', created_at: createdAt }}
        pending={false}
        onAskReply={vi.fn()}
      />,
    )

    const body = container.querySelector('.oa-msg-body')
    const meta = container.querySelector('.oa-meta')
    const badge = container.querySelector('.oa-model-id')
    const separator = container.querySelector('.oa-meta-separator')
    const time = container.querySelector('.oa-message-time')
    expect(body?.firstElementChild).toBe(meta)
    expect(badge?.textContent).toBe('vendor/model-v1')
    expect(badge?.getAttribute('title')).toBe('Model ID: vendor/model-v1')
    expect(separator?.textContent).toBe('·')
    expect(time?.textContent).toBe(new Date(createdAt).toLocaleString())
    expect(time?.getAttribute('datetime')).toBe(createdAt)
  })
})


describe('first-run setup shell', () => {
  test('App renders SetupWizard when GA root is not configured', async () => {
    installBrowserPolyfills()
    globalThis.fetch = setupFetch
    render(<App />)
    await waitFor(() => expect(screen.getByText(/首次启动配置|First/i)).toBeTruthy())
    expect(screen.getByText(/GA Admin Bootstrap/i)).toBeTruthy()
  })
})
