import React from 'react'
import { Bot, ChevronDown } from 'lucide-react'
import { subagentCardGroups } from '../lib/subagentCards.js'

function SubagentCard({ view }) {
  return <article className={`oa-subagent-card tone-${view.tone}`}>
    <div className="oa-subagent-head">
      <Bot size={14}/>
      <span className="oa-subagent-name" title={view.name}>{view.name}</span>
      <span className="oa-subagent-state">{view.label}</span>
    </div>
    <div className="oa-subagent-meta">子任务第 {view.rounds} 轮{view.ago ? ` · 最近更新 ${view.ago}` : ''}</div>
    {view.summary && <div className="oa-subagent-summary">{view.summary}</div>}
  </article>
}

function SubagentGrid({ items }) {
  return <div className="oa-subagents">{items.map(view => <SubagentCard key={view.name} view={view}/>)}</div>
}

export function SubagentStatusPanel({ states = [] }) {
  const groups = subagentCardGroups(states)
  if (!groups.current.length && !groups.history.length) return null
  return <section className="oa-subagent-panel" aria-label="本会话子任务">
    <header className="oa-subagent-panel-head">
      <div><strong>本会话子任务</strong><span>每张卡代表一个独立子任务；“子任务第 N 轮”不是历史对话轮次。</span></div>
      <em>{groups.current.length ? `${groups.current.length} 项进行中或需关注` : '当前没有进行中的子任务'}</em>
    </header>
    {groups.current.length > 0 && <SubagentGrid items={groups.current}/>}
    {groups.history.length > 0 && <details className="oa-subagent-history">
      <summary><span>历史任务</span><em>{groups.history.length}</em><small>已完成或较早的停止、干预记录</small><ChevronDown size={16}/></summary>
      <SubagentGrid items={groups.history}/>
    </details>}
  </section>
}

export default SubagentStatusPanel
