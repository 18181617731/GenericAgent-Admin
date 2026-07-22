import { readFileSync } from 'node:fs'
import React from 'react'
import { afterEach, describe, expect, test, vi } from 'vitest'
import { cleanup, fireEvent, render, screen, waitFor } from '@testing-library/react'
import { ChannelServiceTable, ObservabilityCard, ServiceRow } from './components/common.jsx'
import App, { ChannelsPage } from './App.jsx'
import ChatApp, { ChatMessage, PlanTodoCard, ProviderModelCascade } from './ChatApp.jsx'
import { GoalsPage } from './pages/GoalsPage.jsx'
import { Models } from './pages/ModelsPage.jsx'
import { FilesPage } from './pages/FilesPage.jsx'
import { SettingsPage } from './pages/SettingsPage.jsx'
import { GlobalFeedback, MessageBanner } from './components/feedback.jsx'
import { SchedulerServiceRow } from './components/schedule.jsx'

globalThis.React = React
globalThis.ResizeObserver = class ResizeObserver {
  observe() {}
  unobserve() {}
  disconnect() {}
}

const appStyles = readFileSync('src/style.css', 'utf8')

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

describe('plan todo card disclosure', () => {
  test('starts expanded and toggles the plan body with matching chevrons', () => {
    const { container } = render(<PlanTodoCard plan={{
      active: true,
      done: 1,
      total: 2,
      items: [
        { status: 'done', content: 'Inspect the task' },
        { status: 'in_progress', content: 'Implement collapse' },
      ],
      step: 'Editing the plan card',
    }}/>)

    const collapseButton = screen.getByRole('button', { name: '收起执行计划' })
    const body = container.querySelector('.oa-plan-body')
    expect(collapseButton.getAttribute('aria-expanded')).toBe('true')
    expect(collapseButton.getAttribute('aria-controls')).toBe(body?.id)
    expect(body?.hidden).toBe(false)
    expect(collapseButton.querySelector('.lucide-chevron-down')).toBeTruthy()

    fireEvent.click(collapseButton)

    const expandButton = screen.getByRole('button', { name: '展开执行计划' })
    expect(expandButton.getAttribute('aria-expanded')).toBe('false')
    expect(body?.hidden).toBe(true)
    expect(expandButton.querySelector('.lucide-chevron-left')).toBeTruthy()

    fireEvent.click(expandButton)
    expect(screen.getByRole('button', { name: '收起执行计划' }).getAttribute('aria-expanded')).toBe('true')
    expect(body?.hidden).toBe(false)
  })
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

describe('scheduled-task service controls', () => {
  test('blocks duplicate actions while pending and retries a failed scheduler start', () => {
    const service = { name: 'reflect/scheduler.py', kind: 'reflect', running: false, autostart: false }
    const onStart = vi.fn()
    const schedulerT = { ...t, nav: { logs: 'Logs' }, retry: 'Retry', serviceDesc: { scheduler: 'Scheduled task runner' } }
    const view = render(<SchedulerServiceRow
      service={service}
      t={schedulerT}
      actionState={{ action: 'start', status: 'pending', message: 'Start: Busy' }}
      onStart={onStart}
      onStop={vi.fn()}
      onLogs={vi.fn()}
      onAutostart={vi.fn()}
    />)

    expect(screen.getByRole('button', { name: 'Start' }).disabled).toBe(true)
    expect(screen.getByRole('status').textContent).toMatch(/Start: Busy/i)

    view.rerender(<SchedulerServiceRow
      service={service}
      t={schedulerT}
      actionState={{ action: 'start', status: 'error', message: 'Start: Error · port in use' }}
      onStart={onStart}
      onStop={vi.fn()}
      onLogs={vi.fn()}
      onAutostart={vi.fn()}
    />)
    fireEvent.click(screen.getByRole('button', { name: 'Retry' }))
    expect(onStart).toHaveBeenCalledWith(service.name)
  })
})

describe('overview observability', () => {
  test('explains system state and important counts without raw internal labels', () => {
    const onRefresh = vi.fn()
    render(<ObservabilityCard snapshot={{
      ok: true,
      coreFiles: [{ exists: true }, { exists: true }],
      runtime: { ok: true, pythonOK: true, pythonPath: 'C:\\Python\\python.exe', pythonVersion: '3.12.1', dependencies: [{ ok: true }, { ok: true }], missingModules: [], agentmainOK: true, ultraplanOK: true },
      memory: { sops: [{}, {}] },
      riskItems: [{}, {}, {}],
      warnings: [],
      errors: [],
      generatedAt: '2026-07-20T10:00:00+08:00',
    }} onRefresh={onRefresh}/>)
    expect(screen.getByText('运行概览')).toBeTruthy()
    expect(screen.getByText('系统状态')).toBeTruthy()
    expect(screen.getByText('实际 Python')).toBeTruthy()
    expect(screen.getByText('核心依赖')).toBeTruthy()
    expect(screen.getByText('GA 运行检查')).toBeTruthy()
    expect(screen.queryByText('Health checks')).toBeNull()
    fireEvent.click(screen.getByRole('button', { name: '刷新' }))
    expect(onRefresh).toHaveBeenCalledOnce()
  })

  test('shows actionable runtime errors and invokes one-click repair', () => {
    const onRepair = vi.fn()
    render(<ObservabilityCard snapshot={{
      ok: false,
      coreFiles: [{ exists: true }],
      runtime: { ok: false, pythonOK: true, pythonPath: 'python.exe', dependencies: [{ module: 'requests', ok: false }], missingModules: ['requests'], agentmainOK: false, ultraplanOK: true, repairable: true },
      errors: ["核心依赖缺失: requests"],
    }} onRepair={onRepair} onRefresh={vi.fn()}/>)
    expect(screen.getByText('核心依赖缺失: requests')).toBeTruthy()
    fireEvent.click(screen.getByRole('button', { name: '一键修复' }))
    expect(onRepair).toHaveBeenCalledOnce()
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

    const body = container.querySelector('.oa-msg-body')
    const meta = container.querySelector('.oa-meta')
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

  test('shows the selected provider and model while the assistant response is pending', () => {
    const { container } = render(
      <ChatMessage
        message={{ id: 'pending-model', role: 'assistant', content: '', llm_no: 7, created_at: 0 }}
        models={[{ index: 7, provider: '服务商 A', model: 'model-v1' }]}
        pending
        onAskReply={vi.fn()}
      />,
    )

    const badge = container.querySelector('.oa-model-id')
    expect(badge?.textContent).toBe('服务商 A · model-v1')
    expect(badge?.textContent).not.toBe('未知模型')
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

  test('repeated clicks at one position keep moving the same model upward', () => {
    const orderedProfile = {
      ...modelProfile,
      model: 'model-a',
      models: ['model-a', 'model-b', 'model-c', 'model-d'],
      model_configs: ['model-a', 'model-b', 'model-c', 'model-d'].map(model => ({ model })),
    }
    const props = modelProps({
      profiles: [orderedProfile],
      persistedProfiles: [orderedProfile],
      onSaveModelOrder: vi.fn(async () => true),
    })
    const { container } = render(<Models {...props} />)

    fireEvent.click(container.querySelector('.model-page-actions button[title="调整已保存模型的全局顺序"]'))
    fireEvent.click(container.ownerDocument.querySelector('button[aria-label="上移 model-d"]'), {
      clientX: 520,
      clientY: 420,
      detail: 1,
    })
    expect([...container.ownerDocument.querySelectorAll('.model-order-copy strong')].map(node => node.textContent))
      .toEqual(['model-a', 'model-b', 'model-d', 'model-c'])

    fireEvent.click(container.ownerDocument.querySelector('button[aria-label="上移 model-c"]'), {
      clientX: 520,
      clientY: 420,
      detail: 1,
    })

    expect([...container.ownerDocument.querySelectorAll('.model-order-copy strong')].map(node => node.textContent))
      .toEqual(['model-a', 'model-d', 'model-b', 'model-c'])
    expect(container.ownerDocument.querySelector('button[aria-label="上移 model-d"]')?.closest('.model-order-row')?.classList.contains('is-repeat-target')).toBe(true)
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

describe('mobile chat session navigation', () => {
  test('switches a history session with one tap and closes the sidebar', async () => {
    Object.defineProperty(window, 'matchMedia', {
      writable: true,
      value: vi.fn().mockImplementation(query => ({
        matches: /max-width:\s*(?:900|560)px/.test(query) || /prefers-reduced-motion/.test(query),
        media: query,
        addListener: vi.fn(), removeListener: vi.fn(), addEventListener: vi.fn(), removeEventListener: vi.fn(), dispatchEvent: vi.fn(),
      })),
    })
    Element.prototype.scrollIntoView = vi.fn()
    const sessions = [
      { id:'one', title:'First chat', count:2, updated_at:'2026-07-17T12:00:00Z' },
      { id:'two', title:'Second chat', count:4, updated_at:'2026-07-17T13:00:00Z' },
    ]
    globalThis.fetch = vi.fn(async (url) => {
      const path = String(url)
      if (path === '/api/config') return jsonResponse({ slash_commands:[] })
      if (path === '/api/slash-commands') return jsonResponse({ commands:[] })
      if (path === '/api/chat/sessions') return jsonResponse({ sessions })
      if (path.startsWith('/api/chat/session/')) {
        const id = path.split('/').pop()
        const row = sessions.find(item => item.id === id)
        return jsonResponse({ ...row, messages:[], raw_history:[], history_info:[], settings:{ llm_no:0, tools_mode:'official' } })
      }
      if (path.startsWith('/api/chat/state/')) return jsonResponse({ llms:[], settings:{ llm_no:0, tools_mode:'official' } })
      if (path.startsWith('/api/chat/worldline/')) return jsonResponse({ schema_version:1, nodes:[], current_path:[] })
      throw new Error(`unexpected url ${url}`)
    })

    render(<ChatApp />)
    await waitFor(() => expect(document.querySelector('.oa-title b')?.textContent).toBe('First chat'))
    fireEvent.click(screen.getByRole('button', { name:'展开侧栏' }))
    const second = screen.getByRole('button', { name:/Second chat/ })
    fireEvent.click(second)

    await waitFor(() => expect(document.querySelector('.oa-title b')?.textContent).toBe('Second chat'))
    expect(document.querySelector('.oa-sidebar')?.classList.contains('collapsed')).toBe(true)
    expect(screen.queryByRole('button', { name:'关闭侧栏' })).toBeNull()
    expect(globalThis.fetch.mock.calls.filter(([url]) => String(url) === '/api/chat/session/two')).toHaveLength(1)
  }, 15000)
})

describe('assistant generated image gallery', () => {
  test('opens a local generated image preview and exposes original and download actions', () => {
    const path = String.raw`G:\MygenericAgent\temp\comfy output\final image.png`
    render(<ChatMessage message={{ id:'a1', role:'assistant', content:`图片已生成：${path}` }} />)
    const thumb = screen.getByRole('button', { name:'查看原图 final image.png' })
    const image = thumb.querySelector('img')
    expect(image?.getAttribute('src')).toBe(`/api/files/image?path=${encodeURIComponent(path)}`)

    fireEvent.click(thumb)
    expect(screen.getByRole('dialog', { name:'生成图片预览' })).toBeTruthy()
    expect(screen.getByRole('link', { name:'查看原图' }).getAttribute('href')).toBe(`/api/files/image?path=${encodeURIComponent(path)}`)
    expect(screen.getByRole('link', { name:'下载图片' }).getAttribute('href')).toBe(`/api/files/download?path=${encodeURIComponent(path)}`)
    fireEvent.click(screen.getByRole('button', { name:'关闭图片预览' }))
    expect(screen.queryByRole('dialog', { name:'生成图片预览' })).toBeNull()
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
    expect(screen.getByRole('article', { name: 'Markdown 格式化预览' }).textContent).toContain('hello')
    fireEvent.click(screen.getByRole('button', { name: '编辑' }))
    expect(screen.getByRole('textbox', { name: '文件内容编辑器' }).value).toBe('hello')
  })

  test('formats Markdown by default and switches back to preview after editing', () => {
    const props = fileProps()
    Object.assign(props, {
      filePath: 'memory/guide.md',
      loadedFilePath: 'memory/guide.md',
      loadedFileContent: '# Guide\n\n- one\n- two\n\n| Name | State |\n| --- | --- |\n| GA | Ready |',
      fileContent: '# Guide\n\n- one\n- two\n\n| Name | State |\n| --- | --- |\n| GA | Ready |',
    })
    const { rerender } = render(<FilesPage {...props}/>)

    expect(screen.getByRole('heading', { name: 'Guide' })).toBeTruthy()
    expect(screen.getByRole('table')).toBeTruthy()
    expect(screen.queryByRole('textbox', { name: '文件内容编辑器' })).toBeNull()
    fireEvent.click(screen.getByRole('button', { name: '编辑' }))
    const editor = screen.getByRole('textbox', { name: '文件内容编辑器' })
    const updated = `${props.fileContent}\n\n追加内容`
    fireEvent.change(editor, { target: { value: updated } })
    expect(props.setFileContent).toHaveBeenCalledWith(updated)
    rerender(<FilesPage {...props} fileContent={updated}/>)
    fireEvent.click(screen.getByRole('button', { name: '预览' }))
    expect(screen.getByText('追加内容')).toBeTruthy()
    expect(screen.queryByRole('textbox', { name: '文件内容编辑器' })).toBeNull()
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
