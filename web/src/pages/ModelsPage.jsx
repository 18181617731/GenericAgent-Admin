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
  Network,
  Plus,
  RefreshCw,
  Settings2,
  ShieldCheck,
  Trash2,
  UploadCloud,
  X,
} from 'lucide-react'
import { useEffect, useLayoutEffect, useMemo, useRef, useState } from 'react'
import { Alert, Button, Checkbox, Collapse, Drawer, Input, Modal, Progress, Radio, Select, Space, Tag } from 'antd'
import { DndContext, PointerSensor, closestCenter, useSensor, useSensors } from '@dnd-kit/core'
import { SortableContext, useSortable, verticalListSortingStrategy } from '@dnd-kit/sortable'
import { CSS as DndCSS } from '@dnd-kit/utilities'
import { emptyProfile } from '../lib/format'
import {
  FAILOVER_VAR_PREFIX,
  failoverGroupSuffix,
  failoverGroupVarName,
  migrateFailoverGroupNames,
  nextFailoverGroupName,
  normalizeFailoverGroups,
  API_MODE_OPTIONS,
  THINKING_TYPE_OPTIONS,
  addModelConfigs,
  isModelConfigEnabled,
  modelAvailabilitySummary,
  modelProtocolFields,
  moveOrderedItem,
  orderedFailoverRows,
  orderedModelAndFailoverRows,
  orderedModelRows,
  profileModelConfigs,
  reasoningEffortOptions,
  reconcileModelProbeResults,
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
import {
  modelProbeProviderKey,
  modelProbeProviderName,
  normalizeModelProbeProviderKeys,
  resolveModelProbeTargets,
  runModelBatchProbe,
} from '../lib/modelBatchProbe'

const DEFAULT_PROTOCOL = 'native_oai'
const ORDER_REPEAT_DISTANCE_PX = 8
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

const protocolMeta = (value, t) => {
  const meta = LEGACY_PROTOCOLS.find(item => item.value === value) || OFFICIAL_PROTOCOLS[0]
  const localized = t?.models?.protocols?.[meta.value]
  return localized ? { ...meta, label: localized[0], shortLabel: localized[0], help: localized[1] || meta.help } : meta
}
const protocolLabel = (value, t) => protocolMeta(value, t)?.shortLabel || value || 'Native OAI'
const supportsModelDiscovery = value => !!protocolMeta(value)?.discover
const nextVarName = (protocol, profiles = []) => nextProviderVarName(
  protocolMeta(protocol)?.prefix || 'native_oai_config',
  profiles,
)

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

function StatusTag({ result, t }) {
  const text = t.models
  if (!result) return null
  const errors = result.errors?.length || 0
  const warnings = result.warnings?.length || 0
  if (errors) return <Tag color="error">{text.blockItems(errors)}</Tag>
  if (warnings) return <Tag color="warning">{text.reminders(warnings)}</Tag>
  return <Tag color="success">{text.valid}</Tag>
}

function optionalNumber(value) {
  if (value === '' || value === null || value === undefined) return undefined
  const parsed = Number(value)
  return Number.isNaN(parsed) ? value : parsed
}

function OptionalBoolSelect({ value, onChange, t, trueLabel, falseLabel }) {
  return (
    <Select
      value={value === true || value === false ? value : 'inherit'}
      onChange={next => onChange(next === 'inherit' ? undefined : next)}
      options={[
        { value: 'inherit', label: t.models.inherit },
        { value: true, label: trueLabel || t.enabled },
        { value: false, label: falseLabel || t.disabled },
      ]}
    />
  )
}

function ModelConfigRow({ config, index, protocol, onChange, onRemove, t }) {
  const text = t.models
  const [configOpen, setConfigOpen] = useState(false)
  const fields = modelProtocolFields(protocol)
  const enabled = isModelConfigEnabled(config)
  const availability = config.availability || 'unknown'
  const availabilityTitle = [config.availability_detail, config.availability_latency_ms ? `${config.availability_latency_ms} ms` : ''].filter(Boolean).join(' · ')
  const configSummary = [config.api_mode, config.thinking_type, config.reasoning_effort]
    .filter(Boolean)
    .join(' · ') || text.defaultParams

  return (
    <article className={`model-config-row${configOpen ? ' is-open' : ''}${enabled ? '' : ' is-disabled'}`}>
      <div className="model-config-main">
        <div className="model-config-identity">
          <span className="model-config-index" aria-hidden="true">
            {String(index + 1).padStart(2, '0')}
          </span>
          <div className="model-config-copy">
            {config.name && (
              <span className="model-config-display-name" title={config.name}>
                {config.name}
              </span>
            )}
            <span className="model-config-id" title={config.model || ''}>
              {config.model || text.unnamedModel}
            </span>
            <span className="model-config-summary">{configSummary}</span>
            {availability === 'unavailable'
              ? <Tag color="error" title={availabilityTitle}>{config.auto_disabled ? '已自动禁用' : '不可用'}</Tag>
              : availability === 'available'
                ? <Tag color={enabled ? 'success' : 'default'} title={availabilityTitle}>{enabled ? '可用' : '手动禁用'}</Tag>
                : null}
          </div>
        </div>
        <Button
          type="text"
          className="model-config-action model-config-toggle"
          onClick={() => setConfigOpen(open => !open)}
          aria-expanded={configOpen}
        >
          <span>{configOpen ? text.collapse : text.configure}</span>
          <ChevronDown size={13} className="model-config-chevron" aria-hidden="true" />
        </Button>
        <Button
          danger
          type="text"
          className="model-config-action model-config-delete"
          icon={<Trash2 size={13} />}
          onClick={onRemove}
          aria-label={text.deleteModel(config.model || index + 1)}
        >
          {t.delete}
        </Button>
      </div>

      {configOpen && (
        <div className="model-row-advanced">
          <div className="model-row-advanced-grid">
            <label className="model-field">
              <span className="model-field-label">{text.displayName || '\u663e\u793a\u540d\u79f0'}</span>
              <Input value={config.name || ''} onChange={event => onChange({ name: event.target.value })} placeholder={text.displayNamePlaceholder || '\u4f8b\u5982\uff1a\u4e3b\u6a21\u578b'} />
            </label>
            <label className="model-field">
              <span className="model-field-label">{text.stream}</span>
              <OptionalBoolSelect value={config.stream} onChange={stream => onChange({ stream })} t={t} />
            </label>
            <label className="model-field">
              <span className="model-field-label">{text.maxRetries}</span>
              <Input type="number" min={0} value={config.max_retries ?? ''} onChange={event => onChange({ max_retries: optionalNumber(event.target.value) })} placeholder={text.inherit} />
            </label>
            <label className="model-field">
              <span className="model-field-label">{text.readTimeout}</span>
              <Input type="number" min={1} value={config.read_timeout ?? ''} onChange={event => onChange({ read_timeout: optionalNumber(event.target.value) })} placeholder={text.inherit} />
            </label>
            <label className="model-field">
              <span className="model-field-label">{text.connectTimeout}</span>
              <Input type="number" min={1} value={config.connect_timeout ?? ''} onChange={event => onChange({ connect_timeout: optionalNumber(event.target.value) })} placeholder={text.inherit} />
            </label>
            {fields.userAgent && (
              <label className="model-field">
                <span className="model-field-label">User-Agent</span>
                <Input value={config.user_agent || ''} onChange={event => onChange({ user_agent: event.target.value || undefined })} placeholder={text.optional} />
              </label>
            )}
            {fields.apiMode && (
              <label className="model-field">
                <span className="model-field-label">{text.apiMode}</span>
                <Select allowClear value={config.api_mode || undefined} onChange={api_mode => onChange({ api_mode })} placeholder={text.inherit} options={API_MODE_OPTIONS} />
              </label>
            )}
            {fields.thinkingType && (
              <label className="model-field">
                <span className="model-field-label">{text.thinkingType}</span>
                <Select allowClear value={config.thinking_type || undefined} onChange={thinking_type => onChange({ thinking_type })} placeholder={text.inherit} options={THINKING_TYPE_OPTIONS} />
              </label>
            )}
            {fields.reasoningFamily && (
              <label className="model-field">
                <span className="model-field-label">{text.reasoningEffort}</span>
                <Select allowClear value={config.reasoning_effort || undefined} onChange={reasoning_effort => onChange({ reasoning_effort })} placeholder={text.inherit} options={reasoningEffortOptions(protocol)} />
              </label>
            )}
            {fields.fakeClaudeCode && (
              <label className="model-field">
                <span className="model-field-label">{text.fakeClaude}</span>
                <OptionalBoolSelect value={config.fake_cc_system_prompt} onChange={fake_cc_system_prompt => onChange({ fake_cc_system_prompt })} t={t} />
              </label>
            )}
          </div>
        </div>
      )}
    </article>
  )
}

function ModelConfigEditor({ profile, discovered = [], onChange, onDiscover, onCheck, busy, checking, disabled, availabilityResult, discoveryError = '', t }) {
  const [draft, setDraft] = useState('')
  const [discoverOpen, setDiscoverOpen] = useState(false)
  const text = t.models
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
        <Space size={8}>
          <Button onClick={onCheck} loading={checking} disabled={disabled || busy || checking} icon={<ShieldCheck size={14} />}>
            检测当前服务商
          </Button>
          <Button onClick={openDiscover} disabled={disabled || checking} icon={<RefreshCw size={14} />}>
            获取模型
          </Button>
        </Space>
      </div>

      {availabilityResult && (
        <Alert
          type={availabilityResult.type}
          showIcon
          message={availabilityResult.message}
          description={availabilityResult.description}
          className="model-availability-alert"
        />
      )}

      <div className="model-config-table">
        <div className="model-config-table-head" aria-hidden="true">
          <span>{text.modelId}</span>
          <span>{text.configuration}</span>
          <span>{t.delete}</span>
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
              t={t}
            />
          )) : <div className="model-config-empty">{text.noModels}</div>}
        </div>
      </div>

      <div className="model-quick-add">
        <Input
          value={draft}
          onChange={event => setDraft(event.target.value)}
          onPressEnter={addDraft}
          placeholder={text.manualModel}
          aria-label={text.manualModel}
        />
        <Button icon={<Plus size={14} />} onClick={addDraft} disabled={!draft.trim()}>{text.addModel}</Button>
      </div>

      <Modal
        className="model-discover-modal"
        title={text.fetchModels}
        open={discoverOpen}
        onCancel={() => setDiscoverOpen(false)}
        footer={null}
        width={620}
        destroyOnHidden
      >
        <div className="model-discover-modal-head">
          <span>{busy ? text.fetchingModels : discoveryError ? text.fetchFailed : text.discovered(candidates.length)}</span>
          <Button size="small" type="primary" onClick={() => addCandidates(candidates)} disabled={busy || !!discoveryError || !candidates.length}>
            {text.addAll}
          </Button>
        </div>
        {busy ? (
          <div className="model-discover-modal-state" role="status"><RefreshCw size={18} className="is-spinning" />{text.fetching}</div>
        ) : discoveryError ? (
          <Alert
            type="error"
            showIcon
            message={text.cannotFetch}
            description={discoveryError}
            action={<Button size="small" onClick={onDiscover}>{t.retry}</Button>}
          />
        ) : candidates.length > 0 ? (
          <div className="model-candidate-list">
            {candidates.map(model => (
              <button key={model} type="button" className="model-candidate-item" onClick={() => addCandidates([model])} aria-label={text.addModelAria(model)}>
                <span title={model}>{model}</span>
                <Plus size={14} />
              </button>
            ))}
          </div>
        ) : (
          <div className="model-discover-modal-state" role="status">
            <span>{text.noNewModels}</span>
            <Button size="small" onClick={onDiscover}>{text.refetch}</Button>
          </div>
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
  probeModels,
  revealedKey,
  revealBusy,
  onRevealKey,
  onClearRevealedKey,
  onSave,
  saveState,
  t,
}) {
  const text = t.models
  const [discoverBusy, setDiscoverBusy] = useState(false)
  const [discoverError, setDiscoverError] = useState('')
  const [discovered, setDiscovered] = useState([])
  const [availabilityBusy, setAvailabilityBusy] = useState(false)
  const [availabilityResult, setAvailabilityResult] = useState(null)
  const [dirty, setDirty] = useState(false)
  const [nameDirty, setNameDirty] = useState(false)
  const selectedModels = profileModels(p)
  const availability = modelAvailabilitySummary(p)
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

  const saveName = async value => {
    if (!nameDirty) return
    const name = value.trim()
    const ok = await onSave?.(idx, profileKey, { ...p, name })
    if (ok !== false) {
      setDirty(false)
      setNameDirty(false)
    }
  }

  const requestProviderModels = async () => {
    const configuredKey = String(p.apikey || '').trim()
    const response = await discoverModels({
      protocol: p.type || DEFAULT_PROTOCOL,
      baseUrl: p.apibase,
      apiKey: configuredKey && !isMaskedSecret(configuredKey) ? configuredKey : undefined,
      varName: p.var_name,
    })
    return uniqueModels(response?.models || [])
  }

  const requestProviderProbe = async () => {
    const configuredKey = String(p.apikey || '').trim()
    const configs = profileModelConfigs(p)
    return probeModels({
      protocol: p.type || DEFAULT_PROTOCOL,
      baseUrl: p.apibase,
      apiKey: configuredKey && !isMaskedSecret(configuredKey) ? configuredKey : undefined,
      varName: p.var_name,
      models: configs.map(config => config.model),
      modelOptions: Object.fromEntries(configs.map(config => [config.model, { api_mode: config.api_mode, user_agent: config.user_agent }])),
    })
  }

  const discover = async () => {
    if (!supportsModelDiscovery(p.type || DEFAULT_PROTOCOL)) return
    setDiscoverBusy(true)
    setDiscoverError('')
    try {
      setDiscovered(await requestProviderModels())
    } catch (error) {
      setDiscoverError(String(error?.message || error))
    } finally {
      setDiscoverBusy(false)
    }
  }

  const checkAvailability = async () => {
    if (!supportsModelDiscovery(p.type || DEFAULT_PROTOCOL)) return
    setAvailabilityBusy(true)
    setAvailabilityResult(null)
    try {
      const response = await requestProviderProbe()
      const probeResults = Array.isArray(response?.results) ? response.results : []
      if (!probeResults.length) throw new Error('未获得任何真实对话检测结果，未修改当前配置。')
      setDiscovered(probeResults.filter(item => item.available).map(item => ({ id: item.id })))
      const result = reconcileModelProbeResults(p, probeResults, response.checked_at)
      patchProfile(idx, result.profile)
      const saved = await onSave?.(idx, profileKey, result.profile)
      if (saved === false) {
        patchProfile(idx, p)
        throw new Error('模型状态保存失败，已恢复检测前配置。')
      }
      const { available, unavailable, disabled, restored, checkedAt } = result.summary
      const failures = probeResults.filter(item => !item.available).slice(0, 3).map(item => `${item.id}：${item.detail}`).join('；')
      setAvailabilityResult({
        type: unavailable ? 'warning' : 'success',
        message: `真实对话检测完成：${available} 个可用，${unavailable} 个不可用`,
        description: `自动禁用 ${disabled} 个，自动恢复 ${restored} 个。${failures ? `失败摘要：${failures}。` : ''}检测时间 ${new Date(checkedAt).toLocaleString()}。`,
      })
    } catch (error) {
      setAvailabilityResult({ type: 'error', message: '检测失败，未修改模型状态', description: String(error?.message || error) })
    } finally {
      setAvailabilityBusy(false)
    }
  }

  return (
    <article className={`model-source-card${dirty ? ' is-dirty' : ''}${result?.errors?.length ? ' has-error' : ''}`}>
      <header className="model-source-head">
        <div className="model-source-identity">
          <span className="model-source-index">{String(idx + 1).padStart(2, '0')}</span>
          <div>
            <div className="model-source-title-row">
              <strong>{p.name || providerDisplayName(p.var_name) || `服务商 ${idx + 1}`}</strong>
              <Tag color={meta.color}>{protocolLabel(p.type || DEFAULT_PROTOCOL)}</Tag>
              <span className="model-count-badge">{availability.enabled}/{selectedModels.length} 个启用</span>
            </div>
            <span className="model-source-base">{p.apibase || text.baseMissing}</span>
          </div>
        </div>
        <Space size={8} className="model-source-actions">
          <StatusTag result={result} t={t} />
          {dirty && <span className="model-save-state is-dirty">{text.unsaved}</span>}
          {!dirty && saveOk && <span className="model-save-state is-saved">{text.saved}</span>}
          {saveError && <span className="model-save-state is-error">{text.saveFailed}</span>}
          <Button
            type="primary"
            icon={<CheckCircle2 size={14} />}
            loading={saveBusy}
            disabled={saveBusy || result?.errors?.length > 0}
            onClick={save}
          >
            {t.save}
          </Button>
          <Button danger type="text" icon={<Trash2 size={15} />} onClick={() => removeProfile(idx)} title={text.deleteProviderTitle} />
        </Space>
      </header>

      <div className="model-source-body">
        <div className="model-workflow-heading model-workflow-heading--fields">
          <strong><span>1</span> {text.connectStep}</strong>
          <small>{text.connectHelp}</small>
        </div>
        <div className="model-primary-grid">
          <label className="model-field">
            <span className="model-field-label">显示名称</span>
            <Input
              aria-label="模型名称"
              value={p.name || ''}
              onChange={event => {
                setNameDirty(true)
                patch({ name: event.target.value })
              }}
              onBlur={event => saveName(event.target.value)}
              onPressEnter={event => event.currentTarget.blur()}
              placeholder="例如：主模型"
            />
            <small>可使用中文；失去焦点后自动保存。</small>
          </label>
          <label className="model-field model-field--provider">
            <span className="model-field-label">{text.name}</span>
            <Input
              value={providerDisplayName(p.var_name)}
              onChange={event => patch({
                var_name: providerVarNameFromDisplayName(
                  event.target.value,
                  meta.prefix,
                  p.var_name,
                ),
              })}
              placeholder={text.nameExample}
            />
            <small>{text.nameHelp}</small>
          </label>
          <label className="model-field">
            <span className="model-field-label">{text.protocol}</span>
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
              options={OFFICIAL_PROTOCOLS.map(item => protocolMeta(item.value, t))}
            />
          </label>
          <label className="model-field model-field--base">
            <span className="model-field-label">BaseURL</span>
            <Input value={p.apibase || ''} onChange={event => patch({ apibase: event.target.value })} placeholder="https://api.example.com/v1" />
          </label>
          <label className="model-field model-field--key">
            <span className="model-field-label">API Key <em>{revealed ? text.tempShown : text.hiddenByDefault}</em></span>
            <Input
              type={revealed ? 'text' : 'password'}
              value={shownApiKey}
              onChange={event => {
                onClearRevealedKey?.(idx, p, profileKey)
                patch({ apikey: event.target.value })
              }}
              placeholder={text.keyPlaceholder}
              addonAfter={revealed ? (
                <Space size={2}>
                  <Button size="small" type="text" icon={<EyeOff size={14} />} loading={revealBusy} onClick={() => onRevealKey?.(idx, p, false, profileKey)}>{t.hide}</Button>
                  <Button size="small" type="text" icon={<RefreshCw size={13} />} loading={revealBusy} onClick={() => onRevealKey?.(idx, p, true, profileKey)} title={text.reread} aria-label={`${text.reread} API Key`} />
                </Space>
              ) : (
                <Button size="small" type="text" icon={<Eye size={14} />} loading={revealBusy} onClick={() => onRevealKey?.(idx, p, false, profileKey)}>{t.show}</Button>
              )}
            />
          </label>
        </div>

        <ModelConfigEditor
          profile={p}
          discovered={discovered}
          onChange={patch}
          onDiscover={discover}
          onCheck={checkAvailability}
          busy={discoverBusy}
          checking={availabilityBusy}
          availabilityResult={availabilityResult}
          disabled={discoverBusy || !p.apibase || !supportsModelDiscovery(p.type || DEFAULT_PROTOCOL)}
          discoveryError={discoverError}
          t={t}
        />

        <div className="model-workflow-heading model-workflow-heading--save">
          <strong><span>3</span> {text.saveStep}</strong>
          <small>{result?.errors?.length ? text.fixBlocks : dirty ? text.dirtyHelp : text.savedHelp}</small>
        </div>
        {saveBusy && <Alert type="info" showIcon message={text.savingProvider} description={text.savingDescription} className="model-inline-alert" />}
        {saveOk && !dirty && <Alert type="success" showIcon message={text.savedMykey} description={text.savedDescription} className="model-inline-alert" />}
        {saveError && (
          <Alert
            type="error"
            showIcon
            message={text.providerSaveFailed}
            description={saveState?.error || text.unknownError}
            action={<Button size="small" onClick={save} disabled={!!result?.errors?.length} loading={saveBusy}>{text.retrySave}</Button>}
            className="model-inline-alert"
          />
        )}
        {result?.errors?.length > 0 && (
          <Alert
            type="error"
            showIcon
            message={text.cannotSave}
            description={<ul>{result.errors.map(key => <li key={key}>{text.errors[key] || key}</li>)}</ul>}
            className="model-inline-alert"
          />
        )}
        {result?.warnings?.length > 0 && (
          <Alert
            type="warning"
            showIcon
            message={text.beforeSave}
            description={<ul>{result.warnings.map(key => <li key={key}>{text.errors[key] || key}</li>)}</ul>}
            className="model-inline-alert"
          />
        )}
      </div>
    </article>
  )
}

function AddProfileForm({ profiles, addModelProfiles, t, onClose, onAdded }) {
  const text = t.models
  const [form, setForm] = useState(() => ({
    protocol: DEFAULT_PROTOCOL,
    providerVar: nextVarName(DEFAULT_PROTOCOL, profiles),
    displayName: '',
    baseUrl: '',
    apiKey: '',
  }))
  const [adding, setAdding] = useState(false)
  const [error, setError] = useState('')
  const meta = protocolMeta(form.protocol, t)
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
      setError(text.nameRequired)
      return
    }
    if (!form.baseUrl.trim()) {
      setError(text.baseRequired)
      return
    }
    setAdding(true)
    setError('')
    try {
      const profile = {
        ...emptyProfile(profiles.length, form.protocol),
        var_name: varName,
        type: form.protocol,
        name: form.displayName.trim(),
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
        displayName: '',
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
          <strong>{text.addProvider}</strong>
          <span>{text.addProviderHelp}</span>
        </div>
        <Button type="text" icon={<X size={16} />} onClick={onClose} aria-label={text.closeAdd} />
      </header>
      <div className="model-add-grid">
        <label className="model-field">
          <span className="model-field-label">显示名称</span>
          <Input
            aria-label="新增模型名称"
            value={form.displayName}
            onChange={event => patchForm({ displayName: event.target.value })}
            placeholder="例如：主模型"
          />
          <small>可选，支持中文。</small>
        </label>
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
            placeholder={text.nameExample}
          />
          <small>{text.nameHelp}</small>
        </label>
        <label className="model-field">
          <span className="model-field-label">{text.protocol}</span>
          <Select value={form.protocol} onChange={changeProtocol} options={OFFICIAL_PROTOCOLS.map(item => protocolMeta(item.value, t))} />
          <small>{meta.help}</small>
        </label>
        <label className="model-field model-field--base">
          <span className="model-field-label">BaseURL</span>
          <Input value={form.baseUrl} onChange={event => patchForm({ baseUrl: event.target.value })} placeholder="https://api.example.com/v1" />
        </label>
        <label className="model-field model-field--key">
          <span className="model-field-label">API Key <em>{text.optionalKey}</em></span>
          <Input type="password" value={form.apiKey} onChange={event => patchForm({ apiKey: event.target.value })} placeholder={t.hints?.savedSecret || '填写密钥'} />
        </label>
      </div>
      {error && <Alert className="model-inline-alert" type="error" showIcon message={error} />}
      <footer className="model-add-footer">
        <span>{text.addProviderFooter}</span>
        <Space>
          <Button onClick={onClose}>{t.cancel}</Button>
          <Button type="primary" icon={<Plus size={14} />} loading={adding} onClick={add}>{text.addAndSave}</Button>
        </Space>
      </footer>
    </section>
  )
}

function SortableOrderRow({ row, index, orderRows, orderSaving, moveModelOrder, text, providerDisplayName }) {
  const { attributes, listeners, setNodeRef, transform, transition, isDragging } = useSortable({ id: row.id })
  const style = {
    transform: DndCSS.Transform.toString(transform),
    transition,
    zIndex: isDragging ? 100 : undefined,
    opacity: isDragging ? 0.85 : 1,
    boxShadow: isDragging ? '0 8px 24px rgba(0,0,0,0.18)' : undefined,
    position: 'relative',
  }
  return (
    <div
      ref={setNodeRef}
      style={style}
      data-order-id={row.id}
      role="listitem"
      className={`model-order-row${isDragging ? ' is-dragging' : ''}${row.type === 'failover' ? ' is-failover' : ''}`}
    >
      <span
        {...attributes}
        {...listeners}
        className="model-order-grip"
        style={{ cursor: isDragging ? 'grabbing' : 'grab', display: 'flex', alignItems: 'center', touchAction: 'none' }}
        aria-label="drag to reorder"
      >
        <GripVertical size={17} aria-hidden="true" />
      </span>
      <div className="model-order-index" aria-label={`--llm-no ${index}`}>
        <strong>{index}</strong>
        <span>--llm-no</span>
      </div>
      {row.type === 'failover' ? (
        <>
          <div className="model-order-copy model-order-failover">
            <div className="model-failover-name">
              <code>{row.varName}</code>
              <strong>
                <Network size={14} style={{ marginRight: '4px', verticalAlign: 'text-bottom' }} />
                {text.failoverGroup || 'Failover Group'}
              </strong>
              <span>{row.members?.length || 0} {text.failoverMembers || 'members'}</span>
            </div>
            {row.members && row.members.length > 0 && (
              <div className="model-failover-members">
                {row.members.map((member, memberIndex) => (
                  <div key={memberIndex} className="model-failover-member">
                    <span className="model-failover-member-index">{memberIndex + 1}.</span>
                    <code>{member.provider_var_name || member.model || JSON.stringify(member)}</code>
                  </div>
                ))}
              </div>
            )}
          </div>
        </>
      ) : (
        <div className="model-order-copy">
          <code>{row.variableName}</code>
          <strong title={row.model}>{row.model || text.missingModelId}</strong>
          <span>{text.providerName}: {providerDisplayName(row.providerVarName) || text.unnamed}</span>
        </div>
      )}
      <div className="model-order-actions">
        <Button
          type="text"
          size="small"
          icon={<ArrowUp size={15} />}
          aria-label={`${text.moveUp} ${row.type === 'failover' ? row.varName : (row.model || row.variableName)}`}
          title={text.moveUp}
          disabled={orderSaving || index === 0}
          onClick={() => moveModelOrder(index, index - 1)}
        />
        <Button
          type="text"
          size="small"
          icon={<ArrowDown size={15} />}
          aria-label={`${text.moveDown} ${row.type === 'failover' ? row.varName : (row.model || row.variableName)}`}
          title={text.moveDown}
          disabled={orderSaving || index === orderRows.length - 1}
          onClick={() => moveModelOrder(index, index + 1)}
        />
      </div>
    </div>
  )
}

function SortableFailoverMemberRow({ memberKey, member, memberIndex, groupIndex, groupLength, failoverSaving, candidate, moveFailoverMember, toggleFailoverMember, patchFailoverGroup, group, text, providerDisplayName }) {
  const { attributes, listeners, setNodeRef, transform, transition, isDragging } = useSortable({ id: memberKey })
  const style = {
    transform: DndCSS.Transform.toString(transform),
    transition,
    zIndex: isDragging ? 100 : undefined,
    opacity: isDragging ? 0.85 : 1,
  }
  return (
    <div ref={setNodeRef} style={style} role="listitem" className={`model-failover-priority-row${isDragging ? ' is-dragging' : ''}`}>
      <span
        {...attributes}
        {...listeners}
        style={{ cursor: isDragging ? 'grabbing' : 'grab', display: 'flex', alignItems: 'center', touchAction: 'none' }}
        aria-label="drag to reorder"
      >
        <GripVertical size={15} aria-hidden="true" />
      </span>
      <span className="model-failover-priority-index">{memberIndex + 1}</span>
      <span className="model-failover-priority-copy">
        <strong title={member.model}>{member.model}</strong>
        <small><b>{providerDisplayName(member.provider_var_name) || member.provider_var_name}</b>{candidate?.protocol ? ` · ${candidate.protocol}` : ''}</small>
      </span>
      {!candidate && <span>{text.failoverMissingMember || 'Missing'}</span>}
      <Space size={0}>
        <Button type="text" size="small" icon={<ArrowUp size={13} />} disabled={failoverSaving || memberIndex === 0} onClick={() => moveFailoverMember(groupIndex, memberIndex, memberIndex - 1)} />
        <Button type="text" size="small" icon={<ArrowDown size={13} />} disabled={failoverSaving || memberIndex === groupLength - 1} onClick={() => moveFailoverMember(groupIndex, memberIndex, memberIndex + 1)} />
        <Button danger type="text" size="small" icon={<X size={13} />} disabled={failoverSaving} onClick={() => candidate ? toggleFailoverMember(groupIndex, candidate) : patchFailoverGroup(groupIndex, { members: group.members.filter((_, i) => i !== memberIndex) })} />
      </Space>
    </div>
  )
}

function FailoverCandidatePicker({ candidates = [], selectedKeys, selectedFamilies, failoverSaving, onToggle, text, providerDisplayName, memberKey }) {
  const providerGroups = useMemo(() => {
    const grouped = new Map()
    candidates.forEach(candidate => {
      const providerVarName = String(candidate.providerVarName || '').trim()
      if (!grouped.has(providerVarName)) grouped.set(providerVarName, [])
      grouped.get(providerVarName).push(candidate)
    })
    return Array.from(grouped, ([value, models]) => ({
      value,
      label: providerDisplayName(value) || value || text.unnamed,
      models,
    }))
  }, [candidates, providerDisplayName, text.unnamed])
  const [previewProvider, setPreviewProvider] = useState('')
  useEffect(() => {
    setPreviewProvider(current => providerGroups.some(group => group.value === current) ? current : providerGroups[0]?.value || '')
  }, [providerGroups])
  const activeGroup = providerGroups.find(group => group.value === previewProvider) || providerGroups[0]

  if (!candidates.length) return <div className="model-failover-empty">{text.failoverNoCandidates}</div>

  return (
    <div className="model-failover-cascade" aria-label={text.failoverCandidates}>
      <div className="model-failover-cascade-providers" role="listbox" aria-label={text.providerName || 'Provider'}>
        {providerGroups.map(group => (
          <button
            type="button"
            key={group.value || 'unnamed-provider'}
            role="option"
            aria-selected={group.value === activeGroup?.value}
            className={group.value === activeGroup?.value ? 'is-active' : ''}
            onMouseEnter={() => setPreviewProvider(group.value)}
            onFocus={() => setPreviewProvider(group.value)}
            onClick={() => setPreviewProvider(group.value)}
          >
            <span title={group.label}>{group.label}</span>
            <small>{group.models.length}</small>
            <b aria-hidden="true">›</b>
          </button>
        ))}
      </div>
      <div className="model-failover-cascade-models" role="listbox" aria-label={activeGroup ? `${activeGroup.label} ${text.failoverCandidates}` : text.failoverCandidates}>
        <div className="model-failover-cascade-heading">
          <strong title={activeGroup?.label}>{activeGroup?.label || text.unnamed}</strong>
          <span>{activeGroup?.models.length || 0} {text.models?.(activeGroup?.models.length || 0) || ''}</span>
        </div>
        {activeGroup?.models.map(candidate => {
          const key = memberKey({ provider_var_name: candidate.providerVarName, model: candidate.model })
          const selected = selectedKeys.has(key)
          const locked = selectedFamilies.size > 0 && !selectedFamilies.has(candidate.family)
          const providerLabel = providerDisplayName(candidate.providerVarName) || candidate.providerVarName || text.unnamed
          const modelLabel = candidate.model || text.missingModelId
          return (
            <button
              type="button"
              key={candidate.id}
              role="option"
              aria-selected={selected}
              aria-pressed={selected}
              disabled={failoverSaving || (locked && !selected)}
              className={`model-failover-cascade-model${selected ? ' is-selected' : ''}`}
              title={`${providerLabel} · ${modelLabel} · ${candidate.protocol}`}
              onClick={() => onToggle(candidate)}
            >
              <span className="model-failover-check">{selected ? <CheckCircle2 size={15} /> : null}</span>
              <span className="model-failover-cascade-model-copy">
                <strong title={modelLabel}>{modelLabel}</strong>
                <small>{candidate.protocol}</small>
              </span>
              {locked && !selected && <em>{text.failoverLocked || 'Locked'}</em>}
            </button>
          )
        })}
      </div>
    </div>
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
  onSaveProviderOrder,
  failoverGroups = [],
  onSaveFailoverGroups,
  discoverModels,
  probeModels,
  modelProbeProviders = [],
  onSaveModelProbeProviders,
  onSaveModelProfiles,
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
  const text = t.models
  const [addOpen, setAddOpen] = useState(false)
  const [previewOpen, setPreviewOpen] = useState(false)
  const [activeIndex, setActiveIndex] = useState(0)
  const [orderOpen, setOrderOpen] = useState(false)
  const [orderRows, setOrderRows] = useState([])
  const [orderSaving, setOrderSaving] = useState(false)
  const [orderError, setOrderError] = useState('')
  const [failoverGroupExpanded, setFailoverGroupExpanded] = useState(new Set())
  const [failoverOpen, setFailoverOpen] = useState(false)
  const [failoverDrafts, setFailoverDrafts] = useState([])
  const [failoverSaving, setFailoverSaving] = useState(false)
  const [failoverError, setFailoverError] = useState('')
  const failoverGroupKeySeedRef = useRef(0)
  const [dragIndex, setDragIndex] = useState(null)
  const [repeatOrderRowId, setRepeatOrderRowId] = useState('')
  const orderMoveSessionRef = useRef(null)
  const [providerHoldIndex, setProviderHoldIndex] = useState(null)
  const [providerDrag, setProviderDrag] = useState(null)
  const [providerOrderError, setProviderOrderError] = useState('')
  const providerNavRef = useRef(null)
  const providerInteractionRef = useRef(null)
  const providerHoldTimerRef = useRef(null)
  const providerOrderBusyRef = useRef(false)
  const providerProfilesRef = useRef(profiles)
  const saveProviderOrderRef = useRef(onSaveProviderOrder)
  const suppressProviderClickUntilRef = useRef(0)
  const providerMotionKeysRef = useRef(new WeakMap())
  const providerMotionKeySeedRef = useRef(0)
  const providerFlipRectsRef = useRef(null)
  const [batchProbeBusy, setBatchProbeBusy] = useState(false)
  const [batchProbeProgress, setBatchProbeProgress] = useState(null)
  const [batchProbeResult, setBatchProbeResult] = useState(null)
  const [probeScopeOpen, setProbeScopeOpen] = useState(false)
  const [probeScopeMode, setProbeScopeMode] = useState('all')
  const [probeScopeKeys, setProbeScopeKeys] = useState([])
  const [probeScopeError, setProbeScopeError] = useState('')
  const [probeScopeSaving, setProbeScopeSaving] = useState(false)
  const [effectiveProbeProviders, setEffectiveProbeProviders] = useState(() => normalizeModelProbeProviderKeys(modelProbeProviders))
  const sensors = useSensors(useSensor(PointerSensor, { activationConstraint: { distance: 5 } }))
  const providerMotionKey = profile => {
    if (profile?.client_id) return `client:${profile.client_id}`
    if (!profile || typeof profile !== 'object') return `provider:${String(profile)}`
    if (!providerMotionKeysRef.current.has(profile)) {
      providerMotionKeySeedRef.current += 1
      providerMotionKeysRef.current.set(profile, `local:${providerMotionKeySeedRef.current}`)
    }
    return providerMotionKeysRef.current.get(profile)
  }
  providerProfilesRef.current = profiles
  saveProviderOrderRef.current = onSaveProviderOrder
  const validation = validateModelProfiles(profiles)
  const summary = modelValidationSummary(validation)
  const risk = modelRiskCatalog(riskCatalog, riskCatalogError)
  const hasErrors = summary.errors > 0
  const availabilityTotals = profiles.reduce((total, profile) => {
    const state = modelAvailabilitySummary(profile)
    return { total: total.total + state.total, enabled: total.enabled + state.enabled, disabled: total.disabled + state.disabled }
  }, { total: 0, enabled: 0, disabled: 0 })
  const totalModels = availabilityTotals.total
  const incomingProbeProviderSignature = normalizeModelProbeProviderKeys(modelProbeProviders).join('\n')
  const configuredProbeTargetCount = resolveModelProbeTargets(profiles, effectiveProbeProviders).length
  const probeScopeLabel = !profiles.length
    ? '加载中'
    : effectiveProbeProviders.length
      ? `已选 ${configuredProbeTargetCount}/${profiles.length}`
      : `全部 ${profiles.length}`
  const profileKeyId = (idx, profile) => getProfileKey?.(idx, profile)
    || profile?.client_id
    || `${profile?.var_name || nextVarName(profile?.type || DEFAULT_PROTOCOL, profiles)}:${profile?.type || DEFAULT_PROTOCOL}:${profile?.apibase || ''}:${idx}`

  useEffect(() => {
    setActiveIndex(current => Math.min(Math.max(current, 0), Math.max(profiles.length - 1, 0)))
  }, [profiles.length])

  useLayoutEffect(() => {
    const previousRects = providerFlipRectsRef.current
    providerFlipRectsRef.current = null
    if (!previousRects || window.matchMedia?.('(prefers-reduced-motion: reduce)').matches) return
    const entries = Array.from(providerNavRef.current?.querySelectorAll('[data-provider-motion-key]') || [])
    entries.forEach(entry => {
      const key = entry.dataset.providerMotionKey
      if (key === providerInteractionRef.current?.dragKey) return
      const previous = previousRects.get(key)
      if (!previous) return
      const current = entry.getBoundingClientRect()
      const deltaX = previous.left - current.left
      const deltaY = previous.top - current.top
      if (Math.abs(deltaX) < 0.5 && Math.abs(deltaY) < 0.5) return
      entry.getAnimations?.().forEach(animation => animation.cancel())
      entry.animate(
        [
          { transform: `translate3d(${deltaX}px, ${deltaY}px, 0)` },
          { transform: 'translate3d(0, 0, 0)' },
        ],
        { duration: 240, easing: 'cubic-bezier(.2,.8,.2,1)' },
      )
    })
  }, [profiles])

  useEffect(() => {
    setEffectiveProbeProviders(incomingProbeProviderSignature ? incomingProbeProviderSignature.split('\n') : [])
  }, [incomingProbeProviderSignature])

  const removeProfile = async idx => {
    const profile = profiles[idx]
    const name = profile?.name || profile?.var_name || `服务商 ${idx + 1}`
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

  const clearProviderHold = () => {
    if (providerHoldTimerRef.current) {
      window.clearTimeout(providerHoldTimerRef.current)
      providerHoldTimerRef.current = null
    }
    setProviderHoldIndex(null)
  }

  const releaseProviderPointer = interaction => {
    const target = interaction?.captureTarget
    const pointerId = interaction?.pointerId
    if (target?.hasPointerCapture?.(pointerId)) target.releasePointerCapture(pointerId)
  }

  const finishProviderDrag = async (interaction = providerInteractionRef.current) => {
    const wasActive = Boolean(interaction?.active)
    clearProviderHold()
    setProviderDrag(null)
    providerInteractionRef.current = null
    releaseProviderPointer(interaction)
    if (!wasActive) return
    suppressProviderClickUntilRef.current = Date.now() + 300
    if (interaction.fromIndex === interaction.currentIndex || providerOrderBusyRef.current) return
    const orderedProfiles = providerProfilesRef.current
    if (!saveProviderOrderRef.current) {
      setProviderOrderError('当前页面未提供服务商顺序保存能力，请刷新后重试。')
      return
    }
    providerOrderBusyRef.current = true
    setProviderOrderError('')
    try {
      const ok = await saveProviderOrderRef.current(orderedProfiles)
      if (!ok) setProviderOrderError('服务商顺序保存失败，当前排序草稿已保留，请检查页面提示后重试。')
    } catch (error) {
      setProviderOrderError(error?.message || '服务商顺序保存失败，当前排序草稿已保留。')
    } finally {
      providerOrderBusyRef.current = false
    }
  }

  const cancelProviderDrag = (event, suppressClick = false) => {
    const interaction = providerInteractionRef.current
    clearProviderHold()
    setProviderDrag(null)
    providerInteractionRef.current = null
    releaseProviderPointer(interaction)
    if (suppressClick && interaction) suppressProviderClickUntilRef.current = Date.now() + 300
  }

  const moveProviderPreview = (clientY, interaction = providerInteractionRef.current) => {
    if (!interaction?.active) return
    const items = Array.from(providerNavRef.current?.querySelectorAll('[data-provider-index]') || [])
    const overIndex = items.findIndex(item => {
      const box = item.getBoundingClientRect()
      return clientY >= box.top && clientY <= box.bottom
    })
    const previousIndex = interaction.currentIndex
    if (overIndex < 0 || overIndex === previousIndex) return
    providerFlipRectsRef.current = new Map(items.map(item => [
      item.dataset.providerMotionKey,
      item.getBoundingClientRect(),
    ]))
    interaction.currentIndex = overIndex
    setProfiles(current => {
      const next = moveOrderedItem(current, previousIndex, overIndex)
      interaction.previewProfiles = next
      providerProfilesRef.current = next
      return next
    })
    setActiveIndex(current => {
      if (current === previousIndex) return overIndex
      if (previousIndex < overIndex && current > previousIndex && current <= overIndex) return current - 1
      if (previousIndex > overIndex && current >= overIndex && current < previousIndex) return current + 1
      return current
    })
    setProviderDrag(current => ({ ...current, index: overIndex }))
    setProviderOrderError('')
  }

  const startProviderHold = (idx, event) => {
    if (providerOrderBusyRef.current || (event.pointerType === 'mouse' && event.button !== 0)) return
    clearProviderHold()
    const captureTarget = event.currentTarget
    try {
      captureTarget.setPointerCapture?.(event.pointerId)
    } catch {
      // Pointer capture may be unavailable in synthetic/browser compatibility events.
    }
    providerInteractionRef.current = {
      active: false,
      captureTarget,
      currentIndex: idx,
      fromIndex: idx,
      pointerId: event.pointerId,
      startX: event.clientX,
      startY: event.clientY,
    }
    setProviderHoldIndex(idx)
    providerHoldTimerRef.current = window.setTimeout(() => {
      providerHoldTimerRef.current = null
      const interaction = providerInteractionRef.current
      if (!interaction || interaction.fromIndex !== idx) return
      interaction.active = true
      const rect = interaction.captureTarget.getBoundingClientRect()
      interaction.dragKey = providerMotionKey(providerProfilesRef.current[interaction.currentIndex])
      interaction.grabX = interaction.startX - rect.left
      interaction.grabY = interaction.startY - rect.top
      interaction.width = rect.width
      interaction.height = rect.height
      setProviderHoldIndex(null)
      setProviderDrag({
        index: interaction.currentIndex,
        key: interaction.dragKey,
        left: rect.left,
        top: rect.top,
        width: rect.width,
        height: rect.height,
      })
      setProviderOrderError('')
    }, 350)
  }

  const moveProviderHold = event => {
    const interaction = providerInteractionRef.current
    if (!interaction || event.pointerId !== interaction.pointerId) return
    const deltaX = Math.abs(event.clientX - interaction.startX)
    const deltaY = Math.abs(event.clientY - interaction.startY)
    if (!interaction.active && Math.max(deltaX, deltaY) > 8) {
      cancelProviderDrag(event, true)
      return
    }
    if (!interaction.active) return
    event.preventDefault?.()
    setProviderDrag(current => ({
      ...current,
      left: event.clientX - interaction.grabX,
      top: event.clientY - interaction.grabY,
    }))
    moveProviderPreview(event.clientY, interaction)
  }

  const endProviderHold = event => {
    const interaction = providerInteractionRef.current
    if (!interaction || event.pointerId !== interaction.pointerId) return
    if (interaction.active) event.preventDefault?.()
    void finishProviderDrag(interaction)
  }

  const openAdd = () => setAddOpen(true)

  useEffect(() => () => {
    if (providerHoldTimerRef.current) window.clearTimeout(providerHoldTimerRef.current)
    providerInteractionRef.current = null
  }, [])

  const persistedOrderCount = orderedModelRows(persistedProfiles).length
  const resetOrderMoveSession = () => {
    orderMoveSessionRef.current = null
    setRepeatOrderRowId('')
  }
  const openModelOrder = () => {
    setOrderRows(orderedModelAndFailoverRows(persistedProfiles, failoverGroups))
    setOrderError('')
    setDragIndex(null)
    resetOrderMoveSession()
    setOrderOpen(true)
  }
  const closeModelOrder = () => {
    if (orderSaving) return
    setOrderOpen(false)
    setOrderRows([])
    setOrderError('')
    setDragIndex(null)
    resetOrderMoveSession()
  }
  const moveModelOrder = (rowId, direction, event) => {
    const isPointerClick = Number(event?.detail) > 0
    const pointer = isPointerClick
      ? { x: Number(event.clientX) || 0, y: Number(event.clientY) || 0 }
      : null
    const session = orderMoveSessionRef.current
    const repeatsAtSamePosition = pointer
      && session
      && session.direction === direction
      && Math.hypot(pointer.x - session.x, pointer.y - session.y) <= ORDER_REPEAT_DISTANCE_PX
      && orderRows.some(row => row.id === session.rowId)
    const targetRowId = repeatsAtSamePosition ? session.rowId : rowId

    setOrderRows(current => {
      const fromIndex = current.findIndex(row => row.id === targetRowId)
      return moveOrderedItem(current, fromIndex, fromIndex + direction)
    })
    orderMoveSessionRef.current = pointer
      ? { rowId: targetRowId, direction, ...pointer }
      : null
    setRepeatOrderRowId(targetRowId)
    setOrderError('')
  }
  const dropModelOrder = toIndex => {
    if (Number.isInteger(dragIndex)) {
      setOrderRows(current => moveOrderedItem(current, dragIndex, toIndex))
      setOrderError('')
    }
    setDragIndex(null)
    resetOrderMoveSession()
  }
  const saveModelOrder = async () => {
    if (!onSaveModelOrder) {
      setOrderError(text.orderMissing)
      return
    }
    setOrderSaving(true)
    setOrderError('')
    try {
      const ok = await onSaveModelOrder(orderRows)
      if (!ok) {
        setOrderError(text.orderSaveFailed)
        return
      }
      setOrderOpen(false)
      setOrderRows([])
      setDragIndex(null)
      resetOrderMoveSession()
    } catch (error) {
      setOrderError(error?.message || text.orderSaveFailedShort)
    } finally {
      setOrderSaving(false)
    }
  }

  const failoverCandidates = useMemo(() => orderedModelRows(persistedProfiles).map(row => {
    const protocol = String(persistedProfiles[row.profileIndex]?.type || DEFAULT_PROTOCOL)
    return { ...row, family: protocol.startsWith('native_') ? 'native' : 'legacy', protocol }
  }), [persistedProfiles])
  const failoverMemberKey = member => `${String(member?.provider_var_name || member?.providerVarName || '')}\u0000${String(member?.model || '')}`
  const failoverCandidateMap = new Map(failoverCandidates.map(candidate => [
    failoverMemberKey({ provider_var_name: candidate.providerVarName, model: candidate.model }),
    candidate,
  ]))
  const failoverValidation = (() => {
    const names = new Set()
    for (let groupIndex = 0; groupIndex < failoverDrafts.length; groupIndex += 1) {
      const group = failoverDrafts[groupIndex]
      const suffix = failoverGroupSuffix(group.var_name).trim()
      const name = failoverGroupVarName(suffix)
      if (!/^[A-Za-z0-9_]+$/.test(suffix)) return `${text.failoverGroup || 'Failover group'} ${groupIndex + 1}: ${text.errors?.varNameInvalid || 'invalid variable name'}`
      if (names.has(name)) return `${text.failoverGroup || 'Failover group'} ${groupIndex + 1}: ${text.errors?.varNameDuplicate || 'duplicate variable name'}`
      names.add(name)
      if (!Array.isArray(group.members) || group.members.length < 2) return `${name}: ${text.failoverNeedsTwo}`
      const families = new Set()
      for (const member of group.members) {
        const candidate = failoverCandidateMap.get(failoverMemberKey(member))
        if (!candidate) return `${name}: ${text.failoverMissingMember || 'A selected model is no longer available.'}`
        families.add(candidate.family)
      }
      if (families.size > 1) return `${name}: ${text.failoverSameFamily}`
      const retries = Number(group.max_retries)
      if (!Number.isInteger(retries) || retries < 0) return `${name}: ${text.failoverRetriesInvalid}`
      const delay = Number(group.base_delay)
      if (!Number.isFinite(delay) || delay < 0) return `${name}: ${text.failoverDelayInvalid}`
      if (group.spring_back !== '' && group.spring_back !== undefined && group.spring_back !== null) {
        const springBack = Number(group.spring_back)
        if (!Number.isInteger(springBack) || springBack <= 0) return `${name}: ${text.failoverSpringInvalid}`
      }
    }
    return ''
  })()
  const nextFailoverGroupUiKey = () => `failover-group-${++failoverGroupKeySeedRef.current}`
  const toggleFailoverGroup = groupKey => {
    setFailoverGroupExpanded(current => {
      const next = new Set(current)
      if (next.has(groupKey)) next.delete(groupKey)
      else next.add(groupKey)
      return next
    })
  }
  const openFailover = () => {
    setFailoverDrafts(migrateFailoverGroupNames(normalizeFailoverGroups(failoverGroups)).map(group => ({
      ...group,
      _ui_key: nextFailoverGroupUiKey(),
      members: group.members.map(member => ({ ...member })),
      max_retries: group.max_retries ?? 10,
      base_delay: group.base_delay ?? 0.5,
      spring_back: group.spring_back ?? '',
    })))
    setFailoverGroupExpanded(new Set())
    setFailoverError('')
    setFailoverOpen(true)
  }
  const closeFailover = () => {
    if (failoverSaving) return
    setFailoverOpen(false)
    setFailoverDrafts([])
    setFailoverGroupExpanded(new Set())
    setFailoverError('')
  }
  const addFailoverGroup = () => {
    const groupKey = nextFailoverGroupUiKey()
    setFailoverDrafts(current => [...current, {
      _ui_key: groupKey,
      var_name: nextFailoverGroupName(current),
      members: [],
      max_retries: 10,
      base_delay: 0.5,
      spring_back: '',
    }])
    setFailoverGroupExpanded(current => new Set(current).add(groupKey))
    setFailoverError('')
  }
  const patchFailoverGroup = (groupIndex, patch) => {
    setFailoverDrafts(current => current.map((group, index) => index === groupIndex ? { ...group, ...patch } : group))
    setFailoverError('')
  }
  const removeFailoverGroup = groupIndex => {
    const groupKey = failoverDrafts[groupIndex]?._ui_key
    setFailoverDrafts(current => current.filter((_, index) => index !== groupIndex))
    if (groupKey) {
      setFailoverGroupExpanded(current => {
        const next = new Set(current)
        next.delete(groupKey)
        return next
      })
    }
    setFailoverError('')
  }
  const moveFailoverGroup = (fromIndex, toIndex) => {
    setFailoverDrafts(current => moveOrderedItem(current, fromIndex, toIndex))
    setFailoverError('')
  }
  const toggleFailoverMember = (groupIndex, candidate) => {
    const key = failoverMemberKey({ provider_var_name: candidate.providerVarName, model: candidate.model })
    setFailoverDrafts(current => current.map((group, index) => {
      if (index !== groupIndex) return group
      const members = Array.isArray(group.members) ? group.members : []
      const selected = members.some(member => failoverMemberKey(member) === key)
      return {
        ...group,
        members: selected
          ? members.filter(member => failoverMemberKey(member) !== key)
          : [...members, { provider_var_name: candidate.providerVarName, model: candidate.model }],
      }
    }))
    setFailoverError('')
  }
  const moveFailoverMember = (groupIndex, fromIndex, toIndex) => {
    setFailoverDrafts(current => current.map((group, index) => index === groupIndex
      ? { ...group, members: moveOrderedItem(group.members, fromIndex, toIndex) }
      : group))
    setFailoverError('')
  }
  const saveFailover = async () => {
    if (!onSaveFailoverGroups) {
      setFailoverError(text.failoverSaveMissing)
      return
    }
    if (failoverValidation) {
      setFailoverError(failoverValidation)
      return
    }
    const nextGroups = failoverDrafts.map(group => {
      const normalized = {
        var_name: String(group.var_name || '').trim(),
        members: group.members.map(member => ({
          provider_var_name: String(member.provider_var_name || '').trim(),
          model: String(member.model || '').trim(),
        })),
        max_retries: Number(group.max_retries),
        base_delay: Number(group.base_delay),
      }
      if (group.spring_back !== '' && group.spring_back !== undefined && group.spring_back !== null) normalized.spring_back = Number(group.spring_back)
      return normalized
    })
    setFailoverSaving(true)
    setFailoverError('')
    try {
      const ok = await onSaveFailoverGroups(nextGroups)
      if (!ok) {
        setFailoverError(text.failoverSaveFailed)
        return
      }
      setFailoverOpen(false)
      setFailoverDrafts([])
    } catch (error) {
      setFailoverError(error?.message || text.failoverSaveFailed)
    } finally {
      setFailoverSaving(false)
    }
  }

  const openProbeScope = () => {
    const allKeys = profiles.map(modelProbeProviderKey)
    const specific = effectiveProbeProviders.length > 0
    setProbeScopeMode(specific ? 'selected' : 'all')
    setProbeScopeKeys(specific ? effectiveProbeProviders.filter(key => allKeys.includes(key)) : allKeys)
    setProbeScopeError('')
    setProbeScopeOpen(true)
  }

  const saveProbeScope = async () => {
    const allKeys = profiles.map(modelProbeProviderKey)
    const selected = normalizeModelProbeProviderKeys(probeScopeKeys).filter(key => allKeys.includes(key))
    if (probeScopeMode === 'selected' && !selected.length) {
      setProbeScopeError('至少选择一个服务商。')
      return
    }
    const next = probeScopeMode === 'all' ? [] : selected
    setProbeScopeSaving(true)
    setProbeScopeError('')
    try {
      const saved = await onSaveModelProbeProviders?.(next)
      setEffectiveProbeProviders(normalizeModelProbeProviderKeys(saved ?? next))
      setProbeScopeOpen(false)
    } catch (error) {
      setProbeScopeError(String(error?.message || error))
    } finally {
      setProbeScopeSaving(false)
    }
  }

  const runBatchAvailability = async () => {
    const targets = resolveModelProbeTargets(profiles, effectiveProbeProviders)
    if (!targets.length) {
      setBatchProbeResult({ type: 'error', message: '没有可检测的服务商', description: '请打开“检测范围”重新选择服务商。' })
      return
    }
    setBatchProbeBusy(true)
    setBatchProbeResult(null)
    setBatchProbeProgress({ completed: 0, total: targets.length, current: targets[0].name })
    try {
      const outcome = await runModelBatchProbe({
        profiles,
        configuredKeys: effectiveProbeProviders,
        probeModels,
        onProgress: setBatchProbeProgress,
      })
      if (!outcome.summary.successfulProviders) throw new Error('所有服务商均检测失败，未修改模型状态。')
      const saved = await onSaveModelProfiles?.(outcome.profiles)
      if (saved === false || !onSaveModelProfiles) throw new Error('模型状态保存失败，未应用本次检测结果。')
      const summary = outcome.summary
      const failures = outcome.results.filter(item => item.error).slice(0, 3).map(item => `${item.name}：${item.error}`).join('；')
      setBatchProbeResult({
        type: summary.failedProviders || summary.unavailable ? 'warning' : 'success',
        message: `批量检测完成：${summary.successfulProviders} 个服务商成功，${summary.failedProviders} 个失败`,
        description: `${summary.available} 个模型可用，${summary.unavailable} 个不可用；自动禁用 ${summary.disabled} 个，自动恢复 ${summary.restored} 个。${failures ? `失败摘要：${failures}。` : ''}`,
      })
    } catch (error) {
      setBatchProbeResult({ type: 'error', message: '批量检测失败', description: String(error?.message || error) })
    } finally {
      setBatchProbeBusy(false)
    }
  }

  const riskItems = [{
    key: 'risk',
    label: <Space size={7}><AlertTriangle size={14} />{text.riskTitle}</Space>,
    children: (
      <div className="model-risk-content">
        <Alert
          type={risk.status === 'error' ? 'error' : 'info'}
          message={risk.status === 'ready' ? text.riskReady : risk.status === 'error' ? text.riskUnavailable : text.riskEmpty}
          description={risk.status === 'error' ? risk.error : text.riskHelp}
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
          <Alert type="warning" message={text.missingGates(risk.missingConfirmedWriteRoutes.join(', '))} />
        )}
      </div>
    ),
  }]

  return (
    <section className="models-page">
      <header className="model-page-head model-page-head--actions-only">
        <div className="model-page-actions">
          <Button type="primary" icon={<ShieldCheck size={14} />} onClick={runBatchAvailability} loading={batchProbeBusy} disabled={!profiles.length}>
            对话检测并同步
          </Button>
          <Button icon={<Settings2 size={14} />} onClick={openProbeScope} disabled={batchProbeBusy || !profiles.length} title="配置一键检测涉及的服务商">
            检测范围：{probeScopeLabel}
          </Button>
          <Button icon={<UploadCloud size={14} />} onClick={() => importModels()} loading={importLoading}>重新读取</Button>
          <Button
            icon={<ListOrdered size={14} />}
            onClick={openModelOrder}
            disabled={!persistedOrderCount}
            title={persistedOrderCount ? text.orderAvailable : text.orderUnavailable}
          >
            {text.modelOrder}
          </Button>
          <Button
            icon={<Network size={14} />}
            onClick={openFailover}
            disabled={!persistedOrderCount}
            title={persistedOrderCount ? text.failoverAvailable : text.orderUnavailable}
          >
            {text.failover}
          </Button>
          <Button icon={<FileCode2 size={14} />} onClick={openPreview}>配置预览</Button>
          <Button icon={<Plus size={15} />} onClick={openAdd}>新增服务商</Button>
        </div>
      </header>

      <div className="model-summary-line" aria-label={text.configSummary}>
        <div className="model-summary-status">
          <span className={`model-summary-dot${hasErrors ? ' is-error' : ''}`} />
          <strong>{summary.total} 个服务商</strong>
          <span>{totalModels} 个模型</span>
          {availabilityTotals.disabled > 0 && <span className="is-warning">{availabilityTotals.disabled} 个已禁用</span>}
          {summary.errors > 0 && <span className="is-error">{summary.errors} 个阻断项</span>}
          {summary.warnings > 0 && <span className="is-warning">{summary.warnings} 个提醒</span>}
        </div>
        <div className="model-summary-source"><FileCode2 size={13} /><span>{text.configSource}</span><code>mykey.py</code></div>
      </div>

      {batchProbeBusy && (
        <Alert
          type="info"
          showIcon
          message={`正在检测 ${batchProbeProgress?.completed || 0}/${batchProbeProgress?.total || configuredProbeTargetCount} 个服务商`}
          description={(
            <div className="model-batch-progress">
              <span>{batchProbeProgress?.current || '正在准备真实对话检测…'}</span>
              <Progress
                percent={batchProbeProgress?.total ? Math.round((batchProbeProgress.completed / batchProbeProgress.total) * 100) : 0}
                size="small"
                showInfo={false}
              />
            </div>
          )}
          className="model-page-alert"
        />
      )}
      {!batchProbeBusy && batchProbeResult && (
        <Alert
          type={batchProbeResult.type}
          showIcon
          closable
          onClose={() => setBatchProbeResult(null)}
          message={batchProbeResult.message}
          description={batchProbeResult.description}
          className="model-page-alert"
        />
      )}
      {hasErrors && <Alert type="error" showIcon message="存在不能保存的服务商，请在目录中选择异常项并修复。" className="model-page-alert" />}

      <div className="model-workbench">
        <aside className="model-provider-rail">
          <header className="model-rail-head">
            <div><strong>{text.providerDirectory}</strong><span>{text.chooseProvider}</span></div>
            <b>{profiles.length}</b>
          </header>

          <div ref={providerNavRef} className="model-provider-nav" role="navigation" aria-label="模型服务商">
              {profiles.map((profile, idx) => {
              const result = validation[idx]
              const count = profileModels(profile).length
              const availability = modelAvailabilitySummary(profile)
              const meta = protocolMeta(profile.type || DEFAULT_PROTOCOL)
              const state = result?.errors?.length ? 'error' : (result?.warnings?.length || availability.unavailable) ? 'warning' : 'ready'
                const motionKey = providerMotionKey(profile)
                const isProviderDragging = providerDrag?.key === motionKey
              return (
                <div
                  className={`model-provider-entry${isProviderDragging ? ' is-drag-placeholder' : ''}`}
                  key={motionKey}
                  data-provider-index={idx}
                  data-provider-motion-key={motionKey}
                  style={isProviderDragging ? { height: providerDrag.height } : undefined}
                >
                  <button
                    type="button"
                    className={`model-provider-item${!addOpen && activeIndex === idx ? ' is-active' : ''}${providerHoldIndex === idx ? ' is-holding' : ''}${isProviderDragging ? ' is-dragging' : ''}`}
                    style={isProviderDragging ? {
                      left: providerDrag.left,
                      top: providerDrag.top,
                      width: providerDrag.width,
                      height: providerDrag.height,
                    } : undefined}
                    onClick={() => {
                      if (Date.now() < suppressProviderClickUntilRef.current) return
                      openProfile(idx)
                    }}
                    onPointerDown={event => startProviderHold(idx, event)}
                    onPointerMove={moveProviderHold}
                    onPointerUp={endProviderHold}
                    onPointerCancel={cancelProviderDrag}
                    onPointerLeave={moveProviderHold}
                    aria-current={!addOpen && activeIndex === idx ? 'true' : undefined}
                    aria-label={`${profile.name || providerDisplayName(profile.var_name) || `服务商 ${idx + 1}`}，长按拖动调整顺序`}
                  >
                    <span className="model-provider-item-top">
                      <strong>{profile.name || providerDisplayName(profile.var_name) || `服务商 ${idx + 1}`}</strong>
                      <i className={`is-${state}`} title={state === 'error' ? '存在阻断项' : availability.unavailable ? `${availability.unavailable} 个模型不可用` : state === 'warning' ? '存在提醒' : '配置正常'} />
                    </span>
                    <span className="model-provider-base">{profile.apibase || '尚未填写 BaseURL'}</span>
                    <span className="model-provider-meta"><em>{meta?.shortLabel || protocolLabel(profile.type)}</em><b>{availability.enabled}/{count} 启用</b></span>
                  </button>
                </div>
              )
            })}
          </div>

          {providerOrderError && (
            <Alert type="error" showIcon message={providerOrderError} className="model-provider-order-alert" />
          )}

          <button type="button" className={`model-provider-add${addOpen ? ' is-active' : ''}`} onClick={openAdd}>
            <Plus size={15} /><span>{text.addProvider}</span>
          </button>

          <footer className="model-rail-foot">
            <CheckCircle2 size={13} />
            <span>{text.singleSaveHint}</span>
          </footer>
        </aside>

        <section className="model-editor-workspace" aria-label={addOpen ? text.addProvider : text.providerEditor}>
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
              <div key={profile.client_id || `provider-${idx}`} className="model-editor-slot" hidden={addOpen || activeIndex !== idx}>
                <ProfileCard
                  profile={profile}
                  idx={idx}
                  profileKey={key}
                  result={validation[idx]}
                  profiles={profiles}
                  patchProfile={patchProfile}
                  removeProfile={removeProfile}
                  discoverModels={discoverModels}
                  probeModels={probeModels}
                  revealedKey={revealedKeys[key]}
                  revealBusy={!!revealBusy[key]}
                  onRevealKey={onRevealKey}
                  onClearRevealedKey={onClearRevealedKey}
                  onSave={saveModelProfile}
                  saveState={modelSaveStatus[key] || modelSaveStatus[idx]}
                  t={t}
                />
              </div>
            )
          })}

          {!profiles.length && !addOpen && (
            <div className="model-empty-state">
              <Layers size={36} strokeWidth={1.2} className="model-empty-icon" />
              <strong>{importLoading ? text.loadingMykey : text.noProviders}</strong>
              <span>{importLoading ? text.loadingHelp : text.noProvidersHelp}</span>
              {!importLoading && <Button type="primary" icon={<Plus size={15} />} onClick={openAdd}>{text.addProvider}</Button>}
            </div>
          )}
        </section>
      </div>

      <Collapse ghost items={riskItems} className="model-risk-collapse" />

      {probeScopeOpen && (
        <Modal
          title="配置一键检测范围"
          open
          onCancel={() => { if (!probeScopeSaving) setProbeScopeOpen(false) }}
          onOk={saveProbeScope}
          okText="保存范围"
          cancelText="取消"
          confirmLoading={probeScopeSaving}
          mask={{ closable: !probeScopeSaving }}
          className="model-probe-scope-modal"
        >
          <Alert
            type="info"
            showIcon
            message="配置会保存在当前 GA Admin 服务中"
            description="默认检测目录中的全部服务商；选择指定范围后，后续一键检测只处理已勾选的服务商。"
          />
          <Radio.Group value={probeScopeMode} onChange={event => { setProbeScopeMode(event.target.value); setProbeScopeError('') }} className="model-probe-scope-mode">
            <Radio value="all">全部服务商（默认）</Radio>
            <Radio value="selected">指定服务商</Radio>
          </Radio.Group>
          <div className={`model-probe-provider-list${probeScopeMode === 'all' ? ' is-disabled' : ''}`}>
            {profiles.map((profile, index) => {
              const key = modelProbeProviderKey(profile, index)
              return (
                <Checkbox
                  key={key}
                  checked={probeScopeMode === 'all' || probeScopeKeys.includes(key)}
                  disabled={probeScopeMode === 'all'}
                  onChange={event => {
                    setProbeScopeKeys(current => event.target.checked ? [...current, key] : current.filter(value => value !== key))
                    setProbeScopeError('')
                  }}
                >
                  <span className="model-probe-provider-option">
                    <b>{modelProbeProviderName(profile, index)}</b>
                    <small>{profile.apibase || '尚未填写 BaseURL'}</small>
                  </span>
                </Checkbox>
              )
            })}
          </div>
          {probeScopeError && <Alert type="error" showIcon message={probeScopeError} />}
        </Modal>
      )}

      <Drawer
        title={text.orderTitle}
        placement="right"
        width={620}
        open={orderOpen}
        onClose={closeModelOrder}
        closable={!orderSaving}
        maskClosable={!orderSaving}
        className="model-order-drawer"
        footer={(
          <div className="model-order-footer">
            <span>{text.discardOrder}</span>
            <Space>
              <Button onClick={closeModelOrder} disabled={orderSaving}>{t.cancel}</Button>
              <Button type="primary" onClick={saveModelOrder} loading={orderSaving} disabled={!orderRows.length}>{text.confirmSave}</Button>
            </Space>
          </div>
        )}
      >
        <Alert
          type="info"
          showIcon
          message={text.orderInfo}
          description={text.orderDescription}
        />
        {orderError && <Alert type="error" showIcon message={orderError} className="model-order-error" />}
        <div className="model-order-list" role="list" aria-label={text.savedOrder}>
          {orderRows.map((row, index) => (
            <div
              key={row.id}
              role="listitem"
              className={`model-order-row${dragIndex === index ? ' is-dragging' : ''}${repeatOrderRowId === row.id ? ' is-repeat-target' : ''}${row.type === 'failover' ? ' is-failover' : ''}`}
              draggable={!orderSaving}
              onDragStart={event => {
                resetOrderMoveSession()
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
              {row.type === 'failover' && (
                <div className="model-order-copy model-order-failover">
                  <div className="model-failover-name">
                    <code>{row.varName}</code>
                    <strong>{text.failoverGroup || 'Failover group'}</strong>
                    <span>{row.members?.length || 0} {text.failoverMembers || 'members'}</span>
                  </div>
                  {row.members?.length > 0 && <div className="model-failover-members">
                    {row.members.map((member, memberIndex) => (
                      <div key={`${row.id}-${memberIndex}`} className="model-failover-member">
                        <span className="model-failover-member-index">{memberIndex + 1}.</span>
                        <code>{providerDisplayName(member.provider_var_name) || member.provider_var_name} · {member.model}</code>
                      </div>
                    ))}
                  </div>}
                </div>
              )}
              <div className="model-order-copy">
                <code>{row.variableName}</code>
                <strong title={row.model}>{row.model || '未填写模型 ID'}</strong>
                <span>服务商名称：{row.providerName || providerDisplayName(row.providerVarName) || '未命名'}</span>
              </div>
              <div className="model-order-actions">
                <Button
                  type="text"
                  size="small"
                  icon={<ArrowUp size={15} />}
                  aria-label={`${text.moveUp} ${row.type === 'failover' ? row.varName : (row.model || row.variableName)}`}
                  title={text.moveUp}
                  disabled={orderSaving || index === 0}
                  onClick={event => moveModelOrder(row.id, -1, event)}
                />
                <Button
                  type="text"
                  size="small"
                  icon={<ArrowDown size={15} />}
                  aria-label={`${text.moveDown} ${row.type === 'failover' ? row.varName : (row.model || row.variableName)}`}
                  title={text.moveDown}
                  disabled={orderSaving || index === orderRows.length - 1}
                  onClick={event => moveModelOrder(row.id, 1, event)}
                />
              </div>
            </div>
          ))}
        </div>
      </Drawer>

      <Drawer
        title={text.previewTitle}
        placement="right"
        width={680}
        open={previewOpen}
        onClose={() => setPreviewOpen(false)}
        className="model-preview-drawer"
        extra={<Button icon={<RefreshCw size={14} />} onClick={previewModels}>{text.refreshPreview}</Button>}
      >
        <Alert type="info" showIcon message={text.previewSecret} />
        <pre className="model-preview-pre">{modelPreview || (profiles.length ? text.generatingPreview : text.previewNeedsProvider)}</pre>
      </Drawer>

      <Drawer
        title={text.failoverTitle}
        placement="right"
        width={780}
        open={failoverOpen}
        onClose={closeFailover}
        closable={!failoverSaving}
        maskClosable={!failoverSaving}
        className="model-failover-drawer"
        footer={(
          <div className="model-order-footer model-failover-footer">
            <span>{text.failoverDiscard}</span>
            <Space>
              <Button onClick={closeFailover} disabled={failoverSaving}>{t.cancel}</Button>
              <Button type="primary" onClick={saveFailover} loading={failoverSaving} disabled={Boolean(failoverValidation)}>{text.confirmSave}</Button>
            </Space>
          </div>
        )}
      >
        <div className="model-failover-stack">
          <Alert type="info" showIcon message={text.failoverInfo} description={text.failoverDescription} />
          {failoverError && <Alert type="error" showIcon message={failoverError} />}
          {!failoverError && failoverValidation && <Alert type="warning" showIcon message={failoverValidation} />}

          <section className="model-failover-section">
            <div className="model-failover-section-head">
              <div><span className="model-failover-kicker">00</span><strong>{text.failoverGroups || text.failoverTitle}</strong></div>
              <Button icon={<Plus size={14} />} disabled={failoverSaving} onClick={addFailoverGroup}>{text.addGroup || t.add}</Button>
            </div>
            {!failoverDrafts.length && <div className="model-failover-empty">{text.failoverNoGroups || 'No failover groups. Add one to get started.'}</div>}
          </section>

          {failoverDrafts.map((group, groupIndex) => {
            const selectedKeys = new Set(group.members.map(failoverMemberKey))
            const selectedFamilies = new Set(group.members.map(member => failoverCandidateMap.get(failoverMemberKey(member))?.family).filter(Boolean))
            const groupExpanded = failoverGroupExpanded.has(group._ui_key)
            return (
              <section className={`model-failover-section model-failover-group${groupExpanded ? ' is-expanded' : ' is-collapsed'}`} key={group._ui_key}>
                <div className="model-failover-section-head model-failover-group-head">
                  <div className="model-failover-group-title">
                    <span className="model-failover-kicker">{String(groupIndex + 1).padStart(2, '0')}</span>
                    <span><strong>{group.var_name || text.failoverGroup || 'Failover group'}</strong><small>{group.members.length} {text.failoverMembers || 'members'}</small></span>
                  </div>
                  <Space size={2}>
                    <Button type="text" size="small" icon={<ArrowUp size={14} />} aria-label={text.moveUp} disabled={failoverSaving || groupIndex === 0} onClick={() => moveFailoverGroup(groupIndex, groupIndex - 1)} />
                    <Button type="text" size="small" icon={<ArrowDown size={14} />} aria-label={text.moveDown} disabled={failoverSaving || groupIndex === failoverDrafts.length - 1} onClick={() => moveFailoverGroup(groupIndex, groupIndex + 1)} />
                    <Button danger type="text" size="small" icon={<Trash2 size={14} />} aria-label={t.delete} disabled={failoverSaving} onClick={() => removeFailoverGroup(groupIndex)} />
                    <Button
                      type="text"
                      size="small"
                      className="model-failover-group-toggle"
                      icon={<ChevronDown size={14} />}
                      aria-label={groupExpanded ? text.collapse : text.configure}
                      aria-expanded={groupExpanded}
                      onClick={() => toggleFailoverGroup(group._ui_key)}
                    />
                  </Space>
                </div>
                {groupExpanded && <div className="model-failover-group-body">
                <label className="model-failover-name">
                  <span>{text.varName || 'Variable name'}</span>
                  <Input
                    addonBefore={FAILOVER_VAR_PREFIX}
                    value={failoverGroupSuffix(group.var_name)}
                    disabled={failoverSaving}
                    onChange={event => patchFailoverGroup(groupIndex, { var_name: failoverGroupVarName(event.target.value) })}
                  />
                </label>

                <div className="model-failover-section-head">
                  <div><span className="model-failover-kicker">A</span><strong>{text.failoverCandidates}</strong></div>
                  <span>{group.members.length} / {failoverCandidates.length}</span>
                </div>
                <p>{text.failoverCandidatesHelp}</p>
                <FailoverCandidatePicker
                  candidates={failoverCandidates}
                  selectedKeys={selectedKeys}
                  selectedFamilies={selectedFamilies}
                  failoverSaving={failoverSaving}
                  onToggle={candidate => toggleFailoverMember(groupIndex, candidate)}
                  text={text}
                  providerDisplayName={providerDisplayName}
                  memberKey={failoverMemberKey}
                />

                <div className="model-failover-section-head">
                  <div><span className="model-failover-kicker">B</span><strong>{text.failoverPriority}</strong></div>
                </div>
                <p>{text.failoverPriorityHelp}</p>
                <DndContext
                  sensors={sensors}
                  collisionDetection={closestCenter}
                  onDragEnd={({ active, over }) => {
                    if (!over || active.id === over.id) return
                    const oldIdx = group.members.findIndex(m => failoverMemberKey(m) === active.id)
                    const newIdx = group.members.findIndex(m => failoverMemberKey(m) === over.id)
                    if (oldIdx !== -1 && newIdx !== -1) moveFailoverMember(groupIndex, oldIdx, newIdx)
                  }}
                >
                  <SortableContext items={group.members.map(m => failoverMemberKey(m))} strategy={verticalListSortingStrategy}>
                    <div className="model-failover-priority" role="list" aria-label={text.failoverPriority}>
                      {group.members.map((member, memberIndex) => {
                        const candidate = failoverCandidateMap.get(failoverMemberKey(member))
                        return (
                          <SortableFailoverMemberRow
                            key={failoverMemberKey(member)}
                            memberKey={failoverMemberKey(member)}
                            member={member}
                            memberIndex={memberIndex}
                            groupIndex={groupIndex}
                            groupLength={group.members.length}
                            failoverSaving={failoverSaving}
                            candidate={candidate}
                            moveFailoverMember={moveFailoverMember}
                            toggleFailoverMember={toggleFailoverMember}
                            patchFailoverGroup={patchFailoverGroup}
                            group={group}
                            text={text}
                            providerDisplayName={providerDisplayName}
                          />
                        )
                      })}
                      {!group.members.length && <div className="model-failover-empty">{text.failoverNeedsTwo}</div>}
                    </div>
                  </SortableContext>
                </DndContext>

                <div className="model-failover-section-head">
                  <div><span className="model-failover-kicker">C</span><strong>{text.failoverPolicy}</strong></div>
                </div>
                <p>{text.failoverPolicyHelp}</p>
                <div className="model-failover-settings">
                  <label><span>{text.failoverRetries}</span><Input type="number" min="0" step="1" value={group.max_retries} disabled={failoverSaving} onChange={event => patchFailoverGroup(groupIndex, { max_retries: event.target.value })} /><small>{text.failoverRetriesHelp}</small></label>
                  <label><span>{text.failoverDelay}</span><Input type="number" min="0" step="0.1" value={group.base_delay} disabled={failoverSaving} onChange={event => patchFailoverGroup(groupIndex, { base_delay: event.target.value })} /><small>{text.failoverDelayHelp}</small></label>
                  <label><span>{text.failoverSpring}</span><Input type="number" min="1" step="1" value={group.spring_back} placeholder={text.failoverSpringPlaceholder} disabled={failoverSaving} onChange={event => patchFailoverGroup(groupIndex, { spring_back: event.target.value })} /><small>{text.failoverSpringHelp}</small></label>
                </div>
                </div>}
              </section>
            )
          })}
        </div>
      </Drawer>

      <Drawer
        title={text.previewTitle}
        placement="right"
        width={680}
        open={previewOpen}
        onClose={() => setPreviewOpen(false)}
        className="model-preview-drawer"
        extra={<Button icon={<RefreshCw size={14} />} onClick={previewModels}>{text.refreshPreview}</Button>}
      >
        <Alert type="info" showIcon message={text.previewSecret} />
        <pre className="model-preview-pre">{modelPreview || (profiles.length ? text.generatingPreview : text.previewNeedsProvider)}</pre>
      </Drawer>
    </section>
  )
}
