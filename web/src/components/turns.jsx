import { useState } from 'react'

function statusIcon(s) {
  const status = normalizeStatus(s)
  if (status === 'running') return '○'  // ○ spinning vibe
  if (status === 'done') return '✓'     // ✓
  if (status === 'error') return '✗'    // ✗
  return '·'                            // · pending
}

function normalizeStatus(s) {
  if (s === 'run') return 'running'
  if (s === 'fail' || s === 'failed') return 'error'
  if (s === 'ok' || s === 'complete' || s === 'completed') return 'done'
  return s || 'pending'
}

const UP_MARKER_RE = /^\s*\[(?:ultraplan|phase|done|fail|subagent|next)\]\s*/i

function getUltraPlanState(message) {
  if (!message) return null
  return message.ultraplan || message.ultraplan_state || message.ultraPlanState || null
}

function filterUltraPlanText(content) {
  if (!content) return ''
  return String(content)
    .split(/\r?\n/)
    .filter(line => !UP_MARKER_RE.test(line.trim()))
    .join('\n')
    .trim()
}

function TaskOutput({ output }) {
  if (!output || output.length === 0) return null
  return (
    <div className="up-task-output">
      <pre className="up-output-log">
        {output.map((line, i) => {
          const text = String(line || '').trimEnd()
          // Highlight <summary> tags
          if (text.includes('<summary>')) {
            return <div key={i} className="up-log-summary">{text}</div>
          }
          // Highlight tool calls (🛠️ emoji)
          if (text.includes('🛠') || text.includes('🛠️')) {
            return <div key={i} className="up-log-tool">{text}</div>
          }
          return <div key={i}>{text}</div>
        })}
      </pre>
    </div>
  )
}

function PhaseRow({ phase, depth = 0, taskOutputs = {} }) {
  const p = phase || {}
  const status = normalizeStatus(p.status)
  const tasks = p.tasks || []
  const children = p.children || []
  const [expandedTask, setExpandedTask] = useState(null)
  const cls = `up-phase up-phase--${status}${p.active ? ' up-phase--active' : ''}`

  return (
    <div className={cls} style={{ paddingLeft: depth * 16 + 12 }}>
      <span className="up-phase-icon">{statusIcon(status)}</span>
      <span className="up-phase-name">{p.name}</span>
      {p.desc ? <span className="up-phase-desc">{p.desc}</span> : null}
      {tasks.length > 0 && (
        <div className="up-tasks">
          {tasks.map((t, i) => {
            const ts = normalizeStatus(t.status)
            const taskId = t.id || `${p.name}_${i}`
            const hasOutput = taskOutputs[taskId] && taskOutputs[taskId].length > 0
            const isExpanded = expandedTask === taskId

            return (
              <div key={i}>
                <div
                  className={`up-task up-task--${ts}${hasOutput ? ' up-task--clickable' : ''}`}
                  onClick={() => {
                    if (hasOutput) setExpandedTask(isExpanded ? null : taskId)
                  }}
                  style={{ cursor: hasOutput ? 'pointer' : 'default' }}
                >
                  <span className="up-task-icon">{statusIcon(ts)}</span>
                  <span className="up-task-desc">{t.desc}</span>
                  {hasOutput && <span className="up-task-expand">{isExpanded ? '▼' : '▶'}</span>}
                </div>
                {isExpanded && <TaskOutput output={taskOutputs[taskId]} />}
              </div>
            )
          })}
        </div>
      )}
      {children.map((c, i) => (
        <PhaseRow key={i} phase={c} depth={depth + 1} taskOutputs={taskOutputs} />
      ))}
    </div>
  )
}

function UltraPlanBoard({ data }) {
  if (!data) return null
  const phases = data.phases || []
  const recentTasks = data.recent_tasks || data.tasks || []
  const events = data.events || []
  const taskOutputs = data.task_outputs || {}
  const done = Boolean(data.done || data.complete)
  return (
    <div className={`up-board${done ? ' up-board--done' : ''}`}>
      <div className="up-header">
        <span className="up-label">{done ? '✓ UltraPlan' : '◎ UltraPlan'}</span>
        <span className="up-objective">{data.objective || 'UltraPlan run'}</span>
      </div>
      {phases.length > 0 && (
        <div className="up-phases">
          {phases.map((p, i) => <PhaseRow key={i} phase={p} taskOutputs={taskOutputs} />)}
        </div>
      )}
      {data.current && !done && (
        <div className="up-current">
          <span className="up-current-dot"></span>
          <span>{data.current}</span>
        </div>
      )}
      {recentTasks.length > 0 && (
        <div className="up-recent">
          <div className="up-section-label">recent tasks</div>
          {recentTasks.slice(-6).map((t, i) => {
            const ts = normalizeStatus(t.status)
            return (
              <div key={i} className={`up-task up-task--${ts}`}>
                <span className="up-task-icon">{statusIcon(ts)}</span>
                <span className="up-task-desc">{t.desc}</span>
              </div>
            )
          })}
        </div>
      )}
      {events.length > 0 && (
        <div className="up-events">
          <div className="up-section-label">events</div>
          {events.slice(-5).map((e, i) => (
            <div key={i} className="up-event"><span>{e.time}s</span>{e.msg}</div>
          ))}
        </div>
      )}
    </div>
  )
}

export function TurnBubble({ message }) {
  const m = message || {}
  const up = getUltraPlanState(m)
  const text = filterUltraPlanText(m.content)
  // Merge task_outputs into ultraplan data
  const upData = up ? {...up, task_outputs: m.task_outputs || {}} : null
  return (
    <div className={`bubble ${m.role || 'assistant'} ${m.type || ''} ${m.error ? 'error' : ''}`}>
      <div className="role">{m.title || m.role || 'assistant'}</div>
      {upData && <UltraPlanBoard data={upData} />}
      {text ? <div className="content">{text}</div> : null}
    </div>
  )
}

export function TurnList({ messages, empty, className = '' }) {
  const items = messages || []
  return (
    <div className={`chat-messages turn-list ${className}`}>
      {items.length === 0 && <div className="empty-chat">{empty}</div>}
      {items.map((m, i) => <TurnBubble key={m.id || i} message={m} />)}
    </div>
  )
}
