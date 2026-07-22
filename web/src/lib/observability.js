export const READ_ONLY_OBSERVABILITY_ENDPOINTS = Object.freeze([
  '/api/health',
  '/api/ga/health',
  '/api/ga/inventory',
  '/api/risk/catalog',
])

export const observabilityRequest = (endpoint) => {
  if (!READ_ONLY_OBSERVABILITY_ENDPOINTS.includes(endpoint)) throw new Error(`unsupported observability endpoint: ${endpoint}`)
  return { url: endpoint, options: { method: 'GET' } }
}

const list = (value) => Array.isArray(value) ? value : []
const objectEntries = (value) => value && typeof value === 'object' && !Array.isArray(value) ? Object.entries(value) : []

export const buildObservabilitySnapshot = ({ health, inventory, risks } = {}) => {
  const healthState = health?.health || health || {}
  const inv = inventory || healthState?.inventory || {}
  const memory = inv.memory || {}
  const checks = objectEntries(healthState?.checks).map(([name, state]) => ({ name, state }))
  const riskItems = list(risks?.items || risks)
  const writeRiskItems = riskItems.filter(item => item?.level === 'dangerous' || /write|delete|install|pull|save|stop|start/i.test(`${item?.action || ''} ${item?.reason || ''}`))
  const coreFiles = list(inv.core_files)
  const missingCore = coreFiles.filter(item => !item?.exists)
  return {
    ok: !!healthState?.ok,
    root: healthState?.root || inv.root || '',
    generatedAt: healthState?.generated_at || inv.generated_at || '',
    checks,
    errors: list(healthState?.errors),
    warnings: list(healthState?.warnings),
    runtime: normalizeRuntime(healthState?.runtime),
    coreFiles,
    missingCore,
    tools: list(inv.tools),
    frontends: list(inv.frontends),
    reflect: list(inv.reflect),
    memory: {
      sops: list(memory.sops),
      utils: list(memory.utils),
      rawSessions: list(memory.raw_sessions),
      insight: memory.insight || null,
      facts: memory.facts || null,
    },
    riskItems,
    writeRiskItems,
  }
}

const normalizeRuntime = (runtime) => {
  if (!runtime || typeof runtime !== 'object' || Array.isArray(runtime)) return null
  const dependencies = list(runtime.dependencies)
  return {
    ok: runtime.ok === true,
    pythonOK: runtime.python_ok === true,
    pythonPath: runtime.python_path || '',
    pythonVersion: runtime.python_version || '',
    dependencies,
    missingModules: list(runtime.missing_modules),
    agentmainOK: runtime.agentmain_ok === true,
    agentmainError: runtime.agentmain_error || '',
    ultraplanOK: runtime.ultraplan_ok === true,
    ultraplanMissing: list(runtime.ultraplan_missing),
    ultraplanError: runtime.ultraplan_error || '',
    legacyUltraplanScripts: list(runtime.legacy_ultraplan_scripts),
    repairable: runtime.repairable === true,
    probeError: runtime.probe_error || '',
    durationMS: Number(runtime.duration_ms) || 0,
  }
}

export const observabilityStats = (snapshot = {}) => [
  { label: 'Health checks', value: list(snapshot.checks).length },
  { label: 'Core files', value: list(snapshot.coreFiles).filter(item => item?.exists).length },
  { label: 'Memory SOPs', value: list(snapshot.memory?.sops).length },
  { label: 'Risk rules', value: list(snapshot.riskItems).length },
]

export const observabilitySummary = (snapshot) => {
  if (!snapshot) {
    return {
      status: '检查中',
      tone: 'pending',
      detail: '正在读取本机 GenericAgent 状态',
      stats: [],
    }
  }
  const coreFiles = list(snapshot.coreFiles)
  const readyCore = coreFiles.filter(item => item?.exists).length
  const missingCore = coreFiles.length - readyCore
  const warningCount = list(snapshot.warnings).length
  const errorCount = list(snapshot.errors).length
  const hasCoreFiles = coreFiles.length > 0
  const runtime = snapshot.runtime
  const dependencies = list(runtime?.dependencies)
  const healthy = snapshot.ok === true && hasCoreFiles && missingCore === 0 && runtime?.ok === true
  const incomplete = !hasCoreFiles || !runtime
  const status = healthy ? '正常' : (incomplete ? '待检查' : '需处理')
  const detail = healthy
    ? (warningCount ? `基础检查通过，另有 ${warningCount} 项提醒` : '基础检查通过，可以继续使用')
    : (incomplete ? '尚未取得核心文件清单，请刷新后确认' : `发现 ${Math.max(1, errorCount + missingCore)} 项需要处理的问题`)
  return {
    status,
    tone: healthy ? 'ok' : (incomplete ? 'pending' : 'warn'),
    detail,
    stats: [
      { key: 'status', label: '系统状态', value: status, detail },
      { key: 'python', label: '实际 Python', value: runtime ? (runtime.pythonOK ? (runtime.pythonVersion || '可用') : '不可用') : '未检查', detail: runtime?.pythonPath || runtime?.probeError || '等待运行时检查' },
      { key: 'dependencies', label: '核心依赖', value: runtime ? `${dependencies.filter(item => item?.ok).length}/${dependencies.length}` : '未检查', detail: runtime?.missingModules?.length ? `缺少 ${runtime.missingModules.join(', ')}` : '已安装' },
      { key: 'runtime', label: 'GA 运行检查', value: runtime ? (runtime.agentmainOK && runtime.ultraplanOK ? '通过' : '失败') : '未检查', detail: runtime ? `主程序 ${runtime.agentmainOK ? '正常' : '异常'} · UltraPlan ${runtime.ultraplanOK ? '正常' : '异常'}` : '等待实际导入' },
    ],
  }
}
