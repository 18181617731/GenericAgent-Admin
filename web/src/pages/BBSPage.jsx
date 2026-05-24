import { useState } from 'react'
import { Eye, MessageSquare, Play, RefreshCw, Save, SlidersHorizontal, Square } from 'lucide-react'

export function BBSPage({ status, config, setConfig, onSaveConfig, posts = [], title, setTitle, content, setContent, author, setAuthor, onCreate, onReply, onRefresh, onTestConnection, workerService, onWorkerStart, onWorkerStop, onWorkerLogs, onWorkerAutostart, busy }) {
  const [bbsTab, setBbsTab] = useState('board')
  const totalReplies = posts.reduce((n, p) => n + (p.replies?.length || 0), 0)
  const mode = config?.mode || status?.mode || 'builtin'
  const activeBase = mode === 'external' ? (config?.base_url || status?.base_url || '') : (status?.builtin_base_url || status?.base_url || config?.builtin_base_url || '')
  const activeKey = config?.board_key || status?.board_key || 'ga-team'
  const workerConfig = `{"base_url":"${activeBase || 'http://127.0.0.1:8787'}","board_key":"${activeKey}","name":"worker-1"}`
  const workerState = workerService?.running ? '运行中' : (workerService ? '已停止' : '未发现')
  const patchConfig = (patch) => setConfig({ ...(config || {}), ...patch })
  return <section className="bbs-page compact-bbs bbs-console">
    <div className="bbs-strip" aria-label="BBS status">
      <div className="bbs-titleline">
        <span className="eyebrow">team_work</span>
        <h2>协作频道</h2>
        <span className="muted">{posts.length} 任务 · {totalReplies} 回复 · {mode === 'external' ? '外置' : '内置'}</span>
      </div>
      <div className="bbs-worker-pill" title={workerService?.command?.join(' ') || 'reflect/agent_team_worker.py'}>
        <span className={workerService?.running ? 'dot running' : 'dot'}></span>
        <span>worker {workerState}</span>
        {workerService?.pid && <small>PID {workerService.pid}</small>}
        <button className="ghost mini" disabled={!workerService || workerService.running || busy} onClick={()=>onWorkerStart?.(workerService.name)}><Play size={13}/>启动</button>
        <button className="ghost mini" disabled={!workerService} onClick={()=>onWorkerLogs?.(workerService.name)}><Eye size={13}/>日志</button>
      </div>
    </div>

    <div className="bbs-tabs" role="tablist" aria-label="BBS sections">
      <button role="tab" aria-selected={bbsTab === 'board'} className={bbsTab === 'board' ? 'active' : ''} onClick={()=>setBbsTab('board')}><MessageSquare size={15}/>任务板</button>
      <button role="tab" aria-selected={bbsTab === 'config'} className={bbsTab === 'config' ? 'active' : ''} onClick={()=>setBbsTab('config')}><SlidersHorizontal size={15}/>服务</button>
      <button className="ghost bbs-refresh" onClick={onRefresh} disabled={busy}><RefreshCw size={14}/>刷新</button>
    </div>

    {bbsTab === 'board' && <div className="bbs-tab-panel bbs-board-panel" role="tabpanel">
      <div className="bbs-compose">
        <input className="bbs-title-input" value={title} onChange={e=>setTitle(e.target.value)} placeholder="一句话说明任务" />
        <textarea value={content} onChange={e=>setContent(e.target.value)} placeholder="补充目标、约束、验收方式……" />
        <div className="bbs-compose-foot">
          <input value={author} onChange={e=>setAuthor(e.target.value)} placeholder="作者 admin" />
          <button onClick={onCreate} disabled={busy || !title.trim() || !content.trim()}><Play size={14}/>发布</button>
        </div>
      </div>
      <div className="bbs-feed">{posts.length ? posts.map(p => <BBSPost key={p.id} post={p} author={author} onReply={onReply} busy={busy}/>) : <div className="empty-card bbs-empty"><b>频道空闲</b><span>发布第一条任务，worker 会从这里开始协作。</span></div>}</div>
    </div>}

    {bbsTab === 'config' && <div className="bbs-tab-panel" role="tabpanel">
      <section className="bbs-config-card tabbed">
        <div className="bbs-config-head"><div><p className="eyebrow">service</p><h3>接入与 worker</h3></div>{status?.error && <p className="err">{status.error}</p>}</div>
        <div className="bbs-mode-row">
          <label className={mode === 'builtin' ? 'selected' : ''}><input type="radio" checked={mode === 'builtin'} onChange={()=>patchConfig({ mode:'builtin' })}/><span><b>内置</b><small>Admin 自带</small></span></label>
          <label className={mode === 'external' ? 'selected' : ''}><input type="radio" checked={mode === 'external'} onChange={()=>patchConfig({ mode:'external' })}/><span><b>外置</b><small>远端共享</small></span></label>
        </div>
        <div className="bbs-config-grid">
          <label><span>内置地址</span><input value={status?.builtin_base_url || config?.builtin_base_url || status?.base_url || ''} readOnly /></label>
          <label><span>外置地址</span><input value={config?.base_url || ''} onChange={e=>patchConfig({ base_url:e.target.value })} placeholder="http://host:8787" disabled={mode !== 'external'} /></label>
          <label><span>Board Key</span><input value={config?.board_key || ''} onChange={e=>patchConfig({ board_key:e.target.value })} placeholder="ga-team" /></label>
          <label><span>当前接入</span><input value={activeBase || '-'} readOnly /></label>
        </div>
        <div className="bbs-config-actions"><button onClick={()=>onSaveConfig(config)} disabled={busy || (mode === 'external' && !String(config?.base_url || '').trim())}><Save size={14}/>保存</button><button className="ghost" onClick={onTestConnection || onRefresh} disabled={busy}><RefreshCw size={14}/>测试连接</button><button className="ghost" disabled={!workerService || !workerService.running || busy} onClick={()=>onWorkerStop?.(workerService.name)}><Square size={14}/>停止 worker</button><label className="toggle-inline"><input type="checkbox" disabled={!workerService} checked={!!workerService?.autostart} onChange={e=>onWorkerAutostart?.(workerService.name, e.target.checked)} />自启动</label></div>
        <details className="bbs-setup"><summary>Worker 接入参数</summary><div><span>Base</span><code>{activeBase || '-'}</code></div><div><span>Key</span><code>{activeKey || '-'}</code></div><div><span>setting</span><code>{workerConfig}</code></div></details>
      </section>
    </div>}
  </section>
}

function BBSPost({ post, author, onReply, busy }) {
  const [reply, setReply] = useState('')
  const lastTime = post.updated_at || post.created_at
  return <article className="bbs-post compact-post">
    <header>
      <div><span className="post-id">#{post.id}</span><h3>{post.title}</h3></div>
      <time>{lastTime ? new Date(lastTime).toLocaleString() : ''}</time>
    </header>
    <p className="post-body">{post.content}</p>
    {!!post.replies?.length && <div className="reply-stack">{post.replies.map(r => <div className="reply-line" key={r.id}><b>{r.author || 'agent'}</b><p>{r.content}</p></div>)}</div>}
    <div className="inline-reply"><input value={reply} onChange={e=>setReply(e.target.value)} placeholder={`回复 ${post.author || '任务'}`} /><button disabled={busy || !reply.trim()} onClick={()=>{ onReply(post.id, reply); setReply('') }}>发送</button></div>
  </article>
}
