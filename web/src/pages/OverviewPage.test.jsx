import React from 'react'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { cleanup, render, screen } from '@testing-library/react'
import { I18N, SETTINGS_TEXT } from '../lib/i18n'
import { OverviewPage } from './OverviewPage'

afterEach(() => {
  cleanup()
  vi.restoreAllMocks()
})

const versionStub = (gitStatus) => ({
  info: { version: 'dev', commit: 'abc1234', date: '2026-08-14', runtime: 'go1.25', goos: 'windows', goarch: 'amd64', update_supported: true },
  check: { update: false },
  status: null,
  busy: false,
  gitBusy: false,
  gitStatus,
  autostart: { supported: true, enabled: false },
  checkVersion: vi.fn(),
  updateVersion: vi.fn(),
  checkSource: vi.fn(),
  toggleAutostart: vi.fn(),
})

const renderOverview = ({ lang = 'zh', gitStatus, observability, services = [], githubMirror = '', onSaveGitHubMirror = vi.fn() } = {}) => render(<OverviewPage
  t={I18N[lang]}
  text={SETTINGS_TEXT[lang]}
  services={services}
  schedule={{ task_count: 2 }}
  observability={observability}
  observabilityError=""
  onRefreshObservability={vi.fn()}
  version={versionStub(gitStatus)}
  root="E:/Work/GenericAgent"
  githubMirror={githubMirror}
  onSaveGitHubMirror={onSaveGitHubMirror}
/>)

describe('OverviewPage', () => {
  it('shows failed checks instead of a count-only health pill', () => {
    renderOverview({
      observability: {
        ok: false,
        root: 'E:/Work/GenericAgent',
        generatedAt: '2026-08-14T09:00:00+08:00',
        checks: [
          { name: 'agentmain.py', state: 'ok' },
          { name: 'tools_schema', state: 'missing' },
          { name: 'reflect', state: 'empty' },
        ],
        errors: ['tools_schema: missing'],
        warnings: ['reflect: empty'],
        coreFiles: [{ path: 'agentmain.py', exists: true }],
        missingCore: [],
        riskItems: [],
      },
      services: [{ name: 'hub', running: true, pid: 12 }],
    })

    expect(screen.getByText('只读观测')).toBeTruthy()
    expect(screen.getByText('tools_schema: missing')).toBeTruthy()
    expect(screen.getByText('缺失')).toBeTruthy()
    expect(screen.getByText('hub · PID 12')).toBeTruthy()
    expect(screen.queryByText('写入门禁')).toBeNull()
  })

  it('keeps the GA source card without git but drops the branch row and check button', () => {
    renderOverview({ gitStatus: { ok: true, available: false, reason: 'git_missing' } })
    expect(screen.getByText('GA 源代码')).toBeTruthy()
    expect(screen.getByText('未检测到 git 命令，无法读取分支与更新状态。')).toBeTruthy()
    expect(screen.getByRole('button', { name: /去对话执行 \/update/i })).toBeTruthy()
    expect(screen.queryByText('分支')).toBeNull()
    expect(screen.queryByRole('button', { name: /检查是否最新/i })).toBeNull()
  })

  it('shows the branch row and check button once git can answer', () => {
    renderOverview({ gitStatus: { ok: true, available: true, root: '/ga', branch: 'main', commit: 'abc1234', upstream_configured: true, latest: true } })
    expect(screen.getByText('GA 源代码')).toBeTruthy()
    expect(screen.getByText('分支')).toBeTruthy()
    expect(screen.getByText(/main · abc1234/)).toBeTruthy()
    expect(screen.getByRole('button', { name: /检查是否最新/i })).toBeTruthy()
  })

  it('renders the English dashboard labels the shell test locks in', () => {
    renderOverview({
      lang: 'en',
      gitStatus: { ok: true, available: true, branch: 'main', commit: 'abc1234', latest: true },
    })
    expect(screen.getByText('Version management')).toBeTruthy()
    expect(screen.getByText('Read-only observability')).toBeTruthy()
    expect(screen.getByText('GA source')).toBeTruthy()
    expect(screen.getByRole('button', { name: /Run \/update in chat/i })).toBeTruthy()
  })
})
