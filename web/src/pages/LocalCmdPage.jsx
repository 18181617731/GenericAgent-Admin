import React, { useEffect, useMemo, useRef } from 'react'
import { ChevronLeft, ChevronRight, FolderOpen, Monitor, RefreshCw, Send, ShieldAlert, Square, Terminal, X } from 'lucide-react'
import { Panel } from '../components/common'
import { StatusNotice } from '../components/feedback'
import { useLocalCmdController } from './localCmdController'

const COPY = {
  zh: {
    title: '远程 CMD', description: '命令在运行 GA Admin 的 Windows 主机执行，浏览器只传输终端输入和输出。', pathLabel: '服务端工作目录', pathPlaceholder: '例如：C:\\Users\\你的用户名\\项目', loadDirectory: '读取目录', loadingDirectory: '读取中…', useDirectory: '使用此目录', selectDirectory: '选择服务端目录', root: '磁盘根目录', parent: '上一级', noEntries: '没有可进入的子目录。', create: '新建远程会话', creating: '正在创建会话…', required: '请输入服务端 Windows 主机上的目录。', confirm: path => `将在服务端 Windows 主机的“${path}”启动可执行命令的远程 CMD，会话可被当前浏览器控制。继续吗？`, createCancelled: '已取消创建远程 CMD。', created: '远程 CMD 已连接。', end: '结束会话', endConfirm: '结束远程 CMD 会话？运行中的命令将被终止。', endCancelled: '已取消结束会话。', ended: '远程 CMD 会话已结束。', inputError: '命令输入发送失败。', directoryError: '服务端目录读取失败。', resize: '应用尺寸', resizeConfirm: '应用新的远程终端尺寸？', resized: '终端尺寸已更新。', cols: '列', rows: '行', connecting: '连接中', connected: '已连接', reconnecting: '重连中', exited: '已退出', idle: '未连接', output: '终端输出', command: '输入命令', send: '发送', shortcuts: '手机快捷键', warning: '当前连接使用明文 HTTP；请在公网环境改用 HTTPS 或安全隧道。', hostNote: '命令在服务端 Windows 主机执行，不会在访问者电脑上打开窗口。', safety: '安全提示：远程 CMD 具有 Admin 服务账号权限，可执行任意命令。请确认路径、网络和命令来源可信。', dismiss: '关闭提示', noSession: '先选择目录并创建远程 CMD 会话。', directoryCurrent: '当前目录', refresh: '刷新目录', clear: '清空输出', shortcutTab: 'Tab', shortcutCtrlC: 'Ctrl+C', shortcutCtrlL: 'Ctrl+L', shortcutEsc: 'Esc', shortcutUp: '↑', shortcutDown: '↓', sessionPath: '服务端路径',
  },
  en: {
    title: 'Remote CMD', description: 'Commands run on the Windows host running GA Admin; the browser only transports terminal input and output.', pathLabel: 'Server working directory', pathPlaceholder: 'Example: C:\\Users\\your-name\\project', loadDirectory: 'Read directory', loadingDirectory: 'Reading…', useDirectory: 'Use this directory', selectDirectory: 'Choose server directory', root: 'Drive roots', parent: 'Parent', noEntries: 'No child directories.', create: 'New remote session', creating: 'Creating session…', required: 'Enter a directory on the server Windows host.', confirm: path => `Start a command-capable remote CMD in “${path}” on the server Windows host? The current browser can control it. Continue?`, createCancelled: 'Remote CMD creation cancelled.', created: 'Remote CMD connected.', end: 'End session', endConfirm: 'End the remote CMD session? Running commands will be terminated.', endCancelled: 'Session termination cancelled.', ended: 'Remote CMD session ended.', inputError: 'Failed to send command input.', directoryError: 'Failed to read server directories.', resize: 'Apply size', resizeConfirm: 'Apply the new remote terminal size?', resized: 'Terminal size updated.', cols: 'Columns', rows: 'Rows', connecting: 'Connecting', connected: 'Connected', reconnecting: 'Reconnecting', exited: 'Exited', idle: 'Not connected', output: 'Terminal output', command: 'Command input', send: 'Send', shortcuts: 'Mobile shortcuts', warning: 'This connection uses plain HTTP; use HTTPS or a secure tunnel on public networks.', hostNote: 'Commands run on the server Windows host; no window opens on the visitor computer.', safety: 'Safety: remote CMD has the Admin service account permissions and can execute arbitrary commands. Verify the path, network, and commands.', dismiss: 'Dismiss message', noSession: 'Choose a directory and create a remote CMD session first.', directoryCurrent: 'Current directory', refresh: 'Refresh directory', clear: 'Clear output', shortcutTab: 'Tab', shortcutCtrlC: 'Ctrl+C', shortcutCtrlL: 'Ctrl+L', shortcutEsc: 'Esc', shortcutUp: '↑', shortcutDown: '↓', sessionPath: 'Server path',
  },
}

const textFor = lang => COPY[lang === 'en' ? 'en' : 'zh']

const isPublicPlainHTTP = () => {
  const location = globalThis.location
  if (!location || location.protocol !== 'http:') return false
  return !['localhost', '127.0.0.1', '::1'].includes(location.hostname)
}

function DirectoryBrowser({ text, controller }) {
  const { directories, directoryBusy, setPath, loadDirectories, chooseDirectory } = controller
  const current = directories.current
  return <div className="local-cmd-directory-browser">
    <div className="local-cmd-directory-head"><b>{text.selectDirectory}</b><button type="button" className="secondary" onClick={() => loadDirectories(current)} disabled={directoryBusy}><RefreshCw size={14} aria-hidden="true" />{directoryBusy ? text.loadingDirectory : text.refresh}</button></div>
    <div className="local-cmd-directory-current"><span>{text.directoryCurrent}: {current || text.root}</span>{current && <button type="button" onClick={() => chooseDirectory(directories.parent)} disabled={directoryBusy || !directories.parent}><ChevronLeft size={14} aria-hidden="true" />{text.parent}</button>}</div>
    <div className="local-cmd-directory-list">
      {!current && directories.roots.map(root => <button type="button" key={root} onClick={() => chooseDirectory(root)} disabled={directoryBusy}><FolderOpen size={14} aria-hidden="true" />{root}</button>)}
      {current && directories.entries.map(entry => <button type="button" key={entry.path} onClick={() => chooseDirectory(entry.path)} disabled={directoryBusy}><ChevronRight size={14} aria-hidden="true" />{entry.name}</button>)}
      {current && !directories.entries.length && <span className="muted">{text.noEntries}</span>}
    </div>
    <div className="local-cmd-directory-actions"><button type="button" className="secondary" onClick={() => setPath(current)} disabled={!current || directoryBusy}><FolderOpen size={14} aria-hidden="true" />{text.useDirectory}</button></div>
  </div>
}

function SessionStatus({ text, controller }) {
  const labels = { running: text.connected, connecting: text.connecting, connected: text.connected, reconnecting: text.reconnecting, exited: text.exited, idle: text.idle }
  const status = controller.session?.status || controller.connection
  return <div className="local-cmd-session-status" role="status" aria-live="polite"><span className={`status-pill ${status === 'running' || status === 'connected' ? 'running' : 'stopped'}`}>{labels[status] || status}</span>{controller.session?.path && <code title={controller.session.path}>{text.sessionPath}: {controller.session.path}</code>}</div>
}

function CommandInput({ text, controller }) {
  const inputRef = useRef(null)
  const disabled = !controller.session || controller.sessionStatus.current !== 'running' || Boolean(controller.busy)
  const keyDown = event => {
    if (event.key === 'Enter') { event.preventDefault(); controller.sendCommand(); return }
    if (event.key === 'ArrowUp' || event.key === 'ArrowDown') { event.preventDefault(); controller.moveHistory(event.key === 'ArrowUp' ? 1 : -1); return }
    if (event.key === 'Tab' || event.key === 'Escape' || event.key.startsWith('Arrow')) { event.preventDefault(); controller.shortcut(event.key) }
    if (event.ctrlKey && event.key.toLowerCase() === 'c') { event.preventDefault(); controller.shortcut('Ctrl+C') }
    if (event.ctrlKey && event.key.toLowerCase() === 'l') { event.preventDefault(); controller.shortcut('Ctrl+L') }
  }
  const sendShortcut = (key, label) => <button type="button" className="secondary" onClick={() => { controller.shortcut(key); inputRef.current?.focus() }} disabled={disabled}>{label}</button>
  return <>
    <label htmlFor="local-cmd-input">{text.command}</label>
    <div className="local-cmd-command-row"><input ref={inputRef} id="local-cmd-input" value={controller.input} onChange={event => controller.setInput(event.target.value)} onKeyDown={keyDown} disabled={disabled} autoComplete="off" /><button type="button" className="primary" onClick={controller.sendCommand} disabled={disabled}><Send size={14} aria-hidden="true" />{text.send}</button></div>
    <div className="local-cmd-shortcuts" aria-label={text.shortcuts}><span>{text.shortcuts}</span>{sendShortcut('Tab', text.shortcutTab)}{sendShortcut('Ctrl+C', text.shortcutCtrlC)}{sendShortcut('Ctrl+L', text.shortcutCtrlL)}{sendShortcut('Escape', text.shortcutEsc)}{sendShortcut('ArrowUp', text.shortcutUp)}{sendShortcut('ArrowDown', text.shortcutDown)}</div>
  </>
}

function TerminalOutput({ text, controller }) {
  const outputRef = useRef(null)
  useEffect(() => { if (outputRef.current) outputRef.current.scrollTop = outputRef.current.scrollHeight }, [controller.output])
  return <div className="local-cmd-terminal-wrap"><div className="local-cmd-terminal-head"><b>{text.output}</b><button type="button" className="secondary" onClick={() => controller.setOutput('')} disabled={!controller.output}><X size={14} aria-hidden="true" />{text.clear}</button></div><pre ref={outputRef} className="local-cmd-terminal-output" role="log" aria-label={text.output}>{controller.output || text.noSession}</pre></div>
}

function SessionControls({ text, controller }) {
  const disabled = !controller.session || Boolean(controller.busy)
  return <div className="local-cmd-session-controls">
    <label>{text.cols}<input type="number" min="1" max="500" value={controller.size.cols} onChange={event => controller.setSize(size => ({ ...size, cols: Number(event.target.value) || 1 }))} disabled={disabled} /></label>
    <label>{text.rows}<input type="number" min="1" max="200" value={controller.size.rows} onChange={event => controller.setSize(size => ({ ...size, rows: Number(event.target.value) || 1 }))} disabled={disabled} /></label>
    <button type="button" className="secondary" onClick={controller.resize} disabled={disabled}>{text.resize}</button>
    <button type="button" className="danger" onClick={controller.end} disabled={disabled}><Square size={14} aria-hidden="true" />{controller.busy === 'end' ? text.ended : text.end}</button>
  </div>
}

function LocalCmdNotes({ text }) {
  const insecure = useMemo(isPublicPlainHTTP, [])
  return <div className="local-cmd-notes"><p><Monitor size={15} aria-hidden="true" /><span>{text.hostNote}</span></p>{insecure && <p className="is-warning"><ShieldAlert size={15} aria-hidden="true" /><span>{text.warning}</span></p>}<p><ShieldAlert size={15} aria-hidden="true" /><span>{text.safety}</span></p></div>
}

export function LocalCmdPage({ lang = 'zh' }) {
  const text = textFor(lang)
  const controller = useLocalCmdController(text)
  const hasSession = Boolean(controller.session)
  return <section className="local-cmd-page">
    <Panel title={<span className="local-cmd-panel-title"><Terminal size={18} aria-hidden="true" />{text.title}</span>} className="local-cmd-panel">
      <p className="local-cmd-description">{text.description}</p>
      {!hasSession && <><label htmlFor="local-cmd-path">{text.pathLabel}</label><div className="local-cmd-path-row"><input id="local-cmd-path" value={controller.path} onChange={event => controller.setPath(event.target.value)} placeholder={text.pathPlaceholder} autoComplete="off" /><button type="button" className="secondary" onClick={() => controller.loadDirectories(controller.path)} disabled={controller.directoryBusy || Boolean(controller.busy)}><FolderOpen size={15} aria-hidden="true" />{controller.directoryBusy ? text.loadingDirectory : text.loadDirectory}</button></div><DirectoryBrowser text={text} controller={controller} /><button type="button" className="primary local-cmd-create" onClick={controller.create} disabled={controller.busy || !controller.path.trim()}>{controller.busy === 'create' ? text.creating : text.create}</button></>}
      {hasSession && <><SessionStatus text={text} controller={controller} /><TerminalOutput text={text} controller={controller} /><CommandInput text={text} controller={controller} /><SessionControls text={text} controller={controller} /></>}
      {controller.notice && <StatusNotice kind={controller.notice.kind} message={controller.notice.message} onDismiss={controller.dismiss} dismissLabel={text.dismiss} />}
      <LocalCmdNotes text={text} />
    </Panel>
  </section>
}

export default LocalCmdPage
