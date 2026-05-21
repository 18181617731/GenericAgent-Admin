import { useEffect, useMemo, useRef, useState } from 'react'
import { Activity, Bot, Brain, CalendarClock, CheckCircle2, Copy, Eye, FileCode2, FolderCog, Globe2, MessageSquare, Play, RefreshCw, Save, Search, Server, ShieldAlert, Power, SlidersHorizontal, Square, Target, Terminal, Trash2, UploadCloud, XCircle, Download, GitPullRequest, Users } from 'lucide-react'

const api = async (url, options = {}) => {
  const res = await fetch(url, { headers: { 'Content-Type': 'application/json' }, ...options })
  const text = await res.text()
  let body = null
  try { body = text ? JSON.parse(text) : null } catch { throw new Error(`Expected JSON from ${url}, got ${text.slice(0, 40)}`) }
  if (!res.ok) throw new Error(body?.detail || `${res.status} ${res.statusText}`)
  return body
}

const NAV_ITEMS = ['overview','chat','control','files','tasks','bbs','memory','channels','autonomous','goals','models','logs']
const ROUTE_TABS = NAV_ITEMS.filter(n => n !== 'chat')
const TASK_SUB_TABS = ['services','scheduled','reports']

const parseRoute = () => {
  const parts = (window.location.hash || '').replace(/^#\/?/, '').split('/').filter(Boolean)
  const tab = ROUTE_TABS.includes(parts[0]) ? parts[0] : 'overview'
  const taskSubTab = tab === 'tasks' && TASK_SUB_TABS.includes(parts[1]) ? parts[1] : 'services'
  return { tab, taskSubTab }
}

const buildRoute = (tab, taskSubTab = 'services') => tab === 'tasks' ? `#/${tab}/${taskSubTab}` : `#/${tab}`

const I18N = {
  zh: {
    appName: 'GA Admin', tagline: 'GenericAgent 生命周期控制面', root: 'GenericAgent 根目录', setupTitle: '首次配置 GenericAgent', setupDesc: '请选择已有 GA 根目录，或一键安装到新目录。', validateRoot: '验证并使用', installGA: '安装 GA', installPath: '安装目录', setupOk: 'GA 路径已配置', installDone: 'GA 已安装并配置', browse: '选择目录', checkEnv: '检查 Python / Git', envReady: '环境已就绪', envMissing: '环境缺失', save: '保存', refresh: '刷新', busy: '执行中', ready: '就绪', error: '错误', empty: '暂无', enabled: '启用', disabled: '停用', start: '启动', stop: '停止', running: '运行中', stopped: '已停止', language: '语言', copy: '复制', clear: '清空', delete: '删除', show: '显示', hide: '隐藏', search: '搜索', read: '读取', create: '创建', remove: '删除', backup: '写操作会自动备份', autostart: '开机自启', enableAutostart: '开启自启', disableAutostart: '关闭自启', unsupported: '不支持',
    nav: { overview: '总览', chat: '对话', control: '控制面', files: '文件', tasks: '任务', bbs: 'BBS 协作', memory: '记忆', channels: '通道', autonomous: '自主进化', schedule: '定时', goals: 'Goal 模式', models: '模型', logs: '日志' },
    desc: { overview: '从 GA 的功能域理解并接管生命周期。', chat: '迁移自 reactapp 的 GA 原生对话、文件上传和流式聊天界面。', control: '运行前检查、能力地图、风险摘要与最近报告。', files: '安全浏览 GA 根目录内文本文件，支持 tail 与搜索。', tasks: '普通会话、任务文件、批处理入口、任务型服务与 sche_tasks 定时任务。', bbs: '内置 team_work BBS：发布任务帖，供 reflect/agent_team_worker.py 接单协作。', memory: '分层记忆、SOP 与工具能力索引。', channels: '桌面、TUI、Web、IM Bot 等前端入口。', autonomous: '反思、自主运行、Goal Mode 与团队 Worker。', schedule: 'sche_tasks JSON 定时任务详情、编辑、创建与删除。', goals: '复用 GA Goal Mode SOP 与 reflect/goal_mode.py 的持续目标控制台。', models: '读取/预览/写回 GA mykey.py 模型配置。', logs: '进程状态与输出日志。' },
    cards: { processes: '进程', running: '运行中', stopped: '已停止', memoryLayers: '记忆层', sopTools: 'SOP/工具', schedule: '定时任务', enabledTasks: '已启用', reports: '报告', coreFiles: '核心文件', reflect: '反思脚本', health: 'GA 健康', capabilities: '能力', risks: '风险', version: '版本管理' },
    lists: { serviceGroups: '服务域', coreFiles: '核心文件', reflect: 'Reflect / Autonomous', frontends: '前端 / 通道', memory: '记忆层级', sop: 'SOP 与工具', taskServices: '任务服务', frontendServices: '前端服务', reflectServices: '反思服务', reflectScripts: '反思脚本', scheduledTasks: '定时任务', recentReports: '最近报告', processes: '进程', generatedPreview: '生成预览', riskHints: '接管提示', autostart: '开机自启', capabilities: '能力地图', readiness: '运行前检查', fileList: '文件列表', filePreview: '文件预览', searchResults: '搜索结果', editor: '编辑器' },
    hints: { rootSaved: 'GA 根目录已保存', fileSaved: '文件已保存并备份旧文件', taskSaved: '任务已保存并备份旧文件', taskDeleted: '任务已删除并备份', taskToggled: '任务状态已更新', modelsSaved: 'mykey.py 已备份并写回', savedSecret: '已保存；输入新值可替换', secret: 'API Key / Token', noFrontend: '未发现前端服务', noReflect: '未发现 reflect 服务', noTasks: '暂无 sche_tasks/*.json', noLogs: '暂无日志', previewHelp: '点击“预览”查看配置；点击“写回 mykey.py”会先备份再覆盖 GA 的 mykey.py。', modelSource: '来源', secretHidden: '已隐藏真实密钥', addProfile: '新增 Profile', preview: '预览', writeMykey: '写回 mykey.py', filePath: '相对路径', searchText: '搜索文本', tailLines: '尾部行数', newTaskId: 'new_task', jsonHelp: 'JSON 需为对象；保存/删除会生成 .bak 时间戳。', autostartEnabled: '已开启：用户登录后自动启动 GA Admin。', autostartDisabled: '未开启：需要手动启动 GA Admin。', autostartUnsupported: '当前平台暂不支持自动注册。', autostartChanged: '开机自启状态已更新', goalObjectiveRequired: '目标不能为空', goalObjectiveTooLarge: '目标超过 16384 字节', goalBudgetInteger: '预算分钟必须是整数', goalBudgetPositive: '预算分钟必须大于 0', goalBudgetTooLarge: '预算分钟不能超过 43200', goalTurnsInteger: '最大轮次必须是整数', goalTurnsNonNegative: '最大轮次不能为负数', goalTurnsTooLarge: '最大轮次不能超过 10000', goalLLMInteger: 'LLM # 必须是整数', goalLLMNonNegative: 'LLM # 不能为负数', goalPythonHelp: 'Python 留空时自动选择：GA 根目录 .venv、venv、uv 缓存解释器、PATH python/python3；填写后按该路径启动并记录到 Goal 状态。', goalStarted: 'Goal 已启动', goalStopped: 'Goal 已停止', goalDeleted: 'Goal 已删除', goalDeleteConfirm: '确定删除 Goal {id}？会删除状态和日志文件；运行中的目标请先停止。', goalDeleteRunning: '运行中的 Goal 不能删除，请先停止。', goalStopConfirm: '确认停止 Goal {id}？将仅终止该 Goal 记录的精确 PID {pid}。', goalOutputTruncated: '仅显示输出尾部，前面内容已截断', goalOutputCapped: '请求字节数超过后端上限，已按上限读取', goalOutputDefault: '未指定读取字节数，已使用默认值', goalOutputBytesInteger: '输出字节数必须是整数', goalOutputBytesNonNegative: '输出字节数不能为负数', goalOutputBytesTooLarge: '输出字节数不能超过 1048576', goalOutputCopied: '输出已复制', goalOutputCleared: '输出已清空', goalOutputLogMissing: '日志文件尚未创建，当前无可读取输出' },
    goalOutputStatus: { full: '完整', tail_truncated: '尾部截断', empty_log: '空日志', missing_log: '日志缺失' },
    fields: { varName: '变量名', type: '类型', name: '名称', model: '模型', apiBase: 'API Base', apiKey: 'API Key', stream: '流式', maxRetries: '重试', readTimeout: '超时', reasoningEffort: '推理强度', editor: 'JSON 内容', objective: '目标', budgetMinutes: '预算分钟', maxTurns: '最大轮次', llmNo: 'LLM #（可选）', pythonPath: 'Python 解释器（可选）', pythonAuto: '留空自动选择', goalRuns: 'Goal 运行', outputTail: '输出尾部', maxBytes: '最大字节', outputPreset64k: '64K', outputPreset256k: '256K', outputPreset1m: '1M', outputDefault: '默认64K', outputShown: '已显示', outputLines: '行数', outputLimit: '读取上限', autoRefresh: '自动刷新', notRunning: '未运行', startGoalMode: '启动 Goal Mode', goalPlaceholder: '描述要让 GA Goal Mode 持续推进的目标', pid: 'PID', turn: '轮次', remaining: '剩余', elapsed: '已用', started: '开始', ended: '结束', updated: '更新', stateFile: '状态', logFile: '日志', logMissing: '日志未创建', logReady: '日志就绪', outputStatus: '输出状态' }
  },
  en: {
    appName: 'GA Admin', tagline: 'GenericAgent lifecycle control plane', root: 'GenericAgent root', setupTitle: 'First-time GenericAgent setup', setupDesc: 'Select an existing GA root, or install GA into a new directory.', validateRoot: 'Validate & use', installGA: 'Install GA', installPath: 'Install path', setupOk: 'GA root configured', installDone: 'GA installed and configured', browse: 'Choose directory', checkEnv: 'Check Python / Git', envReady: 'Environment ready', envMissing: 'Environment missing', save: 'Save', refresh: 'Refresh', busy: 'Busy', ready: 'Ready', error: 'Error', empty: 'Empty', enabled: 'Enabled', disabled: 'Disabled', start: 'Start', stop: 'Stop', running: 'Running', stopped: 'Stopped', language: 'Language', copy: 'Copy', clear: 'Clear', delete: 'Delete', show: 'Show', hide: 'Hide', search: 'Search', read: 'Read', create: 'Create', remove: 'Delete', backup: 'writes create backups', autostart: 'Autostart', enableAutostart: 'Enable autostart', disableAutostart: 'Disable autostart', unsupported: 'Unsupported',
    nav: { overview: 'Overview', chat: 'Chat', control: 'Control', files: 'Files', tasks: 'Tasks', bbs: 'BBS', memory: 'Memory', channels: 'Channels', autonomous: 'Autonomous', schedule: 'Schedule', goals: 'Goal Mode', models: 'Models', logs: 'Logs' },
    desc: { overview: 'Understand and take over GA lifecycle by native domains.', chat: 'GA native conversation, uploads and streaming UI migrated from reactapp.', control: 'Readiness, capability map, risks and recent reports.', files: 'Safely browse text files inside GA root with tail and search.', tasks: 'Conversations, task files, batch entrypoints and task services.', memory: 'Layered memory, SOPs and utility indexes.', channels: 'Desktop, TUI, Web and IM Bot entrypoints.', autonomous: 'Reflection, autonomous runs, Goal Mode and team workers.', schedule: 'View, edit, create and delete sche_tasks JSON jobs.', goals: 'Continuous objective control console backed by GA Goal Mode SOP and reflect/goal_mode.py.', models: 'Import, preview and write GA mykey.py model config.', logs: 'Process state and output logs.' },
    cards: { processes: 'Processes', running: 'Running', stopped: 'Stopped', memoryLayers: 'Memory layers', sopTools: 'SOP/tools', schedule: 'Scheduled jobs', enabledTasks: 'Enabled', reports: 'Reports', coreFiles: 'Core files', reflect: 'Reflect scripts', health: 'GA health', capabilities: 'Capabilities', risks: 'Risks' },
    lists: { serviceGroups: 'Service domains', coreFiles: 'Core files', reflect: 'Reflect / Autonomous', frontends: 'Frontends / Channels', memory: 'Memory layers', sop: 'SOPs and tools', taskServices: 'Task services', frontendServices: 'Frontend services', reflectServices: 'Reflect services', reflectScripts: 'Reflect scripts', scheduledTasks: 'Scheduled jobs', recentReports: 'Recent reports', processes: 'Processes', generatedPreview: 'Generated preview', riskHints: 'Takeover hints', autostart: 'Autostart', capabilities: 'Capability map', readiness: 'Readiness', fileList: 'Files', filePreview: 'Preview', searchResults: 'Search results', editor: 'Editor' },
    hints: { rootSaved: 'GA root saved', fileSaved: 'File saved with backup', taskSaved: 'Task saved with backup', taskDeleted: 'Task deleted with backup', taskToggled: 'Task state updated', modelsSaved: 'mykey.py backed up and written', savedSecret: 'Saved; type a new value to replace', secret: 'API Key / Token', noFrontend: 'No frontend service found', noReflect: 'No reflect service found', noTasks: 'No sche_tasks/*.json', noLogs: 'No logs', previewHelp: 'Preview generated config; writing mykey.py backs up first.', modelSource: 'Source', secretHidden: 'Real secret hidden', addProfile: 'Add profile', preview: 'Preview', writeMykey: 'Write mykey.py', filePath: 'relative path', searchText: 'search text', tailLines: 'tail lines', newTaskId: 'new_task', jsonHelp: 'JSON must be an object; save/delete creates timestamped .bak.', autostartEnabled: 'Enabled: GA Admin starts automatically after user login.', autostartDisabled: 'Disabled: GA Admin must be started manually.', autostartUnsupported: 'Autostart registration is not supported on this platform.', autostartChanged: 'Autostart status updated', goalObjectiveRequired: 'Objective is required', goalObjectiveTooLarge: 'Objective exceeds 16384 bytes', goalBudgetInteger: 'Budget minutes must be an integer', goalBudgetPositive: 'Budget minutes must be positive', goalBudgetTooLarge: 'Budget minutes exceeds 43200', goalTurnsInteger: 'Max turns must be an integer', goalTurnsNonNegative: 'Max turns must be non-negative', goalTurnsTooLarge: 'Max turns cannot exceed 10000', goalLLMInteger: 'LLM # must be an integer', goalLLMNonNegative: 'LLM # cannot be negative', goalPythonHelp: 'Leave Python empty to auto-select GA root .venv, venv, uv cached interpreter, then PATH python/python3; a custom path is used for launch and recorded in Goal state.', goalStarted: 'Goal started', goalStopped: 'Goal stopped', goalDeleted: 'Goal deleted', goalDeleteConfirm: 'Delete Goal {id}? This removes state and log files; stop running goals first.', goalDeleteRunning: 'Running goals cannot be deleted; stop it first.', goalStopConfirm: 'Stop Goal {id}? Only the exact PID {pid} recorded for this Goal will be terminated.', goalOutputTruncated: 'Showing tail only; earlier output was truncated', goalOutputCapped: 'Requested bytes exceeded backend limit; reading at the cap', goalOutputDefault: 'No byte limit specified; using default', goalOutputBytesInteger: 'Output bytes must be an integer', goalOutputBytesNonNegative: 'Output bytes cannot be negative', goalOutputBytesTooLarge: 'Output bytes cannot exceed 1048576', goalOutputCopied: 'Output copied', goalOutputCleared: 'Output cleared', goalOutputLogMissing: 'Log file has not been created yet; no output is available' },
    goalOutputStatus: { full: 'full', tail_truncated: 'tail truncated', empty_log: 'empty log', missing_log: 'missing log' },
    fields: { varName: 'Var name', type: 'Type', name: 'Name', model: 'Model', apiBase: 'API Base', apiKey: 'API Key', stream: 'Stream', maxRetries: 'Retries', readTimeout: 'Timeout', reasoningEffort: 'Reasoning effort', editor: 'JSON content', objective: 'Objective', budgetMinutes: 'Budget minutes', maxTurns: 'Max turns', llmNo: 'LLM # (optional)', pythonPath: 'Python interpreter (optional)', pythonAuto: 'leave empty for auto', goalRuns: 'Goal runs', outputTail: 'Output tail', maxBytes: 'Max bytes', outputPreset64k: '64K', outputPreset256k: '256K', outputPreset1m: '1M', outputDefault: 'Default 64K', outputShown: 'Shown', outputLines: 'Lines', outputLimit: 'Limit', autoRefresh: 'Auto refresh', notRunning: 'not running', startGoalMode: 'Start Goal Mode', goalPlaceholder: 'Describe the sustained objective for GA Goal Mode', pid: 'PID', turn: 'turn', remaining: 'remaining', elapsed: 'elapsed', started: 'started', ended: 'ended', updated: 'updated', stateFile: 'state', logFile: 'log', logMissing: 'log not created', logReady: 'log ready', outputStatus: 'output status' }
  }
}

const emptyProfile = (idx = 0) => ({ var_name: `MODEL_${idx + 1}`, type: 'openai', name: '', model: '', apibase: '', apikey: '', stream: true, max_retries: 3, read_timeout: 300, reasoning_effort: '', enabled: true })
const safeJson = (v) => JSON.stringify(v ?? {}, null, 2)
const group = (items, pred) => (items || []).filter(pred)
const formatDuration = (seconds) => {
  const n = Math.max(0, Number(seconds) || 0)
  const h = Math.floor(n / 3600), m = Math.floor((n % 3600) / 60), s = Math.floor(n % 60)
  return h ? `${h}h ${m}m` : (m ? `${m}m ${s}s` : `${s}s`)
}
const formatGoalTime = (value) => value ? new Date(value).toLocaleString() : '-'
const formatBytes = (bytes) => {
  const n = Math.max(0, Number(bytes) || 0)
  if (n >= 1024 * 1024) return `${(n / (1024 * 1024)).toFixed(n >= 10 * 1024 * 1024 ? 0 : 1)} MiB`
  if (n >= 1024) return `${(n / 1024).toFixed(n >= 10 * 1024 ? 0 : 1)} KiB`
  return `${n} B`
}
const outputLineCount = (text) => {
  if (!text) return 0
  const normalized = String(text).replace(/\r?\n$/, '')
  return normalized ? normalized.split(/\r?\n/).length : 0
}


function Stat({ label, value, icon }) { return <div className="stat"><div>{icon}</div><span>{label}</span><b>{value}</b></div> }
function Panel({ title, children, className = '' }) { return <div className={`panel ${className}`}><div className="panel-title">{title}</div>{children}</div> }
function EntryList({ items = [], empty }) { return <div className="entry-list">{items.length ? items.map((e, i) => <div className="entry" key={`${e.path || e.name}-${i}`}><b>{e.name || e.path}</b><span>{e.path}{e.kind ? ` · ${e.kind}` : ''}{e.size ? ` · ${e.size} B` : ''}</span></div>) : <p className="muted">{empty}</p>}</div> }
function ServiceRow({ svc, onStart, onStop, onLogs, onAutostart, t }) { return <div className="service-card"><div><b>{svc.name}</b><span title={svc.command?.join(' ')}>{svc.command?.join(' ')}</span><em>{svc.kind}{svc.pid ? ` · PID ${svc.pid}` : ''}</em></div><div className={svc.running ? 'ok' : 'err'}>{svc.running ? t.running : t.stopped}</div><div className="svc-actions"><button disabled={svc.running} onClick={() => onStart(svc.name)}><Play size={14}/>{t.start}</button><button disabled={!svc.running} onClick={() => onStop(svc.name)}><Square size={14}/>{t.stop}</button><button onClick={() => onLogs?.(svc.name)}><Eye size={14}/>{t.logs}</button><label className="toggle-inline"><input type="checkbox" checked={!!svc.autostart} onChange={e => onAutostart?.(svc.name, e.target.checked)} />{t.autostartService}</label></div></div> }
function ChannelServiceTable({ services = [], onStart, onStop, onLogs, onAutostart, t }) { return <div className="channel-table-wrap"><table className="channel-table"><thead><tr><th>{t.fields?.name || 'Name'}</th><th>{t.fields?.status || 'Status'}</th><th>PID</th><th>{t.fields?.command || 'Command'}</th><th>{t.autostart}</th><th>{t.actions || 'Actions'}</th></tr></thead><tbody>{services.length ? services.map(svc => <tr key={svc.name}><td><b>{svc.name}</b><small>{svc.kind}</small></td><td><span className={svc.running ? 'status-pill running' : 'status-pill stopped'}>{svc.running ? t.running : t.stopped}</span></td><td>{svc.pid || '-'}</td><td><code title={svc.command?.join(' ')}>{svc.command?.join(' ') || '-'}</code></td><td><label className="toggle-inline table-toggle"><input type="checkbox" checked={!!svc.autostart} onChange={e => onAutostart?.(svc.name, e.target.checked)} />{svc.autostart ? t.enabled : t.disabled}</label></td><td><div className="svc-actions table-actions"><button disabled={svc.running} onClick={() => onStart(svc.name)}><Play size={14}/>{t.start}</button><button disabled={!svc.running} onClick={() => onStop(svc.name)}><Square size={14}/>{t.stop}</button><button onClick={() => onLogs?.(svc.name)}><Eye size={14}/>{t.logs}</button></div></td></tr>) : <tr><td colSpan="6" className="empty-cell">{t.hints.noFrontend}</td></tr>}</tbody></table></div> }
function SecretInput({ value, onChange, t }) { const [show, setShow] = useState(false); return <div className="secret-row"><input type={show ? 'text' : 'password'} value={value || ''} placeholder={t.hints.savedSecret} onChange={e => onChange(e.target.value)} /><button type="button" onClick={() => setShow(!show)}>{show ? t.hide : t.show}</button></div> }

export default function App() {
  const defaultLang = (navigator.language || '').toLowerCase().startsWith('zh') ? 'zh' : 'en'
  const [lang, setLang] = useState(localStorage.getItem('ga-admin-lang') || defaultLang)
  const t = I18N[lang] || I18N.en
  const initialRoute = useMemo(() => parseRoute(), [])
  const [tab, setTab] = useState(initialRoute.tab)
  const [cfg, setCfg] = useState(null), [health, setHealth] = useState(null), [control, setControl] = useState(null), [services, setServices] = useState([]), [logs, setLogs] = useState([])
  const [root, setRoot] = useState(''), [installRoot, setInstallRoot] = useState(''), [busy, setBusy] = useState(false), [msg, setMsg] = useState(''), [selected, setSelected] = useState('')
  const [setupEnv, setSetupEnv] = useState(null)
  const [autostart, setAutostart] = useState(null)
  const [versionInfo, setVersionInfo] = useState(null), [versionCheck, setVersionCheck] = useState(null), [versionStatus, setVersionStatus] = useState(null), [versionBusy, setVersionBusy] = useState(false), [gitBusy, setGitBusy] = useState(false), [gitResult, setGitResult] = useState(null), [gitStatus, setGitStatus] = useState(null)
  const [profiles, setProfiles] = useState([]), [modelPreview, setModelPreview] = useState('')
  const [bbsStatus, setBbsStatus] = useState(null), [bbsPosts, setBbsPosts] = useState([]), [bbsTitle, setBbsTitle] = useState(''), [bbsContent, setBbsContent] = useState(''), [bbsAuthor, setBbsAuthor] = useState('admin')
  const [bbsConfig, setBbsConfig] = useState({ mode:'builtin', base_url:'', board_key:'ga-team' })
  const [filePath, setFilePath] = useState('memory'), [fileList, setFileList] = useState([]), [fileContent, setFileContent] = useState(''), [fileSearch, setFileSearch] = useState(''), [searchHits, setSearchHits] = useState([]), [tailLines, setTailLines] = useState(200)
  const [taskId, setTaskId] = useState(''), [taskEditor, setTaskEditor] = useState('{}'), [newTaskId, setNewTaskId] = useState('new_task')
  const [taskSubTab, setTaskSubTab] = useState(initialRoute.taskSubTab)
  const [scheduleArtifactTitle, setScheduleArtifactTitle] = useState(''), [scheduleArtifact, setScheduleArtifact] = useState('')
  const [goals, setGoals] = useState([]), [goalObjective, setGoalObjective] = useState(''), [goalBudget, setGoalBudget] = useState(480), [goalMaxTurns, setGoalMaxTurns] = useState(200), [goalLLMNo, setGoalLLMNo] = useState(''), [selectedGoal, setSelectedGoal] = useState(''), [goalOutput, setGoalOutput] = useState(''), [goalOutputMeta, setGoalOutputMeta] = useState(null)
  const [goalOutputBytes, setGoalOutputBytes] = useState(() => localStorage.getItem('ga-admin-goal-output-bytes') || '120000')
  const [goalAutoRefresh, setGoalAutoRefresh] = useState(() => localStorage.getItem('ga-admin-goal-auto-refresh') !== 'false')
  const goalOutputSeq = useRef(0), goalRefreshBusy = useRef(false)

  const inv = health?.inventory || {}
  const schedule = inv.schedule || {}
  const tasks = schedule.tasks || []
  const taskSvcs = useMemo(() => group(services, s => s.kind === 'task' || s.name?.includes('task')), [services])
  const frontendSvcs = useMemo(() => group(services, s => s.kind === 'frontend'), [services])
  const reflectSvcs = useMemo(() => group(services, s => s.kind === 'reflect' || s.name?.includes('reflect') || s.name?.includes('autonomous')), [services])
  const bbsWorkerSvc = useMemo(() => services.find(s => s.name === 'agent_team_worker.py' || s.name?.includes('agent_team_worker.py')), [services])

  const loadBBS = async () => {
    const [status, cfg, posts] = await Promise.all([api('/api/bbs/status'), api('/api/bbs/config'), api('/api/bbs/posts?limit=100')])
    setBbsStatus(status); setBbsConfig({ mode: cfg.mode || 'builtin', base_url: cfg.base_url || '', board_key: cfg.board_key || 'ga-team', builtin_base_url: cfg.builtin_base_url || status?.builtin_base_url || status?.base_url || '' }); setBbsPosts(Array.isArray(posts) ? posts : [])
    return { status, cfg, posts }
  }
  const saveBBSConfig = async (next = bbsConfig) => {
    setBusy(true); setMsg('')
    try {
      const cfg = await api('/api/bbs/config', { method:'POST', body: JSON.stringify(next) })
      setBbsConfig({ mode: cfg.mode || 'builtin', base_url: cfg.base_url || '', board_key: cfg.board_key || 'ga-team', builtin_base_url: cfg.builtin_base_url || '' })
      setMsg('BBS 服务配置已保存')
      await loadBBS()
    } catch(e){ setMsg(e.message) } finally{ setBusy(false) }
  }
  const testBBSConnection = async () => {
    setBusy(true); setMsg('正在测试 BBS 连接…')
    try {
      const { status, posts } = await loadBBS()
      const mode = status?.mode === 'external' ? '外置' : '内置'
      const count = Array.isArray(posts) ? posts.length : 0
      setMsg(`BBS ${mode}服务连接正常，已读取 ${count} 条任务`)
    } catch(e){ setMsg(`BBS 连接失败：${e.message}`) } finally{ setBusy(false) }
  }
  const createBBSPost = async () => {
    setBusy(true); setMsg('')
    try {
      await api('/api/bbs/posts', { method:'POST', body: JSON.stringify({ title:bbsTitle, content:bbsContent, author:bbsAuthor || 'admin', tags:['task'] }) })
      setBbsTitle(''); setBbsContent(''); setMsg('BBS 任务帖已发布')
      await loadBBS()
    } catch(e){ setMsg(e.message) } finally{ setBusy(false) }
  }
  const createBBSReply = async (post_id, content) => {
    if (!content?.trim()) return
    setBusy(true); setMsg('')
    try { await api('/api/bbs/reply', { method:'POST', body: JSON.stringify({ post_id, author:bbsAuthor || 'admin', content }) }); await loadBBS() } catch(e){ setMsg(e.message) } finally{ setBusy(false) }
  }

  const load = async () => {
    setBusy(true); setMsg('')
    try {
      const [c, h, auto, ver, vstat] = await Promise.all([api('/api/config'), api('/api/ga/health'), api('/api/autostart/status').catch(e => ({ supported:false, enabled:false, error:e.message })), api('/api/version/info').catch(e => ({ error:e.message })), api('/api/version/status').catch(() => null)])
      setCfg(c); setRoot(c.ga_root || ''); setHealth(h); setAutostart(auto); setVersionInfo(ver); if (vstat?.id || vstat?.stage) setVersionStatus(vstat)
      if (!h?.ok) {
        setServices([]); setControl(null); setLogs([]); setFileList([])
        return
      }
      const [svc, ctrl, goalData] = await Promise.all([api('/api/services'), api('/api/ga/control'), api('/api/goals/list').catch(() => ({ goals: [] }))])
      const serviceList = Array.isArray(svc) ? svc : (svc.services || [])
      const goalItems = goalData.goals || []
      setServices(serviceList); setControl(ctrl); setGoals(goalItems)
      const first = serviceList[0]?.name; if (!selected && first) setSelected(first)
      const firstGoal = pickGoalId(goalItems, selectedGoal)
      if (!selectedGoal && firstGoal) setSelectedGoal(firstGoal)
      await Promise.all([loadFiles(filePath), loadBBS().catch(e => setBbsStatus({ error:e.message }))])
    } catch (e) { setMsg(e.message) } finally { setBusy(false) }
  }
  useEffect(() => { load() }, [])
  useEffect(() => {
    const onHashChange = () => {
      const route = parseRoute()
      setTab(route.tab)
      setTaskSubTab(route.taskSubTab)
    }
    window.addEventListener('hashchange', onHashChange)
    return () => window.removeEventListener('hashchange', onHashChange)
  }, [])
  useEffect(() => {
    const next = buildRoute(tab, taskSubTab)
    if (window.location.hash !== next) window.history.replaceState(null, '', next)
  }, [tab, taskSubTab])
  useEffect(() => { localStorage.setItem('ga-admin-lang', lang) }, [lang])
  useEffect(() => { localStorage.setItem('ga-admin-goal-output-bytes', String(goalOutputBytes)) }, [goalOutputBytes])
  useEffect(() => { localStorage.setItem('ga-admin-goal-auto-refresh', goalAutoRefresh ? 'true' : 'false') }, [goalAutoRefresh])
  useEffect(() => { if (selected) api(`/api/logs/${encodeURIComponent(selected)}`).then(d => setLogs(d.lines || [])).catch(e => setMsg(e.message)) }, [selected])

  const toggleAutostart = async () => { setBusy(true); setMsg(''); try { const next = !autostart?.enabled; const d = await api(next ? '/api/autostart/enable' : '/api/autostart/disable', { method:'POST' }); setAutostart(d); setMsg(t.hints.autostartChanged) } catch(e){ setMsg(e.message) } finally{ setBusy(false) } }
  const checkGASource = async () => { setGitBusy(true); setMsg(''); try { const d = await api('/api/ga/git-status?remote=1'); setGitStatus(d); setMsg(d.latest ? 'GA 源代码已是最新' : `GA 源代码落后 ${d.behind || 0} 个提交`) } catch(e){ setGitStatus({ ok:false, error:e.message }); setMsg(e.message) } finally{ setGitBusy(false) } }
  const updateGASource = async () => { if (!window.confirm('使用 git pull --ff-only 更新当前 GA 源代码？请确保本地修改已提交或可快进。')) return; setGitBusy(true); setMsg(''); try { const d = await api('/api/ga/git-update', { method:'POST', body: '{}' }); setGitResult(d); setGitStatus(d); setMsg(d.changed ? `GA 源代码已更新: ${d.before} → ${d.after}` : 'GA 源代码已是最新'); await load() } catch(e){ setMsg(e.message) } finally{ setGitBusy(false) } }
  const saveConfig = async () => { setBusy(true); try { const c = await api('/api/config', { method: 'PUT', body: JSON.stringify({ ...cfg, ga_root: root }) }); setCfg(c); setMsg(t.hints.rootSaved); await load() } catch(e){ setMsg(e.message) } finally{ setBusy(false) } }
  const checkSetupEnv = async () => { setBusy(true); try { const d = await api('/api/setup/env'); setSetupEnv(d); setMsg(d.ok ? t.envReady : t.envMissing) } catch(e){ setMsg(e.message) } finally{ setBusy(false) } }
  const browseSetupDir = async (target = 'root') => { setBusy(true); try { const base = target === 'install' ? installRoot : root; const d = await api('/api/setup/browse', { method:'POST', body: JSON.stringify({ path: base }) }); if (d.path) { target === 'install' ? setInstallRoot(d.path) : setRoot(d.path) } } catch(e){ setMsg(e.message) } finally{ setBusy(false) } }
  const validateSetupRoot = async () => { setBusy(true); try { const d = await api('/api/setup/validate', { method:'POST', body: JSON.stringify({ path: root }) }); if (!d.ok) throw new Error('GenericAgent health check failed'); const c = await api('/api/config', { method:'PUT', body: JSON.stringify({ ...cfg, ga_root: d.root }) }); setCfg(c); setRoot(d.root); setMsg(t.setupOk); await load() } catch(e){ setMsg(e.message) } finally{ setBusy(false) } }
  const installGA = async () => { setBusy(true); try { const env = setupEnv || await api('/api/setup/env'); setSetupEnv(env); if (!env.ok) throw new Error(t.envMissing); const d = await api('/api/setup/install', { method:'POST', body: JSON.stringify({ path: installRoot || root }) }); setRoot(d.root); setMsg(t.installDone); await load() } catch(e){ setMsg(e.message) } finally{ setBusy(false) } }
  const serviceAction = async (name, action) => { setBusy(true); try { await api(`/api/services/${action}`, { method:'POST', body: JSON.stringify({ name }) }); await load(); if (selected === name) setLogs((await api(`/api/logs/${encodeURIComponent(name)}?lines=${tailLines}`)).lines || []) } catch(e){ setMsg(e.message) } finally{ setBusy(false) } }
  const toggleServiceAutostart = async (name, enabled) => { setBusy(true); try { const d = await api('/api/services/autostart', { method:'POST', body: JSON.stringify({ name, enabled }) }); setServices(d.services || []); setMsg(enabled ? t.enabled : t.disabled) } catch(e){ setMsg(e.message) } finally{ setBusy(false) } }
  const loadServiceLogs = async (name = selected) => { if (!name) return; setSelected(name); setLogs((await api(`/api/logs/${encodeURIComponent(name)}?lines=${tailLines}`)).lines || []) }
  const viewServiceLogs = async (name) => { setTab('logs'); await loadServiceLogs(name) }
  const pickGoalId = (items = [], preferred = '') => {
    if (preferred && items.some(g => g.id === preferred)) return preferred
    return items.find(g => g.running)?.id || items[0]?.id || ''
  }
  const loadGoals = async () => { const d = await api('/api/goals/list'); const items = d.goals || []; setGoals(items); return items }
  const startGoal = async () => {
    setBusy(true); setMsg('')
    try {
      const objective = goalObjective.trim()
      const budgetMinutes = Number(goalBudget)
      const maxTurns = Number(goalMaxTurns)
      const llmNo = goalLLMNo === '' ? null : Number(goalLLMNo)
      if (!objective) throw new Error(t.hints.goalObjectiveRequired)
      if (new TextEncoder().encode(objective).length > 16384) throw new Error(t.hints.goalObjectiveTooLarge)
      if (!Number.isInteger(budgetMinutes)) throw new Error(t.hints.goalBudgetInteger)
      if (budgetMinutes <= 0) throw new Error(t.hints.goalBudgetPositive)
      if (budgetMinutes > 43200) throw new Error(t.hints.goalBudgetTooLarge)
      if (!Number.isInteger(maxTurns)) throw new Error(t.hints.goalTurnsInteger)
      if (maxTurns < 0) throw new Error(t.hints.goalTurnsNonNegative)
      if (maxTurns > 10000) throw new Error(t.hints.goalTurnsTooLarge)
      if (llmNo !== null && !Number.isInteger(llmNo)) throw new Error(t.hints.goalLLMInteger)
      if (llmNo !== null && llmNo < 0) throw new Error(t.hints.goalLLMNonNegative)
      const body = { objective, budget_minutes: budgetMinutes, max_turns: maxTurns }
      if (llmNo !== null) body.llm_no = llmNo
      const d = await api('/api/goals/start', { method:'POST', body: JSON.stringify(body) })
      setMsg(`${t.hints.goalStarted}: ${d.goal?.id || ''}`); setGoalObjective(''); setSelectedGoal(d.goal?.id || selectedGoal); await loadGoals(); if (d.goal?.id) await loadGoalOutput(d.goal.id)
    } catch(e){ setMsg(e.message) } finally{ setBusy(false) }
  }
  const stopGoal = async (g) => { if (!g) return; const confirmText = t.hints.goalStopConfirm.replace('{id}', g.id || '-').replace('{pid}', g.pid || '-'); if (!window.confirm(confirmText)) return; setBusy(true); setMsg(''); try { await api('/api/goals/stop', { method:'POST', body: JSON.stringify({ id: g.id, pid: g.pid }) }); setMsg(`${t.hints.goalStopped}: ${g.id}`); await loadGoals(); if (selectedGoal === g.id) await loadGoalOutput(g.id) } catch(e){ setMsg(e.message) } finally{ setBusy(false) } }
  const deleteGoal = async (g) => { if (!g) return; const confirmText = t.hints.goalDeleteConfirm.replace('{id}', g.id || '-'); if (!window.confirm(confirmText)) return; setBusy(true); setMsg(''); try { await api('/api/goals/delete', { method:'POST', body: JSON.stringify({ id: g.id }) }); setMsg(`${t.hints.goalDeleted}: ${g.id}`); const gs = await loadGoals(); if (selectedGoal === g.id) { const next = pickGoalId(gs, ''); setSelectedGoal(next); setGoalOutput(''); setGoalOutputMeta({}); if (next) await loadGoalOutput(next) } } catch(e){ setMsg(e.message) } finally{ setBusy(false) } }
  const loadGoalOutput = async (id = selectedGoal) => {
    if (!id) return
    setSelectedGoal(id)
    const seq = ++goalOutputSeq.current
    try {
      const maxBytes = Number(goalOutputBytes || 0)
      if (!Number.isInteger(maxBytes)) throw new Error(t.hints.goalOutputBytesInteger)
      if (maxBytes < 0) throw new Error(t.hints.goalOutputBytesNonNegative)
      if (maxBytes > 1048576) throw new Error(t.hints.goalOutputBytesTooLarge)
      const d = await api(`/api/goals/output?id=${encodeURIComponent(id)}&max_bytes=${encodeURIComponent(maxBytes)}`)
      if (seq !== goalOutputSeq.current) return
      setGoalOutput(d.output || '')
      setGoalOutputMeta({
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
      if (seq !== goalOutputSeq.current) return
      setMsg(e.message)
      setGoalOutput(e.message)
      setGoalOutputMeta({
        error: e.message,
        bytesReturned: new Blob([e.message || '']).size,
        totalBytes: new Blob([e.message || '']).size,
        requestedBytes: Number(goalOutputBytes || 0),
        maxBytes: Number(goalOutputBytes || 0),
      })
    }
  }
  useEffect(() => {
    if (tab !== 'goals') return
    const refreshGoals = async () => {
      if (goalRefreshBusy.current) return
      goalRefreshBusy.current = true
      try {
        const gs = await loadGoals()
        const active = pickGoalId(gs, selectedGoal)
        if (active) await loadGoalOutput(active)
      } catch (e) {
        setMsg(e.message)
      } finally {
        goalRefreshBusy.current = false
      }
    }
    refreshGoals()
    if (!goalAutoRefresh) return
    const timer = setInterval(refreshGoals, 3000)
    return () => clearInterval(timer)
  }, [tab, selectedGoal, goalOutputBytes, goalAutoRefresh])
  const toggleTask = async (id, enabled) => { setBusy(true); try { await api('/api/schedule/toggle', { method:'POST', body: JSON.stringify({ id, enabled }) }); setMsg(t.hints.taskToggled); await load() } catch(e){ setMsg(e.message) } finally{ setBusy(false) } }

  const loadFiles = async (path = '') => { const d = await api(`/api/files/list?path=${encodeURIComponent(path || '')}`); setFileList(d.items || d.entries || []); setFilePath(path || '') }
  const readFile = async (path = filePath) => { setBusy(true); try { const d = await api(`/api/files/read?path=${encodeURIComponent(path)}`); setFileContent(d.content || ''); setFilePath(path); setTab('files') } catch(e){ setMsg(e.message) } finally{ setBusy(false) } }
  const tailFile = async (path = filePath) => { if (!path) return; setBusy(true); try { const d = await api(`/api/files/tail?path=${encodeURIComponent(path)}&lines=${tailLines}`); setFileContent(d.content || ''); setFilePath(path); setTab('files') } catch(e){ setMsg(e.message) } finally{ setBusy(false) } }
  const saveFile = async () => { if (!filePath) return; setBusy(true); try { const d = await api('/api/files/write', { method:'POST', body: JSON.stringify({ path:filePath, content:fileContent }) }); setFileContent(d.content || fileContent); setMsg(t.hints.fileSaved || t.saved || 'Saved'); await loadFiles(filePath.includes('/') ? filePath.split('/').slice(0,-1).join('/') : '') } catch(e){ setMsg(e.message) } finally{ setBusy(false) } }


  const refreshVersionStatus = async () => {
    const d = await api('/api/version/status')
    setVersionStatus(d)
    if (d?.check) setVersionCheck(d.check)
    return d
  }
  useEffect(() => {
    let stop = false
    const tick = async () => {
      try {
        const d = await refreshVersionStatus()
        if (!stop && d?.running) setTimeout(tick, 1500)
      } catch (_) {}
    }
    tick()
    return () => { stop = true }
  }, [])
  useEffect(() => {
    if (!versionStatus?.running) return
    const timer = setInterval(() => refreshVersionStatus().catch(e => setMsg(e.message)), 1500)
    return () => clearInterval(timer)
  }, [versionStatus?.running])
  const checkVersion = async () => {
    setVersionBusy(true)
    try { const d = await api('/api/version/check'); setVersionCheck(d); setMsg(d.update ? `发现新版本 ${d.latest?.tag_name || ''}` : '已是最新版本') }
    catch(e){ setMsg(e.message) }
    finally{ setVersionBusy(false) }
  }
  const updateVersion = async () => {
    if (!window.confirm('下载并重启 GA Admin 以完成升级？页面可刷新，进度会自动恢复。')) return
    setVersionBusy(true)
    try { const d = await api('/api/version/update', { method:'POST', body:'{}' }); setVersionStatus(d); setMsg(d.message || '升级已启动') }
    catch(e){ setMsg(e.message) }
    finally{ setVersionBusy(false) }
  }
  const runSearch = async () => { setBusy(true); try { const d = await api(`/api/files/search?path=${encodeURIComponent(filePath)}&q=${encodeURIComponent(fileSearch)}&limit=80`); setSearchHits(d.hits || []) } catch(e){ setMsg(e.message) } finally{ setBusy(false) } }

  const loadTask = async (id) => { setBusy(true); try { const d = await api(`/api/schedule/task?id=${encodeURIComponent(id)}`); setTaskId(d.id || id); setTaskEditor(safeJson(d.raw)); setTab('tasks'); setTaskSubTab('scheduled') } catch(e){ setMsg(e.message) } finally{ setBusy(false) } }
  const saveTask = async () => { setBusy(true); try { await api('/api/schedule/task', { method:'PUT', body: JSON.stringify({ id: taskId || newTaskId, raw: JSON.parse(taskEditor) }) }); setMsg(t.hints.taskSaved); await load(); setTaskSubTab('scheduled') } catch(e){ setMsg(e.message) } finally{ setBusy(false) } }
  const createTask = async () => { setTaskId(newTaskId); setTaskEditor(safeJson({ schedule: '09:00', repeat: 'daily', enabled: false, prompt: '' })); setTaskSubTab('scheduled') }
  const deleteTask = async () => { if (!taskId) return; setBusy(true); try { await api('/api/schedule/delete', { method:'POST', body: JSON.stringify({ id: taskId }) }); setMsg(t.hints.taskDeleted); setTaskId(''); setTaskEditor('{}'); await load(); setTaskSubTab('scheduled') } catch(e){ setMsg(e.message) } finally{ setBusy(false) } }
  const readScheduleArtifact = async (path, targetTab = 'tasks') => { setBusy(true); try { const d = await api(`/api/schedule/artifact?path=${encodeURIComponent(path)}`); setScheduleArtifactTitle(path); setScheduleArtifact(d.content || ''); setTab(targetTab); setTaskSubTab('reports') } catch(e){ setMsg(e.message) } finally{ setBusy(false) } }

  const importModels = async () => { setBusy(true); try { const d = await api('/api/models/import-mykey', { method:'POST', body: JSON.stringify({ reveal:false, save:false }) }); setProfiles(d.profiles?.length ? d.profiles : [emptyProfile(0)]); setModelPreview(safeJson(d)) } catch(e){ setMsg(e.message) } finally{ setBusy(false) } }
  useEffect(() => { if (tab === 'models' && profiles.length === 0) importModels() }, [tab])
  const previewModels = async () => { setBusy(true); try { const d = await api('/api/models/preview', { method:'POST', body: JSON.stringify({ profiles }) }); setModelPreview(d.python || safeJson(d)) } catch(e){ setMsg(e.message) } finally{ setBusy(false) } }
  const saveModels = async () => { setBusy(true); try { const d = await api('/api/models/export', { method:'POST', body: JSON.stringify({ profiles, overwrite_active:true }) }); setModelPreview(safeJson(d)); setMsg(t.hints.modelsSaved) } catch(e){ setMsg(e.message) } finally{ setBusy(false) } }
  const patchProfile = (idx, patch) => setProfiles(ps => ps.map((p, i) => i === idx ? { ...p, ...patch } : p))

  const nav = NAV_ITEMS

  const needsSetup = !!health && !health?.ok
  if (needsSetup) return <div className="setup-shell"><div className="setup-card"><div className="brand setup-brand"><Bot/><div><h1>{t.setupTitle}</h1><p>{t.setupDesc}</p></div></div><div className="setup-env"><button className="secondary" onClick={checkSetupEnv} disabled={busy}>{t.checkEnv}</button>{setupEnv?.tools?.map(tool => <span key={tool.name} className={tool.ok ? 'ok' : 'err'} title={[tool.path, tool.version, tool.error].filter(Boolean).join('\n')}>{tool.ok ? '✓' : '×'} {tool.name}</span>)}</div><label>{t.root}<div className="setup-path-row"><input value={root} onChange={e=>setRoot(e.target.value)} placeholder="C:\\Users\\...\\GenericAgent"/><button className="secondary" onClick={()=>browseSetupDir('root')} disabled={busy}>{t.browse}</button></div></label><button onClick={validateSetupRoot} disabled={busy || !root}>{busy ? t.busy : t.validateRoot}</button><div className="setup-divider"><span>or</span></div><label>{t.installPath}<div className="setup-path-row"><input value={installRoot} onChange={e=>setInstallRoot(e.target.value)} placeholder="C:\\Users\\...\\GenericAgent"/><button className="secondary" onClick={()=>browseSetupDir('install')} disabled={busy}>{t.browse}</button></div></label><button className="secondary" onClick={installGA} disabled={busy || !(installRoot || root)}>{t.installGA}</button>{msg && <div className="message">{msg}</div>}<p className="setup-note">git clone https://github.com/lsdefine/GenericAgent</p></div></div>

  return <div className="app">
    <aside className="sidebar"><div className="brand"><Bot/><div><h1>{t.appName}</h1><p>{t.tagline}</p></div></div><div className="lang-switch"><div className="lang-switch-label"><Globe2 size={15}/><span>{t.language}</span></div><div className="lang-options" role="group" aria-label={t.language}><button type="button" className={lang === 'zh' ? 'active' : ''} onClick={()=>setLang('zh')}>中</button><button type="button" className={lang === 'en' ? 'active' : ''} onClick={()=>setLang('en')}>EN</button></div></div><div className="root-box"><label>{t.root}</label><div><input value={root} onChange={e=>setRoot(e.target.value)}/><button onClick={saveConfig}><Save size={14}/></button></div><label>{t.fields.pythonPath}</label><div><input value={cfg?.python_path || ''} onChange={e=>setCfg({...cfg, python_path:e.target.value})} placeholder={t.fields.pythonAuto}/><button onClick={saveConfig}><Save size={14}/></button></div><label>Chat Python 代理</label><div><select value={cfg?.proxy_mode || 'off'} onChange={e=>setCfg({...cfg, proxy_mode:e.target.value})}><option value="off">关闭</option><option value="system">系统</option><option value="custom">自定义</option></select><button onClick={saveConfig}><Save size={14}/></button></div>{(cfg?.proxy_mode || 'off') === 'custom' && <><label>HTTP_PROXY</label><div><input value={cfg?.http_proxy || ''} onChange={e=>setCfg({...cfg, http_proxy:e.target.value})} placeholder="http://127.0.0.1:7890"/></div><label>HTTPS_PROXY</label><div><input value={cfg?.https_proxy || ''} onChange={e=>setCfg({...cfg, https_proxy:e.target.value})} placeholder="http://127.0.0.1:7890"/></div><label>ALL_PROXY</label><div><input value={cfg?.all_proxy || ''} onChange={e=>setCfg({...cfg, all_proxy:e.target.value})} placeholder="socks5://127.0.0.1:7890"/></div><label>NO_PROXY</label><div><input value={cfg?.no_proxy || ''} onChange={e=>setCfg({...cfg, no_proxy:e.target.value})} placeholder="localhost,127.0.0.1"/></div></>}</div><nav>{nav.map(n => <button key={n} className={tab===n?'active':''} onClick={()=> n === 'chat' ? window.open('/chat', '_blank', 'noopener,noreferrer') : setTab(n)}>{icon(n)}{t.nav[n]}{n === 'chat' && <span className="nav-pop">open</span>}</button>)}</nav><button className="refresh" onClick={load} disabled={busy}><RefreshCw size={15}/>{busy ? t.busy : t.refresh}</button>{msg && <div className="message">{msg}</div>}</aside>
    <main className="main"><header><div><h2>{t.nav[tab]}</h2><p>{t.desc[tab]}</p></div><div className="badges"><span>{cfg?.host}:{cfg?.port}</span><span className={health?.ok?'ok':'err'}>{health?.ok ? t.ready : t.error}</span></div></header>
      {tab==='overview' && <section><div className="stats"><Stat label={t.cards.processes} value={services.length} icon={<Server/>}/><Stat label={t.cards.running} value={services.filter(s=>s.running).length} icon={<Activity/>}/><Stat label={t.cards.schedule} value={schedule.task_count || 0} icon={<CalendarClock/>}/><Stat label={t.cards.enabledTasks} value={schedule.enabled || 0} icon={<CheckCircle2/>}/></div><div className="grid2"><Panel title={t.cards.version}><div className="version-card"><div className="autostart-head"><Download size={18}/><strong>GA Admin {versionInfo?.version || 'dev'}</strong><span className={versionCheck?.update ? 'err' : 'ok'}>{versionCheck ? (versionCheck.update ? 'Update' : 'Latest') : (versionInfo?.goos ? `${versionInfo.goos}/${versionInfo.goarch}` : t.empty)}</span></div><p className="muted">commit {versionInfo?.commit || 'unknown'} · {versionInfo?.date || 'unknown'}</p>{versionCheck?.latest && <p>Latest: <a href={versionCheck.latest.html_url} target="_blank" rel="noreferrer">{versionCheck.latest.tag_name}</a></p>}{versionCheck?.asset && <code>{versionCheck.asset.name}</code>}{versionStatus?.stage && <div className="update-progress"><div className="update-progress-head"><span>{versionStatus.running ? '升级中' : (versionStatus.error ? '升级失败' : '升级状态')}</span><b>{versionStatus.progress || 0}%</b></div><div className="progress-bar"><span style={{width:`${Math.max(0, Math.min(100, versionStatus.progress || 0))}%`}}/></div><p className={versionStatus.error ? 'err' : 'muted'}>{versionStatus.message || versionStatus.stage}</p>{versionStatus.stage && <code>{versionStatus.stage}</code>}</div>}<div className="actions"><button onClick={checkVersion} disabled={versionBusy || versionStatus?.running}>{versionBusy ? t.busy : '检查更新'}</button><button onClick={updateVersion} disabled={versionBusy || versionStatus?.running || !versionCheck?.update}>{versionStatus?.running ? '升级中…' : '一键升级'}</button><button className="secondary" onClick={()=>refreshVersionStatus().catch(e=>setMsg(e.message))}>刷新进度</button></div></div></Panel><Panel title="GA 源代码更新"><div className="version-card"><div className="version-head"><GitPullRequest size={18}/><strong>Git 更新</strong><span className={gitStatus?.error ? 'err' : (gitStatus?.latest ? 'ok' : 'warn')}>{gitStatus?.error ? '检查失败' : (gitStatus ? (gitStatus.latest ? '已是最新' : `落后 ${gitStatus.behind || 0} 个提交`) : '未检查')}</span></div><p className="muted">自动 fetch 后对比上游分支；更新只执行 git pull --ff-only。</p>{gitStatus?.root && <code>{gitStatus.root}</code>}<p>分支: {gitStatus?.branch || '-'}　HEAD: {gitStatus?.commit || gitResult?.after || '-'}</p>{gitStatus?.upstream && <p>上游: {gitStatus.upstream}　领先 {gitStatus.ahead || 0} / 落后 {gitStatus.behind || 0}</p>}{gitStatus?.dirty && <p className="warn">工作区有未提交修改</p>}{gitStatus?.error && <p className="err">{gitStatus.error}</p>}{gitStatus?.fetch_error && <pre className="mini-log">{gitStatus.fetch_error}</pre>}{gitResult?.pull && <pre className="mini-log">{gitResult.pull}</pre>}<div className="actions"><button className="secondary" onClick={checkGASource} disabled={gitBusy || busy}>{gitBusy ? t.busy : '检查是否最新'}</button><button onClick={updateGASource} disabled={gitBusy || busy || gitStatus?.latest}>{gitBusy ? t.busy : '更新 GA 源代码'}</button></div></div></Panel><Panel title={t.lists.autostart}><div className="autostart-card"><div className="autostart-head"><Power size={18}/><strong>{t.autostart}</strong><span className={autostart?.enabled ? 'ok' : 'muted'}>{autostart?.supported ? (autostart?.enabled ? t.enabled : t.disabled) : t.unsupported}</span></div><p>{!autostart?.supported ? t.hints.autostartUnsupported : (autostart?.enabled ? t.hints.autostartEnabled : t.hints.autostartDisabled)}</p>{autostart?.path && <code>{autostart.path}</code>}<button onClick={toggleAutostart} disabled={busy || !autostart?.supported}>{autostart?.enabled ? t.disableAutostart : t.enableAutostart}</button></div></Panel><Panel title={t.lists.riskHints}><ul className="risk"><li>{t.root}: {root}</li><li>sche_tasks JSON: {t.backup}</li><li>mykey.py: {t.backup}</li></ul></Panel></div></section>}
      {tab==='chat' && <ChatPage t={t}/>}
      {tab==='control' && <section><div className="stats"><Stat label={t.cards.health} value={health?.ok ? 'OK' : 'FAIL'} icon={<ShieldAlert/>}/><Stat label={t.cards.capabilities} value={control?.capabilities?.length || 0} icon={<Brain/>}/><Stat label={t.cards.risks} value={control?.risks?.length || 0} icon={<ShieldAlert/>}/><Stat label={t.cards.reports} value={control?.reports?.length || 0} icon={<FileCode2/>}/></div><div className="grid2"><Panel title={t.lists.readiness}><EntryList items={(control?.readiness || []).map((r,i)=>({name:r.area, path:r.text, kind:r.level}))} empty="OK"/></Panel><Panel title={t.lists.capabilities}><EntryList items={(control?.capabilities || []).map(c=>({name:c.name,path:c.path,kind:c.kind}))} empty={t.empty}/></Panel><Panel title={t.lists.recentReports}><EntryList items={control?.reports || []} empty={t.empty}/></Panel><Panel title={t.lists.riskHints}><EntryList items={(control?.risks || []).map(r=>({name:r.area,path:r.text,kind:r.level}))} empty="OK"/></Panel></div></section>}
      {tab==='files' && <section><div className="workspace"><Panel title={t.lists.fileList}><div className="inline-form"><input value={filePath} onChange={e=>setFilePath(e.target.value)} placeholder={t.hints.filePath}/><button onClick={()=>loadFiles(filePath)}>{t.read}</button></div><div className="inline-form"><input value={fileSearch} onChange={e=>setFileSearch(e.target.value)} placeholder={t.hints.searchText}/><button onClick={runSearch}><Search size={14}/>{t.search}</button></div><div className="inline-form"><input type="number" value={tailLines} onChange={e=>setTailLines(Number(e.target.value))}/><span>{t.hints.tailLines}</span><button onClick={()=>tailFile(filePath)}>{t.tail || 'Tail'}</button><button onClick={saveFile} disabled={!filePath}><Save size={14}/>{t.save}</button></div><div className="file-list">{fileList.map(e=><button key={e.path} onClick={()=> e.kind==='dir' ? loadFiles(e.path) : readFile(e.path)}>{e.kind==='dir'?'📁':'📄'} {e.path}</button>)}</div><h4>{t.lists.searchResults}</h4>{searchHits.map(h=><button className="hit" key={`${h.path}:${h.line}`} onClick={()=>readFile(h.path)}>{h.path}:{h.line} · {h.preview}</button>)}</Panel><Panel title={t.lists.filePreview} className="log-panel"><textarea className="file-editor" value={fileContent} onChange={e=>setFileContent(e.target.value)} placeholder={t.empty}/></Panel></div></section>}
      {tab==='tasks' && <section className="tasks-page">
        <div className="stats schedule-stats">
          <div className="stat"><Activity/><span>{t.lists.taskServices}</span><b>{taskSvcs.length}</b></div>
          <div className="stat"><CalendarClock/><span>{t.cards.enabledTasks || t.enabled}</span><b>{schedule.enabled || 0}</b></div>
          <div className="stat"><FolderCog/><span>{t.cards.reports || 'Reports'}</span><b>{schedule.done_count || 0}</b></div>
          <div className="stat"><ShieldAlert/><span>{t.error}</span><b>{schedule.errors || 0}</b></div>
        </div>

        <div className="subtabs task-subtabs">
          <button className={taskSubTab==='services' ? 'active' : ''} onClick={()=>setTaskSubTab('services')}><Server size={14}/>{t.lists.taskServices}</button>
          <button className={taskSubTab==='scheduled' ? 'active' : ''} onClick={()=>setTaskSubTab('scheduled')}><CalendarClock size={14}/>{t.lists.scheduledTasks}</button>
          <button className={taskSubTab==='reports' ? 'active' : ''} onClick={()=>setTaskSubTab('reports')}><FolderCog size={14}/>{t.lists.recentReports}</button>
        </div>

        {taskSubTab==='services' && <div className="single-panel">
          <Panel title={t.lists.taskServices}>
            <p className="muted">{t.desc.tasks}</p>
            <div className="service-list clean-list">
              {taskSvcs.length
                ? taskSvcs.map(svc => <ServiceRow key={svc.name} svc={svc} t={t} onStart={n=>serviceAction(n,'start')} onStop={n=>serviceAction(n,'stop')} onLogs={viewServiceLogs} onAutostart={toggleServiceAutostart}/>)
                : <p className="muted">{t.hints.noTasks}</p>}
            </div>
          </Panel>
        </div>}

        {taskSubTab==='scheduled' && <div className="workspace tasks-workspace">
          <Panel title={t.lists.scheduledTasks}>
            <div className="inline-form task-create">
              <input value={newTaskId} onChange={e=>setNewTaskId(e.target.value)} placeholder={t.hints.newTaskId}/>
              <button onClick={createTask}><FileCode2 size={14}/>{t.create}</button>
              {schedule.log?.exists && <button onClick={()=>readScheduleArtifact('sche_tasks/scheduler.log')}><Terminal size={14}/>{t.nav.logs}</button>}
            </div>
            <div className="task-list clean-list">
              {tasks.length
                ? tasks.map(task => <TaskRow key={task.id} task={task} t={t} onToggle={toggleTask} onEdit={loadTask} onArtifact={readScheduleArtifact}/>)
                : <p className="muted">{t.hints.noTasks}</p>}
            </div>
          </Panel>
          <Panel title={`${t.lists.editor} · ${taskId || t.empty}`}>
            <p className="muted">{t.hints.jsonHelp}</p>
            <textarea className="json-editor compact-editor" value={taskEditor} onChange={e=>setTaskEditor(e.target.value)}/>
            <div className="actions">
              <button onClick={saveTask} disabled={!taskId && !newTaskId}><Save size={14}/>{t.save}</button>
              <button onClick={deleteTask} disabled={!taskId}><XCircle size={14}/>{t.remove}</button>
            </div>
          </Panel>
        </div>}

        {taskSubTab==='reports' && <div className="workspace tasks-workspace">
          <Panel title={t.lists.recentReports}>
            <div className="report-list clean-list">
              {(schedule.done_recent || []).length
                ? (schedule.done_recent || []).map(r => <button key={r.path} onClick={()=>readScheduleArtifact(r.path)}>{r.name}<small>{new Date(r.mod_time).toLocaleString()}</small></button>)
                : <p className="muted">{t.empty}</p>}
            </div>
          </Panel>
          <Panel title={scheduleArtifactTitle || t.lists.generatedPreview}>
            <pre className="artifact-view">{scheduleArtifact || t.empty}</pre>
          </Panel>
        </div>}
      </section>}
      {tab==='bbs' && <BBSPage status={bbsStatus} config={bbsConfig} setConfig={setBbsConfig} onSaveConfig={saveBBSConfig} posts={bbsPosts} title={bbsTitle} setTitle={setBbsTitle} content={bbsContent} setContent={setBbsContent} author={bbsAuthor} setAuthor={setBbsAuthor} onCreate={createBBSPost} onReply={createBBSReply} onRefresh={loadBBS} onTestConnection={testBBSConnection} workerService={bbsWorkerSvc} onWorkerStart={n=>serviceAction(n,'start')} onWorkerStop={n=>serviceAction(n,'stop')} onWorkerLogs={viewServiceLogs} onWorkerAutostart={toggleServiceAutostart} busy={busy}/>}
      {tab==='memory' && <section><div className="grid2"><Panel title={t.lists.memory}><EntryList items={[inv.memory?.insight, inv.memory?.facts].filter(Boolean)} empty={t.empty}/></Panel><Panel title={t.lists.sop}><EntryList items={[...(inv.memory?.sops||[]), ...(inv.memory?.utils||[])]} empty={t.empty}/></Panel></div></section>}
      {tab==='channels' && <section className="channels-page"><div className="stats"><Stat label={t.lists.frontendServices} value={frontendSvcs.length} icon={<Server/>}/><Stat label={t.running} value={frontendSvcs.filter(s=>s.running).length} icon={<CheckCircle2/>}/><Stat label={t.stopped} value={frontendSvcs.filter(s=>!s.running).length} icon={<XCircle/>}/></div><Panel title={t.lists.frontendServices} className="channels-panel"><p className="muted">{t.desc.channels}</p><ChannelServiceTable services={frontendSvcs} t={t} onStart={n=>serviceAction(n,'start')} onStop={n=>serviceAction(n,'stop')} onLogs={viewServiceLogs} onAutostart={toggleServiceAutostart}/></Panel></section>}
      {tab==='autonomous' && <section><div className="grid2"><Panel title={t.lists.reflectServices}>{reflectSvcs.length ? reflectSvcs.map(s=><ServiceRow key={s.name} svc={s} t={t} onStart={n=>serviceAction(n,'start')} onStop={n=>serviceAction(n,'stop')} onLogs={viewServiceLogs} onAutostart={toggleServiceAutostart}/>) : <p className="muted">{t.hints.noReflect}</p>}</Panel><Panel title={t.lists.reflectScripts}><EntryList items={inv.reflect || []} empty={t.hints.noReflect}/></Panel></div><Panel title={t.lists.recentReports}><div className="report-list">{(inv.autonomous_reports || []).map(r=><button key={r.path} onClick={()=>readScheduleArtifact(r.path, 'autonomous')}>{r.name}<small>{new Date(r.mod_time).toLocaleString()}</small></button>)}</div><pre className="artifact-view">{scheduleArtifactTitle?.includes('autonomous_reports') ? (scheduleArtifact || t.empty) : t.empty}</pre></Panel></section>}
      {tab==='goals' && <GoalsPage t={t} goals={goals} objective={goalObjective} setObjective={setGoalObjective} budget={goalBudget} setBudget={setGoalBudget} maxTurns={goalMaxTurns} setMaxTurns={setGoalMaxTurns} llmNo={goalLLMNo} setLLMNo={setGoalLLMNo} outputBytes={goalOutputBytes} setOutputBytes={setGoalOutputBytes} autoRefresh={goalAutoRefresh} setAutoRefresh={setGoalAutoRefresh} selected={selectedGoal} output={goalOutput} outputMeta={goalOutputMeta} busy={busy} onStart={startGoal} onStop={stopGoal} onDelete={deleteGoal} onRefresh={loadGoals} onOutput={loadGoalOutput} onClearOutput={()=>{ goalOutputSeq.current += 1; setGoalOutput(''); setGoalOutputMeta(null); setMsg(t.hints.goalOutputCleared) }} setMsg={setMsg}/>}
      {tab==='models' && <Models t={t} profiles={profiles} setProfiles={setProfiles} patchProfile={patchProfile} importModels={importModels} previewModels={previewModels} saveModels={saveModels} modelPreview={modelPreview}/>} 
      {tab==='logs' && <section className="logs-page"><div className="logs-layout"><Panel title={t.lists.processes} className="logs-side"><div className="logs-toolbar"><label>{t.hints.tailLines}<input type="number" min="20" max="2000" value={tailLines} onChange={e=>setTailLines(Number(e.target.value) || 200)}/></label><button disabled={!selected} onClick={()=>loadServiceLogs(selected)}><RefreshCw size={14}/>{t.refresh}</button></div><div className="logs-service-list">{services.map(s => <button className={selected===s.name?'log-service active':'log-service'} key={s.name} onClick={()=>loadServiceLogs(s.name)}><span className={s.running?'dot running':'dot'}></span><span className="log-service-name">{s.name}</span><small>{s.kind}{s.pid ? ` · PID ${s.pid}` : ''}</small></button>)}</div></Panel><Panel title={`Logs · ${selected || '-'}`} className="log-panel"><div className="log-head"><div>{selected && <p className="muted log-command" title={services.find(s=>s.name===selected)?.command?.join(' ')}>{services.find(s=>s.name===selected)?.command?.join(' ')}</p>}<span className="log-count">{logs.length} lines · UTF-8</span></div><div className="actions"><button disabled={!selected || services.find(s=>s.name===selected)?.running} onClick={()=>serviceAction(selected,'start')}><Play size={14}/>{t.start}</button><button disabled={!selected || !services.find(s=>s.name===selected)?.running} onClick={()=>serviceAction(selected,'stop')}><Square size={14}/>{t.stop}</button></div></div><pre className="log-view">{logs.join('\n') || t.hints.noLogs}</pre></Panel></div></section>}
    </main>
  </div>
}


function BBSPage({ status, config, setConfig, onSaveConfig, posts = [], title, setTitle, content, setContent, author, setAuthor, onCreate, onReply, onRefresh, onTestConnection, workerService, onWorkerStart, onWorkerStop, onWorkerLogs, onWorkerAutostart, busy }) {
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

function TurnBubble({ message }) {
  const m = message || {}
  return <div className={`bubble ${m.role || 'assistant'} ${m.type || ''} ${m.error?'error':''}`}>
    <div className="role">{m.title || m.role || 'assistant'}</div>
    <div className="content">{m.content || ''}</div>
  </div>
}

function TurnList({ messages, empty, className = '' }) {
  const items = messages || []
  return <div className={`chat-messages turn-list ${className}`}>
    {items.length===0 && <div className="empty-chat">{empty}</div>}
    {items.map((m, i) => <TurnBubble key={m.id || i} message={m} />)}
  </div>
}

function ChatPage({ t }) {
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


const copyText = async (text) => {
  const value = text || ''
  if (!value) return
  if (navigator.clipboard?.writeText) {
    await navigator.clipboard.writeText(value)
    return
  }
  const el = document.createElement('textarea')
  el.value = value
  el.setAttribute('readonly', '')
  el.style.position = 'fixed'
  el.style.left = '-9999px'
  document.body.appendChild(el)
  el.select()
  document.execCommand('copy')
  document.body.removeChild(el)
}

const clampPercent = (value) => Math.max(0, Math.min(100, Number.isFinite(value) ? value : 0))
const goalTurnPercent = (g) => {
  const serverPct = Number(g?.turn_percent)
  if (Number.isFinite(serverPct)) return clampPercent(serverPct)
  const maxTurns = Number(g?.max_turns || 0)
  if (!maxTurns) return 0
  return clampPercent((Number(g?.turns_used || 0) / maxTurns) * 100)
}
const goalBudgetPercent = (g) => {
  const serverPct = Number(g?.budget_percent)
  if (Number.isFinite(serverPct)) return clampPercent(serverPct)
  const elapsed = Number(g?.elapsed_seconds || 0)
  const remaining = Number(g?.remaining_seconds || 0)
  const total = elapsed + Math.max(0, remaining)
  if (!total) return 0
  return clampPercent((elapsed / total) * 100)
}

function GoalsPage({ t, goals, objective, setObjective, budget, setBudget, maxTurns, setMaxTurns, llmNo, setLLMNo, outputBytes, setOutputBytes, autoRefresh, setAutoRefresh, selected, output, outputMeta, busy, onStart, onStop, onDelete, onRefresh, onOutput, onClearOutput, setMsg }) {
  const goalList = goals || []
  const running = goalList.filter(g => g.running).length
  const selectedGoal = goalList.find(g => g.id === selected) || outputMeta?.goal || null
  const [goalTab, setGoalTab] = useState('runs')
  const outputBadges = []
  if (outputMeta?.error) outputBadges.push(`${t.error}: ${outputMeta.error}`)
  if (outputMeta?.truncated) outputBadges.push(`${t.hints.goalOutputTruncated}: ${formatBytes(outputMeta.bytesReturned)}/${formatBytes(outputMeta.totalBytes)}`)
  if (outputMeta?.maxBytesCapped) outputBadges.push(`${t.hints.goalOutputCapped}: ${formatBytes(outputMeta.requestedBytes)} -> ${formatBytes(outputMeta.maxBytes)}`)
  if (outputMeta?.defaultBytesUsed) outputBadges.push(`${t.hints.goalOutputDefault}: ${formatBytes(outputMeta.defaultBytes || outputMeta.maxBytes)}`)
  if (outputMeta?.outputStatus) outputBadges.push(`${t.fields.outputStatus}: ${t.goalOutputStatus?.[outputMeta.outputStatus] || outputMeta.outputStatus}`)
  if (outputMeta?.goal?.missing_log) outputBadges.push(t.hints.goalOutputLogMissing)

  const outputBytesShown = outputMeta?.bytesReturned ?? new Blob([output || '']).size
  const outputTotalBytes = outputMeta?.totalBytes ?? outputBytesShown
  const outputLinesShown = outputMeta?.linesReturned ?? outputLineCount(output)
  const outputTotalLines = outputMeta?.totalLines ?? outputLinesShown
  const outputLimitLabel = Number(outputBytes || 0) > 0 ? formatBytes(outputBytes) : t.fields.outputDefault
  const selectedTurnPct = selectedGoal ? goalTurnPercent(selectedGoal) : 0
  const selectedBudgetPct = selectedGoal ? goalBudgetPercent(selectedGoal) : 0

  const copyOutput = async () => {
    try { await copyText(output || ''); setMsg(t.hints.goalOutputCopied) }
    catch (e) { setMsg(e.message) }
  }
  const openOutput = (id) => { onOutput(id); setGoalTab('output') }
  const showStatePath = (path) => { if (path) setMsg(`${t.fields.stateFile}: ${path}`) }

  return <section className="goals-page">
    <div className="stats schedule-stats goal-stats">
      <div className="stat"><Target/><span>{t.nav.goals}</span><b>{goalList.length}</b></div>
      <div className="stat"><Activity/><span>{t.running}</span><b>{running}</b></div>
      <div className="stat"><Terminal/><span>reflect/goal_mode.py</span><b>{running ? t.running : t.ready}</b></div>
    </div>

    <div className="goal-tabs" role="tablist" aria-label={t.nav.goals}>
      <button role="tab" aria-selected={goalTab==='runs'} className={goalTab==='runs' ? 'active' : ''} onClick={()=>setGoalTab('runs')}>{t.fields.goalRuns}<span>{goalList.length}</span></button>
      <button role="tab" aria-selected={goalTab==='start'} className={goalTab==='start' ? 'active' : ''} onClick={()=>setGoalTab('start')}>{t.fields.startGoalMode}</button>
      <button role="tab" aria-selected={goalTab==='output'} className={goalTab==='output' ? 'active' : ''} onClick={()=>setGoalTab('output')}>{t.fields.outputTail}<span>{selected || '-'}</span></button>
    </div>

    {goalTab==='start' && <Panel title={t.fields.startGoalMode} className="goal-start-panel goal-tab-panel">
      <p className="muted">{t.desc.goals}</p>
      <label className="goal-field">{t.fields.objective}
        <textarea className="goal-objective" value={objective} maxLength={16384} onChange={e=>setObjective(e.target.value)} placeholder={t.fields.goalPlaceholder}/>
      </label>
      <div className="form-grid compact-form goal-params">
        <label>{t.fields.budgetMinutes}<input type="number" min="1" max="43200" value={budget} onChange={e=>setBudget(e.target.value)}/></label>
        <label>{t.fields.maxTurns}<input type="number" min="0" max="10000" value={maxTurns} onChange={e=>setMaxTurns(e.target.value)}/></label>
        <label>{t.fields.llmNo}<input type="number" min="0" value={llmNo} onChange={e=>setLLMNo(e.target.value)} placeholder="0"/></label>
      </div>
      <div className="actions goal-start-actions">
        <button className="primary" disabled={busy || !objective.trim()} onClick={onStart}><Play size={14}/>{t.start}</button>
        <button disabled={busy} onClick={onRefresh}><RefreshCw size={14}/>{t.refresh}</button>
      </div>
    </Panel>}

    {goalTab==='runs' && <Panel title={t.fields.goalRuns} className="goals-list-panel goal-tab-panel">
      <div className="goal-list clean-list goal-list-tabbed">
        {goalList.length ? goalList.map(g => <GoalRunCard key={g.id} g={g} t={t} selected={selected} onOutput={openOutput} onState={showStatePath} onStop={onStop} onDelete={onDelete}/>) : <p className="muted">{t.empty}</p>}
      </div>
    </Panel>}

    {goalTab==='output' && <Panel title={`${t.fields.outputTail} · ${selected || '-'}`} className="log-panel goal-output-panel goal-tab-panel">
      {selectedGoal ? <div className="goal-focus-card">
        <div className="goal-focus-main">
          <div className="goal-summary-head"><b>{selectedGoal.id}</b><span className={selectedGoal.running ? 'ok' : ''}>{selectedGoal.status || (selectedGoal.running ? t.running : t.fields.notRunning)}</span></div>
          <p>{selectedGoal.objective || t.empty}</p>
          <div className="goal-progress summary-progress"><span title={`${t.fields.turn} ${Math.round(selectedTurnPct)}%`}><i style={{width: `${selectedTurnPct}%`}} /></span><span title={`${t.fields.elapsed} ${Math.round(selectedBudgetPct)}%`}><i style={{width: `${selectedBudgetPct}%`}} /></span></div>
        </div>
        <div className="goal-summary-grid goal-focus-grid">
          <span>{t.fields.turn}: {selectedGoal.turns_used || 0}/{selectedGoal.max_turns || '-'}</span>
          <span>{t.fields.elapsed}: {formatDuration(selectedGoal.elapsed_seconds)}</span>
          <span>{t.fields.remaining}: {formatDuration(selectedGoal.remaining_seconds)}</span>
          <span>{t.fields.updated}: {formatGoalTime(selectedGoal.mod_time)}</span>
        </div>
        <div className="goal-summary-files"><button disabled={!selectedGoal.state_file} onClick={()=>showStatePath(selectedGoal.state_file)}>{t.fields.stateFile}</button><button disabled={!selectedGoal.id || selectedGoal.missing_log} onClick={()=>openOutput(selectedGoal.id)}>{t.fields.logFile}</button></div>
      </div> : <div className="goal-focus-empty">{t.empty}</div>}

      <details className="goal-controls-details">
        <summary>{t.fields.maxBytes} / {t.fields.outputLimit} · {outputLimitLabel}</summary>
        <div className="goal-toolbar">
          <label className="inline-field">{t.fields.maxBytes}
            <input type="number" min="0" max="1048576" step="4096" value={outputBytes} onChange={e=>setOutputBytes(e.target.value)}/>
          </label>
          <div className="goal-output-presets">
            <button type="button" onClick={()=>setOutputBytes('65536')}>{t.fields.outputPreset64k}</button>
            <button type="button" onClick={()=>setOutputBytes('262144')}>{t.fields.outputPreset256k}</button>
            <button type="button" onClick={()=>setOutputBytes('1048576')}>{t.fields.outputPreset1m}</button>
            <button type="button" onClick={()=>setOutputBytes('0')}>{t.fields.outputDefault}</button>
          </div>
          <label className="toggle-inline"><input type="checkbox" checked={!!autoRefresh} onChange={e=>setAutoRefresh(e.target.checked)} />{t.fields.autoRefresh}</label>
          <button disabled={!selected} onClick={()=>onOutput(selected)}><RefreshCw size={14}/>{t.refresh}</button>
          <button disabled={!output} onClick={copyOutput}><Copy size={14}/>{t.copy}</button>
          <button disabled={!output && !outputMeta} onClick={onClearOutput}><XCircle size={14}/>{t.clear}</button>
        </div>
      </details>

      <div className="goal-output-stats compact">
        <span>{t.fields.outputShown}: {formatBytes(outputBytesShown)} / {formatBytes(outputTotalBytes)}</span>
        <span>{t.fields.outputLines}: {outputLinesShown}{outputTotalLines !== outputLinesShown ? ` / ${outputTotalLines}` : ''}</span>
      </div>
      {outputBadges.length > 0 && <div className="goal-output-meta">{outputBadges.map(m => <span key={m}>{m}</span>)}</div>}
      <GoalChatView output={output} empty={t.empty} />
    </Panel>}
  </section>
}

function parseGoalOutput(output) {
  const text = output || ''
  const lines = text.split(/\r?\n/)
  const items = []
  let current = null
  const push = () => {
    if (!current) return
    current.content = current.content.join('\n').trimEnd()
    if (current.content || current.title) items.push(current)
    current = null
  }
  const classify = (raw) => {
    const line = raw.trim()
    if (!line) return null
    let m = line.match(/^<summary>(.*?)<\/summary>\s*(.*)$/i)
    if (m) return { type:'summary', role:'assistant', title:'summary', content:[m[1] || '', m[2] || ''].filter(Boolean).join('\n') }
    m = line.match(/^\[Agent\]\s*(.*)$/i)
    if (m) return { type:'agent', role:'assistant', title:'Agent', content:m[1] }
    m = line.match(/^\[(USER|ASSISTANT|SYSTEM|TOOL|FUNCTION|DEVELOPER)\]\s*:?\s*(.*)$/i)
    if (m) return { type:m[1].toLowerCase(), role:m[1].toLowerCase()==='user'?'user':'assistant', title:m[1].toLowerCase(), content:m[2] }
    m = line.match(/^(\d{4}-\d{2}-\d{2}[ T]\d{2}:\d{2}:\d{2}(?:[,\.]\d+)?)(?:\s+|\s*[-|]\s*)(.*)$/)
    if (m) return { type:'log', role:'system', title:m[1], content:m[2] }
    if (/^(ERROR|ERR|FAIL|FAILED|EXCEPTION|TRACEBACK)\b/i.test(line)) return { type:'error', role:'system', title:'error', content:raw }
    if (/^(WARN|WARNING)\b/i.test(line)) return { type:'warn', role:'system', title:'warning', content:raw }
    if (/^(INFO|DEBUG|STATUS|STEP|TURN|GOAL|PLAN|ACTION|OBSERVATION|RESULT)\b[:：\]]?/i.test(line)) return { type:'event', role:'system', title:line.split(/[:：\]]/)[0].slice(0,24), content:raw }
    return null
  }
  for (const raw of lines) {
    const hit = classify(raw)
    if (hit) {
      push()
      current = { role:hit.role, type:hit.type, title:hit.title, content:[hit.content || ''] }
    } else if (current) {
      current.content.push(raw)
    } else if (raw.trim()) {
      current = { role:'system', type:'raw', title:'log', content:[raw] }
    }
  }
  push()
  return items.map((m, i) => ({ id:`goal-${i}`, ...m }))
}

function GoalChatView({ output, empty }) {
  const items = useMemo(() => parseGoalOutput(output), [output])
  const endRef = useRef(null)
  useEffect(() => { endRef.current?.scrollIntoView({ block:'end' }) }, [items.length, output])
  if (!output) return <div className="goal-chat-empty">{empty}</div>
  return <div className="goal-chat-wrap">
    <div className="goal-chat-stream" aria-live="polite">
      <TurnList messages={items} empty={empty} className="goal-turn-list"/>
      <div ref={endRef} />
    </div>
    <details className="goal-raw-details">
      <summary>Raw log</summary>
      <pre className="log-view goal-output">{output}</pre>
    </details>
  </div>
}

function GoalRunCard({ g, t, selected, onOutput, onState, onStop, onDelete }) {
  const turnPct = goalTurnPercent(g)
  const budgetPct = goalBudgetPercent(g)
  return <div className={`goal-row ${g.running ? 'running' : ''} ${selected===g.id ? 'selected' : ''}`}>
    <button className="goal-row-main" onClick={()=>onOutput(g.id)}>
      <div className="goal-row-title"><b>{g.id}</b><span className={g.running ? 'ok' : ''}>{g.status || '-'}</span></div>
      <div className="goal-row-meta">
        <span>{g.running ? `${t.fields.pid} ${g.pid}` : t.fields.notRunning}</span>
        <span>{t.fields.turn} {g.turns_used || 0}/{g.max_turns || '-'}</span>
        <span>{t.fields.elapsed} {formatDuration(g.elapsed_seconds)}</span>
        <span>{t.fields.remaining} {formatDuration(g.remaining_seconds)}</span>
        {g.python_path ? <span>Python {g.python_path}</span> : null}
      </div>
      <div className="goal-progress"><span title={`${t.fields.turn} ${Math.round(turnPct)}%`}><i style={{width: `${turnPct}%`}} /></span><span title={`${t.fields.elapsed} ${Math.round(budgetPct)}%`}><i style={{width: `${budgetPct}%`}} /></span></div>
      <p>{g.objective || t.empty}</p>
      <small>{t.fields.started} {formatGoalTime(g.start_time ? g.start_time * 1000 : 0)} · {t.fields.updated} {formatGoalTime(g.mod_time)}{g.end_time ? ` · ${t.fields.ended} ${formatGoalTime(g.end_time * 1000)}` : ''}</small>
      <em><span className={g.missing_log ? 'err-text' : 'ok'}>{g.missing_log ? t.fields.logMissing : t.fields.logReady}</span></em>
    </button>
    <div className="actions goal-row-actions">
      <button onClick={()=>onOutput(g.id)}><Eye size={14}/>{t.read}</button>
      <button disabled={!g.state_file} onClick={()=>onState(g.state_file)}>{t.fields.stateFile}</button>
      <button disabled={!g.id || g.missing_log} onClick={()=>onOutput(g.id)}>{t.fields.logFile}</button>
      <button disabled={!g.running || !g.pid} onClick={()=>onStop(g)}><Square size={14}/>{t.stop}</button>
      <button className="danger" disabled={!!g.running} title={g.running ? t.hints.goalDeleteRunning : t.hints.goalDeleteConfirm.replace('{id}', g.id || '-')} onClick={()=>onDelete?.(g)}><Trash2 size={14}/>{t.delete}</button>
    </div>
  </div>
}

function TaskRow({ task, t, onToggle, onEdit, onArtifact }) { return <div className={`task-row status-${(task.status||'').toLowerCase()}`}><div><b>{task.id}</b><span>{task.schedule} · {task.repeat} · {task.status}</span>{task.error && <em className="err-text">{task.error}</em>}{task.next_hint && <em>{task.next_hint}</em>}<p>{task.prompt}</p>{task.recent_reports?.length > 0 && <div className="mini-reports">{task.recent_reports.map(r=><button key={r.path} onClick={()=>onArtifact(r.path)}>{r.name}</button>)}</div>}</div><div className="actions"><button onClick={()=>onEdit(task.id)}><Eye size={14}/>{t.read}</button><button onClick={()=>onToggle(task.id, !task.enabled)}>{task.enabled ? t.disabled : t.enabled}</button></div></div> }
function Models({ t, profiles, setProfiles, patchProfile, importModels, previewModels, saveModels, modelPreview }) { return <section><div className="model-top"><div><h3>{t.nav.models}</h3><p>{t.hints.previewHelp}</p></div><div className="actions"><button onClick={importModels}><RefreshCw size={14}/>{t.hints.modelSource}</button><button onClick={() => setProfiles([...profiles, emptyProfile(profiles.length)])}>{t.hints.addProfile}</button><button onClick={previewModels}><Eye size={14}/>{t.hints.preview}</button><button onClick={saveModels}><UploadCloud size={14}/>{t.hints.writeMykey}</button></div></div><div className="models-layout"><div className="profiles">{profiles.map((p, idx) => <div className="profile" key={idx}><div className="profile-head"><b>#{idx + 1} {p.name || p.var_name}</b><label><input type="checkbox" checked={!!p.enabled} onChange={(e) => patchProfile(idx, { enabled: e.target.checked })}/> enabled</label></div><div className="form-grid"><label>{t.fields.varName}<input value={p.var_name || ''} onChange={(e) => patchProfile(idx, { var_name: e.target.value })}/></label><label>{t.fields.type}<input value={p.type || ''} onChange={(e) => patchProfile(idx, { type: e.target.value })}/></label><label>{t.fields.name}<input value={p.name || ''} onChange={(e) => patchProfile(idx, { name: e.target.value })}/></label><label>{t.fields.model}<input value={p.model || ''} onChange={(e) => patchProfile(idx, { model: e.target.value })}/></label><label className="span2">{t.fields.apiBase}<input value={p.apibase || ''} onChange={(e) => patchProfile(idx, { apibase: e.target.value })}/></label><label className="span2">{t.fields.apiKey}<SecretInput value={p.apikey} onChange={(v) => patchProfile(idx, { apikey: v })} t={t}/></label><label>{t.fields.stream}<select value={String(!!p.stream)} onChange={(e) => patchProfile(idx, { stream: e.target.value === 'true' })}><option value="true">true</option><option value="false">false</option></select></label><label>{t.fields.maxRetries}<input type="number" value={p.max_retries ?? 3} onChange={(e) => patchProfile(idx, { max_retries: Number(e.target.value) })}/></label><label>{t.fields.readTimeout}<input type="number" value={p.read_timeout ?? 300} onChange={(e) => patchProfile(idx, { read_timeout: Number(e.target.value) })}/></label><label>{t.fields.reasoningEffort}<input value={p.reasoning_effort || ''} onChange={(e) => patchProfile(idx, { reasoning_effort: e.target.value })}/></label></div></div>)}</div><Panel title={t.lists.generatedPreview} className="preview"><pre>{modelPreview || t.empty}</pre></Panel></div></section> }
function icon(n) { const m = { overview:<Activity size={16}/>, chat:<MessageSquare size={16}/>, control:<ShieldAlert size={16}/>, files:<FileCode2 size={16}/>, tasks:<Terminal size={16}/>, bbs:<Users size={16}/>, memory:<Brain size={16}/>, channels:<Globe2 size={16}/>, autonomous:<Bot size={16}/>, schedule:<CalendarClock size={16}/>, goals:<Target size={16}/>, models:<SlidersHorizontal size={16}/>, logs:<FolderCog size={16}/> }; return m[n] }
