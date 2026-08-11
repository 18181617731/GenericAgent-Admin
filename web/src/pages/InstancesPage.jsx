import React, { useCallback, useEffect, useState } from 'react'
import { CheckCircle2, CircleHelp, Cpu, Download, Pencil, Plus, RefreshCw, Save, Server, Star, Trash2, X } from 'lucide-react'
import { api } from '../lib/api'
import { confirmDanger } from '../lib/danger'

const EMPTY_FORM = { id: '', name: '', ga_root: '', python_path: '' }

const TEXT = {
  zh: {
    title: 'GA \u5b9e\u4f8b\u7ba1\u7406',
    summary: '\u4e3a\u6bcf\u4e2a GenericAgent \u8fd0\u884c\u65f6\u7ef4\u62a4\u72ec\u7acb\u7684\u8def\u5f84\u548c Python \u73af\u5883\u3002\u9ed8\u8ba4\u5b9e\u4f8b\u4f1a\u627f\u63a5\u672a\u663e\u5f0f\u6307\u5b9a\u5b9e\u4f8b\u7684\u8bf7\u6c42\u3002',
    helpLabel: '\u4e86\u89e3 GA \u5b9e\u4f8b\u4f7f\u7528\u8bf4\u660e', helpTitle: 'GA \u5b9e\u4f8b\u662f\u4ec0\u4e48\uff1f',
    helpText: '\u6bcf\u4e2a\u5b9e\u4f8b\u5bf9\u5e94\u4e00\u4e2a\u72ec\u7acb\u7684 GenericAgent \u8fd0\u884c\u76ee\u5f55\u548c Python \u73af\u5883\u3002',
    helpSteps: ['\u65b0\u5efa\uff1a\u586b\u5199 ID\u3001\u540d\u79f0\u548c\u6839\u76ee\u5f55\uff1bPython \u8def\u5f84\u53ef\u7559\u7a7a\u81ea\u52a8\u68c0\u6d4b\u3002', '\u4e00\u952e\u65b0\u589e\uff1a\u81ea\u52a8\u4e0b\u8f7d\u5e76\u6ce8\u518c\u65b0\u5b9e\u4f8b\uff0c\u4e5f\u53ef\u4e0a\u4f20 GA.zip \u6a21\u677f\u3002', '\u8bbe\u4e3a\u9ed8\u8ba4\uff1a\u672a\u6307\u5b9a\u5b9e\u4f8b\u7684\u8bf7\u6c42\u4f1a\u4f7f\u7528\u5b83\uff1b\u5bf9\u8bdd\u9875\u53ef\u4ece\u4fa7\u680f\u5207\u6362\u3002'],
    helpNote: '\u5220\u9664\u53ea\u79fb\u9664\u7ba1\u7406\u53f0\u8bb0\u5f55\uff0c\u4e0d\u4f1a\u5220\u9664\u78c1\u76d8\u4e0a\u7684 GenericAgent \u76ee\u5f55\u3002',
    add: '\u65b0\u5efa\u5b9e\u4f8b', install: '\u4e00\u952e\u65b0\u589e', installing: '\u6b63\u5728\u4e0b\u8f7d\u5e76\u65b0\u589e\u2026', refresh: '\u5237\u65b0', loading: '\u6b63\u5728\u8bfb\u53d6\u5b9e\u4f8b\u2026', empty: '\u6682\u65e0 GA \u5b9e\u4f8b',
    default: '\u9ed8\u8ba4', setDefault: '\u8bbe\u4e3a\u9ed8\u8ba4', edit: '\u7f16\u8f91', remove: '\u5220\u9664', cancel: '\u53d6\u6d88',
    initializing: '\u521d\u59cb\u5316\u4e2d', ready: '\u5df2\u5c31\u7eea', failed: '\u521d\u59cb\u5316\u5931\u8d25', initError: '\u9519\u8bef\u8be6\u60c5',
    createTitle: '\u65b0\u5efa GA \u5b9e\u4f8b', editTitle: '\u7f16\u8f91 GA \u5b9e\u4f8b', installTitle: '填写新实例 ID', create: '\u521b\u5efa\u5b9e\u4f8b', save: '\u4fdd\u5b58\u4fee\u6539', startInstall: '开始创建',
    createSummary: '设置实例标识与本地运行环境。', editSummary: '更新显示名称或运行时路径。', installSummary: '选择稳定的标识，创建任务将在后台运行。',
    identityGroup: '实例标识', runtimeGroup: '运行环境', sourceGroup: '初始化来源', requiredField: '必填',
    template: 'GA.zip \u6a21\u677f\u5305\uff08\u53ef\u9009\uff09', templateHint: '\u4e0a\u4f20 .zip \u540e\u5c06\u4f7f\u7528\u8be5\u6a21\u677f\u521d\u59cb\u5316\uff1b\u7559\u7a7a\u5219\u4ece main \u5206\u652f\u4e0b\u8f7d\u3002',
    id: '\u5b9e\u4f8b ID', name: '\u663e\u793a\u540d\u79f0', root: 'GenericAgent \u6839\u76ee\u5f55', python: 'Python \u8def\u5f84', effectivePython: '\u5b9e\u9645 Python', auto: '\u81ea\u52a8\u68c0\u6d4b',
    idHint: '\u4ec5\u5efa\u7acb\u65f6\u53ef\u8bbe\u7f6e\uff0c\u5efa\u8bae\u4f7f\u7528\u7b80\u77ed\u4e14\u7a33\u5b9a\u7684\u6807\u8bc6\u3002', rootHint: '\u8be5\u76ee\u5f55\u5e94\u5305\u542b agentmain.py\u3002', pythonHint: '\u7559\u7a7a\u65f6\u7531\u540e\u7aef\u81ea\u52a8\u68c0\u6d4b\u3002',
    required: '\u8bf7\u586b\u5199\u5b9e\u4f8b ID\u3001\u540d\u79f0\u548c GenericAgent \u6839\u76ee\u5f55\u3002',
    loadFailed: '\u8bfb\u53d6\u5b9e\u4f8b\u5931\u8d25', saved: '\u5b9e\u4f8b\u5df2\u4fdd\u5b58', installed: '\u5b9e\u4f8b\u5df2\u65b0\u589e\uff0c\u6b63\u5728\u540e\u53f0\u521d\u59cb\u5316', defaultSaved: '\u9ed8\u8ba4\u5b9e\u4f8b\u5df2\u66f4\u65b0', removed: '\u5b9e\u4f8b\u5df2\u5220\u9664',
    confirmInstall: '\u5c06\u4ece GenericAgent main \u5206\u652f\u4e0b\u8f7d\u6e90\u7801\u5e76\u81ea\u52a8\u6ce8\u518c\u4e3a\u65b0\u5b9e\u4f8b\uff0c\u7ee7\u7eed\u5417\uff1f',
    confirmCreate: '\u786e\u8ba4\u521b\u5efa\u8be5 GA \u5b9e\u4f8b\uff1f', confirmUpdate: '\u786e\u8ba4\u4fdd\u5b58\u8be5 GA \u5b9e\u4f8b\u7684\u4fee\u6539\uff1f',
    confirmDefault: name => `\u786e\u8ba4\u5c06\u201c${name}\u201d\u8bbe\u4e3a\u9ed8\u8ba4\u5b9e\u4f8b\uff1f`,
    confirmDelete: name => `\u786e\u8ba4\u5220\u9664\u201c${name}\u201d\uff1f\u8be5\u64cd\u4f5c\u4e0d\u4f1a\u5220\u9664\u78c1\u76d8\u4e0a\u7684 GenericAgent \u76ee\u5f55\u3002`,
    defaultDeleteHint: '\u8bf7\u5148\u5c06\u5176\u4ed6\u5b9e\u4f8b\u8bbe\u4e3a\u9ed8\u8ba4\u3002',
  },
  en: {
    title: 'GA instance management',
    summary: 'Maintain an isolated path and Python environment for each GenericAgent runtime. The default instance handles requests that do not explicitly select one.',
    helpLabel: 'Learn how GA instances work', helpTitle: 'What is a GA instance?',
    helpText: 'Each instance maps to an isolated GenericAgent directory and Python environment.',
    helpSteps: ['Create: enter an ID, display name, and root directory; Python can be left blank for auto-detection.', 'One-click add: download and register a new instance, or upload a GA.zip template.', 'Set as default: requests without an explicit instance use it; switch instances from the chat sidebar.'],
    helpNote: 'Deleting an instance removes only its admin registry entry; the GenericAgent directory on disk is not deleted.',
    add: 'Add instance', install: 'One-click add', installing: 'Downloading and adding\u2026', refresh: 'Refresh', loading: 'Loading instances\u2026', empty: 'No GA instances configured',
    default: 'Default', setDefault: 'Set as default', edit: 'Edit', remove: 'Delete', cancel: 'Cancel',
    initializing: 'Initializing', ready: 'Ready', failed: 'Initialization failed', initError: 'Error details',
    createTitle: 'Create GA instance', editTitle: 'Edit GA instance', installTitle: 'Choose the new instance ID', create: 'Create instance', save: 'Save changes', startInstall: 'Start creating',
    createSummary: 'Set the instance identity and local runtime.', editSummary: 'Update the display name or runtime paths.', installSummary: 'Choose a stable ID. Creation continues in the background.',
    identityGroup: 'Instance identity', runtimeGroup: 'Runtime environment', sourceGroup: 'Initialization source', requiredField: 'Required',
    template: 'GA.zip template (optional)', templateHint: 'Use this .zip to initialize the instance, or leave empty to download the main branch.',
    id: 'Instance ID', name: 'Display name', root: 'GenericAgent root', python: 'Python path', effectivePython: 'Effective Python', auto: 'Auto-detected',
    idHint: 'Set once at creation. Use a short, stable identifier.', rootHint: 'This directory should contain agentmain.py.', pythonHint: 'Leave blank to let the backend detect Python.',
    required: 'Instance ID, display name, and GenericAgent root are required.',
    loadFailed: 'Failed to load instances', saved: 'Instance saved', installed: 'Instance added and initializing in the background', defaultSaved: 'Default instance updated', removed: 'Instance deleted',
    confirmInstall: 'Download the GenericAgent main branch and register it as a new instance?',
    confirmCreate: 'Create this GA instance?', confirmUpdate: 'Save changes to this GA instance?',
    confirmDefault: name => `Set "${name}" as the default instance?`,
    confirmDelete: name => `Delete "${name}"? This does not remove the GenericAgent directory from disk.`,
    defaultDeleteHint: 'Set another instance as default before deleting this one.',
  },
}

const normalizedItems = (payload) => Array.isArray(payload?.items) ? payload.items : []
const normalizedInitStatus = (instance) => String(instance?.init_status || '').trim().toLowerCase()
const isInitializingInstance = (instance) => normalizedInitStatus(instance) === 'initializing'
const INSTANCE_POLL_MS = 1200

export default function InstancesPage({ lang = 'zh' }) {
  const copy = TEXT[lang] || TEXT.en
  const [items, setItems] = useState([])
  const [defaultID, setDefaultID] = useState('')
  const [loading, setLoading] = useState(true)
  const [busy, setBusy] = useState('')
  const [error, setError] = useState('')
  const [notice, setNotice] = useState('')
  const [editor, setEditor] = useState(null)
  const [form, setForm] = useState(EMPTY_FORM)
  const [templateFile, setTemplateFile] = useState(null)
  const [helpOpen, setHelpOpen] = useState(false)

  const applyPayload = useCallback((payload) => {
    setItems(normalizedItems(payload))
    setDefaultID(String(payload?.default_instance_id || ''))
  }, [])

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
    })
    setEditor(instance.id)
    setError('')
    setNotice('')
  }

  const patchForm = (key, value) => setForm(current => ({ ...current, [key]: value }))

  const submit = async (event) => {
    event.preventDefault()
    const payload = Object.fromEntries(Object.entries(form).map(([key, value]) => [key, String(value || '').trim()]))
    const installing = editor === 'install'
    if (!payload.id || (!installing && (!payload.name || !payload.ga_root))) {
      setError(copy.required)
      return
    }
    const creating = editor === 'create'
    const action = installing ? 'install_instance' : creating ? 'create_instance' : 'update_instance'
    const prompt = installing ? copy.confirmInstall : creating ? copy.confirmCreate : copy.confirmUpdate
    if (!confirmDanger(action, prompt)) return
    setBusy(installing ? 'install' : 'save')
    setError('')
    setNotice('')
    try {
      let body
      if (installing && templateFile) {
        body = new FormData()
        body.append('id', payload.id)
        body.append('template', templateFile)
      } else {
        body = JSON.stringify(installing ? { id: payload.id } : payload)
      }
      const result = await api(installing ? '/api/instances/install' : creating ? '/api/instances/create' : '/api/instances/update', {
        method: installing || creating ? 'POST' : 'PUT',
        dangerous: true,
        body,
      })
      applyPayload(result)
      setEditor(null)
      setNotice(installing ? copy.installed : copy.saved)
    } catch (saveError) {
      setError(saveError.message)
    } finally {
      setBusy('')
    }
  }

  const setDefault = async (instance) => {
    if (!confirmDanger('set_default_instance', copy.confirmDefault(instance.name || instance.id))) return
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
      setNotice(copy.defaultSaved)
    } catch (defaultError) {
      setError(defaultError.message)
    } finally {
      setBusy('')
    }
  }

  const remove = async (instance) => {
    if (!confirmDanger('delete_instance', copy.confirmDelete(instance.name || instance.id))) return
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
      if (editor === instance.id) setEditor(null)
      setNotice(copy.removed)
    } catch (deleteError) {
      setError(deleteError.message)
    } finally {
      setBusy('')
    }
  }

  const anyBusy = Boolean(busy)

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
        {editor !== 'install' && <fieldset className="instance-editor-section">
          <legend><span aria-hidden="true">02</span>{copy.runtimeGroup}</legend>
          <div className="instance-editor-grid">
            <label htmlFor="instance-root"><span>{copy.root}<em>{copy.requiredField}</em></span><input id="instance-root" aria-label={copy.root} value={form.ga_root} disabled={anyBusy} onChange={event => patchForm('ga_root', event.target.value)} required/><small>{copy.rootHint}</small></label>
            <label htmlFor="instance-python"><span>{copy.python}</span><input id="instance-python" aria-label={copy.python} value={form.python_path} disabled={anyBusy} onChange={event => patchForm('python_path', event.target.value)}/><small>{copy.pythonHint}</small></label>
          </div>
        </fieldset>}
        {editor === 'install' && <fieldset className="instance-editor-section">
          <legend><span aria-hidden="true">02</span>{copy.sourceGroup}</legend>
          <div className="instance-editor-grid"><label htmlFor="instance-template"><span>{copy.template}</span><input id="instance-template" aria-label={copy.template} type="file" accept=".zip,application/zip" disabled={anyBusy} onChange={event => setTemplateFile(event.target.files?.[0] || null)}/><small>{copy.templateHint}</small></label></div>
        </fieldset>}
      </div>
      <div className="instance-editor-footer">
        <div className="instance-editor-actions"><button type="button" onClick={() => setEditor(null)} disabled={anyBusy}>{copy.cancel}</button><button type="submit" className="primary" disabled={anyBusy}>{busy === 'install' ? <RefreshCw className="instances-spin" size={15}/> : <Save size={15}/>} {editor === 'create' ? copy.create : editor === 'install' ? copy.startInstall : copy.save}</button></div>
      </div>
    </form>}

    {loading ? <div className="instances-empty"><RefreshCw className="spin" size={22}/><span>{copy.loading}</span></div> : items.length === 0 ? <div className="instances-empty"><Server size={28}/><span>{copy.empty}</span><button type="button" onClick={beginCreate}><Plus size={15}/>{copy.add}</button></div> : <div className="instances-grid">
      {items.map(instance => {
        const isDefault = instance.id === defaultID
        const initStatus = normalizedInitStatus(instance)
        const isInitializing = initStatus === 'initializing'
        const hasInitFailed = initStatus === 'failed'
        const initLabel = isInitializing ? copy.initializing : hasInitFailed ? copy.failed : initStatus === 'ready' ? copy.ready : ''
        const blocksDefaultDelete = isDefault && items.length > 1
        return <article className={`instance-card${isDefault ? ' is-default' : ''}${isInitializing ? ' is-initializing' : ''}${hasInitFailed ? ' has-init-failed' : ''}`} key={instance.id}>
          <header>
            <span className="instance-card-icon"><Cpu size={19}/></span>
            <div><h3>{instance.name || instance.id}</h3><code>{instance.id}</code></div>
            <div className="instance-card-badges">
              {isDefault && <span className="instance-default-badge"><Star size={13}/>{copy.default}</span>}
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
          {hasInitFailed && instance.init_error && <div className="instance-init-error" role="alert"><strong>{copy.initError}</strong><span>{instance.init_error}</span></div>}
          <footer>
            <button type="button" onClick={() => beginEdit(instance)} disabled={anyBusy || isInitializing}><Pencil size={14}/>{copy.edit}</button>
            <button type="button" onClick={() => setDefault(instance)} disabled={anyBusy || isDefault || isInitializing}><Star size={14}/>{copy.setDefault}</button>
            <button type="button" className="danger" title={blocksDefaultDelete ? copy.defaultDeleteHint : ''} onClick={() => remove(instance)} disabled={anyBusy || blocksDefaultDelete}><Trash2 size={14}/>{copy.remove}</button>
          </footer>
        </article>
      })}
    </div>}
  </section>
}
