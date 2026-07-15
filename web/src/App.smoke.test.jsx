import React from 'react'
import { afterEach, describe, expect, test, vi } from 'vitest'
import { cleanup, fireEvent, render, screen, waitFor } from '@testing-library/react'
import { ChannelServiceTable, ServiceRow } from './components/common.jsx'
import App, { ChannelsPage } from './App.jsx'
import { ChatMessage, ProviderModelCascade } from './ChatApp.jsx'
import { Models } from './pages/ModelsPage.jsx'

globalThis.React = React
globalThis.ResizeObserver = class ResizeObserver {
  observe() {}
  unobserve() {}
  disconnect() {}
}

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
    globalThis.React = React

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


describe('chat response model identity', () => {
  test('renders the concrete model ID on its assistant response', () => {
    const { container } = render(
      <ChatMessage
        message={{ id: 'a1', role: 'assistant', content: 'Finished', model_id: '  vendor/model-v1  ', llm_no: 7, created_at: 0 }}
        pending={false}
        onAskReply={vi.fn()}
      />,
    )

    const badge = container.querySelector('.oa-model-id')
    expect(badge?.textContent).toBe('#7 · vendor/model-v1')
    expect(badge?.getAttribute('title')).toBe('Model: #7 · vendor/model-v1')
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

const modelProfile = {
  var_name: 'native_oai_config1',
  type: 'native_oai',
  name: '主模型',
  apibase: 'https://api.example/v1',
  model: 'gpt-test',
  models: ['gpt-test'],
  apikey: '******',
  stream: true,
  max_retries: 3,
  read_timeout: 300,
}

const modelProps = overrides => ({
  t,
  profiles: [modelProfile],
  setProfiles: vi.fn(),
  patchProfile: vi.fn(),
  addModelProfiles: vi.fn(async () => true),
  deleteModelProfile: vi.fn(async () => true),
  importModels: vi.fn(),
  previewModels: vi.fn(),
  saveModelProfile: vi.fn(async () => true),
  discoverModels: vi.fn(async () => ({ models: [] })),
  probeModels: vi.fn(async () => ({ results: [], checked_at: '2026-07-15T06:35:00Z' })),
  getProfileKey: () => 'profile-1',
  onRevealKey: vi.fn(),
  onClearRevealedKey: vi.fn(),
  ...overrides,
})

describe('model profile names', () => {
  test('should display and auto-save a Chinese name when the name is edited', async () => {
    const props = modelProps()
    const view = render(<Models {...props} />)

    const nameInput = screen.getByLabelText('模型名称')
    expect(nameInput.value).toBe('主模型')
    fireEvent.change(nameInput, { target: { value: '主模型-修改' } })
    view.rerender(<Models {...props} profiles={[{ ...modelProfile, name: '主模型-修改' }]} />)
    fireEvent.blur(screen.getByLabelText('模型名称'))

    await waitFor(() => expect(props.saveModelProfile).toHaveBeenCalledWith(
      0,
      'profile-1',
      expect.objectContaining({ name: '主模型-修改' }),
    ))
  })

  test('should remove the profile when the delete action is confirmed by the page flow', async () => {
    const props = modelProps()
    vi.spyOn(window, 'confirm').mockReturnValue(true)
    render(<Models {...props} />)

    fireEvent.click(screen.getByTitle('删除此服务商'))
    await waitFor(() => expect(props.deleteModelProfile).toHaveBeenCalledWith([]))
  })

  test('should not add a named profile when BaseURL is missing', () => {
    const props = modelProps({ profiles: [] })
    render(<Models {...props} />)

    fireEvent.click(screen.getAllByRole('button', { name: '新增服务商' })[0])
    fireEvent.change(screen.getByLabelText('新增模型名称'), { target: { value: '新增中文模型' } })
    fireEvent.click(screen.getByRole('button', { name: '添加并保存' }))

    expect(props.addModelProfiles).not.toHaveBeenCalled()
  }, 10000)

  test('should add and auto-save a profile with a Chinese name when required fields are present', async () => {
    const props = modelProps({ profiles: [] })
    render(<Models {...props} />)

    fireEvent.click(screen.getAllByRole('button', { name: '新增服务商' })[0])
    fireEvent.change(screen.getByLabelText('新增模型名称'), { target: { value: '新增中文模型' } })
    fireEvent.change(screen.getByLabelText('BaseURL'), { target: { value: 'https://api.example/v1' } })
    fireEvent.click(screen.getByRole('button', { name: '添加并保存' }))

    await waitFor(() => expect(props.addModelProfiles).toHaveBeenCalledWith([
      expect.objectContaining({ name: '新增中文模型', apibase: 'https://api.example/v1' }),
    ]))
  }, 10000)
})

describe('provider model availability management', () => {
  test('auto-disables missing models and saves the provider after a successful check', async () => {
    const profile = {
      ...modelProfile,
      model: 'gpt-test',
      models: ['gpt-test', 'retired-model'],
      model_configs: [{ model: 'gpt-test' }, { model: 'retired-model', read_timeout: 600 }],
    }
    const props = modelProps({
      profiles: [profile],
      probeModels: vi.fn(async () => ({
        checked_at: '2026-07-15T06:35:00Z',
        results: [
          { id: 'gpt-test', available: true, detail: '真实对话验证通过', latency_ms: 20 },
          { id: 'retired-model', available: false, detail: 'HTTP 404', latency_ms: 12 },
        ],
      })),
    })
    render(<Models {...props} />)

    fireEvent.click(screen.getByRole('button', { name: '对话检测并同步' }))
    await waitFor(() => expect(props.saveModelProfile).toHaveBeenCalledWith(
      0,
      'profile-1',
      expect.objectContaining({
        model_configs: expect.arrayContaining([
          expect.objectContaining({ model: 'retired-model', enabled: false, auto_disabled: true, read_timeout: 600 }),
        ]),
      }),
    ))
    expect(await screen.findByText('真实对话检测完成：1 个可用，1 个不可用')).toBeTruthy()
  }, 10000)

  test('does not save when the provider returns an empty model list', async () => {
    const props = modelProps({ probeModels: vi.fn(async () => ({ results: [] })) })
    render(<Models {...props} />)

    fireEvent.click(screen.getByRole('button', { name: '对话检测并同步' }))
    expect(await screen.findByText('检测失败，未修改模型状态')).toBeTruthy()
    expect(props.saveModelProfile).not.toHaveBeenCalled()
  }, 10000)
})

describe('reflect service model selector', () => {
  test('shows the complete long model label on the first open and preserves selection', async () => {
    installBrowserPolyfills()
    const onModel = vi.fn()
    const longLabel = 'NativeOAISession/code-specialized-model-with-a-complete-visible-name'

    render(
      <ServiceRow
        svc={reflectService}
        t={t}
        llms={[{ index: 7, label: longLabel }]}
        onStart={vi.fn()}
        onStop={vi.fn()}
        onLogs={vi.fn()}
        onAutostart={vi.fn()}
        onModel={onModel}
      />,
    )

    fireEvent.mouseDown(screen.getByRole('combobox', { name: '模型' }))
    const option = await screen.findByRole('option', { name: longLabel })
    expect(document.querySelector('.service-model-popup')).toBeTruthy()
    fireEvent.click(option)
    expect(onModel).toHaveBeenCalledWith(reflectService.name, 7)
  })
})

describe('mobile chat model selector', () => {
  test('keeps the portal open through the viewport scroll caused by an iOS tap', async () => {
    installBrowserPolyfills()
    const onChange = vi.fn()
    const groups = [{
      value: 'provider-a',
      label: 'Provider A',
      models: [
        { value: 3, label: 'model-three' },
        { value: 4, label: 'model-four' },
      ],
    }]

    render(
      <div style={{ overflow: 'hidden' }}>
        <ProviderModelCascade
          groups={groups}
          selectedProvider="provider-a"
          value={3}
          onChange={onChange}
          mobile
        />
      </div>,
    )

    fireEvent.click(screen.getByRole('button', { name: '选择模型，当前 model-three' }))
    const dialog = await screen.findByRole('dialog', { name: '服务商和模型' })
    expect(dialog.closest('.oa-mobile-picker-backdrop')?.parentElement).toBe(document.body)

    fireEvent.scroll(window)
    expect(screen.getByRole('dialog', { name: '服务商和模型' })).toBeTruthy()

    fireEvent.click(screen.getByRole('button', { name: 'model-four' }))
    expect(onChange).toHaveBeenCalledWith(4)
    expect(screen.queryByRole('dialog', { name: '服务商和模型' })).toBeNull()
  })
})
