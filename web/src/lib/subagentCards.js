// Subagent status cards: pure view-model helpers for the chat page.
// Backend: GET /api/chat/subagents/{sid} -> { subagents: [status] }
// status: { name, dir, exists, rounds, round_ended, latest_summary,
//           has_reply, has_expected, stop_requested, intervened, updated_at }

const STALL_MS = 3 * 60 * 1000

// Quick client-side gate: only poll the endpoint when the transcript
// actually references a subagent launch (`--task name`).
export function hasSubagentLaunch(messages) {
  if (!Array.isArray(messages)) return false
  return messages.some(m => typeof m?.content === 'string' && /--task[\s=]+"?[A-Za-z0-9]/.test(m.content))
}

// Derive card display state from a raw status entry.
// tone: run | done | warn | stop
export function subagentCardView(st, now = Date.now()) {
  if (!st || !st.name) return null
  let label = '运行中'
  let tone = 'run'
  if (st.intervened) { label = '已干预'; tone = 'warn' }
  else if (st.stop_requested) { label = '停止请求'; tone = 'stop' }
  else if (st.round_ended) { label = '本轮完成'; tone = 'done' }
  else if (st.updated_at && now - st.updated_at > STALL_MS) { label = '疑似停滞'; tone = 'warn' }
  return {
    name: st.name,
    label,
    tone,
    rounds: st.rounds || 0,
    summary: st.latest_summary || '',
    hasReply: !!st.has_reply,
    hasExpected: !!st.has_expected,
    ago: formatAgo(st.updated_at, now),
  }
}

export function formatAgo(ts, now = Date.now()) {
  if (!ts) return ''
  const d = Math.max(0, now - ts)
  if (d < 15 * 1000) return '刚刚'
  if (d < 60 * 1000) return `${Math.floor(d / 1000)}秒前`
  if (d < 60 * 60 * 1000) return `${Math.floor(d / 60000)}分钟前`
  if (d < 24 * 60 * 60 * 1000) return `${Math.floor(d / 3600000)}小时前`
  return `${Math.floor(d / 86400000)}天前`
}
