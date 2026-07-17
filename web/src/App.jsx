import React, { Suspense, lazy, useCallback, useEffect, useMemo, useRef, useState } from 'react'
import gsap from 'gsap'
import { useGSAP } from '@gsap/react'
import { Activity, Bot, Brain, CalendarClock, CheckCircle2, ChevronDown, Code2, Copy, Eye, FileCode2, FolderCog, Globe2, GitPullRequest, MessageSquare, Play, RefreshCw, Save, Server, ShieldAlert, Power, SlidersHorizontal, Square, Target, Terminal, Trash2, UploadCloud, X, XCircle, Download, Moon, Sun } from 'lucide-react'
import { api } from './lib/api'
import { buildObservabilitySnapshot, observabilityRequest } from './lib/observability'
import { confirmDanger } from './lib/danger'
import { clampTailLines, dirnameForPath, fileEditorDirty } from './lib/filesSafety'
import { configDraftDirty } from './lib/configDraft'
import { gitSyncPresentation } from './lib/gitSync'
import { DEFAULT_SCHEDULE_TASK, buildScheduleCreateRequest, normalizeScheduleTasksPayload } from './lib/schedule'
import { modelValidationSummary, validateModelProfiles } from './lib/modelsValidation'
import { applyModelOrder, mergePersistedModelOrder } from './lib/modelsEditor'
import { NAV_ITEMS, TASK_SUB_TABS, parseRoute, buildRoute } from './lib/routing'
import { emptyProfile, formatBytes, formatDuration, formatGoalTime, group, modelLabel, outputLineCount, safeJson } from './lib/format'
import { updateStatusPresentation } from './lib/ux'
import { ChannelServiceTable, EntryList, ObservabilityCard, Panel, SecretInput, ServiceRow, Stat } from './components/common'
import { TurnList } from './components/turns'
import { TaskRow } from './components/schedule'
import { ErrorBoundary, GlobalFeedback, RouteFallback } from './components/feedback'
import { ProcessGuard } from './components/ProcessGuard'
import SetupWizard from './components/SetupWizard.jsx'
import { SettingsPage } from './pages/SettingsPage.jsx'
// 页面级代码分割：各 tab 页面按需懒加载，首屏只下载概览/日志所需代码。
const ChatPage = lazy(() => import('./pages/ChatPage').then(m => ({ default: m.ChatPage })))
const GoalsPage = lazy(() => import('./pages/GoalsPage').then(m => ({ default: m.GoalsPage })))
const Models = lazy(() => import('./pages/ModelsPage').then(m => ({ default: m.Models })))
const FilesPage = lazy(() => import('./pages/FilesPage').then(m => ({ default: m.FilesPage })))

gsap.registerPlugin(useGSAP)

const prefersReducedMotion = () => typeof window !== 'undefined' && window.matchMedia?.('(prefers-reduced-motion: reduce)').matches

const I18N = {
  zh: {
    appName: 'GA Admin', tagline: 'GenericAgent 生命周期控制面', root: 'GenericAgent 根目录', setupTitle: '首次配置 GenericAgent', setupDesc: '请选择已有 GA 根目录，或一键安装到新目录。', validateRoot: '验证并使用', installGA: '安装 GA', installPath: '安装目录', setupOk: 'GA 路径已配置', installDone: 'GA 已安装并配置', browse: '选择目录', checkEnv: '检查 Python / Git', envReady: '环境已就绪', envMissing: '环境缺失', save: '保存', refresh: '刷新', busy: '执行中', ready: '就绪', error: '错误', empty: '暂无', enabled: '启用', disabled: '停用', start: '启动', stop: '停止', running: '运行中', stopped: '已停止', language: '语言', copy: '复制', clear: '清空', delete: '删除', show: '显示', hide: '隐藏', search: '搜索', read: '读取', create: '创建', remove: '删除', backup: '写操作会自动备份', autostart: '开机自启', enableAutostart: '开启自启', disableAutostart: '关闭自启', unsupported: '不支持',
    serviceDesc: { scheduler: '定时任务调度器：每 120 秒扫描 sche_tasks/ 中的任务，按 once/daily/weekly/every_Nh 等周期到期触发，并归档 L4 会话记录。', autonomous: '自主待机驱动：每 30 分钟检测一次，当用户离开超过 30 分钟，便提示智能体按自动化 SOP 自行推进任务。' },
    nav: { overview: '总览', chat: '对话', control: '控制面', files: '文件', tasks: '任务', memory: '记忆', channels: '通道', autonomous: '自主进化', schedule: '定时', goals: 'Goal 模式', models: '模型', settings: '配置', logs: '日志' },
    desc: { overview: '从 GA 的功能域理解并接管生命周期。', chat: '迁移自 reactapp 的 GA 原生对话、文件上传和流式聊天界面。', control: '运行前检查、能力地图、风险摘要与最近报告。', files: '安全浏览 GA 根目录内文本文件，支持 tail 与搜索。', tasks: '普通会话、任务文件、批处理入口、任务型服务与 sche_tasks 定时任务。', memory: '分层记忆、SOP 与工具能力索引。', channels: '桌面、TUI、Web、IM Bot 等前端入口。', autonomous: '反思、自主运行、Goal Mode 与团队 Worker。', schedule: 'sche_tasks JSON 定时任务详情、编辑、创建与删除。', goals: '复用 GA Goal Mode SOP 与 reflect/goal_mode.py 的持续目标控制台。', models: '按服务商读取、预览和保存 GA mykey.py 中的模型配置。', settings: '配置 GA 根目录、Python、聊天数据目录与 Chat Python 代理。', logs: '进程状态与输出日志。' },
    cards: { processes: '进程', running: '运行中', stopped: '已停止', memoryLayers: '记忆层', sopTools: 'SOP/工具', schedule: '定时任务', enabledTasks: '已启用', reports: '报告', coreFiles: '核心文件', reflect: '反思脚本', health: 'GA 健康', capabilities: '能力', risks: '风险', version: '版本管理' },
    lists: { serviceGroups: '服务域', coreFiles: '核心文件', reflect: 'Reflect / Autonomous', frontends: '前端 / 通道', memory: '记忆层级', sop: 'SOP 与工具', taskServices: '任务服务', frontendServices: '前端服务', reflectServices: '反思服务', reflectScripts: '反思脚本', scheduledTasks: '定时任务', recentReports: '最近报告', processes: '进程', generatedPreview: '生成预览', riskHints: '接管提示', autostart: '开机自启', capabilities: '能力地图', readiness: '运行前检查', fileList: '文件列表', filePreview: '文件预览', searchResults: '搜索结果', editor: '编辑器' },
    hints: { rootSaved: 'GA 根目录已保存', fileSaved: '文件已保存并备份旧文件', taskSaved: '任务已保存并备份旧文件', taskDeleted: '任务已删除并备份', taskToggled: '任务状态已更新', modelsSaved: 'mykey.py 已备份并保存', savedSecret: '已保存；输入新值可替换', secret: 'API Key / Token', noFrontend: '未发现前端服务', noReflect: '未发现 reflect 服务', noTasks: '暂无 sche_tasks/*.json', noLogs: '暂无日志', previewHelp: '点击“预览”查看配置；点击“保存”会先备份再更新 GA 的 mykey.py。', modelSource: '来源', secretHidden: '已隐藏真实密钥', addProfile: '新增 Profile', preview: '预览', writeMykey: '保存 mykey.py', filePath: '相对路径', searchText: '搜索文本', tailLines: '尾部行数', newTaskId: 'new_task', jsonHelp: 'JSON 需为对象；保存/删除会生成 .bak 时间戳。', autostartEnabled: '已开启：用户登录后自动启动 GA Admin。', autostartDisabled: '未开启：需要手动启动 GA Admin。', autostartUnsupported: '当前平台暂不支持自动注册。', autostartChanged: '开机自启状态已更新', goalObjectiveRequired: '目标不能为空', goalObjectiveTooLarge: '目标超过 16384 字节', goalBudgetInteger: '预算分钟必须是整数', goalBudgetPositive: '预算分钟必须大于 0', goalBudgetTooLarge: '预算分钟不能超过 43200', goalTurnsInteger: '最大轮次必须是整数', goalTurnsNonNegative: '最大轮次不能为负数', goalTurnsTooLarge: '最大轮次不能超过 10000', goalLLMInteger: 'LLM # 必须是整数', goalLLMNonNegative: 'LLM # 不能为负数', goalPythonHelp: 'Python 留空时自动选择：GA 根目录 .venv、venv、uv 缓存解释器、PATH python/python3；填写后按该路径启动并记录到 Goal 状态。', goalHiveHelp: 'Hive 模式会按 GA 官方逻辑启动 Goal Master、临时 BBS 与一个 worker；GA Admin 只做上层管理。', goalStarted: 'Goal 已启动', goalStopped: 'Goal 已停止', goalDeleted: 'Goal 已删除', goalDeleteConfirm: '确定删除 Goal {id}？会删除状态和日志文件；运行中的目标请先停止。', goalDeleteRunning: '运行中的 Goal 不能删除，请先停止。', goalStopConfirm: '确认停止 Goal {id}？将按可用控制级别停止。', goalStopExactConfirm: '确认停止 Admin Goal {id}？将仅终止该 Goal 记录的精确 PID {pid}。', goalStopSoftConfirm: '确认软停止外部 Goal {id}？不会杀进程，只写入状态文件 stopped_by_admin，让 Goal 循环自行退出。', goalOutputTruncated: '仅显示输出尾部，前面内容已截断', goalOutputCapped: '请求字节数超过后端上限，已按上限读取', goalOutputDefault: '未指定读取字节数，已使用默认值', goalOutputBytesInteger: '输出字节数必须是整数', goalOutputBytesNonNegative: '输出字节数不能为负数', goalOutputBytesTooLarge: '输出字节数不能超过 1048576', goalOutputCopied: '输出已复制', goalOutputCleared: '输出已清空', goalOutputLogMissing: '日志文件尚未创建，当前无可读取输出' },
    goalOutputStatus: { full: '完整', tail_truncated: '尾部截断', empty_log: '空日志', missing_log: '日志缺失', model_responses_full: 'model_responses 完整', model_responses_tail_truncated: 'model_responses 尾部截断', model_responses_empty_log: 'model_responses 空文件' },
    goalOrigins: { admin: 'Admin 启动', external: '外部自启' },
    goalStopLevels: { exact_pid: '精确 PID', soft_state: '状态软停止', unsupported: '不可停止', none: '不可停止' },
    goalTrust: { trusted: 'PID 可信', untrusted: 'PID 不可信' },
    fields: { varName: '变量名', type: '类型', name: '名称', model: '模型', apiBase: 'API 基址', apiKey: 'API Key', stream: '流式', maxRetries: '重试', readTimeout: '超时', reasoningEffort: '推理强度', editor: 'JSON 内容', objective: '目标', budgetMinutes: '预算分钟', maxTurns: '最大轮次', llmNo: 'LLM #（可选）', pythonPath: 'Python 解释器（可选）', pythonAuto: '留空自动选择', chatDataDir: '聊天数据目录（可选）', chatDataAuto: '留空使用 %APPDATA%\\GenericAgent-Admin', goalRuns: 'Goal 运行', outputTail: '输出尾部', maxBytes: '最大字节', outputPreset64k: '64K', outputPreset256k: '256K', outputPreset1m: '1M', outputDefault: '默认64K', outputShown: '已显示', outputLines: '行数', outputLimit: '读取上限', autoRefresh: '自动刷新', notRunning: '未运行', startGoalMode: '启动 Goal Mode', goalHive: 'Hive 模式', hiveBoard: 'Hive 看板', hiveWorker: 'Hive Worker', hiveCwd: 'Hive 工作目录', goalPlaceholder: '描述要让 GA Goal Mode 持续推进的目标', pid: 'PID', turn: '轮次', remaining: '剩余', elapsed: '已用', started: '开始', ended: '结束', updated: '更新', stateFile: '状态', logFile: '日志', logMissing: '日志未创建', logReady: '日志就绪', outputStatus: '输出状态', source: '来源', control: '控制', trust: '信任', rawStatus: '原始状态', lastEvent: '最近事件', errorClass: '错误类型' }
  },
  en: {
    appName: 'GA Admin', tagline: 'GenericAgent lifecycle control plane', root: 'GenericAgent root', setupTitle: 'First-time GenericAgent setup', setupDesc: 'Select an existing GA root, or install GA into a new directory.', validateRoot: 'Validate & use', installGA: 'Install GA', installPath: 'Install path', setupOk: 'GA root configured', installDone: 'GA installed and configured', browse: 'Choose directory', checkEnv: 'Check Python / Git', envReady: 'Environment ready', envMissing: 'Environment missing', save: 'Save', refresh: 'Refresh', busy: 'Busy', ready: 'Ready', error: 'Error', empty: 'Empty', enabled: 'Enabled', disabled: 'Disabled', start: 'Start', stop: 'Stop', running: 'Running', stopped: 'Stopped', language: 'Language', copy: 'Copy', clear: 'Clear', delete: 'Delete', show: 'Show', hide: 'Hide', search: 'Search', read: 'Read', create: 'Create', remove: 'Delete', backup: 'writes create backups', autostart: 'Autostart', enableAutostart: 'Enable autostart', disableAutostart: 'Disable autostart', unsupported: 'Unsupported',
    serviceDesc: { scheduler: 'Scheduled-task runner: scans sche_tasks/ every 120s and fires tasks when their once/daily/weekly/every_Nh cadence is due, also archiving L4 session logs.', autonomous: 'Idle autonomy driver: checks every 30 min and, once the user has been away for over 30 min, prompts the agent to advance tasks on its own per the automation SOP.' },
    nav: { overview: 'Overview', chat: 'Chat', control: 'Control', files: 'Files', tasks: 'Tasks', memory: 'Memory', channels: 'Channels', autonomous: 'Autonomous', schedule: 'Schedule', goals: 'Hive Mode', models: 'Models', settings: 'Settings', logs: 'Logs' },
    desc: { overview: 'Understand and take over GA lifecycle by native domains.', chat: 'GA native conversation, uploads and streaming UI migrated from reactapp.', control: 'Readiness, capability map, risks and recent reports.', files: 'Safely browse text files inside GA root with tail and search.', tasks: 'Conversations, task files, batch entrypoints and task services.', memory: 'Layered memory, SOPs and utility indexes.', channels: 'Desktop, TUI, Web and IM Bot entrypoints.', autonomous: 'Reflection, autonomous runs, Goal Mode and team workers.', schedule: 'View, edit, create and delete sche_tasks JSON jobs.', goals: 'Continuous objective control console backed by GA Goal Mode SOP and reflect/goal_mode.py.', models: 'Import, preview and write GA mykey.py model config.', settings: 'Configure GA root, Python, chat data directory, and Chat Python proxy.', logs: 'Process state and output logs.' },
    cards: { processes: 'Processes', running: 'Running', stopped: 'Stopped', memoryLayers: 'Memory layers', sopTools: 'SOP/tools', schedule: 'Scheduled jobs', enabledTasks: 'Enabled', reports: 'Reports', coreFiles: 'Core files', reflect: 'Reflect scripts', health: 'GA health', capabilities: 'Capabilities', risks: 'Risks' },
    lists: { serviceGroups: 'Service domains', coreFiles: 'Core files', reflect: 'Reflect / Autonomous', frontends: 'Frontends / Channels', memory: 'Memory layers', sop: 'SOPs and tools', taskServices: 'Task services', frontendServices: 'Frontend services', reflectServices: 'Reflect services', reflectScripts: 'Reflect scripts', scheduledTasks: 'Scheduled jobs', recentReports: 'Recent reports', processes: 'Processes', generatedPreview: 'Generated preview', riskHints: 'Takeover hints', autostart: 'Autostart', capabilities: 'Capability map', readiness: 'Readiness', fileList: 'Files', filePreview: 'Preview', searchResults: 'Search results', editor: 'Editor' },
    hints: { rootSaved: 'GA root saved', fileSaved: 'File saved with backup', taskSaved: 'Task saved with backup', taskDeleted: 'Task deleted with backup', taskToggled: 'Task state updated', modelsSaved: 'mykey.py backed up and written', savedSecret: 'Saved; type a new value to replace', secret: 'API Key / Token', noFrontend: 'No frontend service found', noReflect: 'No reflect service found', noTasks: 'No sche_tasks/*.json', noLogs: 'No logs', previewHelp: 'Preview generated config; writing mykey.py backs up first.', modelSource: 'Source', secretHidden: 'Real secret hidden', addProfile: 'Add profile', preview: 'Preview', writeMykey: 'Write mykey.py', filePath: 'relative path', searchText: 'search text', tailLines: 'tail lines', newTaskId: 'new_task', jsonHelp: 'JSON must be an object; save/delete creates timestamped .bak.', autostartEnabled: 'Enabled: GA Admin starts automatically after user login.', autostartDisabled: 'Disabled: GA Admin must be started manually.', autostartUnsupported: 'Autostart registration is not supported on this platform.', autostartChanged: 'Autostart status updated', goalObjectiveRequired: 'Objective is required', goalObjectiveTooLarge: 'Objective exceeds 16384 bytes', goalBudgetInteger: 'Budget minutes must be an integer', goalBudgetPositive: 'Budget minutes must be positive', goalBudgetTooLarge: 'Budget minutes exceeds 43200', goalTurnsInteger: 'Max turns must be an integer', goalTurnsNonNegative: 'Max turns must be non-negative', goalTurnsTooLarge: 'Max turns cannot exceed 10000', goalLLMInteger: 'LLM # must be an integer', goalLLMNonNegative: 'LLM # cannot be negative', goalPythonHelp: 'Leave Python empty to auto-select GA root .venv, venv, uv cached interpreter, then PATH python/python3; a custom path is used for launch and recorded in Goal state.', goalHiveHelp: 'Hive mode starts the Goal Master, a temporary BBS, and one worker using the official GA flow; GA Admin only manages it from above.', goalStarted: 'Goal started', goalStopped: 'Goal stopped', goalDeleted: 'Goal deleted', goalDeleteConfirm: 'Delete Goal {id}? This removes state and log files; stop running goals first.', goalDeleteRunning: 'Running goals cannot be deleted; stop it first.', goalStopConfirm: 'Stop Goal {id}? GA Admin will use the available control level.', goalStopExactConfirm: 'Stop Admin Goal {id}? Only the exact PID {pid} recorded for this Goal will be terminated.', goalStopSoftConfirm: 'Soft-stop external Goal {id}? This will not kill the process; it only writes stopped_by_admin to the state file so the Goal loop can exit itself.', goalOutputTruncated: 'Showing tail only; earlier output was truncated', goalOutputCapped: 'Requested bytes exceeded backend limit; reading at the cap', goalOutputDefault: 'No byte limit specified; using default', goalOutputBytesInteger: 'Output bytes must be an integer', goalOutputBytesNonNegative: 'Output bytes cannot be negative', goalOutputBytesTooLarge: 'Output bytes cannot exceed 1048576', goalOutputCopied: 'Output copied', goalOutputCleared: 'Output cleared', goalOutputLogMissing: 'Log file has not been created yet; no output is available' },
    goalOutputStatus: { full: 'full', tail_truncated: 'tail truncated', empty_log: 'empty log', missing_log: 'missing log', model_responses_full: 'model_responses full', model_responses_tail_truncated: 'model_responses tail truncated', model_responses_empty_log: 'model_responses empty' },
    goalOrigins: { admin: 'Admin started', external: 'External/self-started' },
    goalStopLevels: { exact_pid: 'Exact PID', soft_state: 'State soft-stop', unsupported: 'Not stoppable', none: 'Not stoppable' },
    goalTrust: { trusted: 'PID trusted', untrusted: 'PID untrusted' },
    fields: { varName: 'Var name', type: 'Type', name: 'Name', model: 'Model', apiBase: 'API 基址', apiKey: 'API Key', stream: 'Stream', maxRetries: 'Retries', readTimeout: 'Timeout', reasoningEffort: '推理强度', editor: 'JSON content', objective: 'Objective', budgetMinutes: 'Budget minutes', maxTurns: 'Max turns', llmNo: 'LLM # (optional)', pythonPath: 'Python interpreter (optional)', pythonAuto: 'leave empty for auto', chatDataDir: 'Chat data directory (optional)', chatDataAuto: 'empty = %APPDATA%\\GenericAgent-Admin', goalRuns: 'Goal runs', goalHive: 'Hive mode', hiveBoard: 'Hive board', hiveWorker: 'Hive worker', hiveCwd: 'Hive cwd', outputTail: 'Output tail', maxBytes: 'Max bytes', outputPreset64k: '64K', outputPreset256k: '256K', outputPreset1m: '1M', outputDefault: 'Default 64K', outputShown: 'Shown', outputLines: 'Lines', outputLimit: 'Limit', autoRefresh: 'Auto refresh', notRunning: 'not running', startGoalMode: '启动 Goal 模式', goalPlaceholder: 'Describe the sustained objective for GA Goal Mode', pid: 'PID', turn: 'turn', remaining: 'remaining', elapsed: 'elapsed', started: 'started', ended: 'ended', updated: 'updated', stateFile: 'state', logFile: 'log', logMissing: 'log not created', logReady: 'log ready', outputStatus: 'output status', source: 'source', control: 'control', trust: 'trust', rawStatus: 'raw status', lastEvent: 'last event', errorClass: 'error class' }
  }
}

const TaskFormEditor = ({ value, onChange }) => {
  let data
  try { data = JSON.parse(value) } catch {}
  if (!data || typeof data !== 'object' || Array.isArray(data)) {
    return <textarea className="json-editor compact-editor" value={value} placeholder="JSON 解析失败，回退到文本编辑" onChange={e=>onChange(e.target.value)}/>
  }
  const updateField = (key, val) => {
    const next = { ...data, [key]: val }
    onChange(JSON.stringify(next, null, 2))
  }
  const extraKeys = Object.keys(data).filter(k => !['enabled','max_delay_hours','repeat','schedule','prompt'].includes(k))
  const repeatOptions = ['manual','daily','weekly','every_2h','every_4h','every_6h','every_8h','every_12h','once']

  return <div className="schedule-form-editor">
    <div className="form-field">
      <label>enabled（是否启用定时任务）</label>
      <label className="toggle-switch">
        <input type="checkbox" checked={!!data.enabled} onChange={e => updateField('enabled', e.target.checked)} />
        <span className="toggle-slider"></span>
        <span className="toggle-label">{data.enabled ? '已启用' : '已停用'}</span>
      </label>
    </div>
    <div className="form-field">
      <label>max_delay_hours（最大延迟小时数）</label>
      <input type="number" value={data.max_delay_hours ?? ''} onChange={e => updateField('max_delay_hours', e.target.value ? parseInt(e.target.value, 10) : 0)} />
    </div>
    <div className="form-field">
      <label>repeat（执行频率：每日/每周/每N小时/手动/单次）</label>
      <select value={data.repeat || ''} onChange={e => updateField('repeat', e.target.value)}>
        <option value="">-- 选择 --</option>
        {repeatOptions.map(opt => <option key={opt} value={opt}>{opt}</option>)}
      </select>
    </div>
    <div className="form-field">
      <label>schedule（定时执行时间，如 06:00）</label>
      <input type="text" value={data.schedule || ''} onChange={e => updateField('schedule', e.target.value)} placeholder="如 06:00"/>
    </div>
    <div className="form-field">
      <label>prompt（定时任务的提示词内容）</label>
      <textarea value={data.prompt || ''} onChange={e => updateField('prompt', e.target.value)} placeholder="定时任务的提示词内容"/>
    </div>
    {extraKeys.length > 0 && <details className="extra-fields">
      <summary>额外字段 ({extraKeys.length})</summary>
      <pre>{JSON.stringify(Object.fromEntries(Object.entries(data).filter(([k]) => !['enabled','max_delay_hours','repeat','schedule','prompt'].includes(k))), null, 2)}</pre>
    </details>}
  </div>
}

export default function App() {
  const defaultLang = 'zh'
  const [lang, setLang] = useState(() => localStorage.getItem('ga-admin-lang-explicit') === '1' ? (localStorage.getItem('ga-admin-lang') || defaultLang) : defaultLang)
  const chooseLang = (nextLang) => { localStorage.setItem('ga-admin-lang-explicit', '1'); setLang(nextLang) }
  const [theme, setTheme] = useState(() => localStorage.getItem('ga-admin-theme') || (typeof window !== 'undefined' && window.matchMedia?.('(prefers-color-scheme: dark)').matches ? 'dark' : 'light'))
  useEffect(() => { document.documentElement.dataset.theme = theme; localStorage.setItem('ga-admin-theme', theme) }, [theme])
  const t = I18N[lang] || I18N.en
  const initialRoute = useMemo(() => parseRoute(), [])
  const [tab, setTab] = useState(initialRoute.tab)
  const [mobileNavOpen, setMobileNavOpen] = useState(false)
  const [cfg, setCfg] = useState(null), [persistedCfg, setPersistedCfg] = useState(null), [health, setHealth] = useState(null), [control, setControl] = useState(null), [services, setServices] = useState([]), [logs, setLogs] = useState([])
  const [root, setRoot] = useState(''), [installRoot, setInstallRoot] = useState(''), [busy, setBusy] = useState(false), [booting, setBooting] = useState(true), [msg, setMsg] = useState(''), [selected, setSelected] = useState('')
  const [setupEnv, setSetupEnv] = useState(null)
  const [autostart, setAutostart] = useState(null)
  const [versionInfo, setVersionInfo] = useState(null), [versionCheck, setVersionCheck] = useState(null), [versionStatus, setVersionStatus] = useState(null), [versionBusy, setVersionBusy] = useState(false), [gitBusy, setGitBusy] = useState(false), [gitResult, setGitResult] = useState(null), [gitStatus, setGitStatus] = useState(null)
  const [tmwdStatus, setTmwdStatus] = useState(null)
  const [observability, setObservability] = useState(null), [observabilityError, setObservabilityError] = useState('')
  const [profiles, setProfiles] = useState([]), [modelPreview, setModelPreview] = useState('')
  const [persistedModelProfiles, setPersistedModelProfiles] = useState([])
  const [modelSaveStatus, setModelSaveStatus] = useState({})
  const [modelImportLoading, setModelImportLoading] = useState(false)
  const [modelRevealedKeys, setModelRevealedKeys] = useState({}), [modelKeyBusy, setModelKeyBusy] = useState({})
  const [browsePath, setBrowsePath] = useState('memory'), [filePath, setFilePath] = useState(''), [loadedFilePath, setLoadedFilePath] = useState(''), [fileList, setFileList] = useState([]), [fileContent, setFileContent] = useState(''), [loadedFileContent, setLoadedFileContent] = useState(''), [fileSearch, setFileSearch] = useState(''), [searchHits, setSearchHits] = useState([]), [tailLines, setTailLinesRaw] = useState(200)
  const [taskId, setTaskId] = useState(''), [taskEditor, setTaskEditor] = useState('{}'), [loadedTaskEditor, setLoadedTaskEditor] = useState('{}'), [newTaskId, setNewTaskId] = useState('new_task')
  const [editorMode, setEditorMode] = useState('form')
  const [scheduleData, setScheduleData] = useState(null), [scheduleLoading, setScheduleLoading] = useState(false), [scheduleError, setScheduleError] = useState('')
  const [taskSubTab, setTaskSubTab] = useState(initialRoute.taskSubTab)
  const [scheduleArtifactTitle, setScheduleArtifactTitle] = useState(''), [scheduleArtifact, setScheduleArtifact] = useState('')
  const [goals, setGoals] = useState([]), [goalObjective, setGoalObjective] = useState(''), [goalBudget, setGoalBudget] = useState(480), [goalMaxTurns, setGoalMaxTurns] = useState(200), [goalLLMNo, setGoalLLMNo] = useState(''), [goalHive, setGoalHive] = useState(false), [selectedGoal, setSelectedGoal] = useState(''), [goalOutput, setGoalOutput] = useState(''), [goalOutputMeta, setGoalOutputMeta] = useState(null)
  const [goalOutputBytes, setGoalOutputBytes] = useState(() => localStorage.getItem('ga-admin-goal-output-bytes') || '120000')
  const [goalAutoRefresh, setGoalAutoRefresh] = useState(() => localStorage.getItem('ga-admin-goal-auto-refresh') !== 'false')
  const goalOutputSeq = useRef(0), goalRefreshBusy = useRef(false)
  const modelImportAttempted = useRef(false)
  const [llms, setLLMs] = useState([]), [reflectLLMNo, setReflectLLMNo] = useState(''), [showLLMPicker, setShowLLMPicker] = useState(false), [pendingServiceName, setPendingServiceName] = useState('')
  const appScope = useRef(null)
  const allowUnloadRef = useRef(false)
  const dismissMessage = useCallback(() => setMsg(''), [])

  useGSAP(() => {
    if (tab === 'chat' || prefersReducedMotion()) return
    const ctx = gsap.context(() => {
      const q = gsap.utils.selector(appScope)
      let tl
      const play = () => {
        tl = gsap.timeline({ defaults: { ease: 'power2.out', duration: 0.28 } })
        tl.from(q('.main > header'), { y: 8, autoAlpha: 0, clearProps: 'transform,opacity,visibility' })
          .from(q('.stats .stat, .panel, .workspace, .logs-layout, .goals-page, .settings-page'), { y: 10, autoAlpha: 0, stagger: 0.025, clearProps: 'transform,opacity,visibility' }, '-=0.12')
      }
      const raf = window.requestAnimationFrame(play)
      const guard = window.setTimeout(() => {
        gsap.set(q('.main > header, .stats .stat, .panel, .workspace, .logs-layout, .goals-page, .settings-page'), { autoAlpha: 1, clearProps: 'transform,opacity,visibility' })
      }, 900)
      return () => { window.cancelAnimationFrame(raf); window.clearTimeout(guard); tl?.kill() }
    }, appScope)
    return () => ctx.revert()
  }, { scope: appScope, dependencies: [tab, lang] })

  const inv = health?.inventory || {}
  const versionUX = updateStatusPresentation(versionStatus)
  const schedule = scheduleData || inv.schedule || {}
  const tasks = normalizeScheduleTasksPayload(schedule).tasks
  const fileDirty = fileEditorDirty(fileContent, loadedFileContent)
  const taskDirty = fileEditorDirty(taskEditor, loadedTaskEditor)
  const settingsDirty = configDraftDirty(root, cfg, persistedCfg)
  const hasUnsavedChanges = fileDirty || taskDirty || settingsDirty
  const gitSyncView = gitSyncPresentation(gitStatus)
  const taskSvcs = useMemo(() => group(services, s => s.kind === 'task' || s.name?.includes('task') || s.name?.includes('scheduler')), [services])
  const frontendSvcs = useMemo(() => group(services, s => s.kind === 'frontend'), [services])
  const reflectSvcs = useMemo(() => group(services, s => s.name?.includes('scheduler') || s.name?.includes('autonomous')), [services])

  const loadScheduleTasks = async ({ quiet = false } = {}) => {
    if (!quiet) setScheduleLoading(true)
    setScheduleError('')
    try {
      const d = await api('/api/schedule/tasks')
      const normalized = normalizeScheduleTasksPayload(d)
      setScheduleData(normalized)
      return normalized
    } catch (e) {
      setScheduleData({ enabled: false, version: 'unknown', tasks: [] })
      setScheduleError(e.message)
      if (!quiet) setMsg(e.message)
      throw e
    } finally {
      if (!quiet) setScheduleLoading(false)
    }
  }

  const loadLLMs = async () => { try { const d = await api('/api/chat/state'); setLLMs(d.llms || []) } catch(e){ console.error('加载模型列表失败:', e) } }
  const refreshTMWebDriverStatus = async () => {
    const d = await api('/api/tmwebdriver/status')
    setTmwdStatus(d)
    return d
  }
  const repairTMWebDriver = async () => {
    if (!confirmDanger('tmwebdriver-repair', '启动或修复 TMWebDriver master 进程？')) return
    setBusy(true); setMsg('正在启动 TMWebDriver master…')
    try {
      const d = await api('/api/tmwebdriver/repair', { dangerous:true, method:'POST', body: '{}' })
      setTmwdStatus(d.status)
      setMsg(d.message || (d.started ? `已启动 TMWebDriver master PID ${d.pid}` : 'TMWebDriver master 已在运行'))
    } catch(e){ setMsg(`TMWebDriver 修复失败：${e.message}`) } finally{ setBusy(false) }
  }

  const readObservability = async () => {
    const request = (endpoint) => {
      const req = observabilityRequest(endpoint)
      return api(req.url, req.options)
    }
    const [apiHealth, inventory, risks] = await Promise.all([
      request('/api/health'),
      request('/api/ga/inventory'),
      request('/api/risk/catalog')
    ])
    const snapshot = buildObservabilitySnapshot({ health: apiHealth, inventory, risks })
    setObservability(snapshot)
    setObservabilityError('')
    return snapshot
  }

  const load = async () => {
    setBooting(true)
    try {
      const [c, h, auto, ver, vstat] = await Promise.all([
        api('/api/config'),
        api('/api/ga/health'),
        api('/api/autostart/status').catch(e => ({ supported:false, enabled:false, error:e.message })),
        api('/api/version/info').catch(e => ({ error:e.message })),
        api('/api/version/status').catch(() => null)
      ])
      setCfg(c); setPersistedCfg(c); setRoot(c.ga_root || ''); setHealth(h); setAutostart(auto); setVersionInfo(ver); if (vstat?.id || vstat?.stage) setVersionStatus(vstat)
      await readObservability().catch(e => { setObservability(null); setObservabilityError(e.message) })
      if (!h?.ok) {
        setServices([]); setControl(null); setLogs([]); setFileList([])
        return
      }
      const [svc, ctrl] = await Promise.all([api('/api/services'), api('/api/ga/control')])
      const serviceList = Array.isArray(svc) ? svc : (svc.services || [])
      setServices(serviceList); setControl(ctrl)
      const first = serviceList[0]?.name; if (!selected && first) setSelected(first)
      if (tab === 'goals') {
        const goalData = await api('/api/goals/list').catch(() => ({ goals: [] }))
        const goalItems = goalData.goals || []
        setGoals(goalItems)
        const firstGoal = pickGoalId(goalItems, selectedGoal)
        if (!selectedGoal && firstGoal) setSelectedGoal(firstGoal)
      }
      if (tab === 'files') loadFiles(browsePath).catch(e => setMsg(e.message))
      if (tab === 'tasks') loadScheduleTasks({ quiet:true }).catch(e => setScheduleError(e.message))
      if (tab === 'setup') refreshTMWebDriverStatus().catch(e => setTmwdStatus({ ok:false, error:e.message }))
    } catch (e) { setMsg(e.message) } finally { setBooting(false) }
  }
  useEffect(() => { load() }, [])
  useEffect(() => {
    if (tab === 'goals' && health?.ok) { loadGoals().catch(e => setMsg(e.message)); loadLLMs() }
    if (tab === 'autonomous' && health?.ok && !llms.length) loadLLMs()
    if (tab === 'files' && health?.ok && !fileList.length) loadFiles(browsePath).catch(e => setMsg(e.message))
    if (tab === 'setup' && health?.ok && !tmwdStatus) refreshTMWebDriverStatus().catch(e => setTmwdStatus({ ok:false, error:e.message }))
  }, [tab, health?.ok])
  const toggleAutostart = async () => { const next = !autostart?.enabled; if (!confirmDanger('admin-autostart', next ? '启用 GA Admin 开机自启动？' : '禁用 GA Admin 开机自启动？')) return; setBusy(true); setMsg(''); try { const d = await api(next ? '/api/autostart/enable' : '/api/autostart/disable', { dangerous:true, method:'POST' }); setAutostart(d); setMsg(t.hints.autostartChanged) } catch(e){ setMsg(e.message) } finally{ setBusy(false) } }
  const checkGASource = async () => { setGitBusy(true); setMsg(''); try { const d = await api('/api/ga/git-status?remote=1'); setGitStatus(d); setMsg(gitSyncPresentation(d).summary) } catch(e){ setGitStatus({ ok:false, error:e.message }); setMsg(e.message) } finally{ setGitBusy(false) } }
  const updateGASource = async () => { if (!confirmDanger('ga-git-update', '按 daily_git_pull_merge_push 策略同步 GA 仓库？将 fetch origin、合并远端、提交全部本地变更（包括 memory 和 mykey.py）并 push；不会合并 upstream，也不会 force。')) return; setGitBusy(true); setMsg(''); try { const d = await api('/api/ga/git-update', { dangerous:true, method:'POST', body: '{}' }); setGitResult(d); setGitStatus(d); setMsg(gitSyncPresentation(d).summary); await load() } catch(e){ setMsg(e.message) } finally{ setGitBusy(false) } }
  const saveConfig = async () => { if (!settingsDirty || !confirmDanger('config-save', '保存全部 GA Admin 配置？会写入配置文件并可能切换 GA 根目录。')) return; setBusy(true); try { const c = await api('/api/config', { dangerous:true, method: 'PUT', body: JSON.stringify({ ...cfg, ga_root: root }) }); setCfg(c); setPersistedCfg(c); setRoot(c.ga_root || ''); setMsg(t.hints.rootSaved); await load() } catch(e){ setMsg(e.message) } finally{ setBusy(false) } }
  const resetConfigDraft = () => { if (!persistedCfg) return; setCfg({ ...persistedCfg }); setRoot(persistedCfg.ga_root || ''); setMsg('已放弃未保存的配置更改') }
  const refreshApp = () => {
    if (settingsDirty && !window.confirm('配置页面有未保存更改。刷新数据将放弃这些更改，是否继续？')) return
    load()
  }
  const checkSetupEnv = async () => { setBusy(true); try { const d = await api('/api/setup/env'); setSetupEnv(d); setMsg(d.ok ? t.envReady : t.envMissing) } catch(e){ setMsg(e.message) } finally{ setBusy(false) } }
  const browseSetupDir = async (target = 'root') => { setBusy(true); try { const base = target === 'install' ? installRoot : root; const d = await api('/api/setup/browse', { method:'POST', body: JSON.stringify({ path: base }) }); if (d.path) { target === 'install' ? setInstallRoot(d.path) : setRoot(d.path) } } catch(e){ setMsg(e.message) } finally{ setBusy(false) } }
  const validateSetupRoot = async () => { if (!confirmDanger('setup-validate-root', '验证并保存当前 GA 根目录？')) return; setBusy(true); try { const d = await api('/api/setup/validate', { dangerous:true, method:'POST', body: JSON.stringify({ path: root }) }); if (!d.ok) throw new Error('GenericAgent health check failed'); const c = await api('/api/config', { dangerous:true, method:'PUT', body: JSON.stringify({ ...cfg, ga_root: d.root }) }); setCfg(c); setRoot(d.root); setMsg(t.setupOk); await load() } catch(e){ setMsg(e.message) } finally{ setBusy(false) } }
  const installGA = async () => { if (!confirmDanger('setup-install-ga', '安装/克隆 GenericAgent 到目标目录？会写入本地文件。')) return; setBusy(true); try { const env = setupEnv || await api('/api/setup/env'); setSetupEnv(env); if (!env.ok) throw new Error(t.envMissing); const d = await api('/api/setup/install', { dangerous:true, method:'POST', body: JSON.stringify({ path: installRoot || root }) }); setRoot(d.root); setMsg(t.installDone); await load() } catch(e){ setMsg(e.message) } finally{ setBusy(false) } }
  const startReflectService = (name) => {
    const fallbackModel = llms.find(m => m?.index !== undefined && m?.index !== null)
    setReflectLLMNo(current => current !== '' ? current : (fallbackModel?.index?.toString() || '0'))
    setPendingServiceName(name)
    setShowLLMPicker(true)
  }
  const confirmReflectStart = async () => {
    const selectedLLMNo = String(reflectLLMNo || '').trim()
    if (!/^\d+$/.test(selectedLLMNo)) {
      setMsg('请选择有效的模型编号')
      return
    }
    setShowLLMPicker(false)
    await serviceAction(pendingServiceName, 'start', { llm_no: selectedLLMNo })
    setPendingServiceName('')
  }

  const serviceAction = async (name, action, params = null) => { if (!confirmDanger(`service-${action}`, `${action === 'start' ? '启动' : '停止'}服务 ${name}？`)) return; setBusy(true); try { const body = { name }; if (params) body.params = params; await api(`/api/services/${action}`, { dangerous:true, method:'POST', body: JSON.stringify(body) }); await load(); if (selected === name) setLogs((await api(`/api/logs/${encodeURIComponent(name)}?lines=${tailLines}`)).lines || []) } catch(e){ setMsg(e.message) } finally{ setBusy(false) } }
  const toggleServiceAutostart = async (name, enabled) => { if (!confirmDanger('service-autostart', `${enabled ? '启用' : '禁用'}服务 ${name} 自启动？`)) return; setBusy(true); try { const d = await api('/api/services/autostart', { dangerous:true, method:'POST', body: JSON.stringify({ name, enabled }) }); setServices(d.services || []); setMsg(enabled ? t.enabled : t.disabled) } catch(e){ setMsg(e.message) } finally{ setBusy(false) } }
  const setServiceModel = async (name, llm_no) => { setBusy(true); try { const d = await api('/api/services/model', { dangerous:true, method:'POST', body: JSON.stringify({ name, llm_no }) }); setServices(d.services || []); setMsg(t.saved || '已保存') } catch(e){ setMsg(e.message) } finally{ setBusy(false) } }
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
      const body = { objective, budget_minutes: budgetMinutes, max_turns: maxTurns, hive: !!goalHive }
      if (llmNo !== null) body.llm_no = llmNo
      if (!confirmDanger('goals-start', `启动${goalHive ? ' Hive' : ''}自主目标？预算 ${budgetMinutes} 分钟，最大轮次 ${maxTurns || '不限'}。`)) return
      const d = await api('/api/goals/start', { dangerous:true, method:'POST', body: JSON.stringify(body) })
      setMsg(`${t.hints.goalStarted}: ${d.goal?.id || ''}`); setGoalObjective(''); setSelectedGoal(d.goal?.id || selectedGoal); await loadGoals(); if (d.goal?.id) await loadGoalOutput(d.goal.id)
    } catch(e){ setMsg(e.message) } finally{ setBusy(false) }
  }
  const stopGoal = async (g) => {
    if (!g) return
    const exact = !!g.managed
    const tpl = exact ? (t.hints.goalStopExactConfirm || t.hints.goalStopConfirm) : (t.hints.goalStopSoftConfirm || t.hints.goalStopConfirm)
    const confirmText = tpl.replace('{id}', g.id || '-').replace('{pid}', g.pid || '-')
    if (!confirmDanger('goals-stop', confirmText)) return
    setBusy(true); setMsg('')
    try {
      const body = { id: g.id }
      if (g.pid) body.pid = g.pid
      await api('/api/goals/stop', { dangerous:true, method:'POST', body: JSON.stringify(body) })
      setMsg(`${t.hints.goalStopped}: ${g.id}`); await loadGoals(); if (selectedGoal === g.id) await loadGoalOutput(g.id)
    } catch(e){ setMsg(e.message) } finally{ setBusy(false) }
  }
  const deleteGoal = async (g) => { if (!g) return; const confirmText = t.hints.goalDeleteConfirm.replace('{id}', g.id || '-'); if (!confirmDanger('goals-delete', confirmText)) return; setBusy(true); setMsg(''); try { await api('/api/goals/delete', { dangerous:true, method:'POST', body: JSON.stringify({ id: g.id }) }); setMsg(`${t.hints.goalDeleted}: ${g.id}`); const gs = await loadGoals(); if (selectedGoal === g.id) { const next = pickGoalId(gs, ''); setSelectedGoal(next); setGoalOutput(''); setGoalOutputMeta({}); if (next) await loadGoalOutput(next) } } catch(e){ setMsg(e.message) } finally{ setBusy(false) } }
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
  const toggleTask = async (id, enabled) => {
    if (!confirmDanger('schedule-toggle', `${enabled ? '启用' : '停用'}计划任务 ${id}？`)) return
    setBusy(true)
    try { await api('/api/schedule/toggle', { dangerous:true, method:'POST', body: JSON.stringify({ id, enabled }) }); setMsg(t.hints.taskToggled); await load() } catch(e){ setMsg(e.message) } finally{ setBusy(false) }
  }

  const setTailLines = (value) => setTailLinesRaw(clampTailLines(value))
  const loadFiles = async (path = '') => {
    try {
      const d = await api(`/api/files/list?path=${encodeURIComponent(path || '')}`)
      setFileList(d.items || d.entries || [])
      setBrowsePath(path || '')
    } catch (e) {
      setFileList([])
      setMsg(e.message)
      throw e
    }
  }
  const confirmFileReplacement = path => !fileDirty || window.confirm(`文件 ${loadedFilePath || filePath || '-'} 有未保存更改。读取 ${path} 将覆盖当前编辑内容，是否继续？`)
  const readFile = async (path = filePath) => { if (!path || !confirmFileReplacement(path)) return; setBusy(true); try { const d = await api(`/api/files/read?path=${encodeURIComponent(path)}`); const content = d.content || ''; setFileContent(content); setLoadedFileContent(content); setLoadedFilePath(path); setFilePath(path); setTab('files') } catch(e){ setMsg(e.message) } finally{ setBusy(false) } }
  const tailFile = async (path = filePath) => { if (!path || !confirmFileReplacement(path)) return; setBusy(true); try { const safeLines = clampTailLines(tailLines); const d = await api(`/api/files/tail?path=${encodeURIComponent(path)}&lines=${safeLines}`); const content = d.content || ''; setFileContent(content); setLoadedFileContent(content); setLoadedFilePath(path); setTailLinesRaw(safeLines); setFilePath(path); setTab('files') } catch(e){ setMsg(e.message) } finally{ setBusy(false) } }
  const saveFile = async () => { if (!filePath || !fileEditorDirty(fileContent, loadedFileContent)) return; if (loadedFilePath && filePath !== loadedFilePath && !confirmDanger('files-retarget', `Editor content was loaded from ${loadedFilePath}, but will be saved to ${filePath}. Continue?`)) return; if (!confirmDanger('files-write', `Write file ${filePath}? This overwrites content and the backend will create a backup.`)) return; setBusy(true); try { const d = await api('/api/files/write', { dangerous:true, method:'POST', body: JSON.stringify({ path:filePath, content:fileContent }) }); const savedContent = d.content || fileContent; setFileContent(savedContent); setLoadedFileContent(savedContent); setLoadedFilePath(filePath); setMsg(t.hints.fileSaved || t.saved || 'Saved'); await loadFiles(dirnameForPath(filePath)) } catch(e){ setMsg(e.message) } finally{ setBusy(false) } }
  const deleteFile = async (path = filePath) => { if (!path) return; if (!confirmDanger('files-delete', `Delete ${path}? This removes the file or directory under GA root.`)) return; setBusy(true); try { await api('/api/files/delete', { dangerous:true, method:'POST', body: JSON.stringify({ path }) }); if (path === loadedFilePath) { setFileContent(''); setLoadedFileContent(''); setLoadedFilePath('') } setMsg(t.deleted || 'Deleted'); await loadFiles(dirnameForPath(path)) } catch(e){ setMsg(e.message) } finally{ setBusy(false) } }
  const downloadFile = (path = filePath) => { if (!path) return; window.open(`/api/files/download?path=${encodeURIComponent(path)}`, '_blank', 'noopener,noreferrer') }


  const refreshVersionStatus = async () => {
    const d = await api('/api/version/status')
    setVersionStatus(d)
    if (d?.check) setVersionCheck(d.check)
    if (!d?.running && (d?.stage === 'done' || d?.stage === 'error')) {
      const info = await api('/api/version/info').catch(() => null)
      if (info) setVersionInfo(info)
      if (d.stage === 'done') {
        const check = await api('/api/version/check').catch(() => null)
        if (check) setVersionCheck(check)
      }
    }
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
    if (!confirmDanger('version-update', '下载并重启 GA Admin 以完成升级？页面可刷新，进度会自动恢复。')) return
    setVersionBusy(true)
    try { const d = await api('/api/version/update', { dangerous:true, method:'POST', body:'{}' }); setVersionStatus(d); setMsg(d.message || '升级已启动') }
    catch(e){ setMsg(e.message) }
    finally{ setVersionBusy(false) }
  }
  const installTMWebDriverDeps = async () => {
    if (!confirmDanger('tmwebdriver-install-deps', '将使用当前 GA Python 执行 pip install requests bottle simple-websocket-server（清华源）。继续？')) return
    setBusy(true)
    try { const d = await api('/api/tmwebdriver/install-deps', { dangerous:true, method:'POST', body:'{}' }); setTmwdStatus(d.status || d); setMsg(d.ok ? 'TMWebDriver 依赖安装完成' : (d.error || '依赖安装失败，请查看输出')) }
    catch(e){ setMsg(e.message) }
    finally{ setBusy(false) }
  }
  const configureGitMirror = async (enabled) => {
    if (!confirmDanger('git-mirror', enabled ? '将写入全局 git GitHub 镜像 insteadOf 配置。继续？' : '将移除默认 GitHub 镜像 insteadOf 配置。继续？')) return
    setGitBusy(true)
    try { const d = await api('/api/ga/git-mirror', { dangerous:true, method:'POST', body: JSON.stringify({ enabled }) }); setGitResult(d); setMsg(d.ok ? (enabled ? 'GitHub 镜像已启用' : 'GitHub 镜像已关闭') : (d.error || 'Git 镜像配置失败')) }
    catch(e){ setMsg(e.message) }
    finally{ setGitBusy(false) }
  }
  const runSearch = async () => { setBusy(true); try { const d = await api(`/api/files/search?path=${encodeURIComponent(browsePath)}&q=${encodeURIComponent(fileSearch)}&limit=80`); setSearchHits(d.hits || []) } catch(e){ setMsg(e.message) } finally{ setBusy(false) } }

  const loadTask = async (id) => { if (taskDirty && !window.confirm(`定时任务 ${taskId || '-'} 有未保存更改。读取 ${id} 将覆盖当前编辑内容，是否继续？`)) return; setBusy(true); try { const d = await api(`/api/schedule/task?id=${encodeURIComponent(id)}`); const content = safeJson(d.raw); setTaskId(d.id || id); setTaskEditor(content); setLoadedTaskEditor(content); setTab('tasks'); setTaskSubTab('scheduled') } catch(e){ setMsg(e.message) } finally{ setBusy(false) } }
  const saveTask = async () => { const id = taskId || newTaskId; if (!taskDirty || !confirmDanger('schedule-save', `保存定时任务 ${id}？后端会写入 JSON 并生成备份。`)) return; setBusy(true); try { let raw = JSON.parse(taskEditor); if (editorMode==='form') { const known = ['enabled','max_delay_hours','repeat','schedule','prompt']; const filtered = {}; for (const k of known) if (k in raw && raw[k] !== undefined && raw[k] !== null && raw[k] !== '') filtered[k] = raw[k]; raw = filtered; } await api('/api/schedule/task', { dangerous:true, method:'PUT', body: JSON.stringify({ id, raw }) }); const saved = safeJson(raw); setTaskEditor(saved); setLoadedTaskEditor(saved); setMsg(t.hints.taskSaved); await load(); setTaskSubTab('scheduled') } catch(e){ setMsg(e.message) } finally{ setBusy(false) } }
  const createTask = async () => { const id = newTaskId.trim(); if (!id) { setMsg('Schedule task id is required'); return }; if (taskDirty && !window.confirm(`定时任务 ${taskId || '-'} 有未保存更改。创建新任务将替换当前编辑器内容，是否继续？`)) return; if (!confirmDanger('schedule-create', `Create schedule task ${id}? This writes a sche_tasks JSON file.`)) return; setBusy(true); try { const payload = buildScheduleCreateRequest(id, DEFAULT_SCHEDULE_TASK); const d = await api('/api/schedule/create', { dangerous:true, method:'POST', body: JSON.stringify(payload) }); const created = d.task || DEFAULT_SCHEDULE_TASK; const content = safeJson(created.raw || payload.task); setTaskId(created.id || id); setTaskEditor(content); setLoadedTaskEditor(content); setMsg(t.hints.taskSaved); await loadScheduleTasks(); setTaskSubTab('scheduled') } catch(e){ setMsg(e.message) } finally{ setBusy(false) } }
  const deleteTask = async () => { if (!taskId) return; if (!confirmDanger('schedule-delete', `删除定时任务 ${taskId}？后端会先生成备份。`)) return; setBusy(true); try { await api('/api/schedule/delete', { dangerous:true, method:'POST', body: JSON.stringify({ id: taskId }) }); setMsg(t.hints.taskDeleted); setTaskId(''); setTaskEditor('{}'); setLoadedTaskEditor('{}'); await load(); setTaskSubTab('scheduled') } catch(e){ setMsg(e.message) } finally{ setBusy(false) } }
  const readScheduleArtifact = async (path, targetTab = 'tasks') => { setBusy(true); try { const d = await api(`/api/schedule/artifact?path=${encodeURIComponent(path)}`); setScheduleArtifactTitle(path); setScheduleArtifact(d.content || ''); setTab(targetTab); setTaskSubTab('reports') } catch(e){ setMsg(e.message) } finally{ setBusy(false) } }

  const importModels = async ({ quiet = false } = {}) => {
    if (!quiet) setBusy(true)
    setModelImportLoading(true)
    try {
      const d = await api('/api/models/import-mykey', { method:'POST', body: JSON.stringify({ reveal:false, save:false }) })
      const nextProfiles = d.profiles || []
      setProfiles(nextProfiles)
      setPersistedModelProfiles(nextProfiles)
      setModelSaveStatus({})
      setModelRevealedKeys({})
      setModelKeyBusy({})
      setModelPreview(safeJson(d))
      setMsg(`已载入 ${nextProfiles.length} 个服务商配置`)
    } catch(e) { setMsg(e.message) } finally { setModelImportLoading(false); if (!quiet) setBusy(false) }
  }
  useEffect(() => {
    if (tab !== 'models' || modelImportAttempted.current || profiles.length) return
    modelImportAttempted.current = true
    importModels({ quiet: true })
  }, [tab, profiles.length])
  const previewModels = async () => { setBusy(true); try { const d = await api('/api/models/preview', { method:'POST', body: JSON.stringify({ profiles }) }); setModelPreview(d.python || safeJson(d)) } catch(e){ setMsg(e.message) } finally{ setBusy(false) } }
  const isMaskedModelSecret = (value) => String(value || '').trim() === '******'
  const getModelProfileKey = (idx, profile) => profile?.client_id || `${profile?.var_name || `profile_${idx + 1}`}:${profile?.type || 'native_oai'}:${profile?.apibase || ''}:${idx}`
  const clearRevealedModelKey = (idx, profile) => {
    const key = getModelProfileKey(idx, profile || profiles[idx])
    setModelRevealedKeys(prev => {
      const next = { ...prev }
      delete next[key]
      delete next[idx]
      return next
    })
    setModelKeyBusy(prev => {
      const next = { ...prev }
      delete next[key]
      delete next[idx]
      return next
    })
  }
  const discoverModels = async ({ protocol, baseUrl, apiKey, varName } = {}) => {
    const params = new URLSearchParams()
    if (protocol) params.set('protocol', protocol)
    if (baseUrl) params.set('base_url', baseUrl)
    if (apiKey) params.set('api_key', apiKey)
    if (varName) params.set('var_name', varName)
    return api(`/api/models/discover?${params.toString()}`)
  }
  const probeModels = async ({ protocol, baseUrl, apiKey, varName, models, modelOptions } = {}) => api('/api/models/probe', {
    method: 'POST',
    body: JSON.stringify({ protocol, base_url: baseUrl, api_key: apiKey, var_name: varName, models, model_options: modelOptions }),
  })
  const saveModelProbeProviders = async providerKeys => {
    if (!confirmDanger('model-probe-scope', '保存模型批量检测范围？该配置会写入 GA Admin 本地配置文件。')) {
      throw new Error('已取消保存检测范围')
    }
    const c = await api('/api/config', {
      dangerous: true,
      method: 'PUT',
      body: JSON.stringify({ ...cfg, model_probe_providers: providerKeys }),
    })
    setCfg(c)
    setMsg('模型检测范围已保存')
    return c.model_probe_providers || []
  }
  const revealModelKey = async (idx, profile, refresh = false) => {
    const profileKey = getModelProfileKey(idx, profile || profiles[idx])
    if (!refresh && Object.prototype.hasOwnProperty.call(modelRevealedKeys, profileKey)) {
      clearRevealedModelKey(idx, profile)
      return
    }
    setModelKeyBusy(prev => ({ ...prev, [profileKey]: true }))
    try {
      const d = await api('/api/models/raw', { dangerous: true })
      const rawProfiles = d?.profiles || []
      const varName = String(profile?.var_name || '').trim()
      const raw = (varName ? rawProfiles.find(p => String(p.var_name || '').trim() === varName) : null) || rawProfiles[idx]
      const key = String(raw?.apikey || profile?.apikey || '').trim()
      setModelRevealedKeys(prev => ({ ...prev, [profileKey]: key }))
    } catch (e) {
      setMsg(`Failed to reveal model key: ${e.message}`)
    } finally {
      setModelKeyBusy(prev => ({ ...prev, [profileKey]: false }))
    }
  }

  const persistModelProfiles = async (nextProfiles, { confirm = true, statusKeys = [] } = {}) => {
    if (confirm && !confirmDanger('models-save', '保存模型配置会更新 mykey.py，并可能覆盖当前启用配置。确认继续？')) return false
    const saving = Object.fromEntries(statusKeys.map(k => [k, { status: 'saving', error: '', savedAt: null }]))
    if (statusKeys.length) setModelSaveStatus(current => ({ ...current, ...saving }))
    setBusy(true)
    try {
      const d = await api('/api/models/export', { dangerous:true, method:'POST', body: JSON.stringify({ profiles: nextProfiles, overwrite_active:true }) })
      const cleanProfiles = nextProfiles.map(({ previous_var_name: _previousVarName, ...profile }) => profile)
      setPersistedModelProfiles(cleanProfiles)
      setModelPreview(safeJson(d))
      if (statusKeys.length) {
        const saved = Object.fromEntries(statusKeys.map(k => [k, { status: 'saved', error: '', savedAt: d.updated_at || new Date().toISOString() }]))
        setModelSaveStatus(current => ({ ...current, ...saved }))
      }
      setMsg(t.hints.modelsSaved)
      return true
    } catch(e) {
      setMsg(e.message)
      if (statusKeys.length) {
        const failed = Object.fromEntries(statusKeys.map(k => [k, { status: 'error', error: e.message, savedAt: null }]))
        setModelSaveStatus(current => ({ ...current, ...failed }))
      }
      return false
    } finally { setBusy(false) }
  }

  const saveModelProfile = async (idx, profileKeyOverride, profileOverride) => {
    const profile = profileOverride || profiles[idx]
    if (!profile) return
    const profileKey = profileKeyOverride || getModelProfileKey(idx, profile)
    const nextPersisted = (persistedModelProfiles.length ? persistedModelProfiles : profiles).map(p => ({ ...p }))
    while (nextPersisted.length < profiles.length) nextPersisted.push({ ...profiles[nextPersisted.length] })
    const previousVarName = String(persistedModelProfiles[idx]?.var_name || '').trim()
    nextPersisted[idx] = previousVarName && previousVarName !== String(profile.var_name || '').trim()
      ? { ...profile, previous_var_name: previousVarName }
      : { ...profile }
    return await persistModelProfiles(nextPersisted, { confirm: false, statusKeys: [profileKey] })
  }

  const saveModelProfiles = async nextProfiles => {
    const statusKeys = nextProfiles.map((profile, idx) => getModelProfileKey(idx, profile))
    const ok = await persistModelProfiles(nextProfiles, { confirm: false, statusKeys })
    if (ok) setProfiles(nextProfiles)
    return ok
  }

  const saveModelOrder = async (orderedRows) => {
    const nextPersisted = applyModelOrder(persistedModelProfiles, orderedRows)
    const ok = await persistModelProfiles(nextPersisted, { confirm: false })
    if (!ok) return false
    setProfiles(current => mergePersistedModelOrder(current, nextPersisted))
    return true
  }

  const addModelProfiles = async (newProfiles) => {
    const nextProfiles = [...profiles, ...newProfiles]
    const statusKeys = newProfiles.map((profile, i) => getModelProfileKey(profiles.length + i, profile))
    setProfiles(nextProfiles)
    const ok = await persistModelProfiles(nextProfiles, { confirm: false, statusKeys })
    if (!ok) setProfiles(profiles)
    return ok
  }
  const deleteModelProfile = async (nextProfiles) => {
    setProfiles(nextProfiles)
    const ok = await persistModelProfiles(nextProfiles, { confirm: false, statusKeys: [] })
    if (!ok) setProfiles(profiles)
    return ok
  }
  const patchProfile = (idx, patch) => {
    const shouldClearSecret = Object.prototype.hasOwnProperty.call(patch, 'apikey') || Object.prototype.hasOwnProperty.call(patch, 'var_name')
    if (shouldClearSecret) clearRevealedModelKey(idx, profiles[idx])
    setProfiles(ps => ps.map((p, i) => i === idx ? { ...p, ...patch } : p))
  }

  const nav = NAV_ITEMS
  const pushRoute = (nextTab, nextTaskSubTab = taskSubTab) => {
    const route = buildRoute(nextTab, nextTaskSubTab)
    if (window.location.pathname !== route || window.location.hash) window.history.pushState({}, '', route)
  }
  const navigateTo = nextTab => {
    setMobileNavOpen(false)
    dismissMessage()
    if (nextTab === 'chat') {
      if (hasUnsavedChanges && !window.confirm('当前有未保存的文件、任务或配置更改。进入独立 Chat 页面将放弃这些更改，是否继续？')) return
      allowUnloadRef.current = true
      window.location.href = buildRoute('chat')
      return
    }
    pushRoute(nextTab)
    setTab(nextTab)
  }
  const navigateTaskSubTab = nextSubTab => {
    if (!TASK_SUB_TABS.includes(nextSubTab)) return
    dismissMessage()
    pushRoute('tasks', nextSubTab)
    setTaskSubTab(nextSubTab)
  }

  useEffect(() => {
    const route = buildRoute(tab, taskSubTab)
    if (window.location.pathname !== route || window.location.hash) window.history.replaceState({}, '', route)
  }, [tab, taskSubTab])

  useEffect(() => {
    const onPopState = () => {
      const route = parseRoute()
      setTab(route.tab)
      setTaskSubTab(route.taskSubTab)
      setMobileNavOpen(false)
      dismissMessage()
    }
    window.addEventListener('popstate', onPopState)
    return () => window.removeEventListener('popstate', onPopState)
  }, [dismissMessage])

  useEffect(() => {
    if (!hasUnsavedChanges) return undefined
    const onBeforeUnload = event => { if (!allowUnloadRef.current) { event.preventDefault(); event.returnValue = '' } }
    window.addEventListener('beforeunload', onBeforeUnload)
    return () => window.removeEventListener('beforeunload', onBeforeUnload)
  }, [hasUnsavedChanges])

  useEffect(() => {
    if (!mobileNavOpen) return undefined
    const previous = document.body.style.overflow
    document.body.style.overflow = 'hidden'
    return () => { document.body.style.overflow = previous }
  }, [mobileNavOpen])

  const needsSetup = !!health && !health?.ok
  if (needsSetup) {
    return <SetupWizard initialRoot={root} onComplete={(nextCfg) => {
      if (nextCfg?.ga_root) setRoot(nextCfg.ga_root)
      load()
    }} />
  }

  return <>
    {showLLMPicker && <div className="modal-overlay" onClick={() => setShowLLMPicker(false)}>
      <div className="modal-card" onClick={e => e.stopPropagation()}>
        <div className="modal-head"><h3>选择反思模型</h3><button className="modal-close" onClick={() => setShowLLMPicker(false)}>✕</button></div>
        <p className="muted">即将启动反思服务：{pendingServiceName}</p>
        <select value={reflectLLMNo} onChange={e => setReflectLLMNo(e.target.value)}>
          {llms.length ? llms.map(m => <option key={m.index} value={m.index}>{modelLabel(m)}</option>) : <option value="0">未发现模型，使用默认 0</option>}
        </select>
        <div className="modal-actions">
          <button onClick={confirmReflectStart}><Play size={14}/>启动</button>
          <button onClick={() => setShowLLMPicker(false)}>取消</button>
        </div>
      </div>
    </div>}
    <div ref={appScope} className={`app app-tab-${tab}`} aria-busy={busy || versionBusy || undefined}>
    <aside className="sidebar">
      <div className="brand"><Bot aria-hidden="true"/><div><h1>{t.appName}</h1><p>{t.tagline}</p></div></div>
      <div className="lang-switch"><div className="lang-switch-label"><Globe2 size={15} aria-hidden="true"/><span>{t.language}</span></div><div className="lang-options" role="group" aria-label={t.language}><button type="button" aria-pressed={lang === 'zh'} className={lang === 'zh' ? 'active' : ''} onClick={()=>chooseLang('zh')}>中</button><button type="button" aria-pressed={lang === 'en'} className={lang === 'en' ? 'active' : ''} onClick={()=>chooseLang('en')}>EN</button></div><button type="button" className="theme-toggle" onClick={()=>setTheme(theme === 'dark' ? 'light' : 'dark')} aria-label={theme === 'dark' ? '切换到浅色模式' : '切换到深色模式'}>{theme === 'dark' ? <Sun size={15} aria-hidden="true"/> : <Moon size={15} aria-hidden="true"/>}<span>{theme === 'dark' ? '浅色' : '深色'}</span></button></div>
      <button type="button" className="mobile-nav-trigger" onClick={()=>setMobileNavOpen(true)} aria-haspopup="dialog" aria-expanded={mobileNavOpen}><span>{icon(tab)}{t.nav[tab]}</span><ChevronDown size={17}/></button>
      <nav aria-label="主导航">{nav.map(n => <button key={n} type="button" aria-current={tab===n ? 'page' : undefined} className={tab===n?'active':''} onClick={()=>navigateTo(n)}>{icon(n)}{t.nav[n]}</button>)}</nav>
      <button type="button" className="refresh" onClick={refreshApp} disabled={busy} aria-label={busy ? t.busy : t.refresh}><RefreshCw size={15} aria-hidden="true"/><span>{busy ? t.busy : t.refresh}</span></button>
    </aside>
    <main className="main"><header><div><h2>{t.nav[tab]}</h2><p>{t.desc[tab]}</p></div><div className="badges"><span>{cfg?.host}:{cfg?.port}</span><span role="status" aria-live="polite" className={health?.ok?'ok':'err'}>{health?.ok ? t.ready : t.error}</span></div></header>
      <ErrorBoundary resetKey={tab}>
        <Suspense fallback={<RouteFallback label="正在加载页面…" />}>
      {tab==='overview' && <section><div className="stats"><Stat label={t.cards.processes} value={services.length} icon={<Server/>}/><Stat label={t.cards.running} value={services.filter(s=>s.running).length} icon={<Activity/>}/><Stat label={t.cards.schedule} value={schedule.task_count || 0} icon={<CalendarClock/>}/><Stat label={t.cards.enabledTasks} value={schedule.enabled || 0} icon={<CheckCircle2/>}/></div><ObservabilityCard snapshot={observability} error={observabilityError} onRefresh={() => readObservability().catch(e => { setObservability(null); setObservabilityError(e.message) })}/><div className="grid2"><Panel title={t.cards.version}><div className="version-card"><div className="autostart-head"><Download size={18}/><strong>GA Admin {versionInfo?.version || 'dev'}</strong><span className={versionCheck?.update ? 'err' : 'ok'}>{versionCheck ? (versionCheck.update ? '有更新' : '已是最新') : (versionInfo?.goos ? `${versionInfo.goos}/${versionInfo.goarch}` : t.empty)}</span></div><p className="muted">提交 {versionInfo?.commit || '未知'} · {versionInfo?.date || '未知'}</p><p className="muted">运行时 {versionInfo?.runtime || '-'} · 程序 {versionInfo?.exe || '-'}</p>{versionInfo?.update_source_url && <p className="muted">更新源：<a href={versionInfo.update_source_url} target="_blank" rel="noreferrer">{versionInfo.update_repository || versionInfo.update_source_url}</a></p>}{versionInfo && !versionInfo.update_supported && <p className="warn">一键升级不可用：{versionInfo.update_unsupported_reason || '当前平台暂不支持'}</p>}{versionCheck?.latest && <p>最新版本：<a href={versionCheck.latest.html_url} target="_blank" rel="noreferrer">{versionCheck.latest.tag_name}</a></p>}{versionCheck?.asset && <code>{versionCheck.asset.name}</code>}{versionStatus?.stage && <div className="update-progress"><div className="update-progress-head"><span>{versionStatus.running ? '升级中' : (versionStatus.error ? '升级失败' : (versionStatus.stage === 'done' ? '上次升级完成' : '升级状态'))}</span><b>{versionStatus.progress || 0}%</b></div><div className="progress-bar"><span style={{width:`${Math.max(0, Math.min(100, versionStatus.progress || 0))}%`}}/></div><p className={versionStatus.error ? 'err' : 'muted'}>{versionStatus.message || versionStatus.stage}</p>{versionStatus.applied_version && <p className="ok">该次升级已验证版本：{versionStatus.applied_version}</p>}{versionStatus.applied_version && versionInfo?.version && versionStatus.applied_version !== versionInfo.version && <p className="warn">该次升级后程序又被本地编译或替换，当前运行版本为 {versionInfo.version}</p>}<code>{versionStatus.stage}</code></div>}<div className="actions"><button onClick={checkVersion} disabled={versionBusy || versionStatus?.running}>{versionBusy ? t.busy : '检查更新'}</button><button onClick={updateVersion} disabled={versionBusy || versionStatus?.running || !versionCheck?.update}>{versionStatus?.running ? '升级中…' : '一键升级'}</button><button className="secondary" onClick={()=>refreshVersionStatus().catch(e=>setMsg(e.message))}>刷新进度</button></div></div></Panel><Panel title="GA 源代码更新"><div className="version-card"><div className="version-head"><GitPullRequest size={18}/><strong>Git 同步</strong><span className={gitSyncView.state === 'synced' ? 'ok' : (gitSyncView.state === 'error' || gitSyncView.state === 'blocked' ? 'err' : 'warn')}>{gitSyncView.label}</span></div><p className="muted">按 daily_git_pull_merge_push 同步当前 origin：合并远端、提交本地变更并 push；默认不合并 upstream，禁止 force。</p>{gitStatus?.root && <code>{gitStatus.root}</code>}<p>分支: {gitStatus?.branch || '-'}　HEAD: {gitStatus?.commit || gitResult?.after || '-'}</p>{gitStatus?.upstream && <p>跟踪分支: {gitStatus.upstream}　领先 {gitStatus.ahead || 0} / 落后 {gitStatus.behind || 0}</p>}<p className={gitSyncView.state === 'error' || gitSyncView.state === 'blocked' ? 'err' : (gitSyncView.state === 'synced' ? 'ok' : 'warn')}>{gitSyncView.summary}</p>{gitStatus?.dirty && <p className="warn">待提交文件：{gitStatus.changed_files || 0} 个</p>}{gitStatus && !gitStatus.strategy_available && <p className="err">缺少 sche_tasks/git_autosync.py，无法执行同步策略</p>}{gitStatus?.fetch_error && <pre className="mini-log">{gitStatus.fetch_error}</pre>}{gitResult?.sync_output && <pre className="mini-log">{gitResult.sync_output}</pre>}<div className="actions"><button className="secondary" onClick={checkGASource} disabled={gitBusy || busy}>{gitBusy ? t.busy : '检查是否最新'}</button><button onClick={updateGASource} disabled={gitBusy || busy || !gitSyncView.canSync}>{gitBusy ? t.busy : '更新 GA 源代码'}</button></div></div></Panel><Panel title={t.lists.autostart}><div className="autostart-card"><div className="autostart-head"><Power size={18}/><strong>{t.autostart}</strong><span className={autostart?.enabled ? 'ok' : 'muted'}>{autostart?.supported ? (autostart?.enabled ? t.enabled : t.disabled) : t.unsupported}</span></div><p>{!autostart?.supported ? t.hints.autostartUnsupported : (autostart?.enabled ? t.hints.autostartEnabled : t.hints.autostartDisabled)}</p>{autostart?.path && <code>{autostart.path}</code>}<button onClick={toggleAutostart} disabled={busy || !autostart?.supported}>{autostart?.enabled ? t.disableAutostart : t.enableAutostart}</button></div></Panel><Panel title={t.lists.riskHints}><ul className="risk"><li>{t.root}: {root}</li><li>sche_tasks JSON: {t.backup}</li><li>mykey.py: {t.backup}</li></ul></Panel></div></section>}
      {tab==='chat' && <ChatPage t={t} slashCommands={cfg?.slash_commands}/>}
      {tab==='control' && <section>
        <div className="stats">
          <Stat label={t.cards.health} value={health?.ok ? 'OK' : 'FAIL'} icon={<ShieldAlert/>}/>
          <Stat label="GA Version" value={control?.workspace?.version || '-'} icon={<FileCode2/>}/>
          <Stat label={t.cards.capabilities} value={control?.capabilities?.length || 0} icon={<Brain/>}/>
          <Stat label="Logs" value={control?.logs?.items?.length || 0} icon={<Terminal/>}/>
          <Stat label={t.cards.risks} value={control?.risks?.length || 0} icon={<ShieldAlert/>}/>
        </div>
        <div className="grid2">
          <Panel title="GA 工作区">
            <EntryList items={[
              { name: 'Root', path: control?.workspace?.root || root, kind: 'path' },
              { name: 'Python entry', path: control?.workspace?.python?.path || 'agentmain.py', kind: control?.workspace?.python?.exists ? 'ready' : 'missing' },
              { name: 'Memory', path: control?.workspace?.memory?.path || 'memory/global_mem.txt', kind: control?.workspace?.memory?.exists ? 'ready' : 'missing' },
              { name: '集成计划', path: control?.workspace?.plan?.path || 'temp/plan_ga_admin_ga_integration/plan.md', kind: control?.workspace?.plan?.exists ? '就绪' : '缺失' },
            ]} empty={t.empty}/>
          </Panel>
          <Panel title="模型配置">
            <p className="muted">{control?.models?.hint || '除非存在配置文件，否则模型设置由 GA 默认值/记忆管理。'}</p>
            <EntryList items={(control?.models?.files || []).filter(f=>f.exists).map(f=>({ name:f.path, path:`${f.size || 0} bytes`, kind:'配置' }))} empty="No model config file discovered"/>
          </Panel>
          <Panel title={t.lists.readiness}><EntryList items={(control?.readiness || []).map((r,i)=>({name:r.area, path:r.text, kind:r.level}))} empty="正常"/></Panel>
          <Panel title={t.lists.capabilities}><EntryList items={(control?.capabilities || []).map(c=>({name:c.name,path:c.path,kind:c.kind}))} empty={t.empty}/></Panel>
          <Panel title="GitHub 镜像 / 更新辅助" className="git-panel">
            <p className="muted">网络受限时可一键配置全局 git insteadOf：GitHub 地址将走 gh-proxy 镜像；也可随时关闭。</p>
            <div className="actions"><button onClick={() => configureGitMirror(true)} disabled={gitBusy}>启用 GitHub 镜像</button><button onClick={() => configureGitMirror(false)} disabled={gitBusy}>关闭镜像</button></div>
            {gitResult && <pre className="tmwd-paths">{JSON.stringify(gitResult, null, 2)}</pre>}
          </Panel>
          <Panel title="TMWebDriver 监控" className="tmwd-panel"><div className="tmwd-head"><div><b className={tmwdStatus?.ok ? 'ok' : 'err-text'}>{tmwdStatus?.ok ? '基础状态正常' : '需要检查'}</b><p className="muted">{tmwdStatus?.recommendation || tmwdStatus?.error || '检测浏览器进程、18766 master 端口、Python 依赖和 tmwd_cdp_bridge 扩展。'}</p></div><div className="actions"><button onClick={refreshTMWebDriverStatus} disabled={busy}><RefreshCw size={14}/>{t.refresh}</button><button onClick={installTMWebDriverDeps} disabled={busy || !(tmwdStatus?.python_missing?.length > 0)}>安装依赖</button><button onClick={repairTMWebDriver} disabled={busy || tmwdStatus?.port_listening}><Play size={14}/>修复/启动</button></div></div><div className="tmwd-checks">{(tmwdStatus?.checks || []).map(c => <div key={c.name} className={c.ok ? 'status-pill ok' : 'status-pill bad'}><span>{c.ok ? '✓' : '!'}</span><b>{c.name}</b><small>{c.detail}</small></div>)}</div>{tmwdStatus?.python_path && <p className="muted">Python: {tmwdStatus.python_path}</p>}{tmwdStatus?.python_missing?.length > 0 && <div className="tmwd-deps-warning"><b>缺少 TMWebDriver Python 依赖：{tmwdStatus.python_missing.join(', ')}</b>{tmwdStatus?.install_command && <code>{tmwdStatus.install_command}</code>}</div>}{tmwdStatus?.port && <p className="muted">Master port: {tmwdStatus.port}</p>}{tmwdStatus?.extension_paths?.length > 0 && <pre className="tmwd-paths">{tmwdStatus.extension_paths.join(String.fromCharCode(10))}</pre>}</Panel>
          <ProcessGuard/>
          <Panel title="最近日志"><EntryList items={control?.logs?.items || []} empty={t.empty}/></Panel>
          <Panel title={t.lists.recentReports}><EntryList items={control?.reports || []} empty={t.empty}/></Panel>
          <Panel title={t.lists.riskHints}><EntryList items={(control?.risks || []).map(r=>({name:r.area,path:r.text,kind:r.level}))} empty="正常"/></Panel>
        </div>
      </section>}
      {tab==='files' && <FilesPage t={t} browsePath={browsePath} setBrowsePath={setBrowsePath} filePath={filePath} setFilePath={setFilePath} fileList={fileList} fileContent={fileContent} loadedFileContent={loadedFileContent} loadedFilePath={loadedFilePath} setFileContent={setFileContent} fileSearch={fileSearch} setFileSearch={setFileSearch} searchHits={searchHits} tailLines={tailLines} setTailLines={setTailLines} loadFiles={loadFiles} readFile={readFile} tailFile={tailFile} saveFile={saveFile} deleteFile={deleteFile} downloadFile={downloadFile} runSearch={runSearch} clearSearch={()=>{ setFileSearch(''); setSearchHits([]) }} busy={busy}/>}

      {tab==='tasks' && <section className="tasks-page">
        <div className="stats schedule-stats">
          <div className="stat"><Activity/><span>{t.lists.taskServices}</span><b>{taskSvcs.length}</b></div>
          <div className="stat"><CalendarClock/><span>{t.cards.enabledTasks || t.enabled}</span><b>{schedule.enabled || 0}</b></div>
          <div className="stat"><FolderCog/><span>{t.cards.reports || '报告'}</span><b>{schedule.done_count || 0}</b></div>
          <div className="stat"><Target/><span>{t.nav.goals}</span><b>{goals.filter(g=>g.running).length}/{goals.length}</b></div>
          <div className="stat"><ShieldAlert/><span>{t.error}</span><b>{schedule.errors || 0}</b></div>
        </div>

        <div className="subtabs task-subtabs">
          <button className={taskSubTab==='services' ? 'active' : ''} onClick={()=>navigateTaskSubTab('services')}><Server size={14}/>{t.lists.taskServices}</button>
          <button className={taskSubTab==='scheduled' ? 'active' : ''} onClick={()=>navigateTaskSubTab('scheduled')}><CalendarClock size={14}/>{t.lists.scheduledTasks}</button>
          <button className={taskSubTab==='runs' ? 'active' : ''} onClick={()=>navigateTaskSubTab('runs')}><Target size={14}/>{t.nav.goals} / {t.nav.autonomous}</button>
          <button className={taskSubTab==='reports' ? 'active' : ''} onClick={()=>navigateTaskSubTab('reports')}><FolderCog size={14}/>{t.lists.recentReports}</button>
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
            <div className="task-create">
              <div className="task-create-input-row">
                <input value={newTaskId} onChange={e=>setNewTaskId(e.target.value)} placeholder={t.hints.newTaskId}/>
              </div>
              <div className="task-create-btn-row">
                <button onClick={createTask} disabled={busy || !newTaskId.trim()}><FileCode2 size={14}/>{t.create}</button>
                <button onClick={()=>loadScheduleTasks()} disabled={scheduleLoading}><RefreshCw size={14}/>{t.refresh}</button>
                {schedule.log?.exists && <button onClick={()=>readScheduleArtifact('sche_tasks/scheduler.log')}><Terminal size={14}/>{t.nav.logs}</button>}
              </div>
            </div>
            {scheduleError && <p className="err-text">{scheduleError}</p>}
            <div className="task-list clean-list" aria-busy={scheduleLoading}>
              {scheduleLoading
                ? <p className="muted">{t.busy}</p>
                : tasks.length
                  ? tasks.map((task, idx) => <TaskRow key={task.id || task.name || idx} task={task} t={t} onToggle={toggleTask} onEdit={loadTask} onArtifact={readScheduleArtifact}/>)
                  : <p className="muted">{t.hints.noTasks}</p>}
            </div>
          </Panel>
          <Panel title={`${t.lists.editor} · ${taskId || t.empty}`}>
            <div className="editor-mode-toggle">
              <button className={editorMode==='form' ? 'active' : ''} onClick={()=>setEditorMode('form')}><SlidersHorizontal size={14}/> 表单编辑</button>
              <button className={editorMode==='json' ? 'active' : ''} onClick={()=>setEditorMode('json')}><Code2 size={14}/> JSON编辑</button>
            </div>
            <p className="muted">{editorMode==='json' ? t.hints.jsonHelp : '表单编辑定时任务核心字段，修改实时同步到 JSON；保存/删除会生成 .bak 时间戳。'}</p>
            {editorMode==='json'
              ? <textarea className="json-editor compact-editor" value={taskEditor} onChange={e=>setTaskEditor(e.target.value)}/>
              : <TaskFormEditor value={taskEditor} onChange={setTaskEditor}/>}
            <div className="actions">
              <span className={taskDirty ? 'status-pill warn' : 'status-pill ok'}>{taskDirty ? '有未保存更改' : '编辑器已同步'}</span>
              <button onClick={saveTask} disabled={!taskDirty || (!taskId && !newTaskId)}><Save size={14}/>{t.save}</button>
              <button onClick={deleteTask} disabled={!taskId}><XCircle size={14}/>{t.remove}</button>
            </div>
          </Panel>
        </div>}


        {taskSubTab==='runs' && <div className="workspace tasks-workspace">
          <Panel title={`${t.nav.goals} · ${goals.filter(g=>g.running).length}/${goals.length}`}>
            <div className="actions"><button onClick={()=>setTab('goals')}><Target size={14}/>{t.nav.goals}</button><button onClick={loadGoals}><RefreshCw size={14}/>{t.refresh}</button></div>
            <div className="goal-list compact-goals">
              {goals.length
                ? goals.map(g => <button className="goal-row" key={g.id} onClick={()=>{ setTab('goals'); setSelectedGoal(g.id) }}><div><b>{g.objective || g.id}</b><span>{g.status || '-'} · {g.running ? `${t.fields.pid} ${g.pid}` : t.fields.notRunning}</span></div><small>{t.fields.turn} {g.turns_used || 0}/{g.max_turns || '-'}</small></button>)
                : <p className="muted">{t.empty}</p>}
            </div>
          </Panel>
          <Panel title={t.nav.autonomous}>
            <div className="actions"><button onClick={()=>setTab('autonomous')}><Bot size={14}/>{t.nav.autonomous}</button></div>
            <EntryList items={[...(reflectSvcs || []).map(s=>({ name:s.name, path:s.running ? `${t.running}${s.pid ? ` · PID ${s.pid}` : ''}` : t.stopped, kind:s.kind || 'reflect' })), ...((inv.autonomous_reports || []).slice(0, 8).map(r=>({ name:r.name, path:new Date(r.mod_time).toLocaleString(), kind:'report' })))]} empty={t.empty}/>
          </Panel>
        </div>}

        {taskSubTab==='reports' && <div className="workspace tasks-workspace">
          <Panel title={t.lists.recentReports}>
            <div className="report-list clean-list">
              {(schedule.done_recent || []).length
                ? (schedule.done_recent || []).map(r => <button key={r.path} className={scheduleArtifactTitle===r.path ? 'active' : ''} onClick={()=>readScheduleArtifact(r.path)}>{r.name}<small>{new Date(r.mod_time).toLocaleString()}</small></button>)
                : <p className="muted">{t.empty}</p>}
            </div>
          </Panel>
          <Panel title={scheduleArtifactTitle || t.lists.generatedPreview}>
            <pre className="artifact-view">{scheduleArtifact || t.empty}</pre>
          </Panel>
        </div>}
      </section>}
      {tab==='memory' && <section><div className="grid2"><Panel title={t.lists.memory}><EntryList items={[inv.memory?.insight, inv.memory?.facts].filter(Boolean)} empty={t.empty}/></Panel><Panel title={t.lists.sop}><EntryList items={[...(inv.memory?.sops||[]), ...(inv.memory?.utils||[])]} empty={t.empty}/></Panel></div></section>}
      {tab==='channels' && <ChannelsPage frontendSvcs={frontendSvcs} t={t} onStart={n=>serviceAction(n,'start')} onStop={n=>serviceAction(n,'stop')} onLogs={viewServiceLogs} onAutostart={toggleServiceAutostart} onReflectStart={startReflectService}/>}
      {tab==='autonomous' && <section><Panel title={t.lists.reflectServices} className="reflect-services-panel">{reflectSvcs.length ? reflectSvcs.map(s=><ServiceRow key={s.name} svc={s} t={t} llms={llms} onStart={n=>serviceAction(n,'start')} onStop={n=>serviceAction(n,'stop')} onLogs={viewServiceLogs} onAutostart={toggleServiceAutostart} onModel={setServiceModel}/>) : <p className="muted">{t.hints.noReflect}</p>}</Panel><Panel title={t.lists.recentReports}><div className="report-list">{(inv.autonomous_reports || []).map(r=><button key={r.path} className={scheduleArtifactTitle===r.path ? 'active' : ''} onClick={()=>readScheduleArtifact(r.path, 'autonomous')}>{r.name}<small>{new Date(r.mod_time).toLocaleString()}</small></button>)}</div><pre className="artifact-view">{scheduleArtifactTitle?.includes('autonomous_reports') ? (scheduleArtifact || t.empty) : t.empty}</pre></Panel></section>}
      {tab==='goals' && <GoalsPage t={t} goals={goals} objective={goalObjective} setObjective={setGoalObjective} budget={goalBudget} setBudget={setGoalBudget} maxTurns={goalMaxTurns} setMaxTurns={setGoalMaxTurns} llmNo={goalLLMNo} setLLMNo={setGoalLLMNo} llms={llms} hive={goalHive} setHive={setGoalHive} outputBytes={goalOutputBytes} setOutputBytes={setGoalOutputBytes} autoRefresh={goalAutoRefresh} setAutoRefresh={setGoalAutoRefresh} selected={selectedGoal} output={goalOutput} outputMeta={goalOutputMeta} busy={busy} onStart={startGoal} onStop={stopGoal} onDelete={deleteGoal} onRefresh={loadGoals} onOutput={loadGoalOutput} onClearOutput={()=>{ goalOutputSeq.current += 1; setGoalOutput(''); setGoalOutputMeta(null); setMsg(t.hints.goalOutputCleared) }} setMsg={setMsg}/>}
      {tab==='settings' && <SettingsPage t={t} root={root} setRoot={setRoot} config={cfg} setConfig={setCfg} dirty={settingsDirty} busy={busy} onSave={saveConfig} onReset={resetConfigDraft}/>}
      {tab==='models' && <Models t={t} profiles={profiles} persistedProfiles={persistedModelProfiles} setProfiles={setProfiles} patchProfile={patchProfile} addModelProfiles={addModelProfiles} importModels={importModels} previewModels={previewModels} saveModelProfile={saveModelProfile} onSaveModelProfiles={saveModelProfiles} onSaveModelOrder={saveModelOrder} deleteModelProfile={deleteModelProfile} discoverModels={discoverModels} probeModels={probeModels} modelProbeProviders={cfg?.model_probe_providers || []} onSaveModelProbeProviders={saveModelProbeProviders} modelPreview={modelPreview} modelSaveStatus={modelSaveStatus} importLoading={modelImportLoading} riskCatalog={observability?.riskItems || []} riskCatalogError={observabilityError} revealedKeys={modelRevealedKeys} revealBusy={modelKeyBusy} getProfileKey={getModelProfileKey} onRevealKey={revealModelKey} onClearRevealedKey={clearRevealedModelKey}/>}
      {tab==='logs' && <section className="logs-page"><div className="logs-layout"><Panel title={t.lists.processes} className="logs-side"><div className="logs-toolbar"><label>{t.hints.tailLines}<input type="number" min="20" max="2000" value={tailLines} onChange={e=>setTailLines(Number(e.target.value) || 200)}/></label><button disabled={!selected} onClick={()=>loadServiceLogs(selected)}><RefreshCw size={14}/>{t.refresh}</button></div><div className="logs-service-list">{services.map(s => <button className={selected===s.name?'log-service active':'log-service'} key={s.name} onClick={()=>loadServiceLogs(s.name)}><span className={s.running?'dot running':'dot'}></span><span className="log-service-name">{s.name}</span><small>{s.kind}{s.pid ? ` · PID ${s.pid}` : ''}</small></button>)}</div></Panel><Panel title={`Logs · ${selected || '-'}`} className="log-panel"><div className="log-head"><div>{selected && <p className="muted log-command" title={services.find(s=>s.name===selected)?.command?.join(' ')}>{services.find(s=>s.name===selected)?.command?.join(' ')}</p>}<span className="log-count">{logs.length} lines · UTF-8</span></div><div className="actions"><button disabled={!selected || services.find(s=>s.name===selected)?.running} onClick={()=>serviceAction(selected,'start')}><Play size={14}/>{t.start}</button><button disabled={!selected || !services.find(s=>s.name===selected)?.running} onClick={()=>serviceAction(selected,'stop')}><Square size={14}/>{t.stop}</button></div></div><pre className="log-view">{logs.join('\n') || t.hints.noLogs}</pre></Panel></div></section>}        </Suspense>
      </ErrorBoundary>
    </main>
  </div>
      <GlobalFeedback message={msg} onDismiss={dismissMessage} placement={tab === 'chat' ? 'top' : 'bottom'}/>
      {mobileNavOpen && <div className="mobile-nav-backdrop" onMouseDown={event => { if (event.target === event.currentTarget) setMobileNavOpen(false) }}>
        <section className="mobile-nav-sheet" role="dialog" aria-modal="true" aria-label="页面导航">
          <header><div><b>前往功能页面</b><span>{t.nav[tab]}</span></div><button type="button" onClick={()=>setMobileNavOpen(false)} aria-label="关闭导航"><X size={18}/></button></header>
          <div className="mobile-nav-grid">{nav.map(n => <button key={n} type="button" className={tab===n ? 'active' : ''} aria-current={tab===n ? 'page' : undefined} onClick={()=>navigateTo(n)}>{icon(n)}<span>{t.nav[n]}</span></button>)}</div>
        </section>
      </div>}
      </>}



export function ChannelsPage({ frontendSvcs, t, onStart, onStop, onLogs, onAutostart, onReflectStart }) {
  const [config, setConfig] = useState(null)
  const [loading, setLoading] = useState(true)
  const [saving, setSaving] = useState(false)
  const [testing, setTesting] = useState('')
  const [msg, setMsg] = useState('')
  const load = async () => {
    setLoading(true)
    try {
      const d = await api('/api/channels')
      setConfig(d)
      return d
    } catch (e) {
      setMsg(`读取通道配置失败：${e.message}`)
      return null
    } finally { setLoading(false) }
  }
  useEffect(() => { load() }, [])
  const patchField = (profileId, fieldName, value) => {
    setConfig(prev => ({
      ...(prev || {}),
      profiles: (prev?.profiles || []).map(p => p.id === profileId ? {
        ...p,
        fields: (p.fields || []).map(f => f.name === fieldName ? { ...f, value } : f)
      } : p)
    }))
  }
  const save = async () => {
    if (!confirmDanger('channels-save', '写入通道配置会更新 GA mykey.py；Secret 留空将保留原值。确认继续？')) return
    setSaving(true); setMsg('正在写入 GA mykey.py…')
    try {
      const d = await api('/api/channels', { dangerous:true, method:'PUT', body: JSON.stringify({ profiles: config?.profiles || [] }) })
      setConfig(d)
      setMsg(`已保存通道配置：${d.path}`)
    } catch (e) { setMsg(`保存失败：${e.message}`) } finally { setSaving(false) }
  }
  const testProfile = async (profile) => {
    setTesting(profile.id); setMsg(`正在测试 ${profile.name} 凭据…`)
    try {
      const d = await api('/api/channels/test', { method:'POST', body: JSON.stringify({ profile_id: profile.id, fields: profile.fields || [] }) })
      setMsg(`${profile.name}：${d.ok ? '测试通过' : '测试失败'} · ${d.message || ''}`)
    } catch (e) { setMsg(`${profile.name} 测试失败：${e.message}`) } finally { setTesting('') }
  }
  const runningCount = frontendSvcs.filter(s => s.running).length
  return <section className="channels-page">
    <div className="channel-hero">
      <div>
        <span className="eyebrow">Channels</span>
        <h2>通道与前端服务</h2>
        <p>集中管理 GA 的模型通道密钥和上层前端进程。Secret 不回显，留空会保留现有值。</p>
      </div>
      <div className="channel-hero-stats">
        <span><b>{frontendSvcs.length}</b> 服务</span>
        <span><b>{runningCount}</b> 运行中</span>
      </div>
    </div>
    <div className="channels-layout">
      <Panel title="密钥配置" className="channels-panel channel-key-panel">
        <div className="channel-toolbar">
          <div>{config?.path ? <span>配置文件：<code>{config.path}</code></span> : <span>{loading ? '正在读取配置…' : '未读取到配置路径'}</span>}</div>
          <div className="actions"><button onClick={load} disabled={loading || saving}>{loading ? '刷新中…' : t.refresh}</button><button onClick={save} disabled={saving || loading || !config}>{saving ? t.busy : t.save}</button></div>
        </div>
        {msg && <p className={msg.includes('失败') ? 'err channel-message' : 'ok channel-message'}>{msg}</p>}
        <div className="channel-config-list">
          {(config?.profiles || []).map(profile => <article className="channel-config-card" key={profile.id}>
            <div className="channel-config-head">
              <div><h3>{profile.name}</h3><p>{profile.description}</p></div>
              <button onClick={()=>testProfile(profile)} disabled={saving || testing === profile.id}>{testing === profile.id ? '测试中…' : '测试连接'}</button>
            </div>
            <div className="channel-fields">
              {(profile.fields || []).map(field => <label key={field.name}>
                <span>{field.label || field.name}<small>{field.name}{field.secret && field.has_value ? ' · 已保存' : ''}</small></span>
                {field.secret
                  ? <SecretInput value={field.value || ''} onChange={v=>patchField(profile.id, field.name, v)} t={t}/>
                  : field.type === 'bool'
                    ? <select value={String(field.value || 'false').toLowerCase()} onChange={e=>patchField(profile.id, field.name, e.target.value)}><option value="false">False</option><option value="true">True</option></select>
                    : <input value={field.value || ''} placeholder={field.placeholder || ''} onChange={e=>patchField(profile.id, field.name, e.target.value)}/>}
              </label>)}
            </div>
          </article>)}
          {!loading && !config?.profiles?.length && <p className="empty-cell">{t.empty}</p>}
        </div>
      </Panel>
      <Panel title={t.lists.frontendServices} className="channels-panel channel-services-panel">
        <p className="muted">{t.desc.channels}</p>
        <ChannelServiceTable services={frontendSvcs} t={t} onStart={onStart} onStop={onStop} onLogs={onLogs} onAutostart={onAutostart} onReflectStart={onReflectStart}/>
      </Panel>
    </div>
  </section>
}

function icon(n) { const m = { overview:<Activity size={16}/>, chat:<MessageSquare size={16}/>, control:<ShieldAlert size={16}/>, files:<FileCode2 size={16}/>, tasks:<Terminal size={16}/>, memory:<Brain size={16}/>, channels:<Globe2 size={16}/>, autonomous:<Bot size={16}/>, schedule:<CalendarClock size={16}/>, goals:<Target size={16}/>, models:<SlidersHorizontal size={16}/>, settings:<FolderCog size={16}/>, logs:<FolderCog size={16}/> }; return m[n] }
