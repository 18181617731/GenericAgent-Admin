import { profileModelConfigs, reconcileModelProbeResults } from './modelsEditor.js'

export const modelProbeProviderKey = (profile, index) => String(
  profile?.var_name || profile?.client_id || `provider-${index}`,
).trim()

export const modelProbeProviderName = (profile, index) => String(
  profile?.name || profile?.var_name || `服务商 ${index + 1}`,
).trim()

export function normalizeModelProbeProviderKeys(values = []) {
  return Array.from(new Set(values.map(value => String(value || '').trim()).filter(Boolean)))
}

export function resolveModelProbeTargets(profiles = [], configuredKeys = []) {
  const selected = new Set(normalizeModelProbeProviderKeys(configuredKeys))
  const all = selected.size === 0
  return profiles.map((profile, index) => ({
    profile,
    index,
    key: modelProbeProviderKey(profile, index),
    name: modelProbeProviderName(profile, index),
  })).filter(item => all || selected.has(item.key))
}

function isMaskedSecret(value) {
  const secret = String(value || '').trim()
  return /^\*{4,}$/.test(secret) || /\*{2,}/.test(secret)
}

function modelProbeInput(profile) {
  const configs = profileModelConfigs(profile)
  const configuredKey = String(profile?.apikey || '').trim()
  return {
    protocol: profile?.type || 'native_oai',
    baseUrl: profile?.apibase,
    apiKey: configuredKey && !isMaskedSecret(configuredKey) ? configuredKey : undefined,
    varName: profile?.var_name,
    models: configs.map(config => config.model),
    modelOptions: Object.fromEntries(configs.map(config => [config.model, {
      api_mode: config.api_mode,
      user_agent: config.user_agent,
    }])),
  }
}

function summarizeModelProbeResults(results) {
  return results.reduce((summary, result) => {
    summary.completed += 1
    if (result.error) summary.failedProviders += 1
    else {
      summary.successfulProviders += 1
      summary.available += result.summary.available
      summary.unavailable += result.summary.unavailable
      summary.disabled += result.summary.disabled
      summary.restored += result.summary.restored
    }
    return summary
  }, { completed: 0, successfulProviders: 0, failedProviders: 0, available: 0, unavailable: 0, disabled: 0, restored: 0 })
}

async function probeProvider(target, probeModels) {
  const response = await probeModels(modelProbeInput(target.profile))
  const probeResults = Array.isArray(response?.results) ? response.results : []
  if (!probeResults.length) throw new Error('未获得任何真实对话检测结果')
  const reconciled = reconcileModelProbeResults(target.profile, probeResults, response.checked_at)
  return { ...target, profile: reconciled.profile, summary: reconciled.summary, probeResults }
}

export async function runModelBatchProbe({ profiles = [], configuredKeys = [], probeModels, onProgress }) {
  const targets = resolveModelProbeTargets(profiles, configuredKeys)
  const nextProfiles = profiles.map(profile => ({ ...profile }))
  const results = []

  for (const target of targets) {
    onProgress?.({ completed: results.length, total: targets.length, current: target.name })
    let result
    try {
      result = await probeProvider(target, probeModels)
      nextProfiles[target.index] = result.profile
    } catch (error) {
      result = { ...target, error: String(error?.message || error) }
    }
    results.push(result)
    onProgress?.({ completed: results.length, total: targets.length, current: targets[results.length]?.name || target.name })
  }

  return { profiles: nextProfiles, results, summary: summarizeModelProbeResults(results) }
}
