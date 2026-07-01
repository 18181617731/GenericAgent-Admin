import { AlertTriangle, CheckCircle2, Eye, EyeOff, Layers, Plus, RefreshCw, Trash2, UploadCloud } from 'lucide-react'
import { useMemo, useState } from 'react'
import { Alert, Button, Card, Collapse, Input, Select, Space, Tag } from 'antd'
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

const modelIdOf = value => String(value?.id || value?.name || value || '').trim()
const uniqueModels = values => {
  const seen = new Set()
  return (values || []).map(modelIdOf).filter(v => {
    if (!v || seen.has(v)) return false
    seen.add(v)
    return true
  })
}
const profileModels = profile => uniqueModels([...(Array.isArray(profile?.models) ? profile.models : []), profile?.model])
const modelPatch = models => ({ models: uniqueModels(models), model: uniqueModels(models)[0] || '' })
const modelCountLabel = profile => {
  const count = profileModels(profile).length
  return count > 1 ? `${count} 个模型` : (profile?.model || '未选择模型')
}

function DiscoverRow({ value = [], onChange, opts, onDiscover, busy, disabled }) {
  const selected = useMemo(() => uniqueModels(value), [value])
  const options = useMemo(() => uniqueModels([...(opts || []), ...selected]).map(v => ({ value: v, label: v })), [opts, selected])
  const [open, setOpen] = useState(false)
  const handleFocus = () => { if (options.length) setOpen(true) }
  const handleDiscover = async () => {
    await onDiscover()
    setOpen(true)
  }
  return (
    <div className="discover-row discover-row--multi">
      <label>
        模型 IDs
        <Select
          mode="tags"
          value={selected}
          options={options}
          open={open && options.length > 0}
          onDropdownVisibleChange={setOpen}
          onFocus={handleFocus}
          onChange={models => onChange(uniqueModels(models))}
          tokenSeparators={[',', '\n']}
          placeholder="输入一个或多个 model，或从获取结果中选择"
          notFoundContent="先点击获取模型，或直接输入模型 ID"
          className="model-autocomplete model-multi-select"
          maxTagCount="responsive"
        />
        <span className="model-discover-count">
          {selected.length ? `已选择 ${selected.length} 个模型` : '可把同协议、同 BaseURL 的多个模型聚合在一张卡片里。'}
          {options.length > 0 ? ` · 已获取 ${options.length} 个可选项` : ''}
        </span>
      </label>
      <Button onClick={handleDiscover} loading={busy} disabled={disabled} icon={<RefreshCw size={13} />}>
        {busy ? '获取中…' : '获取模型'}
      </Button>
    </div>
  )
}

function ProfileCard({ p, idx, profileKey, result, patchProfile, removeProfile, discoverModels, profiles, revealedKey, revealBusy, onRevealKey, onClearRevealedKey, onSave, saveState }) {
  const [busy, setBusy] = useState(false)
  const [discErr, setDiscErr] = useState('')
  const [discovered, setDiscovered] = useState([])
  const patch = obj => patchProfile(idx, obj)
  const shownApiKey = revealedKey ?? p.apikey ?? ''
  const revealed = revealedKey != null
  const meta = protocolMeta(p.type || DEFAULT_PROTOCOL)
  const canDiscover = supportsModelDiscovery(p.type || DEFAULT_PROTOCOL)

  const selectedModels = profileModels(p)
  const modelOpts = useMemo(() => uniqueModels([...selectedModels, ...discovered.map(modelIdOf)]), [discovered, selectedModels])

  const discover = async () => {
    if (!canDiscover) return
    setBusy(true); setDiscErr('')
    try {
      const d = await discoverModels({ protocol: p.type || DEFAULT_PROTOCOL, baseUrl: p.apibase, apiKey: shownApiKey, varName: p.var_name })
      const models = d?.models || []
      setDiscovered(models)
      if (models.length && selectedModels.length === 0) patch(modelPatch([modelIdOf(models[0])]))
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
    <div className="model-card-header">
      <div className="model-card-name">{p.name || p.var_name || `模型 ${idx + 1}`}</div>
      <div className="model-card-sub">
        <Tag color={meta.color} className="model-proto-tag">{protocolLabel(p.type || DEFAULT_PROTOCOL)}</Tag>
        {selectedModels.length > 0 && <span className="model-card-model">{modelCountLabel(p)}</span>}
        {p.apibase && <span className="model-card-base">{p.apibase}</span>}
      </div>
    </div>
  )
  const saveBusy = saveState?.status === 'saving'
  const saveOk = saveState?.status === 'saved'
  const saveErr = saveState?.status === 'error'
  const extra = (
    <Space size={6} onClick={e => e.stopPropagation()}>
      <StatusTag result={result} />
      {saveOk && <span className="model-save-chip model-save-chip--ok">✓ 已保存</span>}
      {saveErr && <span className="model-save-chip model-save-chip--err">✗ 失败</span>}
      <Button size="small" type="primary" icon={<CheckCircle2 size={13} />} loading={saveBusy} disabled={saveBusy || result?.errors?.length > 0} onClick={() => onSave?.(idx, profileKey)}>保存</Button>
      <Button size="small" danger icon={<Trash2 size={12} />} onClick={() => removeProfile(idx)} title="删除此配置" />
    </Space>
  )
  const profileItems = [{
    key: 'profile',
    label: header,
    extra,
    children: (
      <>
        <div className="form-grid">
          <label>显示名<Input value={p.name || ''} onChange={e => patch({ name: e.target.value })} placeholder="例如 glm-5.1" /></label>
          <label>BaseURL<Input value={p.apibase || ''} onChange={e => patch({ apibase: e.target.value })} placeholder="https://api.openai.com/v1" /></label>
          <label className="span2">API Key
            {revealed ? (
              <Input
                value={shownApiKey}
                onChange={e => { onClearRevealedKey?.(idx, p, profileKey); patch({ apikey: e.target.value }) }}
                placeholder="保留 ****** 表示不覆盖已保存密钥"
                addonAfter={(
                  <Space size={4}>
                    <Button size="small" type="text" icon={<EyeOff size={13} />} loading={revealBusy} onClick={() => onRevealKey?.(idx, p, false, profileKey)}>隐藏</Button>
                    <Button size="small" type="text" icon={<RefreshCw size={13} />} loading={revealBusy} onClick={() => onRevealKey?.(idx, p, true, profileKey)}>刷新</Button>
                  </Space>
                )}
              />
            ) : (
              <Input
                type="password"
                value={shownApiKey}
                onChange={e => { onClearRevealedKey?.(idx, p, profileKey); patch({ apikey: e.target.value }) }}
                placeholder="保留 ****** 表示不覆盖已保存密钥"
                addonAfter={(
                  <Space size={4}>
                    <Button size="small" type="text" icon={<Eye size={13} />} loading={revealBusy} onClick={() => onRevealKey?.(idx, p, false, profileKey)}>显示</Button>
                  </Space>
                )}
              />
            )}
          </label>
        </div>
        <DiscoverRow value={selectedModels} onChange={models => patch(modelPatch(models))} opts={modelOpts}
          onDiscover={discover} busy={busy} disabled={busy || !canDiscover || !p.apibase} />
        {!canDiscover && <Alert className="model-inline-alert" type="info" showIcon message="该官方协议没有通用模型列表接口，请手动填写 model。" />}
        {discErr && <Alert type="error" showIcon message={discErr} className="model-inline-alert" />}
        {saveErr && <Alert type="error" showIcon message={`保存失败：${saveState?.error || '未知错误'}`} className="model-inline-alert" />}
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
  const [f, setF] = useState({ protocol: DEFAULT_PROTOCOL, baseUrl: '', apiKey: '', models: [], name: '' })
  const [busy, setBusy] = useState(false)
  const [discovered, setDiscovered] = useState([])
  const [err, setErr] = useState('')
  const pf = obj => setF(prev => ({ ...prev, ...obj }))
  const meta = protocolMeta(f.protocol)
  const canDiscover = supportsModelDiscovery(f.protocol)
  const selectedModels = uniqueModels(f.models)
  const modelOpts = useMemo(() => uniqueModels([...selectedModels, ...discovered.map(modelIdOf)]), [discovered, selectedModels])

  const discover = async () => {
    if (!canDiscover) return
    setBusy(true); setErr('')
    try {
      const d = await discoverModels({ protocol: f.protocol || DEFAULT_PROTOCOL, baseUrl: f.baseUrl, apiKey: f.apiKey })
      const models = d?.models || []
      setDiscovered(models)
      if (models.length && selectedModels.length === 0) pf({ models: [modelIdOf(models[0])] })
    } catch (e) { setErr(String(e?.message || e)) }
    finally { setBusy(false) }
  }

  const add = () => {
    const idx = profiles.length
    const models = uniqueModels(f.models)
    setProfiles([...profiles, { ...emptyProfile(idx, f.protocol), var_name: nextVarName(f.protocol, profiles), type: f.protocol, apibase: f.baseUrl, apikey: f.apiKey, models, model: models[0] || '', name: f.name || models[0] || `model-${idx + 1}` }])
    setF({ protocol: DEFAULT_PROTOCOL, baseUrl: '', apiKey: '', models: [], name: '' })
    setDiscovered([])
    setErr('')
  }

  return (
    <Card size="small"
      title={<span className="model-add-card-title"><Plus size={13} strokeWidth={2.5} />新增模型配置</span>}
      className="model-add-card">
      <div className="model-add-body">
        <div className="model-add-row2">
          <label className="model-add-label">官方协议
            <Select value={f.protocol} onChange={v => pf({ protocol: v, model: supportsModelDiscovery(v) ? f.model : '' })} options={OFFICIAL_PROTOCOLS} style={{width:'100%'}} />
          </label>
          <label className="model-add-label">BaseURL
            <Input value={f.baseUrl} onChange={e => pf({ baseUrl: e.target.value })} placeholder="https://api.openai.com/v1" />
          </label>
        </div>
        <label className="model-add-label">API Key（可选）
          <Input type="password" value={f.apiKey} onChange={e => pf({ apiKey: e.target.value })} placeholder={t.hints?.savedSecret || '保留空白或填写密钥'} />
        </label>
        {meta.help && <p className="model-add-hint"><Tag color={meta.color} style={{marginRight:4}}>{protocolLabel(f.protocol)}</Tag>{meta.help}</p>}
        <DiscoverRow value={selectedModels} onChange={models => pf({ models })}
          opts={modelOpts}
          onDiscover={discover} busy={busy} disabled={busy || !canDiscover || !f.baseUrl} />
        {!canDiscover && <Alert className="model-inline-alert" type="info" showIcon message="该官方协议没有通用模型列表接口，请手动填写 model。" />}
        {err && <Alert className="model-inline-alert" type="error" showIcon message={err} />}
        <label className="model-add-label">显示名称
          <Input value={f.name} onChange={e => pf({ name: e.target.value })} placeholder={selectedModels[0] || '可选，留空自动填入'} />
        </label>
        <Button type="primary" icon={<Plus size={13} />} disabled={!selectedModels.length || !f.baseUrl} onClick={add} block className="model-add-button">
          添加配置
        </Button>
      </div>
    </Card>
  )
}

export function Models({ t, profiles, setProfiles, patchProfile, importModels, previewModels, saveModelProfile, discoverModels, modelPreview, modelSaveStatus = {}, importLoading = false, riskCatalog, riskCatalogError, revealedKeys = {}, revealBusy = {}, getProfileKey, onRevealKey, onClearRevealedKey }) {
  const validation = validateModelProfiles(profiles)
  const summary = modelValidationSummary(validation)
  const risk = modelRiskCatalog(riskCatalog, riskCatalogError)
  const hasErrors = summary.errors > 0
  const profileKeyId = (idx, profile) => getProfileKey?.(idx, profile) || profile?.client_id || `${profile?.var_name || nextVarName(profile?.type || DEFAULT_PROTOCOL, profiles)}:${profile?.type || DEFAULT_PROTOCOL}:${profile?.apibase || ''}:${idx}`
  const removeProfile = idx => {
    onClearRevealedKey?.(idx, profiles[idx], profileKeyId(idx, profiles[idx]))
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
          description={risk.status === 'error' ? risk.error : '获取模型为只读；每个模型卡片点击“保存”后才会保存，保存仍受 confirmDanger 门禁保护。'}
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
        <div className="model-toolbar-info">
          <Tag color={hasErrors ? 'error' : 'success'} className="model-summary-tag">
            <span className="mst-num">{summary.total}</span><span className="mst-label">项</span>
            {summary.errors > 0 && <><span className="mst-dot" />  <span className="mst-num mst-err">{summary.errors}</span><span className="mst-label"> 错</span></>}
            {summary.warnings > 0 && <><span className="mst-dot" /><span className="mst-num mst-warn">{summary.warnings}</span><span className="mst-label"> 警</span></>}
          </Tag>
        </div>
        <div className="model-toolbar-actions">
          <Button icon={<UploadCloud size={14} />} onClick={() => importModels()} loading={importLoading}>读取</Button>
          <Button icon={<Eye size={14} />} onClick={previewModels}>预览</Button>
        </div>
      </div>
      {hasErrors && <Alert type="error" showIcon message="保存前请先修复红色阻断项" className="model-inline-alert" />}

      <div className="model-layout">
        <div className="model-left">
          <AddProfileForm profiles={profiles} setProfiles={setProfiles} discoverModels={discoverModels} t={t} />
          <div className="model-list">
            {profiles.map((p, idx) => {
              const keyId = profileKeyId(idx, p)
              return <ProfileCard key={keyId} p={p} idx={idx} profileKey={keyId} profiles={profiles}
                result={validation[idx]} patchProfile={patchProfile} removeProfile={removeProfile} discoverModels={discoverModels}
                revealedKey={revealedKeys[keyId]} revealBusy={!!revealBusy[keyId]}
                onRevealKey={onRevealKey} onClearRevealedKey={onClearRevealedKey} onSave={saveModelProfile} saveState={modelSaveStatus[keyId] || modelSaveStatus[idx]} />
            })}
            {!profiles.length && (
              <div className="model-empty-state">
                <Layers size={36} strokeWidth={1.2} className="model-empty-icon" />
                <p className="model-empty-title">{importLoading ? '正在读取模型配置…' : '暂无模型配置'}</p>
                <span className="model-empty-sub">{importLoading ? '自动从 mykey.py 读取中，请稍候。' : '进入此页自动读取 mykey.py，也可点击工具栏“读取”手动导入，或直接新增。'}</span>
              </div>
            )}
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
