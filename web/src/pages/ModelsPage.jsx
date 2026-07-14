import {
  AlertTriangle,
  ArrowDown,
  ArrowUp,
  CheckCircle2,
  ChevronDown,
  Eye,
  EyeOff,
  FileCode2,
  GripVertical,
  Layers,
  ListOrdered,
  Plus,
  RefreshCw,
  Trash2,
  UploadCloud,
  X,
} from 'lucide-react'
import { useEffect, useMemo, useState } from 'react'
import { Alert, Button, Collapse, Drawer, Input, Modal, Select, Space, Tag } from 'antd'
import { emptyProfile } from '../lib/format'
import {
  API_MODE_OPTIONS,
  THINKING_TYPE_OPTIONS,
  addModelConfigs,
  modelProtocolFields,
  moveOrderedItem,
  orderedModelRows,
  profileModelConfigs,
  reasoningEffortOptions,
  removeModelConfig,
  updateModelConfig,
} from '../lib/modelsEditor'
import {
  nextProviderVarName,
  providerDisplayName,
  providerVarNameFromDisplayName,
  providerVarNameOnProtocolChange,
} from '../lib/modelsProvider'
import { modelRiskCatalog, modelValidationSummary, validateModelProfiles } from '../lib/modelsValidation'

const DEFAULT_PROTOCOL = 'native_oai'
const OFFICIAL_PROTOCOLS = [
  { value: 'native_oai', label: 'Native OAI（推荐 / OpenAI 兼容）', shortLabel: 'Native OAI', prefix: 'native_oai_config', discover: true, color: 'blue', help: '适合 OpenAI-compatible 接口，新配置优先使用。' },
  { value: 'native_claude', label: 'Native Claude（Anthropic 兼容）', shortLabel: 'Native Claude', prefix: 'native_claude_config', discover: true, color: 'purple', help: '适合 Anthropic-compatible 接口。' },
  { value: 'oai', label: 'OAI / LLMSession（旧协议）', shortLabel: 'OAI', prefix: 'oai_config', discover: true, color: 'cyan', help: 'GenericAgent 旧版 OpenAI 文本协议。' },
  { value: 'claude', label: 'ClaudeSession（旧协议）', shortLabel: 'Claude', prefix: 'claude_config', discover: true, color: 'magenta', help: 'GenericAgent 旧版 Claude 文本协议。' },
]
const LEGACY_PROTOCOLS = [
  ...OFFICIAL_PROTOCOLS,
  { value: 'openai', label: '兼容旧值：openai', shortLabel: 'OpenAI（旧值）', prefix: 'native_oai_config', discover: true, color: 'blue' },
  { value: 'openai-compatible', label: '兼容旧值：openai-compatible', shortLabel: 'OpenAI Compatible（旧值）', prefix: 'native_oai_config', discover: true, color: 'blue' },
  { value: 'chatgpt', label: '兼容旧值：chatgpt', shortLabel: 'ChatGPT（旧值）', prefix: 'oai_config', discover: true, color: 'cyan' },
]

const protocolMeta = value => LEGACY_PROTOCOLS.find(item => item.value === value) || OFFICIAL_PROTOCOLS[0]
const protocolLabel = value => protocolMeta(value)?.shortLabel || value || 'Native OAI'
const supportsModelDiscovery = value => !!protocolMeta(value)?.discover
const nextVarName = (protocol, profiles = []) => nextProviderVarName(
  protocolMeta(protocol)?.prefix || 'native_oai_config',
  profiles,
)

const ERR_KEYS = {
  varNameRequired: '必须填写变量名',
  varNameInvalid: '变量名须为 Python 标识符',
  varNameDiscoveryToken: '变量名须含 api / config / cookie，或使用官方命名',
  varNameDuplicate: '变量名重复',
  modelRequired: '至少选择一个模型',
  apiBaseRequired: '必须填写 API Base',
  apiBaseProtocol: 'API Base 须以 http:// 或 https:// 开头',
  maxRetriesInvalid: '重试次数须 ≥ 0',
  readTimeoutInvalid: '超时须 > 0',
  apiKeyEmpty: 'API Key 为空（仅适用于本地或无认证端点）',
}

const modelIdOf = value => String(value?.id || value?.name || value || '').trim()
const uniqueModels = values => {
  const seen = new Set()
  return (values || []).map(modelIdOf).filter(value => {
    if (!value || seen.has(value)) return false
    seen.add(value)
    return true
  })
}
const profileModels = profile => uniqueModels([...(Array.isArray(profile?.models) ? profile.models : []), profile?.model])
const isMaskedSecret = value => {
  const secret = String(value || '').trim()
  return /^\*{4,}$/.test(secret) || /\*{2,}/.test(secret)
}

function StatusTag({ result }) {
  if (!result) return null
  const errors = result.errors?.length || 0
  const warnings = result.warnings?.length || 0
  if (errors) return <Tag color="error">{errors} 个阻断项</Tag>
  if (warnings) return <Tag color="warning">{warnings} 个提醒</Tag>
  return <Tag color="success">配置有效</Tag>
}

function optionalNumber(value) {
  if (value === '' || value === null || value === undefined) return undefined
  const parsed = Number(value)
  return Number.isNaN(parsed) ? value : parsed
}

function OptionalBoolSelect({ value, onChange, trueLabel = '启用', falseLabel = '关闭' }) {
  return (
    <Select
      value={value === true || value === false ? value : 'inherit'}
      onChange={next => onChange(next === 'inherit' ? undefined : next)}
      options={[
        { value: 'inherit', label: '使用默认值' },
        { value: true, label: trueLabel },
        { value: false, label: falseLabel },
      ]}
    />
  )
}

function ModelConfigRow({ config, index, protocol, onChange, onRemove }) {
  const [configOpen, setConfigOpen] = useState(false)
  const fields = modelProtocolFields(protocol)
  const configSummary = [config.api_mode, config.thinking_type, config.reasoning_effort]
    .filter(Boolean)
    .join(' · ') || '默认参数'

  return (
    <article className={`model-config-row${configOpen ? ' is-open' : ''}`}>
      <div className="model-config-main">
        <div className="model-config-identity">
          <span className="model-config-index" aria-hidden="true">
            {String(index + 1).padStart(2, '0')}
          </span>
          <div className="model-config-copy">
            <span className="model-config-id" title={config.model || ''}>
              {config.model || '未命名模型'}
            </span>
            <span className="model-config-summary">{configSummary}</span>
          </div>
        </div>
        <Button
          type="text"
          className="model-config-action model-config-toggle"
          onClick={() => setConfigOpen(open => !open)}
          aria-expanded={configOpen}
        >
          <span>{configOpen ? '收起' : '配置'}</span>
          <ChevronDown size={13} className="model-config-chevron" aria-hidden="true" />
        </Button>
        <Button
          danger
          type="text"
          className="model-config-action model-config-delete"
          icon={<Trash2 size={13} />}
          onClick={onRemove}
          aria-label={`删除模型 ${config.model || index + 1}`}
        >
          删除
        </Button>
      </div>

      {configOpen && (
        <div className="model-row-advanced">
          <div className="model-row-advanced-grid">
            <label className="model-field">
              <span className="model-field-label">流式输出</span>
              <OptionalBoolSelect value={config.stream} onChange={stream => onChange({ stream })} />
            </label>
            <label className="model-field">
              <span className="model-field-label">最大重试次数</span>
              <Input type="number" min={0} value={config.max_retries ?? ''} onChange={event => onChange({ max_retries: optionalNumber(event.target.value) })} placeholder="使用默认值" />
            </label>
            <label className="model-field">
              <span className="model-field-label">读取超时（秒）</span>
              <Input type="number" min={1} value={config.read_timeout ?? ''} onChange={event => onChange({ read_timeout: optionalNumber(event.target.value) })} placeholder="使用默认值" />
            </label>
            <label className="model-field">
              <span className="model-field-label">连接超时（秒）</span>
              <Input type="number" min={1} value={config.connect_timeout ?? ''} onChange={event => onChange({ connect_timeout: optionalNumber(event.target.value) })} placeholder="使用默认值" />
            </label>
            {fields.userAgent && (
              <label className="model-field">
                <span className="model-field-label">User-Agent</span>
                <Input value={config.user_agent || ''} onChange={event => onChange({ user_agent: event.target.value || undefined })} placeholder="可选" />
              </label>
            )}
            {fields.apiMode && (
              <label className="model-field">
                <span className="model-field-label">API 模式</span>
                <Select allowClear value={config.api_mode || undefined} onChange={api_mode => onChange({ api_mode })} placeholder="使用默认值" options={API_MODE_OPTIONS} />
              </label>
            )}
            {fields.thinkingType && (
              <label className="model-field">
                <span className="model-field-label">思考类型</span>
                <Select allowClear value={config.thinking_type || undefined} onChange={thinking_type => onChange({ thinking_type })} placeholder="使用默认值" options={THINKING_TYPE_OPTIONS} />
              </label>
            )}
            {fields.reasoningFamily && (
              <label className="model-field">
                <span className="model-field-label">推理强度</span>
                <Select allowClear value={config.reasoning_effort || undefined} onChange={reasoning_effort => onChange({ reasoning_effort })} placeholder="使用默认值" options={reasoningEffortOptions(protocol)} />
              </label>
            )}
            {fields.fakeClaudeCode && (
              <label className="model-field">
                <span className="model-field-label">模拟 Claude Code 系统提示</span>
                <OptionalBoolSelect value={config.fake_cc_system_prompt} onChange={fake_cc_system_prompt => onChange({ fake_cc_system_prompt })} />
              </label>
            )}
          </div>
        </div>
      )}
    </article>
  )
}

function ModelConfigEditor({ profile, discovered = [], onChange, onDiscover, busy, disabled }) {
  const [draft, setDraft] = useState('')
  const [discoverOpen, setDiscoverOpen] = useState(false)
  const configs = profileModelConfigs(profile)
  const existing = new Set(configs.map(config => modelIdOf(config)))
  const candidates = uniqueModels(discovered).filter(model => !existing.has(model))

  const addModels = values => onChange(addModelConfigs(profile, values))
  const addDraft = () => {
    const model = draft.trim()
    if (!model) return
    addModels([model])
    setDraft('')
  }
  const openDiscover = () => {
    setDiscoverOpen(true)
    onDiscover?.()
  }
  const addCandidates = values => {
    addModels(values)
    if (values.length === candidates.length) setDiscoverOpen(false)
  }

  return (
    <section className="model-config-editor">
      <div className="model-config-toolbar">
        <strong>模型列表</strong>
        <Button onClick={openDiscover} disabled={disabled} icon={<RefreshCw size={14} />}>
          获取模型
        </Button>
      </div>

      <div className="model-config-table">
        <div className="model-config-table-head" aria-hidden="true">
          <span>模型 ID</span>
          <span>配置</span>
          <span>删除</span>
        </div>
        <div className="model-config-list">
          {configs.length > 0 ? configs.map((config, index) => (
            <ModelConfigRow
              key={index}
              config={config}
              index={index}
              protocol={profile.type || DEFAULT_PROTOCOL}
              onChange={patch => onChange(updateModelConfig(profile, index, patch))}
              onRemove={() => onChange(removeModelConfig(profile, index))}
            />
          )) : <div className="model-config-empty">还没有模型。手动输入模型 ID，或先从接口获取。</div>}
        </div>
      </div>

      <div className="model-quick-add">
        <Input
          value={draft}
          onChange={event => setDraft(event.target.value)}
          onPressEnter={addDraft}
          placeholder="手动输入模型 ID"
          aria-label="手动输入模型 ID"
        />
        <Button icon={<Plus size={14} />} onClick={addDraft} disabled={!draft.trim()}>添加模型</Button>
      </div>

      <Modal
        className="model-discover-modal"
        title="获取模型"
        open={discoverOpen}
        onCancel={() => setDiscoverOpen(false)}
        footer={null}
        width={620}
        destroyOnHidden
      >
        <div className="model-discover-modal-head">
          <span>{busy ? '正在从服务商接口获取模型…' : `发现 ${candidates.length} 个未添加模型`}</span>
          <Button size="small" type="primary" onClick={() => addCandidates(candidates)} disabled={busy || !candidates.length}>
            全部添加
          </Button>
        </div>
        {busy ? (
          <div className="model-discover-modal-state"><RefreshCw size={18} className="is-spinning" />正在获取模型</div>
        ) : candidates.length > 0 ? (
          <div className="model-candidate-list">
            {candidates.map(model => (
              <button key={model} type="button" className="model-candidate-item" onClick={() => addCandidates([model])}>
                <span title={model}>{model}</span>
                <Plus size={14} />
              </button>
            ))}
          </div>
        ) : (
          <div className="model-discover-modal-state">没有发现新的模型</div>
        )}
      </Modal>
    </section>
  )
}

function ProfileCard({
  profile: p,
  idx,
  profileKey,
  result,
  profiles,
  patchProfile,
  removeProfile,
  discoverModels,
  revealedKey,
  revealBusy,
  onRevealKey,
  onClearRevealedKey,
  onSave,
  saveState,
}) {
  const [discoverBusy, setDiscoverBusy] = useState(false)
  const [discoverError, setDiscoverError] = useState('')
  const [discovered, setDiscovered] = useState([])
  const [dirty, setDirty] = useState(false)
  const selectedModels = profileModels(p)
  const meta = protocolMeta(p.type || DEFAULT_PROTOCOL)
  const revealed = revealedKey != null && String(revealedKey).trim() !== '' && !isMaskedSecret(revealedKey)
  const shownApiKey = revealed ? revealedKey : (p.apikey ?? '')
  const saveBusy = saveState?.status === 'saving'
  const saveOk = saveState?.status === 'saved'
  const saveError = saveState?.status === 'error'

  const patch = next => {
    setDirty(true)
    patchProfile(idx, next)
  }

  useEffect(() => {
    if (saveState?.status === 'saved') setDirty(false)
  }, [saveState?.status, saveState?.savedAt])

  const save = async () => {
    const ok = await onSave?.(idx, profileKey)
    if (ok !== false) setDirty(false)
  }

  const discover = async () => {
    if (!supportsModelDiscovery(p.type || DEFAULT_PROTOCOL)) return
    setDiscoverBusy(true)
    setDiscoverError('')
    try {
      const configuredKey = String(p.apikey || '').trim()
      const response = await discoverModels({
        protocol: p.type || DEFAULT_PROTOCOL,
        baseUrl: p.apibase,
        apiKey: configuredKey && !isMaskedSecret(configuredKey) ? configuredKey : undefined,
        varName: p.var_name,
      })
      setDiscovered(response?.models || [])
    } catch (error) {
      setDiscoverError(String(error?.message || error))
    } finally {
      setDiscoverBusy(false)
    }
  }

  return (
    <article className={`model-source-card${dirty ? ' is-dirty' : ''}${result?.errors?.length ? ' has-error' : ''}`}>
      <header className="model-source-head">
        <div className="model-source-identity">
          <span className="model-source-index">{String(idx + 1).padStart(2, '0')}</span>
          <div>
            <div className="model-source-title-row">
              <strong>{providerDisplayName(p.var_name) || `服务商 ${idx + 1}`}</strong>
              <Tag color={meta.color}>{protocolLabel(p.type || DEFAULT_PROTOCOL)}</Tag>
              <span className="model-count-badge">{selectedModels.length} 个模型</span>
            </div>
            <span className="model-source-base">{p.apibase || '尚未填写 BaseURL'}</span>
          </div>
        </div>
        <Space size={8} className="model-source-actions">
          <StatusTag result={result} />
          {dirty && <span className="model-save-state is-dirty">未保存</span>}
          {!dirty && saveOk && <span className="model-save-state is-saved">已保存</span>}
          {saveError && <span className="model-save-state is-error">保存失败</span>}
          <Button
            type="primary"
            icon={<CheckCircle2 size={14} />}
            loading={saveBusy}
            disabled={saveBusy || result?.errors?.length > 0}
            onClick={save}
          >
            保存
          </Button>
          <Button danger type="text" icon={<Trash2 size={15} />} onClick={() => removeProfile(idx)} title="删除此服务商" />
        </Space>
      </header>

      <div className="model-source-body">
        <div className="model-primary-grid">
          <label className="model-field model-field--provider">
            <span className="model-field-label">名称</span>
            <Input
              value={providerDisplayName(p.var_name)}
              onChange={event => patch({
                var_name: providerVarNameFromDisplayName(
                  event.target.value,
                  meta.prefix,
                  p.var_name,
                ),
              })}
              placeholder="例如 gpt55_medium"
            />
            <small>用于区分服务商；内部协议前缀会自动维护。</small>
          </label>
          <label className="model-field">
            <span className="model-field-label">官方协议</span>
            <Select
              value={p.type || DEFAULT_PROTOCOL}
              onChange={value => patch({
                type: value,
                var_name: providerVarNameOnProtocolChange(
                  p.var_name,
                  protocolMeta(value)?.prefix,
                  profiles,
                  idx,
                ),
              })}
              options={OFFICIAL_PROTOCOLS}
            />
          </label>
          <label className="model-field model-field--base">
            <span className="model-field-label">BaseURL</span>
            <Input value={p.apibase || ''} onChange={event => patch({ apibase: event.target.value })} placeholder="https://api.example.com/v1" />
          </label>
          <label className="model-field model-field--key">
            <span className="model-field-label">API Key <em>{revealed ? '临时显示' : '默认隐藏'}</em></span>
            <Input
              type={revealed ? 'text' : 'password'}
              value={shownApiKey}
              onChange={event => {
                onClearRevealedKey?.(idx, p, profileKey)
                patch({ apikey: event.target.value })
              }}
              placeholder="保留掩码表示不覆盖已保存密钥"
              addonAfter={revealed ? (
                <Space size={2}>
                  <Button size="small" type="text" icon={<EyeOff size={14} />} loading={revealBusy} onClick={() => onRevealKey?.(idx, p, false, profileKey)}>隐藏</Button>
                  <Button size="small" type="text" icon={<RefreshCw size={13} />} loading={revealBusy} onClick={() => onRevealKey?.(idx, p, true, profileKey)} title="重新读取" />
                </Space>
              ) : (
                <Button size="small" type="text" icon={<Eye size={14} />} loading={revealBusy} onClick={() => onRevealKey?.(idx, p, false, profileKey)}>显示</Button>
              )}
            />
          </label>
        </div>

        <ModelConfigEditor
          profile={p}
          discovered={discovered}
          onChange={patch}
          onDiscover={discover}
          busy={discoverBusy}
          disabled={discoverBusy || !p.apibase || !supportsModelDiscovery(p.type || DEFAULT_PROTOCOL)}
        />

        {discoverError && <Alert type="error" showIcon message={discoverError} className="model-inline-alert" />}
        {saveError && <Alert type="error" showIcon message={`保存失败：${saveState?.error || '未知错误'}`} className="model-inline-alert" />}
        {result?.errors?.length > 0 && (
          <Alert
            type="error"
            showIcon
            message="此服务商暂时不能保存"
            description={<ul>{result.errors.map(key => <li key={key}>{ERR_KEYS[key] || key}</li>)}</ul>}
            className="model-inline-alert"
          />
        )}
        {result?.warnings?.length > 0 && (
          <Alert
            type="warning"
            showIcon
            message="保存前请留意"
            description={<ul>{result.warnings.map(key => <li key={key}>{ERR_KEYS[key] || key}</li>)}</ul>}
            className="model-inline-alert"
          />
        )}
      </div>
    </article>
  )
}

function AddProfileForm({ profiles, addModelProfiles, t, onClose, onAdded }) {
  const [form, setForm] = useState(() => ({
    protocol: DEFAULT_PROTOCOL,
    providerVar: nextVarName(DEFAULT_PROTOCOL, profiles),
    baseUrl: '',
    apiKey: '',
  }))
  const [adding, setAdding] = useState(false)
  const [error, setError] = useState('')
  const meta = protocolMeta(form.protocol)
  const patchForm = next => setForm(current => ({ ...current, ...next }))
  const changeProtocol = protocol => setForm(current => ({
    ...current,
    protocol,
    providerVar: providerVarNameOnProtocolChange(
      current.providerVar,
      protocolMeta(protocol)?.prefix,
      profiles,
    ),
  }))

  const add = async () => {
    const varName = form.providerVar.trim()
    if (!providerDisplayName(varName)) {
      setError('请填写名称。')
      return
    }
    if (!form.baseUrl.trim()) {
      setError('请填写 BaseURL。')
      return
    }
    setAdding(true)
    setError('')
    try {
      const profile = {
        ...emptyProfile(profiles.length, form.protocol),
        var_name: varName,
        type: form.protocol,
        apibase: form.baseUrl.trim(),
        apikey: form.apiKey,
        model: '',
        models: [],
        model_configs: [],
      }
      const ok = await addModelProfiles([profile])
      if (!ok) return
      setForm({
        protocol: DEFAULT_PROTOCOL,
        providerVar: nextProviderVarName(
          protocolMeta(DEFAULT_PROTOCOL)?.prefix || 'native_oai_config',
          [...profiles, profile],
        ),
        baseUrl: '',
        apiKey: '',
      })
      onAdded?.()
    } finally {
      setAdding(false)
    }
  }

  return (
    <section className="model-add-panel">
      <header className="model-add-head">
        <div>
          <strong>新增服务商</strong>
          <span>一个服务商可同时管理多个模型，只需配置一次协议、BaseURL 和密钥。</span>
        </div>
        <Button type="text" icon={<X size={16} />} onClick={onClose} aria-label="关闭新增面板" />
      </header>
      <div className="model-add-grid">
        <label className="model-field">
          <span className="model-field-label">名称</span>
          <Input
            value={providerDisplayName(form.providerVar)}
            onChange={event => patchForm({
              providerVar: providerVarNameFromDisplayName(
                event.target.value,
                meta.prefix,
                form.providerVar,
              ),
            })}
            placeholder="例如 gpt55_medium"
          />
          <small>用于区分服务商；内部协议前缀会自动维护。</small>
        </label>
        <label className="model-field">
          <span className="model-field-label">官方协议</span>
          <Select value={form.protocol} onChange={changeProtocol} options={OFFICIAL_PROTOCOLS} />
          <small>{meta.help}</small>
        </label>
        <label className="model-field model-field--base">
          <span className="model-field-label">BaseURL</span>
          <Input value={form.baseUrl} onChange={event => patchForm({ baseUrl: event.target.value })} placeholder="https://api.example.com/v1" />
        </label>
        <label className="model-field model-field--key">
          <span className="model-field-label">API Key <em>可选</em></span>
          <Input type="password" value={form.apiKey} onChange={event => patchForm({ apiKey: event.target.value })} placeholder={t.hints?.savedSecret || '填写密钥'} />
        </label>
      </div>
      {error && <Alert className="model-inline-alert" type="error" showIcon message={error} />}
      <footer className="model-add-footer">
        <span>添加后会立即保存到 mykey.py；后续修改仍需在卡片中点击“保存”。</span>
        <Space>
          <Button onClick={onClose}>取消</Button>
          <Button type="primary" icon={<Plus size={14} />} loading={adding} onClick={add}>添加并保存</Button>
        </Space>
      </footer>
    </section>
  )
}

export function Models({
  t,
  profiles,
  persistedProfiles = [],
  setProfiles,
  patchProfile,
  addModelProfiles,
  deleteModelProfile,
  importModels,
  previewModels,
  saveModelProfile,
  onSaveModelOrder,
  discoverModels,
  modelPreview,
  modelSaveStatus = {},
  importLoading = false,
  riskCatalog,
  riskCatalogError,
  revealedKeys = {},
  revealBusy = {},
  getProfileKey,
  onRevealKey,
  onClearRevealedKey,
}) {
  const [addOpen, setAddOpen] = useState(false)
  const [previewOpen, setPreviewOpen] = useState(false)
  const [activeIndex, setActiveIndex] = useState(0)
  const [orderOpen, setOrderOpen] = useState(false)
  const [orderRows, setOrderRows] = useState([])
  const [orderSaving, setOrderSaving] = useState(false)
  const [orderError, setOrderError] = useState('')
  const [dragIndex, setDragIndex] = useState(null)
  const validation = validateModelProfiles(profiles)
  const summary = modelValidationSummary(validation)
  const risk = modelRiskCatalog(riskCatalog, riskCatalogError)
  const hasErrors = summary.errors > 0
  const totalModels = profiles.reduce((count, profile) => count + profileModels(profile).length, 0)
  const profileKeyId = (idx, profile) => getProfileKey?.(idx, profile)
    || profile?.client_id
    || `${profile?.var_name || nextVarName(profile?.type || DEFAULT_PROTOCOL, profiles)}:${profile?.type || DEFAULT_PROTOCOL}:${profile?.apibase || ''}:${idx}`

  useEffect(() => {
    setActiveIndex(current => Math.min(Math.max(current, 0), Math.max(profiles.length - 1, 0)))
  }, [profiles.length])

  const removeProfile = async idx => {
    const profile = profiles[idx]
    const name = profile?.var_name || `服务商 ${idx + 1}`
    if (!window.confirm(`删除“${name}”及其中 ${profileModels(profile).length} 个模型？\n\n此操作会立即保存到 mykey.py。`)) return
    onClearRevealedKey?.(idx, profile, profileKeyId(idx, profile))
    const nextProfiles = profiles.filter((_, index) => index !== idx)
    setActiveIndex(current => current > idx ? current - 1 : Math.min(current, Math.max(nextProfiles.length - 1, 0)))
    if (deleteModelProfile) await deleteModelProfile(nextProfiles)
    else setProfiles(nextProfiles)
  }

  const openPreview = async () => {
    setPreviewOpen(true)
    await previewModels()
  }

  const openProfile = idx => {
    setAddOpen(false)
    setActiveIndex(idx)
  }

  const openAdd = () => setAddOpen(true)

  const persistedOrderCount = orderedModelRows(persistedProfiles).length
  const openModelOrder = () => {
    setOrderRows(orderedModelRows(persistedProfiles))
    setOrderError('')
    setDragIndex(null)
    setOrderOpen(true)
  }
  const closeModelOrder = () => {
    if (orderSaving) return
    setOrderOpen(false)
    setOrderRows([])
    setOrderError('')
    setDragIndex(null)
  }
  const moveModelOrder = (fromIndex, toIndex) => {
    setOrderRows(current => moveOrderedItem(current, fromIndex, toIndex))
    setOrderError('')
  }
  const dropModelOrder = toIndex => {
    if (Number.isInteger(dragIndex)) moveModelOrder(dragIndex, toIndex)
    setDragIndex(null)
  }
  const saveModelOrder = async () => {
    if (!onSaveModelOrder) {
      setOrderError('当前页面未提供顺序保存能力，请刷新后重试。')
      return
    }
    setOrderSaving(true)
    setOrderError('')
    try {
      const ok = await onSaveModelOrder(orderRows)
      if (!ok) {
        setOrderError('保存失败，当前排序草稿已保留，请检查页面提示后重试。')
        return
      }
      setOrderOpen(false)
      setOrderRows([])
      setDragIndex(null)
    } catch (error) {
      setOrderError(error?.message || '保存失败，当前排序草稿已保留。')
    } finally {
      setOrderSaving(false)
    }
  }

  const riskItems = [{
    key: 'risk',
    label: <Space size={7}><AlertTriangle size={14} />模型路由与保存安全</Space>,
    children: (
      <div className="model-risk-content">
        <Alert
          type={risk.status === 'error' ? 'error' : 'info'}
          message={risk.status === 'ready' ? '风险目录已加载' : risk.status === 'error' ? '风险目录不可用' : '暂无目录条目'}
          description={risk.status === 'error' ? risk.error : '获取模型是只读操作；新增、删除与每张卡片的“保存”才会更新 mykey.py。'}
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
          <Alert type="warning" message={`目录缺少已确认写入门禁：${risk.missingConfirmedWriteRoutes.join(', ')}`} />
        )}
      </div>
    ),
  }]

  return (
    <section className="models-page">
      <header className="model-page-head model-page-head--actions-only">
        <div className="model-page-actions">
          <Button icon={<UploadCloud size={14} />} onClick={() => importModels()} loading={importLoading}>重新读取</Button>
          <Button
            icon={<ListOrdered size={14} />}
            onClick={openModelOrder}
            disabled={!persistedOrderCount}
            title={persistedOrderCount ? '调整已保存模型的全局顺序' : '保存模型后才能调整顺序'}
          >
            模型顺序
          </Button>
          <Button icon={<FileCode2 size={14} />} onClick={openPreview}>配置预览</Button>
          <Button type="primary" icon={<Plus size={15} />} onClick={openAdd}>新增服务商</Button>
        </div>
      </header>

      <div className="model-summary-line" aria-label="配置概况">
        <div className="model-summary-status">
          <span className={`model-summary-dot${hasErrors ? ' is-error' : ''}`} />
          <strong>{summary.total} 个服务商</strong>
          <span>{totalModels} 个模型</span>
          {summary.errors > 0 && <span className="is-error">{summary.errors} 个阻断项</span>}
          {summary.warnings > 0 && <span className="is-warning">{summary.warnings} 个提醒</span>}
        </div>
        <div className="model-summary-source"><FileCode2 size={13} /><span>配置来源</span><code>mykey.py</code></div>
      </div>

      {hasErrors && <Alert type="error" showIcon message="存在不能保存的服务商，请在目录中选择异常项并修复。" className="model-page-alert" />}

      <div className="model-workbench">
        <aside className="model-provider-rail">
          <header className="model-rail-head">
            <div><strong>服务商目录</strong><span>选择一项进行编辑</span></div>
            <b>{profiles.length}</b>
          </header>

          <nav className="model-provider-nav" aria-label="模型服务商">
            {profiles.map((profile, idx) => {
              const result = validation[idx]
              const count = profileModels(profile).length
              const meta = protocolMeta(profile.type || DEFAULT_PROTOCOL)
              const state = result?.errors?.length ? 'error' : result?.warnings?.length ? 'warning' : 'ready'
              return (
                <button
                  key={profileKeyId(idx, profile)}
                  type="button"
                  className={`model-provider-item${!addOpen && activeIndex === idx ? ' is-active' : ''}`}
                  onClick={() => openProfile(idx)}
                  aria-current={!addOpen && activeIndex === idx ? 'true' : undefined}
                >
                  <span className="model-provider-item-top">
                    <strong>{providerDisplayName(profile.var_name) || `服务商 ${idx + 1}`}</strong>
                    <i className={`is-${state}`} title={state === 'error' ? '存在阻断项' : state === 'warning' ? '存在提醒' : '配置正常'} />
                  </span>
                  <span className="model-provider-base">{profile.apibase || '尚未填写 BaseURL'}</span>
                  <span className="model-provider-meta"><em>{meta?.shortLabel || protocolLabel(profile.type)}</em><b>{count} 个模型</b></span>
                </button>
              )
            })}
          </nav>

          <button type="button" className={`model-provider-add${addOpen ? ' is-active' : ''}`} onClick={openAdd}>
            <Plus size={15} /><span>新增服务商</span>
          </button>

          <footer className="model-rail-foot">
            <CheckCircle2 size={13} />
            <span>单项保存，密钥不会出现在预览中</span>
          </footer>
        </aside>

        <section className="model-editor-workspace" aria-label={addOpen ? '新增服务商' : '服务商编辑器'}>
          {addOpen && (
            <AddProfileForm
              profiles={profiles}
              addModelProfiles={addModelProfiles}
              t={t}
              onClose={() => setAddOpen(false)}
              onAdded={() => {
                setActiveIndex(profiles.length)
                setAddOpen(false)
              }}
            />
          )}

          {profiles.map((profile, idx) => {
            const key = profileKeyId(idx, profile)
            return (
              <div key={key} className="model-editor-slot" hidden={addOpen || activeIndex !== idx}>
                <ProfileCard
                  profile={profile}
                  idx={idx}
                  profileKey={key}
                  result={validation[idx]}
                  profiles={profiles}
                  patchProfile={patchProfile}
                  removeProfile={removeProfile}
                  discoverModels={discoverModels}
                  revealedKey={revealedKeys[key]}
                  revealBusy={!!revealBusy[key]}
                  onRevealKey={onRevealKey}
                  onClearRevealedKey={onClearRevealedKey}
                  onSave={saveModelProfile}
                  saveState={modelSaveStatus[key] || modelSaveStatus[idx]}
                />
              </div>
            )
          })}

          {!profiles.length && !addOpen && (
            <div className="model-empty-state">
              <Layers size={36} strokeWidth={1.2} className="model-empty-icon" />
              <strong>{importLoading ? '正在读取 mykey.py…' : '还没有服务商'}</strong>
              <span>{importLoading ? '请稍候，配置加载后会显示在这里。' : '创建第一个服务商，将同源模型集中管理。'}</span>
              {!importLoading && <Button type="primary" icon={<Plus size={15} />} onClick={openAdd}>新增服务商</Button>}
            </div>
          )}
        </section>
      </div>

      <Collapse ghost items={riskItems} className="model-risk-collapse" />

      <Drawer
        title="调整模型顺序"
        placement="right"
        width={620}
        open={orderOpen}
        onClose={closeModelOrder}
        closable={!orderSaving}
        maskClosable={!orderSaving}
        className="model-order-drawer"
        footer={(
          <div className="model-order-footer">
            <span>关闭会丢弃未保存的顺序调整</span>
            <Space>
              <Button onClick={closeModelOrder} disabled={orderSaving}>取消</Button>
              <Button type="primary" onClick={saveModelOrder} loading={orderSaving} disabled={!orderRows.length}>确认并保存</Button>
            </Space>
          </div>
        )}
      >
        <Alert
          type="info"
          showIcon
          message="列表顺序就是 --llm-no 的取值"
          description="编号从 0 开始。保存后，mykey.py 中展开的模型变量会按此顺序声明；页面中尚未保存的模型不会出现在这里。"
        />
        {orderError && <Alert type="error" showIcon message={orderError} className="model-order-error" />}
        <div className="model-order-list" role="list" aria-label="已保存模型的全局顺序">
          {orderRows.map((row, index) => (
            <div
              key={row.id}
              role="listitem"
              className={`model-order-row${dragIndex === index ? ' is-dragging' : ''}`}
              draggable={!orderSaving}
              onDragStart={event => {
                setDragIndex(index)
                event.dataTransfer.effectAllowed = 'move'
                event.dataTransfer.setData('text/plain', row.id)
              }}
              onDragOver={event => {
                event.preventDefault()
                event.dataTransfer.dropEffect = 'move'
              }}
              onDrop={event => {
                event.preventDefault()
                dropModelOrder(index)
              }}
              onDragEnd={() => setDragIndex(null)}
            >
              <GripVertical size={17} className="model-order-grip" aria-hidden="true" />
              <div className="model-order-index" aria-label={`--llm-no ${index}`}>
                <strong>{index}</strong>
                <span>--llm-no</span>
              </div>
              <div className="model-order-copy">
                <code>{row.variableName}</code>
                <strong title={row.model}>{row.model || '未填写模型 ID'}</strong>
                <span>服务商名称：{providerDisplayName(row.providerVarName) || '未命名'}</span>
              </div>
              <div className="model-order-actions">
                <Button
                  type="text"
                  size="small"
                  icon={<ArrowUp size={15} />}
                  aria-label={`上移 ${row.model || row.variableName}`}
                  title="上移"
                  disabled={orderSaving || index === 0}
                  onClick={() => moveModelOrder(index, index - 1)}
                />
                <Button
                  type="text"
                  size="small"
                  icon={<ArrowDown size={15} />}
                  aria-label={`下移 ${row.model || row.variableName}`}
                  title="下移"
                  disabled={orderSaving || index === orderRows.length - 1}
                  onClick={() => moveModelOrder(index, index + 1)}
                />
              </div>
            </div>
          ))}
        </div>
      </Drawer>

      <Drawer
        title="mykey.py 配置预览"
        placement="right"
        width={680}
        open={previewOpen}
        onClose={() => setPreviewOpen(false)}
        className="model-preview-drawer"
        extra={<Button icon={<RefreshCw size={14} />} onClick={previewModels}>刷新预览</Button>}
      >
        <Alert type="info" showIcon message="预览不会读取或显示已保存的明文密钥。" />
        <pre className="model-preview-pre">{modelPreview || (profiles.length ? '正在生成预览…' : '添加至少一个服务商后显示预览。')}</pre>
      </Drawer>
    </section>
  )
}
