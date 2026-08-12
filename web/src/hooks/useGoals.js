import { useEffect, useRef, useState } from 'react'
import { api } from '../lib/api'
import { confirmDanger } from '../lib/danger'

const pickGoalId = (items = [], preferred = '') => {
  if (preferred && items.some(g => g.id === preferred)) return preferred
  return items.find(g => g.running)?.id || items[0]?.id || ''
}

// Goal Mode runs. While the goals page is active the list and the selected
// run's output tail are polled; stopping honours the control level GA reports.
export function useGoals({ t, lang, setMsg, setBusy, active }) {
  const [goals, setGoals] = useState([])
  const [objective, setObjective] = useState('')
  const [budget, setBudget] = useState(480)
  const [maxTurns, setMaxTurns] = useState(200)
  const [llmNo, setLLMNo] = useState('')
  const [hive, setHive] = useState(false)
  const [selected, setSelected] = useState('')
  const [output, setOutput] = useState('')
  const [outputMeta, setOutputMeta] = useState(null)
  const [outputBytes, setOutputBytesRaw] = useState(() => localStorage.getItem('ga-admin-goal-output-bytes') || '120000')
  const [autoRefresh, setAutoRefreshRaw] = useState(() => localStorage.getItem('ga-admin-goal-auto-refresh') !== 'false')
  const outputSeq = useRef(0)
  const refreshBusy = useRef(false)

  const setOutputBytes = (value) => {
    setOutputBytesRaw(value)
    localStorage.setItem('ga-admin-goal-output-bytes', String(value))
  }
  const setAutoRefresh = (value) => {
    setAutoRefreshRaw(value)
    localStorage.setItem('ga-admin-goal-auto-refresh', value ? 'true' : 'false')
  }

  const loadGoals = async () => {
    const d = await api('/api/goals/list')
    const items = d.goals || []
    setGoals(items)
    return items
  }

  const loadOutput = async (id = selected) => {
    if (!id) return
    setSelected(id)
    const seq = ++outputSeq.current
    try {
      const maxBytes = Number(outputBytes || 0)
      if (!Number.isInteger(maxBytes)) throw new Error(t.hints.goalOutputBytesInteger)
      if (maxBytes < 0) throw new Error(t.hints.goalOutputBytesNonNegative)
      if (maxBytes > 1048576) throw new Error(t.hints.goalOutputBytesTooLarge)
      const d = await api(`/api/goals/output?id=${encodeURIComponent(id)}&max_bytes=${encodeURIComponent(maxBytes)}`)
      if (seq !== outputSeq.current) return
      setOutput(d.output || '')
      setOutputMeta({
        truncated: !!d.truncated,
        totalBytes: d.total_bytes || 0,
        bytesReturned: d.bytes_returned || 0,
        linesReturned: d.lines_returned || 0,
        totalLines: d.total_lines || 0,
        requestedBytes: d.requested_bytes || 0,
        maxBytes: d.max_bytes || 0,
        defaultBytes: d.default_bytes || 0,
        defaultBytesUsed: !!d.default_bytes_used,
        maxBytesCapped: !!d.max_bytes_capped,
        outputStatus: d.output_status || '',
        goal: d.goal || null,
      })
      if (d.goal) setGoals(gs => gs.map(g => g.id === d.goal.id ? d.goal : g))
    } catch (e) {
      if (seq !== outputSeq.current) return
      setMsg(e.message)
      setOutput(e.message)
      setOutputMeta({
        error: e.message,
        bytesReturned: new Blob([e.message || '']).size,
        totalBytes: new Blob([e.message || '']).size,
        requestedBytes: Number(outputBytes || 0),
        maxBytes: Number(outputBytes || 0),
      })
    }
  }

  const start = async () => {
    setBusy(true); setMsg('')
    try {
      const goalObjective = objective.trim()
      const budgetMinutes = Number(budget)
      const turns = Number(maxTurns)
      const model = llmNo === '' ? null : Number(llmNo)
      if (!goalObjective) throw new Error(t.hints.goalObjectiveRequired)
      if (new TextEncoder().encode(goalObjective).length > 16384) throw new Error(t.hints.goalObjectiveTooLarge)
      if (!Number.isInteger(budgetMinutes)) throw new Error(t.hints.goalBudgetInteger)
      if (budgetMinutes <= 0) throw new Error(t.hints.goalBudgetPositive)
      if (budgetMinutes > 43200) throw new Error(t.hints.goalBudgetTooLarge)
      if (!Number.isInteger(turns)) throw new Error(t.hints.goalTurnsInteger)
      if (turns < 0) throw new Error(t.hints.goalTurnsNonNegative)
      if (turns > 10000) throw new Error(t.hints.goalTurnsTooLarge)
      if (model !== null && !Number.isInteger(model)) throw new Error(t.hints.goalLLMInteger)
      if (model !== null && model < 0) throw new Error(t.hints.goalLLMNonNegative)
      const body = { objective: goalObjective, budget_minutes: budgetMinutes, max_turns: turns, hive: !!hive }
      if (model !== null) body.llm_no = model
      if (!confirmDanger('goals-start', lang === 'zh' ? `启动${hive ? ' Hive' : ''}自主目标？预算 ${budgetMinutes} 分钟，最大轮次 ${turns || '不限'}。` : `Start${hive ? ' a Hive' : ' an'} autonomous goal? Budget: ${budgetMinutes} minutes. Maximum turns: ${turns || 'unlimited'}.`)) return
      const d = await api('/api/goals/start', { dangerous:true, method:'POST', body: JSON.stringify(body) })
      setMsg(`${t.hints.goalStarted}: ${d.goal?.id || ''}`)
      setObjective('')
      setSelected(d.goal?.id || selected)
      await loadGoals()
      if (d.goal?.id) await loadOutput(d.goal.id)
    } catch (e) { setMsg(e.message) } finally { setBusy(false) }
  }

  const stop = async (goal) => {
    if (!goal) return
    const exact = !!goal.managed
    const template = exact ? (t.hints.goalStopExactConfirm || t.hints.goalStopConfirm) : (t.hints.goalStopSoftConfirm || t.hints.goalStopConfirm)
    if (!confirmDanger('goals-stop', template.replace('{id}', goal.id || '-').replace('{pid}', goal.pid || '-'))) return
    setBusy(true); setMsg('')
    try {
      const body = { id: goal.id }
      if (goal.pid) body.pid = goal.pid
      await api('/api/goals/stop', { dangerous:true, method:'POST', body: JSON.stringify(body) })
      setMsg(`${t.hints.goalStopped}: ${goal.id}`)
      await loadGoals()
      if (selected === goal.id) await loadOutput(goal.id)
    } catch (e) { setMsg(e.message) } finally { setBusy(false) }
  }

  const remove = async (goal) => {
    if (!goal) return
    if (!confirmDanger('goals-delete', t.hints.goalDeleteConfirm.replace('{id}', goal.id || '-'))) return
    setBusy(true); setMsg('')
    try {
      await api('/api/goals/delete', { dangerous:true, method:'POST', body: JSON.stringify({ id: goal.id }) })
      setMsg(`${t.hints.goalDeleted}: ${goal.id}`)
      const items = await loadGoals()
      if (selected === goal.id) {
        const next = pickGoalId(items, '')
        setSelected(next)
        setOutput('')
        setOutputMeta({})
        if (next) await loadOutput(next)
      }
    } catch (e) { setMsg(e.message) } finally { setBusy(false) }
  }

  const clearOutput = () => {
    outputSeq.current += 1
    setOutput('')
    setOutputMeta(null)
    setMsg(t.hints.goalOutputCleared)
  }

  useEffect(() => {
    if (!active) return undefined
    const refresh = async () => {
      if (refreshBusy.current) return
      refreshBusy.current = true
      try {
        const items = await loadGoals()
        const next = pickGoalId(items, selected)
        if (next) await loadOutput(next)
      } catch (e) {
        setMsg(e.message)
      } finally {
        refreshBusy.current = false
      }
    }
    refresh()
    if (!autoRefresh) return undefined
    const timer = setInterval(refresh, 3000)
    return () => clearInterval(timer)
  }, [active, selected, outputBytes, autoRefresh])

  return {
    goals, loadGoals,
    objective, setObjective, budget, setBudget, maxTurns, setMaxTurns,
    llmNo, setLLMNo, hive, setHive,
    selected, setSelected, output, outputMeta, loadOutput, clearOutput,
    outputBytes, setOutputBytes, autoRefresh, setAutoRefresh,
    start, stop, remove,
  }
}
