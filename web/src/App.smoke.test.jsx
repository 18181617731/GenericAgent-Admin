import React from 'react'
import { afterEach, describe, expect, test, vi } from 'vitest'
import { cleanup, fireEvent, render, screen, waitFor } from '@testing-library/react'
import { ChannelServiceTable } from './components/common.jsx'
import App, { ChannelsPage } from './App.jsx'
import { ChatMessage, PlanTodoCard } from './ChatApp.jsx'
import { Models } from './pages/ModelsPage.jsx'
import { FilesPage } from './pages/FilesPage.jsx'

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

  test('keeps unrelated service controls available while one action is pending', () => {
    const services = [
      { ...reflectService, name: 'frontend/alpha', kind: 'frontend', running: false },
      { ...reflectService, name: 'frontend/beta', kind: 'frontend', running: false },
    ]
    render(
      <ChannelServiceTable
        services={services}
        t={{ ...t, ready: 'Ready', error: 'Error', retry: 'Retry' }}
        onStart={vi.fn()}
        onStop={vi.fn()}
        onLogs={vi.fn()}
        onAutostart={vi.fn()}
        actionState={{ name: 'frontend/alpha', action: 'start', status: 'pending', message: 'Start: Busy' }}
      />,
    )

    const startButtons = screen.getAllByRole('button', { name: /Start/i })
    const stopButtons = screen.getAllByRole('button', { name: /Stop/i })
    expect(startButtons[0].disabled).toBe(true)
    expect(stopButtons[0].disabled).toBe(true)
    expect(startButtons[1].disabled).toBe(false)
    expect(screen.getAllByRole('button', { name: /Logs/i }).every(button => !button.disabled)).toBe(true)
    expect(screen.getByRole('status').textContent).toMatch(/Start: Busy/i)
  })

  test('shows a contextual service failure and retries the same action', () => {
    const onStart = vi.fn()
    render(
      <ChannelServiceTable
        services={[{ ...reflectService, name: 'frontend/alpha', kind: 'frontend', running: false }]}
        t={{ ...t, ready: 'Ready', error: 'Error', retry: 'Retry' }}
        onStart={onStart}
        onStop={vi.fn()}
        onLogs={vi.fn()}
        onAutostart={vi.fn()}
        actionState={{ name: 'frontend/alpha', action: 'start', status: 'error', message: 'Start: Error · port in use' }}
      />,
    )

    expect(screen.getByRole('alert').textContent).toMatch(/port in use/i)
    fireEvent.click(screen.getByRole('button', { name: /Retry/i }))
    expect(onStart).toHaveBeenCalledWith('frontend/alpha')
  })
})

const validModelProfile = {
  var_name: 'native_oai_config_demo',
  type: 'native_oai',
  apibase: 'https://api.example.com/v1',
  apikey: 'masked',
  model: 'demo-model',
  models: ['demo-model'],
}

function ModelsHarness({
  initialProfile = validModelProfile,
  discoverModels = vi.fn(async () => ({ models: [] })),
  saveModelProfile,
  modelSaveStatus = {},
}) {
  const [profiles, setProfiles] = React.useState([{ ...initialProfile }])
  const patchProfile = (idx, patch) => {
    setProfiles(current => current.map((profile, index) => (
      index === idx ? { ...profile, ...patch } : profile
    )))
  }

  return (
    <Models
      t={{}}
      profiles={profiles}
      persistedProfiles={[{ ...initialProfile }]}
      setProfiles={setProfiles}
      patchProfile={patchProfile}
      importModels={vi.fn()}
      previewModels={vi.fn()}
      discoverModels={discoverModels}
      saveModelProfile={saveModelProfile}
      modelSaveStatus={modelSaveStatus}
      riskCatalog={[]}
      getProfileKey={() => 'profile-key'}
    />
  )
}

describe('Models provider editor', () => {
  test('keeps focus in the provider name while its controlled value changes', () => {
    installBrowserPolyfills()

    const { container } = render(<ModelsHarness />)
    const nameInput = container.querySelector('.model-field--provider input')
    nameInput.focus()
    expect(document.activeElement).toBe(nameInput)

    fireEvent.change(nameInput, { target: { value: 'renamed' } })

    const updatedNameInput = container.querySelector('.model-field--provider input')
    expect(updatedNameInput.value).toBe('renamed')
    expect(document.activeElement).toBe(updatedNameInput)
  })

  test('shows discovery pending then empty state with a recovery action', async () => {
    installBrowserPolyfills()
    let resolveDiscovery
    const discoverModels = vi.fn(() => new Promise(resolve => { resolveDiscovery = resolve }))
    render(<ModelsHarness discoverModels={discoverModels} />)

    fireEvent.click(screen.getByRole('button', { name: '获取模型' }))
    expect(await screen.findByText('正在获取模型')).toBeTruthy()

    resolveDiscovery({ models: [] })
    expect(await screen.findByText(/没有发现新的模型/)).toBeTruthy()
    fireEvent.click(screen.getByRole('button', { name: '重新获取' }))
    expect(discoverModels).toHaveBeenCalledTimes(2)
  })

  test('shows discovery failure and retries in place', async () => {
    installBrowserPolyfills()
    const discoverModels = vi.fn()
      .mockRejectedValueOnce(new Error('upstream unavailable'))
      .mockResolvedValueOnce({ models: [] })
    render(<ModelsHarness discoverModels={discoverModels} />)

    fireEvent.click(screen.getByRole('button', { name: '获取模型' }))
    expect(await screen.findByText('无法获取候选模型')).toBeTruthy()
    expect(screen.getByText('upstream unavailable')).toBeTruthy()

    fireEvent.click(screen.getByRole('button', { name: /重\s*试/ }))
    await waitFor(() => expect(discoverModels).toHaveBeenCalledTimes(2))
    expect(await screen.findByText(/没有发现新的模型/)).toBeTruthy()
  })

  test('inserts a discovered candidate into the profile', async () => {
    installBrowserPolyfills()
    const discoverModels = vi.fn(async () => ({ models: ['new-model'] }))
    const { container } = render(<ModelsHarness discoverModels={discoverModels} />)

    fireEvent.click(screen.getByRole('button', { name: '获取模型' }))
    fireEvent.click(await screen.findByRole('button', { name: '添加模型 new-model' }))

    await waitFor(() => expect(container.textContent).toContain('new-model'))
  })

  test('shows invalid profile errors and API key warning separately', () => {
    installBrowserPolyfills()
    render(<ModelsHarness initialProfile={{ ...validModelProfile, var_name: '', apibase: '', apikey: '' }} />)

    expect(screen.getByText('此服务商暂时不能保存')).toBeTruthy()
    expect(screen.getByText('必须填写变量名')).toBeTruthy()
    expect(screen.getByText('必须填写 API Base')).toBeTruthy()
    expect(screen.getByText('保存前请留意')).toBeTruthy()
    expect(screen.getByText(/API Key 为空/)).toBeTruthy()
  })

  test('shows pending and successful per-profile save feedback', () => {
    installBrowserPolyfills()
    const { rerender } = render(<ModelsHarness modelSaveStatus={{ 'profile-key': { status: 'saving' } }} />)
    expect(screen.getByText('正在保存此服务商')).toBeTruthy()

    rerender(<ModelsHarness modelSaveStatus={{ 'profile-key': { status: 'saved', savedAt: 1 } }} />)
    expect(screen.getByText('已保存到 mykey.py')).toBeTruthy()
  })

  test('shows failed save detail and retries the same profile', () => {
    installBrowserPolyfills()
    const saveModelProfile = vi.fn(async () => true)
    render(
      <ModelsHarness
        saveModelProfile={saveModelProfile}
        modelSaveStatus={{ 'profile-key': { status: 'error', error: 'disk is read-only' } }}
      />,
    )

    expect(screen.getByText('此服务商保存失败')).toBeTruthy()
    expect(screen.getByText('disk is read-only')).toBeTruthy()
    fireEvent.click(screen.getByRole('button', { name: '重试保存' }))
    expect(saveModelProfile).toHaveBeenCalledWith(0, 'profile-key')
  })
})


describe('plan todo card disclosure', () => {
  test('starts expanded and toggles the plan body with matching chevrons', () => {
    const { container } = render(
      <PlanTodoCard plan={{
        active: true,
        done: 1,
        total: 2,
        items: [
          { status: 'done', content: 'Inspect the task' },
          { status: 'in_progress', content: 'Implement collapse' },
        ],
        step: 'Editing the plan card',
      }}/>,
    )

    const collapseButton = screen.getByRole('button', { name: '\u6536\u8d77\u6267\u884c\u8ba1\u5212' })
    const body = container.querySelector('.oa-plan-body')
    expect(collapseButton.getAttribute('aria-expanded')).toBe('true')
    expect(collapseButton.getAttribute('aria-controls')).toBe(body?.id)
    expect(body?.hidden).toBe(false)
    expect(collapseButton.querySelector('.lucide-chevron-down')).toBeTruthy()

    fireEvent.click(collapseButton)

    const expandButton = screen.getByRole('button', { name: '\u5c55\u5f00\u6267\u884c\u8ba1\u5212' })
    expect(expandButton.getAttribute('aria-expanded')).toBe('false')
    expect(body?.hidden).toBe(true)
    expect(expandButton.querySelector('.lucide-chevron-left')).toBeTruthy()

    fireEvent.click(expandButton)

    expect(screen.getByRole('button', { name: '\u6536\u8d77\u6267\u884c\u8ba1\u5212' }).getAttribute('aria-expanded')).toBe('true')
    expect(body?.hidden).toBe(false)
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

  test('continues live elapsed time from the persisted backend start after refresh', () => {
    const startedAt = Date.parse('2026-07-17T08:00:00.000Z')
    const refreshedAt = startedAt + 60_000
    const { container } = render(
      <ChatMessage
        message={{ id: 'pending', role: 'assistant', content: '', created_at: refreshedAt, run_started_at_ms: startedAt }}
        pending
        clockNow={startedAt + 90_000}
        onAskReply={vi.fn()}
      />,
    )

    expect(container.querySelector('.oa-usage-time')?.textContent).toContain('1m 30s')
  })

  test('uses the persisted terminal elapsed duration instead of continuing the live clock', () => {
    const { container } = render(
      <ChatMessage
        message={{ id: 'done', role: 'assistant', content: 'Finished', elapsed_ms: 4_200, run_started_at_ms: 1 }}
        pending={false}
        clockNow={100_000}
        onAskReply={vi.fn()}
      />,
    )

    expect(container.querySelector('.oa-usage-time')?.textContent).toContain('4s')
  })

  test('renders an explicit empty result for a worldline command', () => {
    render(
      <ChatMessage
        message={{ id: 'worldline-empty', role: 'assistant', commandResult: { command:'worldline', action:'list', tree:{ nodes:[] } } }}
        pending={false}
        onAskReply={vi.fn()}
      />,
    )

    expect(screen.getByText('0 个世界线节点')).toBeTruthy()
  })

  test('edits and resends a terminal user message, then closes the editor on success', async () => {
    const onEditResend = vi.fn().mockResolvedValue(undefined)
    const { container } = render(
      <ChatMessage
        message={{ id: 'user-edit-ok', role: 'user', content: 'original text' }}
        pending={false}
        onAskReply={vi.fn()}
        onEditResend={onEditResend}
      />,
    )

    fireEvent.click(screen.getByRole('button', { name: '\u7f16\u8f91\u5e76\u91cd\u65b0\u53d1\u9001' }))
    const editor = screen.getByRole('textbox', { name: '\u7f16\u8f91\u5df2\u53d1\u9001\u6d88\u606f' })
    fireEvent.change(editor, { target: { value: '  revised text  ' } })
    fireEvent.click(screen.getByRole('button', { name: '\u53d1\u9001' }))

    await waitFor(() => expect(onEditResend).toHaveBeenCalledWith('user-edit-ok', 'revised text'))
    await waitFor(() => expect(container.querySelector('.oa-message-editor')).toBeNull())
  })

  test('keeps the edited draft and exposes the error when resend fails', async () => {
    const onEditResend = vi.fn().mockRejectedValue(new Error('resend failed'))
    render(
      <ChatMessage
        message={{ id: 'user-edit-fail', role: 'user', content: 'original text' }}
        pending={false}
        onAskReply={vi.fn()}
        onEditResend={onEditResend}
      />,
    )

    fireEvent.click(screen.getByRole('button', { name: '\u7f16\u8f91\u5e76\u91cd\u65b0\u53d1\u9001' }))
    const editor = screen.getByRole('textbox', { name: '\u7f16\u8f91\u5df2\u53d1\u9001\u6d88\u606f' })
    fireEvent.change(editor, { target: { value: 'draft survives' } })
    fireEvent.click(screen.getByRole('button', { name: '\u53d1\u9001' }))

    const alert = await screen.findByRole('alert')
    expect(alert.textContent).toContain('resend failed')
    expect(screen.getByRole('textbox', { name: '\u7f16\u8f91\u5df2\u53d1\u9001\u6d88\u606f' }).value).toBe('draft survives')
  })

  test('disables edit-resend while the current conversation is running', () => {
    render(
      <ChatMessage
        message={{ id: 'user-edit-busy', role: 'user', content: 'cannot edit yet' }}
        pending={false}
        onAskReply={vi.fn()}
        onEditResend={vi.fn()}
        editDisabled
      />,
    )

    const editButton = screen.getByRole('button', { name: '\u7f16\u8f91\u5e76\u91cd\u65b0\u53d1\u9001' })
    expect(editButton.disabled).toBe(true)
    fireEvent.click(editButton)
    expect(screen.queryByRole('textbox', { name: '\u7f16\u8f91\u5df2\u53d1\u9001\u6d88\u606f' })).toBeNull()
  })

  test('renders worldline node IDs so a restore command can reference them', () => {
    render(
      <ChatMessage
        message={{ id: 'worldline-nodes', role: 'assistant', commandResult: { command:'worldline', action:'list', tree:{ nodes:[{ id:'node-42', title:'Checkpoint' }] } } }}
        pending={false}
        onAskReply={vi.fn()}
      />,
    )

    expect(screen.getByText('node-42')).toBeTruthy()
    expect(screen.getByText('Checkpoint')).toBeTruthy()
  })
})




describe('file workflow confidence', () => {
  const fileT = {
    lists: { fileList: 'Files', filePreview: 'Preview' },
    hints: { filePath: 'Path', searchText: 'Search text', tailLines: 'Tail lines' },
    read: 'Read',
    search: 'Search',
    tail: 'Tail',
    download: 'Download',
    delete: 'Delete',
    save: 'Save',
    empty: 'No content',
  }

  const baseProps = () => ({
    t: fileT,
    filePath: '',
    setFilePath: vi.fn(),
    fileList: [],
    fileContent: '',
    loadedFileContent: '',
    loadedFilePath: '',
    setFileContent: vi.fn(),
    fileSearch: '',
    setFileSearch: vi.fn(),
    searchHits: [],
    tailLines: 100,
    setTailLines: vi.fn(),
    loadFiles: vi.fn(),
    readFile: vi.fn(),
    tailFile: vi.fn(),
    saveFile: vi.fn(),
    discardChanges: vi.fn(),
    deleteFile: vi.fn(),
    downloadFile: vi.fn(),
    runSearch: vi.fn(),
    busy: false,
    fileStatus: null,
    dismissFileStatus: vi.fn(),
  })

  test('starts empty and explains why Save is disabled', () => {
    render(<FilesPage {...baseProps()} />)

    const save = screen.getByRole('button', { name: 'Save' })
    expect(save.disabled).toBe(true)
    expect(save.getAttribute('aria-describedby')).toBe('file-save-reason')
    expect(document.getElementById('file-save-reason')?.textContent).toMatch(/Read a file before saving/i)
    expect(screen.getByText(/尚未加载文件/)).toBeTruthy()
  })

  test('shows dirty and retargeted state, saves explicitly, and can discard', () => {
    const props = baseProps()
    Object.assign(props, {
      filePath: 'C:/ga/renamed.txt',
      loadedFilePath: 'C:/ga/original.txt',
      loadedFileContent: 'before',
      fileContent: 'after',
    })
    render(<FilesPage {...props} />)

    expect(screen.getByText('有未保存更改')).toBeTruthy()
    expect(screen.getByText('Save target changed')).toBeTruthy()
    expect(document.querySelector('.file-save-review')?.textContent).toMatch(/renamed\.txt/)
    const save = screen.getByRole('button', { name: 'Save' })
    expect(save.disabled).toBe(false)
    fireEvent.click(save)
    expect(props.saveFile).toHaveBeenCalledTimes(1)
    fireEvent.click(screen.getByRole('button', { name: /Discard changes/i }))
    expect(props.discardChanges).toHaveBeenCalledTimes(1)
  })

  test('keeps no-match search distinct from the initial search hint', () => {
    const props = baseProps()
    Object.assign(props, {
      filePath: 'C:/ga',
      fileSearch: 'missing-token',
      fileStatus: { kind: 'success', action: 'search', message: 'No matches found for \"missing-token\".' },
    })
    render(<FilesPage {...props} />)

    expect(screen.getByText('No matches found')).toBeTruthy()
    expect(screen.queryByText(/Enter search text, then run search/)).toBeNull()
  })

  test('renders save success and a recoverable save error', () => {
    const successProps = baseProps()
    successProps.fileStatus = { kind: 'success', message: 'Saved C:/ga/a.txt' }
    const { rerender } = render(<FilesPage {...successProps} />)
    expect(screen.getByText('Saved C:/ga/a.txt')).toBeTruthy()

    const errorProps = baseProps()
    const retrySave = vi.fn()
    errorProps.fileStatus = { kind: 'error', message: 'Save failed: disk full', onRetry: retrySave }
    rerender(<FilesPage {...errorProps} />)
    expect(screen.getByRole('alert').textContent).toMatch(/Save failed: disk full/)
    fireEvent.click(screen.getByRole('button', { name: /Retry file action/i }))
    expect(retrySave).toHaveBeenCalledTimes(1)
  })
})

describe('operator shell feedback', () => {
  const shellPayload = (url) => {
    const path = new URL(url, 'http://localhost').pathname
    const payloads = {
      '/api/config': { host: '127.0.0.1', port: 8900, ga_root: 'C:/ga' },
      '/api/ga/health': { ok: true },
      '/api/autostart/status': { supported: true, enabled: false },
      '/api/version/info': { version: 'dev' },
      '/api/version/status': {},
      '/api/observability/health': { ok: true },
      '/api/observability/inventory': {},
      '/api/observability/risks': {},
      '/api/services': { services: [] },
    }
    return jsonResponse(payloads[path] ?? {})
  }

  test('navigation exposes the selected route with native keyboard semantics', async () => {
    installBrowserPolyfills()
    globalThis.fetch = vi.fn(async (url) => shellPayload(url))
    render(<App />)
    const files = await screen.findByRole('button', { name: /文件|Files/i })
    const overview = screen.getByRole('button', { name: /总览|Overview/i })
    expect(overview.getAttribute('aria-current')).toBe('page')
    files.focus()
    expect(document.activeElement).toBe(files)
    expect(files.tagName).toBe('BUTTON')
    fireEvent.click(files)
    expect(files.getAttribute('aria-current')).toBe('page')
    expect(files.disabled).toBe(false)
  })

  test('refresh shows pending, success, and a recoverable error', async () => {
    installBrowserPolyfills()
    let configCalls = 0
    let releaseRefresh
    globalThis.fetch = vi.fn((url) => {
      const path = new URL(url, 'http://localhost').pathname
      if (path === '/api/config') {
        configCalls += 1
        if (configCalls === 2) return new Promise(resolve => { releaseRefresh = () => resolve(shellPayload(url)) })
        if (configCalls === 3) return Promise.reject(new Error('network offline'))
      }
      return Promise.resolve(shellPayload(url))
    })
    render(<App />)
    await screen.findByText(/运行状态已刷新/)
    const refresh = document.querySelector('button.refresh')
    expect(refresh).toBeTruthy()

    fireEvent.click(refresh)
    expect(await screen.findByText(/正在刷新运行状态/)).toBeTruthy()
    expect(refresh.disabled).toBe(true)
    releaseRefresh()
    expect(await screen.findByText(/运行状态已刷新/)).toBeTruthy()
    await waitFor(() => expect(refresh.disabled).toBe(false))

    fireEvent.click(refresh)
    const alert = await screen.findByRole('alert')
    expect(alert.textContent).toMatch(/刷新失败.*network offline/i)
    expect(screen.getByRole('button', { name: /刷新状态/ }).disabled).toBe(false)
  })

  test('service actions stay local to one card and expose failure recovery', async () => {
    installBrowserPolyfills()
    window.history.replaceState({}, '', '/channels')
    vi.spyOn(window, 'confirm').mockReturnValue(true)
    const services = [
      { name: 'alpha-ui', kind: 'frontend', running: false, autostart: false },
      { name: 'beta-ui', kind: 'frontend', running: false, autostart: false },
    ]
    let actionAttempts = 0
    let rejectAction
    globalThis.fetch = vi.fn((url, options = {}) => {
      const path = new URL(url, 'http://localhost').pathname
      if (path === '/api/services/start' && options.method === 'POST') {
        actionAttempts += 1
        if (actionAttempts === 1) return new Promise((resolve, reject) => { rejectAction = reject })
        return Promise.resolve(jsonResponse({ ok: true }))
      }
      if (path === '/api/services') return Promise.resolve(jsonResponse({ services }))
      return Promise.resolve(shellPayload(url))
    })

    render(<App />)
    const alphaLabel = await screen.findByText('alpha-ui')
    const betaLabel = screen.getByText('beta-ui')
    const alphaCard = alphaLabel.closest('article')
    const betaCard = betaLabel.closest('article')
    const alphaStart = alphaCard.querySelectorAll('button')[0]
    const betaStart = betaCard.querySelectorAll('button')[0]

    fireEvent.click(alphaStart)
    await waitFor(() => expect(alphaCard.getAttribute('aria-busy')).toBe('true'))
    expect(alphaStart.disabled).toBe(true)
    expect(betaStart.disabled).toBe(false)
    fireEvent.click(alphaStart)
    expect(actionAttempts).toBe(1)

    rejectAction(new Error('backend offline'))
    const actionAlert = await screen.findByRole('alert')
    expect(actionAlert.textContent).toContain('backend offline')
    expect(betaStart.disabled).toBe(false)

    fireEvent.click(screen.getByRole('button', { name: /Retry|\u91cd\u8bd5/i }))
    await waitFor(() => expect(actionAttempts).toBe(2))
    await waitFor(() => expect(alphaCard.querySelector('.service-action-status.success')).toBeTruthy())
  })

  test('log streaming distinguishes selection, connection, empty, failure, retry, pause, and resume', async () => {
    installBrowserPolyfills()
    window.history.replaceState({}, '', '/logs')
    const services = [{ name: 'alpha-worker', kind: 'task', running: true, pid: 42, command: ['agentmain', '--worker'] }]
    globalThis.fetch = vi.fn((url) => {
      const path = new URL(url, 'http://localhost').pathname
      if (path === '/api/services') return Promise.resolve(jsonResponse({ services }))
      return Promise.resolve(shellPayload(url))
    })

    const streams = []
    class FakeEventSource {
      constructor(url) { this.url = url; this.listeners = {}; this.close = vi.fn(); streams.push(this) }
      addEventListener(name, handler) { this.listeners[name] = handler }
      emit(name, payload) { this.listeners[name]?.({ data: JSON.stringify(payload) }) }
    }
    vi.stubGlobal('EventSource', FakeEventSource)

    render(<App />)
    await screen.findByText('alpha-worker')
    expect(document.querySelector('.log-selection-empty')).toBeTruthy()

    fireEvent.click(screen.getByText('alpha-worker').closest('button'))
    await waitFor(() => expect(streams).toHaveLength(1))
    expect(streams[0].url).toBe('/api/logs/alpha-worker/stream?lines=200')
    expect(document.querySelector('.stream-state.connecting')).toBeTruthy()

    streams[0].onopen()
    streams[0].emit('snapshot', { lines: [] })
    await waitFor(() => expect(document.querySelector('.log-output-empty')).toBeTruthy())

    streams[0].onerror()
    const streamAlert = await screen.findByRole('alert')
    expect(streamAlert.textContent).toMatch(/log|\u65e5\u5fd7/i)
    fireEvent.click(screen.getByRole('button', { name: /Retry|\u91cd\u8bd5/i }))
    await waitFor(() => expect(streams).toHaveLength(2))
    expect(streams[0].close).toHaveBeenCalled()

    streams[1].onopen()
    streams[1].emit('log', { line: 'ready' })
    expect(await screen.findByText('ready')).toBeTruthy()
    const logView = document.querySelector('.log-view')
    Object.defineProperties(logView, {
      scrollHeight: { configurable: true, value: 500 },
      clientHeight: { configurable: true, value: 100 },
      scrollTop: { configurable: true, writable: true, value: 0 },
    })
    fireEvent.scroll(logView)
    const follow = screen.getByRole('button', { name: /Follow|\u8ddf\u968f/i })
    expect(follow.getAttribute('aria-pressed')).toBe('false')
    expect(document.querySelector('.log-follow-status.paused')).toBeTruthy()
    fireEvent.click(follow)
    expect(follow.getAttribute('aria-pressed')).toBe('true')
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
