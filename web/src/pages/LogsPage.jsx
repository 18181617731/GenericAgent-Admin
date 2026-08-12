import React from 'react'
import { Activity, Play, RefreshCw, Square, Trash2 } from 'lucide-react'
import { Panel } from '../components/common'

const STREAM_LABELS = {
  zh: { live: '实时传输', reconnecting: '正在重连', connecting: '正在连接', idle: '未连接' },
  en: { live: 'Live', reconnecting: 'Reconnecting', connecting: 'Connecting', idle: 'Idle' },
}

const STREAM_STATUS = {
  zh: { connecting: '正在连接日志流', live: '日志流已连接', reconnecting: '日志流连接中断', idle: '日志流空闲' },
  en: { connecting: 'Connecting to log stream', live: 'Log stream live', reconnecting: 'Log stream interrupted', idle: 'Log stream idle' },
}

export function LogsPage({ t, lang, services, stream, onStart, onStop }) {
  const label = STREAM_LABELS[lang] || STREAM_LABELS.en
  const status = STREAM_STATUS[lang] || STREAM_STATUS.en
  const selectedService = services.find(s => s.name === stream.selected)
  return <section className="logs-page">
    <div className="logs-layout">
      <Panel title={t.lists.processes} className="logs-side">
        <div className="logs-toolbar">
          <label>{t.hints.tailLines}<input type="number" min="20" max="2000" value={stream.tailLines} onChange={e=>stream.setTailLines(Number(e.target.value) || 200)}/></label>
          <button disabled={!stream.selected} onClick={()=>stream.select(stream.selected)}><RefreshCw size={14}/>{t.refresh}</button>
        </div>
        <div className="logs-service-list">
          {services.map(s => <button className={stream.selected===s.name?'log-service active':'log-service'} key={s.name} onClick={()=>stream.select(s.name)}>
            <span className={s.running?'dot running':'dot'}></span>
            <span className="log-service-copy"><span className="log-service-name">{s.name}</span><small>{s.kind}{s.pid ? ` · PID ${s.pid}` : ''}</small></span>
          </button>)}
        </div>
      </Panel>
      <Panel title={`Logs · ${stream.selected || '-'}`} className="log-panel">
        <div className="log-head">
          <div className="log-meta">
            <div className={`stream-state ${stream.streamState}`}><span></span>{label[stream.streamState] || label.idle}</div>
            <span className="log-count">{stream.lines.length} lines · UTF-8</span>
            {stream.selected && <p className="muted log-command" title={selectedService?.command?.join(' ')}>{selectedService?.command?.join(' ')}</p>}
          </div>
          <div className="actions log-actions">
            <button className={stream.follow ? 'active' : ''} disabled={!stream.selected} aria-pressed={stream.follow} onClick={()=>stream.setFollow(value=>!value)}><Activity size={14}/>{lang === 'zh' ? '跟随' : 'Follow'}</button>
            <button disabled={!stream.lines.length} onClick={stream.clear}><Trash2 size={14}/>{t.clear}</button>
            <button disabled={!stream.selected || selectedService?.running} onClick={()=>onStart(stream.selected)}><Play size={14}/>{t.start}</button>
            <button disabled={!stream.selected || !selectedService?.running} onClick={()=>onStop(stream.selected)}><Square size={14}/>{t.stop}</button>
          </div>
        </div>
        {!stream.selected ? <div className="log-selection-empty">{lang === 'zh' ? '选择一个服务以查看日志' : 'Select a service to view logs'}</div> : <>
          <div className="log-stream-controls">
            <span className={`stream-state ${stream.streamState}`} role="status">{status[stream.streamState] || status.idle}</span>
            <span className={`log-follow-status ${stream.follow ? 'following' : 'paused'}`}>{stream.follow ? (lang === 'zh' ? '自动跟随' : 'Following') : (lang === 'zh' ? '已暂停跟随' : 'Follow paused')}</span>
          </div>
          {stream.streamState === 'reconnecting' && <div className="log-stream-error" role="alert">
            <span>{lang === 'zh' ? '日志流连接已中断，可重试。' : 'The log stream was interrupted. You can retry.'}</span>
            <button type="button" onClick={stream.retry}>{t.retry || (lang === 'zh' ? '重试' : 'Retry')}</button>
          </div>}
          {!stream.lines.length && stream.streamState === 'live' && <p className="log-output-empty">{lang === 'zh' ? '当前没有日志输出' : 'No log output yet'}</p>}
          <pre ref={stream.viewRef} onScroll={stream.handleScroll} className="log-view">{stream.lines.join('\n')}</pre>
        </>}
      </Panel>
    </div>
  </section>
}

export default LogsPage
