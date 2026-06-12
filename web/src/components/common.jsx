import { useState } from 'react'
import { Eye, Play, Square } from 'lucide-react'

const serviceCommand = (svc) => Array.isArray(svc?.command) ? svc.command.join(' ') : (svc?.command || '-')
const servicePid = (svc) => svc?.pid ?? '-'
const serviceReturnCode = (svc) => svc?.returncode ?? svc?.return_code ?? '-'
const serviceStartedAt = (svc) => svc?.started_at || '-'
const serviceLogPath = (svc) => svc?.log_path || svc?.log || ''

function ServiceMeta({ svc, compact = false }) {
  const cmd = serviceCommand(svc)
  const logPath = serviceLogPath(svc)
  return <div className={compact ? 'service-meta service-meta-compact' : 'service-meta'}>
    <span><em>PID</em><b>{servicePid(svc)}</b></span>
    <span><em>返回码</em><b>{serviceReturnCode(svc)}</b></span>
    <span><em>启动时间</em><code title={serviceStartedAt(svc)}>{serviceStartedAt(svc)}</code></span>
    <span><em>工作目录</em><code title={svc?.workdir}>{svc?.workdir || '-'}</code></span>
    <span><em>命令</em><code title={cmd}>{cmd}</code></span>
    {logPath && <span><em>日志</em><code title={logPath}>{logPath}</code></span>}
  </div>
}

export function Stat({ label, value, icon }) { return <div className="stat"><div>{icon}</div><span>{label}</span><b>{value}</b></div> }
export function Panel({ title, children, className = '' }) { return <div className={`panel ${className}`}><div className="panel-title">{title}</div>{children}</div> }
export function EntryList({ items = [], empty }) { return <div className="entry-list">{items.length ? items.map((e, i) => <div className="entry" key={`${e.path || e.name}-${i}`}><b>{e.name || e.path}</b><span>{e.path}{e.kind ? ` - ${e.kind}` : ''}{e.size ? ` - ${e.size} B` : ''}</span></div>) : <p className="muted">{empty}</p>}</div> }
export function ServiceRow({ svc, onStart, onStop, onLogs, onAutostart, t }) {
  return <article className={`service-card ${svc.running ? 'is-running' : 'is-stopped'}`}>
    <div className="service-card-head">
      <div className="service-title"><b>{svc.name}</b><span>{svc.kind}</span></div>
      <span className={svc.running ? 'status-pill running' : 'status-pill stopped'}>{svc.running ? t.running : t.stopped}</span>
    </div>
    <ServiceMeta svc={svc}/>
    <div className="svc-actions service-actions-row">
      <button disabled={svc.running} onClick={() => onStart(svc.name)}><Play size={14}/>{t.start}</button>
      <button disabled={!svc.running} onClick={() => onStop(svc.name)}><Square size={14}/>{t.stop}</button>
      <button onClick={() => onLogs?.(svc.name)}><Eye size={14}/>{t.logs}</button>
      <label className="toggle-inline"><input type="checkbox" checked={!!svc.autostart} onChange={e => onAutostart?.(svc.name, e.target.checked)} />{t.autostartService}</label>
    </div>
  </article>
}
export function ChannelServiceTable({ services = [], onStart, onStop, onLogs, onAutostart, t }) {
  if (!services.length) return <div className="channel-service-empty">{t.hints.noFrontend}</div>
  return <div className="channel-service-list">{services.map(svc => <article className={`channel-service-card ${svc.running ? 'is-running' : 'is-stopped'}`} key={svc.name}>
    <div className="channel-service-main">
      <div><b>{svc.name}</b><small>{svc.kind}</small></div>
      <span className={svc.running ? 'status-pill running' : 'status-pill stopped'}>{svc.running ? t.running : t.stopped}</span>
    </div>
    <ServiceMeta svc={svc} compact/>
    <div className="channel-service-actions">
      <label className="toggle-inline"><input type="checkbox" checked={!!svc.autostart} onChange={e => onAutostart?.(svc.name, e.target.checked)} />{svc.autostart ? t.enabled : t.disabled}</label>
      <div className="svc-actions"><button disabled={svc.running} onClick={() => onStart(svc.name)}><Play size={14}/>{t.start}</button><button disabled={!svc.running} onClick={() => onStop(svc.name)}><Square size={14}/>{t.stop}</button><button onClick={() => onLogs?.(svc.name)}><Eye size={14}/>{t.logs}</button></div>
    </div>
  </article>)}</div>
}

const count = (items) => Array.isArray(items) ? items.length : 0

const firstText = (...values) => values.find(value => typeof value === 'string' && value.trim())?.trim() || ''

const recoveryHintFromRisk = (item = {}) => ({
  changes: firstText(item.changes, item.write, item.effect, item.description),
  recoverable: firstText(item.recoverable, item.rollback, item.safe_after_confirm),
  backup: firstText(item.backup, item.backup_path, item.backupHint, item.backup_hint),
  recovery: firstText(item.recovery, item.recovery_hint, item.restore, item.restore_hint),
})

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
  const stats = [
    ['Health checks', count(snapshot?.checks)],
    ['核心文件', count(snapshot?.coreFiles?.filter?.(item => item?.exists) || [])],
    ['记忆 SOP', count(snapshot?.memory?.sops)],
    ['Risk rules', count(snapshot?.riskItems)],
  ]
  const missing = snapshot?.missingCore || []
  const writeRisks = snapshot?.writeRiskItems || []
  return <section className="observability-card" aria-label="只读观测">
    <div className="observability-head">
      <div><b>只读观测</b><span>{snapshot?.root || 'GET /api/health + /api/ga/inventory + /api/risk/catalog'}</span></div>
      <button type="button" onClick={onRefresh}>刷新</button>
    </div>
    {error ? <p className="err-text">{error}</p> : <>
      <div className="observability-stats">{stats.map(([label, value]) => <span key={label}><em>{label}</em><b>{value}</b></span>)}</div>
      <div className="observability-body">
        <p className={snapshot?.ok ? 'ok' : 'warn'}>{snapshot ? (snapshot.ok ? '健康端点报告正常' : '健康端点需要关注') : '等待只读快照'}</p>
        {snapshot?.generatedAt && <p className="muted">生成时间：{snapshot.generatedAt}</p>}
        {missing.length > 0 && <p className="warn">核心文件缺失：{missing.map(x => x.path || x.name).join(', ')}</p>}
        {writeRisks.length > 0 && <>
          <p className="muted">已登记危险写入门禁端点：{writeRisks.length}</p>
          <div className="danger-recovery-grid">{writeRisks.slice(0, 3).map((item, index) => {
            const hint = recoveryHintFromRisk(item)
            return <DangerRecoveryNotice
              key={`${item?.method || 'write'}-${item?.path || item?.name || index}`}
              operation={item?.path || item?.name || `受保护写入 ${index + 1}`}
              changes={hint.changes}
              recoverable={hint.recoverable}
              backup={hint.backup}
              recovery={hint.recovery}
            />
          })}</div>
        </>}
      </div>
    </>}
  </section>
}

export function SecretInput({ value, onChange, t }) { const [show, setShow] = useState(false); return <div className="secret-row"><input type={show ? 'text' : 'password'} value={value || ''} placeholder={t.hints.savedSecret} onChange={e => onChange(e.target.value)} /><button type="button" onClick={() => setShow(!show)}>{show ? t.hide : t.show}</button></div> }
