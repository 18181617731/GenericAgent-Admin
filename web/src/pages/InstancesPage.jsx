import React, { useCallback, useEffect, useState } from 'react'
import { CheckCircle2, Cpu, Download, Pencil, Plus, RefreshCw, Save, Server, Star, Trash2, X } from 'lucide-react'
import { api } from '../lib/api'
import { confirmDanger } from '../lib/danger'

const EMPTY_FORM = { id: '', name: '', ga_root: '', python_path: '' }

const TEXT = {
  zh: {
    title: 'GA \u5b9e\u4f8b\u7ba1\u7406',
    summary: '\u4e3a\u6bcf\u4e2a GenericAgent \u8fd0\u884c\u65f6\u7ef4\u62a4\u72ec\u7acb\u7684\u8def\u5f84\u548c Python \u73af\u5883\u3002\u9ed8\u8ba4\u5b9e\u4f8b\u4f1a\u627f\u63a5\u672a\u663e\u5f0f\u6307\u5b9a\u5b9e\u4f8b\u7684\u8bf7\u6c42\u3002',
    add: '\u65b0\u5efa\u5b9e\u4f8b', install: '\u4e00\u952e\u65b0\u589e', installing: '\u6b63\u5728\u4e0b\u8f7d\u5e76\u65b0\u589e\u2026', refresh: '\u5237\u65b0', loading: '\u6b63\u5728\u8bfb\u53d6\u5b9e\u4f8b\u2026', empty: '\u6682\u65e0 GA \u5b9e\u4f8b',
    default: '\u9ed8\u8ba4', setDefault: '\u8bbe\u4e3a\u9ed8\u8ba4', edit: '\u7f16\u8f91', remove: '\u5220\u9664', cancel: '\u53d6\u6d88',
    createTitle: '\u65b0\u5efa GA \u5b9e\u4f8b', editTitle: '\u7f16\u8f91 GA \u5b9e\u4f8b', create: '\u521b\u5efa\u5b9e\u4f8b', save: '\u4fdd\u5b58\u4fee\u6539',
    id: '\u5b9e\u4f8b ID', name: '\u663e\u793a\u540d\u79f0', root: 'GenericAgent \u6839\u76ee\u5f55', python: 'Python \u8def\u5f84', effectivePython: '\u5b9e\u9645 Python', auto: '\u81ea\u52a8\u68c0\u6d4b',
    idHint: '\u4ec5\u5efa\u7acb\u65f6\u53ef\u8bbe\u7f6e\uff0c\u5efa\u8bae\u4f7f\u7528\u7b80\u77ed\u4e14\u7a33\u5b9a\u7684\u6807\u8bc6\u3002', rootHint: '\u8be5\u76ee\u5f55\u5e94\u5305\u542b agentmain.py\u3002', pythonHint: '\u7559\u7a7a\u65f6\u7531\u540e\u7aef\u81ea\u52a8\u68c0\u6d4b\u3002',
    required: '\u8bf7\u586b\u5199\u5b9e\u4f8b ID\u3001\u540d\u79f0\u548c GenericAgent \u6839\u76ee\u5f55\u3002',
    loadFailed: '\u8bfb\u53d6\u5b9e\u4f8b\u5931\u8d25', saved: '\u5b9e\u4f8b\u5df2\u4fdd\u5b58', installed: 'GenericAgent \u5df2\u4e0b\u8f7d\u5e76\u65b0\u589e', defaultSaved: '\u9ed8\u8ba4\u5b9e\u4f8b\u5df2\u66f4\u65b0', removed: '\u5b9e\u4f8b\u5df2\u5220\u9664',
    confirmInstall: '\u5c06\u4ece GenericAgent main \u5206\u652f\u4e0b\u8f7d\u6e90\u7801\u5e76\u81ea\u52a8\u6ce8\u518c\u4e3a\u65b0\u5b9e\u4f8b\uff0c\u7ee7\u7eed\u5417\uff1f',
    confirmCreate: '\u786e\u8ba4\u521b\u5efa\u8be5 GA \u5b9e\u4f8b\uff1f', confirmUpdate: '\u786e\u8ba4\u4fdd\u5b58\u8be5 GA \u5b9e\u4f8b\u7684\u4fee\u6539\uff1f',
    confirmDefault: name => `\u786e\u8ba4\u5c06\u201c${name}\u201d\u8bbe\u4e3a\u9ed8\u8ba4\u5b9e\u4f8b\uff1f`,
    confirmDelete: name => `\u786e\u8ba4\u5220\u9664\u201c${name}\u201d\uff1f\u8be5\u64cd\u4f5c\u4e0d\u4f1a\u5220\u9664\u78c1\u76d8\u4e0a\u7684 GenericAgent \u76ee\u5f55\u3002`,
    defaultDeleteHint: '\u8bf7\u5148\u5c06\u5176\u4ed6\u5b9e\u4f8b\u8bbe\u4e3a\u9ed8\u8ba4\u3002',
  },
  en: {
    title: 'GA instance management',
    summary: 'Maintain an isolated path and Python environment for each GenericAgent runtime. The default instance handles requests that do not explicitly select one.',
    add: 'Add instance', install: 'One-click add', installing: 'Downloading and adding\u2026', refresh: 'Refresh', loading: 'Loading instances\u2026', empty: 'No GA instances configured',
    default: 'Default', setDefault: 'Set as default', edit: 'Edit', remove: 'Delete', cancel: 'Cancel',
    createTitle: 'Create GA instance', editTitle: 'Edit GA instance', create: 'Create instance', save: 'Save changes',
    id: 'Instance ID', name: 'Display name', root: 'GenericAgent root', python: 'Python path', effectivePython: 'Effective Python', auto: 'Auto-detected',
    idHint: 'Set once at creation. Use a short, stable identifier.', rootHint: 'This directory should contain agentmain.py.', pythonHint: 'Leave blank to let the backend detect Python.',
    required: 'Instance ID, display name, and GenericAgent root are required.',
    loadFailed: 'Failed to load instances', saved: 'Instance saved', installed: 'GenericAgent downloaded and added', defaultSaved: 'Default instance updated', removed: 'Instance deleted',
    confirmInstall: 'Download the GenericAgent main branch and register it as a new instance?',
    confirmCreate: 'Create this GA instance?', confirmUpdate: 'Save changes to this GA instance?',
    confirmDefault: name => `Set "${name}" as the default instance?`,
    confirmDelete: name => `Delete "${name}"? This does not remove the GenericAgent directory from disk.`,
    defaultDeleteHint: 'Set another instance as default before deleting this one.',
  },
}

const normalizedItems = (payload) => Array.isArray(payload?.items) ? payload.items : []

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

  const applyPayload = (payload) => {
    setItems(normalizedItems(payload))
    setDefaultID(String(payload?.default_instance_id || ''))
  }

  const loadInstances = useCallback(async () => {
    setLoading(true)
    setError('')
    try {
      applyPayload(await api('/api/instances'))
    } catch (loadError) {
      setError(`${copy.loadFailed}: ${loadError.message}`)
    } finally {
      setLoading(false)
    }
  }, [copy.loadFailed])

  useEffect(() => { loadInstances() }, [loadInstances])

  const beginCreate = () => {
    setForm(EMPTY_FORM)
    setEditor('create')
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
    if (!payload.id || !payload.name || !payload.ga_root) {
      setError(copy.required)
      return
    }
    const creating = editor === 'create'
    if (!confirmDanger(creating ? 'create_instance' : 'update_instance', creating ? copy.confirmCreate : copy.confirmUpdate)) return
    setBusy('save')
    setError('')
    setNotice('')
    try {
      const result = await api(creating ? '/api/instances/create' : '/api/instances/update', {
        method: creating ? 'POST' : 'PUT',
        dangerous: true,
        body: JSON.stringify(payload),
      })
      applyPayload(result)
      setEditor(null)
      setNotice(copy.saved)
    } catch (saveError) {
      setError(saveError.message)
    } finally {
      setBusy('')
    }
  }

  const install = async () => {
    if (!confirmDanger('install_instance', copy.confirmInstall)) return
    setBusy('install')
    setError('')
    setNotice('')
    try {
      const result = await api('/api/instances/install', {
        method: 'POST',
        dangerous: true,
      })
      applyPayload(result)
      setNotice(copy.installed)
    } catch (installError) {
      setError(installError.message)
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
      <div className="instances-hero-copy"><h2>{copy.title}</h2><p>{copy.summary}</p></div>
      <div className="instances-toolbar">
        <button type="button" className="ghost" onClick={loadInstances} disabled={loading || anyBusy}><RefreshCw size={15}/>{copy.refresh}</button>
        <button type="button" onClick={beginCreate} disabled={anyBusy}><Plus size={16}/>{copy.add}</button>
        <button type="button" className="primary instances-install-button" onClick={install} disabled={loading || anyBusy} aria-busy={busy === 'install'}>
          {busy === 'install' ? <RefreshCw className="instances-spin" size={16}/> : <Download size={16}/>} {busy === 'install' ? copy.installing : copy.install}
        </button>
      </div>
    </div>

    {(error || notice) && <div className={`instances-message ${error ? 'error' : 'success'}`} role="status">
      {error ? <X size={16}/> : <CheckCircle2 size={16}/>}<span>{error || notice}</span>
    </div>}

    {editor && <form className="instance-editor" onSubmit={submit}>
      <div className="instance-editor-heading"><div><small>{editor === 'create' ? copy.add : form.id}</small><h3>{editor === 'create' ? copy.createTitle : copy.editTitle}</h3></div><button type="button" className="icon-btn" aria-label={copy.cancel} onClick={() => setEditor(null)} disabled={anyBusy}><X size={18}/></button></div>
      <div className="instance-form-grid">
        <label htmlFor="instance-id"><span>{copy.id}</span><input id="instance-id" aria-label={copy.id} value={form.id} disabled={editor !== 'create' || anyBusy} onChange={event => patchForm('id', event.target.value)} required/><small>{copy.idHint}</small></label>
        <label htmlFor="instance-name"><span>{copy.name}</span><input id="instance-name" aria-label={copy.name} value={form.name} disabled={anyBusy} onChange={event => patchForm('name', event.target.value)} required/></label>
        <label className="instance-field-wide" htmlFor="instance-root"><span>{copy.root}</span><input id="instance-root" aria-label={copy.root} value={form.ga_root} disabled={anyBusy} onChange={event => patchForm('ga_root', event.target.value)} required/><small>{copy.rootHint}</small></label>
        <label className="instance-field-wide" htmlFor="instance-python"><span>{copy.python}</span><input id="instance-python" aria-label={copy.python} value={form.python_path} disabled={anyBusy} onChange={event => patchForm('python_path', event.target.value)}/><small>{copy.pythonHint}</small></label>
      </div>
      <div className="instance-editor-actions"><button type="button" onClick={() => setEditor(null)} disabled={anyBusy}>{copy.cancel}</button><button type="submit" className="primary" disabled={anyBusy}><Save size={15}/>{editor === 'create' ? copy.create : copy.save}</button></div>
    </form>}

    {loading ? <div className="instances-empty"><RefreshCw className="spin" size={22}/><span>{copy.loading}</span></div> : items.length === 0 ? <div className="instances-empty"><Server size={28}/><span>{copy.empty}</span><button type="button" onClick={beginCreate}><Plus size={15}/>{copy.add}</button></div> : <div className="instances-grid">
      {items.map(instance => {
        const isDefault = instance.id === defaultID
        const blocksDefaultDelete = isDefault && items.length > 1
        return <article className={`instance-card${isDefault ? ' is-default' : ''}`} key={instance.id}>
          <header>
            <span className="instance-card-icon"><Cpu size={19}/></span>
            <div><h3>{instance.name || instance.id}</h3><code>{instance.id}</code></div>
            {isDefault && <span className="instance-default-badge"><Star size={13}/>{copy.default}</span>}
          </header>
          <dl>
            <div><dt>{copy.root}</dt><dd title={instance.ga_root || ''}>{instance.ga_root || '-'}</dd></div>
            <div><dt>{copy.python}</dt><dd title={instance.python_path || ''}>{instance.python_path || copy.auto}</dd></div>
            <div><dt>{copy.effectivePython}</dt><dd title={instance.effective_python || ''}>{instance.effective_python || copy.auto}</dd></div>
          </dl>
          <footer>
            <button type="button" onClick={() => beginEdit(instance)} disabled={anyBusy}><Pencil size={14}/>{copy.edit}</button>
            <button type="button" onClick={() => setDefault(instance)} disabled={anyBusy || isDefault}><Star size={14}/>{copy.setDefault}</button>
            <button type="button" className="danger" title={blocksDefaultDelete ? copy.defaultDeleteHint : ''} onClick={() => remove(instance)} disabled={anyBusy || blocksDefaultDelete}><Trash2 size={14}/>{copy.remove}</button>
          </footer>
        </article>
      })}
    </div>}
  </section>
}
