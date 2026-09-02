import React, { useCallback, useEffect, useRef, useState } from 'react'
import { AlertTriangle, CheckCircle2, CircleHelp, Cpu, Download, Pencil, Plus, RefreshCw, Save, Server, Settings2, Star, Trash2, X } from 'lucide-react'
import { api } from '../lib/api'
import { confirmDanger } from '../lib/danger'

const EMPTY_FORM = { id: '', name: '', ga_root: '', python_path: '', source_instance_id: '', copy_memory: false, copy_mykey: false }

const TEXT = {
  zh: {
    title: 'GA \u5b9e\u4f8b\u7ba1\u7406',
    summary: '每个 GenericAgent 实例拥有独立的项目根目录、Python 环境、聊天数据、定时任务、自主进化记录、记忆与模型设置；切换实例会同步切换整套工作区。未显式选择实例的请求使用默认实例。',
    helpLabel: '\u4e86\u89e3 GA \u5b9e\u4f8b\u4f7f\u7528\u8bf4\u660e', helpTitle: 'GA \u5b9e\u4f8b\u662f\u4ec0\u4e48\uff1f',
    helpText: '每个实例是一份独立的 GA 项目副本，不只隔离运行目录，还隔离聊天、定时任务、自主进化、记忆、模型和用量数据。',
    helpSteps: ['新建：填写 ID、名称和根目录；Python 路径可留空自动检测。', '克隆：选择已有实例会复制程序、任务和项目设置；定时服务自启动默认关闭，避免多个实例抢占端口。', '切换：顶部实例选择器会让文件、聊天、任务、自主进化、记忆、模型和用量一起切换；未指定实例的请求使用默认实例。'],
    helpNote: '删除只移除管理台记录，不会删除磁盘上的 GenericAgent 目录；如有正在运行的聊天、循环或标题生成，请先停止；本地 CMD 会随实例删除而关闭；复制记忆和 mykey.py 都必须显式勾选。',
    add: '\u65b0\u5efa\u5b9e\u4f8b', install: '\u4e00\u952e\u65b0\u589e', installing: '\u6b63\u5728\u4e0b\u8f7d\u5e76\u65b0\u589e\u2026', refresh: '\u5237\u65b0', loading: '\u6b63\u5728\u8bfb\u53d6\u5b9e\u4f8b\u2026', empty: '\u6682\u65e0 GA \u5b9e\u4f8b',
    default: '\u9ed8\u8ba4', setDefault: '\u8bbe\u4e3a\u9ed8\u8ba4', configureModels: '\u914d\u7f6e\u6a21\u578b', edit: '\u7f16\u8f91', remove: '\u5220\u9664', cancel: '\u53d6\u6d88',
    initializing: '\u521d\u59cb\u5316\u4e2d', ready: '\u5df2\u5c31\u7eea', failed: '\u521d\u59cb\u5316\u5931\u8d25', initError: '\u9519\u8bef\u8be6\u60c5',
    stages: { queued: '\u7b49\u5f85\u5f00\u59cb', preparing: '\u51c6\u5907\u5b89\u88c5\u76ee\u5f55', cloning: '\u590d\u5236\u5df2\u6709 GenericAgent \u9879\u76ee', downloading: '\u4e0b\u8f7d\u5e76\u89e3\u538b GenericAgent', extracting: '\u89e3\u538b\u4e0a\u4f20\u7684\u6a21\u677f', verifying: '\u6821\u9a8c\u5b89\u88c5\u6587\u4ef6', finalizing: '\u4fdd\u5b58\u5b9e\u4f8b\u914d\u7f6e', complete: '\u521d\u59cb\u5316\u5b8c\u6210' },
    createTitle: '\u65b0\u5efa GA \u5b9e\u4f8b', editTitle: '\u7f16\u8f91 GA \u5b9e\u4f8b', installTitle: '填写新实例 ID', create: '\u521b\u5efa\u5b9e\u4f8b', save: '\u4fdd\u5b58\u4fee\u6539', startInstall: '开始创建',
    createSummary: '设置实例标识与本地运行环境。', editSummary: '更新显示名称或运行时路径。', installSummary: '选择稳定的标识，创建任务将在后台运行。',
    identityGroup: '实例标识', runtimeGroup: '运行环境', sourceGroup: '初始化来源', requiredField: '必填', optionalField: '可选', manualSource: '手动指定新的根目录', sourceHint: '复制已有实例的程序、任务、自主进化记录和项目设置；定时服务自启动不会复制，以免多个实例争抢端口；根目录留空时由系统自动分配隔离目录。', copyMemory: '复制已有记忆', copyMemoryHint: '记忆可能包含个性化上下文，仅在确认需要继承时勾选。', copyMyKey: '复制 mykey.py', copyMyKeyHint: '包含 API Key / Token 等敏感配置，请谨慎勾选。', active: '当前使用中', switchTo: '切换到此实例', switchHint: '切换后文件、任务、记忆、模型、聊天、用量和服务状态会重新加载。',
    template: 'GA.zip \u6a21\u677f\u5305\uff08\u53ef\u9009\uff09', templateHint: '\u4e0a\u4f20\u7684 .zip \u4f1a\u6301\u4e45\u4fdd\u5b58\u5e76\u66ff\u6362\u5f53\u524d\u6a21\u677f\uff0c\u540c\u65f6\u7528\u4e8e\u672c\u6b21\u521d\u59cb\u5316\u3002',
    reuseTemplate: '\u4f7f\u7528\u5df2\u4fdd\u5b58\u7684 GA.zip \u6a21\u677f', templateReady: '\u6a21\u677f\u5df2\u4fdd\u5b58\uff0c\u53ef\u76f4\u63a5\u590d\u7528\u3002', templateMissing: '\u5c1a\u65e0\u5df2\u4fdd\u5b58\u7684\u6a21\u677f\uff0c\u7559\u7a7a\u5c06\u4ece main \u5206\u652f\u4e0b\u8f7d\u3002',
    id: '\u5b9e\u4f8b ID', name: '\u663e\u793a\u540d\u79f0', root: 'GenericAgent \u6839\u76ee\u5f55', python: 'Python \u8def\u5f84', effectivePython: '\u5b9e\u9645 Python', auto: '\u81ea\u52a8\u68c0\u6d4b',
    idHint: '\u4ec5\u5efa\u7acb\u65f6\u53ef\u8bbe\u7f6e\uff0c\u5efa\u8bae\u4f7f\u7528\u7b80\u77ed\u4e14\u7a33\u5b9a\u7684\u6807\u8bc6\u3002', rootHint: '\u8be5\u76ee\u5f55\u5e94\u5305\u542b agentmain.py\u3002', pythonHint: '\u7559\u7a7a\u65f6\u7531\u540e\u7aef\u81ea\u52a8\u68c0\u6d4b\u3002',
    required: '\u8bf7\u586b\u5199\u5b9e\u4f8b ID\u3001\u540d\u79f0\u548c GenericAgent \u6839\u76ee\u5f55\u3002',
    loadFailed: '\u8bfb\u53d6\u5b9e\u4f8b\u5931\u8d25', saved: '\u5b9e\u4f8b\u5df2\u4fdd\u5b58', installed: '\u5b9e\u4f8b\u5df2\u65b0\u589e\uff0c\u6b63\u5728\u540e\u53f0\u521d\u59cb\u5316', creating: '\u5b9e\u4f8b\u5df2\u767b\u8bb0\uff0c\u6b63\u5728\u540e\u53f0\u590d\u5236\u9879\u76ee', defaultSaved: '\u9ed8\u8ba4\u5b9e\u4f8b\u5df2\u66f4\u65b0', removed: '\u5b9e\u4f8b\u5df2\u5220\u9664',
    confirmInstall: '\u5c06\u4ece GenericAgent main \u5206\u652f\u4e0b\u8f7d\u6e90\u7801\u5e76\u81ea\u52a8\u6ce8\u518c\u4e3a\u65b0\u5b9e\u4f8b\uff0c\u7ee7\u7eed\u5417\uff1f',
    confirmCreate: '\u786e\u8ba4\u521b\u5efa\u8be5 GA \u5b9e\u4f8b\uff1f', confirmUpdate: '\u786e\u8ba4\u4fdd\u5b58\u8be5 GA \u5b9e\u4f8b\u7684\u4fee\u6539\uff1f',
    confirmDefault: name => `\u786e\u8ba4\u5c06\u201c${name}\u201d\u8bbe\u4e3a\u9ed8\u8ba4\u5b9e\u4f8b\uff1f`,
    deleteTitle: '\u786e\u8ba4\u5220\u9664\u5b9e\u4f8b', deleteConfirm: '\u786e\u8ba4\u5220\u9664',
    confirmDelete: name => `\u5373\u5c06\u4ece GenericAgent-Admin \u4e2d\u79fb\u9664\u201c${name}\u201d\u3002\u8be5\u64cd\u4f5c\u4e0d\u4f1a\u5220\u9664\u78c1\u76d8\u4e0a\u7684 GenericAgent \u76ee\u5f55\u3002`,
    defaultDeleteHint: '\u8bf7\u5148\u5c06\u5176\u4ed6\u5b9e\u4f8b\u8bbe\u4e3a\u9ed8\u8ba4\u3002',
  },
  en: {
    title: 'GA instance management',
    summary: 'Each GenericAgent instance owns an isolated project root, Python environment, chat data, scheduled tasks, autonomous records, memory, and model settings. Switching instances changes the whole workspace; unscoped requests use the default instance.',
    helpLabel: 'Learn how GA instances work', helpTitle: 'What is a GA instance?',
    helpText: 'Each instance is an independent GA project copy: not only the runtime directory, but also chat, schedules, autonomous records, memory, models, and usage data are isolated.',
    helpSteps: ['Create: enter an ID, display name, and root directory; Python can be left blank for auto-detection.', 'Clone: selecting an existing instance copies its program, tasks, and project preferences; service autostart stays off to avoid port collisions.', 'Switch: the instance selector reloads files, chat, tasks, autonomous work, memory, models, and usage together; unscoped requests use the default instance.'],
    helpNote: 'Deleting an instance removes only its admin registry entry; its GenericAgent directory stays on disk. Stop active chat, loops, or title generation first; local CMD sessions close with the instance. Memory and mykey.py are copied only when explicitly enabled.',
    add: 'Add instance', install: 'One-click add', installing: 'Downloading and adding\u2026', refresh: 'Refresh', loading: 'Loading instances\u2026', empty: 'No GA instances configured',
    default: 'Default', setDefault: 'Set as default', configureModels: 'Configure models', edit: 'Edit', remove: 'Delete', cancel: 'Cancel',
    initializing: 'Initializing', ready: 'Ready', failed: 'Initialization failed', initError: 'Error details',
    stages: { queued: 'Waiting to start', preparing: 'Preparing install directory', cloning: 'Copying the existing GenericAgent project', downloading: 'Downloading and extracting GenericAgent', extracting: 'Extracting uploaded template', verifying: 'Verifying installed files', finalizing: 'Saving instance configuration', complete: 'Initialization complete' },
    createTitle: 'Create GA instance', editTitle: 'Edit GA instance', installTitle: 'Choose the new instance ID', create: 'Create instance', save: 'Save changes', startInstall: 'Start creating',
    createSummary: 'Set the instance identity and local runtime.', editSummary: 'Update the display name or runtime paths.', installSummary: 'Choose a stable ID. Creation continues in the background.',
    identityGroup: 'Instance identity', runtimeGroup: 'Runtime environment', sourceGroup: 'Initialization source', requiredField: 'Required', optionalField: 'Optional', manualSource: 'Choose a new root directory', sourceHint: 'Copies the selected instance program, tasks, autonomous records, and project preferences. Service autostart stays off to avoid port collisions. Leave the root blank to allocate an isolated directory automatically.', copyMemory: 'Copy existing memory', copyMemoryHint: 'Memory may contain personalized context; enable this only when inheritance is intended.', copyMyKey: 'Copy mykey.py', copyMyKeyHint: 'Contains API keys or tokens. Enable only when you accept copying secrets.', active: 'In use', switchTo: 'Switch to this instance', switchHint: 'Switching reloads files, tasks, memory, models, chat, usage, and service state.',
    template: 'GA.zip template (optional)', templateHint: 'An uploaded .zip is saved persistently, replaces the current template, and initializes this instance.',
    reuseTemplate: 'Use the saved GA.zip template', templateReady: 'A saved template is ready to reuse.', templateMissing: 'No saved template yet. Leave empty to download the main branch.',
    id: 'Instance ID', name: 'Display name', root: 'GenericAgent root', python: 'Python path', effectivePython: 'Effective Python', auto: 'Auto-detected',
    idHint: 'Set once at creation. Use a short, stable identifier.', rootHint: 'This directory should contain agentmain.py.', pythonHint: 'Leave blank to let the backend detect Python.',
    required: 'Instance ID, display name, and GenericAgent root are required.',
    loadFailed: 'Failed to load instances', saved: 'Instance saved', installed: 'Instance added and initializing in the background', creating: 'Instance registered; copying the project in the background', defaultSaved: 'Default instance updated', removed: 'Instance deleted',
    confirmInstall: 'Download the GenericAgent main branch and register it as a new instance?',
    confirmCreate: 'Create this GA instance?', confirmUpdate: 'Save changes to this GA instance?',
    confirmDefault: name => `Set "${name}" as the default instance?`,
    deleteTitle: 'Confirm instance deletion', deleteConfirm: 'Delete instance',
    confirmDelete: name => `"${name}" will be removed from GenericAgent-Admin. This will not delete the GenericAgent directory from disk. Active chat, loops, or title generation must be stopped first.`,
    defaultDeleteHint: 'Set another instance as default before deleting this one.',
  },
}

const normalizedItems = (payload) => Array.isArray(payload?.items) ? payload.items : []
const normalizedInitStatus = (instance) => String(instance?.init_status || '').trim().toLowerCase()
const isInitializingInstance = (instance) => normalizedInitStatus(instance) === 'initializing'
const PROTECTED_DEFAULT_INSTANCE_ID = 'default'
const INSTANCE_POLL_MS = 1200

export default function InstancesPage({ lang = 'zh', onConfigureModels, activeInstanceID = '', onSelectInstance, onInstancesChange }) {
  const copy = TEXT[lang] || TEXT.en
  const [items, setItems] = useState([])
  const [defaultID, setDefaultID] = useState('')
  const [loading, setLoading] = useState(true)
  const [busy, setBusy] = useState('')
  const [error, setError] = useState('')
  const [notice, setNotice] = useState('')
  const [helpOpen, setHelpOpen] = useState(false)
  const [editor, setEditor] = useState(null)
  const [form, setForm] = useState(EMPTY_FORM)
  const [templateAvailable, setTemplateAvailable] = useState(false)
  const [useTemplate, setUseTemplate] = useState(false)
  const [templateFile, setTemplateFile] = useState(null)
  const [deleteTarget, setDeleteTarget] = useState(null)
  const deleteCancelRef = useRef(null)
  const deleteTriggerRef = useRef(null)

  const applyPayload = useCallback((payload) => {
    setItems(normalizedItems(payload))
    setDefaultID(String(payload?.default_instance_id || ''))
    setTemplateAvailable(Boolean(payload?.template_available))
  }, [])

  const notifyInstancesChanged = () => {
    onInstancesChange?.()
    if (typeof window !== 'undefined') window.dispatchEvent(new CustomEvent('ga-admin-instances-change'))
  }

  const loadInstances = useCallback(async ({ silent = false } = {}) => {
    if (!silent) {
      setLoading(true)
      setError('')
    }
    try {
      applyPayload(await api('/api/instances'))
    } catch (loadError) {
      if (!silent) setError(`${copy.loadFailed}: ${loadError.message}`)
    } finally {
      if (!silent) setLoading(false)
    }
  }, [applyPayload, copy.loadFailed])

  useEffect(() => { loadInstances() }, [loadInstances])

  const hasInitializing = items.some(isInitializingInstance)
  useEffect(() => {
    if (!hasInitializing) return undefined

    let cancelled = false
    let timer
    const poll = async () => {
      await loadInstances({ silent: true })
      if (!cancelled) timer = window.setTimeout(poll, INSTANCE_POLL_MS)
    }
    timer = window.setTimeout(poll, INSTANCE_POLL_MS)
    return () => {
      cancelled = true
      window.clearTimeout(timer)
    }
  }, [hasInitializing, loadInstances])

  const beginCreate = () => {
    setForm(EMPTY_FORM)
    setTemplateFile(null)
    setEditor('create')
    setError('')
    setNotice('')
  }

  const beginInstall = () => {
    setForm(EMPTY_FORM)
    setTemplateFile(null)
    setUseTemplate(templateAvailable)
    setEditor('install')
    setError('')
    setNotice('')
  }

  const beginEdit = (instance) => {
    setForm({
      id: String(instance.id || ''),
      name: String(instance.name || ''),
      ga_root: String(instance.ga_root || ''),
      python_path: String(instance.python_path || ''),
      source_instance_id: '',
      copy_memory: false,
      copy_mykey: false,
    })
    setEditor(instance.id)
    setError('')
    setNotice('')
  }

  const patchForm = (key, value) => setForm(current => ({ ...current, [key]: value }))

  const submit = async (event) => {
    event.preventDefault()
    const payload = {
      id: String(form.id || '').trim(),
      name: String(form.name || '').trim(),
      ga_root: String(form.ga_root || '').trim(),
      python_path: String(form.python_path || '').trim(),
    }
    const installing = editor === 'install'
    const creating = editor === 'create'
    const sourceInstanceID = creating ? String(form.source_instance_id || '').trim() : ''
    if (!payload.id || (!installing && (!payload.name || (!sourceInstanceID && !payload.ga_root)))) {
      setError(copy.required)
      return
    }
    const action = installing ? 'install_instance' : creating ? 'create_instance' : 'update_instance'
    const prompt = installing ? copy.confirmInstall : creating ? copy.confirmCreate : copy.confirmUpdate
    if (!await confirmDanger(action, prompt)) return
    setBusy(installing ? 'install' : 'save')
    setError('')
    setNotice('')
    try {
      let body
      if (installing && templateFile) {
        body = new FormData()
        body.append('id', payload.id)
        body.append('use_template', 'true')
        body.append('template', templateFile)
      } else {
        const createPayload = sourceInstanceID
          ? { ...payload, source_instance_id: sourceInstanceID, copy_memory: Boolean(form.copy_memory), copy_mykey: Boolean(form.copy_mykey) }
          : payload
        body = JSON.stringify(installing ? { id: payload.id, use_template: useTemplate } : createPayload)
      }
      const result = await api(installing ? '/api/instances/install' : creating ? '/api/instances/create' : '/api/instances/update', {
        method: installing || creating ? 'POST' : 'PUT',
        dangerous: true,
        body,
      })
      applyPayload(result)
      notifyInstancesChanged()
      setEditor(null)
      const cloning = creating && normalizedInitStatus(result?.instance) === 'initializing'
      setNotice(cloning ? copy.creating : installing ? copy.installed : copy.saved)
    } catch (saveError) {
      setError(saveError.message)
    } finally {
      setBusy('')
    }
  }

  const setDefault = async (instance) => {
    if (!await confirmDanger('set_default_instance', copy.confirmDefault(instance.name || instance.id))) return
    setBusy(`default:${instance.id}`)
    setError('')
    setNotice('')
    try {
      const result = await api('/api/instances/default', {
        method: 'PUT',
        dangerous: true,
        body: JSON.stringify({ id: instance.id }),
      })
      applyPayload(result)
      notifyInstancesChanged()
      setNotice(copy.defaultSaved)
    } catch (defaultError) {
      setError(defaultError.message)
    } finally {
      setBusy('')
    }
  }

  const requestDelete = (instance) => {
    deleteTriggerRef.current = document.activeElement
    setDeleteTarget(instance)
  }

  const cancelDelete = () => {
    setDeleteTarget(null)
    window.setTimeout(() => deleteTriggerRef.current?.focus(), 0)
  }

  useEffect(() => {
    if (!deleteTarget) return undefined
    deleteCancelRef.current?.focus()
    const onKeyDown = (event) => {
      if (event.key === 'Escape' && !busy) cancelDelete()
    }
    document.addEventListener('keydown', onKeyDown)
    return () => document.removeEventListener('keydown', onKeyDown)
  }, [deleteTarget, busy])

  const remove = async (instance) => {
    setDeleteTarget(null)
    setBusy(`delete:${instance.id}`)
    setError('')
    setNotice('')
    try {
      const result = await api('/api/instances/delete', {
        method: 'DELETE',
        dangerous: true,
        body: JSON.stringify({ id: instance.id }),
      })
      applyPayload(result)
      notifyInstancesChanged()
      if (editor === instance.id) setEditor(null)
      setNotice(copy.removed)
    } catch (deleteError) {
      setError(deleteError.message)
    } finally {
      setBusy('')
    }
  }

  const anyBusy = Boolean(busy)
  const sourceInstanceID = String(form.source_instance_id || '').trim()
  const sourceInstance = items.find(instance => String(instance.id || '') === sourceInstanceID)

  return <section className="instances-page" aria-busy={anyBusy || loading}>
    <div className="instances-hero">
      <div className="instances-hero-mark"><Server size={24}/></div>
      <div className="instances-hero-copy">
        <div className="instances-title-row">
          <h2>{copy.title}</h2>
          <div className={`instances-help${helpOpen ? ' is-open' : ''}`} onKeyDown={event => { if (event.key === 'Escape') setHelpOpen(false) }}>
            <button
              type="button"
              className="instances-help-trigger"
              aria-label={copy.helpLabel}
              aria-expanded={helpOpen}
              aria-controls="instances-help-popover"
              title={copy.helpLabel}
              onClick={() => setHelpOpen(current => !current)}
            >
              <CircleHelp size={17}/>
            </button>
            <div id="instances-help-popover" className="instances-help-popover" role="tooltip">
              <strong>{copy.helpTitle}</strong>
              <p>{copy.helpText}</p>
              <ul>{copy.helpSteps.map(step => <li key={step}>{step}</li>)}</ul>
              <small>{copy.helpNote}</small>
            </div>
          </div>
        </div>
        <p>{copy.summary}</p>
      </div>
      <div className="instances-toolbar">
        <button type="button" className="ghost" onClick={loadInstances} disabled={loading || anyBusy}><RefreshCw size={15}/>{copy.refresh}</button>
        <button type="button" onClick={beginCreate} disabled={anyBusy}><Plus size={16}/>{copy.add}</button>
        <button type="button" className="primary instances-install-button" onClick={beginInstall} disabled={loading || anyBusy || editor === 'install'}>
          <Download size={16}/> {copy.install}
        </button>
      </div>
    </div>

    {(error || notice) && <div className={`instances-message ${error ? 'error' : 'success'}`} role="status">
      {error ? <X size={16}/> : <CheckCircle2 size={16}/>}<span>{error || notice}</span>
    </div>}

    {editor && <form className={`instance-editor is-${editor}`} onSubmit={submit} aria-labelledby="instance-editor-title">
      <div className="instance-editor-heading">
        <div className="instance-editor-heading-icon" aria-hidden="true">{editor === 'install' ? <Download size={18}/> : editor === 'create' ? <Plus size={18}/> : <Pencil size={17}/>}</div>
        <div className="instance-editor-heading-copy">
          <h3 id="instance-editor-title">{editor === 'create' ? copy.createTitle : editor === 'install' ? copy.installTitle : copy.editTitle}</h3>
          <p>{editor === 'create' ? copy.createSummary : editor === 'install' ? copy.installSummary : copy.editSummary}</p>
        </div>
      </div>
      <div className="instance-editor-body">
        <fieldset className="instance-editor-section">
          <legend><span aria-hidden="true">01</span>{copy.identityGroup}</legend>
          <div className="instance-editor-grid instance-editor-grid-identity">
            <label htmlFor="instance-id"><span>{copy.id}<em>{copy.requiredField}</em></span><input id="instance-id" aria-label={copy.id} value={form.id} disabled={editor !== 'create' && editor !== 'install' || anyBusy} onChange={event => patchForm('id', event.target.value)} required/><small>{copy.idHint}</small></label>
            {editor !== 'install' && <label htmlFor="instance-name"><span>{copy.name}<em>{copy.requiredField}</em></span><input id="instance-name" aria-label={copy.name} value={form.name} disabled={anyBusy} onChange={event => patchForm('name', event.target.value)} required/></label>}
          </div>
        </fieldset>
        {editor === 'create' && <fieldset className="instance-editor-section instance-clone-section">
          <legend><span aria-hidden="true">02</span>{copy.sourceGroup}</legend>
          <div className="instance-editor-grid">
            <label htmlFor="instance-source"><span>{copy.sourceGroup}<em>{copy.optionalField}</em></span><select id="instance-source" aria-label={copy.sourceGroup} value={sourceInstanceID} disabled={anyBusy} onChange={event => patchForm('source_instance_id', event.target.value)}><option value="">{copy.manualSource}</option>{items.filter(instance => String(instance.id || '') !== String(form.id || '') && !isInitializingInstance(instance)).map(instance => <option key={instance.id} value={instance.id}>{instance.name || instance.id}</option>)}</select><small>{sourceInstance ? `${copy.sourceHint} ${sourceInstance.name || sourceInstance.id}` : copy.sourceHint}</small></label>
            <div className="instance-copy-options" aria-label={copy.sourceGroup}>
              <label><span><input type="checkbox" aria-label={copy.copyMemory} checked={Boolean(form.copy_memory)} disabled={anyBusy || !sourceInstanceID} onChange={event => patchForm('copy_memory', event.target.checked)}/>{copy.copyMemory}</span><small>{copy.copyMemoryHint}</small></label>
              <label><span><input type="checkbox" aria-label={copy.copyMyKey} checked={Boolean(form.copy_mykey)} disabled={anyBusy || !sourceInstanceID} onChange={event => patchForm('copy_mykey', event.target.checked)}/>{copy.copyMyKey}</span><small>{copy.copyMyKeyHint}</small></label>
            </div>
          </div>
        </fieldset>}
        {editor !== 'install' && <fieldset className="instance-editor-section">
          <legend><span aria-hidden="true">{editor === 'create' ? '03' : '02'}</span>{copy.runtimeGroup}</legend>
          <div className="instance-editor-grid">
            <label htmlFor="instance-root"><span>{copy.root}<em>{sourceInstanceID ? copy.optionalField : copy.requiredField}</em></span><input id="instance-root" aria-label={copy.root} value={form.ga_root} disabled={anyBusy} onChange={event => patchForm('ga_root', event.target.value)} required={!sourceInstanceID}/><small>{sourceInstanceID ? copy.sourceHint : copy.rootHint}</small></label>
            <label htmlFor="instance-python"><span>{copy.python}</span><input id="instance-python" aria-label={copy.python} value={form.python_path} disabled={anyBusy} onChange={event => patchForm('python_path', event.target.value)}/><small>{copy.pythonHint}</small></label>
          </div>
        </fieldset>}
        {editor === 'install' && <fieldset className="instance-editor-section">
          <legend><span aria-hidden="true">02</span>{copy.sourceGroup}</legend>
          <div className="instance-editor-grid">
            <label className="instance-template-reuse" htmlFor="instance-use-template"><span><input id="instance-use-template" aria-label={copy.reuseTemplate} type="checkbox" checked={Boolean(templateFile) || useTemplate} disabled={anyBusy || Boolean(templateFile) || !templateAvailable} onChange={event => setUseTemplate(event.target.checked)}/>{copy.reuseTemplate}</span><small>{templateAvailable ? copy.templateReady : copy.templateMissing}</small></label>
            <label htmlFor="instance-template"><span>{copy.template}</span><input id="instance-template" aria-label={copy.template} type="file" accept=".zip,application/zip" disabled={anyBusy} onChange={event => { const file = event.target.files?.[0] || null; setTemplateFile(file); if (file) setUseTemplate(true) }}/><small>{copy.templateHint}</small></label>
          </div>
        </fieldset>}
      </div>
      <div className="instance-editor-footer">
        <div className="instance-editor-actions"><button type="button" onClick={() => setEditor(null)} disabled={anyBusy}>{copy.cancel}</button><button type="submit" className="primary" disabled={anyBusy}>{anyBusy ? <RefreshCw className="instances-spin" size={15}/> : <Save size={15}/>} {editor === 'create' ? copy.create : editor === 'install' ? copy.startInstall : copy.save}</button></div>
      </div>
    </form>}

    {loading ? <div className="instances-empty"><RefreshCw className="spin" size={22}/><span>{copy.loading}</span></div> : items.length === 0 ? <div className="instances-empty"><Server size={28}/><span>{copy.empty}</span><button type="button" onClick={beginCreate}><Plus size={15}/>{copy.add}</button></div> : <div className="instances-grid">
      {items.map(instance => {
        const isDefault = instance.id === defaultID
        const initStatus = normalizedInitStatus(instance)
        const isInitializing = initStatus === 'initializing'
        const hasInitFailed = initStatus === 'failed'
        const initLabel = isInitializing ? copy.initializing : hasInitFailed ? copy.failed : initStatus === 'ready' ? copy.ready : ''
        const initStage = String(instance.init_stage || '').trim().toLowerCase()
        const initProgress = Math.max(0, Math.min(100, Number(instance.init_progress) || 0))
        const initStageLabel = copy.stages[initStage] || copy.initializing
        const showInitProgress = (isInitializing || hasInitFailed) && Boolean(initStage || initProgress > 0)
        const isProtectedDefault = instance.id === PROTECTED_DEFAULT_INSTANCE_ID
        const blocksDefaultDelete = isProtectedDefault || isDefault && items.length > 1
        const isActive = String(activeInstanceID || '') === String(instance.id || '')
        return <article className={`instance-card${isDefault ? ' is-default' : ''}${isActive ? ' is-active' : ''}${isInitializing ? ' is-initializing' : ''}${hasInitFailed ? ' has-init-failed' : ''}`} key={instance.id}>
          <header>
            <span className="instance-card-icon"><Cpu size={19}/></span>
            <div><h3>{instance.name || instance.id}</h3><code>{instance.id}</code></div>
            <div className="instance-card-badges">
              {isDefault && <span className="instance-default-badge"><Star size={13}/>{copy.default}</span>}
              {isActive && <span className="instance-active-badge"><CheckCircle2 size={13}/>{copy.active}</span>}
              {initLabel && <span className={`instance-status-badge is-${initStatus}`}>
                {isInitializing ? <RefreshCw className="instances-spin" size={13}/> : hasInitFailed ? <X size={13}/> : <CheckCircle2 size={13}/>}
                {initLabel}
              </span>}
            </div>
          </header>
          <dl>
            <div><dt>{copy.root}</dt><dd title={instance.ga_root || ''}>{instance.ga_root || '-'}</dd></div>
            <div><dt>{copy.python}</dt><dd title={instance.python_path || ''}>{instance.python_path || copy.auto}</dd></div>
            <div><dt>{copy.effectivePython}</dt><dd title={instance.effective_python || ''}>{instance.effective_python || copy.auto}</dd></div>
          </dl>
          {showInitProgress && <div className={`instance-init-progress${hasInitFailed ? ' has-failed' : ''}`}>
            <div className="instance-init-progress-copy"><span>{initStageLabel}</span><strong>{initProgress}%</strong></div>
            <progress aria-label={initStageLabel} max="100" value={initProgress}>{initProgress}%</progress>
          </div>}
          {hasInitFailed && instance.init_error && <div className="instance-init-error" role="alert"><strong>{copy.initError}</strong><span>{instance.init_error}</span></div>}
          <footer>
            {onSelectInstance && <button type="button" className={isActive ? 'active-instance-action' : ''} onClick={() => onSelectInstance(instance.id)} disabled={anyBusy || isActive || isInitializing} title={copy.switchHint}><RefreshCw size={14}/>{isActive ? copy.active : copy.switchTo}</button>}
            <button type="button" onClick={() => onConfigureModels?.(instance)} disabled={anyBusy || isInitializing}><Settings2 size={14}/>{copy.configureModels}</button>
            <button type="button" onClick={() => beginEdit(instance)} disabled={anyBusy || isInitializing}><Pencil size={14}/>{copy.edit}</button>
            <button type="button" onClick={() => setDefault(instance)} disabled={anyBusy || isDefault || isInitializing}><Star size={14}/>{copy.setDefault}</button>
            <button type="button" className="danger" title={blocksDefaultDelete ? copy.defaultDeleteHint : ''} onClick={() => requestDelete(instance)} disabled={anyBusy || blocksDefaultDelete}><Trash2 size={14}/>{copy.remove}</button>
          </footer>
        </article>
      })}
    </div>}

    {deleteTarget && <div className="instance-delete-backdrop" onMouseDown={event => {
      if (event.target === event.currentTarget && !busy) cancelDelete()
    }}>
      <section className="instance-delete-dialog" role="dialog" aria-modal="true" aria-labelledby="instance-delete-title" aria-describedby="instance-delete-description">
        <div className="instance-delete-icon" aria-hidden="true"><AlertTriangle size={22}/></div>
        <div className="instance-delete-copy">
          <h3 id="instance-delete-title">{copy.deleteTitle}</h3>
          <p id="instance-delete-description">{copy.confirmDelete(deleteTarget.name || deleteTarget.id)}</p>
          <code>{deleteTarget.id}</code>
        </div>
        <div className="instance-delete-actions">
          <button ref={deleteCancelRef} type="button" onClick={cancelDelete} disabled={Boolean(busy)}>{copy.cancel}</button>
          <button type="button" className="danger" onClick={() => remove(deleteTarget)} disabled={Boolean(busy)}><Trash2 size={15}/>{copy.deleteConfirm}</button>
        </div>
      </section>
    </div>}
  </section>
}
