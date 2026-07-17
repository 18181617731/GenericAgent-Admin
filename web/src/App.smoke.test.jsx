import React from 'react'
import { afterEach, describe, expect, test, vi } from 'vitest'
import { cleanup, fireEvent, render, screen, waitFor } from '@testing-library/react'
import { ChannelServiceTable, ServiceRow } from './components/common.jsx'
import App, { ChannelsPage } from './App.jsx'
import { ChatMessage, ProviderModelCascade } from './ChatApp.jsx'
import { GoalsPage } from './pages/GoalsPage.jsx'
import { Models } from './pages/ModelsPage.jsx'
import { FilesPage } from './pages/FilesPage.jsx'
import { SettingsPage } from './pages/SettingsPage.jsx'
import { GlobalFeedback, MessageBanner } from './components/feedback.jsx'

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
  test('renders the provider display name and concrete model ID on its assistant response', () => {
    const { container } = render(
      <ChatMessage
        message={{ id: 'a1', role: 'assistant', content: 'Finished', model_id: '  model-v1  ', llm_no: 7, created_at: 0 }}
        models={[{ index: 7, provider: '服务商 A', model: 'model-v1' }]}
        pending={false}
        onAskReply={vi.fn()}
      />,
    )

    const badge = container.querySelector('.oa-model-id')
    expect(badge?.textContent).toBe('服务商 A · model-v1')
    expect(badge?.getAttribute('title')).toBe('服务商：服务商 A；模型：model-v1；内部编号：#7')
  })

  test('falls back to the recorded model ID when the historical provider no longer exists', () => {
    const { container } = render(
      <ChatMessage
        message={{ id: 'a2', role: 'assistant', content: 'Finished', model_id: 'retired-model', llm_no: 18, created_at: 0 }}
        models={[]}
        pending={false}
        onAskReply={vi.fn()}
      />,
    )

    const badge = container.querySelector('.oa-model-id')
    expect(badge?.textContent).toBe('retired-model')
    expect(badge?.textContent).not.toContain('#18')
    expect(badge?.getAttribute('title')).toBe('模型：retired-model；内部编号：#18')
  })

  test('matches the recorded model ID instead of a stale internal index after reordering', () => {
    const { container } = render(
      <ChatMessage
        message={{ id: 'a3', role: 'assistant', content: 'Finished', model_id: 'gpt-5.6-sol', llm_no: 18, created_at: 0 }}
        models={[
          { index: 0, provider: '自费帅API gpt', model: 'gpt-5.6-sol' },
          { index: 18, provider: '其他服务商', model: 'gpt-5.6-terra' },
        ]}
        pending={false}
        onAskReply={vi.fn()}
      />,
    )

    const badge = container.querySelector('.oa-model-id')
    expect(badge?.textContent).toBe('自费帅API gpt · gpt-5.6-sol')
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
  onSaveModelProfiles: vi.fn(async () => true),
  discoverModels: vi.fn(async () => ({ models: [] })),
  probeModels: vi.fn(async () => ({ results: [], checked_at: '2026-07-15T06:35:00Z' })),
  modelProbeProviders: [],
  onSaveModelProbeProviders: vi.fn(async keys => keys),
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

  test('model order uses the configured provider display name', () => {
    render(<Models {...modelProps({ persistedProfiles: [modelProfile], onSaveModelOrder: vi.fn(async () => true) })} />)

    fireEvent.click(screen.getByRole('button', { name: '模型顺序' }))

    expect(screen.getByText('服务商名称：主模型')).toBeTruthy()
    expect(screen.queryByText('服务商名称：1')).toBeNull()
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

    fireEvent.click(screen.getByRole('button', { name: '检测当前服务商' }))
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

    fireEvent.click(screen.getByRole('button', { name: '检测当前服务商' }))
    expect(await screen.findByText('检测失败，未修改模型状态')).toBeTruthy()
    expect(props.saveModelProfile).not.toHaveBeenCalled()
  }, 10000)
})

describe('provider model batch availability management', () => {
  test('checks every provider by default and saves all reconciled states once', async () => {
    const probeModels = vi.fn(async request => ({
      checked_at: '2026-07-16T15:00:00+08:00',
      results: [{ id: request.models[0], available: true, detail: '真实对话验证通过', latency_ms: 10 }],
    }))
    const props = modelProps({ profiles: [modelProfile], probeModels })
    render(<Models {...props} />)

    fireEvent.click(screen.getByRole('button', { name: '对话检测并同步' }))

    await waitFor(() => expect(probeModels).toHaveBeenCalledTimes(1))
    expect(probeModels).toHaveBeenCalledWith(expect.objectContaining({ varName: modelProfile.var_name }))
    await waitFor(() => expect(props.onSaveModelProfiles).toHaveBeenCalledTimes(1))
    expect(props.onSaveModelProfiles).toHaveBeenCalledWith([
      expect.objectContaining({ var_name: modelProfile.var_name }),
    ])
    expect(await screen.findByText('批量检测完成：1 个服务商成功，0 个失败')).toBeTruthy()
  }, 10000)

  test('saves a configured provider scope for later batches', async () => {
    const props = modelProps({ profiles: [modelProfile] })
    render(<Models {...props} />)

    fireEvent.click(screen.getByRole('button', { name: '检测范围：全部 1' }))
    fireEvent.click(screen.getByRole('radio', { name: '指定服务商' }))
    fireEvent.click(screen.getByRole('button', { name: '保存范围' }))

    await waitFor(() => expect(props.onSaveModelProbeProviders).toHaveBeenCalledWith([modelProfile.var_name]))
  }, 30000)
})

describe('reflect service model selector', () => {
  test('shows the complete long model label on the first open and preserves selection', async () => {
    installBrowserPolyfills()
    const onModel = vi.fn()
    const longLabel = 'code-specialized-model-with-a-complete-visible-name'

    render(
      <ServiceRow
        svc={reflectService}
        t={t}
        llms={[{ index: 7, provider: 'Provider A', model: longLabel }]}
        onStart={vi.fn()}
        onStop={vi.fn()}
        onLogs={vi.fn()}
        onAutostart={vi.fn()}
        onModel={onModel}
      />,
    )

    fireEvent.click(screen.getByRole('button', { name: /选择模型/ }))
    expect(await screen.findByRole('dialog', { name: '服务商和模型' })).toBeTruthy()
    fireEvent.click(screen.getByRole('option', { name: 'Provider A' }))
    fireEvent.click(screen.getByRole('option', { name: longLabel }))
    expect(onModel).toHaveBeenCalledWith(reflectService.name, 7)
  }, 10000)
})

describe('goal mode model selector', () => {
  test('selects an optional LLM through the shared provider and model cascade', async () => {
    installBrowserPolyfills()
    const setLLMNo = vi.fn()
    const goalT = {
      nav: { goals: 'Goal 模式' },
      fields: {
        goalRuns: 'Goal 运行', startGoalMode: '启动 Goal Mode', outputTail: '输出尾部',
        objective: '目标', goalPlaceholder: '描述目标', budgetMinutes: '预算分钟', maxTurns: '最大轮次',
        llmNo: 'LLM #（可选）', goalHive: 'Hive 模式', outputDefault: '默认',
      },
      desc: { goals: 'Goal 模式说明' },
      hints: { goalHiveHelp: 'Hive 模式说明' },
      running: '运行中', ready: '就绪', empty: '空', start: '启动', refresh: '刷新', error: '错误',
    }

    render(<GoalsPage
      t={goalT}
      goals={[]}
      objective="目标"
      setObjective={vi.fn()}
      budget="480"
      setBudget={vi.fn()}
      maxTurns="200"
      setMaxTurns={vi.fn()}
      llmNo=""
      setLLMNo={setLLMNo}
      llms={[{ index: 7, provider: 'Provider A', model: 'goal-model' }]}
      hive={false}
      setHive={vi.fn()}
      outputBytes="0"
      setOutputBytes={vi.fn()}
      autoRefresh={false}
      setAutoRefresh={vi.fn()}
      selected=""
      output=""
      outputMeta={null}
      busy={false}
      onStart={vi.fn()}
      onStop={vi.fn()}
      onDelete={vi.fn()}
      onRefresh={vi.fn()}
      onOutput={vi.fn()}
      onClearOutput={vi.fn()}
      setMsg={vi.fn()}
    />)

    fireEvent.click(screen.getByRole('tab', { name: '启动 Goal Mode' }))
    fireEvent.click(screen.getByRole('button', { name: /选择模型/ }))
    fireEvent.click(screen.getByRole('option', { name: 'Provider A' }))
    fireEvent.click(screen.getByRole('option', { name: 'goal-model' }))
    expect(setLLMNo).toHaveBeenCalledWith('7')
  }, 10000)
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

    fireEvent.click(screen.getByRole('button', { name: '选择模型，当前 Provider A · model-three' }))
    const dialog = await screen.findByRole('dialog', { name: '服务商和模型' })
    expect(dialog.closest('.oa-mobile-picker-backdrop')?.parentElement).toBe(document.body)

    fireEvent.scroll(window)
    expect(screen.getByRole('dialog', { name: '服务商和模型' })).toBeTruthy()

    fireEvent.click(screen.getByRole('option', { name: 'model-four' }))
    expect(onChange).toHaveBeenCalledWith(4)
    expect(screen.queryByRole('dialog', { name: '服务商和模型' })).toBeNull()
  })

  test('locks background scrolling and restores focus when Escape closes the mobile picker', async () => {
    installBrowserPolyfills()
    const groups = [{ value: 'provider-a', label: 'Provider A', models: [{ value: 3, label: 'model-three' }] }]
    render(<ProviderModelCascade groups={groups} selectedProvider="provider-a" value={3} onChange={vi.fn()} mobile />)

    const trigger = screen.getByRole('button', { name: /选择模型/ })
    trigger.focus()
    fireEvent.click(trigger)
    await screen.findByRole('dialog', { name: '服务商和模型' })
    expect(document.body.style.overflow).toBe('hidden')

    fireEvent.keyDown(document, { key: 'Escape' })
    await waitFor(() => expect(screen.queryByRole('dialog', { name: '服务商和模型' })).toBeNull())
    expect(document.body.style.overflow).toBe('')
    await waitFor(() => expect(document.activeElement).toBe(trigger))
  })
})

describe('keyboard-friendly model selector', () => {
  test('filters models and supports arrow navigation across provider and model columns', async () => {
    installBrowserPolyfills()
    const onChange = vi.fn()
    const groups = [
      { value: 'openai', label: 'OpenAI', models: [{ value: 1, label: 'gpt-5' }] },
      { value: 'anthropic', label: 'Anthropic', models: [{ value: 2, label: 'claude-sonnet' }] },
    ]
    render(<ProviderModelCascade groups={groups} selectedProvider="openai" value={1} onChange={onChange} />)
    fireEvent.click(screen.getByRole('button', { name: /选择模型/ }))

    const search = await screen.findByRole('textbox', { name: '搜索服务商或模型' })
    expect(document.activeElement).toBe(search)
    fireEvent.change(search, { target: { value: 'sonnet' } })
    expect(screen.queryByRole('option', { name: 'gpt-5' })).toBeNull()
    expect(screen.getByRole('option', { name: 'claude-sonnet' })).toBeTruthy()

    fireEvent.change(search, { target: { value: '' } })
    await waitFor(() => expect(screen.getByRole('option', { name: 'OpenAI' })).toBeTruthy())
    fireEvent.keyDown(search, { key: 'ArrowDown' })
    expect(document.activeElement).toBe(screen.getByRole('option', { name: 'OpenAI' }))
    fireEvent.keyDown(document.activeElement, { key: 'ArrowDown' })
    expect(document.activeElement).toBe(screen.getByRole('option', { name: 'Anthropic' }))
    fireEvent.keyDown(document.activeElement, { key: 'ArrowRight' })
    await waitFor(() => expect(document.activeElement).toBe(screen.getByRole('option', { name: 'claude-sonnet' })))
    fireEvent.click(document.activeElement)
    expect(onChange).toHaveBeenCalledWith(2)
  })
})

describe('shared action feedback', () => {
  test('announces inferred errors and supports copying and dismissing the message', async () => {
    const onDismiss = vi.fn()
    const writeText = vi.fn().mockResolvedValue(undefined)
    Object.defineProperty(navigator, 'clipboard', { configurable: true, value: { writeText } })
    render(<MessageBanner message="升级失败：网络不可用" onDismiss={onDismiss} copyable />)
    expect(screen.getByRole('alert').textContent).toContain('升级失败')
    fireEvent.click(screen.getByRole('button', { name: '复制详情' }))
    await waitFor(() => expect(writeText).toHaveBeenCalledWith('升级失败：网络不可用'))
    expect(screen.getByRole('button', { name: '已复制详情' })).toBeTruthy()
    fireEvent.click(screen.getByRole('button', { name: '关闭消息' }))
    expect(onDismiss).toHaveBeenCalledOnce()
    delete navigator.clipboard
  })
})

describe('mobile file workflow', () => {
  const fileProps = () => ({
    t: {
      read: '读取', search: '搜索', tail: '尾读', download: '下载', delete: '删除', save: '保存', empty: '空内容',
      lists: { fileList: '文件列表', filePreview: '文件预览', searchResults: '搜索结果' },
      hints: { filePath: '相对路径', searchText: '搜索文本', tailLines: '尾部行数' },
    },
    browsePath: 'memory', setBrowsePath: vi.fn(), filePath: '', setFilePath: vi.fn(),
    fileList: [{ kind: 'dir', path: 'memory/logs' }, { kind: 'file', path: 'memory/notes.md' }],
    fileContent: '', loadedFileContent: '', loadedFilePath: '', setFileContent: vi.fn(),
    fileSearch: '', setFileSearch: vi.fn(), searchHits: [], tailLines: 200, setTailLines: vi.fn(),
    loadFiles: vi.fn(), readFile: vi.fn(), tailFile: vi.fn(), saveFile: vi.fn(), deleteFile: vi.fn(),
    downloadFile: vi.fn(), runSearch: vi.fn(), clearSearch: vi.fn(), busy: false,
  })

  test('should keep directory browsing separate from selected file actions', () => {
    const props = fileProps()
    render(<FilesPage {...props}/>)

    fireEvent.click(screen.getByRole('button', { name: /logs/i }))
    expect(props.loadFiles).toHaveBeenCalledWith('memory/logs')
    expect(props.setFilePath).not.toHaveBeenCalled()

    fireEvent.click(screen.getByRole('button', { name: /notes\.md/i }))
    expect(props.readFile).toHaveBeenCalledWith('memory/notes.md')
  })

  test('should switch to preview when a file finishes loading', () => {
    const props = fileProps()
    const { rerender } = render(<FilesPage {...props}/>)
    expect(screen.getByRole('tab', { name: '文件' }).getAttribute('aria-selected')).toBe('true')

    rerender(<FilesPage {...props} filePath="memory/notes.md" loadedFilePath="memory/notes.md" fileContent="hello" loadedFileContent="hello"/>)
    expect(screen.getByRole('tab', { name: '预览' }).getAttribute('aria-selected')).toBe('true')
    expect(screen.getByRole('textbox', { name: '文件内容编辑器' }).value).toBe('hello')
  })

  test('protects dirty content before opening another file and exposes search result counts', () => {
    const props = fileProps()
    props.fileContent = 'changed'
    props.loadedFileContent = 'original'
    props.loadedFilePath = 'memory/current.md'
    props.fileSearch = 'note'
    props.searchHits = [{ path: 'memory/notes.md', line: 2, preview: 'note' }]
    const confirmSpy = vi.spyOn(window, 'confirm').mockReturnValue(false)
    render(<FilesPage {...props}/>)

    expect(screen.getByText('1', { selector: '.files-search-results-head span' })).toBeTruthy()
    fireEvent.click(screen.getAllByRole('button', { name: /notes\.md/i })[0])
    expect(confirmSpy).toHaveBeenCalled()
    expect(props.readFile).not.toHaveBeenCalled()

    fireEvent.click(screen.getByRole('button', { name: '清空文件搜索' }))
    expect(props.clearSearch).toHaveBeenCalledOnce()
  })
})

describe('configuration editing experience', () => {
  const settingsT = {
    busy: '执行中', root: 'GenericAgent 根目录',
    nav: { settings: '配置' },
    fields: { pythonPath: 'Python 解释器', pythonAuto: '自动选择', chatDataDir: '聊天目录', chatDataAuto: '自动目录' },
  }

  test('should expose one save action and a reset action when settings are dirty', () => {
    const onSave = vi.fn()
    const onReset = vi.fn()
    render(<SettingsPage t={settingsT} root="D:/GA" setRoot={vi.fn()} config={{ proxy_mode: 'off', slash_commands: [] }} setConfig={vi.fn()} dirty busy={false} onSave={onSave} onReset={onReset}/>)

    expect(screen.getByText('有未保存更改')).toBeTruthy()
    fireEvent.click(screen.getByRole('button', { name: '放弃更改' }))
    fireEvent.click(screen.getByRole('button', { name: '保存全部配置' }))
    expect(onReset).toHaveBeenCalledTimes(1)
    expect(onSave).toHaveBeenCalledTimes(1)
    expect(screen.getAllByRole('button', { name: /保存/ })).toHaveLength(1)
  })

  test('should keep save and reset disabled when settings match persisted configuration', () => {
    render(<SettingsPage t={settingsT} root="D:/GA" setRoot={vi.fn()} config={{ proxy_mode: 'off', slash_commands: [] }} setConfig={vi.fn()} dirty={false} busy={false} onSave={vi.fn()} onReset={vi.fn()}/>)
    expect(screen.getByRole('button', { name: '放弃更改' }).disabled).toBe(true)
    expect(screen.getByRole('button', { name: '保存全部配置' }).disabled).toBe(true)
  })
})

describe('global feedback experience', () => {
  test('should keep errors assertive and dismissible', () => {
    const onDismiss = vi.fn()
    render(<GlobalFeedback message="保存失败：permission denied" onDismiss={onDismiss} successTimeout={0}/>)
    expect(screen.getByRole('alert')).toBeTruthy()
    fireEvent.click(screen.getByRole('button', { name: '关闭提示' }))
    expect(onDismiss).toHaveBeenCalledTimes(1)
  })

  test('should expose successful feedback as a polite status', () => {
    render(<GlobalFeedback message="配置已保存" onDismiss={vi.fn()} successTimeout={0}/>)
    expect(screen.getByRole('status')).toBeTruthy()
  })
})
