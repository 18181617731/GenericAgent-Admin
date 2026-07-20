import React, { useState } from 'react'
import { BookOpenCheck, CircleCheckBig, Eye, FileCheck2, Play, RefreshCw, ShieldCheck, Square, TriangleAlert } from 'lucide-react'
import { ProviderModelCascade, buildModelProviderGroups, findModelProviderValue, modelProvider, runtimeModelLabel } from './ModelProviderCascade.jsx'
import { observabilitySummary } from '../lib/observability.js'

const serviceCommand = (svc) => Array.isArray(svc?.command) ? svc.command.join(' ') : (svc?.command || '-')
const servicePid = (svc) => svc?.pid ?? '-'
const serviceReturnCode = (svc) => svc?.returncode ?? svc?.return_code ?? '-'
const serviceStartedAt = (svc) => svc?.started_at || '-'
const serviceLogPath = (svc) => svc?.log_path || svc?.log || ''

function ServiceMeta({ svc, compact = false, llms = [], onModel, t }) {
  const cmd = serviceCommand(svc)
  const logPath = serviceLogPath(svc)
  const isReflect = svc?.kind === 'reflect' || String(svc?.name || '').startsWith('reflect/')
  const modelMatch = (svc?.model_no === null || svc?.model_no === undefined) ? null : llms.find(m => m.index === svc.model_no)
  const defaultLabel = (t && t.runModelDefault) || '默认（启动时选择）'
  const modelText = modelMatch ? `${modelProvider(modelMatch)} · ${runtimeModelLabel(modelMatch)}` : (isReflect ? defaultLabel : null)
  const editable = isReflect && !svc?.running && typeof onModel === 'function'
  const modelGroups = buildModelProviderGroups(llms, { defaultLabel })
  const modelValue = svc.model_no ?? ''
  const selectedProvider = findModelProviderValue(modelGroups, modelValue)
  return <div className={compact ? 'service-meta service-meta-compact' : 'service-meta'}>
    {editable
      ? <div className="model-edit service-meta-item"><em>模型</em><ProviderModelCascade
          groups={modelGroups}
          selectedProvider={selectedProvider}
          value={modelValue}
          showLabel={false}
          placement="auto"
          align="start"
          className="service-provider-cascade"
          onChange={value => onModel(svc.name, value === '' ? null : Number(value))}
        /></div>
      : (modelText !== null && <span><em>模型</em><code title={modelText}>{modelText}</code></span>)}
    <span><em>PID</em><b>{servicePid(svc)}</b></span>
    <span><em>返回码</em><b>{serviceReturnCode(svc)}</b></span>
    <span><em>启动时间</em><code title={serviceStartedAt(svc)}>{serviceStartedAt(svc)}</code></span>
    <span><em>工作目录</em><code title={svc?.workdir}>{svc?.workdir || '-'}</code></span>
    <span><em>命令</em><code title={cmd}>{cmd}</code></span>
    {logPath && <span><em>日志</em><code title={logPath}>{logPath}</code></span>}
  </div>
}

export function Stat({ label, value, detail = '', tone = '', icon }) {
  return <div className={`stat${tone ? ` is-${tone}` : ''}`}>
    <div>{icon}</div>
    <div className="stat-content"><span>{label}</span><b>{value}</b>{detail && <small>{detail}</small>}</div>
  </div>
}
export function Panel({ title, children, className = '' }) { return <div className={`panel ${className}`}><div className="panel-title">{title}</div>{children}</div> }
export function EntryList({ items = [], empty }) { return <div className="entry-list">{items.length ? items.map((e, i) => <div className="entry" key={`${e.path || e.name}-${i}`}><b>{e.name || e.path}</b><span>{e.path}{e.kind ? ` - ${e.kind}` : ''}{e.size ? ` - ${e.size} B` : ''}</span></div>) : <p className="muted">{empty}</p>}</div> }
export function ServiceRow({ svc, onStart, onStop, onLogs, onAutostart, onModel, llms = [], t }) {
  const isReflect = svc?.kind === 'reflect' || String(svc?.name || '').startsWith('reflect/')
  const descKey = String(svc?.name || '').includes('scheduler') ? 'scheduler' : (String(svc?.name || '').includes('autonomous') ? 'autonomous' : null)
  const desc = isReflect && descKey ? t?.serviceDesc?.[descKey] : null
  return <article className={`service-card ${svc.running ? 'is-running' : 'is-stopped'}`}>
    <div className="service-card-head">
      <div className="service-title"><b>{svc.name}</b><span>{svc.kind}</span></div>
      <span className={svc.running ? 'status-pill running' : 'status-pill stopped'}>{svc.running ? t.running : t.stopped}</span>
    </div>
    {desc && <p className="service-desc">{desc}</p>}
    <ServiceMeta svc={svc} llms={llms} onModel={onModel} t={t}/>
    <div className="svc-actions service-actions-row">
      <button disabled={svc.running} onClick={() => onStart(svc.name)}><Play size={14}/>{t.start}</button>
      <button disabled={!svc.running} onClick={() => onStop(svc.name)}><Square size={14}/>{t.stop}</button>
      <button onClick={() => onLogs?.(svc.name)}><Eye size={14}/>{t.logs}</button>
      <label className="toggle-inline"><input type="checkbox" checked={!!svc.autostart} onChange={e => onAutostart?.(svc.name, e.target.checked)} />{t.autostartService}</label>
    </div>
  </article>
}
export function ChannelServiceTable({ services = [], onStart, onStop, onLogs, onAutostart, onReflectStart, t }) {
  if (!services.length) return <div className="channel-service-empty">{t.hints.noFrontend}</div>
  const isReflectService = (svc) => svc?.kind === 'reflect' || String(svc?.name || '').startsWith('reflect/')
  return <div className="channel-service-list">{services.map(svc => <article className={`channel-service-card ${svc.running ? 'is-running' : 'is-stopped'}`} key={svc.name}>
    <div className="channel-service-main">
      <div><b>{svc.name}</b><small>{svc.kind}</small></div>
      <span className={svc.running ? 'status-pill running' : 'status-pill stopped'}>{svc.running ? t.running : t.stopped}</span>
    </div>
    <ServiceMeta svc={svc} compact/>
    <div className="channel-service-actions">
      <label className="toggle-inline"><input type="checkbox" checked={!!svc.autostart} onChange={e => onAutostart?.(svc.name, e.target.checked)} />{svc.autostart ? t.enabled : t.disabled}</label>
      <div className="svc-actions"><button disabled={svc.running} onClick={() => isReflectService(svc) && onReflectStart ? onReflectStart(svc.name) : onStart(svc.name)}><Play size={14}/>{t.start}</button><button disabled={!svc.running} onClick={() => onStop(svc.name)}><Square size={14}/>{t.stop}</button><button onClick={() => onLogs?.(svc.name)}><Eye size={14}/>{t.logs}</button></div>
    </div>
  </article>)}</div>
}

const count = (items) => Array.isArray(items) ? items.length : 0

export function DangerRecoveryNotice({
  operation = 'Dangerous operation',
  changes = '',
  recoverable = '',
  backup = '',
  recovery = '',
  children,
}) {
  const items = [
    ['变更内容', changes || '确认后可能写入本地配置、文件或进程状态。'],
    ['What remains recoverable', recoverable || 'Existing confirmation gates stay in place; no request is sent unless the user confirms.'],
    backup && ['Backup hint', backup],
    recovery && ['Recovery hint', recovery],
  ].filter(Boolean)
  return <aside className="danger-recovery-notice" aria-label={`${operation} recovery guidance`}>
    <div className="danger-recovery-head"><b>{operation}</b><span>确认前请先核对恢复信息</span></div>
    <ul>{items.map(([label, value]) => <li key={label}><span>{label}</span><p>{value}</p></li>)}</ul>
    {children && <div className="danger-recovery-extra">{children}</div>}
  </aside>
}

export function ObservabilityCard({ snapshot, error = '', onRefresh }) {
  const summary = observabilitySummary(snapshot)
  const icons = { status: CircleCheckBig, core: FileCheck2, knowledge: BookOpenCheck, protection: ShieldCheck }
  const missing = snapshot?.missingCore || []
  const checkedAt = snapshot?.generatedAt ? new Date(snapshot.generatedAt).toLocaleString() : ''
  return <section className="observability-card" aria-label="运行概览">
    <div className="observability-head">
      <div><b>运行概览</b><span>{summary.detail}</span></div>
      <button type="button" onClick={onRefresh} title="重新读取运行概览"><RefreshCw size={14}/>刷新</button>
    </div>
    {error ? <p className="err-text">{error}</p> : <>
      {summary.stats.length > 0
        ? <div className="observability-stats">{summary.stats.map(item => {
          const Icon = icons[item.key] || TriangleAlert
          return <div key={item.key} className={`observability-stat is-${summary.tone}`}>
            <Icon size={18} aria-hidden="true"/>
            <div><em>{item.label}</em><b>{item.value}</b><small>{item.detail}</small></div>
          </div>
        })}</div>
        : <p className="observability-empty">正在读取本机 GenericAgent 状态</p>}
      <div className="observability-body">
        {checkedAt && <p className="muted">上次检查：{checkedAt}</p>}
        {missing.length > 0 && <p className="warn">核心文件缺失：{missing.map(x => x.path || x.name).join(', ')}</p>}
      </div>
    </>}
  </section>
}

export function SecretInput({ value, onChange, t }) { const [show, setShow] = useState(false); return <div className="secret-row"><input type={show ? 'text' : 'password'} value={value || ''} placeholder={t.hints.savedSecret} onChange={e => onChange(e.target.value)} /><button type="button" onClick={() => setShow(!show)}>{show ? t.hide : t.show}</button></div> }
