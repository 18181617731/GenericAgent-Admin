const IDENTIFIER_RE = /^[A-Za-z_][A-Za-z0-9_]*$/

const text = (value) => String(value ?? '').trim()
const numberValue = (value) => Number(value)

export function validateModelProfiles(profiles = []) {
  const counts = new Map()
  for (const profile of profiles || []) {
    const name = text(profile?.var_name)
    if (name) counts.set(name, (counts.get(name) || 0) + 1)
  }

  return (profiles || []).map((profile = {}, idx) => {
    const errors = []
    const warnings = []
    const varName = text(profile.var_name)
    const name = text(profile.name)
    const model = text(profile.model)
    const apiBase = text(profile.apibase)
    const maxRetries = numberValue(profile.max_retries ?? 3)
    const readTimeout = numberValue(profile.read_timeout ?? 300)

    if (!varName) errors.push('varNameRequired')
    else if (!IDENTIFIER_RE.test(varName)) errors.push('varNameInvalid')
    // Mirrors internal/modelconfig.Validate: var_name must contain api/config/cookie.
    else if (!/(api|config|cookie)/i.test(varName)) errors.push('varNameDiscoveryToken')
    else if ((counts.get(varName) || 0) > 1) errors.push('varNameDuplicate')

    if (!name) errors.push('nameRequired')
    if (!model) errors.push('modelRequired')
    if (!apiBase) errors.push('apiBaseRequired')
    else if (!/^https?:\/\//i.test(apiBase)) warnings.push('apiBaseProtocol')
    if (!Number.isFinite(maxRetries) || maxRetries < 0) errors.push('maxRetriesInvalid')
    if (!Number.isFinite(readTimeout) || readTimeout <= 0) errors.push('readTimeoutInvalid')
    if (!text(profile.apikey)) warnings.push('apiKeyEmpty')

    return { idx, errors, warnings, ok: errors.length === 0 }
  })
}

export function modelValidationSummary(results = []) {
  const errors = results.reduce((sum, item) => sum + (item.errors?.length || 0), 0)
  const warnings = results.reduce((sum, item) => sum + (item.warnings?.length || 0), 0)
  return { errors, warnings, ready: results.length > 0 && errors === 0 }
}


const MODEL_ROUTE_RE = /^\/api\/models(?:\/|$)/

const asList = (value) => Array.isArray(value) ? value : []
const textOf = (value) => String(value ?? '').trim()

export function modelRiskCatalog(catalog, error = '') {
  const items = asList(catalog?.items || catalog)
    .filter(item => MODEL_ROUTE_RE.test(textOf(item?.route || item?.path || item?.endpoint || item?.url)))
    .map(item => ({
      route: textOf(item.route || item.path || item.endpoint || item.url),
      method: textOf(item.method || 'GET').toUpperCase(),
      level: textOf(item.level || item.risk || 'review'),
      action: textOf(item.action || item.name || item.summary),
      reason: textOf(item.reason || item.description || item.note),
    }))

  const writeRoutes = new Set(['/api/models/export', '/api/models/import-mykey'])
  const confirmedRoutes = new Set(items.filter(item => item.level === 'dangerous' || /confirm|danger/i.test(`${item.action} ${item.reason}`)).map(item => item.route))
  return {
    status: error ? 'error' : (items.length ? 'ready' : 'empty'),
    error: textOf(error),
    items,
    writeRoutes: [...writeRoutes],
    confirmedWriteRoutes: [...writeRoutes].filter(route => confirmedRoutes.has(route)),
    missingConfirmedWriteRoutes: [...writeRoutes].filter(route => !confirmedRoutes.has(route)),
  }
}
