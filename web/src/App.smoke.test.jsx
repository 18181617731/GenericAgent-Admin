import { readFileSync } from 'node:fs'
import React from 'react'
import { afterEach, describe, expect, test, vi } from 'vitest'
import { cleanup, fireEvent, render, screen, waitFor, within } from '@testing-library/react'
import { ChannelServiceTable, ObservabilityCard, ServiceRow } from './components/common.jsx'
import App, { ChannelsPage, I18N } from './App.jsx'
import ChatApp, { ChatMessage, GoalStatusCard, PlanTodoCard, ProviderModelCascade, WorldlinePanel } from './ChatApp.jsx'
import { GoalsPage } from './pages/GoalsPage.jsx'
import { Models } from './pages/ModelsPage.jsx'
import { draftChangeSummary } from './lib/modelsEditor.js'
import { FilesPage } from './pages/FilesPage.jsx'
import { SettingsPage } from './pages/SettingsPage.jsx'
import { UsagePage } from './pages/UsagePage.jsx'
import { AutonomousPage } from './pages/AutonomousPage.jsx'
import { GlobalFeedback, MessageBanner } from './components/feedback.jsx'
import { SchedulerServiceRow } from './components/schedule.jsx'
import { SubagentStatusPanel } from './components/SubagentStatusPanel.jsx'
import { EnvironmentGuardianSection, GoalWorkflowGuide } from './components/ServicePlacement.jsx'
import { ModuleTodoPanel } from './components/ModuleTodoPanel.jsx'
import { registerDialogAdapter } from './lib/danger.js'

globalThis.React = React
globalThis.ResizeObserver = class ResizeObserver {
  observe() {}
  unobserve() {}
  disconnect() {}
}

const appStyles = readFileSync('src/style.css', 'utf8')
const adminMobileStyles = readFileSync('src/admin-mobile.css', 'utf8')

globalThis.React = React

const t = {
  ...I18N.en,
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

const installStableChatPolyfills = () => {
  installBrowserPolyfills()
  Object.defineProperty(window, 'matchMedia', {
    configurable: true,
    writable: true,
    value: vi.fn().mockImplementation(query => ({
      matches: /prefers-reduced-motion/.test(query),
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


let unregisterDialogAdapter = () => {}
const mockDialog = (result = true) => {
  const adapter = vi.fn(() => result)
  unregisterDialogAdapter()
  unregisterDialogAdapter = registerDialogAdapter(adapter)
  return adapter
}

afterEach(() => {
  unregisterDialogAdapter()
  unregisterDialogAdapter = () => {}
  cleanup()
  window.localStorage.clear()
  window.history.replaceState({}, '', '/')
  vi.restoreAllMocks()
})

const pendingApproval = {
  id: 'draft-one', title: '补充自主操作 SOP', state: 'pending', status: '待批未落地',
  source: 'R37', target: 'memory/autonomous_sop.md', problem: '避免自主操作方案只停留在报告里，实际执行时没有统一依据', risk: '低', evidence: '目标文件不存在', next_step: '批准后生成文档',
}

const approvalOverview = (items = [pendingApproval]) => ({
  source_exists: true,
  items,
  pending: items.filter(item => item.state === 'pending').length,
  approved: items.filter(item => item.state === 'approved').length,
  rejected: items.filter(item => item.state === 'rejected').length,
})

describe('autonomous operations page', () => {
  test('should approve and queue a pending draft when the user confirms', async () => {
    installBrowserPolyfills()
    const decided = { ...pendingApproval, state: 'approved', decision: 'approved', decided_at: '2026-07-28T10:00:00Z' }
    globalThis.fetch = vi.fn(async (url, options = {}) => {
      if (String(url) === '/api/autonomous/approvals' && options.method === 'POST') return jsonResponse({ queued: true, overview: approvalOverview([decided]) })
      if (String(url) === '/api/autonomous/approvals') return jsonResponse(approvalOverview())
      throw new Error(`unexpected url ${url}`)
    })
    vi.spyOn(window, 'confirm').mockReturnValue(true)
    const setMessage = vi.fn()
    render(<AutonomousPage lang="zh" reports={[]} setMessage={setMessage}/>)

    fireEvent.click(await screen.findByRole('tab', { name: '待审批 (1)' }))
    fireEvent.change(screen.getByRole('textbox', { name: '审批意见或补充要求（可选）' }), { target: { value: '先验证，再执行' } })
    fireEvent.click(screen.getByRole('button', { name: '批准并加入队列' }))

    await waitFor(() => expect(setMessage).toHaveBeenCalledWith('已批准并加入自主任务队列', 'success'))
    const post = globalThis.fetch.mock.calls.find(([, options]) => options?.method === 'POST')
    expect(post?.[1]?.headers?.['X-GA-Confirm']).toBe('dangerous')
    expect(JSON.parse(post?.[1]?.body)).toEqual({ id: 'draft-one', decision: 'approved', note: '先验证，再执行' })
    fireEvent.click(screen.getByRole('tab', { name: /已处理/ }))
    expect((await screen.findAllByText('已批准')).length).toBeGreaterThan(0)
  })

  test('should group handled approvals and toggle individual status groups', async () => {
    installBrowserPolyfills()
    const approved = { ...pendingApproval, id: 'approved-one', title: '已批准任务', state: 'approved', decision: 'approved', execution_state: 'completed' }
    const rejected = { ...pendingApproval, id: 'rejected-one', title: '已拒绝任务', state: 'rejected', decision: 'rejected' }
    const archived = { ...pendingApproval, id: 'archived-one', title: '历史归档任务', state: 'closed' }
    globalThis.fetch = vi.fn(async url => {
      if (String(url) === '/api/autonomous/approvals') return jsonResponse(approvalOverview([approved, rejected, archived]))
      throw new Error(`unexpected url ${url}`)
    })
    render(<AutonomousPage lang="zh" reports={[]}/>)

    fireEvent.click(await screen.findByRole('tab', { name: '待审批' }))
    fireEvent.click(await screen.findByRole('tab', { name: /已处理/ }))
    const summaries = () => Array.from(document.querySelectorAll('.autonomous-approval-group > summary'))
    const approvedSummary = summaries().find(summary => summary.textContent.includes('已完成并归档'))
    const rejectedSummary = summaries().find(summary => summary.textContent.includes('已拒绝，不执行'))
    expect(summaries()).toHaveLength(3)
    expect(approvedSummary?.closest('details')?.open).toBe(true)
    expect(rejectedSummary?.closest('details')?.open).toBe(false)

    fireEvent.click(rejectedSummary)
    await waitFor(() => expect(rejectedSummary?.closest('details')?.open).toBe(true))
    fireEvent.click(rejectedSummary)
    await waitFor(() => expect(rejectedSummary?.closest('details')?.open).toBe(false))
  })

  test('should exclude completed and no-approval items from the pending view', async () => {
    installBrowserPolyfills()
    const completed = { ...pendingApproval, id: 'completed-stale', title: '已完成但台账未更新', status: '已完成并通过验证' }
    const noApproval = { ...pendingApproval, id: 'no-approval', title: '无需审批的例行检查', status: '无需审批' }
    const current = { ...pendingApproval, id: 'still-pending', title: '仍需人工确认' }
    globalThis.fetch = vi.fn(async url => {
      if (String(url) === '/api/autonomous/approvals') return jsonResponse(approvalOverview([completed, noApproval, current]))
      throw new Error(`unexpected url ${url}`)
    })
    render(<AutonomousPage lang="zh" reports={[]}/> )

    expect(await screen.findByRole('tab', { name: '待审批 (1)' })).toBeTruthy()
    fireEvent.click(screen.getByRole('tab', { name: '待审批 (1)' }))
    expect(await screen.findByText('仍需人工确认')).toBeTruthy()
    expect(screen.queryByText('已完成但台账未更新')).toBeNull()
    expect(screen.queryByText('无需审批的例行检查')).toBeNull()
    fireEvent.click(screen.getByRole('tab', { name: /已处理/ }))
    expect(await screen.findByText('已完成但台账未更新')).toBeTruthy()
    expect(await screen.findByText('无需审批的例行检查')).toBeTruthy()
  })

  test('should show live bulk approval progress and retry failed items', async () => {
    installBrowserPolyfills()
    const first = { ...pendingApproval, id: 'draft-one', title: '第一项自主任务' }
    const second = { ...pendingApproval, id: 'draft-two', title: '第二项自主任务' }
    let resolveFirst
    let resolveSecond
    let secondAttempts = 0
    const firstResponse = new Promise(resolve => { resolveFirst = resolve })
    const secondResponse = new Promise(resolve => { resolveSecond = resolve })
    globalThis.fetch = vi.fn(async (url, options = {}) => {
      if (String(url) === '/api/autonomous/approvals' && options.method === 'POST') {
        const body = JSON.parse(options.body)
        if (body.id === first.id) return firstResponse
        secondAttempts += 1
        if (secondAttempts === 1) return secondResponse
        return jsonResponse({ queued: true, overview: approvalOverview([]) })
      }
      if (String(url) === '/api/autonomous/approvals') return jsonResponse(approvalOverview([first, second]))
      throw new Error(`unexpected url ${url}`)
    })
    vi.spyOn(window, 'confirm').mockReturnValue(true)
    render(<AutonomousPage lang="zh" reports={[]} />)

    fireEvent.click(await screen.findByRole('tab', { name: '待审批 (2)' }))
    fireEvent.click(screen.getByRole('button', { name: '全选待审批' }))
    fireEvent.click(screen.getByRole('button', { name: '批量批准并加入队列' }))

    expect(await screen.findByRole('status', { name: '批量处理进度' })).toBeTruthy()
    expect(screen.getByText('已处理 0 / 2 项')).toBeTruthy()
    resolveFirst(jsonResponse({ queued: true, overview: approvalOverview([second]) }))
    await waitFor(() => expect(screen.getByText('已处理 1 / 2 项')).toBeTruthy())
    expect(screen.getByText('成功 1 项')).toBeTruthy()
    resolveSecond({ ok: false, status: 500, statusText: 'Server Error', text: async () => JSON.stringify({ error: '第二项校验失败' }) })
    await waitFor(() => expect(screen.getByText('失败原因：第二项校验失败')).toBeTruthy())
    expect(screen.getByText('失败 1 项')).toBeTruthy()
    fireEvent.click(screen.getByRole('button', { name: '重试失败项（1）' }))

    await waitFor(() => expect(screen.getByText('已处理 1 / 1 项')).toBeTruthy())
    expect(screen.getByText('成功 1 项')).toBeTruthy()
    expect(screen.queryByText('失败原因：第二项校验失败')).toBeNull()
    expect(secondAttempts).toBe(2)
  })

  test('should render generated approval-card values in Chinese', async () => {
    installBrowserPolyfills()
    const englishReview = {
      ...pendingApproval,
      title: 'R49_complete_task approval review',
      status: 'report requires human approval',
      risk: 'human review required',
      evidence: 'approval evidence is missing or unverifiable',
      next_step: 'Review the report evidence, then approve or reject explicitly',
      review_decision: 'needs_approval',
      review_confidence: 'high',
      review_reason: 'report is blocked; the proposed source change is not confirmed as implemented; model review unavailable: model review in progress; conservative rule retained',
    }
    globalThis.fetch = vi.fn(async url => {
      if (String(url) === '/api/autonomous/approvals') return jsonResponse(approvalOverview([englishReview]))
      throw new Error(String(url))
    })
    render(<AutonomousPage lang="zh" reports={[]}/>)

    fireEvent.click(await screen.findByRole('tab', { name: '待审批 (1)' }))
    expect(await screen.findByText('报告需要人工审批')).toBeTruthy()
    expect(screen.getByText('需要人工复核')).toBeTruthy()
    expect(screen.getByText('未自动批准')).toBeTruthy()
    expect(screen.getByText('高')).toBeTruthy()
    expect(screen.queryByText('请核查报告证据后明确批准或拒绝')).toBeNull()
    expect(screen.getByText(/报告处于阻塞状态/)).toBeTruthy()
    expect(screen.getByText('避免自主操作方案只停留在报告里，实际执行时没有统一依据')).toBeTruthy()
    expect(screen.queryByText('human review required')).toBeNull()
  })

  test('should link an approved task to its execution result', async () => {
    installBrowserPolyfills()
    const report = { name: 'R99_execution.md', path: 'temp/autonomous_reports/R99_execution.md', mod_time: '2026-07-28T10:00:00Z' }
    const decided = {
      ...pendingApproval,
      state: 'approved',
      decision: 'approved',
      decided_at: '2026-07-28T09:00:00Z',
      execution_state: 'completed',
      execution_report: report,
      execution_summary: '已完成并通过验证',
    }
    globalThis.fetch = vi.fn(async (url, options = {}) => {
      if (String(url) === '/api/autonomous/approvals' && options.method === 'POST') return jsonResponse({ queued: true, overview: approvalOverview([decided]) })
      if (String(url) === '/api/autonomous/approvals') return jsonResponse(approvalOverview())
      if (String(url).startsWith('/api/files/read')) return jsonResponse({ content: '# 执行完成\n\n已完成并通过验证' })
      throw new Error(`unexpected url ${url}`)
    })
    vi.spyOn(window, 'confirm').mockReturnValue(true)
    render(<AutonomousPage lang="zh" reports={[report]}/>)

    fireEvent.click(await screen.findByRole('tab', { name: '待审批 (1)' }))
    fireEvent.click(screen.getByRole('button', { name: '批准并加入队列' }))
    fireEvent.click(await screen.findByRole('tab', { name: /已处理/ }))
    expect(await screen.findByText('已完成')).toBeTruthy()
    expect(screen.getByText(/已完成并通过验证/)).toBeTruthy()
    fireEvent.click(screen.getByRole('button', { name: /查看执行结果：R99_execution\.md/ }))
    expect(await screen.findByRole('heading', { name: '执行完成' })).toBeTruthy()
  })

  test('should keep rejection dialog actionable when the decision request fails', async () => {
    installBrowserPolyfills()
    globalThis.fetch = vi.fn(async (url, options = {}) => {
      if (String(url) === '/api/autonomous/approvals' && options.method === 'POST') return { ok: false, status: 500, statusText: 'Server Error', text: async () => JSON.stringify({ error: 'write failed' }) }
      return jsonResponse(approvalOverview())
    })
    vi.spyOn(window, 'confirm').mockReturnValue(true)
    const setMessage = vi.fn()
    render(<AutonomousPage lang="zh" reports={[]} setMessage={setMessage}/>)

    fireEvent.click(await screen.findByRole('tab', { name: '待审批 (1)' }))
    fireEvent.click(screen.getByRole('button', { name: '拒绝' }))
    fireEvent.change(screen.getByRole('textbox', { name: '拒绝原因（可选）' }), { target: { value: '暂不处理' } })
    fireEvent.click(screen.getByRole('button', { name: '确认拒绝' }))

    await waitFor(() => expect(setMessage).toHaveBeenCalledWith(expect.stringContaining('write failed'), 'error'))
    expect(screen.getByRole('dialog')).toBeTruthy()
    expect(screen.getByRole('button', { name: '确认拒绝' }).disabled).toBe(false)
  })

  test('should render report markdown safely and return to the record list', async () => {
    installBrowserPolyfills()
    globalThis.fetch = vi.fn(async url => {
      if (String(url) === '/api/autonomous/approvals') return jsonResponse(approvalOverview([]))
      if (String(url).startsWith('/api/files/read')) return jsonResponse({ content: '# 执行完成\n\n<script>window.__autonomousInjected=true</script>\n\n| 项目 | 结果 |\n| --- | --- |\n| 验证 | 通过 |' })
      throw new Error(`unexpected url ${url}`)
    })
    const reports = [{ name: 'daily-review.md', path: 'temp/autonomous_reports/daily-review.md', mod_time: '2026-07-28T10:00:00Z' }]
    const { container } = render(<AutonomousPage lang="zh" reports={reports}/>)

    fireEvent.click(screen.getByRole('tab', { name: '执行记录' }))
    fireEvent.click(screen.getByRole('button', { name: /daily-review\.md/ }))

    expect(await screen.findByRole('heading', { name: '执行完成' })).toBeTruthy()
    expect(container.querySelector('.autonomous-markdown script')).toBeNull()
    expect(globalThis.window.__autonomousInjected).toBeUndefined()
    fireEvent.click(screen.getByRole('button', { name: '返回记录列表' }))
    expect(screen.getByText('选择左侧记录查看详情')).toBeTruthy()
  })

  test('should expose detailed keyboard-accessible help for autonomous services', async () => {
    installBrowserPolyfills()
    globalThis.fetch = vi.fn(async () => jsonResponse(approvalOverview([])))
    render(<AutonomousPage lang="zh" services={[{ name: 'reflect/autonomous.py', running: false }]}/>)

    // Services render in compact form on the default overview tab.
    expect(await screen.findByText('主自主引擎')).toBeTruthy()
    fireEvent.click(screen.getByText('说明与运行详情'))
    const help = await screen.findByText(/自主任务队列/)
    expect(help).toBeTruthy()
  })
})

describe('service placement experience', () => {
  test('shows watchdog under runtime protection without a model selector', () => {
    const onStart = vi.fn()
    render(<EnvironmentGuardianSection
      services={[{ name: 'reflect/watchdog.py', kind: 'guardian', running: false, command: ['python', 'reflect/watchdog.py'] }]}
      onStart={onStart}
      onStop={vi.fn()}
      onLogs={vi.fn()}
      onAutostart={vi.fn()}
    />)

    expect(screen.getByRole('region', { name: '运行保障' })).toBeTruthy()
    expect(screen.getByText('服务看护器')).toBeTruthy()
    expect(screen.queryByText('执行模型')).toBeNull()
    fireEvent.click(screen.getByRole('button', { name: '启动' }))
    expect(onStart).toHaveBeenCalledWith('reflect/watchdog.py')
  })

  test('explains Goal workflow components without standalone service controls', () => {
    render(<GoalWorkflowGuide services={[
      { name: 'reflect/agent_team_worker.py', kind: 'reflect' },
      { name: 'reflect/checklist_master.py', kind: 'reflect' },
    ]}/>)

    expect(screen.getByRole('region', { name: 'Goal 协作组件' })).toBeTruthy()
    expect(screen.getByText('团队协作工作器')).toBeTruthy()
    expect(screen.getByText('检查清单管理器')).toBeTruthy()
    expect(screen.getAllByText('脚本可用')).toHaveLength(2)
    expect(screen.queryByRole('button')).toBeNull()
    expect(screen.getByText(/不提供独立启动、开机自启或全局模型配置/)).toBeTruthy()
  })
})

describe('chat subagent status presentation', () => {
  test('should distinguish current subagent work from collapsed session history', () => {
    const now = Date.now()
    render(<SubagentStatusPanel states={[
      { name: 'active-task', rounds: 2, updated_at: now, latest_summary: '正在核验下载结果' },
      { name: 'completed-task', rounds: 3, round_ended: true, updated_at: now - 3600000, latest_summary: '报告已生成' },
    ]}/>)

    expect(screen.getByRole('region', { name: '本会话子任务' })).toBeTruthy()
    expect(screen.getByText('每张卡代表一个独立子任务；“子任务第 N 轮”不是历史对话轮次。')).toBeTruthy()
    expect(screen.getByText('子任务第 2 轮 · 最近更新 刚刚')).toBeTruthy()
    const history = screen.getByText('历史任务').closest('details')
    expect(history.open).toBe(false)
    fireEvent.click(screen.getByText('历史任务'))
    expect(history.open).toBe(true)
    expect(screen.getByText('子任务第 3 轮 · 最近更新 1小时前')).toBeTruthy()
  })
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
  test('ChannelsPage leaves the page heading to the app shell', async () => {
    globalThis.fetch = vi.fn(() => Promise.resolve(jsonResponse({ profiles: [] })))

    const { container } = render(
      <ChannelsPage
        frontendSvcs={[]}
        t={t}
        onStart={vi.fn()}
        onStop={vi.fn()}
        onLogs={vi.fn()}
        onAutostart={vi.fn()}
        onReflectStart={vi.fn()}
      />,
    )

    await waitFor(() => expect(container.querySelector('.channels-layout')).toBeTruthy())
    expect(container.querySelector('header, h1, h2')).toBeNull()
    expect(container.querySelector('.channel-tabs [role="tab"]')).toBeTruthy()
  })

  test('ChannelsPage edits one channel at a time and keeps the write target in reach', async () => {
    const profiles = [
      { id: 'feishu', name: 'Feishu', testable: true, fields: [{ name: 'fs_app_id', label: 'App ID', value: 'cli_a' }] },
      { id: 'telegram', name: 'Telegram', testable: true, fields: [{ name: 'tg_bot_token', label: 'Bot Token', secret: true, has_value: false, value: '' }] },
    ]
    globalThis.fetch = vi.fn(() => Promise.resolve(jsonResponse({ path: '/ga/mykey.py', profiles })))

    const { container } = render(
      <ChannelsPage
        frontendSvcs={[]}
        t={t}
        onStart={vi.fn()}
        onStop={vi.fn()}
        onLogs={vi.fn()}
        onAutostart={vi.fn()}
        onReflectStart={vi.fn()}
      />,
    )

    const feishuEntry = await screen.findByRole('button', { name: /Lark/ })
    expect(feishuEntry.getAttribute('aria-current')).toBe('true')
    expect(screen.getByLabelText('App ID')).toBeTruthy()
    expect(screen.queryByLabelText('Bot Token')).toBeNull()
    expect(container.querySelector('.channel-commit-path code').textContent).toBe('/ga/mykey.py')

    fireEvent.click(screen.getByRole('button', { name: /Telegram/ }))
    expect(screen.getByLabelText('Bot Token')).toBeTruthy()
    expect(screen.queryByLabelText('App ID')).toBeNull()

    fireEvent.change(screen.getByLabelText('Bot Token'), { target: { value: 'token-1' } })
    expect((await screen.findByRole('status')).textContent).toMatch(/1 channel with unsaved changes/i)
  })

  test('ChannelsPage switches accessible task tabs with pointer and keyboard', async () => {
    globalThis.fetch = vi.fn(() => Promise.resolve(jsonResponse({ profiles: [] })))

    const { container } = render(
      <ChannelsPage
        frontendSvcs={[]}
        t={t}
        onStart={vi.fn()}
        onStop={vi.fn()}
        onLogs={vi.fn()}
        onAutostart={vi.fn()}
        onReflectStart={vi.fn()}
      />,
    )

    const configTab = await screen.findByRole('tab', { name: /Channel config/i })
    const servicesTab = screen.getByRole('tab', { name: /Service management/i })
    const configPanel = container.querySelector('#channel-panel-config')
    const servicesPanel = container.querySelector('#channel-panel-services')

    expect(configTab.getAttribute('aria-selected')).toBe('true')
    expect(configTab.tabIndex).toBe(0)
    expect(configPanel.hidden).toBe(false)
    expect(servicesPanel.hidden).toBe(true)

    fireEvent.click(servicesTab)
    expect(servicesTab.getAttribute('aria-selected')).toBe('true')
    expect(servicesTab.tabIndex).toBe(0)
    expect(configPanel.hidden).toBe(true)
    expect(servicesPanel.hidden).toBe(false)

    fireEvent.keyDown(servicesTab, { key: 'ArrowRight' })
    expect(configTab.getAttribute('aria-selected')).toBe('true')
    expect(document.activeElement).toBe(configTab)

    fireEvent.keyDown(configTab, { key: 'End' })
    expect(servicesTab.getAttribute('aria-selected')).toBe('true')
    expect(document.activeElement).toBe(servicesTab)

    fireEvent.keyDown(servicesTab, { key: 'Home' })
    expect(configTab.getAttribute('aria-selected')).toBe('true')
    expect(document.activeElement).toBe(configTab)
  })

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

  test('keeps an external shared Hub visible but not stoppable', () => {
    const onStop = vi.fn()
    render(
      <ChannelServiceTable
        services={[{ ...reflectService, name: 'frontends/hub.py', kind: 'frontend', running: true, shared: true, managed: false, pid: 29812 }]}
        t={t}
        onStart={vi.fn()}
        onStop={onStop}
        onLogs={vi.fn()}
        onAutostart={vi.fn()}
      />,
    )

    expect(screen.getByText(t.service.sharedRunning)).toBeTruthy()
    expect(screen.getByText(t.service.sharedHubNotice)).toBeTruthy()
    const stopButton = screen.getByRole('button', { name: /Stop/i })
    expect(stopButton.disabled).toBe(true)
    expect(stopButton.title).toBe(t.service.sharedStopDisabled)
    fireEvent.click(stopButton)
    expect(onStop).not.toHaveBeenCalled()
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

  test('selects a scheduler model, clears it back to GA default, and locks changes while running', () => {
    const service = { name: 'reflect/scheduler.py', kind: 'reflect', running: false, autostart: false, model_no: 2 }
    const onModel = vi.fn()
    const schedulerT = { ...t, nav: { logs: 'Logs' }, retry: 'Retry', serviceDesc: { scheduler: 'Scheduled task runner' } }
    const llms = [
      { index: 0, provider: 'Provider A', model: 'model-a' },
      { index: 2, provider: 'Provider B', model: 'model-b' },
    ]
    const view = render(<SchedulerServiceRow
      service={service}
      llms={llms}
      t={schedulerT}
      onStart={vi.fn()}
      onStop={vi.fn()}
      onLogs={vi.fn()}
      onAutostart={vi.fn()}
      onModel={onModel}
    />)

    const trigger = screen.getByRole('button', { name: /选择模型，当前 Provider B/ })
    expect(trigger.textContent).toContain('model-b')
    fireEvent.click(trigger)
    fireEvent.click(screen.getByRole('option', { name: '默认' }))
    fireEvent.click(screen.getByRole('option', { name: /GA default model/ }))
    expect(onModel).toHaveBeenCalledWith(service.name, null)

    view.rerender(<SchedulerServiceRow
      service={{ ...service, running: true }}
      llms={llms}
      t={schedulerT}
      onStart={vi.fn()}
      onStop={vi.fn()}
      onLogs={vi.fn()}
      onAutostart={vi.fn()}
      onModel={onModel}
    />)
    expect(screen.getByRole('button', { name: /选择模型/ }).disabled).toBe(true)
    expect(screen.getByRole('button', { name: /选择模型/ }).title).toContain('Stop the scheduler')
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
  initialFailoverGroups = [],
  saveState = {},
  saveAll = vi.fn(async () => true),
  discardDraft = vi.fn(),
}) {
  const persisted = React.useRef([{ ...initialProfile, client_id: 'profile-key' }])
  const persistedGroups = React.useRef(initialFailoverGroups)
  const [profiles, setProfiles] = React.useState(persisted.current)
  const [failoverGroups, setFailoverGroups] = React.useState(initialFailoverGroups)
  // Mirrors the hook: the saved name has to survive a rename so that the
  // change count reports one edit rather than an add plus a removal.
  const patchProfile = (idx, patch) => {
    setProfiles(current => current.map((profile, index) => {
      if (index !== idx) return profile
      const next = { ...profile, ...patch }
      if (patch.var_name !== undefined && next.previous_var_name === undefined && profile.var_name) {
        next.previous_var_name = profile.var_name
      }
      return next
    }))
  }

  return (
    <Models
      t={I18N.zh}
      profiles={profiles}
      setProfiles={setProfiles}
      patchProfile={patchProfile}
      addModelProfiles={vi.fn(() => [])}
      removeModelProfile={vi.fn()}
      importModels={vi.fn()}
      previewModels={vi.fn()}
      discoverModels={discoverModels}
      failoverGroups={failoverGroups}
      setFailoverGroups={setFailoverGroups}
      changes={draftChangeSummary(profiles, persisted.current, failoverGroups, persistedGroups.current)}
      saveState={saveState}
      saveAll={saveAll}
      discardDraft={discardDraft}
      riskCatalog={[]}
      getProfileKey={(idx, profile) => profile?.client_id || `profile-${idx}`}
    />
  )
}

// The provider drawer renders in a body portal, so its fields are read from
// the document rather than the render container.
const openProviderDrawer = () => fireEvent.click(document.querySelector('.model-connection-card'))
const providerNameInput = () => document.querySelector('.model-field--provider input')
const openAddModel = () => fireEvent.click(screen.getByRole('button', { name: /添加模型$/ }))

describe('Models call list', () => {
  test('lists every model as a call slot numbered by --llm-no', () => {
    installBrowserPolyfills()
    render(<ModelsHarness initialProfile={{
      ...validModelProfile,
      models: ['demo-model', 'demo-model-2'],
      model_configs: [{ model: 'demo-model' }, { model: 'demo-model-2' }],
    }} />)

    const slots = [...document.querySelectorAll('.model-call-slot strong')]
    expect(slots.map(slot => slot.textContent)).toEqual(['0', '1'])
    expect(document.querySelector('.model-call-row .model-call-title strong').textContent).toBe('demo-model')
  })

  test('edits a model display name without changing its model ID', () => {
    installBrowserPolyfills()
    render(<ModelsHarness initialProfile={{
      ...validModelProfile,
      model_configs: [{ model: 'demo-model', name: 'Demo Friendly' }],
    }} />)

    fireEvent.click(screen.getByRole('button', { name: /^配置: / }))
    const displayNameInput = screen.getByLabelText('显示名称')
    expect(displayNameInput.value).toBe('Demo Friendly')

    fireEvent.change(displayNameInput, { target: { value: 'Renamed Friendly' } })

    expect(screen.getByLabelText('显示名称').value).toBe('Renamed Friendly')
    expect(document.querySelector('.model-call-title strong').textContent).toBe('Renamed Friendly')
    expect(document.querySelector('.model-call-sub em').textContent).toBe('demo-model')
  })

  test('keeps focus in the provider name while its controlled value changes', () => {
    installBrowserPolyfills()

    render(<ModelsHarness />)
    openProviderDrawer()
    const nameInput = providerNameInput()
    nameInput.focus()
    expect(document.activeElement).toBe(nameInput)

    fireEvent.change(nameInput, { target: { value: 'renamed' } })

    const updatedNameInput = providerNameInput()
    expect(updatedNameInput.value).toBe('renamed')
    expect(document.activeElement).toBe(updatedNameInput)
  })

  test('shows discovery pending then an empty state that can be retried', async () => {
    installBrowserPolyfills()
    let resolveDiscovery
    const discoverModels = vi.fn(() => new Promise(resolve => { resolveDiscovery = resolve }))
    render(<ModelsHarness discoverModels={discoverModels} />)

    openAddModel()
    fireEvent.click(screen.getByRole('button', { name: '从服务商获取' }))
    expect(await screen.findByText(/正在获取模型/)).toBeTruthy()

    resolveDiscovery({ models: [] })
    expect(await screen.findByText(/没有发现新的模型/)).toBeTruthy()
    fireEvent.click(screen.getByRole('button', { name: '从服务商获取' }))
    expect(discoverModels).toHaveBeenCalledTimes(2)
  })

  test('shows discovery failure and retries in place', async () => {
    installBrowserPolyfills()
    const discoverModels = vi.fn()
      .mockRejectedValueOnce(new Error('upstream unavailable'))
      .mockResolvedValueOnce({ models: [] })
    render(<ModelsHarness discoverModels={discoverModels} />)

    openAddModel()
    fireEvent.click(screen.getByRole('button', { name: '从服务商获取' }))
    expect(await screen.findByText('无法获取候选模型')).toBeTruthy()
    expect(screen.getByText('upstream unavailable')).toBeTruthy()

    fireEvent.click(screen.getByRole('button', { name: /重\s*试/ }))
    await waitFor(() => expect(discoverModels).toHaveBeenCalledTimes(2))
    expect(await screen.findByText(/没有发现新的模型/)).toBeTruthy()
  })

  test('appends a discovered candidate to the end of the call list', async () => {
    installBrowserPolyfills()
    const discoverModels = vi.fn(async () => ({ models: ['new-model'] }))
    render(<ModelsHarness discoverModels={discoverModels} />)

    openAddModel()
    fireEvent.click(screen.getByRole('button', { name: '从服务商获取' }))
    fireEvent.click(await screen.findByRole('button', { name: '添加模型 new-model' }))

    await waitFor(() => {
      const titles = [...document.querySelectorAll('.model-call-title strong')]
      expect(titles.map(title => title.textContent)).toEqual(['demo-model', 'new-model'])
    })
  })

  test('shows invalid provider errors and the API key warning in its drawer', () => {
    installBrowserPolyfills()
    render(<ModelsHarness initialProfile={{ ...validModelProfile, var_name: '', apibase: '', apikey: '' }} />)

    expect(screen.getByText(/有服务商存在阻断项/)).toBeTruthy()
    openProviderDrawer()

    expect(screen.getByText('此服务商暂时不能保存')).toBeTruthy()
    expect(screen.getByText('必须填写变量名')).toBeTruthy()
    expect(screen.getByText('必须填写 API Base')).toBeTruthy()
    expect(screen.getByText('保存前请留意')).toBeTruthy()
    expect(screen.getByText(/API Key 为空/)).toBeTruthy()
  })

  test('collects edits into one draft that only the page-level save writes', () => {
    installBrowserPolyfills()
    const saveAll = vi.fn(async () => true)
    render(<ModelsHarness saveAll={saveAll} />)

    expect(screen.getByText('与 mykey.py 一致')).toBeTruthy()
    expect(screen.getByRole('button', { name: '保存到 mykey.py' }).disabled).toBe(true)

    openProviderDrawer()
    fireEvent.change(providerNameInput(), { target: { value: 'renamed' } })

    expect(screen.getByText('1 处未保存改动')).toBeTruthy()
    fireEvent.click(screen.getByRole('button', { name: '保存到 mykey.py' }))
    expect(saveAll).toHaveBeenCalledTimes(1)
  })

  test('reports a failed save at the top of the page and keeps the draft', () => {
    installBrowserPolyfills()
    render(<ModelsHarness saveState={{ status: 'error', error: 'disk is read-only' }} />)

    openProviderDrawer()
    fireEvent.change(providerNameInput(), { target: { value: 'renamed' } })

    expect(screen.getByText('保存失败')).toBeTruthy()
    expect(screen.getByText('disk is read-only')).toBeTruthy()
    expect(screen.getByText('1 处未保存改动')).toBeTruthy()
    expect(providerNameInput().value).toBe('renamed')
  })

  test('keeps failover groups independently collapsed and expands a newly added group', () => {
    installBrowserPolyfills()
    const twoModels = {
      ...validModelProfile,
      models: ['demo-model', 'demo-model-2'],
      model_configs: [{ model: 'demo-model' }, { model: 'demo-model-2' }],
    }
    render(
      <ModelsHarness
        initialProfile={twoModels}
        initialFailoverGroups={[
          {
            var_name: 'mixin_config_primary',
            members: [{ provider_var_name: validModelProfile.var_name, model: validModelProfile.model }],
            max_retries: 10,
            base_delay: 0.5,
          },
          {
            var_name: 'mixin_config_secondary',
            members: [],
            max_retries: 10,
            base_delay: 0.5,
          },
        ]}
      />,
    )

    const readGroups = () => [...document.querySelectorAll('.model-call-row.is-failover')]
    let groups = readGroups()
    let toggles = groups.map(group => group.querySelector('.model-call-toggle'))
    expect(groups).toHaveLength(2)
    expect(toggles.map(toggle => toggle?.getAttribute('aria-expanded'))).toEqual(['false', 'false'])
    expect(groups.every(group => !group.querySelector('.model-row-body'))).toBe(true)

    fireEvent.click(toggles[0])
    groups = readGroups()
    toggles = groups.map(group => group.querySelector('.model-call-toggle'))
    expect(toggles.map(toggle => toggle?.getAttribute('aria-expanded'))).toEqual(['true', 'false'])
    expect(groups[0].querySelector('.model-row-body')).toBeTruthy()
    expect(groups[1].querySelector('.model-row-body')).toBeNull()

    fireEvent.click(toggles[1])
    fireEvent.click(toggles[0])
    groups = readGroups()
    toggles = groups.map(group => group.querySelector('.model-call-toggle'))
    expect(toggles.map(toggle => toggle?.getAttribute('aria-expanded'))).toEqual(['false', 'true'])
    expect(groups[0].querySelector('.model-row-body')).toBeNull()
    expect(groups[1].querySelector('.model-row-body')).toBeTruthy()

    fireEvent.click(screen.getByRole('button', { name: '新建故障转移组' }))
    groups = readGroups()
    toggles = groups.map(group => group.querySelector('.model-call-toggle'))
    expect(groups).toHaveLength(3)
    expect(toggles.map(toggle => toggle?.getAttribute('aria-expanded'))).toEqual(['false', 'true', 'true'])
    expect(groups[2].querySelector('.model-row-body')).toBeTruthy()
  })

  test('groups failover candidates by provider and keeps the full model identity visible', () => {
    installBrowserPolyfills()
    render(
      <ModelsHarness
        failoverGroups={[{
          var_name: 'mixin_config_primary',
          members: [{ provider_var_name: validModelProfile.var_name, model: validModelProfile.model }],
          max_retries: 10,
          base_delay: 0.5,
        }]}
      />,
    )

    fireEvent.click(screen.getByRole('button', { name: '\u6545\u969c\u8f6c\u79fb' }))
    fireEvent.click(document.querySelector('.model-failover-group-toggle'))

    const provider = document.querySelector('.model-failover-cascade-providers button')
    expect(provider?.textContent).toContain('demo')
    expect(document.querySelector('.model-failover-cascade-heading strong')?.textContent).toBe('demo')
    const model = document.querySelector('.model-failover-cascade-model')
    expect(model?.textContent).toContain('demo-model')
    expect(model?.getAttribute('title')).toContain('demo · demo-model · native_oai')
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

    expect(container.querySelector('.oa-usage-time')?.textContent).toContain('1m')
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

  test('normalizes goal start seconds and hides an invalid epoch date', () => {
    const startSeconds = Math.floor(Date.parse('2026-07-17T08:09:10.000Z') / 1000)
    const { container, rerender } = render(
      <GoalStatusCard state={{ status: 'done', start_time: startSeconds, elapsed_seconds: 10 }} />,
    )

    expect(container.querySelector('.oa-goalcard-meta')?.textContent)
      .toContain(new Date(startSeconds * 1000).toLocaleString())

    rerender(<GoalStatusCard state={{ status: 'done', start_time: 1777777, elapsed_seconds: 10 }} />)
    expect(container.querySelector('.oa-goalcard-meta')?.textContent).not.toContain('启动')
  })

  test('keeps each goal card at the tail of its owning assistant output', () => {
    const messages = [
      { id: 'goal-old', role: 'assistant', content: 'Old output', goal_state: { status: 'done', objective: 'Old goal' } },
      { id: 'goal-new', role: 'assistant', content: 'New output', goal_state: { status: 'done', objective: 'New goal' } },
    ]
    const { container } = render(<>{messages.map(message => (
      <ChatMessage key={message.id} message={message} pending={false} onAskReply={vi.fn()} />
    ))}</>)

    const assistants = [...container.querySelectorAll('.oa-message.assistant')]
    expect(assistants).toHaveLength(2)
    expect(assistants[0].querySelector('.oa-goalcard')?.textContent).toContain('Old goal')
    expect(assistants[0].textContent).not.toContain('New goal')
    expect(assistants[1].querySelector('.oa-goalcard')?.textContent).toContain('New goal')
    expect(assistants[1].textContent).not.toContain('Old goal')
    expect(assistants[0].querySelector('.oa-msg-body + .oa-goalcard')).toBeTruthy()
    expect(appStyles).toMatch(
      /\.oa-message\.assistant:has\(> \.oa-goalcard\) > \.oa-goalcard\s*\{[\s\S]*?grid-column:\s*1;[\s\S]*?grid-row:\s*2;/,
    )
  })

  test('shows a recoverable model-service card without exposing raw HTML', () => {
    const onRetry = vi.fn()
    const content = '!!!Error: HTTP 403: <!DOCTYPE html><html><head><title>Attention Required! | Cloudflare</title></head><body>blocked</body></html>'
    const { container } = render(
      <ChatMessage
        message={{ id: 'provider-error', role: 'assistant', content, error: true }}
        pending={false}
        onAskReply={vi.fn()}
        onRetry={onRetry}
      />,
    )

    expect(screen.getByRole('alert', { name: '模型服务错误' }).textContent).toContain('HTTP 403')
    expect(container.textContent).not.toContain('<html')
    const details = screen.getByText('查看技术详情').closest('details')
    expect(details.open).toBe(false)
    fireEvent.click(details.querySelector('summary'))
    expect(details.open).toBe(true)
    expect(details.textContent).toContain('Cloudflare')
    fireEvent.click(screen.getByRole('button', { name: '重新发送' }))
    expect(onRetry).toHaveBeenCalledTimes(1)
  })

  test('keeps partial output visible when the user stops generation', () => {
    render(
      <ChatMessage
        message={{ id: 'stopped', role: 'assistant', content: '已生成的部分内容\n\n[已中止生成]', error: true }}
        pending={false}
        onAskReply={vi.fn()}
      />,
    )

    expect(screen.queryByRole('alert')).toBeNull()
    expect(screen.queryByRole('button', { name: '重新发送' })).toBeNull()
    expect(screen.getByText(/已生成的部分内容/)).toBeTruthy()
    expect(screen.getByText(/已中止生成/)).toBeTruthy()
  })

  test('renders comparison-report markdown without leaking syntax or unsafe HTML', () => {
    const content = [
      '<summary>source differences confirmed</summary>',
      '',
      '## Two legacy CPLD TU comparison report',
      '',
      '### Basic information',
      '| Item | tianchi_101 | server_103 |',
      '|------|-------------|------------|',
      '| **Code size** | 689 lines | 1223 lines |',
      '',
      '---',
      '',
      '#### 1. **Data sources and maintenance**',
      '| Dimension | tianchi_101 | server_103 |',
      '|------|-------------|------------|',
      '| **Data source** | **WebService**<br>dynamic address | **Local Excel**<br/>five xlsx files |',
      '',
      '#### 3. **Update core logic**',
      'Both use `update_cpld_firmware()`; ~~obsolete path~~.',
      '<img src=x onerror="window.__markdownInjected=true">',
    ].join('\n')
    const { container } = render(
      <ChatMessage
        message={{ id: 'markdown-comparison', role: 'assistant', content }}
        pending={false}
        onAskReply={vi.fn()}
      />,
    )

    expect(container.querySelector('.oa-response-summary')?.textContent).toContain('source differences confirmed')
    expect(screen.getByRole('heading', { level: 2, name: 'Two legacy CPLD TU comparison report' })).toBeTruthy()
    expect(screen.getByRole('heading', { level: 3, name: 'Basic information' })).toBeTruthy()
    const firstDetailHeading = container.querySelector('.oa-md h4')
    expect(firstDetailHeading?.textContent).toBe('1. Data sources and maintenance')
    expect(firstDetailHeading?.querySelector('strong')?.textContent).toBe('Data sources and maintenance')
    expect(container.querySelectorAll('.oa-md-table')).toHaveLength(2)
    expect(container.querySelectorAll('.oa-md-table br')).toHaveLength(2)
    expect(container.querySelector('.oa-md hr')).toBeTruthy()
    expect(container.querySelector('.oa-md code')?.textContent).toBe('update_cpld_firmware()')
    expect(container.querySelector('.oa-md del')?.textContent).toBe('obsolete path')
    expect(container.querySelector('.oa-md img')).toBeNull()
    expect(container.querySelector('.oa-md')?.textContent).toContain('<img src=x onerror="window.__markdownInjected=true">')
    expect(container.querySelector('.oa-md')?.textContent).not.toContain('<br>')
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

describe('chat model cascade', () => {
  const groups = [
    { value: 'alpha', label: 'Alpha', models: [{ value: 'a-1', label: 'Alpha One' }] },
    { value: 'beta', label: 'Beta', models: [{ value: 'b-1', label: 'Beta One' }] },
  ]

  test('exposes menu state, resets previews on reopen, and returns focus on Escape', async () => {
    render(<ProviderModelCascade groups={groups} selectedProvider="alpha" value="a-1" onChange={vi.fn()} />)
    const trigger = screen.getByRole('button', { name: '选择模型，当前 Alpha · Alpha One' })

    expect(trigger.getAttribute('aria-expanded')).toBe('false')
    fireEvent.click(trigger)
    expect(trigger.getAttribute('aria-expanded')).toBe('true')
    expect(screen.getByRole('dialog', { name: '\u670d\u52a1\u5546\u548c\u6a21\u578b' }).id).toBe(trigger.getAttribute('aria-controls'))

    fireEvent.mouseEnter(screen.getByRole('option', { name: 'Beta' }))
    expect(screen.getByText('Beta One')).toBeTruthy()
    expect(screen.getByRole('option', { name: 'Beta' }).getAttribute('aria-selected')).toBe('true')

    fireEvent.keyDown(document, { key: 'Escape' })
    expect(screen.queryByRole('dialog', { name: '\u670d\u52a1\u5546\u548c\u6a21\u578b' })).toBeNull()
    await waitFor(() => expect(document.activeElement).toBe(trigger))

    fireEvent.click(trigger)
    const reopenedMenu = screen.getByRole('dialog', { name: '\u670d\u52a1\u5546\u548c\u6a21\u578b' })
    expect(reopenedMenu.textContent).toContain('Alpha One')
    expect(reopenedMenu.textContent).not.toContain('Beta One')
  })

  test('selects a previewed provider model and closes the menu', () => {
    const onChange = vi.fn()
    render(<ProviderModelCascade groups={groups} selectedProvider="alpha" value="a-1" onChange={onChange} />)

    fireEvent.click(screen.getByRole('button', { name: '选择模型，当前 Alpha · Alpha One' }))
    fireEvent.click(screen.getByRole('option', { name: 'Beta' }))
    fireEvent.click(screen.getByRole('option', { name: 'Beta One' }))

    expect(onChange).toHaveBeenCalledWith('b-1')
    expect(screen.queryByRole('dialog', { name: '\u670d\u52a1\u5546\u548c\u6a21\u578b' })).toBeNull()
  })

  test('uses a body portal and click-only provider switching on mobile', () => {
    const originalMatchMedia = window.matchMedia
    Object.defineProperty(window, 'matchMedia', {
      configurable: true,
      writable: true,
      value: vi.fn(query => ({
        matches: query === '(max-width: 560px)',
        media: query,
        addEventListener: vi.fn(),
        removeEventListener: vi.fn(),
      })),
    })
    const onChange = vi.fn()

    try {
      render(<ProviderModelCascade groups={groups} selectedProvider="alpha" value="a-1" onChange={onChange} />)
      fireEvent.click(screen.getByRole('button', { name: '\u9009\u62e9\u6a21\u578b\uff0c\u5f53\u524d Alpha \u00b7 Alpha One' }))

      const dialog = screen.getByRole('dialog', { name: '\u670d\u52a1\u5546\u548c\u6a21\u578b' })
      expect(dialog.closest('.oa-mobile-picker-backdrop')?.parentElement).toBe(document.body)
      expect(document.body.style.overflow).toBe('hidden')
      fireEvent.pointerDown(screen.getByRole('option', { name: 'Beta' }))
      expect(screen.queryByText('Beta One')).toBeNull()
      fireEvent.click(screen.getByRole('option', { name: 'Beta' }))
      fireEvent.click(screen.getByRole('option', { name: 'Beta One' }))

      expect(onChange).toHaveBeenCalledWith('b-1')
      expect(screen.queryByRole('dialog', { name: '\u670d\u52a1\u5546\u548c\u6a21\u578b' })).toBeNull()
      expect(document.body.style.overflow).toBe('')

      fireEvent.click(screen.getByRole('button', { name: '\u9009\u62e9\u6a21\u578b\uff0c\u5f53\u524d Alpha \u00b7 Alpha One' }))
      fireEvent.click(screen.getByRole('button', { name: '\u5173\u95ed\u6a21\u578b\u9009\u62e9' }))
      expect(screen.queryByRole('dialog', { name: '\u670d\u52a1\u5546\u548c\u6a21\u578b' })).toBeNull()
    } finally {
      cleanup()
      Object.defineProperty(window, 'matchMedia', {
        configurable: true,
        writable: true,
        value: originalMatchMedia,
      })
    }
  })

  test('scrolls only the model column when the current model is below its viewport', () => {
    const rectSpy = vi.spyOn(HTMLElement.prototype, 'getBoundingClientRect').mockImplementation(function () {
      if (this.classList?.contains('oa-cascade-models')) return { top: 100, bottom: 200 }
      if (this.getAttribute?.('aria-selected') === 'true' && this.closest('.oa-cascade-models')) return { top: 250, bottom: 280 }
      return { top: 0, bottom: 0 }
    })

    try {
      render(<ProviderModelCascade groups={groups} selectedProvider="alpha" value="a-1" onChange={vi.fn()} />)
      fireEvent.click(screen.getByRole('button', { name: '选择模型，当前 Alpha · Alpha One' }))

      expect(document.querySelector('.oa-cascade-models').scrollTop).toBe(82)
    } finally {
      rectSpy.mockRestore()
    }
  })
})



describe('file workflow confidence', () => {
  const fileT = {
    ...I18N.en,
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
    expect(screen.getByText(/No file loaded/)).toBeTruthy()
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

    expect(screen.getByText('Unsaved changes')).toBeTruthy()
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

describe('usage overview page', () => {
  const payload = {
    totals: { input_tokens: 1200, output_tokens: 345, total_tokens: 1545 },
    session_count: 2,
    sessions_with_usage: 1,
    assistant_replies: 3,
    skipped_sessions: 0,
    models: [{ id: 'gpt-5', name: 'gpt-5', assistant_replies: 3, totals: { input_tokens: 1200, output_tokens: 345, total_tokens: 1545 } }],
    sessions: [{ id: 'session-1', name: 'Alpha', updated_at: 1700000000000, assistant_replies: 3, totals: { input_tokens: 1200, output_tokens: 345, total_tokens: 1545 } }],
    daily: [{ date: new Date().toISOString().slice(0, 10), assistant_replies: 3, totals: { input_tokens: 1200, output_tokens: 345, total_tokens: 1545 } }],
  }

  test('renders aggregate and breakdown data', async () => {
    globalThis.fetch = vi.fn(async () => jsonResponse(payload))
    render(<UsagePage lang="en" />)
    expect((await screen.findAllByText('1,545')).length).toBeGreaterThan(0)
    expect((screen.getAllByText('gpt-5')).length).toBeGreaterThan(0)
    expect(screen.queryByText('Alpha')).toBeNull()
    expect(screen.queryByText('Session details')).toBeNull()
    expect(screen.getByText('Daily activity')).toBeTruthy()
    const heatCells = document.querySelectorAll('.usage-heat-cell')
    expect(heatCells.length).toBeGreaterThanOrEqual(358)
    expect(heatCells.length).toBeLessThanOrEqual(364)
    expect(document.querySelector('.usage-heat-cell:not([data-level="0"])')).toBeTruthy()
  })

  test('renders an explicit empty state', async () => {
    globalThis.fetch = vi.fn(async () => jsonResponse({ ...payload, totals: {}, session_count: 0, sessions_with_usage: 0, assistant_replies: 0, models: [], sessions: [] }))
    render(<UsagePage lang="en" />)
    expect(await screen.findByText('No token usage has been recorded yet.')).toBeTruthy()
  })

  test('recovers from a request error', async () => {
    globalThis.fetch = vi.fn()
      .mockRejectedValueOnce(new Error('network offline'))
      .mockResolvedValueOnce(jsonResponse(payload))
    render(<UsagePage lang="en" />)
    expect((await screen.findByRole('alert')).textContent).toMatch(/network offline/i)
    fireEvent.click(screen.getByRole('button', { name: 'Retry' }))
    expect((await screen.findAllByText('1,545')).length).toBeGreaterThan(0)
    expect(globalThis.fetch).toHaveBeenCalledTimes(2)
  })

  test('queries usage records without exposing message content', async () => {
    const recordPayload = {
      ...payload,
      record_total: 2,
      record_page: 1,
      record_page_size: 20,
      record_total_pages: 1,
      record_providers: ['Local provider'],
      record_models: ['Friendly model'],
      records: [{
        id: 'usage-1', session_id: 'session-1', session_name: 'Session one', provider: 'Local provider', model_id: 'model-real', model_name: 'Friendly model',
        created_at_ms: Date.now(), elapsed_ms: 1250, input_tokens: 40, cached_tokens: 4, output_tokens: 12, total_tokens: 52,
      }],
    }
    const requestedUrls = []
    globalThis.fetch = vi.fn(async url => {
      requestedUrls.push(String(url))
      return jsonResponse(recordPayload)
    })
    render(<UsagePage lang="en" />)
    expect((await screen.findAllByText('Session one')).length).toBeGreaterThan(0)
    fireEvent.change(screen.getByPlaceholderText('Search model, session, or ID'), { target: { value: 'Friendly model' } })
    fireEvent.click(screen.getByRole('button', { name: 'Query' }))
    await waitFor(() => expect(requestedUrls.some(url => url.includes('model=Friendly+model'))).toBe(true))
    expect((screen.getAllByText('Local provider')).length).toBeGreaterThan(0)
    expect((screen.getAllByText('1.3 s')).length).toBeGreaterThan(0)
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
      '/api/ga/git-status': { ok: true, available: true, branch: 'main', commit: 'abc1234', upstream: 'origin/main', upstream_configured: true, latest: true },
    }
    return jsonResponse(payloads[path] ?? {})
  }

  test('keeps the mobile sidebar above its scrim', () => {
    expect(adminMobileStyles).toMatch(/\.app > \.sidebar\s*\{[^}]*z-index:\s*1001\s*!important;/s)
    expect(adminMobileStyles).toMatch(/\.admin-sidebar-scrim\s*\{[^}]*z-index:\s*1000;/s)
  })

  test('navigation exposes the selected route with native keyboard semantics', async () => {
    installBrowserPolyfills()
    globalThis.fetch = vi.fn(async (url) => shellPayload(url))
    render(<App />)
    const files = await screen.findByRole('button', { name: /文件|Files/i })
    const usage = screen.getByRole('button', { name: /用量总览|Usage/i })
    const overview = screen.getByRole('button', { name: /^(总览|Overview)$/i })
    const pageHeader = document.querySelector('.admin-page-header')
    expect(document.querySelectorAll('.admin-page-header')).toHaveLength(1)
    // The header is a label line only: the service status belongs to the shell,
    // which renders one copy for the sidebar and one for the collapsed bar.
    expect(pageHeader?.querySelector('.admin-service-status')).toBeNull()
    const statuses = document.querySelectorAll('.admin-service-status')
    expect(statuses).toHaveLength(2)
    expect(document.querySelector('#admin-sidebar > .admin-service-status')).toBeTruthy()
    expect(document.querySelector('.admin-mobile-bar > .admin-service-status')).toBeTruthy()
    statuses.forEach(status => {
      expect(status.getAttribute('aria-label')).toMatch(/服务状态|Service status/i)
      expect(status.querySelector('.admin-service-health')?.getAttribute('role')).toBe('status')
    })
    expect(pageHeader?.querySelector('h2')?.textContent).toMatch(/总览|Overview/i)
    expect(overview.getAttribute('aria-current')).toBe('page')
    expect(usage.tagName).toBe('BUTTON')
    expect(usage.disabled).toBe(false)
    files.focus()
    expect(document.activeElement).toBe(files)
    expect(files.tagName).toBe('BUTTON')
    fireEvent.click(files)
    expect(files.getAttribute('aria-current')).toBe('page')
    expect(files.disabled).toBe(false)
    expect(document.querySelectorAll('.admin-page-header')).toHaveLength(1)
    expect(document.querySelector('.admin-page-header h2')?.textContent).toMatch(/文件|Files/i)
  })

  test('overview hides duplicate cards and keeps actionable summary cards', async () => {
    installBrowserPolyfills()
    globalThis.fetch = vi.fn(async url => shellPayload(url))
    render(<App />)

    const scheduledCard = await screen.findByRole('button', { name: /定时任务:/ })
    expect(document.querySelectorAll('.overview-stats .stat-link')).toHaveLength(2)
    expect(screen.queryByRole('button', { name: /服务控制:/ })).toBeNull()
    expect(screen.queryByRole('button', { name: /调度提醒:/ })).toBeNull()
    expect(scheduledCard.className).toContain('stat-link')
    fireEvent.click(scheduledCard)
    await waitFor(() => expect(window.location.pathname).toBe('/tasks/scheduled'))
  })

  test('hides an applied update status without repeatedly checking GitHub', async () => {
    installBrowserPolyfills()
    globalThis.fetch = vi.fn(async url => {
      const path = new URL(url, 'http://localhost').pathname
      const payloads = {
        '/api/config': { ga_root: '' },
        '/api/ga/health': { ok: false, error: 'GA root not configured' },
        '/api/autostart/status': { supported: false, enabled: false },
        '/api/version/info': { version: 'v1.0.32' },
        '/api/version/status': {
          id: 'old-update',
          stage: 'done',
          running: false,
          progress: 100,
          applied_version: 'v1.0.32',
          message: 'SHOULD_HIDE_STALE_PROGRESS',
          check: { latest: { tag_name: 'v1.0.32' } },
        },
        '/api/ga/inventory': {},
        '/api/risk/catalog': {},
      }
      if (!(path in payloads)) throw new Error(`unexpected url ${url}`)
      return jsonResponse(payloads[path])
    })

    render(<App />)
    await waitFor(() => expect(globalThis.fetch).toHaveBeenCalledWith('/api/version/status', expect.anything()))
    await waitFor(() => expect(screen.queryByText('SHOULD_HIDE_STALE_PROGRESS')).toBeNull())
    expect(globalThis.fetch.mock.calls.filter(([url]) => String(url).includes('/api/version/check'))).toHaveLength(0)
  })

  test('keeps configured service domains visible when health reports a degraded runtime', async () => {
    installBrowserPolyfills()
    window.history.replaceState({}, '', '/autonomous')
    const services = [
      { name: 'reflect/scheduler.py', kind: 'reflect', running: true, model_no: 6 },
      { name: 'reflect/autonomous.py', kind: 'reflect', running: false, model_no: 6 },
    ]
    globalThis.fetch = vi.fn(async url => {
      const path = new URL(url, 'http://localhost').pathname
      if (path === '/api/config') return jsonResponse({ host: '127.0.0.1', port: 8787, ga_root: 'C:/ga' })
      if (path === '/api/ga/health') return jsonResponse({ ok: false, root: 'C:/ga', errors: ['chat runtime failed'] })
      if (path === '/api/autostart/status') return jsonResponse({ supported: true, enabled: true })
      if (path === '/api/version/info') return jsonResponse({ version: 'dev' })
      if (path === '/api/version/status') return jsonResponse({})
      if (path === '/api/ga/inventory') return jsonResponse({ autonomous_reports: [] })
      if (path === '/api/risk/catalog') return jsonResponse({ items: [] })
      if (path === '/api/services') return jsonResponse({ services })
      if (path === '/api/chat/state') return jsonResponse({ llms: [] })
      if (path === '/api/autonomous/approvals') return jsonResponse({ source_exists: false, items: [], pending: 0 })
      throw new Error(`unexpected url ${url}`)
    })

    render(<App />)

    expect(await screen.findByText('reflect/autonomous.py')).toBeTruthy()
    expect(screen.queryByText('鏈彂鐜拌嚜涓昏繘鍖栨湇鍔?')).toBeNull()
  })

  test('switches the complete overview shell to English without stale Chinese labels', async () => {
    installBrowserPolyfills()
    globalThis.fetch = vi.fn(async (url) => shellPayload(url))
    render(<App />)

    fireEvent.click(await screen.findByRole('button', { name: '常规' }))
    fireEvent.click(await screen.findByRole('button', { name: 'English' }))
    expect(screen.getByRole('button', { name: /Appearance/i })).toBeTruthy()
    fireEvent.click(screen.getByRole('button', { name: /^Overview$/i }))

    expect(await screen.findByText('Version management')).toBeTruthy()
    expect(screen.getByText('Read-only observability')).toBeTruthy()
    expect(screen.getByText('GA source')).toBeTruthy()
    expect(screen.getByRole('button', { name: /Run \/update in chat/i })).toBeTruthy()
    expect(screen.queryByText('只读观测')).toBeNull()
    expect(screen.queryByText('版本管理')).toBeNull()
    expect(screen.queryByText('GA 源代码')).toBeNull()
    expect(document.documentElement.lang).toBe('en')
    expect(window.localStorage.getItem('ga-admin-lang')).toBe('en')
  }, 60000)

  // A GA root without git still has a path worth showing and /update to point at,
  // so only the rows that need git disappear.
  test('keeps the GA source card without git but drops the branch row and check button', async () => {
    installBrowserPolyfills()
    globalThis.fetch = vi.fn(async (url) => {
      const path = new URL(url, 'http://localhost').pathname
      if (path === '/api/ga/git-status') return jsonResponse({ ok: true, available: false, reason: 'git_missing' })
      return shellPayload(url)
    })
    render(<App />)

    fireEvent.click(await screen.findByRole('button', { name: /^(总览|Overview)$/i }))

    expect(await screen.findByText('版本管理')).toBeTruthy()
    expect(screen.getByText('GA 源代码')).toBeTruthy()
    expect(screen.getByText('未检测到 git 命令，无法读取分支与更新状态。')).toBeTruthy()
    expect(screen.getByRole('button', { name: /去对话执行 \/update/i })).toBeTruthy()
    expect(screen.queryByText('分支')).toBeNull()
    expect(screen.queryByRole('button', { name: /检查是否最新/i })).toBeNull()
  })

  test('shows the branch row and check button once git can answer', async () => {
    installBrowserPolyfills()
    globalThis.fetch = vi.fn(async (url) => {
      const path = new URL(url, 'http://localhost').pathname
      if (path === '/api/ga/git-status') {
        return jsonResponse({ ok: true, available: true, root: '/ga', branch: 'main', commit: 'abc1234', upstream_configured: true, latest: true })
      }
      return shellPayload(url)
    })
    render(<App />)

    fireEvent.click(await screen.findByRole('button', { name: /^(总览|Overview)$/i }))

    expect(await screen.findByText('GA 源代码')).toBeTruthy()
    expect(screen.getByText('分支')).toBeTruthy()
    expect(screen.getByText(/main · abc1234/)).toBeTruthy()
    expect(screen.getByRole('button', { name: /检查是否最新/i })).toBeTruthy()
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
    expect(screen.getByRole('button', { name: /重试|Retry/ }).disabled).toBe(false)
  })

  test('service actions stay local to one card and expose failure recovery', async () => {
    installBrowserPolyfills()
    window.history.replaceState({}, '', '/channels')
    mockDialog()
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
    fireEvent.click(await screen.findByRole('tab', { name: /服务管理|Service management/i }))
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
    // Scrolling away from the tail offers a way back instead of a status line.
    expect(document.querySelector('.log-jump')).toBeTruthy()
    fireEvent.click(follow)
    expect(follow.getAttribute('aria-pressed')).toBe('true')
    expect(document.querySelector('.log-jump')).toBeFalsy()
  })

  test('log filtering hides non-matching lines but keeps their tail position', async () => {
    installBrowserPolyfills()
    window.history.replaceState({}, '', '/logs')
    const services = [{ name: 'alpha-worker', kind: 'task', running: true, pid: 42, command: ['agentmain'] }]
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
    fireEvent.click((await screen.findByText('alpha-worker')).closest('button'))
    await waitFor(() => expect(streams).toHaveLength(1))
    streams[0].onopen()
    streams[0].emit('snapshot', { lines: ['boot ok', 'ERROR disk full', 'still going'] })

    await waitFor(() => expect(document.querySelectorAll('.log-line')).toHaveLength(3))
    expect(document.querySelector('.log-line.is-error')).toBeTruthy()

    const filter = screen.getByRole('searchbox')
    fireEvent.change(filter, { target: { value: 'disk' } })
    await waitFor(() => expect(document.querySelectorAll('.log-line')).toHaveLength(1))
    expect(document.querySelector('.log-line-no').textContent).toBe('2')
    expect(document.querySelector('.log-view mark').textContent).toBe('disk')

    fireEvent.change(filter, { target: { value: 'nothing-matches' } })
    await waitFor(() => expect(document.querySelector('.log-output-empty')).toBeTruthy())
  })
})

describe('scheduled task execution history', () => {
  const tasks = [
    {
      id: 'alpha',
      enabled: true,
      schedule: '09:00',
      repeat: 'daily',
      prompt: 'alpha prompt',
      recent_reports: [
        { name: 'alpha-latest.md', path: 'sche_tasks/done/alpha-latest.md', mod_time: '2026-08-28T09:00:00Z' },
        { name: 'alpha-previous.md', path: 'sche_tasks/done/alpha-previous.md', mod_time: '2026-08-27T09:00:00Z' },
      ],
    },
    {
      id: 'beta',
      enabled: true,
      schedule: '10:00',
      repeat: 'daily',
      prompt: 'beta prompt',
      recent_reports: [],
    },
  ]

  test('switches task history, renders an empty state, previews in place, and keeps reports navigation', async () => {
    installBrowserPolyfills()
    window.history.replaceState({}, '', '/tasks/scheduled')
    const artifactCalls = []
    let holdArtifact = false
    let releaseArtifact
    let failedTaskId = ''
    globalThis.fetch = vi.fn(async url => {
      const requestURL = new URL(url, 'http://localhost')
      const path = requestURL.pathname
      if (path === '/api/config') return jsonResponse({ host: '127.0.0.1', port: 8787, ga_root: 'C:/ga' })
      if (path === '/api/ga/health') return jsonResponse({ ok: true, root: 'C:/ga' })
      if (path === '/api/autostart/status') return jsonResponse({ supported: true, enabled: false })
      if (path === '/api/version/info') return jsonResponse({ version: 'dev' })
      if (path === '/api/version/status') return jsonResponse({})
      if (path === '/api/ga/inventory') return jsonResponse({})
      if (path === '/api/risk/catalog') return jsonResponse({ items: [] })
      if (path === '/api/services') return jsonResponse({ services: [{ name: 'reflect/scheduler.py', kind: 'reflect', running: false }] })
      if (path === '/api/ga/git-status') return jsonResponse({ ok: true, available: false })
      if (path === '/api/schedule/tasks') return jsonResponse({ enabled: 2, tasks })
      if (path === '/api/schedule/task') {
        const id = requestURL.searchParams.get('id')
        if (id === failedTaskId) return { ...jsonResponse({ error: 'task unavailable' }), ok: false, status: 500, statusText: 'Internal Server Error' }
        const task = tasks.find(item => item.id === id)
        return jsonResponse({ id, raw: JSON.stringify(task || {}) })
      }
      if (path === '/api/schedule/artifact') {
        artifactCalls.push(requestURL.searchParams.get('path'))
        if (holdArtifact) return new Promise(resolve => { releaseArtifact = () => resolve(jsonResponse({ content: '# stale scheduled preview' })) })
        return jsonResponse({ content: '# Alpha execution report\n\nThe report was rendered in place.' })
      }
      if (path === '/api/goals/list') return jsonResponse({ goals: [] })
      if (path === '/api/autonomous/approvals') return jsonResponse({ items: [] })
      return jsonResponse({})
    })

    render(<App />)
    await screen.findByRole('option', { name: /alpha/ }, { timeout: 10000 })
    expect(document.querySelector('.scheduled-task-workbench')).toBeTruthy()
    expect(document.querySelector('.scheduled-task-detail-empty')).toBeTruthy()
    const workbench = document.querySelector('.scheduled-task-workbench')
    await waitFor(() => expect(document.querySelector('.schedule-service-panel')).toBeTruthy())
    const servicePanel = document.querySelector('.schedule-service-panel')
    expect(workbench).toBeTruthy()
    expect(servicePanel).toBeTruthy()
    expect(workbench.compareDocumentPosition(servicePanel) & Node.DOCUMENT_POSITION_FOLLOWING).toBeTruthy()
    await waitFor(() => expect(within(servicePanel).getByRole('button', { name: /启动|Start/i })).toBeTruthy())

    const cards = () => [...document.querySelectorAll('.scheduled-task-row')]
    fireEvent.click(cards()[0])
    await screen.findByText('alpha-latest.md', {}, { timeout: 10000 })
    expect(document.querySelector('.scheduled-task-history-card')?.textContent).toContain('alpha')
    failedTaskId = 'beta'
    fireEvent.click(cards()[1])
    await waitFor(() => expect(document.querySelector('.scheduled-task-history-card')?.textContent).toContain('alpha'))
    expect(document.querySelector('.json-editor, .schedule-form-editor')).toBeTruthy()
    failedTaskId = ''

    fireEvent.click(cards()[1])
    await waitFor(() => expect(document.querySelector('.scheduled-task-history-card')?.textContent).toContain('beta'))
    expect(screen.getByText(/No reports|暂无执行记录/i)).toBeTruthy()
    expect(screen.queryByText('alpha-latest.md')).toBeNull()

    fireEvent.click(cards()[0])
    await screen.findByText('alpha-latest.md', {}, { timeout: 10000 })
    fireEvent.click(screen.getByRole('button', { name: /alpha-latest\.md/i }))
    await screen.findByText('Alpha execution report')
    expect(artifactCalls).toEqual(['sche_tasks/done/alpha-latest.md'])
    expect(window.location.pathname).toBe('/tasks/scheduled')
    expect(document.querySelector('.task-subtabs button.active')?.textContent).toMatch(/Scheduled tasks|定时任务/i)

    fireEvent.click(cards()[1])
    await waitFor(() => expect(document.querySelector('.scheduled-task-history-card')?.textContent).toContain('beta'))
    expect(screen.queryByText('Alpha execution report')).toBeNull()
    expect(screen.queryByText('alpha-latest.md')).toBeNull()

    fireEvent.click(cards()[0])
    await screen.findByText('alpha-latest.md', {}, { timeout: 10000 })
    fireEvent.click(document.querySelector('.scheduled-task-detail .task-reports'))
    await waitFor(() => expect(window.location.pathname).toBe('/tasks/reports'))
    expect(document.querySelector('.schedule-report-tree')).toBeTruthy()

    fireEvent.click(within(document.querySelector('.task-subtabs')).getByRole('button', { name: '定时任务' }))
    await waitFor(() => expect(window.location.pathname).toBe('/tasks/scheduled'))
    await screen.findByText('alpha-latest.md', {}, { timeout: 10000 })
    holdArtifact = true
    fireEvent.click(screen.getByRole('button', { name: /alpha-latest\.md/i }))
    await waitFor(() => expect(releaseArtifact).toEqual(expect.any(Function)))
    fireEvent.click(document.querySelector('.scheduled-task-detail .task-reports'))
    await waitFor(() => expect(window.location.pathname).toBe('/tasks/reports'))
    expect(document.querySelector('.app')?.getAttribute('aria-busy')).not.toBe('true')
    expect(document.querySelector('.task-subtabs button')?.disabled).toBe(false)
    releaseArtifact()
    await waitFor(() => expect(screen.queryByText('stale scheduled preview')).toBeNull())
    expect(document.querySelector('.schedule-report-tree')).toBeTruthy()

    fireEvent.click(within(document.querySelector('.task-subtabs')).getByRole('button', { name: '定时任务' }))
    await waitFor(() => expect(window.location.pathname).toBe('/tasks/scheduled'))
    expect(document.querySelector('.task-run')?.disabled).toBe(false)
    await screen.findByText('alpha-latest.md', {}, { timeout: 10000 })
    fireEvent.click(screen.getByRole('button', { name: /alpha-latest\.md/i }))
    await waitFor(() => expect(releaseArtifact).toEqual(expect.any(Function)))
    fireEvent.click(screen.getByRole('button', { name: '总览' }))
    await waitFor(() => expect(window.location.pathname).toBe('/overview'))
    expect(document.querySelector('.app')?.getAttribute('aria-busy')).not.toBe('true')
    releaseArtifact()
    await waitFor(() => expect(screen.queryByText('stale scheduled preview')).toBeNull())
    expect(window.location.pathname).toBe('/overview')
    fireEvent.click(within(document.querySelector('nav[aria-label="主导航"]')).getByRole('button', { name: '定时任务' }))
    await waitFor(() => expect(window.location.pathname).toBe('/tasks/scheduled'))
    expect(document.querySelector('.task-run')?.disabled).toBe(false)
  })

  test('clears the old preview before selecting a newly created task', async () => {
    installBrowserPolyfills()
    mockDialog(true)
    window.history.replaceState({}, '', '/tasks/scheduled')
    let currentTasks = [...tasks]
    let createRequest = null
    globalThis.fetch = vi.fn(async (url, options = {}) => {
      const requestURL = new URL(url, 'http://localhost')
      const path = requestURL.pathname
      if (path === '/api/config') return jsonResponse({ host: '127.0.0.1', port: 8787, ga_root: 'C:/ga' })
      if (path === '/api/ga/health') return jsonResponse({ ok: true, root: 'C:/ga' })
      if (path === '/api/autostart/status') return jsonResponse({ supported: true, enabled: false })
      if (path === '/api/version/info') return jsonResponse({ version: 'dev' })
      if (path === '/api/version/status') return jsonResponse({})
      if (path === '/api/ga/inventory') return jsonResponse({})
      if (path === '/api/risk/catalog') return jsonResponse({ items: [] })
      if (path === '/api/services') return jsonResponse({ services: [{ name: 'reflect/scheduler.py', kind: 'reflect', running: false }] })
      if (path === '/api/ga/git-status') return jsonResponse({ ok: true, available: false })
      if (path === '/api/schedule/tasks') return jsonResponse({ enabled: currentTasks.filter(task => task.enabled).length, tasks: currentTasks })
      if (path === '/api/schedule/task') {
        const id = requestURL.searchParams.get('id')
        const task = currentTasks.find(item => item.id === id)
        return jsonResponse({ id, raw: JSON.stringify(task || {}) })
      }
      if (path === '/api/schedule/artifact') return jsonResponse({ content: '# Alpha execution report' })
      if (path === '/api/schedule/create') {
        createRequest = JSON.parse(options.body)
        const created = { id: 'created-task', enabled: false, schedule: '', repeat: 'manual', prompt: 'created prompt', recent_reports: [] }
        currentTasks = [...currentTasks, created]
        return jsonResponse({ task: { ...created, raw: JSON.stringify(created) } })
      }
      if (path === '/api/goals/list') return jsonResponse({ goals: [] })
      if (path === '/api/autonomous/approvals') return jsonResponse({ items: [] })
      return jsonResponse({})
    })

    render(<App />)
    await screen.findByRole('option', { name: /alpha/ }, { timeout: 10000 })
    fireEvent.click(screen.getByRole('option', { name: /alpha/ }))
    await screen.findByRole('button', { name: /alpha-latest\.md/i }, { timeout: 10000 })
    fireEvent.click(screen.getByRole('button', { name: /alpha-latest\.md/i }))
    await screen.findByText('Alpha execution report')
    expect(document.querySelector('.schedule-task-history-preview')).toBeTruthy()

    const toolbar = document.querySelector('.scheduled-workbench-toolbar')
    fireEvent.click(within(toolbar).getByRole('button', { name: '创建' }))
    const disclosure = document.querySelector('.scheduled-task-create-disclosure')
    fireEvent.change(within(disclosure).getByRole('textbox'), { target: { value: 'created-task' } })
    fireEvent.click(within(disclosure).getByRole('button', { name: '创建' }))

    await waitFor(() => expect(createRequest?.task).toBeTruthy())
    await waitFor(() => expect(document.querySelector('.scheduled-task-detail h3')?.textContent).toBe('created-task'))
    expect(screen.getByText(/暂无执行记录/)).toBeTruthy()
    expect(screen.queryByText('Alpha execution report')).toBeNull()
    expect(screen.queryByText('alpha-latest.md')).toBeNull()
    expect(document.querySelector('.schedule-task-history-preview')).toBeNull()
    expect(window.location.pathname).toBe('/tasks/scheduled')
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
  t: I18N.zh,
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
  }, 30000)

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
  }, 30000)

  test('model order uses the configured provider display name', () => {
    render(<Models {...modelProps({ persistedProfiles: [modelProfile], onSaveModelOrder: vi.fn(async () => true) })} />)

    fireEvent.click(screen.getByRole('button', { name: '模型顺序' }))

    expect(screen.getByText('服务商名称：主模型')).toBeTruthy()
    expect(screen.queryByText('服务商名称：1')).toBeNull()
  }, 30000)

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
  }, 30000)
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
  }, 30000)

  test('does not save when the provider returns an empty model list', async () => {
    const props = modelProps({ probeModels: vi.fn(async () => ({ results: [] })) })
    render(<Models {...props} />)

    fireEvent.click(screen.getByRole('button', { name: '检测当前服务商' }))
    expect(await screen.findByText('检测失败，未修改模型状态')).toBeTruthy()
    expect(props.saveModelProfile).not.toHaveBeenCalled()
  }, 30000)
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
  }, 30000)

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
  test('compact view reveals only the final assistant result during hover', async () => {
    installBrowserPolyfills()
    Element.prototype.scrollIntoView = vi.fn()
    window.localStorage.setItem('ga-admin-chat-privacy-mode', 'true')
    const sessions = [{ id:'result-session', title:'SECRET_RESULT_TITLE', count:2, updated_at:'2026-08-18T10:00:00Z' }]
    const assistant = [
      'LLM Running (Turn 1)', '<summary>SECRET_EXECUTION_SUMMARY</summary>', 'SECRET_EXECUTION_LOG',
      '```', '[Info] Final response to user.', '```', 'FINAL_RESULT_ONLY',
    ].join('\n')
    globalThis.fetch = vi.fn(async url => {
      const path = String(url)
      if (path === '/api/config') return jsonResponse({ slash_commands:[] })
      if (path === '/api/slash-commands') return jsonResponse({ commands:[] })
      if (path === '/api/chat/sessions') return jsonResponse({ sessions })
      if (path === '/api/chat/session/result-session') return jsonResponse({ ...sessions[0], messages:[{ id:'u-result', role:'user', content:'SECRET_QUESTION' }, { id:'a-result', role:'assistant', content:assistant }], raw_history:[], history_info:[], settings:{ llm_no:0, tools_mode:'official' } })
      if (path === '/api/chat/state/result-session') return jsonResponse({ llms:[], settings:{ llm_no:0, tools_mode:'official' }, loop:{ enabled:false } })
      throw new Error(`unexpected url ${url}`)
    })

    render(<ChatApp />)
    const curtain = await screen.findByRole('region', { name:'任务状态' }, { timeout:10000 })
    await waitFor(() => expect(curtain.textContent).toContain('任务已完成'))
    expect(document.body.innerHTML).not.toContain('FINAL_RESULT_ONLY')
    expect(document.body.innerHTML).not.toContain('SECRET_EXECUTION_LOG')

    fireEvent.pointerEnter(curtain, { pointerType:'mouse' })
    await waitFor(() => expect(screen.getByText('FINAL_RESULT_ONLY')).toBeTruthy())
    expect(document.body.innerHTML).not.toContain('SECRET_EXECUTION_LOG')
    expect(document.body.innerHTML).not.toContain('SECRET_QUESTION')
    fireEvent.pointerLeave(curtain, { pointerType:'mouse' })
    expect(document.body.innerHTML).not.toContain('FINAL_RESULT_ONLY')
  }, 30000)

  test('privacy mode hides loaded transcript data and restores it only after explicit disable', async () => {
    installBrowserPolyfills()
    Element.prototype.scrollIntoView = vi.fn()
    window.localStorage.setItem('ga-admin-chat-privacy-mode', 'true')
    const sessions = [{ id:'private-session', title:'SECRET_SESSION_TITLE', count:2, updated_at:'2026-08-18T10:00:00Z' }]
    const messages = [
      { id:'u-private', role:'user', content:'SECRET_USER_MESSAGE D:/private/input.txt', created_at:1770000000 },
      { id:'a-private', role:'assistant', content:'SECRET_ASSISTANT_REPLY [FILE:D:/private/output.txt]', created_at:1770000010, elapsed_ms:12000, usage:{ input_tokens:1200, output_tokens:320 } },
    ]
    globalThis.fetch = vi.fn(async url => {
      const path = String(url)
      if (path === '/api/config') return jsonResponse({ slash_commands:[{ cmd:'/secret', desc:'SECRET_COMMAND_DESCRIPTION', content:'SECRET_COMMAND_CONTENT' }] })
      if (path === '/api/slash-commands') return jsonResponse({ commands:[] })
      if (path === '/api/chat/sessions') return jsonResponse({ sessions })
      if (path === '/api/chat/session/private-session') return jsonResponse({ ...sessions[0], messages, raw_history:[{ content:'SECRET_CONTEXT' }], history_info:[], settings:{ llm_no:0, tools_mode:'official' } })
      if (path === '/api/chat/state/private-session') return jsonResponse({ llms:[], settings:{ llm_no:0, tools_mode:'official' }, loop:{ enabled:true, status:'running', round:2, max_rounds:5 }, backend:{ diagnosis:{ code:'python_error', detail:'SECRET_DIAGNOSIS D:/private/python.exe' } } })
      throw new Error(`unexpected url ${url}`)
    })

    render(<ChatApp />)
    const curtain = await screen.findByRole('region', { name:'任务状态' }, { timeout:10000 })
    await waitFor(() => expect(curtain.textContent).toContain('2 条'))
    expect(document.querySelector('.oa-chat-stats')).toBeNull()
    expect(document.body.textContent).not.toContain('首 token 平均')
    expect(document.body.textContent).not.toContain('缓存命中')
    expect(document.body.innerHTML).not.toContain('SECRET_SESSION_TITLE')
    expect(document.body.innerHTML).not.toContain('SECRET_USER_MESSAGE')
    expect(document.body.innerHTML).not.toContain('SECRET_ASSISTANT_REPLY')
    expect(document.body.innerHTML).not.toContain('SECRET_CONTEXT')
    expect(document.body.innerHTML).not.toContain('SECRET_DIAGNOSIS')
    expect(document.body.innerHTML).not.toContain('SECRET_COMMAND_DESCRIPTION')
    expect(document.body.innerHTML).not.toContain('D:/private')
    expect(document.querySelector('.oa-title b')?.textContent).toBe('会话 01')
    fireEvent.keyDown(window, { key:'k', ctrlKey:true })
    expect(screen.queryByRole('dialog', { name:'搜索会话' })).toBeNull()
    fireEvent.change(screen.getByPlaceholderText('向 GenericAgent 发送消息，可选择/粘贴/拖拽任意文件…'), { target:{ value:'/' } })
    expect(document.body.innerHTML).not.toContain('SECRET_COMMAND_DESCRIPTION')
    const privateComposerTools = [...document.querySelectorAll('.oa-composer-bar .oa-attach-btn')]
    expect(privateComposerTools.find(button => button.textContent === '命令')?.disabled).toBe(true)
    expect(privateComposerTools.find(button => button.textContent === '系统提示')?.disabled).toBe(true)

    fireEvent.click(screen.getByRole('switch', { name:/精简显示/ }))
    await waitFor(() => expect(screen.getByText('SECRET_USER_MESSAGE D:/private/input.txt')).toBeTruthy())
    expect(document.querySelector('.oa-chat-stats')).toBeNull()
    expect(document.body.textContent).not.toContain('工具调用 0.0s')
    expect(document.body.textContent).not.toContain('输入 1.2K · 输出 320')
    expect(document.querySelector('.oa-title b')?.textContent).toBe('SECRET_SESSION_TITLE')
    expect(document.body.innerHTML).toContain('SECRET_DIAGNOSIS')
  }, 30000)

  test('shows one semantic copy of each tool in the mobile overflow menu', async () => {
    installBrowserPolyfills()
    Element.prototype.scrollIntoView = vi.fn()
    globalThis.fetch = vi.fn(async url => {
      const path = String(url)
      if (path === '/api/config') return jsonResponse({ slash_commands:[] })
      if (path === '/api/slash-commands') return jsonResponse({ commands:[] })
      if (path === '/api/chat/sessions') return jsonResponse({ sessions:[] })
      throw new Error(`unexpected url ${url}`)
    })

    render(<ChatApp />)
    const trigger = await screen.findByRole('button', { name:'打开聊天工具' })
    fireEvent.click(trigger)
    const dialog = screen.getByRole('dialog', { name:'聊天工具' })
    expect(within(dialog).getAllByRole('button', { name:/上下文/ })).toHaveLength(1)
    expect(within(dialog).getAllByRole('button', { name:/世界线/ })).toHaveLength(1)
    expect(within(dialog).getAllByRole('button', { name:/外观/ })).toHaveLength(1)
    expect(within(dialog).getAllByRole('group', { name:'界面缩放' })).toHaveLength(1)

    fireEvent.keyDown(document, { key:'Escape' })
    await waitFor(() => expect(screen.queryByRole('dialog', { name:'聊天工具' })).toBeNull())
    expect(document.activeElement).toBe(trigger)
  })

  test('returns focus to the mobile sidebar trigger after Escape closes the drawer', async () => {
    Object.defineProperty(window, 'matchMedia', {
      writable: true,
      value: vi.fn().mockImplementation(query => ({
        matches: /max-width:\s*(?:900|560)px/.test(query) || /prefers-reduced-motion/.test(query),
        media: query,
        addListener: vi.fn(), removeListener: vi.fn(), addEventListener: vi.fn(), removeEventListener: vi.fn(), dispatchEvent: vi.fn(),
      })),
    })
    Element.prototype.scrollIntoView = vi.fn()
    globalThis.fetch = vi.fn(async url => {
      const path = String(url)
      if (path === '/api/config') return jsonResponse({ slash_commands:[] })
      if (path === '/api/slash-commands') return jsonResponse({ commands:[] })
      if (path === '/api/chat/sessions') return jsonResponse({ sessions:[] })
      throw new Error(`unexpected url ${url}`)
    })

    render(<ChatApp />)
    const trigger = await screen.findByRole('button', { name:'展开侧栏' })
    fireEvent.click(trigger)
    fireEvent.keyDown(document, { key:'Escape' })

    await waitFor(() => expect(document.activeElement).toBe(screen.getByRole('button', { name:'展开侧栏' })))
    expect(document.querySelector('.oa-sidebar')?.classList.contains('collapsed')).toBe(true)
  })

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
    expect(document.querySelectorAll('.oa-sidebar .oa-session-row')).toHaveLength(sessions.length)
    // The closed mobile drawer stays mounted for animation, but its controls must
    // not remain in the accessibility tree behind the visible chat toolbar.
    expect(screen.getAllByRole('button', { name:'返回管理台' })).toHaveLength(1)
    expect(document.querySelector('.oa-admin-back-trigger')?.textContent).toBe('管理台')
    expect(document.querySelector('.oa-topbar-actions .oa-context-btn .oa-context-label')?.textContent).toBe('上下文')
    expect(document.querySelector('.oa-topbar-actions .oa-worldline-btn .oa-context-label')?.textContent).toBe('世界线')
    expect(screen.getByText('历史会话')).toBeTruthy()
    expect(screen.queryByText('最近对话')).toBeNull()
    fireEvent.click(screen.getByRole('button', { name:'展开侧栏' }))
    const second = screen.getByRole('button', { name:/Second chat/ })
    fireEvent.click(second)

    await waitFor(() => expect(document.querySelector('.oa-title b')?.textContent).toBe('Second chat'))
    expect(document.querySelector('.oa-sidebar')?.classList.contains('collapsed')).toBe(true)
    expect(screen.queryByRole('button', { name:'关闭侧栏' })).toBeNull()
    expect(globalThis.fetch.mock.calls.filter(([url]) => String(url) === '/api/chat/session/two')).toHaveLength(1)
  }, 30000)
})

describe('chat project management', () => {
  test('clears the rename disclosure after a successful project rename', async () => {
    installStableChatPolyfills()
    let currentProject = 'Alpha'
    const sessions = [{ id:'project-session', title:'Project chat', project_mode:'Alpha', workspace:'temp/projects/Alpha', count:1, updated_at:'2026-08-18T10:00:00Z' }]
    globalThis.fetch = vi.fn(async (url, options = {}) => {
      const path = String(url).split('?')[0]
      if (path === '/api/config') return jsonResponse({ slash_commands:[] })
      if (path === '/api/slash-commands') return jsonResponse({ commands:[] })
      if (path === '/api/extra-system-prompt-presets') return jsonResponse({ presets:[] })
      if (path === '/api/instances') return jsonResponse({ instances:[] })
      if (path === '/api/chat/sessions') return jsonResponse({ sessions: sessions.map(session => ({ ...session, project_mode:currentProject })), projects:[currentProject], pinned_projects:[currentProject] })
      if (path === '/api/chat/session/project-session') return jsonResponse({ ...sessions[0], project_mode:currentProject, messages:[], raw_history:[], history_info:[], settings:{ llm_no:0, tools_mode:'official' } })
      if (path === '/api/chat/state/project-session') return jsonResponse({ llms:[], settings:{ llm_no:0, tools_mode:'official' }, loop:{ enabled:false } })
      if (path === '/api/chat/projects/rename' && options.method === 'PATCH') {
        expect(options.headers?.['X-GA-Confirm']).toBe('dangerous')
        currentProject = JSON.parse(options.body).new_name
        return jsonResponse({ ok:true, old_name:'Alpha', name:currentProject, projects:[currentProject], pinned_projects:[currentProject] })
      }
      throw new Error(`unexpected url ${url}`)
    })
    mockDialog(true)
    render(<ChatApp />)

    await waitFor(() => expect(document.querySelector('.oa-title b')?.textContent).toBe('Project chat'))
    fireEvent.click(await screen.findByRole('tab', { name:'项目' }))
    fireEvent.click(await screen.findByRole('button', { name:'管理' }))
    const dialog = await screen.findByRole('dialog', { name:'管理项目' })
    fireEvent.click(within(dialog).getByRole('button', { name:'编辑项目 Alpha' }))
    const input = within(dialog).getByRole('textbox', { name:'重命名项目 Alpha' })
    fireEvent.change(input, { target:{ value:'Beta' } })
    fireEvent.click(within(dialog).getByRole('button', { name:'保存' }))

    await waitFor(() => expect(screen.getByRole('status').textContent).toContain('项目已重命名为 Beta'))
    expect(screen.queryByRole('textbox', { name:'重命名项目 Alpha' })).toBeNull()
    expect(within(screen.getByRole('dialog', { name:'管理项目' })).getByRole('button', { name:'编辑项目 Beta' })).toBeTruthy()
    expect(within(screen.getByRole('dialog', { name:'管理项目' })).getByText(/已置顶/)).toBeTruthy()
    expect(within(screen.getByRole('dialog', { name:'管理项目' })).queryByRole('alert')).toBeNull()
  }, 30000)

  test('shows project validation errors inside the manager and clears them on cancel', async () => {
    installStableChatPolyfills()
    const session = { id:'project-session', title:'Project chat', project_mode:'Alpha', workspace:'temp/projects/Alpha', count:1, updated_at:'2026-08-18T10:00:00Z' }
    globalThis.fetch = vi.fn(async (url, options = {}) => {
      const path = String(url).split('?')[0]
      if (path === '/api/config') return jsonResponse({ slash_commands:[] })
      if (path === '/api/slash-commands') return jsonResponse({ commands:[] })
      if (path === '/api/extra-system-prompt-presets') return jsonResponse({ presets:[] })
      if (path === '/api/instances') return jsonResponse({ instances:[] })
      if (path === '/api/chat/sessions') return jsonResponse({ sessions:[session], projects:['Alpha','Beta'], pinned_projects:[] })
      if (path === '/api/chat/session/project-session') return jsonResponse({ ...session, messages:[], raw_history:[], history_info:[], settings:{ llm_no:0, tools_mode:'official' } })
      if (path === '/api/chat/state/project-session') return jsonResponse({ llms:[], settings:{ llm_no:0, tools_mode:'official' }, loop:{ enabled:false } })
      if (path === '/api/chat/worldline/project-session') return jsonResponse({ schema_version:1, nodes:[], current_path:[] })
      if (path === '/api/chat/projects/rename' && options.method === 'PATCH') throw new Error('unexpected validation request')
      throw new Error(`unexpected url ${url}`)
    })
    mockDialog(true)
    render(<ChatApp />)

    await waitFor(() => expect(document.querySelector('.oa-title b')?.textContent).toBe('Project chat'))
    fireEvent.click(await screen.findByRole('tab', { name:'项目' }))
    fireEvent.click(await screen.findByRole('button', { name:'管理' }))
    const dialog = await screen.findByRole('dialog', { name:'管理项目' })
    fireEvent.click(within(dialog).getByRole('button', { name:'编辑项目 Alpha' }))
    const input = within(dialog).getByRole('textbox', { name:'重命名项目 Alpha' })

    fireEvent.change(input, { target:{ value:'bad/name' } })
    fireEvent.click(within(dialog).getByRole('button', { name:'保存' }))
    await waitFor(() => expect(within(dialog).getByRole('alert').textContent).toContain('项目名不能包含'))
    fireEvent.change(input, { target:{ value:'Alpha' } })
    fireEvent.click(within(dialog).getByRole('button', { name:'保存' }))
    await waitFor(() => expect(within(dialog).getByRole('alert').textContent).toContain('项目名称没有变化'))
    fireEvent.change(input, { target:{ value:'Beta' } })
    fireEvent.click(within(dialog).getByRole('button', { name:'保存' }))
    await waitFor(() => expect(within(dialog).getByRole('alert').textContent).toContain('项目 Beta 已存在'))

    fireEvent.click(within(dialog).getByRole('button', { name:'取消' }))
    await waitFor(() => expect(within(dialog).queryByRole('alert')).toBeNull())
    expect(within(dialog).queryByRole('textbox', { name:'重命名项目 Alpha' })).toBeNull()
  }, 30000)

  test('keeps a project API failure visible in the manager until canceled', async () => {
    installStableChatPolyfills()
    const session = { id:'project-session', title:'Project chat', project_mode:'Alpha', workspace:'temp/projects/Alpha', count:1, updated_at:'2026-08-18T10:00:00Z' }
    const apiError = '项目正在运行，暂时无法重命名。'
    globalThis.fetch = vi.fn(async (url, options = {}) => {
      const path = String(url).split('?')[0]
      if (path === '/api/config') return jsonResponse({ slash_commands:[] })
      if (path === '/api/slash-commands') return jsonResponse({ commands:[] })
      if (path === '/api/extra-system-prompt-presets') return jsonResponse({ presets:[] })
      if (path === '/api/instances') return jsonResponse({ instances:[] })
      if (path === '/api/chat/sessions') return jsonResponse({ sessions:[session], projects:['Alpha'], pinned_projects:[] })
      if (path === '/api/chat/session/project-session') return jsonResponse({ ...session, messages:[], raw_history:[], history_info:[], settings:{ llm_no:0, tools_mode:'official' } })
      if (path === '/api/chat/state/project-session') return jsonResponse({ llms:[], settings:{ llm_no:0, tools_mode:'official' }, loop:{ enabled:false } })
      if (path === '/api/chat/worldline/project-session') return jsonResponse({ schema_version:1, nodes:[], current_path:[] })
      if (path === '/api/chat/projects/rename' && options.method === 'PATCH') {
        expect(options.headers?.['X-GA-Confirm']).toBe('dangerous')
        return { ...jsonResponse({ error:apiError }), ok:false, status:409, statusText:'Conflict' }
      }
      throw new Error(`unexpected url ${url}`)
    })
    mockDialog(true)
    render(<ChatApp />)

    await waitFor(() => expect(document.querySelector('.oa-title b')?.textContent).toBe('Project chat'))
    fireEvent.click(await screen.findByRole('tab', { name:'项目' }))
    fireEvent.click(await screen.findByRole('button', { name:'管理' }))
    const dialog = await screen.findByRole('dialog', { name:'管理项目' })
    fireEvent.click(within(dialog).getByRole('button', { name:'编辑项目 Alpha' }))
    const input = within(dialog).getByRole('textbox', { name:'重命名项目 Alpha' })
    fireEvent.change(input, { target:{ value:'Beta' } })
    fireEvent.click(within(dialog).getByRole('button', { name:'保存' }))

    const alert = await within(dialog).findByRole('alert')
    expect(alert.textContent).toBe(apiError)
    expect(alert.closest('.oa-project-manager-modal')).toBe(dialog)
    expect(document.querySelector('.oa-chat-feedback')).toBeNull()
    fireEvent.click(within(dialog).getByRole('button', { name:'取消' }))
    await waitFor(() => expect(within(dialog).queryByRole('alert')).toBeNull())
    expect(within(dialog).queryByRole('textbox', { name:'重命名项目 Alpha' })).toBeNull()
  }, 30000)

  test('deletes a project workspace while keeping the active chat history', async () => {
    installStableChatPolyfills()
    let deleted = false
    const session = { id:'project-session', title:'Project chat', project_mode:'Alpha', workspace:'temp/projects/Alpha', count:1, updated_at:'2026-08-18T10:00:00Z' }
    globalThis.fetch = vi.fn(async (url, options = {}) => {
      const path = String(url).split('?')[0]
      if (path === '/api/config') return jsonResponse({ slash_commands:[] })
      if (path === '/api/slash-commands') return jsonResponse({ commands:[] })
      if (path === '/api/extra-system-prompt-presets') return jsonResponse({ presets:[] })
      if (path === '/api/instances') return jsonResponse({ instances:[] })
      if (path === '/api/chat/sessions') return jsonResponse({ sessions:[{ ...session, project_mode:deleted ? '' : 'Alpha', workspace:deleted ? '' : session.workspace }], projects:deleted ? [] : ['Alpha'], pinned_projects:[] })
      if (path === '/api/chat/session/project-session') return jsonResponse({ ...session, messages:[{ id:'message-1', role:'assistant', content:'历史仍在' }], raw_history:[], history_info:[], settings:{ llm_no:0, tools_mode:'official' } })
      if (path === '/api/chat/state/project-session') return jsonResponse({ llms:[], settings:{ llm_no:0, tools_mode:'official' }, loop:{ enabled:false } })
      if (path === '/api/chat/projects/delete' && options.method === 'DELETE') {
        expect(options.headers?.['X-GA-Confirm']).toBe('dangerous')
        deleted = true
        return jsonResponse({ ok:true, name:'Alpha', detached_sessions:1, sessions_preserved:true, projects:[], pinned_projects:[] })
      }
      throw new Error(`unexpected url ${url}`)
    })
    mockDialog(true)
    render(<ChatApp />)

    await waitFor(() => expect(document.querySelector('.oa-title b')?.textContent).toBe('Project chat'))
    fireEvent.click(await screen.findByRole('tab', { name:'项目' }))
    fireEvent.click(await screen.findByRole('button', { name:'管理' }))
    const dialog = await screen.findByRole('dialog', { name:'管理项目' })
    fireEvent.click(within(dialog).getByRole('button', { name:'删除项目 Alpha' }))

    await waitFor(() => expect(screen.getByRole('status').textContent).toContain('项目 Alpha 已删除'))
    expect(within(screen.getByRole('dialog', { name:'管理项目' })).getByText('暂无项目，请先新建一个项目。')).toBeTruthy()
    expect(document.querySelector('.oa-title b')?.textContent).toBe('Project chat')
    expect(document.body.textContent).toContain('历史仍在')
  }, 30000)
})

describe('chat worldline controls', () => {
  const worldline = {
    schema_version: 1,
    available: true,
    root_id: 'root',
    head: 'left',
    current_path: ['root', 'left'],
    nodes: [
      { id: 'root', title: '起点', parent_id: null, mapping_status: 'unmapped', ordinal: 0 },
      { id: 'left', title: '当前分支', parent_id: 'root', mapping_status: 'mapped', ordinal: 0 },
      { id: 'right', title: '另一分支', parent_id: 'root', mapping_status: 'mapped', ordinal: 1 },
    ],
  }

  test('shows a loading explanation before worldline data arrives', () => {
    render(<WorldlinePanel state={null} loading switchingId="" disabled={false} onClose={vi.fn()} onRefresh={vi.fn()} onSwitch={vi.fn()} />)
    expect(screen.getByText('正在初始化并读取当前会话的世界线…')).toBeTruthy()
    expect(screen.getByText('正在读取世界线数据')).toBeTruthy()
  })

  test('lists branches and switches a mapped node', () => {
    const onSwitch = vi.fn()
    render(<WorldlinePanel state={worldline} loading={false} switchingId="" disabled={false} onClose={vi.fn()} onRefresh={vi.fn()} onSwitch={onSwitch} />)
    expect(screen.getByText('共 3 个节点 · 1 个分支节点')).toBeTruthy()
    expect(screen.getByText('当前分支')).toBeTruthy()
    expect(screen.getByText('另一分支')).toBeTruthy()
    expect(screen.getByText('仅记录')).toBeTruthy()
    fireEvent.click(screen.getByRole('button', { name: '切换' }))
    expect(onSwitch).toHaveBeenCalledWith('right')
  })

  test('activates the current session before loading worldline state', async () => {
    const sessions = [{ id: 'worldline-session', title: 'Worldline chat', count: 2, updated_at: '2026-08-05T10:00:00Z' }]
    globalThis.fetch = vi.fn(async url => {
      const path = String(url)
      if (path === '/api/config') return jsonResponse({ slash_commands: [] })
      if (path === '/api/slash-commands') return jsonResponse({ commands: [] })
      if (path === '/api/chat/sessions') return jsonResponse({ sessions })
      if (path === '/api/chat/session/worldline-session') return jsonResponse({ ...sessions[0], messages: [], raw_history: [], history_info: [], settings: { llm_no: 0, tools_mode: 'official' } })
      if (path === '/api/chat/state/worldline-session') return jsonResponse({ llms: [], settings: { llm_no: 0, tools_mode: 'official' } })
      if (path === '/api/chat/worldline/worldline-session?activate=true') return jsonResponse(worldline)
      throw new Error(`unexpected url ${url}`)
    })

    Element.prototype.scrollIntoView = vi.fn()
    render(<ChatApp />)
    await waitFor(() => expect(document.querySelector('.oa-title b')?.textContent).toBe('Worldline chat'))
    fireEvent.click(await screen.findByRole('button', { name: '查看和切换对话世界线' }, { timeout: 20000 }))
    await waitFor(() => expect(screen.getByText('共 3 个节点 · 1 个分支节点')).toBeTruthy())
    expect(globalThis.fetch.mock.calls.some(([url]) => String(url) === '/api/chat/worldline/worldline-session?activate=true')).toBe(true)
  }, 30000)
})

describe('chat loop controls', () => {
  test('privacy mode hides a Loop objective in the same interaction that starts it', async () => {
    installBrowserPolyfills()
    Element.prototype.scrollIntoView = vi.fn()
    window.localStorage.setItem('ga-admin-chat-privacy-mode', 'true')
    const sessions = [{ id:'private-loop', title:'SECRET_LOOP_CHAT', count:0, updated_at:'2026-08-18T10:00:00Z' }]
    let resolveStart
    let startedBody = null
    const startResponse = new Promise(resolve => { resolveStart = resolve })
    globalThis.fetch = vi.fn(async (url, options = {}) => {
      const path = String(url).split('?')[0]
      if (path === '/api/config') return jsonResponse({ slash_commands:[] })
      if (path === '/api/slash-commands') return jsonResponse({ commands:[] })
      if (path === '/api/instances') return jsonResponse({ items:[] })
      if (path === '/api/chat/sessions') return jsonResponse({ sessions })
      if (path === '/api/chat/session/private-loop') return jsonResponse({ ...sessions[0], messages:[], raw_history:[], history_info:[], settings:{ llm_no:0, tools_mode:'official' } })
      if (path === '/api/chat/state/private-loop') return jsonResponse({ llms:[], settings:{ llm_no:0, tools_mode:'official' }, loop:{ enabled:false, status:'waiting', round:0, max_rounds:0 } })
      if (path === '/api/chat/loop/private-loop/start') {
        startedBody = JSON.parse(options.body)
        return startResponse
      }
      throw new Error(`unexpected url ${url}`)
    })

    render(<ChatApp />)
    await screen.findByRole('region', { name:'任务状态' }, { timeout:10000 })
    fireEvent.click(screen.getByRole('button', { name:'展开 Loop 栏' }))
    fireEvent.click(screen.getByRole('button', { name:'配置 Loop' }))
    const objective = screen.getByRole('textbox', { name:'目标' })
    fireEvent.change(objective, { target:{ value:'SECRET_LOOP_OBJECTIVE' } })
    expect(document.body.innerHTML).toContain('SECRET_LOOP_OBJECTIVE')

    fireEvent.click(screen.getByRole('button', { name:'启动 Loop' }))
    await waitFor(() => expect(startedBody?.objective).toBe('SECRET_LOOP_OBJECTIVE'))
    expect(document.body.innerHTML).not.toContain('SECRET_LOOP_OBJECTIVE')
    expect(screen.queryByRole('textbox', { name:'目标' })).toBeNull()

    resolveStart(jsonResponse({ ok:true, loop:{ enabled:true, status:'waiting', round:0, max_rounds:10, controller_prompt:'SECRET_LOOP_OBJECTIVE' } }))
    await waitFor(() => expect(screen.getByRole('button', { name:'停止 Loop' })).toBeTruthy())
    expect(document.body.innerHTML).not.toContain('SECRET_LOOP_OBJECTIVE')
  }, 60000)

  test('explains an empty objective, uses the current message, and completes start-stop flow', async () => {
    installBrowserPolyfills()
    Element.prototype.scrollIntoView = vi.fn()
    const sessions = [{ id:'loop-session', title:'Loop chat', count:0, updated_at:'2026-08-11T10:00:00Z' }]
    const model = { index:0, provider:'Provider A', model:'loop-model' }
    const stoppedLoop = { enabled:false, status:'waiting', epoch:0, round:0, max_rounds:0, controller_llm_no:0 }
    let startedBody = null
    globalThis.fetch = vi.fn(async (url, options = {}) => {
      const path = String(url).split('?')[0]
      if (path === '/api/config') return jsonResponse({ slash_commands:[] })
      if (path === '/api/slash-commands') return jsonResponse({ commands:[] })
      if (path === '/api/extra-system-prompt-presets') return jsonResponse({ presets:[] })
      if (path === '/api/instances') return jsonResponse({ items:[] })
      if (path === '/api/chat/sessions') return jsonResponse({ sessions })
      if (path === '/api/chat/session/loop-session') return jsonResponse({ ...sessions[0], messages:[], raw_history:[], history_info:[], settings:{ llm_no:0, tools_mode:'official' } })
      if (path === '/api/chat/state/loop-session') return jsonResponse({ llms:[model], settings:{ llm_no:0, tools_mode:'official' }, loop:startedBody ? { enabled:true, status:'waiting', epoch:1, round:0, max_rounds:3, controller_prompt:startedBody.objective, controller_llm_no:startedBody.controller_llm_no } : stoppedLoop })
      if (path === '/api/chat/loop/loop-session/start') {
        startedBody = JSON.parse(options.body)
        return jsonResponse({ ok:true, loop:{ enabled:true, status:'waiting', epoch:1, round:0, max_rounds:startedBody.max_rounds, controller_prompt:startedBody.objective, controller_llm_no:startedBody.controller_llm_no } })
      }
      if (path === '/api/chat/loop/loop-session/stop') return jsonResponse({ ok:true, loop:{ enabled:false, status:'stopped', epoch:2, round:0, max_rounds:10, stop_reason:'user', controller_prompt:startedBody?.objective || '' } })
      throw new Error(`unexpected url ${url}`)
    })

    render(<ChatApp />)
    await waitFor(() => expect(document.querySelector('.oa-title b')?.textContent).toBe('Loop chat'))
    expect(screen.queryByRole('complementary', { name:'Loop 控制' })).toBeNull()
    fireEvent.click(screen.getByRole('button', { name:'展开 Loop 栏' }))
    expect(screen.getByRole('complementary', { name:'Loop 控制' })).toBeTruthy()
    fireEvent.click(screen.getByRole('button', { name:'配置 Loop' }))
    const startButton = screen.getByRole('button', { name:'启动 Loop' })
    const objective = screen.getByRole('textbox', { name:'目标' })
    const maxRounds = screen.getByRole('spinbutton', { name:'最多轮次' })
    expect(screen.getByText('项目 TODO 修复闭环')).toBeTruthy()
    expect(startButton.disabled).toBe(false)
    fireEvent.click(startButton)
    await waitFor(() => expect(screen.getByText('请填写目标，或先在当前输入框写入任务后再启动。')).toBeTruthy())

    fireEvent.click(screen.getByRole('button', { name:'填入此 Demo' }))
    expect(objective.value).toContain('请检查当前项目 TODO')
    expect(maxRounds.value).toBe('3')
    fireEvent.change(objective, { target:{ value:'' } })

    const prompt = '请整理当前项目的待办，并在完成后汇报结果。'
    fireEvent.change(document.querySelector('.oa-composer textarea'), { target:{ value:prompt } })
    expect(screen.getByText('目标为空，启动后会立即执行当前输入内容。')).toBeTruthy()

    fireEvent.click(screen.getByRole('button', { name:/Loop 控制模型/ }))
    await waitFor(() => expect(document.querySelector('.oa-cselect-menu-portal')).toBeTruthy())
    fireEvent.mouseDown(screen.getByRole('option', { name:'Provider A / loop-model' }))
    fireEvent.click(screen.getByRole('button', { name:'启动 Loop' }))
    await waitFor(() => expect(startedBody?.objective).toBe(prompt))
    expect(startedBody?.max_rounds).toBe(3)
    expect(document.querySelector('.oa-composer textarea')?.value).toBe('')
    expect(screen.getByRole('button', { name:'停止 Loop' })).toBeTruthy()

    fireEvent.click(screen.getByRole('button', { name:'收起' }))
    expect(screen.queryByRole('complementary', { name:'Loop 控制' })).toBeNull()
    expect(screen.getByRole('button', { name:'展开 Loop 栏' }).textContent).toContain('0/3')
    fireEvent.click(screen.getByRole('button', { name:'展开 Loop 栏' }))

    fireEvent.click(screen.getByRole('button', { name:'停止 Loop' }))
    await waitFor(() => expect(globalThis.fetch.mock.calls.some(([url, options]) => String(url).split('?')[0] === '/api/chat/loop/loop-session/stop' && options?.method === 'POST')).toBe(true))
  }, 60000)
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
      files: I18N.zh.files,
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

  test('expands the preview workspace and restores the file list', () => {
    const props = fileProps()
    Object.assign(props, {
      filePath: 'memory/guide.md',
      loadedFilePath: 'memory/guide.md',
      loadedFileContent: '# Guide',
      fileContent: '# Guide',
    })
    const { container } = render(<FilesPage {...props}/>)
    const workspace = container.querySelector('.files-workspace')

    expect(workspace.classList.contains('files-preview-expanded')).toBe(false)
    fireEvent.click(screen.getByRole('button', { name: '扩大预览' }))
    expect(workspace.classList.contains('files-preview-expanded')).toBe(true)
    expect(screen.getByRole('button', { name: '恢复分栏' }).getAttribute('aria-pressed')).toBe('true')

    fireEvent.click(screen.getByRole('button', { name: '恢复分栏' }))
    expect(workspace.classList.contains('files-preview-expanded')).toBe(false)
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

describe('project TODO module panel', () => {
  const payload = {
    source_exists: true,
    source_path: 'temp/TODO.txt',
    total: 3,
    open: 2,
    completed: 1,
    items: [
      { id: 'task-1', title: '修复定时任务调度', module: 'tasks', status: 'queued', approved: true, summary: '任务需要按计划执行', line: 12 },
      { id: 'model-1', title: '模型可用性检查', module: 'models', status: 'pending', line: 18 },
      { id: 'file-1', title: '已归档文件清理', module: 'files', status: 'completed', line: 22 },
    ],
    modules: [
      { module: 'tasks', total: 1, open: 1, completed: 0 },
      { module: 'models', total: 1, open: 1, completed: 0 },
      { module: 'files', total: 1, open: 0, completed: 1 },
    ],
  }

  test('shows only the owning module, readable status, search, and source action', async () => {
    globalThis.fetch = vi.fn(async url => {
      expect(String(url)).toBe('/api/todos')
      return jsonResponse(payload)
    })
    const onOpenSource = vi.fn()
    render(<ModuleTodoPanel module="tasks" onOpenSource={onOpenSource}/>)

    await screen.findByText('修复定时任务调度')
    expect(screen.getByText('已批准，等待执行')).toBeTruthy()
    expect(screen.queryByText('模型可用性检查')).toBeNull()
    fireEvent.click(screen.getByRole('button', { name: '已完成' }))
    expect(screen.getByText('没有匹配的待办')).toBeTruthy()
    fireEvent.click(screen.getByRole('button', { name: '未完成' }))
    fireEvent.change(screen.getByRole('textbox', { name: '搜索标题、章节或轮次' }), { target: { value: '调度' } })
    expect(screen.getByText('修复定时任务调度')).toBeTruthy()
    fireEvent.click(screen.getByRole('button', { name: '查看源文件：修复定时任务调度' }))
    expect(onOpenSource).toHaveBeenCalledWith(expect.objectContaining({ id: 'task-1', sourcePath: 'temp/TODO.txt' }))
  })

  test('overview groups TODOs by processing status instead of product modules', async () => {
    globalThis.fetch = vi.fn(async () => jsonResponse(payload))
    render(<ModuleTodoPanel module="overview"/>)
    await screen.findByText('项目待办')
    expect(screen.getByText('按处理状态')).toBeTruthy()
    expect(screen.getByText('待处理')).toBeTruthy()
    expect(screen.getByText('待执行')).toBeTruthy()
    expect(screen.getByText('待同步')).toBeTruthy()
    expect(screen.getByRole('button', { name: '已完成：1 项' })).toBeTruthy()
    expect(screen.queryByTitle('进入模块：模型')).toBeNull()
    fireEvent.click(screen.getByRole('button', { name: /待执行/ }))
    expect(screen.getByText('修复定时任务调度')).toBeTruthy()
    expect(screen.queryByText('模型可用性检查')).toBeNull()
  })

  test('exposes a retry state when the TODO endpoint is temporarily unavailable', async () => {
    let attempts = 0
    globalThis.fetch = vi.fn(async () => {
      attempts += 1
      if (attempts === 1) throw new Error('offline')
      return jsonResponse(payload)
    })
    render(<ModuleTodoPanel module="tasks"/>)
    await screen.findByText(/待办读取失败/)
    fireEvent.click(screen.getByRole('button', { name: '重试' }))
    await screen.findByText('修复定时任务调度')
    expect(attempts).toBe(2)
  })
})
