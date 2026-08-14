import React, { useEffect, useRef } from 'react'
import { Alert, Button, Card, Col, Descriptions, Divider, Input, Row, Space, Steps, Tag, Typography } from 'antd'
import { Bot, CheckCircle2, Download, FolderOpen, GitPullRequest, Play, RefreshCw, Terminal, Wand2 } from 'lucide-react'
import { SETUP_TEXT } from '../lib/i18n.js'
import { useSetupWizard } from '../hooks/useSetupWizard.js'

const { Paragraph, Text, Title } = Typography

const TOOL_ORDER = ['python', 'git', 'uv', 'npm']
const TOOL_LABELS = { python: 'Python', git: 'Git', uv: 'uv', npm: 'npm' }
// Python is the only tool the wizard cannot work around, so it is the only one
// whose absence is framed as missing rather than optional.
const REQUIRED_TOOLS = new Set(['python'])

function ToolStatus({ tool, label, required, text }) {
  const detail = tool.version || tool.path || tool.error || text.env.notSelected
  const color = tool.ok ? 'green' : required ? 'orange' : 'blue'
  const state = tool.ok ? text.env.available : required ? text.env.missing : text.env.optional
  return <Descriptions size="small" column={1}>
    <Descriptions.Item label={label}>
      <Space wrap>
        <Tag color={color}>{state}</Tag>
        <Text type={tool.ok ? undefined : required ? 'warning' : 'secondary'}>{detail}</Text>
      </Space>
    </Descriptions.Item>
  </Descriptions>
}

export default function SetupWizard({ initialRoot = '', lang = 'zh', text, onComplete }) {
  const copy = text || SETUP_TEXT[lang] || SETUP_TEXT.zh
  const wizard = useSetupWizard({ text: copy, initialRoot, onComplete })
  const {
    env, progress, steps, currentStep, blockReason, busy, notice, logLines,
    rootDraft, setRootDraft, installDraft, setInstallDraft, installTarget,
  } = wizard
  const logRef = useRef(null)

  useEffect(() => {
    if (!logRef.current) return
    logRef.current.scrollTop = logRef.current.scrollHeight
  }, [logLines])

  const working = Boolean(busy)
  const stepItems = steps.map(step => ({
    key: step.key,
    title: copy.steps[step.key].title,
    content: copy.steps[step.key].desc,
    status: step.status,
  }))

  return <div className="setup-wizard-shell">
    <Card className="setup-wizard-card" variant="borderless">
      <div className="setup-wizard-hero">
        <div className="setup-wizard-copy">
          <Text className="eyebrow">{copy.eyebrow}</Text>
          <Title level={1}>{copy.title}</Title>
          <Paragraph>{copy.intro}</Paragraph>
        </div>
        <Space className="setup-wizard-status" size={8}>
          <Bot size={18} aria-hidden="true"/>
          <span>{copy.statusLabels[wizard.statusTone]}</span>
        </Space>
      </div>

      <Steps className="setup-ant-steps" current={currentStep} responsive items={stepItems} />

      <Card className="setup-env-card" size="small" title={<Space><Wand2 size={16} aria-hidden="true"/>{copy.env.title}</Space>}
        extra={env.checked ? <Text type="secondary" className="setup-env-checked">{copy.env.checkedAt(env.checked)}</Text> : null}>
        <Row gutter={[12, 8]}>
          {TOOL_ORDER.map(name => <Col xs={24} md={12} key={name}>
            <ToolStatus tool={env[name]} label={TOOL_LABELS[name]} required={REQUIRED_TOOLS.has(name)} text={copy} />
          </Col>)}
        </Row>
        <Space wrap>
          <Button icon={<RefreshCw size={15}/>} onClick={wizard.refresh} disabled={working} loading={busy === 'setup-refresh'}>
            {copy.env.recheck}
          </Button>
          {!env.python.ok && <Button type="primary" icon={<Download size={15}/>} onClick={wizard.installPython}
            disabled={working || !env.canInstallPython} loading={busy === 'setup-python-install'}>
            {copy.env.installPython}
          </Button>}
        </Space>
        <Paragraph type="secondary" className="setup-env-hint">
          {env.error || (!env.python.ok && !env.canInstallPython ? copy.env.pythonInstallerUnavailable : copy.env.hint)}
        </Paragraph>
      </Card>

      <Row className="setup-grid" gutter={[16, 16]}>
        <Col xs={24} lg={12}>
          <Card className="setup-panel" title={<Space><GitPullRequest size={18} aria-hidden="true"/>{copy.root.title}</Space>}
            extra={<Tag color={progress.rootReady ? 'green' : 'default'}>{progress.rootReady ? copy.root.selected : copy.root.unselected}</Tag>}>
            <Space orientation="vertical" size={14} className="setup-stack">
              <div className="setup-field">
                <Text strong>{copy.root.existingLabel}</Text>
                <div className="setup-path-row">
                  <Input value={rootDraft} onChange={e => setRootDraft(e.target.value)}
                    placeholder={copy.root.existingPlaceholder} disabled={working} allowClear />
                  <Button icon={<FolderOpen size={15}/>} onClick={() => wizard.browse('root')}
                    disabled={working} loading={busy === 'setup-browse-root'}>{copy.root.browse}</Button>
                </div>
                <Button type="primary" icon={<CheckCircle2 size={15}/>} onClick={wizard.validateRoot}
                  disabled={working || !rootDraft.trim()} loading={busy === 'setup-validate'}>
                  {copy.root.validate}
                </Button>
              </div>

              <Divider plain>{copy.root.or}</Divider>

              <div className="setup-field">
                <Text strong>{copy.root.installLabel}</Text>
                <div className="setup-path-row">
                  <Input value={installDraft} onChange={e => setInstallDraft(e.target.value)}
                    placeholder={copy.root.installPlaceholder} disabled={working} allowClear />
                  <Button icon={<FolderOpen size={15}/>} onClick={() => wizard.browse('install')}
                    disabled={working} loading={busy === 'setup-browse-install'}>{copy.root.browse}</Button>
                </div>
                <Button icon={<Wand2 size={15}/>} onClick={wizard.installGA}
                  disabled={working || !installDraft.trim()} loading={busy === 'setup-install'}>
                  {copy.root.install}
                </Button>
                {installTarget && <Text code className="setup-install-hint">{installTarget}</Text>}
                <Text type="secondary" className="setup-install-hint">{env.git.ok ? copy.root.sourceGit : copy.root.sourceArchive}</Text>
              </div>

              {progress.savedRoot && <Alert
                type={progress.rootReady ? 'success' : 'warning'}
                showIcon
                title={copy.root.currentRoot}
                description={<>
                  <Text code copyable>{progress.savedRoot}</Text>
                  {!progress.rootReady && <div className="setup-root-warning">{copy.root.unhealthy}</div>}
                </>}
              />}
            </Space>
          </Card>
        </Col>

        <Col xs={24} lg={12}>
          <Card className="setup-panel" title={<Space><Play size={18} aria-hidden="true"/>{copy.runtime.title}</Space>}
            extra={<Tag color={progress.smokeReady ? 'green' : 'blue'}>{progress.smokeReady ? copy.runtime.validated : copy.runtime.unvalidated}</Tag>}>
            <Space orientation="vertical" size={14} className="setup-stack">
              <Descriptions size="small" column={1} bordered className="setup-descriptions">
                <Descriptions.Item label={copy.runtime.python}>
                  <Text code>{progress.python || env.effectivePython || copy.env.notSelected}</Text>
                </Descriptions.Item>
                <Descriptions.Item label={copy.runtime.venv}>
                  <Text code>{progress.venv?.ok ? progress.venv.path : copy.runtime.venvMissing}</Text>
                </Descriptions.Item>
              </Descriptions>

              <Space wrap className="setup-actions-stack">
                <Button icon={<RefreshCw size={15}/>} onClick={wizard.createVenv}
                  disabled={working || !progress.rootReady} loading={busy === 'setup-venv-create'}>
                  {copy.runtime.createVenv}
                </Button>
                <Button icon={<Terminal size={15}/>} onClick={wizard.installDeps}
                  disabled={working || !progress.rootReady} loading={busy === 'setup-deps-install'}>
                  {copy.runtime.installDeps}
                </Button>
                <Button icon={<CheckCircle2 size={15}/>} onClick={wizard.runSmoke}
                  disabled={working || !progress.rootReady} loading={busy === 'setup-smoke'}>
                  {copy.runtime.smoke}
                </Button>
                <Button type="primary" onClick={wizard.finish}
                  disabled={working || Boolean(blockReason)} loading={busy === 'setup-complete'}>
                  {copy.runtime.finish}
                </Button>
              </Space>

              {/* A disabled finish button always says why, so the wizard never
                  looks stuck for a reason the user has to guess. */}
              {(blockReason || !progress.depsReady) && <Text type="secondary" className="setup-block-reason">
                {blockReason ? copy.runtime.blocked[blockReason] : copy.runtime.depsUnconfirmed}
              </Text>}
            </Space>
          </Card>
        </Col>
      </Row>

      {notice && <Alert className="setup-message" type={notice.tone} showIcon title={notice.text} />}

      <Card className="setup-log-card" size="small" title={<Space><Terminal size={16} aria-hidden="true"/>{copy.log.title}</Space>}>
        <pre className="setup-log" ref={logRef}>{logLines.join('\n') || copy.log.empty}</pre>
      </Card>
    </Card>
  </div>
}
