import React, { useEffect, useRef, useState } from 'react'
import { CheckCircle2, CircleAlert, Clock3, Eye, EyeOff, ShieldCheck } from 'lucide-react'

const REVEAL_TIMEOUT_MS = 12000

const statusIcon = status => {
  if (status === 'failed') return CircleAlert
  if (status === 'completed') return CheckCircle2
  return Clock3
}

const useTemporaryReveal = available => {
  const [hovered, setHovered] = useState(false)
  const [focused, setFocused] = useState(false)
  const [latched, setLatched] = useState(false)
  const [suppressed, setSuppressed] = useState(false)
  useEffect(() => {
    if (!available) { setHovered(false); setFocused(false); setLatched(false); setSuppressed(false) }
  }, [available])
  useEffect(() => {
    if (!latched) return undefined
    const timer = window.setTimeout(() => setLatched(false), REVEAL_TIMEOUT_MS)
    return () => window.clearTimeout(timer)
  }, [latched])
  const revealed = available && !suppressed && (hovered || focused || latched)
  return {
    revealed,
    latched,
    enter: () => { setSuppressed(false); setHovered(true) },
    leave: () => { setHovered(false); setSuppressed(false) },
    setFocused,
    toggleLatched: () => {
      if (revealed) { setHovered(false); setFocused(false); setLatched(false); setSuppressed(true) }
      else { setSuppressed(false); setLatched(true) }
    },
  }
}

export default function ChatPrivacyCurtain({ lang = 'zh', status = 'waiting', metrics = [], renderResult }) {
  const english = lang === 'en'
  const labels = english ? {
    waiting:'Waiting for a chat', running:'Task running', queued:'Message queued', completed:'Task completed', stopped:'Task stopped', failed:'Task failed',
  } : {
    waiting:'等待新对话', running:'任务执行中', queued:'消息已排队', completed:'任务已完成', stopped:'任务已停止', failed:'任务执行失败',
  }
  const StatusIcon = statusIcon(status)
  const canReveal = status === 'completed' && typeof renderResult === 'function'
  const reveal = useTemporaryReveal(canReveal)
  const pointerFocusRef = useRef(false)
  const revealLabel = reveal.revealed ? (english ? 'Hide latest result' : '收起最后结果') : (english ? 'Temporarily show latest result' : '临时查看最后结果')
  return <section
    className={`oa-privacy-curtain is-${status} ${reveal.revealed ? 'is-revealed' : ''}`}
    aria-live="polite"
    aria-label={english ? 'Task status' : '任务状态'}
    onPointerEnter={event => canReveal && event.pointerType !== 'touch' && reveal.enter()}
    onPointerLeave={reveal.leave}
  >
    <div className="oa-privacy-shield" aria-hidden="true"><ShieldCheck size={24}/></div>
    <div className="oa-privacy-copy">
      <h2>{labels[status] || labels.waiting}</h2>
    </div>
    {metrics.length > 0 && <dl className="oa-privacy-metrics">
      {metrics.map(metric => <div key={metric.label}><dt>{metric.label}</dt><dd>{metric.value}</dd></div>)}
    </dl>}
    <span className="oa-privacy-actions">
      {canReveal && <button
        type="button"
        className="oa-privacy-reveal"
        aria-label={revealLabel}
        aria-pressed={reveal.latched}
        title={revealLabel}
        onPointerDown={() => { pointerFocusRef.current = true }}
        onPointerUp={() => { pointerFocusRef.current = false }}
        onPointerCancel={() => { pointerFocusRef.current = false }}
        onFocus={() => { if (!pointerFocusRef.current) reveal.setFocused(true) }}
        onBlur={() => reveal.setFocused(false)}
        onClick={reveal.toggleLatched}
      >{reveal.revealed ? <EyeOff size={17}/> : <Eye size={17}/>}</button>}
      <span className="oa-privacy-state-icon" aria-hidden="true"><StatusIcon size={17}/></span>
    </span>
    {reveal.revealed && <div className="oa-privacy-result" role="region" aria-label={english ? 'Latest result' : '最后结果'}>
      {renderResult()}
    </div>}
  </section>
}
