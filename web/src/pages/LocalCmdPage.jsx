import React, { useState } from 'react'
import { FolderOpen, ShieldAlert, Terminal } from 'lucide-react'
import { Panel } from '../components/common'
import { StatusNotice } from '../components/feedback'
import { api } from '../lib/api'
import { confirmDanger } from '../lib/danger'

const COPY = {
  zh: {
    title: '在本机打开 CMD',
    description: '选择或输入任意本机目录，在该目录中启动一个独立、可交互的 Windows 命令提示符窗口。',
    pathLabel: '工作目录',
    pathPlaceholder: '例如：C:\\Users\\你的用户名\\项目',
    browse: '选择目录',
    browseLoading: '正在打开目录选择器…',
    open: '打开 CMD 窗口',
    opening: '正在打开 CMD…',
    required: '请输入要打开的本机目录。',
    cancelled: '已取消选择目录。',
    openCancelled: '已取消打开 CMD。',
    browseSuccess: '目录已选择，可以打开 CMD。',
    openSuccess: 'CMD 窗口已打开。',
    confirm: path => `即将在本机目录“${path}”打开一个可执行命令的 CMD 窗口，是否继续？`,
    windowsOnly: '仅 Windows 支持此功能；窗口会在你的本机桌面独立打开。',
    safety: '安全提示：CMD 具有当前用户权限，可执行任意本机命令。请确认目录和命令来源可信。',
    dismiss: '关闭提示',
  },
  en: {
    title: 'Open CMD on this computer',
    description: 'Choose or enter any local directory to start an independent, interactive Windows command prompt there.',
    pathLabel: 'Working directory',
    pathPlaceholder: 'Example: C:\\Users\\your-name\\project',
    browse: 'Choose directory',
    browseLoading: 'Opening the directory picker…',
    open: 'Open CMD window',
    opening: 'Opening CMD…',
    required: 'Enter a local directory first.',
    cancelled: 'Directory selection was cancelled.',
    openCancelled: 'Opening CMD was cancelled.',
    browseSuccess: 'Directory selected. CMD is ready to open.',
    openSuccess: 'CMD window opened.',
    confirm: path => `Open an interactive command-capable CMD window in “${path}” on this computer?`,
    windowsOnly: 'Windows only; the window opens independently on your local desktop.',
    safety: 'Safety: CMD runs with your user permissions and can execute local commands. Verify the directory and commands before continuing.',
    dismiss: 'Dismiss message',
  },
}

const textFor = lang => COPY[lang === 'en' ? 'en' : 'zh']

const browseDirectory = path => api('/api/setup/browse', { method: 'POST', body: JSON.stringify({ path }) })

const openDirectory = path => api('/api/local-cmd/open', {
  dangerous: true,
  method: 'POST',
  body: JSON.stringify({ path }),
})

const browseAction = async ({ path, text, setPath, setBusyAction, setNotice }) => {
  setBusyAction('browse')
  setNotice({ kind: 'pending', message: text.browseLoading })
  try {
    const result = await browseDirectory(path)
    if (result?.cancelled || result?.ok === false) {
      setNotice({ kind: 'success', message: text.cancelled })
      return
    }
    const selected = String(result?.path || '').trim()
    if (!selected) throw new Error(text.required)
    setPath(selected)
    setNotice({ kind: 'success', message: text.browseSuccess })
  } catch (error) {
    setNotice({ kind: 'error', message: error.message })
  } finally {
    setBusyAction('')
  }
}

const openAction = async ({ path, text, setBusyAction, setNotice }) => {
  const selected = path.trim()
  if (!selected) {
    setNotice({ kind: 'error', message: text.required })
    return
  }
  if (!confirmDanger('local-cmd-open', text.confirm(selected))) {
    setNotice({ kind: 'success', message: text.openCancelled })
    return
  }
  setBusyAction('open')
  setNotice({ kind: 'pending', message: text.opening })
  try {
    await openDirectory(selected)
    setNotice({ kind: 'success', message: text.openSuccess })
  } catch (error) {
    setNotice({ kind: 'error', message: error.message })
  } finally {
    setBusyAction('')
  }
}

const useLocalCmdController = text => {
  const [path, setPath] = useState('')
  const [busyAction, setBusyAction] = useState('')
  const [notice, setNotice] = useState(null)
  const browse = () => browseAction({ path, text, setPath, setBusyAction, setNotice })
  const open = () => openAction({ path, text, setBusyAction, setNotice })
  const dismiss = () => setNotice(null)
  return { path, setPath, busyAction, notice, browse, open, dismiss }
}

function LocalCmdForm({ text, path, setPath, busyAction, onBrowse, onOpen }) {
  const busy = Boolean(busyAction)
  return <div className="local-cmd-form">
    <label htmlFor="local-cmd-path">{text.pathLabel}</label>
    <div className="local-cmd-path-row">
      <input id="local-cmd-path" value={path} onChange={event => setPath(event.target.value)} placeholder={text.pathPlaceholder} autoComplete="off" />
      <button type="button" className="secondary" onClick={onBrowse} disabled={busy}><FolderOpen size={16} aria-hidden="true" />{busyAction === 'browse' ? text.browseLoading : text.browse}</button>
    </div>
    <div className="local-cmd-actions">
      <button type="button" className="primary" onClick={onOpen} disabled={busy || !path.trim()}><Terminal size={16} aria-hidden="true" />{busyAction === 'open' ? text.opening : text.open}</button>
    </div>
  </div>
}

function LocalCmdNotes({ text }) {
  return <div className="local-cmd-notes">
    <p><Terminal size={15} aria-hidden="true" /><span>{text.windowsOnly}</span></p>
    <p><ShieldAlert size={15} aria-hidden="true" /><span>{text.safety}</span></p>
  </div>
}

export function LocalCmdPage({ lang = 'zh' }) {
  const text = textFor(lang)
  const controller = useLocalCmdController(text)
  return <section className="local-cmd-page">
    <Panel title={<span className="local-cmd-panel-title"><Terminal size={18} aria-hidden="true" />{text.title}</span>} className="local-cmd-panel">
      <p className="local-cmd-description">{text.description}</p>
      <LocalCmdForm text={text} {...controller} onBrowse={controller.browse} onOpen={controller.open} />
      {controller.notice && <StatusNotice kind={controller.notice.kind} message={controller.notice.message} onDismiss={controller.dismiss} dismissLabel={text.dismiss} />}
      <LocalCmdNotes text={text} />
    </Panel>
  </section>
}

export default LocalCmdPage
