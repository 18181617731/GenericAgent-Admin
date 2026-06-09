import { useState } from 'react'
import { Eye, Play, Square } from 'lucide-react'

export function Stat({ label, value, icon }) { return <div className="stat"><div>{icon}</div><span>{label}</span><b>{value}</b></div> }
export function Panel({ title, children, className = '' }) { return <div className={`panel ${className}`}><div className="panel-title">{title}</div>{children}</div> }
export function EntryList({ items = [], empty }) { return <div className="entry-list">{items.length ? items.map((e, i) => <div className="entry" key={`${e.path || e.name}-${i}`}><b>{e.name || e.path}</b><span>{e.path}{e.kind ? ` · ${e.kind}` : ''}{e.size ? ` · ${e.size} B` : ''}</span></div>) : <p className="muted">{empty}</p>}</div> }
export function ServiceRow({ svc, onStart, onStop, onLogs, onAutostart, t }) { return <div className="service-card"><div><b>{svc.name}</b><span title={svc.command?.join(' ')}>{svc.command?.join(' ')}</span><em>{svc.kind}{svc.pid ? ` - PID ${svc.pid}` : ''}{svc.started_at ? ` - ${svc.started_at}` : ''}</em>{svc.workdir && <small title={svc.workdir}>cwd: {svc.workdir}</small>}</div><div className={svc.running ? 'ok' : 'err'}>{svc.running ? t.running : t.stopped}</div><div className="svc-actions"><button disabled={svc.running} onClick={() => onStart(svc.name)}><Play size={14}/>{t.start}</button><button disabled={!svc.running} onClick={() => onStop(svc.name)}><Square size={14}/>{t.stop}</button><button onClick={() => onLogs?.(svc.name)}><Eye size={14}/>{t.logs}</button><label className="toggle-inline"><input type="checkbox" checked={!!svc.autostart} onChange={e => onAutostart?.(svc.name, e.target.checked)} />{t.autostartService}</label></div></div> }
export function ChannelServiceTable({ services = [], onStart, onStop, onLogs, onAutostart, t }) {
  if (!services.length) return <div className="channel-service-empty">{t.hints.noFrontend}</div>
  return <div className="channel-service-list">{services.map(svc => {
    const cmd = svc.command?.join(' ') || '-'
    return <article className={`channel-service-card ${svc.running ? 'is-running' : 'is-stopped'}`} key={svc.name}>
      <div className="channel-service-main">
        <div><b>{svc.name}</b><small>{svc.kind}{svc.started_at ? ` · ${svc.started_at}` : ''}</small></div>
        <span className={svc.running ? 'status-pill running' : 'status-pill stopped'}>{svc.running ? t.running : t.stopped}</span>
      </div>
      <div className="channel-service-meta">
        <span><em>PID</em>{svc.pid || '-'}</span>
        <span><em>cwd</em><code title={svc.workdir}>{svc.workdir || '-'}</code></span>
        <span><em>cmd</em><code title={cmd}>{cmd}</code></span>
      </div>
      <div className="channel-service-actions">
        <label className="toggle-inline"><input type="checkbox" checked={!!svc.autostart} onChange={e => onAutostart?.(svc.name, e.target.checked)} />{svc.autostart ? t.enabled : t.disabled}</label>
        <div className="svc-actions"><button disabled={svc.running} onClick={() => onStart(svc.name)}><Play size={14}/>{t.start}</button><button disabled={!svc.running} onClick={() => onStop(svc.name)}><Square size={14}/>{t.stop}</button><button onClick={() => onLogs?.(svc.name)}><Eye size={14}/>{t.logs}</button></div>
      </div>
    </article>
  })}</div>
}

export function SecretInput({ value, onChange, t }) { const [show, setShow] = useState(false); return <div className="secret-row"><input type={show ? 'text' : 'password'} value={value || ''} placeholder={t.hints.savedSecret} onChange={e => onChange(e.target.value)} /><button type="button" onClick={() => setShow(!show)}>{show ? t.hide : t.show}</button></div> }
