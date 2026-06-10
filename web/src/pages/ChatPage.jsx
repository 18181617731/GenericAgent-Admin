import { useEffect, useMemo, useRef, useState } from 'react'
import { Paperclip, Play, RefreshCw, Square, X } from 'lucide-react'
import { api, apiStream } from '../lib/api'
import { TurnList } from '../components/turns'

const readFileDataURL = (file) => new Promise((resolve, reject) => {
  const reader = new FileReader()
  reader.onload = () => resolve({ name: file.name, type: file.type || 'application/octet-stream', dataURL: reader.result })
  reader.onerror = () => reject(reader.error || new Error('读取附件失败'))
  reader.readAsDataURL(file)
})

const compactFileSize = (size = 0) => {
  if (size >= 1024 * 1024) return `${(size / 1024 / 1024).toFixed(1)} MB`
  if (size >= 1024) return `${Math.ceil(size / 1024)} KB`
  return `${size} B`
}

export function ChatPage({ t }) {
  const [sessions, setSessions] = useState([]), [sid, setSid] = useState(''), [messages, setMessages] = useState([])
  const [prompt, setPrompt] = useState(''), [busy, setBusy] = useState(false), [err, setErr] = useState('')
  const [files, setFiles] = useState([])
  const [settings, setSettings] = useState({ llm_no: 0, tools_mode: 'official' })
  const activeSidRef = useRef('')
  const fileInputRef = useRef(null)

  const loadSessions = async () => {
    const d = await api('/api/chat/sessions')
    setSessions(d.sessions || [])
    if (!activeSidRef.current && d.sessions?.[0]) await openSession(d.sessions[0].id)
  }
  const openSession = async (id) => {
    const d = await api(`/api/chat/session/${id}`)
    activeSidRef.current = d.id
    setSid(d.id)
    setMessages(d.messages || [])
    setSettings({ llm_no: d.settings?.llm_no || 0, tools_mode: d.settings?.tools_mode || 'official' })
  }
  const newSession = async () => {
    if (busy) { setErr('当前正在执行，完成后可创建新会话'); return }
    const d = await api('/api/chat/session/new', { method:'POST', body:'{}' })
    activeSidRef.current = d.id
    setSid(d.id)
    setMessages([])
    setFiles([])
    setSettings({ llm_no: d.settings?.llm_no || 0, tools_mode: d.settings?.tools_mode || 'official' })
    await loadSessions()
  }
  useEffect(()=>{ loadSessions().catch(e=>setErr(e.message)) }, [])

  const encodedFiles = useMemo(() => files.map(f => ({ name: f.name, type: f.type, dataURL: f.dataURL })), [files])
  const addFiles = async (list) => {
    const picked = Array.from(list || [])
    if (!picked.length) return
    try {
      const converted = await Promise.all(picked.map(async f => ({ ...(await readFileDataURL(f)), size: f.size })))
      setFiles(fs => [...fs, ...converted].slice(0, 8))
    } catch (e) {
      setErr(e.message || '读取附件失败')
    } finally {
      if (fileInputRef.current) fileInputRef.current.value = ''
    }
  }
  const removeFile = (idx) => setFiles(fs => fs.filter((_, i) => i !== idx))

  const stop = async () => {
    const cur = activeSidRef.current || sid
    if (!cur) return
    try { await api(`/api/chat/${cur}/cancel`, { method:'POST', body:'{}' }) } catch(e) { setErr(e.message) }
  }

  const reinjectTools = async () => {
    const cur = activeSidRef.current || sid
    if (!cur || busy) return
    try {
      const ev = await api(`/api/chat/${cur}/reinject-tools`, { method:'POST', body:'{}' })
      setErr(ev.message || ev.result?.message || 'Tools 已重注入')
    } catch(e) { setErr(e.message) }
  }

  const send = async () => {
    const text = prompt.trim()
    if (text === '/new') {
      setPrompt('')
      await newSession()
      return
    }
    if ((!text && files.length === 0) || busy) return
    let cur = activeSidRef.current || sid
    if (!cur) {
      const d = await api('/api/chat/session/new', { method:'POST', body:'{}' })
      cur = d.id
      activeSidRef.current = d.id
      setSid(cur)
    }
    setPrompt(''); setErr(''); setBusy(true)
    const user = { id: `u-${Date.now()}`, role:'user', content: text || '[附件]', files: files.map(({ dataURL, ...meta }) => meta), created_at: Math.floor(Date.now()/1000) }
    const assistant = { id: `a-${Date.now()}`, role:'assistant', content:'', created_at: Math.floor(Date.now()/1000) }
    setMessages(ms => [...ms, user, assistant])
    const sendFiles = encodedFiles
    setFiles([])
    try {
      const res = await apiStream(`/api/chat/${cur}`, {
        method:'POST',
        body: JSON.stringify({ prompt: text, files: sendFiles, settings, client_user_id: user.id })
      })
      const reader = res.body.getReader(); const dec = new TextDecoder(); let buf = ''
      while (true) {
        const {value, done} = await reader.read(); if (done) break
        buf += dec.decode(value, {stream:true})
        let idx
        while ((idx = buf.indexOf('\n')) >= 0) {
          const line = buf.slice(0, idx).trim(); buf = buf.slice(idx+1)
          if (!line) continue
          const ev = JSON.parse(line)
          if (ev.type === 'delta') setMessages(ms => ms.map(m => m.id === assistant.id ? {...m, content:(m.content||'') + (ev.delta||'')} : m))
          if (ev.type === 'notice') setErr(ev.message?.message || ev.message || 'notice')
          if (ev.type === 'done' || ev.type === 'error') {
            setMessages(ms => ms.map(m => m.id === assistant.id ? ev.message : m))
            if (ev.type === 'error') setErr(ev.message?.content || 'error')
          }
        }
      }
      await loadSessions()
    } catch(e) {
      setErr(e.message)
      setMessages(ms => ms.map(m => m.id === assistant.id ? {...m, content:`失败：${e.message}`, error:true} : m))
    } finally { setBusy(false) }
  }

  return <section className="chat-shell native-chat">
    <div className="chat-top"><div><h3>{t.nav.chat}</h3><p>Admin 原生对话：由 Go API 管理会话，按需启动 Python GA Worker。</p></div><div className="actions"><button onClick={loadSessions}><RefreshCw size={14}/>{t.refresh}</button><button onClick={newSession}><Play size={14}/>新会话</button><span className="ok">Native</span></div></div>
    {err && <div className="message">{err}</div>}
    <div className="chat-grid"><aside className="chat-sessions"><button className="primary" onClick={newSession}>+ 新会话</button>{sessions.map(s => <button key={s.id} className={s.id===sid?'active':''} onClick={()=>openSession(s.id)}><b>{s.title || '新会话'}</b><small>{s.count || 0} 条</small></button>)}</aside>
      <main className="chat-main"><TurnList messages={messages} empty="选择或创建会话后开始对话"/>
        <div className="chat-settings"><label>LLM <input type="number" min="0" value={settings.llm_no} onChange={e=>setSettings(v => ({...v, llm_no:Number(e.target.value)||0}))}/></label><label>Tools <select value={settings.tools_mode} onChange={e=>setSettings(v => ({...v, tools_mode:e.target.value}))}><option value="official">官方模式</option><option value="fixed">固定注入</option></select></label><button type="button" onClick={reinjectTools} disabled={!sid || busy}>重注入 Tools</button></div>
        {files.length > 0 && <div className="chat-attachments">{files.map((f, i) => <span key={`${f.name}-${i}`}><Paperclip size={13}/>{f.name}<small>{compactFileSize(f.size)}</small><button type="button" onClick={()=>removeFile(i)}><X size={12}/></button></span>)}</div>}
        <div className="chat-compose"><input ref={fileInputRef} type="file" multiple hidden onChange={e=>addFiles(e.target.files)}/><button className="icon" type="button" onClick={()=>fileInputRef.current?.click()} disabled={busy}><Paperclip size={16}/></button><textarea value={prompt} onChange={e=>setPrompt(e.target.value)} onKeyDown={e=>{ if(e.key==='Enter' && e.ctrlKey) send() }} placeholder="输入给 GenericAgent 的任务，Ctrl+Enter 发送；可附加图片/文件"/><button disabled={busy || (!prompt.trim() && files.length===0)} onClick={send}>{busy?'执行中...':'发送'}</button>{busy && <button className="danger" type="button" onClick={stop}><Square size={14}/>停止</button>}</div>
      </main></div>
  </section>
}
