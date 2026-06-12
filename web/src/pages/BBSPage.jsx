import { useState } from 'react'
import { Eye, MessageSquare, Play, RefreshCw, Save, SlidersHorizontal, Square } from 'lucide-react'
import { buildWorkerSetting, normalizeBBSConnection, REQUIRED_BBS_ENDPOINTS, validateBBSReadmeContract } from '../lib/bbsContract'

export function BBSPage({ status, config, setConfig, onSaveConfig, posts = [], readme = '', title, setTitle, content, setContent, author, setAuthor, onCreate, onReply, onRefresh, onTestConnection, workerService, onWorkerStart, onWorkerStop, onWorkerLogs, onWorkerAutostart, busy }) {
  const [bbsTab, setBbsTab] = useState('board')
  const totalReplies = posts.reduce((n, p) => n + (p.replies?.length || 0), 0)
  const conn = normalizeBBSConnection({ status, config })
  const mode = conn.mode
  const activeBase = conn.activeBase
  const activeKey = conn.boardKey
  const readmeText = String(readme || '')
  const workerConfig = buildWorkerSetting(conn)
  const readmeOK = validateBBSReadmeContract(readmeText)
  const workerState = workerService?.running ? '运行中' : (workerService ? '已停止' : '未配置')
  const needsExternalBase = mode === 'external' && !String(config?.base_url || '').trim()
  const boardReady = conn.enabled && !conn.inputErrors?.length
  const composeBlockedReason = !boardReady ? (conn.error || '协作看板尚未就绪，请先检查服务配置。') : ''
  const trimmedTitle = String(title || '').trim()
  const trimmedContent = String(content || '').trim()
  const workerHint = workerService
    ? (workerService.running ? 'Worker 循环运行中，可通过日志查看最近活动。' : 'Worker 服务已配置但未启动；当前协作板有可执行任务时再启动。')
    : '当前管理会话尚未配置 Worker 服务。请先保存协作板连接，再从服务控制区启动 Worker。'
  const patchConfig = (patch) => setConfig({ ...(config || {}), ...patch })

  return <section className="bbs-page compact-bbs bbs-console">
    <div className="bbs-strip" aria-label="BBS 状态">
      <div className="bbs-titleline">
        <span className="eyebrow">team_work</span>
        <h2>协作看板</h2>
        <span className="muted">{posts.length} 个帖子 / {totalReplies} 条回复 / {mode === 'external' ? '外部' : '内置'} / {boardReady ? '就绪' : '未就绪'}</span>
      </div>
      <div className="bbs-worker-pill" title={workerService?.command?.join(' ') || 'reflect/agent_team_worker.py'}>
        <span className={workerService?.running ? 'dot running' : 'dot'}></span>
        <span>Worker {workerState}</span>
        {workerService?.pid && <small>PID {workerService.pid}</small>}
        <button className="ghost mini" disabled={!workerService || workerService.running || busy} onClick={()=>onWorkerStart?.(workerService.name)}><Play size={13}/>启动</button>
        <button className="ghost mini" disabled={!workerService} onClick={()=>onWorkerLogs?.(workerService.name)}><Eye size={13}/>日志</button>
      </div>
    </div>

    <div className="bbs-tabs" role="tablist" aria-label="BBS 分区">
      <button role="tab" aria-selected={bbsTab === 'board'} className={bbsTab === 'board' ? 'active' : ''} onClick={()=>setBbsTab('board')}><MessageSquare size={15}/>看板</button>
      <button role="tab" aria-selected={bbsTab === 'config'} className={bbsTab === 'config' ? 'active' : ''} onClick={()=>setBbsTab('config')}><SlidersHorizontal size={15}/>服务配置</button>
      <button className="ghost bbs-refresh" onClick={onRefresh} disabled={busy}><RefreshCw size={14}/>刷新</button>
    </div>

    {bbsTab === 'board' && <div className="bbs-tab-panel bbs-board-panel" role="tabpanel">
      <div className="bbs-compose">
        {!boardReady && <div className="empty-card" role="alert"><b>BBS 尚未就绪</b><span>{composeBlockedReason}</span></div>}
        <input className="bbs-title-input" value={title || ''} onChange={e=>setTitle(e.target.value)} placeholder="简短任务标题" disabled={!boardReady || busy} />
        <textarea value={content || ''} onChange={e=>setContent(e.target.value)} placeholder="目标、约束、验收标准..." disabled={!boardReady || busy} />
        <div className="bbs-compose-foot">
          <input value={author || ''} onChange={e=>setAuthor(e.target.value)} placeholder="author admin" disabled={!boardReady || busy} />
          <button onClick={onCreate} disabled={busy || !boardReady || !trimmedTitle || !trimmedContent}><Play size={14}/>发布</button>
        </div>
      </div>
      <div className="bbs-feed">{posts.length ? posts.map(p => <BBSPost key={p.id} post={p} author={author} onReply={onReply} busy={busy || !boardReady}/>) : <div className="empty-card bbs-empty" role="status"><b>{boardReady ? '看板暂无动态' : '未加载到帖子'}</b><span>{boardReady ? '发布一个任务，让 Worker 在这里协作。' : '请修复 BBS 连接或再次刷新；不会假定上一次成功结果仍然有效。'}</span></div>}</div>
    </div>}

    {bbsTab === 'config' && <div className="bbs-tab-panel" role="tabpanel">
      <section className="bbs-config-card tabbed">
        <div className="bbs-config-head"><div><p className="eyebrow">服务</p><h3>连接与 Worker 循环</h3></div>{conn.error && <p className="err">{conn.error}</p>}</div>
        <div className="bbs-mode-row">
          <label className={mode === 'builtin' ? 'selected' : ''}><input type="radio" checked={mode === 'builtin'} onChange={()=>patchConfig({ mode:'builtin' })}/><span><b>内置</b><small>Admin 托管</small></span></label>
          <label className={mode === 'external' ? 'selected' : ''}><input type="radio" checked={mode === 'external'} onChange={()=>patchConfig({ mode:'external' })}/><span><b>外部</b><small>远程共享</small></span></label>
        </div>
        <div className="bbs-config-grid">
          <label><span>内置地址</span><input value={conn.builtinBase || status?.base_url || ''} readOnly /></label>
          <label><span>外部地址</span><input value={config?.base_url || ''} onChange={e=>patchConfig({ base_url:e.target.value })} placeholder="http://host:8787" disabled={mode !== 'external'} /></label>
          <label><span>看板密钥</span><input value={config?.board_key || ''} onChange={e=>patchConfig({ board_key:e.target.value })} placeholder="ga-team" /></label>
          <label><span>当前地址</span><input value={activeBase || '-'} readOnly /></label>
        </div>
        <div className="bbs-health-grid" aria-label="BBS 连接摘要">
          <div><span>模式</span><b>{mode === 'external' ? '外部共享' : '内置 Admin'}</b></div>
          <div><span>状态</span><b className={conn.enabled ? 'ok-text' : 'err'}>{conn.enabled ? '就绪' : '错误'}</b></div>
          <div><span>服务端帖子</span><b>{conn.postCount}</b></div>
          <div><span>README 协议</span><b className={readmeOK ? 'ok-text' : 'warn-text'}>{readmeOK ? '完整' : '需刷新'}</b></div>
        </div>
        {!!conn.inputErrors?.length && <div className="empty-card" role="alert"><b>连接需要处理</b><span>{conn.inputErrors.join(' ')}</span></div>}
        {!!conn.inputErrors?.length && <div className="empty-card" role="alert"><b>连接需要处理</b><span>{conn.inputErrors.join(' ')}</span></div>}
        {needsExternalBase && <div className="empty-card" role="alert"><b>外部 BBS 需要基础 URL</b><span>请输入共享看板地址，保存并测试连接后再发布任务。</span></div>}
        <div className="empty-card" role="status"><b>Worker {workerState}</b><span>{workerHint}</span></div>
        <div className="bbs-config-actions"><button onClick={()=>onSaveConfig(config)} disabled={busy || !!conn.inputErrors?.length}><Save size={14}/>保存</button><button className="ghost" onClick={onTestConnection || onRefresh} disabled={busy}><RefreshCw size={14}/>测试连接</button><button className="ghost" disabled={!workerService || !workerService.running || busy} onClick={()=>onWorkerStop?.(workerService.name)}><Square size={14}/>停止 Worker</button><label className="toggle-inline"><input type="checkbox" disabled={!workerService} checked={!!workerService?.autostart} onChange={e=>onWorkerAutostart?.(workerService.name, e.target.checked)} />自动启动</label></div>
        <details className="bbs-setup"><summary>Worker 设置</summary><div><span>基础地址</span><code>{activeBase || '-'}</code></div><div><span>看板密钥</span><code>{activeKey || '-'}</code></div><div><span>Worker 配置</span><code>{workerConfig}</code></div></details>
        <details className="bbs-readme" open={!readmeOK}><summary>/api/bbs/readme / Worker 协议</summary><ul>{REQUIRED_BBS_ENDPOINTS.map(endpoint => <li key={endpoint} className={readmeText.includes(endpoint) ? 'ok-text' : 'warn-text'}>{endpoint}</li>)}</ul><pre>{readmeText || '点击刷新加载 BBS README。'}</pre></details>
      </section>
    </div>}
  </section>
}

function BBSPost({ post, author, onReply, busy }) {
  const [reply, setReply] = useState('')
  const lastTime = post.updated_at || post.created_at
  const replyAuthor = author || 'admin'
  const replyText = reply.trim()
  return <article className="bbs-post compact-post">
    <header>
      <div><span className="post-id">#{post.id}</span><h3>{post.title}</h3></div>
      <time>{lastTime ? new Date(lastTime).toLocaleString() : ''}</time>
    </header>
    <p className="post-body">{post.content}</p>
    {!!post.replies?.length && <div className="reply-stack">{post.replies.map(r => <div className="reply-line" key={r.id}><b>{r.author || '智能体'}</b><p>{r.content}</p></div>)}</div>}
    <div className="inline-reply"><input value={reply} onChange={e=>setReply(e.target.value)} placeholder={`以 ${replyAuthor} 回复`} disabled={busy} /><button disabled={busy || !replyText} onClick={()=>{ onReply(post.id, replyText); setReply('') }}>发送</button></div>
  </article>
}
