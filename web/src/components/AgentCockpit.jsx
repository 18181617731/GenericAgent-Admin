import { useCallback, useEffect, useMemo, useRef, useState } from 'react'
import {
  Activity,
  Bot,
  CheckCircle2,
  CircleAlert,
  Clock3,
  ExternalLink,
  MessageSquareText,
  Radio,
  RefreshCw,
} from 'lucide-react'
import { api } from '../lib/api'
import './AgentCockpit.css'

const POLL_MS = 12 * 1000
const STALE_MS = 3 * 60 * 1000
const MAX_SESSIONS_TO_INSPECT = 12

const COPY = {
  zh: {
    eyebrow: '实时指挥台',
    title: 'Agent 运行驾驶舱',
    intro: '把正在执行、需要关注和刚刚完成的 Agent 收在同一张工作面上。',
    live: '自动刷新',
    refresh: '刷新',
    refreshing: '同步中',
    openChat: '进入会话工作台',
    active: '执行中',
    attention: '需关注',
    finished: '本轮完成',
    sessions: '会话数',
    roster: '运行席位',
    rosterHint: '主 Agent 与子 Agent 的最新状态',
    recent: '最近任务',
    recentHint: '按最后活动时间排列',
    emptyTitle: '当前没有正在运行的 Agent',
    emptyBody: '从会话工作台发起任务后，这里会自动出现它的进度和回报。',
    noSessions: '还没有会话记录',
    retry: '重试',
    loadFailed: '暂时无法读取 Agent 状态',
    partial: '部分子 Agent 状态未返回，已保留其他实时结果。',
    primary: '主 Agent',
    subagent: '子 Agent',
    running: '运行中',
    stalled: '疑似停滞',
    intervened: '已干预',
    stopRequested: '停止请求',
    roundEnded: '本轮完成',
    waiting: '等待状态回报',
    messageCount: n => `${n} 条消息`,
    roundCount: n => `${n} 轮`,
    justNow: '刚刚',
    secondsAgo: n => `${n}秒前`,
    minutesAgo: n => `${n}分钟前`,
    hoursAgo: n => `${n}小时前`,
    daysAgo: n => `${n}天前`,
    never: '尚无时间',
    updated: ago => `${ago}更新`,
    openSession: '打开会话',
    loading: '正在建立 Agent 状态视图…',
    untitled: '未命名任务',
  },
  en: {
    eyebrow: 'LIVE COMMAND',
    title: 'Agent Operations Cockpit',
    intro: 'One working surface for agents in flight, needing attention, or just finished.',
    live: 'Auto refresh',
    refresh: 'Refresh',
    refreshing: 'Syncing',
    openChat: 'Open chat workspace',
    active: 'In flight',
    attention: 'Attention',
    finished: 'Round complete',
    sessions: 'Sessions',
    roster: 'Operations roster',
    rosterHint: 'Latest state from primary agents and subagents',
    recent: 'Recent missions',
    recentHint: 'Ordered by last activity',
    emptyTitle: 'No agents are running right now',
    emptyBody: 'Start a task in Chat and its progress and reports will appear here automatically.',
    noSessions: 'No chat sessions yet',
    retry: 'Retry',
    loadFailed: 'Agent status is temporarily unavailable',
    partial: 'Some subagent states did not return. Other live results are preserved.',
    primary: 'Primary',
    subagent: 'Subagent',
    running: 'Running',
    stalled: 'Possibly stalled',
    intervened: 'Intervened',
    stopRequested: 'Stop requested',
    roundEnded: 'Round complete',
    waiting: 'Waiting for a status report',
    messageCount: n => `${n} messages`,
    roundCount: n => `${n} rounds`,
    justNow: 'just now',
    secondsAgo: n => `${n}s ago`,
    minutesAgo: n => `${n}m ago`,
    hoursAgo: n => `${n}h ago`,
    daysAgo: n => `${n}d ago`,
    never: 'no timestamp',
    updated: ago => `updated ${ago}`,
    openSession: 'Open session',
    loading: 'Building the agent status view…',
    untitled: 'Untitled mission',
  },
}

const timestampMs = value => {
  const parsed = Number(value || 0)
  if (!Number.isFinite(parsed) || parsed <= 0) return 0
  return parsed < 1e12 ? parsed * 1000 : parsed
}

const formatAgo = (value, now, text) => {
  const ts = timestampMs(value)
  if (!ts) return text.never
  const delta = Math.max(0, now - ts)
  if (delta < 15 * 1000) return text.justNow
  if (delta < 60 * 1000) return text.secondsAgo(Math.floor(delta / 1000))
  if (delta < 60 * 60 * 1000) return text.minutesAgo(Math.floor(delta / 60000))
  if (delta < 24 * 60 * 60 * 1000) return text.hoursAgo(Math.floor(delta / 3600000))
  return text.daysAgo(Math.floor(delta / 86400000))
}

const stateForAgent = (agent, now, text) => {
  if (agent.kind === 'primary') return { tone: 'run', label: text.running }
  if (agent.intervened) return { tone: 'warn', label: text.intervened }
  if (agent.stop_requested) return { tone: 'stop', label: text.stopRequested }
  if (agent.round_ended) return { tone: 'done', label: text.roundEnded }
  if (timestampMs(agent.updated_at) && now - timestampMs(agent.updated_at) > STALE_MS) {
    return { tone: 'warn', label: text.stalled }
  }
  return { tone: 'run', label: text.running }
}

const agentSortScore = (agent, now, text) => {
  const tone = stateForAgent(agent, now, text).tone
  const priority = { warn: 4, stop: 3, run: 2, done: 1 }[tone] || 0
  return priority * 1e15 + timestampMs(agent.updated_at)
}

const uniqueSessions = sessions => {
  const seen = new Set()
  return sessions.filter(session => {
    if (!session?.id || seen.has(session.id)) return false
    seen.add(session.id)
    return true
  })
}

function StatusGlyph({ tone }) {
  if (tone === 'done') return <CheckCircle2 aria-hidden="true" />
  if (tone === 'warn' || tone === 'stop') return <CircleAlert aria-hidden="true" />
  return <Activity aria-hidden="true" />
}

export default function AgentCockpit({ lang = 'zh' }) {
  const text = COPY[lang === 'en' ? 'en' : 'zh']
  const requestRef = useRef(0)
  const [snapshot, setSnapshot] = useState({ sessions: [], agents: [], loaded: false, partialFailures: 0 })
  const [error, setError] = useState('')
  const [refreshing, setRefreshing] = useState(false)
  const [now, setNow] = useState(Date.now())

  const load = useCallback(async ({ visible = false } = {}) => {
    const requestID = ++requestRef.current
    if (visible) setRefreshing(true)
    try {
      const payload = await api('/api/chat/sessions')
      const sessions = Array.isArray(payload?.sessions) ? payload.sessions.filter(item => item?.id) : []
      const inspect = uniqueSessions([
        ...sessions.filter(session => session.running),
        ...sessions.slice(0, MAX_SESSIONS_TO_INSPECT),
      ]).slice(0, MAX_SESSIONS_TO_INSPECT)
      const statusResults = await Promise.allSettled(inspect.map(async session => {
        const result = await api(`/api/chat/subagents/${encodeURIComponent(session.id)}`)
        return (Array.isArray(result?.subagents) ? result.subagents : []).map(agent => ({
          ...agent,
          kind: 'subagent',
          sessionID: session.id,
          sessionTitle: session.title || '',
        }))
      }))
      if (requestID !== requestRef.current) return
      const subagents = statusResults.flatMap(result => result.status === 'fulfilled' ? result.value : [])
      const primaryAgents = sessions.filter(session => session.running).map(session => ({
        kind: 'primary',
        name: session.title || text.untitled,
        sessionID: session.id,
        sessionTitle: session.title || '',
        updated_at: session.updated_at,
        count: session.count || 0,
        workspace: session.workspace || '',
      }))
      setSnapshot({
        sessions,
        agents: [...primaryAgents, ...subagents],
        loaded: true,
        partialFailures: statusResults.filter(result => result.status === 'rejected').length,
      })
      setError('')
      setNow(Date.now())
    } catch (loadError) {
      if (requestID !== requestRef.current) return
      setError(loadError?.message || text.loadFailed)
      setSnapshot(previous => ({ ...previous, loaded: true }))
    } finally {
      if (requestID === requestRef.current) setRefreshing(false)
    }
  }, [text.loadFailed, text.untitled])

  useEffect(() => {
    load()
    const poll = window.setInterval(() => {
      if (!document.hidden) load()
    }, POLL_MS)
    const clock = window.setInterval(() => setNow(Date.now()), 30 * 1000)
    return () => {
      requestRef.current += 1
      window.clearInterval(poll)
      window.clearInterval(clock)
    }
  }, [load])

  const agents = useMemo(
    () => [...snapshot.agents].sort((a, b) => agentSortScore(b, now, text) - agentSortScore(a, now, text)),
    [snapshot.agents, now, text],
  )
  const states = useMemo(() => agents.map(agent => stateForAgent(agent, now, text)), [agents, now, text])
  const activeCount = states.filter(state => state.tone === 'run').length
  const attentionCount = states.filter(state => state.tone === 'warn' || state.tone === 'stop').length
  const finishedCount = states.filter(state => state.tone === 'done').length
  const lastUpdated = snapshot.sessions.reduce((latest, session) => Math.max(latest, timestampMs(session.updated_at)), 0)

  return <section className="agent-cockpit" aria-labelledby="agent-cockpit-title" data-testid="agent-cockpit">
    <header className="agent-cockpit__header">
      <div className="agent-cockpit__heading">
        <span className="agent-cockpit__eyebrow"><Radio size={13} aria-hidden="true" />{text.eyebrow}</span>
        <h2 id="agent-cockpit-title">{text.title}</h2>
        <p>{text.intro}</p>
      </div>
      <div className="agent-cockpit__actions">
        <span className="agent-cockpit__live"><i aria-hidden="true" />{text.live}{lastUpdated ? ` · ${formatAgo(lastUpdated, now, text)}` : ''}</span>
        <button type="button" className="secondary" onClick={() => load({ visible: true })} disabled={refreshing}>
          <RefreshCw size={14} className={refreshing ? 'is-spinning' : ''} aria-hidden="true" />
          {refreshing ? text.refreshing : text.refresh}
        </button>
        <a className="agent-cockpit__chat-link" href="/chat">
          {text.openChat}<ExternalLink size={14} aria-hidden="true" />
        </a>
      </div>
    </header>

    <div className="agent-cockpit__metrics" aria-label={text.title}>
      <div className="agent-cockpit__metric is-live"><span>{text.active}</span><strong>{activeCount}</strong></div>
      <div className="agent-cockpit__metric is-attention"><span>{text.attention}</span><strong>{attentionCount}</strong></div>
      <div className="agent-cockpit__metric is-done"><span>{text.finished}</span><strong>{finishedCount}</strong></div>
      <div className="agent-cockpit__metric"><span>{text.sessions}</span><strong>{snapshot.sessions.length}</strong></div>
    </div>

    {error && <div className="agent-cockpit__notice is-error" role="alert">
      <CircleAlert size={16} aria-hidden="true" />
      <span><strong>{text.loadFailed}</strong>{error !== text.loadFailed ? ` — ${error}` : ''}</span>
      <button type="button" onClick={() => load({ visible: true })}>{text.retry}</button>
    </div>}
    {!error && snapshot.partialFailures > 0 && <div className="agent-cockpit__notice" role="status">
      <CircleAlert size={15} aria-hidden="true" /><span>{text.partial}</span>
    </div>}

    <div className="agent-cockpit__board">
      <section className="agent-cockpit__roster" aria-labelledby="agent-roster-title">
        <div className="agent-cockpit__section-title">
          <div><h3 id="agent-roster-title">{text.roster}</h3><p>{text.rosterHint}</p></div>
          <Bot size={18} aria-hidden="true" />
        </div>
        {!snapshot.loaded && <div className="agent-cockpit__loading" aria-live="polite">
          <i aria-hidden="true" /><span>{text.loading}</span>
        </div>}
        {snapshot.loaded && agents.length === 0 && <div className="agent-cockpit__empty">
          <span><Bot size={22} aria-hidden="true" /></span>
          <div><strong>{text.emptyTitle}</strong><p>{text.emptyBody}</p></div>
        </div>}
        {agents.length > 0 && <div className="agent-cockpit__agent-list">
          {agents.slice(0, 10).map((agent, index) => {
            const state = stateForAgent(agent, now, text)
            const title = agent.kind === 'primary' ? agent.name : (agent.name || text.untitled)
            const summary = agent.kind === 'primary'
              ? (agent.workspace || text.messageCount(agent.count || 0))
              : (agent.latest_summary || text.waiting)
            const meta = agent.kind === 'primary'
              ? text.primary
              : `${text.subagent}${agent.rounds ? ` · ${text.roundCount(agent.rounds)}` : ''}`
            return <article className={`agent-cockpit__agent is-${state.tone}`} key={`${agent.kind}-${agent.sessionID}-${agent.name || index}`}>
              <div className="agent-cockpit__agent-state"><StatusGlyph tone={state.tone} /><span>{state.label}</span></div>
              <div className="agent-cockpit__agent-copy">
                <div className="agent-cockpit__agent-name"><strong>{title}</strong><span>{meta}</span></div>
                <p>{summary}</p>
                <small><Clock3 size={12} aria-hidden="true" />{text.updated(formatAgo(agent.updated_at, now, text))}<span aria-hidden="true">·</span>{agent.sessionTitle || text.untitled}</small>
              </div>
              <a href={`/chat?sid=${encodeURIComponent(agent.sessionID)}`} aria-label={`${text.openSession}: ${title}`} title={text.openSession}>
                <ExternalLink size={15} aria-hidden="true" />
              </a>
            </article>
          })}
        </div>}
      </section>

      <aside className="agent-cockpit__recent" aria-labelledby="agent-recent-title">
        <div className="agent-cockpit__section-title">
          <div><h3 id="agent-recent-title">{text.recent}</h3><p>{text.recentHint}</p></div>
          <MessageSquareText size={18} aria-hidden="true" />
        </div>
        {snapshot.loaded && snapshot.sessions.length === 0 && <p className="agent-cockpit__recent-empty">{text.noSessions}</p>}
        <div className="agent-cockpit__session-list">
          {snapshot.sessions.slice(0, 7).map(session => <a href={`/chat?sid=${encodeURIComponent(session.id)}`} className={session.running ? 'is-running' : ''} key={session.id}>
            <i aria-hidden="true" />
            <span><strong>{session.title || text.untitled}</strong><small>{text.messageCount(session.count || 0)} · {formatAgo(session.updated_at, now, text)}</small></span>
            {session.running && <em>{text.running}</em>}
            <ExternalLink size={13} aria-hidden="true" />
          </a>)}
        </div>
      </aside>
    </div>
  </section>
}
