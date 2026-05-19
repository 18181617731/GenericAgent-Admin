import { useEffect, useMemo, useState } from 'react'
import { Activity, Bot, Brain, CalendarClock, CheckCircle2, Eye, FileCode2, FolderCog, GitBranch, Play, RefreshCw, Save, Server, SlidersHorizontal, Square, Terminal, UploadCloud, XCircle } from 'lucide-react'

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

function Card({ title, value, sub, icon: Icon, tone = '' }) {
  return <div className={`stat ${tone}`}><div><span>{title}</span><strong>{value}</strong><small>{sub}</small></div>{Icon && <Icon size={28}/>}</div>
}

function MiniList({ title, items = [], empty = '暂无', render }) {
  return <div className="panel"><div className="panel-title">{title}</div><div className="mini-list">{items.length ? items.map((it, idx) => <div className="mini-row" key={`${it.path || it.name || idx}`}>{render ? render(it, idx) : <><b>{it.name}</b><span>{it.path}</span></>}</div>) : <div className="empty">{empty}</div>}</div></div>
}

export default function App() {
  const [tab, setTab] = useState('overview')
  const [config, setConfig] = useState(null)
  const [gaRoot, setGaRoot] = useState('')
  const [services, setServices] = useState([])
  const [selected, setSelected] = useState('')
  const [logs, setLogs] = useState([])
  const [summary, setSummary] = useState({ total: 0, running: 0, stopped: 0 })
  const [inventory, setInventory] = useState(null)
  const [gaHealth, setGaHealth] = useState(null)
  const [schedule, setSchedule] = useState(null)
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
  const loadConfig = async () => { const data = await api('/api/config'); setConfig(data); setGaRoot(data.ga_root) }
  const refreshServices = async () => {
    const [list, sum] = await Promise.all([api('/api/services'), api('/api/services/summary')])
    setServices(list); setSummary(sum)
    if (!selected && list.length) setSelected(list[0].name)
    if (selected && !list.some((item) => item.name === selected)) setSelected(list[0]?.name || '')
  }
  const refreshGA = async () => {
    const [inv, health, sched] = await Promise.all([api('/api/ga/inventory'), api('/api/ga/health'), api('/api/schedule/tasks')])
    setInventory(inv); setGaHealth(health); setSchedule(sched)
  }
  const refreshAll = async () => { await Promise.all([loadConfig(), refreshServices(), refreshGA(), loadModels(true)]) }
  const loadLogs = async (name = selected) => { if (!name) return setLogs([]); const data = await api(`/api/services/${encodeURIComponent(name)}/logs`); setLogs(data.lines || []) }
  const loadModels = async (raw = true) => { const data = await api(raw ? '/api/models/raw' : '/api/models'); setProfiles(data.profiles || []); setModelSource(data.source || null) }

  useEffect(() => { run(refreshAll) }, [])
  useEffect(() => { if (selected) loadLogs(selected) }, [selected])

  const svcByKind = useMemo(() => ({
    frontend: services.filter(s => s.kind === 'frontend'),
    reflect: services.filter(s => s.kind === 'reflect'),
    other: services.filter(s => s.kind !== 'frontend' && s.kind !== 'reflect'),
  }), [services])

  const saveRoot = () => run(async () => { const data = await api('/api/config', { method: 'PUT', body: JSON.stringify({ ga_root: gaRoot }) }); setConfig(data); await refreshAll() }, 'GA 根目录已保存')
  const start = (name) => run(async () => { await api('/api/services/start', { method: 'POST', body: JSON.stringify({ name }) }); await refreshServices(); await loadLogs(name) }, `${name} 已启动`)
  const stop = (name) => run(async () => { await api('/api/services/stop', { method: 'POST', body: JSON.stringify({ name }) }); await refreshServices(); await loadLogs(name) }, `${name} 已停止`)
  const toggleTask = (task) => run(async () => { await api('/api/schedule/toggle', { method: 'POST', body: JSON.stringify({ id: task.id, enabled: !task.enabled }) }); await refreshGA() }, `${task.id} 已${task.enabled ? '停用' : '启用'}`)

  const patchProfile = (idx, patch) => setProfiles((arr) => arr.map((p, i) => i === idx ? { ...p, ...patch } : p))
  const saveModels = () => run(async () => { await api('/api/models/raw/save', { method: 'POST', body: JSON.stringify({ profiles }) }); await loadModels(true) }, 'mykey.py 已备份并写回')
  const previewModels = () => run(async () => { const data = await api('/api/models/raw/preview', { method: 'POST', body: JSON.stringify({ profiles }) }); setPreview(data.preview || '') })

  const nav = [
    ['overview', Activity, 'Overview'], ['tasks', Bot, 'Tasks'], ['memory', Brain, 'Memory'], ['channels', Terminal, 'Channels'], ['autonomous', GitBranch, 'Autonomous'], ['schedule', CalendarClock, 'Schedule'], ['models', SlidersHorizontal, 'Models'], ['logs', FileCode2, 'Logs'],
  ]

  const ServiceRow = ({ svc }) => <div className={`service-card ${svc.running ? 'running' : ''}`} onClick={() => setSelected(svc.name)}><div><b>{svc.name}</b><span>{svc.command?.join(' ')}</span></div><em>{svc.running ? 'RUNNING' : 'STOPPED'}</em><div className="svc-actions">{svc.running ? <button onClick={(e) => { e.stopPropagation(); stop(svc.name) }}><Square size={14}/>停止</button> : <button onClick={(e) => { e.stopPropagation(); start(svc.name) }}><Play size={14}/>启动</button>}</div></div>

  return <div className="app">
    <aside className="sidebar">
      <div className="brand"><Server size={34}/><div><h1>GA Admin</h1><p>GenericAgent 生命周期控制面</p></div></div>
      <div className="root-box"><label>GenericAgent Root</label><div><input value={gaRoot} onChange={(e) => setGaRoot(e.target.value)} /><button onClick={saveRoot} disabled={busy}><Save size={14}/>保存</button></div></div>
      <nav>{nav.map(([id, Icon, label]) => <button key={id} className={tab === id ? 'active' : ''} onClick={() => setTab(id)}><Icon size={18}/>{label}</button>)}</nav>
      <button className="refresh" onClick={() => run(refreshAll)} disabled={busy}><RefreshCw size={16}/>刷新全部</button>
    </aside>
    <main className="main">
      <header><div><h2>{nav.find(n => n[0] === tab)?.[2]}</h2><p>{config?.ga_root || '未配置 GA 根目录'}</p></div><div className="badges">{error && <span className="err">{error}</span>}{ok && <span className="ok">{ok}</span>}{busy && <span>处理中...</span>}</div></header>

      {tab === 'overview' && <section>
        <div className="stats">
          <Card title="GA Health" value={gaHealth?.ok ? 'Healthy' : 'Check'} sub={`${gaHealth?.missing_required?.length || 0} missing`} icon={gaHealth?.ok ? CheckCircle2 : XCircle} tone={gaHealth?.ok ? 'good' : 'warn'} />
          <Card title="Services" value={`${summary.running}/${summary.total}`} sub="running / total" icon={Server} />
          <Card title="Memory" value={inventory?.memory?.sop_count || 0} sub={`SOP · L4 ${inventory?.memory?.l4_session_count || 0}`} icon={Brain} />
          <Card title="Schedule" value={schedule?.summary?.enabled || 0} sub={`${schedule?.summary?.total || 0} tasks`} icon={CalendarClock} />
        </div>
        <div className="grid2">
          <MiniList title="GA 功能域健康" items={gaHealth?.checks || []} render={(c) => <><b>{c.ok ? '✓' : '×'} {c.name}</b><span>{c.path}</span></>} />
          <MiniList title="核心文件" items={inventory?.core_files || []} render={(f) => <><b>{f.exists ? '✓' : '×'} {f.path}</b><span>{f.exists ? `${f.size || 0} bytes` : 'missing'}</span></>} />
          <MiniList title="Reflect / Autonomous" items={inventory?.reflect || []} />
          <MiniList title="Frontends / Channels" items={inventory?.frontends || []} render={(f) => <><b>{f.name}</b><span>{f.kind} · {f.path}</span></>} />
        </div>
      </section>}

      {tab === 'tasks' && <section><div className="panel"><div className="panel-title"><Bot size={18}/> Task Runtime</div><p className="hint">下一阶段接入 <code>agentmain.py --task &lt;dir&gt;</code>，用于页面创建任务、查看实时输出、继续 reply 与停止任务。本轮已先完成 GA Inventory/Health，为任务运行域做基础。</p></div></section>}

      {tab === 'memory' && <section><div className="stats"><Card title="L3 SOP" value={inventory?.memory?.sop_count || 0} sub="memory/*.md" icon={Brain}/><Card title="Utils" value={inventory?.memory?.util_count || 0} sub="memory/*.py" icon={FileCode2}/><Card title="L4 Sessions" value={inventory?.memory?.l4_session_count || 0} sub="raw sessions" icon={FolderCog}/></div><div className="grid2"><MiniList title="最近 SOP" items={inventory?.memory?.recent_sops || []}/><MiniList title="Memory Layer Files" items={inventory?.memory?.layers || []} render={(f) => <><b>{f.name}</b><span>{f.path}</span></>}/></div></section>}

      {tab === 'channels' && <section><div className="grid2"><div className="panel"><div className="panel-title">Frontend Services</div>{svcByKind.frontend.map(s => <ServiceRow key={s.name} svc={s}/>)}{!svcByKind.frontend.length && <div className="empty">未发现 frontend 服务</div>}</div><MiniList title="Discovered Channel Files" items={inventory?.frontends || []} render={(f) => <><b>{f.name}</b><span>{f.kind} · {f.path}</span></>}/></div></section>}

      {tab === 'autonomous' && <section><div className="grid2"><div className="panel"><div className="panel-title">Reflect Services</div>{svcByKind.reflect.map(s => <ServiceRow key={s.name} svc={s}/>)}{!svcByKind.reflect.length && <div className="empty">未发现 reflect 服务</div>}</div><MiniList title="Reflect Scripts" items={inventory?.reflect || []}/></div></section>}

      {tab === 'schedule' && <section><div className="stats"><Card title="Total" value={schedule?.summary?.total || 0} sub="schedule json" icon={CalendarClock}/><Card title="Enabled" value={schedule?.summary?.enabled || 0} sub="active tasks" icon={CheckCircle2}/><Card title="Reports" value={schedule?.reports?.length || 0} sub="sche_tasks/done" icon={FileCode2}/></div><div className="panel"><div className="panel-title">Scheduled Tasks</div>{(schedule?.tasks || []).map(t => <div className="task-row" key={t.id}><div><b>{t.id}</b><span>{t.schedule || '-'} · {t.repeat || '-'} · max delay {t.max_delay_hours || 6}h</span><p>{t.prompt}</p></div><button onClick={() => toggleTask(t)}>{t.enabled ? '停用' : '启用'}</button></div>)}{!(schedule?.tasks || []).length && <div className="empty">暂无 sche_tasks/*.json</div>}</div><MiniList title="最近报告" items={schedule?.reports || []}/></section>}

      {tab === 'models' && <section><div className="model-top"><div><h3>模型配置</h3><p>来源：{modelSource?.path || 'mykey.py'} · 已隐藏真实密钥</p></div><div className="actions"><button onClick={() => setProfiles([...profiles, emptyProfile(profiles.length)])}>新增 Profile</button><button onClick={previewModels}><Eye size={14}/>预览</button><button onClick={saveModels}><UploadCloud size={14}/>写回 mykey.py</button></div></div><div className="models-layout"><div className="profiles">{profiles.map((p, idx) => <div className="profile" key={idx}><div className="profile-head"><b>#{idx + 1} {p.name || p.var_name}</b><label><input type="checkbox" checked={!!p.enabled} onChange={(e) => patchProfile(idx, { enabled: e.target.checked })}/> enabled</label></div><div className="form-grid"><label>变量名<input value={p.var_name || ''} onChange={(e) => patchProfile(idx, { var_name: e.target.value })}/></label><label>类型<input value={p.type || ''} onChange={(e) => patchProfile(idx, { type: e.target.value })}/></label><label>Name<input value={p.name || ''} onChange={(e) => patchProfile(idx, { name: e.target.value })}/></label><label>Model<input value={p.model || ''} onChange={(e) => patchProfile(idx, { model: e.target.value })}/></label><label className="span2">API Base<input value={p.apibase || ''} onChange={(e) => patchProfile(idx, { apibase: e.target.value })}/></label><label className="span2">API Key<SecretInput value={p.apikey} onChange={(v) => patchProfile(idx, { apikey: v })}/></label><label>Stream<select value={String(!!p.stream)} onChange={(e) => patchProfile(idx, { stream: e.target.value === 'true' })}><option value="true">true</option><option value="false">false</option></select></label><label>Max Retries<input type="number" value={p.max_retries ?? 3} onChange={(e) => patchProfile(idx, { max_retries: Number(e.target.value) })}/></label><label>Read Timeout<input type="number" value={p.read_timeout ?? 300} onChange={(e) => patchProfile(idx, { read_timeout: Number(e.target.value) })}/></label><label>Reasoning<input value={p.reasoning_effort || ''} onChange={(e) => patchProfile(idx, { reasoning_effort: e.target.value })}/></label></div></div>)}</div><div className="panel preview"><div className="panel-title"><FileCode2 size={18}/> 生成预览</div><pre>{preview || '点击“预览”查看配置；点击“写回 mykey.py”会先备份再覆盖 GA 的 mykey.py。'}</pre></div></div></section>}

      {tab === 'logs' && <section><div className="workspace"><div className="panel"><div className="panel-title">Processes</div>{services.map(s => <ServiceRow key={s.name} svc={s}/>)}</div><div className="panel log-panel"><div className="panel-title">Logs · {selected}</div><pre>{logs.join('\n') || '暂无日志'}</pre></div></div></section>}
    </main>
  </div>
}
