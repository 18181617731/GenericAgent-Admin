const text = value => String(value ?? '').trim()

const DEFAULT_MODEL_PROTOCOL = 'native_oai'
const MODEL_PROTOCOL_FIELDS = {
  native_oai: { apiMode: true, reasoningFamily: 'oai', userAgent: true },
  native_claude: { thinkingType: true, reasoningFamily: 'claude', userAgent: true, fakeClaudeCode: true },
  oai: { apiMode: true, reasoningFamily: 'oai', userAgent: true },
  claude: { thinkingType: true, reasoningFamily: 'claude' },
}

export const API_MODE_OPTIONS = ['chat_completions', 'responses'].map(value => ({ value, label: value }))
export const THINKING_TYPE_OPTIONS = ['adaptive', 'enabled', 'disabled'].map(value => ({ value, label: value }))

const REASONING_EFFORT_OPTIONS = {
  oai: ['none', 'minimal', 'low', 'medium', 'high', 'xhigh', 'max'].map(value => ({ value, label: value })),
  claude: ['low', 'medium', 'high', 'xhigh'].map(value => ({ value, label: value })),
}

export const modelProtocolFields = protocol => (
  MODEL_PROTOCOL_FIELDS[protocol] || MODEL_PROTOCOL_FIELDS[DEFAULT_MODEL_PROTOCOL]
)

export const reasoningEffortOptions = protocol => (
  REASONING_EFFORT_OPTIONS[modelProtocolFields(protocol).reasoningFamily] || []
)

const MODEL_SETTING_KEYS = [
  'enabled',
  'auto_disabled',
  'availability',
  'availability_checked_at',
  'availability_detail',
  'availability_latency_ms',
  'stream',
  'max_retries',
  'read_timeout',
  'connect_timeout',
  'user_agent',
  'api_mode',
  'thinking_type',
  'reasoning_effort',
  'fake_cc_system_prompt',
  'extra',
]

const modelIdOf = value => text(
  typeof value === 'string'
    ? value
    : value?.id || value?.model || value?.name,
)

const uniqueModelIds = values => {
  const seen = new Set()
  return (values || []).map(modelIdOf).filter(model => {
    if (!model || seen.has(model)) return false
    seen.add(model)
    return true
  })
}

const copyPresentSettings = source => {
  const settings = {}
  for (const key of MODEL_SETTING_KEYS) {
    if (source?.[key] !== undefined) settings[key] = source[key]
  }
  return settings
}

export const createModelConfig = (model, settings = {}) => ({
  model: modelIdOf(model),
  stream: true,
  max_retries: 3,
  read_timeout: 300,
  ...copyPresentSettings(settings),
})

export const profileModelConfigs = (profile = {}) => {
  if (Array.isArray(profile.model_configs) && profile.model_configs.length) {
    return profile.model_configs.map(config => ({
      ...config,
      model: modelIdOf(config),
    }))
  }

  const models = uniqueModelIds([
    ...(Array.isArray(profile.models) ? profile.models : []),
    profile.model,
  ])
  const settings = copyPresentSettings(profile)
  return models.map(model => ({ model, ...settings }))
}

export const isModelConfigEnabled = config => config?.enabled !== false

export const modelAvailabilitySummary = (profile = {}) => {
  const configs = profileModelConfigs(profile)
  const checked = configs.filter(config => !!config.availability_checked_at)
  return {
    total: configs.length,
    enabled: configs.filter(isModelConfigEnabled).length,
    disabled: configs.filter(config => !isModelConfigEnabled(config)).length,
    unavailable: configs.filter(config => config.availability === 'unavailable').length,
    checked: checked.length,
    checkedAt: checked.map(config => config.availability_checked_at).sort().at(-1) || '',
  }
}

export const withModelConfigs = (profile = {}, configs = []) => {
  const normalized = (configs || []).map(config => ({
    ...config,
    model: modelIdOf(config),
  }))
  const models = normalized.map(config => config.model).filter(Boolean)
  return {
    ...profile,
    model: models[0] || '',
    models,
    model_configs: normalized,
  }
}

export const reconcileModelAvailability = (profile = {}, values = [], checkedAt = new Date().toISOString()) => {
  const availableModels = new Set(uniqueModelIds(values))
  const results = profileModelConfigs(profile).map(config => ({
    id: modelIdOf(config),
    available: availableModels.has(modelIdOf(config)),
    status: availableModels.has(modelIdOf(config)) ? 'available' : 'unavailable',
  }))
  return reconcileModelProbeResults(profile, results, checkedAt)
}

export const reconcileModelProbeResults = (profile = {}, values = [], checkedAt = new Date().toISOString()) => {
  const results = new Map((values || []).map(value => [modelIdOf(value?.id ?? value?.model), value]))
  let disabled = 0
  let restored = 0
  let unavailable = 0
  const configs = profileModelConfigs(profile).map(config => {
    const probe = results.get(modelIdOf(config))
    if (probe?.available === true) {
      const restore = config.auto_disabled === true
      if (restore) restored += 1
      return {
        ...config,
        enabled: restore ? true : config.enabled,
        auto_disabled: restore ? false : config.auto_disabled,
        availability: 'available',
        availability_checked_at: checkedAt,
        availability_detail: probe.detail || '',
        availability_latency_ms: Number(probe.latency_ms) || 0,
      }
    }
    unavailable += 1
    const autoDisable = isModelConfigEnabled(config) || config.auto_disabled === true
    if (isModelConfigEnabled(config)) disabled += 1
    return {
      ...config,
      enabled: autoDisable ? false : config.enabled,
      auto_disabled: autoDisable,
      availability: 'unavailable',
      availability_checked_at: checkedAt,
      availability_detail: probe?.detail || '未获得真实对话验证结果',
      availability_latency_ms: Number(probe?.latency_ms) || 0,
    }
  })
  return {
    profile: withModelConfigs(profile, configs),
    summary: { available: configs.length - unavailable, unavailable, disabled, restored, checkedAt },
  }
}

export const addModelConfigs = (profile = {}, values = []) => {
  const configs = profileModelConfigs(profile)
  const seen = new Set(configs.map(config => modelIdOf(config)))
  for (const model of uniqueModelIds(values)) {
    if (seen.has(model)) continue
    seen.add(model)
    configs.push(createModelConfig(model))
  }
  return withModelConfigs(profile, configs)
}

export const updateModelConfig = (profile = {}, index, patch) => {
  const configs = profileModelConfigs(profile)
  if (index < 0 || index >= configs.length) return withModelConfigs(profile, configs)
  configs[index] = { ...configs[index], ...patch }
  return withModelConfigs(profile, configs)
}

export const removeModelConfig = (profile = {}, index) => withModelConfigs(
  profile,
  profileModelConfigs(profile).filter((_, rowIndex) => rowIndex !== index),
)

export const orderedProviderProfiles = (profiles = []) => profiles
  .map((profile, index) => ({ profile, index, order: Number.isInteger(profile?.provider_sort_order) ? profile.provider_sort_order : index }))
  .sort((left, right) => left.order - right.order || left.index - right.index)
  .map(item => item.profile)

export const applyProviderOrder = (profiles = []) => profiles.map((profile, index) => ({
  ...profile,
  provider_sort_order: index,
}))

export const orderedModelRows = (profiles = []) => {
  let defaultOrder = 0
  const rows = profiles.flatMap((profile, profileIndex) => (
    profileModelConfigs(profile).map((config, configIndex) => {
      const row = {
        id: `${profileIndex}:${configIndex}`,
        profileIndex,
        configIndex,
        model: text(config.model),
        providerName: text(profile.name),
        providerVarName: text(profile.var_name),
        variableName: `${text(profile.var_name)}${configIndex ? `_${configIndex + 1}` : ''}`,
        order: Number.isInteger(config.sort_order) ? config.sort_order : defaultOrder,
        defaultOrder,
      }
      defaultOrder += 1
      return row
    })
  ))
  return rows.sort((left, right) => left.order - right.order)
}

export const orderedModelAndFailoverRows = (profiles = [], failoverGroups = []) => {
  let defaultOrder = 0
  
  // Model rows
  const modelRows = profiles.flatMap((profile, profileIndex) => (
    profileModelConfigs(profile).map((config, configIndex) => {
      const row = {
        type: 'model',
        id: `${profileIndex}:${configIndex}`,
        profileIndex,
        configIndex,
        model: text(config.model),
        providerVarName: text(profile.var_name),
        variableName: `${text(profile.var_name)}${configIndex ? `_${configIndex + 1}` : ''}`,
        order: Number.isInteger(config.sort_order) ? config.sort_order : defaultOrder,
        defaultOrder,
      }
      defaultOrder += 1
      return row
    })
  ))
  
  // Failover group rows - place at the beginning by default (negative order)
  const failoverRows = (Array.isArray(failoverGroups) ? failoverGroups : []).map((group, groupIndex) => ({
    type: 'failover',
    id: `failover:${groupIndex}`,
    groupIndex,
    varName: text(group.var_name),
    members: group.members || [],
    order: Number.isInteger(group.sort_order) ? group.sort_order : -(failoverGroups.length - groupIndex),
    defaultOrder: -(failoverGroups.length - groupIndex),
  }))
  
  return [...modelRows, ...failoverRows].sort((left, right) => left.order - right.order)
}

export const mergePersistedModelOrder = (draftProfiles = [], persistedProfiles = []) => {
  const persistedOrders = new Map()
  let persistedCount = 0

  persistedProfiles.forEach((profile, profileIndex) => {
    profileModelConfigs(profile).forEach(config => {
      const key = `${profileIndex}\0${modelIdOf(config)}`
      const orders = persistedOrders.get(key) || []
      orders.push(Number.isInteger(config.sort_order) ? config.sort_order : persistedCount)
      persistedOrders.set(key, orders)
      persistedCount += 1
    })
  })

  const consumed = new Map()
  let draftOnlyOrder = persistedCount
  return draftProfiles.map((profile, profileIndex) => withModelConfigs(
    profile,
    profileModelConfigs(profile).map(config => {
      const key = `${profileIndex}\0${modelIdOf(config)}`
      const occurrence = consumed.get(key) || 0
      const persistedOrder = persistedOrders.get(key)?.[occurrence]
      consumed.set(key, occurrence + 1)
      return {
        ...config,
        sort_order: persistedOrder === undefined ? draftOnlyOrder++ : persistedOrder,
      }
    }),
  ))
}

export const FAILOVER_VAR_PREFIX = 'mixin_config_'

export const failoverGroupSuffix = value => {
  const name = String(value ?? '')
  return name.startsWith(FAILOVER_VAR_PREFIX) ? name.slice(FAILOVER_VAR_PREFIX.length) : name
}

export const failoverGroupVarName = suffix => `${FAILOVER_VAR_PREFIX}${String(suffix ?? '')}`

const legacyFailoverSuffix = (value, fallback) => {
  const suffix = text(value)
    .replace(/[^A-Za-z0-9_]+/g, '_')
    .replace(/^_+|_+$/g, '')
  return suffix || fallback
}

export const migrateFailoverGroupNames = (groups = []) => {
  const list = Array.isArray(groups) ? groups : []
  const reserved = new Set(list
    .map(group => text(group?.var_name))
    .filter(name => name.startsWith(FAILOVER_VAR_PREFIX)))
  const used = new Set()

  return list.map((group, index) => {
    const original = text(group?.var_name)
    if (original.startsWith(FAILOVER_VAR_PREFIX)) {
      used.add(original)
      return { ...group, var_name: original }
    }

    const suffix = legacyFailoverSuffix(original, String(index + 1))
    let varName = failoverGroupVarName(suffix)
    let serial = 2
    while (reserved.has(varName) || used.has(varName)) {
      varName = failoverGroupVarName(`${suffix}_${serial}`)
      serial += 1
    }
    used.add(varName)
    return { ...group, var_name: varName }
  })
}

export const normalizeFailoverGroups = (groups = []) => (Array.isArray(groups) ? groups : []).map(group => {
  const next = {
    var_name: text(group?.var_name),
    members: (Array.isArray(group?.members) ? group.members : []).map(member => ({
      provider_var_name: text(member?.provider_var_name),
      model: text(member?.model),
    })),
    max_retries: Number(group?.max_retries ?? 10),
    base_delay: Number(group?.base_delay ?? 0.5),
  }
  if (group?.spring_back !== undefined && group?.spring_back !== null && group?.spring_back !== '') {
    next.spring_back = Number(group.spring_back)
  }
  if (group?.sort_order !== undefined && group?.sort_order !== null) {
    next.sort_order = Number(group.sort_order)
  }
  return next
})

export const nextFailoverGroupName = (groups = []) => {
  const used = new Set((Array.isArray(groups) ? groups : []).map(group => text(group?.var_name)))
  let suffix = 1
  while (used.has(failoverGroupVarName(suffix))) suffix += 1
  return failoverGroupVarName(suffix)
}

export const remapFailoverGroupReferences = (groups = [], changes = []) => {
  const mappings = (Array.isArray(changes) ? changes : [])
    .map(change => ({
      fromProvider: text(change?.from_provider_var_name),
      fromModel: text(change?.from_model),
      toProvider: text(change?.to_provider_var_name),
      toModel: text(change?.to_model),
    }))
    .filter(change => change.fromProvider && change.fromModel && change.toProvider && change.toModel)
  if (!mappings.length) return normalizeFailoverGroups(groups)
  return normalizeFailoverGroups(groups).map(group => ({
    ...group,
    members: group.members.map(member => {
      const change = mappings.find(candidate => (
        candidate.fromProvider === member.provider_var_name && candidate.fromModel === member.model
      ))
      return change ? { provider_var_name: change.toProvider, model: change.toModel } : member
    }),
  }))
}

const FAILOVER_FIELDS = [
  'failover_order',
  'failover_max_retries',
  'failover_base_delay',
  'failover_spring_back',
]

export const mergePersistedFailoverConfig = (draftProfiles = [], persistedProfiles = []) => {
  const byIdentity = new Map()
  const bySlot = new Map()
  persistedProfiles.forEach((profile, profileIndex) => {
    profileModelConfigs(profile).forEach((config, configIndex) => {
      const metadata = Object.fromEntries(FAILOVER_FIELDS
        .filter(key => config[key] !== undefined)
        .map(key => [key, config[key]]))
      const identity = `${text(profile.var_name)}\0${modelIdOf(config)}`
      const matches = byIdentity.get(identity) || []
      matches.push(metadata)
      byIdentity.set(identity, matches)
      bySlot.set(`${profileIndex}:${configIndex}`, metadata)
    })
  })

  const consumed = new Map()
  return draftProfiles.map((profile, profileIndex) => withModelConfigs(
    profile,
    profileModelConfigs(profile).map((config, configIndex) => {
      const identity = `${text(profile.var_name)}\0${modelIdOf(config)}`
      const occurrence = consumed.get(identity) || 0
      const matches = byIdentity.get(identity)
      const metadata = matches?.[occurrence] || bySlot.get(`${profileIndex}:${configIndex}`) || {}
      consumed.set(identity, occurrence + 1)
      const next = { ...config }
      FAILOVER_FIELDS.forEach(key => delete next[key])
      return { ...next, ...metadata }
    }),
  ))
}

export const moveOrderedItem = (items = [], fromIndex, toIndex) => {
  if (
    !Number.isInteger(fromIndex)
    || !Number.isInteger(toIndex)
    || fromIndex < 0
    || toIndex < 0
    || fromIndex >= items.length
    || toIndex >= items.length
    || fromIndex === toIndex
  ) return items

  const next = [...items]
  const [moved] = next.splice(fromIndex, 1)
  next.splice(toIndex, 0, moved)
  return next
}

export const applyModelOrder = (profiles = [], orderedRows = []) => {
  const orderById = new Map(orderedRows.map((row, order) => [row.id, order]))
  return profiles.map((profile, profileIndex) => withModelConfigs(
    profile,
    profileModelConfigs(profile).map((config, configIndex) => {
      const sortOrder = orderById.get(`${profileIndex}:${configIndex}`)
      return sortOrder === undefined ? { ...config } : { ...config, sort_order: sortOrder }
    }),
  ))
}

export const applyModelAndFailoverOrder = (profiles = [], failoverGroups = [], orderedRows = []) => {
  const orderById = new Map(orderedRows.map((row, order) => [row.id, order]))
  
  // Apply order to model configs
  const nextProfiles = profiles.map((profile, profileIndex) => withModelConfigs(
    profile,
    profileModelConfigs(profile).map((config, configIndex) => {
      const sortOrder = orderById.get(`${profileIndex}:${configIndex}`)
      return sortOrder === undefined ? { ...config } : { ...config, sort_order: sortOrder }
    }),
  ))
  
  // Apply order to failover groups
  const nextFailoverGroups = (Array.isArray(failoverGroups) ? failoverGroups : []).map((group, groupIndex) => {
    const sortOrder = orderById.get(`failover:${groupIndex}`)
    return sortOrder === undefined ? { ...group } : { ...group, sort_order: sortOrder }
  })
  
  return { profiles: nextProfiles, failoverGroups: nextFailoverGroups }
}


export const orderedFailoverRows = (profiles = []) => orderedModelRows(profiles)
  .map(row => ({ ...row, config: profileModelConfigs(profiles[row.profileIndex])[row.configIndex] }))
  .filter(row => Number.isInteger(row.config.failover_order) && row.config.failover_order >= 0)
  .sort((left, right) => left.config.failover_order - right.config.failover_order)

export const applyFailoverConfig = (profiles = [], orderedRows = [], settings = {}) => {
  const orderById = new Map(orderedRows.map((row, order) => [row.id, order]))
  return profiles.map((profile, profileIndex) => withModelConfigs(
    profile,
    profileModelConfigs(profile).map((config, configIndex) => {
      const next = { ...config }
      const order = orderById.get(`${profileIndex}:${configIndex}`)
      if (order === undefined) {
        delete next.failover_order
        delete next.failover_max_retries
        delete next.failover_base_delay
        delete next.failover_spring_back
        return next
      }
      next.failover_order = order
      next.failover_max_retries = settings.maxRetries
      next.failover_base_delay = settings.baseDelay
      if (settings.springBack === undefined || settings.springBack === null || settings.springBack === '') {
        delete next.failover_spring_back
      } else {
        next.failover_spring_back = settings.springBack
      }
      return next
    }),
  ))
}
