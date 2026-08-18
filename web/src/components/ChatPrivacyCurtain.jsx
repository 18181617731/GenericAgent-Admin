import React from 'react'
import { CheckCircle2, CircleAlert, Clock3, LockKeyhole, ShieldCheck } from 'lucide-react'

const statusIcon = status => {
  if (status === 'failed') return CircleAlert
  if (status === 'completed') return CheckCircle2
  return Clock3
}

export default function ChatPrivacyCurtain({ lang = 'zh', status = 'waiting', metrics = [] }) {
  const english = lang === 'en'
  const labels = english ? {
    waiting:'Waiting for a chat', running:'Task running', queued:'Message queued', completed:'Task completed', stopped:'Task stopped', failed:'Task failed',
  } : {
    waiting:'等待新对话', running:'任务执行中', queued:'消息已排队', completed:'任务已完成', stopped:'任务已停止', failed:'任务执行失败',
  }
  const StatusIcon = statusIcon(status)
  return <section className={`oa-privacy-curtain is-${status}`} aria-live="polite" aria-label={english ? 'Privacy mode status' : '隐私模式状态'}>
    <div className="oa-privacy-shield" aria-hidden="true"><ShieldCheck size={24}/></div>
    <div className="oa-privacy-copy">
      <span><LockKeyhole size={13} aria-hidden="true"/>{english ? 'PRIVACY MODE' : '隐私模式'}</span>
      <h2>{labels[status] || labels.waiting}</h2>
    </div>
    {metrics.length > 0 && <dl className="oa-privacy-metrics">
      {metrics.map(metric => <div key={metric.label}><dt>{metric.label}</dt><dd>{metric.value}</dd></div>)}
    </dl>}
    <span className="oa-privacy-state-icon" aria-hidden="true"><StatusIcon size={17}/></span>
  </section>
}
