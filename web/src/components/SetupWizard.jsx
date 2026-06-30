import React, { useEffect, useMemo, useRef, useState } from 'react'
import { Alert, Button, Card, Col, Descriptions, Divider, Input, Row, Space, Steps, Tag, Typography } from 'antd'
import { Bot, CheckCircle2, Download, GitPullRequest, Play, RefreshCw, Terminal, Wand2 } from 'lucide-react'
import { api, apiStream } from '../lib/api.js'
import { confirmDanger } from '../lib/danger.js'

const { Paragraph, Text, Title } = Typography

const steps = [
  { key: 'root', title: '选择 GenericAgent', desc: '接管已有源码目录，或安装到新目录。' },
  { key: 'venv', title: '创建 Python venv', desc: '在 GA 根目录下创建隔离虚拟环境。' },
  { key: 'deps', title: '安装依赖', desc: '执行 pip install -r requirements.txt，并显示实时日志。' },
  { key: 'smoke', title: '冒烟验证', desc: '确认后端可用 Python 启动并识别 GA。' },
  { key: 'complete', title: '完成接管', desc: '写入 bootstrap_done，进入 GA Admin。' }
]

const normalizeRoot = (value) => (value || '').trim()

function toolByName(tools, name) {
  return (Array.isArray(tools) ? tools : []).find(tool => tool?.name === name) || {}
}

function normalizeSetupEnv(payload = {}) {
  const tools = Array.isArray(payload.tools) ? payload.tools : []
  return {
    ...payload,
    python: payload.python || toolByName(tools, 'python'),
    git: payload.git || toolByName(tools, 'git'),
    uv: payload.uv || toolByName(tools, 'uv'),
    npm: payload.npm || toolByName(tools, 'npm'),
    can_auto_install_python: Boolean(payload.can_auto_install_python ?? payload.python_installer),
  }
}

function setupEnvError(error) {
  return normalizeSetupEnv({ ok: false, error: error?.message || String(error || '环境检测失败'), tools: [] })
}

function pythonDisplay(pythonPath, pythonInfo) {
  return pythonPath || pythonInfo.version || pythonInfo.path || pythonInfo.error || '未选择'
}

function statusText(state) {
  if (!state) return '读取中'
  if (state.bootstrap_done) return '已完成'
  if (state.ga_root) return '已选择 GA'
  return '首次配置'
}

function isErrorMessage(message) {
  return /失败|错误|error|ERROR/i.test(message || '')
}

export default function SetupWizard({ initialRoot = '', onComplete }) {
  const [state, setState] = useState(null)
  const [root, setRoot] = useState(initialRoot || '')
  const [installPath, setInstallPath] = useState(initialRoot || '')
  const [busy, setBusy] = useState('')
  const [message, setMessage] = useState('')
  const [logLines, setLogLines] = useState([])
  const [lastSmoke, setLastSmoke] = useState(null)
  const logRef = useRef(null)

  const effectiveRoot = normalizeRoot(root || state?.ga_root)
  const currentIndex = useMemo(() => {
    if (!effectiveRoot) return 0
    if (!state?.venv?.ok) return 1
    if (!lastSmoke?.ok) return 2
    return 4
  }, [effectiveRoot, state?.venv?.ok, lastSmoke?.ok])

  const reload = async () => {
    const [next, envResult] = await Promise.all([
      api('/api/setup/state'),
      api('/api/setup/env').catch(setupEnvError),
    ])
    const merged = { ...next, env: normalizeSetupEnv(envResult) }
    setState(merged)
    if (merged?.ga_root && !root) setRoot(merged.ga_root)
    if (merged?.ga_root && !installPath) setInstallPath(merged.ga_root)
    return merged
  }

  const refresh = async () => {
    setBusy('setup-refresh')
    setMessage('')
    try {
      await reload()
      setMessage('环境检测已刷新。')
    } catch (e) {
      setMessage(e.message)
    } finally {
      setBusy('')
    }
  }

  useEffect(() => { reload().catch(e => setMessage(e.message)) }, [])
  useEffect(() => {
    if (!logRef.current) return
    logRef.current.scrollTop = logRef.current.scrollHeight
  }, [logLines])

  const env = state?.env || {}
  const pythonInfo = env.python || {}
  const gitInfo = env.git || {}
  const osInfo = env.os || {}
  const pythonOk = Boolean(state?.python || pythonInfo.ok)
  const gitOk = Boolean(gitInfo.ok)
  const canInstallPython = Boolean(env.can_auto_install_python)
  const pythonStatus = pythonDisplay(state?.python, pythonInfo)
  const selectedPython = pythonDisplay(state?.python, pythonInfo)
  const gitStatus = gitInfo.version || gitInfo.path || gitInfo.error || '\u672a\u68c0\u6d4b\u5230 Git\uff0c\u5c06\u4f7f\u7528 ZIP \u5f52\u6863\u5b89\u88c5'
  const installSourceCopy = gitOk
    ? '优先使用 Git clone；如网络或 Git 异常，将自动回退到 GitHub ZIP 归档。'
    : '未检测到 Git，将直接下载 GitHub ZIP 归档安装 GenericAgent。'

  const runJson = async (operation, text, url, body = {}) => {
    if (!confirmDanger(operation, text)) return null
    setBusy(operation)
    setMessage('')
    try {
      const result = await api(url, { dangerous: true, method: 'POST', body: JSON.stringify(body) })
      if (result?.root) setRoot(result.root)
      if (result?.health || result?.venv || result?.config) await reload()
      return result
    } catch (e) {
      setMessage(e.message)
      return null
    } finally {
      setBusy('')
    }
  }

  const validateRoot = async () => {
    const target = normalizeRoot(root)
    if (!target) { setMessage('请先填写 GenericAgent 根目录。'); return }
    const result = await runJson('setup-validate', `验证并保存 GA 根目录：${target}？`, '/api/setup/validate', { path: target })
    if (!result) return
    setMessage(result.ok ? 'GA 根目录验证通过。' : '目录未通过 GA 健康检查，请确认包含 GenericAgent 源码。')
  }

  const installGA = async () => {
    const installDir = normalizeRoot(installPath || root)
    if (!installDir) { setMessage('请填写安装父目录。'); return }
    const trimmedInstallDir = installDir.replace(/[\\/]+$/, '')
    const separator = trimmedInstallDir.includes('\\') ? '\\' : '/'
    const finalTarget = `${trimmedInstallDir}${separator}GenericAgent`
    const result = await runJson('setup-install', `${installSourceCopy}\n安装父目录：${installDir}\n将生成 GA 根目录：${finalTarget}。继续？`, '/api/setup/install', { path: installDir })
    if (!result) return
    setMessage(result.ok ? `GenericAgent 已通过 ${result.method === 'archive' ? 'ZIP 归档' : 'Git'} 安装/接管。` : '安装完成但健康检查未通过。')
  }

  const installPython = async () => {
    if (!canInstallPython) {
      setMessage('当前系统不支持内置 Python 安装器，请先手动安装 Python 3.11+ 后刷新环境。')
      return
    }
    const result = await runJson('setup-python-install', '将下载并静默安装 Python，然后写入 GA Admin 配置。继续？', '/api/setup/python/install', {})
    if (!result) return
    setMessage(`Python 已安装并写入配置：${result.version || result.python || 'Python OK'}`)
  }

  const createVenv = async () => {
    const target = effectiveRoot
    if (!target) { setMessage('请先完成 GA 根目录验证。'); return }
    const result = await runJson('setup-venv-create', `将在 ${target} 下创建或更新 .venv。继续？`, '/api/setup/venv/create', { root: target })
    if (!result) return
    setMessage('虚拟环境已创建，GA Admin 已切换到 venv Python。')
  }

  const installDeps = async () => {
    const target = effectiveRoot
    if (!target) { setMessage('请先完成 GA 根目录验证。'); return }
    if (!confirmDanger('setup-deps-install', `将在 ${target} 执行 pip install -r requirements.txt。继续？`)) return
    setBusy('setup-deps-install')
    setMessage('正在安装依赖…')
    setLogLines([])
    try {
      const res = await apiStream('/api/setup/deps/install', { dangerous: true, method: 'POST', body: JSON.stringify({ root: target }) })
      const reader = res.body?.getReader()
      if (!reader) throw new Error('当前浏览器不支持流式读取依赖安装输出')
      const decoder = new TextDecoder()
      let buf = ''
      let doneEvent = null
      while (true) {
        const { value, done } = await reader.read()
        if (done) break
        buf += decoder.decode(value, { stream: true })
        const parts = buf.split('\n')
        buf = parts.pop() || ''
        for (const part of parts) {
          if (!part.trim()) continue
          const ev = JSON.parse(part)
          if (ev.line) setLogLines(lines => [...lines, ev.line].slice(-600))
          if (ev.error) setLogLines(lines => [...lines, `ERROR: ${ev.error}`].slice(-600))
          if (ev.type === 'done') doneEvent = ev
        }
      }
      if (buf.trim()) {
        const ev = JSON.parse(buf)
        if (ev.line) setLogLines(lines => [...lines, ev.line].slice(-600))
        if (ev.error) setLogLines(lines => [...lines, `ERROR: ${ev.error}`].slice(-600))
        if (ev.type === 'done') doneEvent = ev
      }
      if (!doneEvent?.ok) throw new Error(doneEvent?.error || '依赖安装失败')
      setMessage('依赖安装完成。')
    } catch (e) {
      setMessage(e.message)
    } finally {
      setBusy('')
    }
  }

  const smoke = async () => {
    const target = effectiveRoot
    if (!target) { setMessage('请先完成 GA 根目录验证。'); return }
    const result = await runJson('setup-smoke', `使用当前 Python 对 ${target} 执行冒烟验证。继续？`, '/api/setup/smoke', { root: target })
    if (!result) return
    setLastSmoke(result)
    setMessage(`冒烟验证通过：${result.python || 'Python OK'}`)
  }

  const complete = async () => {
    const target = effectiveRoot
    if (!target) { setMessage('请先完成 GA 根目录验证。'); return }
    const result = await runJson('setup-complete', '确认完成首次配置并进入 GA Admin？', '/api/setup/complete', { root: target })
    if (!result) return
    setMessage('首次配置完成，正在进入 GA Admin…')
    onComplete?.(result)
  }

  const isBusy = Boolean(busy)
  const smokeReady = lastSmoke?.ok
  const stepItems = steps.map((step, index) => ({
    key: step.key,
    title: step.title,
    description: step.desc,
    status: index < currentIndex || (step.key === 'smoke' && smokeReady) ? 'finish' : index === currentIndex ? 'process' : 'wait'
  }))

  return <div className="setup-wizard-shell">
    <div className="setup-wizard-bg" />
    <Card className="setup-wizard-card" bordered={false}>
      <div className="setup-wizard-hero">
        <div className="setup-wizard-copy">
          <Text className="eyebrow">GA Admin Bootstrap</Text>
          <Title level={1}>首次启动配置</Title>
          <Paragraph>
            按顺序接管 GenericAgent、创建 Python 隔离环境、安装依赖并完成冒烟验证。每个会修改本机状态的动作都会先弹出危险确认。
          </Paragraph>
        </div>
        <Space className="setup-wizard-status" size={8}>
          <Bot size={18}/>
          <span>{statusText(state)}</span>
        </Space>
      </div>

      <Steps className="setup-ant-steps" current={currentIndex} responsive items={stepItems} />

      <Card className="setup-env-card" size="small" title={<Space><Wand2 size={16}/>本机环境预检</Space>}>
        <Row gutter={[12, 12]}>
          <Col xs={24} md={12}>
            <Descriptions size="small" column={1}>
              <Descriptions.Item label="Python">
                <Space wrap>
                  <Tag color={pythonOk ? 'green' : 'orange'}>{pythonOk ? '可用' : '缺失'}</Tag>
                  <Text type={pythonOk ? undefined : 'warning'}>{pythonStatus}</Text>
                </Space>
              </Descriptions.Item>
            </Descriptions>
          </Col>
          <Col xs={24} md={12}>
            <Descriptions size="small" column={1}>
              <Descriptions.Item label="Git">
                <Space wrap>
                  <Tag color={gitOk ? 'green' : 'blue'}>{gitOk ? '可用' : '可选'}</Tag>
                  <Text type={gitOk ? undefined : 'secondary'}>{gitStatus}</Text>
                </Space>
              </Descriptions.Item>
            </Descriptions>
          </Col>
        </Row>
        <Space wrap>
          <Button icon={<RefreshCw size={15}/>} onClick={refresh} disabled={isBusy}>重新检测环境</Button>
          {!pythonOk && (
            <Button type="primary" icon={<Download size={15}/>} onClick={installPython} disabled={isBusy || !canInstallPython} loading={busy === 'setup-python-install'}>
              自动安装 Python
            </Button>
          )}
        </Space>
        <Paragraph type="secondary" className="setup-env-hint">
          Git 不再是首次配置的硬依赖；缺少 Git 时会下载 GitHub ZIP 归档。缺少 Python 时，可在 Windows 上直接触发内置安装器。
        </Paragraph>
      </Card>

      <Row className="setup-grid" gutter={[16, 16]}>
        <Col xs={24} lg={12}>
          <Card className="setup-panel" title={<Space><GitPullRequest size={18}/>接管或安装 GenericAgent</Space>} extra={<Tag color={effectiveRoot ? 'green' : 'default'}>{effectiveRoot ? '已选择' : '待选择'}</Tag>}>
            <Space direction="vertical" size={14} className="setup-stack">
              <div className="setup-field">
                <Text strong>已有 GA 根目录</Text>
                <Input
                  value={root}
                  onChange={e => setRoot(e.target.value)}
                  placeholder="例如 C:\\Users\\you\\Desktop\\code\\GenericAgent"
                  disabled={isBusy}
                  allowClear
                />
                <Button type="primary" icon={<CheckCircle2 size={15}/>} onClick={validateRoot} disabled={isBusy || !normalizeRoot(root)} loading={busy === 'setup-validate'}>
                  验证并使用
                </Button>
              </div>

              <Divider plain>或</Divider>

              <div className="setup-field">
                <Text strong>安装父目录</Text>
                <Input
                  value={installPath}
                  onChange={e => setInstallPath(e.target.value)}
                  placeholder="例如 C:\Users\you\Desktop\code（将在其下创建 GenericAgent）"
                  disabled={isBusy}
                  allowClear
                />
                <Button icon={<Wand2 size={15}/>} onClick={installGA} disabled={isBusy || !normalizeRoot(installPath || root)} loading={busy === 'setup-install'}>
                  安装 GA
                </Button>
                <Text type="secondary" className="setup-install-hint">{installSourceCopy}</Text>
              </div>

              {state?.ga_root && <Alert
                type="success"
                showIcon
                message="当前 GA Root"
                description={<Text code copyable>{state.ga_root}</Text>}
              />}
            </Space>
          </Card>
        </Col>

        <Col xs={24} lg={12}>
          <Card className="setup-panel" title={<Space><Play size={18}/>Python / 依赖 / 验证</Space>} extra={<Tag color={smokeReady ? 'green' : 'blue'}>{smokeReady ? '验证通过' : '待验证'}</Tag>}>
            <Space direction="vertical" size={14} className="setup-stack">
              <Descriptions size="small" column={1} bordered className="setup-descriptions">
                <Descriptions.Item label="Python"><Text code>{selectedPython}</Text></Descriptions.Item>
                <Descriptions.Item label="venv"><Text code>{state?.venv?.ok ? state.venv.path : '未创建'}</Text></Descriptions.Item>
              </Descriptions>

              <Space wrap className="setup-actions-stack">
                <Button icon={<RefreshCw size={15}/>} onClick={createVenv} disabled={isBusy || !effectiveRoot} loading={busy === 'setup-venv-create'}>
                  创建 venv
                </Button>
                <Button icon={<Terminal size={15}/>} onClick={installDeps} disabled={isBusy || !effectiveRoot} loading={busy === 'setup-deps-install'}>
                  安装依赖
                </Button>
                <Button icon={<CheckCircle2 size={15}/>} onClick={smoke} disabled={isBusy || !effectiveRoot} loading={busy === 'setup-smoke'}>
                  冒烟验证
                </Button>
                <Button type="primary" onClick={complete} disabled={isBusy || !effectiveRoot || !smokeReady} loading={busy === 'setup-complete'}>
                  完成并进入
                </Button>
              </Space>
            </Space>
          </Card>
        </Col>
      </Row>

      {message && <Alert className="setup-message" type={isErrorMessage(message) ? 'error' : 'success'} showIcon message={message} />}

      <Card className="setup-log-card" title={<Space><Terminal size={16}/>依赖安装日志</Space>} size="small">
        <pre className="setup-log" ref={logRef}>{logLines.join('\n') || '依赖安装日志会显示在这里。'}</pre>
      </Card>
    </Card>
  </div>
}
