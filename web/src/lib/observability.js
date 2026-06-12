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
