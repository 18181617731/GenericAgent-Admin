import { AlertTriangle, CheckCircle2, Eye, EyeOff, Plus, RefreshCw, UploadCloud } from 'lucide-react'
import { useMemo, useState } from 'react'
import { Alert, AutoComplete, Button, Card, Collapse, Input, Select, Space, Tag } from 'antd'
import { emptyProfile } from '../lib/format'
import { modelRiskCatalog, modelValidationSummary, validateModelProfiles } from '../lib/modelsValidation'

const DEFAULT_PROTOCOL = 'native_oai'
const OFFICIAL_PROTOCOLS = [
  { value: 'native_oai', label: 'Native OAI（推荐 / OpenAI 兼容）', prefix: 'native_oai_config', discover: true, color: 'blue', help: '官方 NativeOAISession：变量名同时包含 native 与 oai，支持 OpenAI-compatible /models 获取模型。' },
  { value: 'native_claude', label: 'Native Claude（Anthropic 兼容）', prefix: 'native_claude_config', discover: true, color: 'purple', help: '官方 NativeClaudeSession：变量名同时包含 native 与 claude，支持 Anthropic-compatible /models 或 /v1/models 获取模型。' },
  { value: 'oai', label: 'OAI / LLMSession（旧协议）', prefix: 'oai_config', discover: true, color: 'cyan', help: '官方非 Native OpenAI 文本协议：变量名包含 oai。新配置建议优先使用 Native OAI。' },
  { value: 'claude', label: 'ClaudeSession（旧协议）', prefix: 'claude_config', discover: true, color: 'magenta', help: '官方非 Native Claude 文本协议：变量名包含 claude，支持 Anthropic-compatible /models 或 /v1/models 获取模型。新配置建议优先使用 Native Claude。' },
]
const LEGACY_PROTOCOLS = [
  ...OFFICIAL_PROTOCOLS,
  { value: 'openai', label: '兼容旧值：openai', prefix: 'native_oai_config', discover: true, color: 'blue' },
  { value: 'openai-compatible', label: '兼容旧值：openai-compatible', prefix: 'native_oai_config', discover: true, color: 'blue' },
  { value: 'chatgpt', label: '兼容旧值：chatgpt', prefix: 'oai_config', discover: true, color: 'cyan' },
]

const protocolMeta = value => LEGACY_PROTOCOLS.find(x => x.value === value) || OFFICIAL_PROTOCOLS[0]
const protocolLabel = value => protocolMeta(value)?.label || value || 'Native OAI'
const supportsModelDiscovery = value => !!protocolMeta(value)?.discover
const nextVarName = (protocol, profiles = []) => {
  const prefix = protocolMeta(protocol)?.prefix || 'native_oai_config'
  const used = new Set((profiles || []).map(p => p?.var_name).filter(Boolean))
  let i = Math.max(1, (profiles || []).length + 1)
  while (used.has(`${prefix}${i}`)) i += 1
  return `${prefix}${i}`
}

const ERR_KEYS = {
  varNameRequired: '必须填写变量名', varNameInvalid: '变量名须为 Python 标识符',
  varNameDiscoveryToken: '变量名须含 api / config / cookie，或使用官方 native_oai_config / native_claude_config / oai_config / claude_config 命名',
  varNameDuplicate: '变量名重复', nameRequired: '必须填写名称',
  modelRequired: '必须填写模型', apiBaseRequired: '必须填写 API Base',
  apiBaseProtocol: 'API Base 须以 http:// 或 https:// 开头',
  maxRetriesInvalid: '重试次数须 ≥ 0', readTimeoutInvalid: '超时须 > 0',
  apiKeyEmpty: 'API Key 为空（仅用于本地或无认证端点）',
}

function StatusTag({ result }) {
  if (!result) return null
  const e = result.errors?.length, w = result.warnings?.length
  return e ? <Tag color="error">{e} 错</Tag> : w ? <Tag color="warning">{w} 警</Tag> : <Tag color="success">✓</Tag>
}

function DiscoverRow({ value, onChange, opts, onDiscover, busy, disabled }) {
  const options = useMemo(() => {
    const seen = new Set()
    return opts.filter(Boolean).filter(v => {
      if (seen.has(v)) return false
      seen.add(v)
      return true
    }).map(v => ({ value: v, label: v }))
  }, [opts])
  const [open, setOpen] = useState(false)
  const handleFocus = () => { if (options.length) setOpen(true) }
  const handleDiscover = async () => {
    await onDiscover()
    setOpen(true)
  }
  return (
    <div className="discover-row">
      <label>
        模型
        <AutoComplete
          value={value}
          options={options}
          open={open && options.length > 0}
          onOpenChange={setOpen}
          onFocus={handleFocus}
          onChange={onChange}
          onSelect={v => onChange(v)}
          filterOption={false}
          placeholder="输入 model，或从自动获取结果中选择"
          notFoundContent="先点击获取模型"
          className="model-autocomplete"
        />
        {options.length > 0 && <span className="model-discover-count">已获取 {options.length} 个模型，点击输入框可展开全部。</span>}
      </label>
      <Button onClick={handleDiscover} loading={busy} disabled={disabled} icon={<RefreshCw size={13} />}>
        {busy ? '获取中…' : '获取模型'}
      </Button>
    </div>
  )
}

function ProfileCard({ p, idx, result, patchProfile, removeProfile, discoverModels, profiles, revealedKey, revealBusy, onRevealKey, onClearRevealedKey }) {
  const [busy, setBusy] = useState(false)
  const [discErr, setDiscErr] = useState('')
  const [discovered, setDiscovered] = useState([])
  const patch = obj => patchProfile(idx, obj)
  const shownApiKey = revealedKey ?? p.apikey ?? ''
  const revealed = revealedKey != null
  const meta = protocolMeta(p.type || DEFAULT_PROTOCOL)
  const canDiscover = supportsModelDiscovery(p.type || DEFAULT_PROTOCOL)

  const modelOpts = useMemo(() => {
    const seen = new Set()
    return [p.model, ...discovered.map(m => m.id || m.name || m)]
      .filter(Boolean).filter(v => { if (seen.has(v)) return false; seen.add(v); return true })
  }, [discovered, p.model])

  const discover = async () => {
    if (!canDiscover) return
    setBusy(true); setDiscErr('')
    try {
      const d = await discoverModels({ protocol: p.type || DEFAULT_PROTOCOL, baseUrl: p.apibase, apiKey: shownApiKey, varName: p.var_name })
      const models = d?.models || []
      setDiscovered(models)
      if (models.length && !p.model) patch({ model: models[0].id || models[0].name || models[0] })
    } catch (e) { setDiscErr(String(e?.message || e)) }
    finally { setBusy(false) }
  }

  const advItems = [{
    key: 'adv', label: '高级配置', children: (
      <div className="form-grid">
        <label>变量名<Input value={p.var_name || ''} onChange={e => patch({ var_name: e.target.value })} /></label>
        <label>官方协议<Select value={p.type || DEFAULT_PROTOCOL} onChange={v => patch({ type: v, var_name: nextVarName(v, profiles) })} options={OFFICIAL_PROTOCOLS} /></label>
        <label>流式<Select value={String(!!p.stream)} onChange={v => patch({ stream: v === 'true' })}
          options={[{ value: 'true', label: 'true' }, { value: 'false', label: 'false' }]} /></label>
        <label>重试<Input type="number" value={p.max_retries ?? 3} onChange={e => patch({ max_retries: Number(e.target.value) })} /></label>
        <label>超时(s)<Input type="number" value={p.read_timeout ?? 300} onChange={e => patch({ read_timeout: Number(e.target.value) })} /></label>
        <label>reasoning_effort<Input value={p.reasoning_effort || ''} onChange={e => patch({ reasoning_effort: e.target.value })} /></label>
      </div>
    )
  }]

  const header = (
    <Space size={6} wrap>
      <b>{p.name || p.var_name || `模型 ${idx + 1}`}</b>
      <Tag color={meta.color}>{protocolLabel(p.type || DEFAULT_PROTOCOL)}</Tag>
      {p.model && <Tag>{p.model}</Tag>}
      {p.apibase && <span className="sub">{p.apibase}</span>}
    </Space>
  )
  const extra = (
    <Space size={6} onClick={e => e.stopPropagation()}>
      <StatusTag result={result} />
      <Button size="small" danger onClick={() => removeProfile(idx)}>删除</Button>
    </Space>
  )
  const profileItems = [{
    key: 'profile',
    label: header,
    extra,
    children: (
      <>
        <div className="model-protocol-note">
          <Tag color={meta.color}>{protocolLabel(p.type || DEFAULT_PROTOCOL)}</Tag>
          <span>{meta.help}</span>
        </div>
        <div className="form-grid">
          <label>显示名<Input value={p.name || ''} onChange={e => patch({ name: e.target.value })} placeholder="例如 glm-5.1" /></label>
          <label>BaseURL<Input value={p.apibase || ''} onChange={e => patch({ apibase: e.target.value })} placeholder="https://api.openai.com/v1" /></label>
          <label className="span2">API Key
            {revealed ? (
              <Input
                value={shownApiKey}
                onChange={e => { onClearRevealedKey?.(idx, p); patch({ apikey: e.target.value }) }}
                placeholder="保留 ****** 表示不覆盖已保存密钥"
                addonAfter={(
                  <Space size={4}>
                    <Button size="small" type="text" icon={<EyeOff size={13} />} loading={revealBusy} onClick={() => onRevealKey?.(idx, p, false)}>隐藏</Button>
                    <Button size="small" type="text" icon={<RefreshCw size={13} />} loading={revealBusy} onClick={() => onRevealKey?.(idx, p, true)}>刷新</Button>
                  </Space>
                )}
              />
            ) : (
              <Input
                type="password"
                value={shownApiKey}
                onChange={e => { onClearRevealedKey?.(idx, p); patch({ apikey: e.target.value }) }}
                placeholder="保留 ****** 表示不覆盖已保存密钥"
                addonAfter={(
                  <Space size={4}>
                    <Button size="small" type="text" icon={<Eye size={13} />} loading={revealBusy} onClick={() => onRevealKey?.(idx, p, false)}>显示</Button>
                  </Space>
                )}
              />
            )}
          </label>
        </div>
        <DiscoverRow value={p.model || ''} onChange={model => patch({ model })} opts={modelOpts}
          onDiscover={discover} busy={busy} disabled={busy || !canDiscover || !p.apibase} />
        {!canDiscover && <Alert className="model-inline-alert" type="info" showIcon message="该官方协议没有通用模型列表接口，请手动填写 model。" />}
        {discErr && <Alert type="error" showIcon message={discErr} className="model-inline-alert" />}
        {result?.errors?.length > 0 && <Alert type="error" showIcon message="阻断项"
          description={<ul>{result.errors.map(k => <li key={k}>{ERR_KEYS[k] || k}</li>)}</ul>} className="model-inline-alert" />}
        {result?.warnings?.length > 0 && <Alert type="warning" showIcon message="警告"
          description={<ul>{result.warnings.map(k => <li key={k}>{ERR_KEYS[k] || k}</li>)}</ul>} className="model-inline-alert" />}
        <Collapse ghost items={advItems} />
      </>
    )
  }]

  return <Collapse className="model-profile-card" size="small" items={profileItems} />
}

function AddProfileForm({ profiles, setProfiles, discoverModels, t }) {
  const [f, setF] = useState({ protocol: DEFAULT_PROTOCOL, baseUrl: '', apiKey: '', model: '', name: '' })
  const [busy, setBusy] = useState(false)
  const [discovered, setDiscovered] = useState([])
  const [err, setErr] = useState('')
  const pf = obj => setF(prev => ({ ...prev, ...obj }))
  const meta = protocolMeta(f.protocol)
  const canDiscover = supportsModelDiscovery(f.protocol)

  const discover = async () => {
    if (!canDiscover) return
    setBusy(true); setErr('')
    try {
      const d = await discoverModels({ protocol: f.protocol || DEFAULT_PROTOCOL, baseUrl: f.baseUrl, apiKey: f.apiKey })
      const models = d?.models || []
      setDiscovered(models)
      if (models.length && !f.model) pf({ model: models[0].id || models[0].name || models[0] })
    } catch (e) { setErr(String(e?.message || e)) }
    finally { setBusy(false) }
  }

  const add = () => {
    const idx = profiles.length
    setProfiles([...profiles, { ...emptyProfile(idx, f.protocol), var_name: nextVarName(f.protocol, profiles), type: f.protocol, apibase: f.baseUrl, apikey: f.apiKey, model: f.model, name: f.name || f.model || `model-${idx + 1}` }])
    setF({ protocol: DEFAULT_PROTOCOL, baseUrl: '', apiKey: '', model: '', name: '' })
    setDiscovered([])
    setErr('')
  }

  return (
    <Card size="small" title="新增模型配置" className="model-add-card">
      <div className="model-protocol-note">
        <Tag color={meta.color}>{protocolLabel(f.protocol)}</Tag>
        <span>{meta.help}</span>
      </div>
      <div className="form-grid">
        <label>官方协议<Select value={f.protocol} onChange={v => pf({ protocol: v, model: supportsModelDiscovery(v) ? f.model : '' })} options={OFFICIAL_PROTOCOLS} /></label>
        <label>BaseURL<Input value={f.baseUrl} onChange={e => pf({ baseUrl: e.target.value })} placeholder="https://api.openai.com/v1" /></label>
        <label className="span2">API Key（可选）
          <Input type="password" value={f.apiKey} onChange={e => pf({ apiKey: e.target.value })} placeholder={t.hints?.savedSecret || '保留空白或填写密钥'} />
        </label>
      </div>
      <DiscoverRow value={f.model} onChange={model => pf({ model })}
        opts={[f.model, ...discovered.map(m => m.id || m.name || m)].filter(Boolean)}
        onDiscover={discover} busy={busy} disabled={busy || !canDiscover || !f.baseUrl} />
      {!canDiscover && <Alert className="model-inline-alert" type="info" showIcon message="该官方协议没有通用模型列表接口，请手动填写 model。" />}
      {err && <Alert className="model-inline-alert" type="error" showIcon message={err} />}
      <label className="span2 model-display-name">显示名称
        <Input value={f.name} onChange={e => pf({ name: e.target.value })} placeholder={f.model || '可选，留空自动填入'} />
      </label>
      <Button type="primary" icon={<Plus size={13} />} disabled={!f.model || !f.baseUrl} onClick={add} className="model-add-button">
        添加配置
      </Button>
    </Card>
  )
}

export function Models({ t, profiles, setProfiles, patchProfile, importModels, previewModels, saveModels, discoverModels, modelPreview, riskCatalog, riskCatalogError, revealedKeys = {}, revealBusy = {}, onRevealKey, onClearRevealedKey }) {
  const validation = validateModelProfiles(profiles)
  const summary = modelValidationSummary(validation)
  const risk = modelRiskCatalog(riskCatalog, riskCatalogError)
  const hasErrors = summary.errors > 0
  const profileKeyId = (idx, profile) => `${idx}:${profile?.var_name || ''}`
  const removeProfile = idx => {
    onClearRevealedKey?.(idx, profiles[idx])
    setProfiles(profiles.filter((_, i) => i !== idx))
  }

  const riskItems = [{
    key: 'risk',
    label: <Space size={6}><AlertTriangle size={14} />模型路由安全</Space>,
    children: (
      <>
        <Alert
          type={risk.status === 'error' ? 'error' : 'info'}
          message={risk.status === 'ready' ? '风险目录已加载' : risk.status === 'error' ? '风险目录不可用' : '暂无条目'}
          description={risk.status === 'error' ? risk.error : '自动获取模型为只读；写回操作受 confirmDanger 门禁保护。'}
        />
        {risk.items.length > 0 && (
          <div className="model-risk-grid">
            {risk.items.map(item => (
              <div key={`${item.method}-${item.route}`}>
                <b>{item.method} {item.route}</b>
                <small>{item.action || item.reason}</small>
              </div>
            ))}
          </div>
        )}
        {risk.missingConfirmedWriteRoutes.length > 0 && (
          <Alert type="warning" message={`目录缺少已确认写入门禁：${risk.missingConfirmedWriteRoutes.join(', ')}`} className="model-inline-alert" />
        )}
      </>
    )
  }]

  return (
    <section className="models-page">
      <div className="model-toolbar">
        <Space wrap>
          <Tag color={hasErrors ? 'error' : 'success'}>{summary.total} 项 · {summary.errors} 错 · {summary.warnings} 警</Tag>
          <Button icon={<UploadCloud size={14} />} onClick={() => importModels(false)}>读取</Button>
          <Button icon={<Eye size={14} />} onClick={previewModels}>预览</Button>
          <Button type="primary" icon={<CheckCircle2 size={14} />} disabled={hasErrors} onClick={saveModels}>写回</Button>
        </Space>
      </div>
      {hasErrors && <Alert type="error" showIcon message="写回前请先修复红色阻断项" className="model-inline-alert" />}

      <div className="model-layout">
        <div className="model-left">
          <AddProfileForm profiles={profiles} setProfiles={setProfiles} discoverModels={discoverModels} t={t} />
          <div className="model-list">
            {profiles.map((p, idx) => <ProfileCard key={`${p.var_name}-${idx}`} p={p} idx={idx} profiles={profiles}
              result={validation[idx]} patchProfile={patchProfile} removeProfile={removeProfile} discoverModels={discoverModels}
              revealedKey={revealedKeys[profileKeyId(idx, p)]} revealBusy={!!revealBusy[profileKeyId(idx, p)]}
              onRevealKey={onRevealKey} onClearRevealedKey={onClearRevealedKey} />)}
            {!profiles.length && <Card className="empty-card">尚未读取到模型配置。点击“读取”导入 mykey.py，或直接新增。</Card>}
          </div>
        </div>

        <div className="model-right">
          <Card title="生成预览" size="small">
            <pre className="model-preview-pre">{modelPreview || (profiles.length ? '点击“预览”按钮生成预览。' : '添加至少一个模型配置后显示预览。')}</pre>
          </Card>
          <Collapse ghost items={riskItems} className="model-inline-alert" />
        </div>
      </div>
    </section>
  )
}
