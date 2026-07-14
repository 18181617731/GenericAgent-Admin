const text = value => String(value ?? '').trim()

const OFFICIAL_PROVIDER_PREFIXES = [
  'native_oai_config',
  'native_claude_config',
  'oai_config',
  'claude_config',
]

export function providerDisplayName(varName) {
  const value = text(varName)
  const prefix = OFFICIAL_PROVIDER_PREFIXES.find(item => value.startsWith(item))
  if (!prefix) return value
  return value.slice(prefix.length).replace(/^_/, '')
}

export function providerVarNameFromDisplayName(displayName, prefix, currentVarName = '') {
  const name = text(displayName)
  const current = text(currentVarName)
  const currentPrefix = OFFICIAL_PROVIDER_PREFIXES.find(item => current.startsWith(item))
  if (!currentPrefix) return name

  const nextPrefix = text(prefix) || currentPrefix
  if (!name) return nextPrefix
  const suffix = current.slice(currentPrefix.length)
  const separator = !suffix || suffix.startsWith('_') ? '_' : ''
  return `${nextPrefix}${separator}${name}`
}

export function nextProviderVarName(prefix, profiles = []) {
  const safePrefix = text(prefix) || 'native_oai_config'
  const used = new Set((profiles || []).map(profile => text(profile?.var_name)).filter(Boolean))
  let index = Math.max(1, (profiles || []).length + 1)
  while (used.has(`${safePrefix}${index}`)) index += 1
  return `${safePrefix}${index}`
}

export function suggestedProviderVarOnProtocolChange(current, previousSuggestion, nextSuggestion) {
  const value = text(current)
  if (!value || value === text(previousSuggestion)) return text(nextSuggestion)
  return value
}

export function providerVarNameOnProtocolChange(current, nextPrefix, profiles = [], currentIndex = -1) {
  const value = text(current)
  const previousPrefix = OFFICIAL_PROVIDER_PREFIXES.find(prefix => value.startsWith(prefix))
  if (!previousPrefix) return value

  const candidate = `${text(nextPrefix)}${value.slice(previousPrefix.length)}`
  const used = new Set(
    (profiles || [])
      .filter((_, index) => index !== currentIndex)
      .map(profile => text(profile?.var_name))
      .filter(Boolean),
  )
  if (!used.has(candidate)) return candidate

  let suffix = 2
  while (used.has(`${candidate}_${suffix}`)) suffix += 1
  return `${candidate}_${suffix}`
}
