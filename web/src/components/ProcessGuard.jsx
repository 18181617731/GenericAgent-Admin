import { useEffect, useMemo, useState } from 'react'
import { RefreshCw, ShieldAlert, Skull, Link2, CheckCircle2, AlertTriangle } from 'lucide-react'
import { api } from '../lib/api'
import { confirmDanger } from '../lib/danger'
import { Panel } from './common'

const riskLabel = (risk) => ({ managed: '托管', unmanaged: '未托管', suspicious: '风险' }[risk] || risk || '未知')
const processCommand = (p) => p.command_line || p.cmdline || p.command || p.executable_path || '-'
const processPath = (p) => p.executable_path || p.exe || p.path || '-'

export function ProcessGuard() {
  const [snapshot, setSnapshot] = useState(null)
  const [loading, setLoading] = useState(false)
  const [actingPid, setActingPid] = useState(0)
  const [error, setError] = useState('')
  const [lastAction, setLastAction] = useState(null)

  const processes = snapshot?.items || snapshot?.processes || []
  const counts = useMemo(() => processes.reduce((acc, p) => {
    acc.total += 1
    if (p.managed) acc.managed += 1
    else acc.unmanaged += 1
    if (p.risk && p.risk !== 'managed') acc.risk += 1
    return acc
  }, { total: 0, managed: 0, unmanaged: 0, risk: 0 }), [processes])

  const load = async () => {
    setLoading(true)
    setError('')
    try { setSnapshot(await api('/api/ga/processes')) }
    catch (e) { setError(e.message) }
    finally { setLoading(false) }
  }

  useEffect(() => {
    load()
    const id = setInterval(() => load().catch(() => {}), 10000)
    return () => clearInterval(id)
  }, [])

  const adoptProcess = async (pid) => {
    if (!confirmDanger('ga-process-adopt', `接管 GA 进程 PID ${pid}？这会影响正在运行的任务。`)) return
    setActingPid(pid)
    setError('')
    try {
      const result = await api('/api/ga/processes/adopt', { dangerous:true, method:'POST', body: JSON.stringify({ pid }) })
      setLastAction(result)
      await load()
    } catch (e) {
      setError(e.message)
    } finally {
      setActingPid(0)
    }
  }

  const killProcess = async (pid) => {
    if (!confirmDanger('ga-process-kill', `终止 GA 进程 PID ${pid}？这会影响正在运行的任务。`)) return
    setActingPid(pid)
    setError('')
    try {
      const result = await api('/api/ga/processes/kill', { dangerous:true, method:'POST', body: JSON.stringify({ pid }) })
      setLastAction(result)
      await load()
    } catch (e) {
      setError(e.message)
    } finally {
      setActingPid(0)
    }
  }

  return <Panel title="GA 进程守卫" className="process-guard-panel">
    <div className="process-guard-head">
      <div>
        <p className="muted">巡检 GARoot 下 agentmain / reflect / chat worker，识别游离进程并提供危险确认后终止或接管。</p>
        {snapshot?.scanned_at && <small>最近扫描：{snapshot.scanned_at} · 总计 {counts.total}</small>}
      </div>
      <button type="button" onClick={load} disabled={loading}><RefreshCw size={14}/>{loading ? '扫描中' : '刷新'}</button>
    </div>
    <div className="process-guard-stats">
      <span><CheckCircle2 size={14}/>托管 <b>{counts.managed}</b></span>
      <span><ShieldAlert size={14}/>未托管 <b>{counts.unmanaged}</b></span>
      <span><AlertTriangle size={14}/>风险 <b>{counts.risk}</b></span>
    </div>
    {error && <div className="error process-guard-error">{error}</div>}
    {lastAction && <div className="process-guard-action">{lastAction.action || '操作'} PID {lastAction.pid}: {lastAction.message || '完成'}</div>}
    <div className="process-table-wrap">
      <table className="process-table">
        <thead><tr><th>PID</th><th>类型</th><th>风险</th><th>可执行文件</th><th>命令</th><th>操作</th></tr></thead>
        <tbody>
          {processes.length === 0 && <tr><td colSpan="6" className="muted">未发现 GA 进程。</td></tr>}
          {processes.map(p => <tr key={p.pid} className={p.managed ? 'managed' : 'unmanaged'}>
            <td><b>{p.pid}</b><small>{p.ppid ? `父进程 ${p.ppid}` : ''}</small></td>
            <td>{p.kind || p.name || 'ga'}<small>{p.managed ? '已登记' : '未登记'}</small></td>
            <td><span className={`process-risk risk-${riskLabel(p.risk)}`}>{riskLabel(p.risk)}</span></td>
            <td><code title={processPath(p)}>{processPath(p)}</code><small>{p.cwd ? `工作目录 ${p.cwd}` : ''}</small></td>
            <td><code title={processCommand(p)}>{processCommand(p)}</code></td>
            <td><div className="process-actions">
              {!p.managed && <button type="button" disabled={actingPid === p.pid} onClick={() => adoptProcess(p.pid)}><Link2 size={13}/>接管</button>}
              <button type="button" className="danger" disabled={actingPid === p.pid} onClick={() => killProcess(p.pid)}><Skull size={13}/>终止</button>
            </div></td>
          </tr>)}
        </tbody>
      </table>
    </div>
  </Panel>
}
