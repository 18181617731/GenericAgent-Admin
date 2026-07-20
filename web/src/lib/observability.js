export const READ_ONLY_OBSERVABILITY_ENDPOINTS = Object.freeze([
  '/api/health',
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
  const inv = inventory || health?.inventory || {}
  const memory = inv.memory || {}
  const checks = objectEntries(health?.checks).map(([name, state]) => ({ name, state }))
  const riskItems = list(risks?.items || risks)
  const writeRiskItems = riskItems.filter(item => item?.level === 'dangerous' || /write|delete|install|pull|save|stop|start/i.test(`${item?.action || ''} ${item?.reason || ''}`))
  const coreFiles = list(inv.core_files)
  const missingCore = coreFiles.filter(item => !item?.exists)
  return {
    ok: !!health?.ok,
    root: health?.root || inv.root || '',
    generatedAt: health?.generated_at || inv.generated_at || '',
    checks,
    errors: list(health?.errors),
    warnings: list(health?.warnings),
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
  const healthy = snapshot.ok === true && hasCoreFiles && missingCore === 0
  const incomplete = snapshot.ok === true && !hasCoreFiles
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
      { key: 'core', label: '核心文件', value: coreFiles.length ? `${readyCore}/${coreFiles.length}` : '未检查', detail: missingCore ? `${missingCore} 项缺失` : '已就绪' },
      { key: 'knowledge', label: '知识与 SOP', value: `${list(snapshot.memory?.sops).length} 项`, detail: '可用文档' },
      { key: 'protection', label: '操作保护', value: `${list(snapshot.riskItems).length} 项`, detail: '重要操作需确认' },
    ],
  }
}
