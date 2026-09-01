import React, { useMemo } from 'react'
import { ChevronLeft, ChevronRight, FolderOpen, Monitor, RefreshCw, ShieldAlert, Square, Terminal, X } from 'lucide-react'
import { Panel } from '../components/common'
import { StatusNotice } from '../components/feedback'
import RemoteCmdTerminal from './RemoteCmdTerminal'
import { useLocalCmdController } from './localCmdController'

const COPY = {
  zh: {
      title: '远程 CMD', description: '在当前 GA 实例下连接运行 GA Admin 的 Windows 主机终端，可承载 Codex 等全屏交互程序。', pathLabel: '服务端工作目录', pathPlaceholder: '例如：C:\\Users\\你的用户名\\项目', loadDirectory: '读取目录', loadingDirectory: '读取中…', useDirectory: '使用此目录', selectDirectory: '选择服务端目录', root: '磁盘根目录', parent: '上一级', noEntries: '没有可进入的子目录。', create: '新建远程会话', creating: '正在创建会话…', required: '请输入服务端 Windows 主机上的目录。', confirm: path => `将在服务端 Windows 主机的“${path}”启动可执行命令的远程 CMD，会话可被当前浏览器控制。继续吗？`, createCancelled: '已取消创建远程 CMD。', created: '远程 CMD 已连接。', end: '结束会话', endConfirm: '结束远程 CMD 会话？运行中的命令将被终止。', endCancelled: '已取消结束会话。', ended: '远程 CMD 会话已结束。', ending: '正在结束会话…', inputError: '命令输入发送失败。', directoryError: '服务端目录读取失败。', size: '自动尺寸', connecting: '连接中', connected: '已连接', reconnecting: '重连中', exited: '已退出', idle: '未连接', output: '远程终端', shortcuts: '手机快捷键', warning: '当前连接使用明文 HTTP；请在公网环境改用 HTTPS 或安全隧道。', hostNote: '命令在服务端 Windows 主机执行，不会在访问者电脑上打开窗口；会话按当前 GA 实例隔离。', safety: '安全提示：远程 CMD 具有 Admin 服务账号权限，可执行任意命令。请确认路径、网络和命令来源可信。', dismiss: '关闭提示', directoryCurrent: '当前目录', refresh: '刷新目录', clear: '清空终端', shortcutTab: 'Tab', shortcutCtrlC: 'Ctrl+C', shortcutCtrlL: 'Ctrl+L', shortcutEsc: 'Esc', shortcutUp: '↑', shortcutDown: '↓', sessionPath: '服务端路径', terminalHint: '点击终端即可直接逐键输入；移动端可使用下方快捷键。',
  },
  en: {
      title: 'Remote CMD', description: 'Connect to the GA Admin Windows host terminal for the current GA instance, including full-screen apps such as Codex.', pathLabel: 'Server working directory', pathPlaceholder: 'Example: C:\\Users\\your-name\\project', loadDirectory: 'Read directory', loadingDirectory: 'Reading…', useDirectory: 'Use this directory', selectDirectory: 'Choose server directory', root: 'Drive roots', parent: 'Parent', noEntries: 'No child directories.', create: 'New remote session', creating: 'Creating session…', required: 'Enter a directory on the server Windows host.', confirm: path => `Start a command-capable remote CMD in “${path}” on the server Windows host? The current browser can control it. Continue?`, createCancelled: 'Remote CMD creation cancelled.', created: 'Remote CMD connected.', end: 'End session', endConfirm: 'End the remote CMD session? Running commands will be terminated.', endCancelled: 'Session termination cancelled.', ended: 'Remote CMD session ended.', ending: 'Ending session…', inputError: 'Failed to send command input.', directoryError: 'Failed to read server directories.', size: 'Automatic size', connecting: 'Connecting', connected: 'Connected', reconnecting: 'Reconnecting', exited: 'Exited', idle: 'Not connected', output: 'Remote terminal', shortcuts: 'Mobile shortcuts', warning: 'This connection uses plain HTTP; use HTTPS or a secure tunnel on public networks.', hostNote: 'Commands run on the server Windows host; no window opens on the visitor computer. Sessions are isolated by the current GA instance.', safety: 'Safety: remote CMD has the Admin service account permissions and can execute arbitrary commands. Verify the path, network, and commands.', dismiss: 'Dismiss message', directoryCurrent: 'Current directory', refresh: 'Refresh directory', clear: 'Clear terminal', shortcutTab: 'Tab', shortcutCtrlC: 'Ctrl+C', shortcutCtrlL: 'Ctrl+L', shortcutEsc: 'Esc', shortcutUp: '↑', shortcutDown: '↓', sessionPath: 'Server path', terminalHint: 'Click the terminal for direct key-by-key input; mobile users can use the shortcuts below.',
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

function TerminalShortcuts({ text, controller }) {
  const disabled = !controller.session || controller.sessionStatus.current !== 'running' || Boolean(controller.busy)
  const shortcut = (key, label) => <button type="button" className="secondary" onClick={() => controller.sendShortcut(key)} disabled={disabled}>{label}</button>
  return <div className="local-cmd-shortcuts" aria-label={text.shortcuts}><span>{text.shortcuts}</span>{shortcut('Tab', text.shortcutTab)}{shortcut('Ctrl+C', text.shortcutCtrlC)}{shortcut('Ctrl+L', text.shortcutCtrlL)}{shortcut('Escape', text.shortcutEsc)}{shortcut('ArrowUp', text.shortcutUp)}{shortcut('ArrowDown', text.shortcutDown)}</div>
}

function TerminalOutput({ text, controller }) {
  const interactive = controller.sessionStatus.current === 'running'
  return <div className="local-cmd-terminal-wrap">
    <div className="local-cmd-terminal-head"><b>{text.output}</b><button type="button" className="secondary" onClick={controller.clearTerminal} disabled={!controller.terminalChunks.length}><X size={14} aria-hidden="true" />{text.clear}</button></div>
    <RemoteCmdTerminal chunks={controller.terminalChunks} revision={controller.terminalRevision} clearToken={controller.terminalClearToken} interactive={interactive} label={text.output} onData={controller.sendText} onResize={controller.syncTerminalSize} />
    <p className="local-cmd-helper-text">{text.terminalHint}</p>
    <TerminalShortcuts text={text} controller={controller} />
  </div>
}

function SessionControls({ text, controller }) {
  const disabled = !controller.session || Boolean(controller.busy)
  return <div className="local-cmd-session-controls">
    <span className="local-cmd-session-size"><small>{text.size}</small><b>{controller.size.cols} × {controller.size.rows}</b></span>
    <button type="button" className="danger" onClick={controller.end} disabled={disabled}><Square size={14} aria-hidden="true" />{controller.busy === 'end' ? text.ending : text.end}</button>
  </div>
}

function LocalCmdNotes({ text }) {
  const insecure = useMemo(isPublicPlainHTTP, [])
  return <div className="local-cmd-notes"><p><Monitor size={15} aria-hidden="true" /><span>{text.hostNote}</span></p>{insecure && <p className="is-warning"><ShieldAlert size={15} aria-hidden="true" /><span>{text.warning}</span></p>}<p><ShieldAlert size={15} aria-hidden="true" /><span>{text.safety}</span></p></div>
}

export function LocalCmdPage({ lang = 'zh', activeInstanceID = '' }) {
  const text = textFor(lang)
  const controller = useLocalCmdController(text, activeInstanceID)
  const hasSession = Boolean(controller.session)
  return <section className="local-cmd-page">
    <Panel title={<span className="local-cmd-panel-title"><Terminal size={18} aria-hidden="true" />{text.title}</span>} className="local-cmd-panel">
      <p className="local-cmd-description">{text.description}</p>
      {!hasSession && <><label htmlFor="local-cmd-path">{text.pathLabel}</label><div className="local-cmd-path-row"><input id="local-cmd-path" value={controller.path} onChange={event => controller.setPath(event.target.value)} placeholder={text.pathPlaceholder} autoComplete="off" /><button type="button" className="secondary" onClick={() => controller.loadDirectories(controller.path)} disabled={controller.directoryBusy || Boolean(controller.busy)}><FolderOpen size={15} aria-hidden="true" />{controller.directoryBusy ? text.loadingDirectory : text.loadDirectory}</button></div><DirectoryBrowser text={text} controller={controller} /><button type="button" className="primary local-cmd-create" onClick={controller.create} disabled={controller.busy || !controller.path.trim()}>{controller.busy === 'create' ? text.creating : text.create}</button></>}
      {hasSession && <><SessionStatus text={text} controller={controller} /><TerminalOutput text={text} controller={controller} /><SessionControls text={text} controller={controller} /></>}
      {controller.notice && <StatusNotice kind={controller.notice.kind} message={controller.notice.message} onDismiss={controller.dismiss} dismissLabel={text.dismiss} />}
      <LocalCmdNotes text={text} />
    </Panel>
  </section>
}

export default LocalCmdPage
