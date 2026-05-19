import { useEffect, useMemo, useState } from 'react'
import { Play, Square, RefreshCw, Server, FolderCog, Terminal, Search, SlidersHorizontal, Plus, Trash2, Save, FileCode2, UploadCloud, Eye, Download } from 'lucide-react'

const api = async (url, options = {}) => {
  const res = await fetch(url, { headers: { 'Content-Type': 'application/json' }, ...options })
  if (!res.ok) {
    const body = await res.json().catch(() => ({}))
    throw new Error(body.detail || `${res.status} ${res.statusText}`)
  }
  return res.json()
}

const emptyProfile = (i = 0) => ({
  enabled: true,
  var_name: `native_oai_config${i}`,
  type: 'native_oai',
  name: 'new-model',
  apibase: '',
  model: '',
  apikey: '',
  fake_cc_system_prompt: null,
  thinking_type: '',
  reasoning_effort: '',
  stream: true,
  max_retries: 3,
  read_timeout: 300,
  connect_timeout: null,
  user_agent: '',
  api_mode: '',
  extra: {},
})

function SecretInput({ value, onChange }) {
  const [show, setShow] = useState(false)
  const display = value === '***SET***' ? '' : (value || '')
  return <div className="secret-row"><input type={show ? 'text' : 'password'} value={display} onChange={(e) => onChange(e.target.value)} placeholder={value === '***SET***' ? '已保存；输入新值可替换' : 'API Key / Token'} /><button type="button" className="ghost" onClick={() => setShow(!show)}><Eye size={14}/>{show ? '隐藏' : '显示'}</button></div>
}

export default function App() {
  const [tab, setTab] = useState('services')
  const [config, setConfig] = useState(null)
  const [gaRoot, setGaRoot] = useState('')
  const [services, setServices] = useState([])
  const [selected, setSelected] = useState('')
  const [logs, setLogs] = useState([])
  const [summary, setSummary] = useState({ total: 0, running: 0, stopped: 0 })
  const [query, setQuery] = useState('')
  const [kind, setKind] = useState('all')
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState('')
  const [ok, setOk] = useState('')
  const [profiles, setProfiles] = useState([])
  const [modelSource, setModelSource] = useState(null)
  const [preview, setPreview] = useState('')

  const run = async (fn, success = '') => {
    setBusy(true); setError(''); setOk('')
    try { const ret = await fn(); if (success) setOk(success); return ret } catch (e) { setError(e.message) } finally { setBusy(false) }
  }

  const loadConfig = async () => {
    const data = await api('/api/config')
    setConfig(data); setGaRoot(data.ga_root)
  }

  const refresh = async () => {
    const [list, sum] = await Promise.all([api('/api/services'), api('/api/services/summary')])
    setServices(list); setSummary(sum)
    if (!selected && list.length) setSelected(list[0].name)
    if (selected && !list.some((item) => item.name === selected)) setSelected(list[0]?.name || '')
  }

  const loadLogs = async (name = selected) => {
    if (!name) return setLogs([])
    const data = await api(`/api/services/${encodeURIComponent(name)}/logs`)
    setLogs(data.lines || [])
  }

  const loadModels = async (raw = true) => {
    const data = await api(raw ? '/api/models/raw' : '/api/models')
    setProfiles(data.profiles || [])
    setModelSource(data.source || null)
  }

  useEffect(() => { run(async () => { await loadConfig(); await refresh(); await loadModels(true) }) }, [])
  useEffect(() => { if (selected) loadLogs(selected) }, [selected])

  const filtered = useMemo(() => services.filter((svc) => {
    const q = query.trim().toLowerCase()
    return (kind === 'all' || svc.kind === kind) && (!q || svc.name.toLowerCase().includes(q) || svc.command.join(' ').toLowerCase().includes(q))
  }), [services, query, kind])

  const saveRoot = () => run(async () => { const data = await api('/api/config', { method: 'PUT', body: JSON.stringify({ ga_root: gaRoot }) }); setConfig(data); await refresh(); await loadModels(true) }, 'GA 根目录已保存')
  const start = (name) => run(async () => { await api('/api/services/start', { method: 'POST', body: JSON.stringify({ name }) }); await refresh(); await loadLogs(name) })
  const stop = (name) => run(async () => { await api(`/api/services/${encodeURIComponent(name)}/stop`, { method: 'POST' }); await refresh(); await loadLogs(name) })
  const stopAll = () => run(async () => { await api('/api/services/stop-all', { method: 'POST' }); await refresh() })

  const patchProfile = (idx, patch) => setProfiles((rows) => rows.map((p, i) => i === idx ? { ...p, ...patch } : p))
  const cloneProfiles = () => profiles.map((p) => ({ ...p, apikey: p.apikey === '***SET***' ? '' : p.apikey }))
  const saveModels = () => run(async () => { const data = await api('/api/models', { method: 'PUT', body: JSON.stringify({ profiles: cloneProfiles() }) }); setProfiles(data.profiles || cloneProfiles()); setModelSource(data.source) }, '模型草稿已保存')
  const previewModels = () => run(async () => { const data = await api('/api/models/preview', { method: 'POST', body: JSON.stringify({ profiles: cloneProfiles() }) }); setPreview(data.python || '') }, '已生成预览')
  const importMyKey = () => run(async () => { const data = await api('/api/models/import-mykey', { method: 'POST', body: JSON.stringify({ save: true, reveal: false }) }); setProfiles(data.profiles || []); await loadModels(false); setPreview(`已从 mykey.py 导入 ${data.profiles?.length || 0} 个配置。密钥默认脱敏；如需替换请在密钥框输入新值。`) }, '已从 mykey.py 导入')
  const exportModels = () => run(async () => { if (!confirm('将先备份当前 mykey.py，然后用页面配置覆盖它。确认写回并接管 GA 模型配置？')) return; const ret = await api('/api/models/export', { method: 'POST', body: JSON.stringify({ profiles: cloneProfiles(), overwrite_active: true }) }); await loadModels(false); setPreview(`已写回：${ret.active_path}\n备份：${ret.backup_path || '无旧文件'}\n生成副本：${ret.generated_path}`) }, '已备份并写回 mykey.py')

  return <div className="app">
    <aside className="sidebar">
      <div className="brand"><Server/><div><h1>GA Admin</h1><p>GenericAgent 控制台</p></div></div>
      <button className={`nav ${tab === 'services' ? 'active' : ''}`} onClick={() => setTab('services')}><Terminal size={16}/> 服务管理</button>
      <button className={`nav ${tab === 'models' ? 'active' : ''}`} onClick={() => setTab('models')}><SlidersHorizontal size={16}/> 模型配置</button>
      <div className="metric"><span>服务总数</span><b>{summary.total}</b></div>
      <div className="metric running"><span>运行中</span><b>{summary.running}</b></div>
      <button className="danger wide" onClick={stopAll} disabled={busy || !summary.running}><Square size={16}/> 停止全部</button>
    </aside>

    <main>
      <section className="panel config">
        <FolderCog size={20}/><input value={gaRoot} onChange={(e) => setGaRoot(e.target.value)} placeholder="GenericAgent 根目录" /><button disabled={busy || gaRoot === config?.ga_root} onClick={saveRoot}>保存并重扫</button>
      </section>
      {error && <div className="error">{error}</div>}{ok && <div className="ok">{ok}</div>}

      {tab === 'services' && <section className="workspace">
        <div className="panel services">
          <div className="toolbar"><div className="search"><Search size={16}/><input value={query} onChange={(e) => setQuery(e.target.value)} placeholder="搜索服务" /></div><select value={kind} onChange={(e) => setKind(e.target.value)}><option value="all">全部</option><option value="reflect">Reflect</option><option value="frontend">Frontend</option></select><button onClick={() => run(refresh)} disabled={busy}><RefreshCw size={16}/> 刷新</button></div>
          <div className="list">{filtered.map((svc) => <div key={svc.name} className={`service ${selected === svc.name ? 'active' : ''}`} onClick={() => setSelected(svc.name)}><div><b>{svc.name}</b><small>{svc.command.join(' ')}</small></div><span className={svc.running ? 'badge on' : 'badge'}>{svc.running ? `running #${svc.pid}` : 'stopped'}</span>{svc.running ? <button className="danger" disabled={busy} onClick={(e) => { e.stopPropagation(); stop(svc.name) }}><Square size={14}/> Stop</button> : <button disabled={busy} onClick={(e) => { e.stopPropagation(); start(svc.name) }}><Play size={14}/> Start</button>}</div>)}</div>
        </div>
        <div className="panel log-panel"><div className="panel-title"><Terminal size={18}/> 输出日志 <code>{selected || '未选择'}</code></div><pre>{logs.length ? logs.join('\n') : '暂无输出。'}</pre></div>
      </section>}

      {tab === 'models' && <section className="models-layout">
        <div className="panel model-top"><div><h2>模型配置</h2><p>可从 GA 的 mykey.py 安全导入：仅 AST 解析赋值字典，不执行文件；密钥默认脱敏。GA 发现变量名需包含 api/config/cookie。</p>{modelSource && <small>当前 GA 源：<code>{modelSource.active_source}</code> · 生成文件：<code>{modelSource.generated_path}</code></small>}</div><div className="actions"><button onClick={importMyKey} disabled={busy}><Download size={16}/> 从 mykey.py 导入</button><button onClick={() => setProfiles([...profiles, emptyProfile(profiles.length)])}><Plus size={16}/> 新增</button><button onClick={saveModels} disabled={busy}><Save size={16}/> 保存草稿</button><button onClick={previewModels} disabled={busy}><FileCode2 size={16}/> 预览</button><button onClick={exportModels} disabled={busy}><UploadCloud size={16}/> 写回 mykey.py</button></div></div>
        <div className="model-grid">
          <div className="profiles">{profiles.map((p, idx) => <div className="profile-card" key={`${p.var_name}-${idx}`}>
            <div className="profile-head"><label><input type="checkbox" checked={!!p.enabled} onChange={(e) => patchProfile(idx, { enabled: e.target.checked })}/> 启用</label><button className="ghost danger-text" onClick={() => setProfiles(profiles.filter((_, i) => i !== idx))}><Trash2 size={14}/> 删除</button></div>
            <div className="form-grid"><label>变量名<input value={p.var_name || ''} onChange={(e) => patchProfile(idx, { var_name: e.target.value })}/></label><label>类型<select value={p.type || 'native_oai'} onChange={(e) => patchProfile(idx, { type: e.target.value })}><option value="native_oai">OpenAI Compatible</option><option value="native_claude">Native Claude / CC Relay</option><option value="gemini">Gemini Compatible</option><option value="custom">Custom</option></select></label><label>显示名<input value={p.name || ''} onChange={(e) => patchProfile(idx, { name: e.target.value })}/></label><label>模型<input value={p.model || ''} onChange={(e) => patchProfile(idx, { model: e.target.value })}/></label><label className="span2">API Base<input value={p.apibase || ''} onChange={(e) => patchProfile(idx, { apibase: e.target.value })}/></label><label className="span2">密钥<SecretInput value={p.apikey || ''} onChange={(v) => patchProfile(idx, { apikey: v })}/></label><label>流式<select value={String(p.stream ?? true)} onChange={(e) => patchProfile(idx, { stream: e.target.value === 'true' })}><option value="true">true</option><option value="false">false</option></select></label><label>重试<input type="number" value={p.max_retries ?? ''} onChange={(e) => patchProfile(idx, { max_retries: Number(e.target.value || 0) })}/></label><label>读超时<input type="number" value={p.read_timeout ?? ''} onChange={(e) => patchProfile(idx, { read_timeout: Number(e.target.value || 0) })}/></label><label>Thinking<input value={p.thinking_type || ''} onChange={(e) => patchProfile(idx, { thinking_type: e.target.value })} placeholder="如 enabled"/></label><label>Reasoning<input value={p.reasoning_effort || ''} onChange={(e) => patchProfile(idx, { reasoning_effort: e.target.value })} placeholder="low/medium/high"/></label><label>Fake CC<select value={String(p.fake_cc_system_prompt ?? '')} onChange={(e) => patchProfile(idx, { fake_cc_system_prompt: e.target.value === '' ? null : e.target.value === 'true' })}><option value="">未设置</option><option value="true">true</option><option value="false">false</option></select></label><label className="span2">Extra JSON<textarea value={JSON.stringify(p.extra || {}, null, 2)} onChange={(e) => { try { patchProfile(idx, { extra: JSON.parse(e.target.value || '{}') }) } catch { patchProfile(idx, { extra_text_error: true }) } }}/></label></div>
          </div>)}</div>
          <div className="panel preview"><div className="panel-title"><FileCode2 size={18}/> 生成预览</div><pre>{preview || '点击“预览”查看配置；点击“写回 mykey.py”会先备份再覆盖 GA 的 mykey.py。'}</pre></div>
        </div>
      </section>}
    </main>
  </div>
}
