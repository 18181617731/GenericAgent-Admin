import { useEffect, useState } from 'react'
import { Play, RefreshCw } from 'lucide-react'
import { api } from '../lib/api'
import { TurnList } from '../components/turns'

export function ChatPage({ t }) {
  const [sessions, setSessions] = useState([]), [sid, setSid] = useState(''), [messages, setMessages] = useState([])
  const [prompt, setPrompt] = useState(''), [busy, setBusy] = useState(false), [err, setErr] = useState('')
  const loadSessions = async () => { const d = await api('/api/chat/sessions'); setSessions(d.sessions || []); if (!sid && d.sessions?.[0]) await openSession(d.sessions[0].id) }
  const openSession = async (id) => { const d = await api(`/api/chat/session/${id}`); setSid(d.id); setMessages(d.messages || []) }
  const newSession = async () => { const d = await api('/api/chat/session/new', { method:'POST', body:'{}' }); setSid(d.id); setMessages([]); await loadSessions() }
  useEffect(()=>{ loadSessions().catch(e=>setErr(e.message)) }, [])
  const send = async () => {
    if (!prompt.trim() || busy) return
    let cur = sid
    if (!cur) { const d = await api('/api/chat/session/new', { method:'POST', body:'{}' }); cur = d.id; setSid(cur) }
    const text = prompt; setPrompt(''); setBusy(true); setErr('')
    const user = { id: `u-${Date.now()}`, role:'user', content:text, created_at: Math.floor(Date.now()/1000) }
    const assistant = { id: `a-${Date.now()}`, role:'assistant', content:'', created_at: Math.floor(Date.now()/1000) }
    setMessages(ms => [...ms, user, assistant])
    try {
      const res = await fetch(`/api/chat/${cur}`, { method:'POST', headers:{'Content-Type':'application/json'}, body: JSON.stringify({ prompt:text, client_user_id:user.id }) })
      if (!res.ok) throw new Error(await res.text())
      const reader = res.body.getReader(), dec = new TextDecoder(); let buf = '', content = ''
      while (true) {
        const {value, done} = await reader.read(); if (done) break
        buf += dec.decode(value, {stream:true}); const lines = buf.split('\n'); buf = lines.pop() || ''
        for (const line of lines) { if (!line.trim()) continue; const ev = JSON.parse(line)
          if (ev.type === 'delta') { content += ev.delta || ''; setMessages(ms => ms.map(m => m.id === assistant.id ? {...m, content} : m)) }
          if ((ev.type === 'done' || ev.type === 'error') && ev.message) { setMessages(ms => ms.map(m => m.id === assistant.id ? ev.message : m)); if (ev.type === 'error') setErr(ev.message.content || 'error') }
        }
      }
      await loadSessions()
    } catch(e) { setErr(e.message); setMessages(ms => ms.map(m => m.id === assistant.id ? {...m, content:`失败：${e.message}`, error:true} : m)) }
    finally { setBusy(false) }
  }
  return <section className="chat-shell native-chat"><div className="chat-top"><div><h3>{t.nav.chat}</h3><p>Admin 原生对话：由 Go API 管理会话，按需启动 Python GA Worker。</p></div><div className="actions"><button onClick={loadSessions}><RefreshCw size={14}/>{t.refresh}</button><button onClick={newSession}><Play size={14}/>新会话</button><span className="ok">Native</span></div></div>{err && <div className="message">{err}</div>}<div className="chat-grid"><aside className="chat-sessions"><button className="primary" onClick={newSession}>+ 新会话</button>{sessions.map(s => <button key={s.id} className={s.id===sid?'active':''} onClick={()=>openSession(s.id)}><b>{s.title || '新会话'}</b><small>{s.count || 0} 条</small></button>)}</aside><main className="chat-main"><TurnList messages={messages} empty="选择或创建会话后开始对话"/><div className="chat-compose"><textarea value={prompt} onChange={e=>setPrompt(e.target.value)} onKeyDown={e=>{ if(e.key==='Enter' && e.ctrlKey) send() }} placeholder="输入给 GenericAgent 的任务，Ctrl+Enter 发送"/><button disabled={busy || !prompt.trim()} onClick={send}>{busy?'执行中...':'发送'}</button></div></main></div></section>
}
